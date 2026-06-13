package eval

import (
	"fmt"

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
		return i.checkTriggerBody(td.Body, false, false)
	}
	return nil
}

// checkTriggerBody обходит тело триггера как императивную область, семантически
// тождественную top-level (§TR-5): inFunction=false (вернуть вне функции → §TR-7.D),
// inStep=false (действия-шага запрещены → §TR-7.C, §PM-6.B), loopDepth=0.
// inMetricTrigger/inEventTrigger включают контекст-гард значение/событие (§TR-7.A).
// Зеркало analyzeStep: collectVars ловит дубль локальных пусть/для; vars/letLine
// засеваются пусто (локальные имена тела). Глобальные пусть видны рантайму, но в
// семпроходе плоский Ident не резолвится (analyze.go: declaredness — рантайму).
func (i *Interpreter) checkTriggerBody(body *ast.Block, inMetricTrigger bool, inEventTrigger bool) error {
	letLine := map[string]int{}
	vars := map[string]bool{}
	if err := collectVars(body.Stmts, letLine, vars); err != nil {
		return err
	}
	return i.checkStmts(body.Stmts, vars, false, false, 0, inMetricTrigger, inEventTrigger)
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
	return i.checkStmts(step.Body, vars, false, true, 0, false, false)
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
	return i.checkStmts(stmts, vars, inFunction, false, loopDepth, false, false)
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
func (i *Interpreter) checkStmts(stmts []ast.Statement, vars map[string]bool, inFunction bool, inStep bool, loopDepth int, inMetricTrigger bool, inEventTrigger bool) error {
	for _, s := range stmts {
		if err := i.checkStmt(s, vars, inFunction, inStep, loopDepth, inMetricTrigger, inEventTrigger); err != nil {
			return err
		}
	}
	return nil
}

func (i *Interpreter) checkStmt(s ast.Statement, vars map[string]bool, inFunction bool, inStep bool, loopDepth int, inMetricTrigger bool, inEventTrigger bool) error {
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
		if err := i.checkStmts(st.Then.Stmts, vars, inFunction, inStep, loopDepth, inMetricTrigger, inEventTrigger); err != nil {
			return err
		}
		return i.checkElse(st.Else, vars, inFunction, inStep, loopDepth, inMetricTrigger, inEventTrigger)
	case *ast.WhileStmt:
		if err := i.checkExpr(st.Cond, vars, inMetricTrigger, inEventTrigger); err != nil {
			return err
		}
		return i.checkStmts(st.Body.Stmts, vars, inFunction, inStep, loopDepth+1, inMetricTrigger, inEventTrigger)
	case *ast.ForStmt:
		if err := i.checkExpr(st.Iterable, vars, inMetricTrigger, inEventTrigger); err != nil {
			return err
		}
		return i.checkStmts(st.Body.Stmts, vars, inFunction, inStep, loopDepth+1, inMetricTrigger, inEventTrigger)
	case *ast.AssignAction, *ast.CallAction, *ast.NotifyAction:
		// Контекст-гард действий (§PM-4, D-11): вне шага — СемантическаяОшибка
		// §PM-6.B; в шаге валидно, payload (Args/Value) НЕ обходится (резолв/
		// арность/deferred аргументов — рантайму 006); рантайм-deferred
		// (stmt.go:64) в 005 недостижим — тело шага не исполняется (§PM-5).
		if !inStep {
			return semErr(st.Pos(), fmt.Sprintf("действие '%s' допустимо только в шаге процесса", constructName(st)))
		}
		return nil
	}
	return nil
}

func (i *Interpreter) checkElse(e *ast.ElseClause, vars map[string]bool, inFunction bool, inStep bool, loopDepth int, inMetricTrigger bool, inEventTrigger bool) error {
	if e == nil {
		return nil
	}
	if e.IsFinal() {
		return i.checkStmts(e.Body.Stmts, vars, inFunction, inStep, loopDepth, inMetricTrigger, inEventTrigger)
	}
	if err := i.checkExpr(e.Cond, vars, inMetricTrigger, inEventTrigger); err != nil {
		return err
	}
	if err := i.checkStmts(e.Then.Stmts, vars, inFunction, inStep, loopDepth, inMetricTrigger, inEventTrigger); err != nil {
		return err
	}
	return i.checkElse(e.Else, vars, inFunction, inStep, loopDepth, inMetricTrigger, inEventTrigger)
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
// deferred-встроенная → SEM-DEFERRED-BUILTIN; иначе → SEM-FN-UNDECLARED.
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
