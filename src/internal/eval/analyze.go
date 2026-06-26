package eval

import (
	"fmt"
	"strconv"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// Analyze — семантический проход стадии 3 (§9, FR-034/035). Лёгкий fail-fast
// обход AST ДО исполнения: регистрирует top-level функции (forward-вызовы
// разрешены), источники, метрики и процессы, валидирует метрики (Шаг 1b) и
// процессы (Шаг 1c), проверяет повтор объявлений, контекст вернуть/прервать/
// продолжить, контекст-гард действий (присвоить/вызвать/уведомить — только в
// теле шага), резолв вызовов + ФИКС. арность и deferred-границу. НЕ проверяет:
// declaredness переменной в позиции значения (flow-зависимо), типы, арность
// вариативных/перегруженных встроенных.
func (i *Interpreter) Analyze(prog *ast.Program) error {
	i.analyzed = true

	// Шаг 1 — объединённый упорядоченный проход по top-level декларациям
	// (FunctionDecl+SourceDecl+MetricDecl+ProcessDecl) в общем глобальном
	// пространстве имён (§SM-4, §PM-4/D-5). declLine — строка первого объявления
	// имени; declIsFunc — было ли первое объявление функцией (для различения
	// текста повтора, R5).
	declLine := map[string]int{}
	declIsFunc := map[string]bool{}
	for _, item := range prog.Items {
		var name string
		var pos ast.Position
		isFunc := false
		switch d := item.(type) {
		case *ast.FunctionDecl:
			name, pos, isFunc = d.Name.Name, d.Name.Pos(), true
		case *ast.SourceDecl:
			name, pos = d.Name.Name, d.Name.Pos()
		case *ast.MetricDecl:
			name, pos = d.Name.Name, d.Name.Pos()
		case *ast.ProcessDecl:
			name, pos = d.Name.Name, d.Name.Pos()
		default:
			continue
		}
		if line, exists := declLine[name]; exists {
			// функция↔функция → старый текст (регресс 003, R5); любая коллизия
			// с участием источника/метрики → общий §SM-9.A текст.
			if isFunc && declIsFunc[name] {
				return semErr(pos, fmt.Sprintf("функция '%s' уже объявлена в строке %d", name, line))
			}
			return semErr(pos, fmt.Sprintf("'%s' уже объявлено в строке %d", name, line))
		}
		declLine[name] = pos.Line
		declIsFunc[name] = isFunc
		switch d := item.(type) {
		case *ast.FunctionDecl:
			i.funcs[name] = d
		case *ast.SourceDecl:
			i.sources[name] = d
			if err := i.checkReservedDeclName(name, d.Pos()); err != nil {
				return err
			}
		case *ast.MetricDecl:
			i.metrics[name] = d
			if err := i.checkReservedDeclName(name, d.Pos()); err != nil {
				return err
			}
		case *ast.ProcessDecl:
			i.processes[name] = d
			if err := i.checkReservedDeclName(name, d.Pos()); err != nil {
				return err
			}
		}
	}

	// Шаг 1a' — статическая валидация источников (010-A1, §SC-4-sem): множество
	// значений тип:, обязательность поля: для csv/ndjson, множество типов полей.
	// Статически (без чтения файла), ДО валидации метрик; тексты §SC-9.A дословно.
	for _, item := range prog.Items {
		sd, ok := item.(*ast.SourceDecl)
		if !ok {
			continue
		}
		if err := checkSourceDecl(sd); err != nil {
			return err
		}
	}

	// Шаг 1b — статическая валидация метрик: обязательность источник/агрегат,
	// связка период↔по_дате, резолв источника (§SM-4, §SM-9.A). НЕ резолвит поля
	// записи/типы/голое поле/цикл — это eval-time (D-5/D-6/D-8).
	for _, item := range prog.Items {
		md, ok := item.(*ast.MetricDecl)
		if !ok {
			continue
		}
		if err := i.checkMetricDecl(md); err != nil {
			return err
		}
	}

	// Шаг 1c — структурная валидация процессов: уникальность шагов, резолв
	// 'после' (только строго назад), 'срок' без 'исполнитель' (§PM-4, §PM-6.B).
	for _, item := range prog.Items {
		pd, ok := item.(*ast.ProcessDecl)
		if !ok {
			continue
		}
		if err := i.checkProcessDecl(pd); err != nil {
			return err
		}
	}

	// Шаг 1d — семантическая валидация триггеров (зеркало Шага 1b/1c, §TR-4):
	// резолв метрики (метрики уже зарегистрированы Шагом 1), проверка порога, обход
	// тела по виду. Регистрация в i.triggers — в порядке prog.Items (детерминизм
	// fire-if-true-прохода §TR-6, FR-012). У триггера нет имени верхнего уровня —
	// namespace-проверки Шага 1 его не касаются.
	for _, item := range prog.Items {
		td, ok := item.(*ast.TriggerDecl)
		if !ok {
			continue
		}
		if err := i.checkTrigger(td); err != nil {
			return err
		}
		i.triggers = append(i.triggers, td)
	}

	// Шаг 2 — обход глобальной области и тел функций (блоки область не открывают).
	var globalStmts []ast.Statement
	for _, item := range prog.Items {
		if st, ok := item.(ast.Statement); ok {
			globalStmts = append(globalStmts, st)
		}
	}
	if err := i.analyzeArea(globalStmts, nil, false, 0); err != nil {
		return err
	}
	for _, item := range prog.Items {
		if fd, ok := item.(*ast.FunctionDecl); ok {
			if err := i.analyzeArea(fd.Body.Stmts, fd.Params, true, 0); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkMetricDecl — статическая валидация одной метрики (Шаг 1b, §SM-4). Порядок:
// (1) обязательность источник/агрегат; (2) пара период↔по_дате; (3) резолв
// источника. Тексты §SM-9.A дословно; fail-fast. Поля/типы/голое поле/цикл не
// резолвятся — eval-time. Присутствие атрибута определяется по ненулевой строке
// его позиции в Attrs (парсер выставляет Line!=0 ровно для присутствующих).
func (i *Interpreter) checkMetricDecl(m *ast.MetricDecl) error {
	name := m.Name.Name
	// (1) обязательные атрибуты источник/агрегат (поз. токена метрика).
	if m.Attrs.SourcePos.Line == 0 {
		return semErr(m.Pos(), fmt.Sprintf("метрика '%s': отсутствует обязательный атрибут 'источник'", name))
	}
	if m.Attrs.AggregatePos.Line == 0 {
		return semErr(m.Pos(), fmt.Sprintf("метрика '%s': отсутствует обязательный атрибут 'агрегат'", name))
	}
	// (2) связка период↔по_дате.
	hasPeriod := m.Attrs.PeriodPos.Line != 0
	hasByDate := m.Attrs.ByDatePos.Line != 0
	if hasPeriod && !hasByDate {
		return semErr(m.Attrs.PeriodPos, fmt.Sprintf("метрика '%s': 'период' требует 'по_дате'", name))
	}
	if hasByDate && !hasPeriod {
		return semErr(m.Attrs.ByDatePos, fmt.Sprintf("метрика '%s': 'по_дате' без 'период' не имеет смысла", name))
	}
	// (2b) статические проверки оконных форм period: (011-A2, §MW-SEM-1/2/3). Обход
	// AST m.Period: WindowPeriodLit (скользящее «последние N <ед>») и
	// LastCompletedPeriodLit («прошлый <noun>»). Адверб-константа v1 (Ident) — без
	// проверок (множество гарантировано предрегистрацией). A2-4 (по_дате) — НЕ
	// дублируется (покрыто связкой выше). Позиция — period attr (PeriodPos). Тексты
	// §MW-8.A byte-identical.
	switch lit := m.Period.(type) {
	case *ast.WindowPeriodLit:
		// §MW-SEM-1: единица окна ∈ {дн,нед,мес}.
		if lit.Unit != "дн" && lit.Unit != "нед" && lit.Unit != "мес" {
			return semErr(m.Attrs.PeriodPos, fmt.Sprintf("метрика '%s': единица '%s' недопустима для окна (допустимо: дн, нед, мес)", name, lit.Unit))
		}
		// §MW-SEM-2: размер окна N ≥ 1. Amount — нормализованная цифровая строка из
		// DURATION (цифры без знака). РАЗДЕЛЕНЫ два случая (self-check Ф8): ParseInt-err
		// ⟺ переполнение int64 (N положителен, лишь вне диапазона) → «слишком велик»;
		// успешный парс с N < 1 (т.е. N == 0, «0дн»/«00дн») → «должен быть положительным».
		n, err := strconv.ParseInt(lit.Amount, 10, 64)
		if err != nil {
			return semErr(m.Attrs.PeriodPos, fmt.Sprintf("метрика '%s': размер окна слишком велик", name))
		}
		if n < 1 {
			return semErr(m.Attrs.PeriodPos, fmt.Sprintf("метрика '%s': размер окна должен быть положительным", name))
		}
	case *ast.LastCompletedPeriodLit:
		// §MW-SEM-3: noun завершённого периода ∈ {день,неделя,месяц,квартал,год}.
		switch lit.Noun {
		case "день", "неделя", "месяц", "квартал", "год":
		default:
			return semErr(m.Attrs.PeriodPos, fmt.Sprintf("метрика '%s': неизвестный период '%s' (допустимо: день, неделя, месяц, квартал, год)", name, lit.Noun))
		}
	}
	// (3) резолв источника: имя должно быть зарегистрированным источником.
	src := m.Source.Name
	if _, ok := i.sources[src]; !ok {
		if _, fn := i.funcs[src]; fn {
			return semErr(m.Attrs.SourcePos, fmt.Sprintf("метрика '%s': '%s' — не источник", name, src))
		}
		if _, mt := i.metrics[src]; mt {
			return semErr(m.Attrs.SourcePos, fmt.Sprintf("метрика '%s': '%s' — не источник", name, src))
		}
		return semErr(m.Attrs.SourcePos, fmt.Sprintf("метрика '%s': источник '%s' не объявлен", name, src))
	}
	return nil
}

// fieldTypeNames — допустимое множество аннотаций поля: (A1-4, §SC-5). Порядок
// фиксирован для текста диагностики «неизвестный тип поля» (§SC-9.A).
var fieldTypeNames = []string{"Целое", "Дробное", "Строка", "Логическое", "Дата"}

// isFieldTypeName — принадлежность имени множеству допустимых аннотаций поля.
func isFieldTypeName(name string) bool {
	for _, t := range fieldTypeNames {
		if name == t {
			return true
		}
	}
	return false
}

// checkSourceDecl — статическая валидация одного источника (Шаг 1a', §SC-4-sem,
// 010-A1). Порядок (fail-fast): (1) значение тип: ∈ {json,csv,ndjson} (пусто=json
// ок) — иначе semErr с поз. TypePos; (2) csv/ndjson требуют поля: (A1-3) — иначе
// semErr с поз. TypePos; (3) каждый тип поля ∈ {Целое,Дробное,Строка,Логическое,
// Дата} — иначе semErr с поз. FieldDef.Pos. Дубли имён полей ловит парсер
// (§SC-4-sem п.4) — здесь НЕ дублируется. Тексты §SC-9.A byte-identical; без
// чтения файла.
func checkSourceDecl(sd *ast.SourceDecl) error {
	name := sd.Name.Name
	typ := sd.Type.Name
	// (1) множество значений тип: (пусто ≡ json, A1-2).
	if typ != "" && typ != "json" && typ != "csv" && typ != "ndjson" {
		pos := sd.TypePos
		if pos.Line == 0 {
			pos = sd.Pos()
		}
		return semErr(pos, fmt.Sprintf("источник '%s': неизвестный тип источника '%s' (допустимо: json, csv, ndjson)", name, typ))
	}
	// (2) поля: обязательно для csv/ndjson (A1-3).
	if (typ == "csv" || typ == "ndjson") && len(sd.Fields) == 0 {
		pos := sd.TypePos
		if pos.Line == 0 {
			pos = sd.Pos()
		}
		return semErr(pos, fmt.Sprintf("источник '%s': тип '%s' требует объявления полей (поля:)", name, typ))
	}
	// (3) множество типов полей (A1-4).
	for _, fd := range sd.Fields {
		if !isFieldTypeName(fd.TypeName.Name) {
			return semErr(fd.Pos, fmt.Sprintf("источник '%s': неизвестный тип поля '%s' (допустимо: Целое, Дробное, Строка, Логическое, Дата)", name, fd.TypeName.Name))
		}
	}
	return nil
}

// checkProcessDecl — статическая валидация одного процесса (Шаг 1c, §PM-4).
// Порядок (fail-fast): (1) уникальность имён шагов (свой namespace процесса, D-5);
// (2) резолв 'после' — ссылка только строго назад (j < i), ацикличность по
// построению, топосорт НЕ делается (D-4, зарезервирован под v2); (3) 'срок' без
// 'исполнитель' — позиция на строке срок: (DeadlinePos), не на начале шага;
// (4) анализ тел шагов — analyzeStep (D-12). Тексты §PM-6.B дословно. Выражения
// атрибутов шага НЕ обходятся (D-11).
func (i *Interpreter) checkProcessDecl(pd *ast.ProcessDecl) error {
	// (1) уникальность шагов: имя → индекс первого объявления.
	stepIdx := map[string]int{}
	for idx, step := range pd.Steps {
		name := step.Name.Name
		if first, ok := stepIdx[name]; ok {
			return semErr(step.Name.Pos(), fmt.Sprintf("шаг '%s' уже объявлен в строке %d", name, pd.Steps[first].Name.Pos().Line))
		}
		stepIdx[name] = idx
	}
	// (2) резолв 'после': каждый X шага S обязан быть объявлен строго раньше.
	for idx, step := range pd.Steps {
		for _, x := range step.After {
			j, ok := stepIdx[x.Name]
			if !ok {
				return semErr(x.Pos(), fmt.Sprintf("шаг '%s' после '%s', но шаг '%s' не объявлен", step.Name.Name, x.Name, x.Name))
			}
			if j >= idx {
				return semErr(x.Pos(), fmt.Sprintf("шаг '%s' после '%s', но '%s' объявлен позже", step.Name.Name, x.Name, x.Name))
			}
		}
	}
	// (3) 'срок' без 'исполнитель' не имеет эффекта (§11.4).
	for _, step := range pd.Steps {
		if step.Attrs.DeadlinePos.Line != 0 && step.Attrs.AssigneePos.Line == 0 {
			return semErr(step.Attrs.DeadlinePos, fmt.Sprintf("шаг '%s': срок без исполнитель не имеет эффекта", step.Name.Name))
		}
	}
	// (4) анализ тел шагов (D-12).
	for _, step := range pd.Steps {
		if err := i.analyzeStep(step, pd.Params); err != nil {
			return err
		}
	}
	return nil
}

// checkTrigger — диспетчер семантической валидации одного триггера по виду
// (Шаг 1d, §TR-4). Метрика-триггер: резолв имени метрики против i.metrics
// (§TR-7.B), проверка порога обычным checkExpr (резолв вызовов/арность; тип-чек
// «число vs число» — рантайм, FR-022), обход тела с inMetricTrigger=true.
// Событие-триггер: имя события в 007a НЕ резолвится (реестра событий нет — 007b),
// тело валидируется полностью с inEventTrigger=true (FR-022). Расписание-триггер:
// содержимое строки "ЧЧ:ММ" НЕ анализируется (FR-005, → 007b), тело с обоими
// флагами false. Тексты §TR-7 дословно; fail-fast.
func (i *Interpreter) checkTrigger(td *ast.TriggerDecl) error {
	switch spec := td.Spec.(type) {
	case *ast.MetricTrigger:
		// (1) резолв метрики: имя обязано быть зарегистрированной метрикой.
		// Различающий текст: имя занято не-метрикой → TR-MET-NOTMETRIC, иначе
		// TR-MET-UNDECL (зеркало checkRunProcess/checkMetricDecl).
		name := spec.Metric.Name
		if _, ok := i.metrics[name]; !ok {
			if _, fn := i.funcs[name]; fn {
				return semErr(spec.Metric.Pos(), fmt.Sprintf("'%s' — не метрика", name))
			}
			if _, src := i.sources[name]; src {
				return semErr(spec.Metric.Pos(), fmt.Sprintf("'%s' — не метрика", name))
			}
			if _, pr := i.processes[name]; pr {
				return semErr(spec.Metric.Pos(), fmt.Sprintf("'%s' — не метрика", name))
			}
			return semErr(spec.Metric.Pos(), fmt.Sprintf("метрика '%s' не объявлена", name))
		}
		// (8) порог — обычное выражение (значение/событие в нём недопустимы → оба false).
		if err := i.checkExpr(spec.Threshold, map[string]bool{}, false, false); err != nil {
			return err
		}
		return i.checkTriggerBody(td.Body, true, false)
	case *ast.EventTrigger:
		return i.checkTriggerBody(td.Body, false, true)
	case *ast.ScheduleTrigger:
		// SE-TIME-FORMAT (007b, R-8/FR-014): формат строки «в "ЧЧ:ММ"» — это точка,
		// где 007a отложил проверку содержимого строки (→ 007b). Аддитивно: синтаксис/
		// AST/реестр диагностик 007a не меняются (§TR-11). Проверки ДО обхода тела
		// (fail-fast, зеркало резолва метрики выше).
		if at, ok := spec.Spec.(*ast.AtSchedule); ok {
			if err := checkTimeFormat(at.At); err != nil {
				return err
			}
		}
		// SCHED-1: интервал «каждые <amount><ед>» должен быть строго положительным —
		// иначе next==LastFire (или next в прошлом) → busy-fire на каждом тике (для тела
		// «запустить процесс» = лавина инстансов; см. shiftEvery/checkEvery демона).
		// Единиценезависимо: плейсхолдер — Amount+Unit (как scheduleName, trigger_run.go),
		// работает одинаково для сек/мин/час/дн и календарных нед/мес. Amount —
		// нормализованная лексемой строка цифр без знака, так что n<=0 ⟺ ноль во всех
		// формах («0»/«00»). Переполнение int64/time.Duration огромным, но положительным
		// amount — осознанный край v1 (data-model.md §SCHED-2), здесь не ловится.
		if ev, ok := spec.Spec.(*ast.EverySchedule); ok {
			if n, err := strconv.ParseInt(ev.Every.Amount, 10, 64); err == nil && n <= 0 {
				return semErr(ev.Every.Pos(), fmt.Sprintf("интервал расписания должен быть положительным: 'каждые %s'", ev.Every.Amount+ev.Every.Unit))
			}
		}
		return i.checkTriggerBody(td.Body, false, false)
	case *ast.DeadlineTrigger:
		// Эскалация-триггер (016 B4a, §AU-6.1.3): (а) процесс объявлен; (б) шаг
		// существует в процессе; (в) тело — как РАСПИСАНИЕ (lenient-scope, D-AU-6):
		// оба контекст-флага false. Свободный `факт` в теле статической ошибки не
		// даёт — резолвится в рантайме против инжекта InstanceVariables (B4b).
		pd, ok := i.processes[spec.Process.Name]
		if !ok {
			return semErr(spec.Process.Pos(), fmt.Sprintf("процесс '%s' не объявлен", spec.Process.Name))
		}
		found := false
		for _, st := range pd.Steps {
			if st.Name.Name == spec.Step.Name {
				found = true
				break
			}
		}
		if !found {
			return semErr(spec.Step.Pos(), fmt.Sprintf("шаг '%s' не найден в процессе '%s'", spec.Step.Name, spec.Process.Name))
		}
		return i.checkTriggerBody(td.Body, false, false)
	}
	return nil
}

// checkTimeFormat валидирует строку времени суток подформы «в "ЧЧ:ММ"» (SE-TIME-
// FORMAT, R-8). Правила (FR-014): ровно 5 рун, руна[2]==':', руны [0][1][3][4] —
// десятичные цифры, часы 00–23, минуты 00–59 (обязательные ведущие нули: «9:05»/
// «09:5» невалидны как длина≠5). Проверка посимвольная по рунам, БЕЗ regex
// (Принцип II). Нарушение → СемантическаяОшибка (semErr) с позицией токена строки
// (Принцип IV), двухстрочный канон §13. Текст — единый для всех нарушений
// (импл-факт, diagnostics.md): различение причин не несёт пользы, формат целиком.
func checkTimeFormat(at ast.StringLit) error {
	r := []rune(at.Value)
	bad := func() error {
		return semErr(at.Pos(), fmt.Sprintf("неверный формат времени '%s': ожидается \"ЧЧ:ММ\" (часы 00–23, минуты 00–59)", at.Value))
	}
	if len(r) != 5 || r[2] != ':' {
		return bad()
	}
	for _, idx := range [4]int{0, 1, 3, 4} {
		if r[idx] < '0' || r[idx] > '9' {
			return bad()
		}
	}
	hh := int(r[0]-'0')*10 + int(r[1]-'0')
	mm := int(r[3]-'0')*10 + int(r[4]-'0')
	if hh > 23 || mm > 59 {
		return bad()
	}
	return nil
}

// checkTriggerBody обходит тело триггера как императивную область, семантически
// тождественную top-level (§TR-5): inFunction=false (вернуть вне функции → §TR-7.D),
// inStep=false, loopDepth=0, inTriggerBody=true. Последний открывает allow-список
// §AU-6.1.3 (общий для метрика/событие/расписание/дедлайн): `вызвать`/`уведомить`
// в теле триггера РАЗРЕШЕНЫ (durable-golden §AU-12.B), а `присвоить` — НЕТ (не «ядро»
// + read-only барьер D-AU-6). Действия-шага `исполнитель:`/`срок:` структурно
// невозможны вне шага (атрибуты StepDecl). inMetricTrigger/inEventTrigger включают
// контекст-гард значение/событие (§TR-7.A).
// Зеркало analyzeStep: collectVars ловит дубль локальных пусть/для; vars/letLine
// засеваются пусто (локальные имена тела). Глобальные пусть видны рантайму, но в
// семпроходе плоский Ident не резолвится (analyze.go: declaredness — рантайму).
func (i *Interpreter) checkTriggerBody(body *ast.Block, inMetricTrigger bool, inEventTrigger bool) error {
	letLine := map[string]int{}
	vars := map[string]bool{}
	if err := collectVars(body.Stmts, letLine, vars); err != nil {
		return err
	}
	return i.checkStmts(body.Stmts, vars, false, false, 0, inMetricTrigger, inEventTrigger, true)
}

// analyzeStep анализирует тело одного шага как императивную область (D-12, §PM-4):
// inStep=true, inFunction=false, loopDepth=0. Отличия от analyzeArea: vars
// засевается параметрами процесса (чтение/вызов параметра → рантайм, не
// «не объявлено»), letLine параметрами НЕ засевается — 'пусть' с именем параметра
// в шаге разрешён (теняет, §6.4 — отличие от тела функции). collectVars ловит
// дубль шаг-локальных 'пусть'/'для' общим текстом.
func (i *Interpreter) analyzeStep(step *ast.StepDecl, params []ast.Ident) error {
	letLine := map[string]int{}
	vars := map[string]bool{}
	for _, p := range params {
		vars[p.Name] = true
	}
	if err := collectVars(step.Body, letLine, vars); err != nil {
		return err
	}
	return i.checkStmts(step.Body, vars, false, true, 0, false, false, false)
}

// checkReservedDeclName запрещает имени источника/метрики/процесса совпадать с
// зарезервированным (§SM-4, §SM-9.A, ревью №1 C1-1): встроенной функцией (активной
// или deferred — туда же входят 5 агрегатов сумма/количество/среднее/мин/макс) либо
// предопределённым периодом (value.PeriodNames). Позиция — ведущий токен декларации
// (decl.Pos()). Пользовательских функций НЕ касается: для них затенение
// встроенного — предупреждение (SPEC §6.5), а не жёсткий запрет.
func (i *Interpreter) checkReservedDeclName(name string, pos ast.Position) error {
	if _, ok := i.builtins[name]; ok {
		return semErr(pos, fmt.Sprintf("имя '%s' зарезервировано встроенной функцией", name))
	}
	for _, p := range value.PeriodNames {
		if name == p {
			return semErr(pos, fmt.Sprintf("имя '%s' зарезервировано предопределённым периодом", name))
		}
	}
	return nil
}

// analyzeArea анализирует одну область (кадр функции / global). Два подпрохода:
// (1) собрать множество имён-переменных области и поймать повтор пусть/для;
// (2) проверить контексты сигналов, резолв вызовов и deferred-узлы.
func (i *Interpreter) analyzeArea(stmts []ast.Statement, params []ast.Ident, inFunction bool, loopDepth int) error {
	letLine := map[string]int{} // имя пусть/параметра → строка первого объявления
	vars := map[string]bool{}   // ВСЕ имена-переменные области (пусть+параметры+для)
	for _, p := range params {
		letLine[p.Name] = p.Pos().Line
		vars[p.Name] = true
	}
	if err := collectVars(stmts, letLine, vars); err != nil {
		return err
	}
	return i.checkStmts(stmts, vars, inFunction, false, loopDepth, false, false, false)
}

// collectVars рекурсивно собирает переменные области (нисходя в блоки, но не
// открывая новых областей) и ловит повтор объявления (SEM-REDECL-VAR). Переменная
// для НЕ добавляется в letLine — два «для x» подряд легальны (D-R13), но «пусть x»
// при объявленном x (и наоборот) — ошибка.
func collectVars(stmts []ast.Statement, letLine map[string]int, vars map[string]bool) error {
	for _, s := range stmts {
		switch st := s.(type) {
		case *ast.LetStmt:
			name := st.Name.Name
			if line, ok := letLine[name]; ok {
				return semErr(st.Name.Pos(), fmt.Sprintf("'%s' уже объявлено в строке %d", name, line))
			}
			letLine[name] = st.Name.Pos().Line
			vars[name] = true
		case *ast.ForStmt:
			name := st.Var.Name
			if line, ok := letLine[name]; ok {
				return semErr(st.Var.Pos(), fmt.Sprintf("'%s' уже объявлено в строке %d", name, line))
			}
			vars[name] = true
			if err := collectVars(st.Body.Stmts, letLine, vars); err != nil {
				return err
			}
		case *ast.IfStmt:
			if err := collectVars(st.Then.Stmts, letLine, vars); err != nil {
				return err
			}
			if err := collectVarsElse(st.Else, letLine, vars); err != nil {
				return err
			}
		case *ast.WhileStmt:
			if err := collectVars(st.Body.Stmts, letLine, vars); err != nil {
				return err
			}
		case *ast.TryStmt:
			// 029: оба арма пытаться/словить — обычные блоки; `пусть` в них видны
			// дальше (как у если/пока, скоуп блока не вводит новых границ).
			if err := collectVars(st.Try.Stmts, letLine, vars); err != nil {
				return err
			}
			if err := collectVars(st.Catch.Stmts, letLine, vars); err != nil {
				return err
			}
		}
	}
	return nil
}

func collectVarsElse(e *ast.ElseClause, letLine map[string]int, vars map[string]bool) error {
	if e == nil {
		return nil
	}
	if e.IsFinal() {
		return collectVars(e.Body.Stmts, letLine, vars)
	}
	if err := collectVars(e.Then.Stmts, letLine, vars); err != nil {
		return err
	}
	return collectVarsElse(e.Else, letLine, vars)
}

// checkStmts — второй подпроход: контексты сигналов, вызовы, deferred-узлы.
// inMetricTrigger/inEventTrigger — контекст тела триггера (зеркало inStep, §TR-5):
// разрешают значение/событие в checkExpr; на top-level/в функции/в шаге оба false.
func (i *Interpreter) checkStmts(stmts []ast.Statement, vars map[string]bool, inFunction bool, inStep bool, loopDepth int, inMetricTrigger bool, inEventTrigger bool, inTriggerBody bool) error {
	for _, s := range stmts {
		if err := i.checkStmt(s, vars, inFunction, inStep, loopDepth, inMetricTrigger, inEventTrigger, inTriggerBody); err != nil {
			return err
		}
	}
	return nil
}

func (i *Interpreter) checkStmt(s ast.Statement, vars map[string]bool, inFunction bool, inStep bool, loopDepth int, inMetricTrigger bool, inEventTrigger bool, inTriggerBody bool) error {
	switch st := s.(type) {
	case *ast.LetStmt:
		return i.checkExpr(st.Value, vars, inMetricTrigger, inEventTrigger)
	case *ast.AssignStmt:
		return i.checkExpr(st.Value, vars, inMetricTrigger, inEventTrigger)
	case *ast.ExpressionStmt:
		return i.checkExpr(st.Expr, vars, inMetricTrigger, inEventTrigger)
	case *ast.ReturnStmt:
		if !inFunction {
			// §7.3 / §PM-6.B: в шаге процесса — двухконтекстный текст (базовый
			// 003 + суффикс-подсказка); вне шага (в т.ч. тело триггера, §TR-7.D) —
			// только базовый текст 003.
			msg := "'вернуть' допустимо только внутри функции"
			if inStep {
				msg += "; в шаге процесса используйте 'присвоить'"
			}
			return semErr(st.Pos(), msg)
		}
		if st.Value != nil {
			return i.checkExpr(st.Value, vars, inMetricTrigger, inEventTrigger)
		}
		return nil
	case *ast.BreakStmt:
		if loopDepth == 0 {
			return semErr(st.Pos(), "'прервать' допустимо только внутри 'пока' или 'для'")
		}
		return nil
	case *ast.ContinueStmt:
		if loopDepth == 0 {
			return semErr(st.Pos(), "'продолжить' допустимо только внутри 'пока' или 'для'")
		}
		return nil
	case *ast.IfStmt:
		if err := i.checkExpr(st.Cond, vars, inMetricTrigger, inEventTrigger); err != nil {
			return err
		}
		if err := i.checkStmts(st.Then.Stmts, vars, inFunction, inStep, loopDepth, inMetricTrigger, inEventTrigger, inTriggerBody); err != nil {
			return err
		}
		return i.checkElse(st.Else, vars, inFunction, inStep, loopDepth, inMetricTrigger, inEventTrigger, inTriggerBody)
	case *ast.WhileStmt:
		if err := i.checkExpr(st.Cond, vars, inMetricTrigger, inEventTrigger); err != nil {
			return err
		}
		return i.checkStmts(st.Body.Stmts, vars, inFunction, inStep, loopDepth+1, inMetricTrigger, inEventTrigger, inTriggerBody)
	case *ast.ForStmt:
		if err := i.checkExpr(st.Iterable, vars, inMetricTrigger, inEventTrigger); err != nil {
			return err
		}
		return i.checkStmts(st.Body.Stmts, vars, inFunction, inStep, loopDepth+1, inMetricTrigger, inEventTrigger, inTriggerBody)
	case *ast.TryStmt:
		// 029: try/catch не меняет контекст сигналов — `вернуть`/`прервать`/
		// `продолжить` проходят насквозь (loopDepth/inFunction/inStep неизменны).
		if err := i.checkStmts(st.Try.Stmts, vars, inFunction, inStep, loopDepth, inMetricTrigger, inEventTrigger, inTriggerBody); err != nil {
			return err
		}
		return i.checkStmts(st.Catch.Stmts, vars, inFunction, inStep, loopDepth, inMetricTrigger, inEventTrigger, inTriggerBody)
	case *ast.CallAction, *ast.NotifyAction:
		// Контекст-гард действий (§PM-4, D-11): вне шага — СемантическаяОшибка
		// §PM-6.B; в шаге валидно, payload (Args/Value) НЕ обходится (резолв/
		// арность/deferred аргументов — рантайму 006); рантайм-deferred
		// (stmt.go:64) в 005 недостижим — тело шага не исполняется (§PM-5).
		// В ТЕЛЕ ТРИГГЕРА (§AU-6.1.3, гард общий для метрика/событие/расписание/
		// дедлайн): `вызвать`/`уведомить` РАЗРЕШЕНЫ (allow-список «императивное ядро +
		// уведомить/вызвать/запустить процесс») — без них durable-golden §AU-12.B
		// `уведомить руководитель(факт)` недостижим. Действия-шага `исполнитель:`/`срок:`
		// структурно невозможны вне шага (атрибуты StepDecl, не операторы — process.go);
		// `присвоить` (AssignAction) — НЕ в allow-списке (не «ядро») + тело-env read-only
		// (D-AU-6) → отдельная ветка ниже отвергает его и в теле триггера.
		if !inStep && !inTriggerBody {
			return semErr(st.Pos(), fmt.Sprintf("действие '%s' допустимо только в шаге процесса", constructName(st)))
		}
		return nil
	case *ast.AssignAction:
		// `присвоить` вне шага запрещено всегда: вне тела триггера — §PM-6.B; в теле
		// триггера — НЕ в allow-списке §AU-6.1.3 + read-only барьер тела (D-AU-6,
		// TR-BODY-RO). Текст один и тот же (контекст-гард действий, переиспользуется).
		if !inStep {
			return semErr(st.Pos(), fmt.Sprintf("действие '%s' допустимо только в шаге процесса", constructName(st)))
		}
		return nil
	}
	return nil
}

func (i *Interpreter) checkElse(e *ast.ElseClause, vars map[string]bool, inFunction bool, inStep bool, loopDepth int, inMetricTrigger bool, inEventTrigger bool, inTriggerBody bool) error {
	if e == nil {
		return nil
	}
	if e.IsFinal() {
		return i.checkStmts(e.Body.Stmts, vars, inFunction, inStep, loopDepth, inMetricTrigger, inEventTrigger, inTriggerBody)
	}
	if err := i.checkExpr(e.Cond, vars, inMetricTrigger, inEventTrigger); err != nil {
		return err
	}
	if err := i.checkStmts(e.Then.Stmts, vars, inFunction, inStep, loopDepth, inMetricTrigger, inEventTrigger, inTriggerBody); err != nil {
		return err
	}
	return i.checkElse(e.Else, vars, inFunction, inStep, loopDepth, inMetricTrigger, inEventTrigger, inTriggerBody)
}

// checkExpr рекурсивно проверяет резолв CallExpr + фикс. арность и RunProcessExpr
// (checkRunProcess, §PM-4). DurationLit семантически чист (006/D-7: литерал валиден
// в любой позиции, значение даёт рантайм). Плоский Ident НЕ проверяется
// (declaredness — рантайму). inMetricTrigger/inEventTrigger — контекст тела
// триггера (§TR-5): включают контекст-гард значение/событие (§TR-7.A).
func (i *Interpreter) checkExpr(e ast.Expression, vars map[string]bool, inMetricTrigger bool, inEventTrigger bool) error {
	switch ex := e.(type) {
	case *ast.BinaryExpr:
		if err := i.checkExpr(ex.Left, vars, inMetricTrigger, inEventTrigger); err != nil {
			return err
		}
		return i.checkExpr(ex.Right, vars, inMetricTrigger, inEventTrigger)
	case *ast.UnaryExpr:
		return i.checkExpr(ex.Operand, vars, inMetricTrigger, inEventTrigger)
	case *ast.CallExpr:
		for _, a := range ex.Args {
			if err := i.checkExpr(a, vars, inMetricTrigger, inEventTrigger); err != nil {
				return err
			}
		}
		return i.checkCall(ex, vars, inMetricTrigger, inEventTrigger)
	case *ast.IndexExpr:
		if err := i.checkExpr(ex.Target, vars, inMetricTrigger, inEventTrigger); err != nil {
			return err
		}
		return i.checkExpr(ex.Index, vars, inMetricTrigger, inEventTrigger)
	case *ast.FieldExpr:
		// событие.поле: FieldExpr.Target = *ast.EventExpr → тот же контекст-гард ниже.
		return i.checkExpr(ex.Target, vars, inMetricTrigger, inEventTrigger)
	case *ast.ListLit:
		for _, el := range ex.Elements {
			if err := i.checkExpr(el, vars, inMetricTrigger, inEventTrigger); err != nil {
				return err
			}
		}
		return nil
	case *ast.RunProcessExpr:
		return i.checkRunProcess(ex, vars, inMetricTrigger, inEventTrigger)
	case *ast.ValueExpr:
		// §TR-7.A: 'значение' допустимо ТОЛЬКО в теле метрика-триггера.
		if !inMetricTrigger {
			return semErr(ex.Pos(), "выражение 'значение' допустимо только в теле триггера метрики")
		}
		return nil
	case *ast.EventExpr:
		// §TR-7.A: 'событие' допустимо ТОЛЬКО в теле событие-триггера.
		if !inEventTrigger {
			return semErr(ex.Pos(), "выражение 'событие' допустимо только в теле триггера события")
		}
		return nil
	}
	return nil
}

// checkCall резолвит вызов на семпроходе (§9): имя-переменная области → рантайм;
// пользовательская/активная встроенная фикс. арности → проверка числа аргументов;
// deferred-встроенная → SEM-DEFERRED-BUILTIN (инертный backstop, активных deferred
// нет с 008/§DB-6); иначе → SEM-FN-UNDECLARED.
func (i *Interpreter) checkCall(c *ast.CallExpr, vars map[string]bool, inMetricTrigger bool, inEventTrigger bool) error {
	id, ok := c.Callee.(*ast.Ident)
	if !ok {
		return i.checkExpr(c.Callee, vars, inMetricTrigger, inEventTrigger)
	}
	name := id.Name
	if vars[name] {
		return nil // затенение переменной → решает рантайм (§5.2)
	}
	if decl, ok := i.funcs[name]; ok {
		if len(c.Args) != len(decl.Params) {
			return semErr(c.Pos(), fmt.Sprintf("'%s' принимает %d аргументов, передано %d", name, len(decl.Params), len(c.Args)))
		}
		return nil
	}
	if b, ok := i.builtins[name]; ok {
		// backstop: при пустом deferredNames недостижим; поле Deferred и механизм
		// НЕ удаляются (008/§DB-6, D-DB-якорь).
		if b.Deferred {
			return semErr(c.Pos(), fmt.Sprintf("функция '%s' не поддерживается в этой версии", name))
		}
		if b.Arity == ArityFixed && len(c.Args) != b.N {
			return semErr(c.Pos(), fmt.Sprintf("'%s' принимает %d аргументов, передано %d", name, b.N, len(c.Args)))
		}
		return nil // вариативная/перегруженная — арность в рантайме
	}
	return semErr(c.Pos(), fmt.Sprintf("функция '%s' не объявлена", name))
}

// checkRunProcess резолвит 'запустить процесс' на семпроходе (§PM-4, D-10):
// аргументы — args-first, fail-fast (как checkCall); имя — ТОЛЬКО против реестра
// процессов i.processes (НЕ vars, НЕ builtins — синтаксис фиксирован 'запустить
// процесс Ident'; имя встроенной падает в общий «не объявлен», осознанно);
// найден → фикс. арность. Тексты §PM-6.C; позиция — токен запустить (r.Pos()).
// Реестр готов с Шага 1 → работает в любой области (глобаль/функция/шаг).
// Исполнение запуска остаётся рантайм-deferred (expr.go:49, §PM-5).
func (i *Interpreter) checkRunProcess(r *ast.RunProcessExpr, vars map[string]bool, inMetricTrigger bool, inEventTrigger bool) error {
	for _, a := range r.Args {
		if err := i.checkExpr(a, vars, inMetricTrigger, inEventTrigger); err != nil {
			return err
		}
	}
	name := r.Process.Name
	if pd, ok := i.processes[name]; ok {
		if len(r.Args) != len(pd.Params) {
			return semErr(r.Pos(), fmt.Sprintf("'%s' принимает %d аргументов, передано %d", name, len(pd.Params), len(r.Args)))
		}
		return nil
	}
	if _, ok := i.funcs[name]; ok {
		return semErr(r.Pos(), fmt.Sprintf("'%s' — функция, не процесс", name))
	}
	if _, ok := i.metrics[name]; ok {
		return semErr(r.Pos(), fmt.Sprintf("'%s' — не процесс", name))
	}
	if _, ok := i.sources[name]; ok {
		return semErr(r.Pos(), fmt.Sprintf("'%s' — не процесс", name))
	}
	return semErr(r.Pos(), fmt.Sprintf("процесс '%s' не объявлен", name))
}
