package eval

import (
	"bytes"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// salesPath — путь к examples/data/sales.json (фича 026: каталог данных переехал
// data/ → examples/data/). Тесты eval бегут из каталога пакета internal/eval;
// поднимаемся к корню репо (..\..\..) и спускаемся в examples/data. Подставляем этот
// путь в SourceDecl.File после парса (loadSource принимает его как есть, §SM-8.1).
func salesPath() string { return filepath.Join("..", "..", "examples", "data", "sales.json") }

// buildMetricInterp парсит исходник декларатива src, переписывает путь файла
// ЕДИНСТВЕННОГО источника на абсолютный salesPath() (чтобы загрузка не зависела от
// cwd), прогоняет Analyze и возвращает интерпретатор с FixedClock{2026-05-31}.
func buildMetricInterp(t *testing.T, src string) *Interpreter {
	t.Helper()
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные лексические/синтаксические ошибки: %v", errList.Error())
	}
	// Переписать путь источника на абсолютный (golden использует фикстуру data/sales.json).
	for _, item := range prog.Items {
		if sd, ok := item.(*ast.SourceDecl); ok {
			sd.File.Value = salesPath()
		}
	}
	i := NewInterpreter(&bytes.Buffer{}, 0, testClock)
	if err := i.Analyze(prog); err != nil {
		t.Fatalf("Analyze вернул ошибку: %v", err)
	}
	return i
}

// goldenMetric строит фикстуру §SM-10 с подставляемыми атрибутами where/aggregate/
// period/byDate. Пустая строка атрибута — атрибут опускается (связку период↔по_дате
// держим согласованной в каждом кейсе).
func goldenMetric(where, aggregate, period, byDate string) string {
	src := "источник продажи:\n    файл: \"data/sales.json\"\n\nметрика m:\n    источник: продажи\n"
	if where != "" {
		src += "    где:      " + where + "\n"
	}
	src += "    агрегат:  " + aggregate + "\n"
	if period != "" {
		src += "    период:   " + period + "\n"
	}
	if byDate != "" {
		src += "    по_дате:  " + byDate + "\n"
	}
	return src
}

// evalGolden строит метрику m из атрибутов, вычисляет её и возвращает value.String.
func evalGolden(t *testing.T, where, aggregate, period, byDate string) string {
	t.Helper()
	i := buildMetricInterp(t, goldenMetric(where, aggregate, period, byDate))
	v, err := i.evalMetricByName("m", ast.Position{Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("evalMetric вернул ошибку: %v", err)
	}
	return value.String(v)
}

// evalMetricErr строит метрику m, переписывает путь источника на абсолютный,
// прогоняет Analyze (если он не падает) и вычисляет m, возвращая текст ошибки.
// Используется для краевых eval-кейсов §SM-9.B/C.
func evalMetricErr(t *testing.T, where, aggregate, period, byDate string) (msg string) {
	t.Helper()
	i := buildMetricInterp(t, goldenMetric(where, aggregate, period, byDate))
	_, err := i.evalMetricByName("m", ast.Position{Line: 1, Col: 1})
	if err == nil {
		t.Fatalf("ожидалась ошибка вычисления метрики, получено nil")
	}
	_, _, msg = evalErr(t, err)
	return msg
}

// analyzeMetricErr парсит декларатив src (БЕЗ переписи пути) и возвращает текст
// ошибки Analyze (§SM-9.A — статическая валидация деклараций).
func analyzeMetricErr(t *testing.T, src string) string {
	t.Helper()
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные лексические/синтаксические ошибки: %v", errList.Error())
	}
	i := NewInterpreter(&bytes.Buffer{}, 0, testClock)
	err := i.Analyze(prog)
	if err == nil {
		t.Fatalf("ожидалась ошибка Analyze, получено nil")
	}
	_, _, msg := evalErr(t, err)
	return msg
}

// T028: 7 краевых exact-match §SM-9 (тексты байт-точно).

// (1) период без по_дате → §SM-9.A (ловит Analyze, фаза D).
func TestEdgePeriodWithoutByDate(t *testing.T) {
	src := "источник продажи:\n    файл: \"data/sales.json\"\n\n" +
		"метрика m:\n    источник: продажи\n    агрегат:  сумма(сумма_заказа)\n    период:   ежемесячно\n"
	got := analyzeMetricErr(t, src)
	want := "метрика 'm': 'период' требует 'по_дате'"
	if got != want {
		t.Errorf("msg = %q\nхотим %q", got, want)
	}
}

// (2) неизвестный источник → §SM-9.A (Analyze).
func TestEdgeUnknownSource(t *testing.T) {
	src := "источник продажи:\n    файл: \"data/sales.json\"\n\n" +
		"метрика m:\n    источник: несуществующий\n    агрегат:  сумма(сумма_заказа)\n"
	got := analyzeMetricErr(t, src)
	want := "метрика 'm': источник 'несуществующий' не объявлен"
	if got != want {
		t.Errorf("msg = %q\nхотим %q", got, want)
	}
}

// (3) файл не найден → §SM-9.B (loadSource). Путь НЕ переписываем — используем битый.
func TestEdgeFileNotFound(t *testing.T) {
	src := "источник продажи:\n    файл: \"нет.json\"\n\n" +
		"метрика m:\n    источник: продажи\n    агрегат:  сумма(сумма_заказа)\n"
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные ошибки разбора: %v", errList.Error())
	}
	i := NewInterpreter(&bytes.Buffer{}, 0, testClock)
	if err := i.Analyze(prog); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	_, err := i.evalMetricByName("m", ast.Position{Line: 1, Col: 1})
	if err == nil {
		t.Fatalf("ожидалась ошибка загрузки")
	}
	_, _, msg := evalErr(t, err)
	want := "источник 'продажи': файл «нет.json» не найден"
	if msg != want {
		t.Errorf("msg = %q\nхотим %q", msg, want)
	}
}

// (4) где-не-Булево → §SM-9.C (ОшибкаТипа).
func TestEdgeWhereNotBool(t *testing.T) {
	got := evalMetricErr(t, "статус", "сумма(сумма_заказа)", "", "")
	want := "'где' должно давать Булево, получено Строка"
	if got != want {
		t.Errorf("msg = %q\nхотим %q", got, want)
	}
}

// (5) голое поле в агрегате → §SM-9.C.
func TestEdgeBareField(t *testing.T) {
	got := evalMetricErr(t, `статус == "оплачен"`, "сумма_заказа", "", "")
	want := "поле 'сумма_заказа' использовано вне агрегатной функции"
	if got != want {
		t.Errorf("msg = %q\nхотим %q", got, want)
	}
}

// (6) опечатка поля → §SM-9.C с отсортированным списком известных полей.
func TestEdgeFieldTypo(t *testing.T) {
	got := evalMetricErr(t, `статус_опечатка == "x"`, "сумма(сумма_заказа)", "", "")
	want := "неизвестное поле 'статус_опечатка' (известные: дата_заказа, клиент, статус, сумма_заказа)"
	if got != want {
		t.Errorf("msg = %q\nхотим %q", got, want)
	}
}

// (7) дата невалидна → §SM-9.C (конструктор дата(), фаза C).
func TestEdgeInvalidDate(t *testing.T) {
	got := evalMetricErr(t, `дата("2026-13-40") == дата("2026-13-40")`, "сумма(сумма_заказа)", "", "")
	want := "дата: «2026-13-40» не является датой"
	if got != want {
		t.Errorf("msg = %q\nхотим %q", got, want)
	}
}

// TestEdgeNestedAggregate — вложенный агрегат в проекции → §SM-9.C (приёмка T023).
// Внешний сумма(...) — корневой агрегат; его аргумент среднее(...) содержит вложенный
// агрегатный вызов → «вложенный агрегат недопустим» (findNestedAggregate, §10.3).
// где задаём валидным Булево, чтобы ошибка пришла именно от агрегата, а не от 'где'.
func TestEdgeNestedAggregate(t *testing.T) {
	got := evalMetricErr(t, `статус == "оплачен"`, "сумма(среднее(сумма_заказа))", "", "")
	want := "вложенный агрегат недопустим"
	if got != want {
		t.Errorf("msg = %q\nхотим %q", got, want)
	}
}

// TestSourceAsValue — имя источника в позиции значения → §SM-9.B (R6/FR-032).
func TestSourceAsValue(t *testing.T) {
	src := "источник продажи:\n    файл: \"data/sales.json\"\n\nпусть x = продажи\n"
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные ошибки разбора: %v", errList.Error())
	}
	i := NewInterpreter(&bytes.Buffer{}, 0, testClock)
	err := i.Run(prog)
	if err == nil {
		t.Fatalf("ожидалась ошибка: источник как значение")
	}
	_, _, msg := evalErr(t, err)
	want := "'продажи' — источник, его нельзя использовать как значение"
	if msg != want {
		t.Errorf("msg = %q\nхотим %q", msg, want)
	}
}

// TestMetricCycle — две взаимно-ссылающиеся метрики → §SM-9.B (цикл, D-8). Метрика
// a ссылается на b как на значение (в агрегате через глобальное имя), b — на a.
func TestMetricCycle(t *testing.T) {
	src := "источник продажи:\n    файл: \"data/sales.json\"\n\n" +
		"метрика a:\n    источник: продажи\n    агрегат:  b\n\n" +
		"метрика b:\n    источник: продажи\n    агрегат:  a\n"
	i := buildMetricInterp(t, src)
	_, err := i.evalMetricByName("a", ast.Position{Line: 1, Col: 1})
	if err == nil {
		t.Fatalf("ожидалась ошибка цикла зависимостей метрик")
	}
	_, _, msg := evalErr(t, err)
	want := "цикл зависимостей метрик: a → b → a"
	if msg != want {
		t.Errorf("msg = %q\nхотим %q", msg, want)
	}
}

// TestMetricAsValueReentrant — метрика-как-значение в агрегате вложенно меняет
// recordCtx; после её пересчёта recordCtx внешней метрики должен восстановиться
// (реентерабельность, D-9). Внешняя метрика total суммирует значение внутренней
// метрики paid (константа на всех записях) → 3 × 2000000 = 6000000.
func TestMetricAsValueReentrant(t *testing.T) {
	src := "источник продажи:\n    файл: \"data/sales.json\"\n\n" +
		"метрика paid:\n    источник: продажи\n    где:      статус == \"оплачен\"\n    агрегат:  сумма(сумма_заказа)\n\n" +
		"метрика total:\n    источник: продажи\n    агрегат:  сумма(paid)\n"
	i := buildMetricInterp(t, src)
	v, err := i.evalMetricByName("total", ast.Position{Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("evalMetric(total): %v", err)
	}
	if got := value.String(v); got != "6000000" {
		t.Errorf("total = %q, хотим 6000000", got)
	}
}

// TestMetricAsValueReentrantPeriodScope — контракт D-9/D9-1 на реентерабельном пути:
// период: вложенной метрики (достигнутой по имени из агрегат: внешней) вычисляется в
// ГЛОБАЛЬНОЙ области (recordCtx сброшен на входе в evalMetric), а НЕ в scope полей
// внешней записи. Внутренней метрике inner задаём период: с голым именем «статус» —
// это поле ВНЕШНЕЙ схемы (продажи). Без сброса recordCtx оно молча резолвилось бы в
// поле внешней записи (→ «'период' должно давать Период, получено Строка»); со сбросом
// — резолвится глобально и, не найдясь, даёт «'статус' не объявлено». Дополняет
// TestMetricAsValueReentrant (тот покрывает восстановление recordCtx без период:).
func TestMetricAsValueReentrantPeriodScope(t *testing.T) {
	src := "источник продажи:\n    файл: \"data/sales.json\"\n\n" +
		"метрика inner:\n    источник: продажи\n    агрегат:  сумма(сумма_заказа)\n    период:   статус\n    по_дате:  дата(дата_заказа)\n\n" +
		"метрика outer:\n    источник: продажи\n    агрегат:  сумма(inner)\n"
	i := buildMetricInterp(t, src)
	_, err := i.evalMetricByName("outer", ast.Position{Line: 1, Col: 1})
	if err == nil {
		t.Fatalf("ожидалась ошибка: голое имя «статус» в период: вложенной метрики")
	}
	_, _, msg := evalErr(t, err)
	want := "'статус' не объявлено"
	if msg != want {
		t.Errorf("msg = %q\nхотим %q (период: вложенной метрики должен резолвиться в глобальной области, не в поле внешней записи)", msg, want)
	}
}

// T027: golden §SM-10 — 15 строк байт-в-байт (FixedClock{2026-05-31} + data/sales.json),
// включая деривативные кейсы D4-1 (пустое окно → пусто; непустое → штатный расчёт).
func TestGoldenSM10(t *testing.T) {
	const (
		wherePaid = `статус == "оплачен"`
		sumOrders = `сумма(сумма_заказа)`
		byDate    = `дата(дата_заказа)`
	)
	cases := []struct {
		name      string
		where     string
		aggregate string
		period    string
		byDate    string
		want      string
	}{
		{"ежемесячно", wherePaid, sumOrders, "ежемесячно", byDate, "2000000"},
		{"ежеквартально", wherePaid, sumOrders, "ежеквартально", byDate, "2000000"},
		{"ежегодно", wherePaid, sumOrders, "ежегодно", byDate, "2000000"},
		{"еженедельно", wherePaid, sumOrders, "еженедельно", byDate, "0"},
		{"ежедневно", wherePaid, sumOrders, "ежедневно", byDate, "0"},
		{"без_периода", wherePaid, sumOrders, "", "", "2000000"},
		{"без_где", "", sumOrders, "ежемесячно", byDate, "2500000"},
		{"количество", wherePaid, "количество(запись)", "ежемесячно", byDate, "2"},
		{"среднее", wherePaid, "среднее(сумма_заказа)", "ежемесячно", byDate, "1000000.0"},
		{"мин", wherePaid, "мин(сумма_заказа)", "ежемесячно", byDate, "800000"},
		{"макс", wherePaid, "макс(сумма_заказа)", "ежемесячно", byDate, "1200000"},
		{"среднее_пустое_окно", wherePaid, "среднее(сумма_заказа)", "ежедневно", byDate, "пусто"},
		// D4-1: деривативное выражение на ПУСТОМ окне → Пусто коротким замыканием по
		// корню (НЕ 0/0 и НЕ Целое 1) — §SM-10, §SM-8 шаг 5.
		{"средний_чек_пустое_окно", wherePaid, "сумма(сумма_заказа) / количество(запись)", "ежедневно", byDate, "пусто"},
		{"сумма_плюс1_пустое_окно", wherePaid, "сумма(сумма_заказа) + 1", "ежедневно", byDate, "пусто"},
		// D4-1 контроль: НЕ-пустой деривативный путь по-прежнему вычисляется штатно —
		// сумма(2000000)/количество(2) = Дробное 1000000.0 (семантика «/» не меняется).
		{"средний_чек_ежемесячно", wherePaid, "сумма(сумма_заказа) / количество(запись)", "ежемесячно", byDate, "1000000.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := evalGolden(t, tc.where, tc.aggregate, tc.period, tc.byDate)
			if got != tc.want {
				t.Errorf("метрика(%s) = %q, хотим %q", tc.name, got, tc.want)
			}
		})
	}
}

// === 028: характеризационные замки числовых краевых веток combineBinary/
// combineUnary/arith через ДЕРИВАТИВ метрики (A1–A5). Окно непустое
// (paid+ежемесячно или единичная кастом-запись): на пустом окне корневой дериватив
// короткозамыкается в Пусто ДО combine* (metric_engine.go:79-81, D4-1). ===

// buildMetricInterpCustomSource записывает кастомную JSON-фикстуру во временный
// каталог, парсит источник «продажи» + метрику m с агрегатом aggregate, переписывает
// путь источника на временный файл (НЕ salesPath()), прогоняет Analyze и вычисляет m,
// возвращая (value.Value, error). Окно фиксированное: где: статус == "оплачен" +
// период: ежемесячно + по_дате: дата(дата_заказа) под FixedClock{2026-05-31} — записи
// фикстуры с датой в мае 2026 и статусом «оплачен» выживают (непустое окно). Нужен
// для A3 (поле = MinInt64) и A4 (поле = 1e300): стандартная sales.json их не несёт.
func buildMetricInterpCustomSource(t *testing.T, jsonRecords, aggregate string) (value.Value, error) {
	t.Helper()
	path := writeJSON(t, t.TempDir(), "f.json", jsonRecords)
	src := "источник продажи:\n    файл: \"f.json\"\n\n" +
		"метрика m:\n    источник: продажи\n" +
		"    где:      статус == \"оплачен\"\n" +
		"    агрегат:  " + aggregate + "\n" +
		"    период:   ежемесячно\n" +
		"    по_дате:  дата(дата_заказа)\n"
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные лексические/синтаксические ошибки: %v", errList.Error())
	}
	for _, item := range prog.Items {
		if sd, ok := item.(*ast.SourceDecl); ok {
			sd.File.Value = path
		}
	}
	i := NewInterpreter(&bytes.Buffer{}, 0, testClock)
	if err := i.Analyze(prog); err != nil {
		t.Fatalf("Analyze вернул ошибку: %v", err)
	}
	return i.evalMetricByName("m", ast.Position{Line: 1, Col: 1})
}

// customRecord — одна выживающая запись кастом-фикстуры с числовым полем field=val
// (val — JSON-токен дословно), датой мая 2026 и статусом «оплачен».
func customRecord(field, val string) string {
	return `[{"` + field + `": ` + val + `, "дата_заказа": "2026-05-15", "статус": "оплачен"}]`
}

// A1 (FR-001) — деление на ноль на НЕПУСТОМ окне через дериватив. Знаменатель =
// количество(запись) - количество(запись) = Целое 0. Три ветки: «/» (evalDiv, float),
// «//» (evalFloorDiv, целочисл.), «%» (evalMod, целочисл.). Ассерт: isRuntime +
// сообщение вербатим + позиция оператора (line>=1 && col>=1, Конст. IV) + не паника.
func TestMetricDivByZero(t *testing.T) {
	const where = `статус == "оплачен"`
	const byDate = `дата(дата_заказа)`
	cases := []struct {
		name      string
		op        string // оператор-литерал; позиция ошибки обязана указывать на него
		aggregate string
	}{
		{"div", "/", `сумма(сумма_заказа) / (количество(запись) - количество(запись))`},
		{"floordiv", "//", `сумма(сумма_заказа) // (количество(запись) - количество(запись))`},
		{"mod", "%", `сумма(сумма_заказа) % (количество(запись) - количество(запись))`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := goldenMetric(where, tc.aggregate, "ежемесячно", byDate)
			i := buildMetricInterp(t, src)
			_, err := i.evalMetricByName("m", ast.Position{Line: 1, Col: 1})
			if err == nil {
				t.Fatalf("ожидалась ошибка деления на ноль, получено nil")
			}
			if !isRuntime(err) {
				t.Fatalf("ошибка не ОшибкаВыполнения: %T %v", err, err)
			}
			line, col, msg := evalErr(t, err)
			if msg != "деление на ноль" {
				t.Errorf("msg = %q, хотим %q", msg, "деление на ноль")
			}
			// FR-001 «с номером строки»: позиция = токен оператора (BinaryExpr.Pos(),
			// §8.2). goldenMetric детерминирован: агрегат — строка 7 (источник:1, файл:2,
			// пусто:3, метрика:4, источник:5, где:6, агрегат:7). Колонка обязана указывать
			// на оператор tc.op. Краснеет при любом сдвиге позиции (мутпроба §SC-004).
			const wantLine = 7
			if line != wantLine {
				t.Errorf("line = %d, хотим %d (строка агрегата)", line, wantLine)
			}
			srcLines := strings.Split(src, "\n")
			if line < 1 || line > len(srcLines) {
				t.Fatalf("line %d вне источника (%d строк)", line, len(srcLines))
			}
			runes := []rune(srcLines[line-1])
			if col < 1 || col > len(runes) || !strings.HasPrefix(string(runes[col-1:]), tc.op) {
				t.Errorf("позиция (%d,%d) не указывает на оператор %q", line, col, tc.op)
			}
		})
	}
}

// A2 (FR-002) — переполнение целого на стандартной непустой фикстуре (сумма=2000000>0).
// add: сумма + MaxInt64 (addInt64); mul: сумма * MaxInt64 (mulInt64). Литерал
// 9223372036854775807 = MaxInt64 (в диапазоне int64). Ассерт: isRuntime + текст вербатим.
func TestMetricIntOverflow(t *testing.T) {
	const where = `статус == "оплачен"`
	const byDate = `дата(дата_заказа)`
	cases := []struct {
		name      string
		aggregate string
	}{
		{"add", `сумма(сумма_заказа) + 9223372036854775807`},
		{"mul", `сумма(сумма_заказа) * 9223372036854775807`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			i := buildMetricInterp(t, goldenMetric(where, tc.aggregate, "ежемесячно", byDate))
			_, err := i.evalMetricByName("m", ast.Position{Line: 1, Col: 1})
			if err == nil {
				t.Fatalf("ожидалась ошибка переполнения, получено nil")
			}
			if !isRuntime(err) {
				t.Fatalf("ошибка не ОшибкаВыполнения: %T %v", err, err)
			}
			_, _, msg := evalErr(t, err)
			if msg != "переполнение целого числа" {
				t.Errorf("msg = %q, хотим %q", msg, "переполнение целого числа")
			}
		})
	}
}

// A2-floordiv (FR-002) — floorDivInt64(MinInt64, -1) через кастом-фикстуру. Числитель
// мин(поле)=Целое{MinInt64}; знаменатель количество-количество-1 = 2-2-1 = -1 (НЕ 0,
// иначе сработал бы div-zero) → ловушка a==MinInt64 && b==-1 (arith.go:284). Поле
// -9223372036854775808 грузится строгим путём как Целое{MinInt64} без переполнения.
func TestMetricIntOverflowFloorDiv(t *testing.T) {
	v, err := buildMetricInterpCustomSource(t,
		customRecord("поле", "-9223372036854775808"),
		`мин(поле) // (количество(запись) - количество(запись) - 1)`)
	if err == nil {
		t.Fatalf("ожидалась ошибка переполнения, получено значение %v", v)
	}
	if !isRuntime(err) {
		t.Fatalf("ошибка не ОшибкаВыполнения: %T %v", err, err)
	}
	_, _, msg := evalErr(t, err)
	if msg != "переполнение целого числа" {
		t.Errorf("msg = %q, хотим %q", msg, "переполнение целого числа")
	}
}

// A3-neg-float (FR-003, ЯДРО combineUnary) — -(среднее(...)) над Дробным.
// среднее(сумма_заказа) на окне paid+ежемесячно = Дробное{1000000.0} →
// combineUnary(neg, Дробное) (metric_engine.go:293) → Дробное{-1000000.0}.
// Реально прогоняет combineUnary (закрывает 0% покрытия, SC-002).
func TestMetricUnaryNegFloat(t *testing.T) {
	i := buildMetricInterp(t, goldenMetric(`статус == "оплачен"`,
		`-(среднее(сумма_заказа))`, "ежемесячно", `дата(дата_заказа)`))
	v, err := i.evalMetricByName("m", ast.Position{Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("evalMetric вернул ошибку: %v", err)
	}
	if got := value.String(v); got != "-1000000.0" {
		t.Errorf("value.String = %q, хотим %q", got, "-1000000.0")
	}
	if _, ok := v.(value.Дробное); !ok {
		t.Errorf("тип = %T, хотим value.Дробное", v)
	}
}

// A3-neg-min (FR-003, ЯДРО combineUnary) — -(мин(поле)) над Целое{MinInt64}.
// Кастом-фикстура поле=-9223372036854775808 (грузится строгим путём как
// Целое{MinInt64}); мин(поле)=Целое{MinInt64} → combineUnary(neg, Целое) ветка
// v.V == math.MinInt64 (metric_engine.go:289) → переполнение. Реально прогоняет
// combineUnary (закрывает 0% покрытия, SC-002).
func TestMetricUnaryNegOverflow(t *testing.T) {
	v, err := buildMetricInterpCustomSource(t,
		customRecord("поле", "-9223372036854775808"), `-(мин(поле))`)
	if err == nil {
		t.Fatalf("ожидалась ошибка переполнения, получено значение %v", v)
	}
	if !isRuntime(err) {
		t.Fatalf("ошибка не ОшибкаВыполнения: %T %v", err, err)
	}
	_, _, msg := evalErr(t, err)
	if msg != "переполнение целого числа" {
		t.Errorf("msg = %q, хотим %q", msg, "переполнение целого числа")
	}
}

// A4 (FR-004) — пропагация ±Inf/NaN через combineBinary/combineUnary на кастом-
// фикстуре поле=1e300 (конечное; источник ОТВЕРГает 1e400 строгим путём). Результат
// во всех строках — value.Дробное (НЕ паника/None), спец-значение по IEEE-754.
// Проверка через math.IsInf/math.IsNaN (НЕ ==, NaN != NaN).
func TestMetricFloatSpecials(t *testing.T) {
	const records = `[{"огромное": 1e300, "дата_заказа": "2026-05-15", "статус": "оплачен"}]`
	t.Run("pinf", func(t *testing.T) {
		v, err := buildMetricInterpCustomSource(t, records, `среднее(огромное) * среднее(огромное)`)
		if err != nil {
			t.Fatalf("ожидалось значение, получена ошибка: %v", err)
		}
		d, ok := v.(value.Дробное)
		if !ok {
			t.Fatalf("тип = %T, хотим value.Дробное", v)
		}
		if !math.IsInf(d.V, +1) {
			t.Errorf("значение = %v, хотим +Inf", d.V)
		}
	})
	t.Run("ninf", func(t *testing.T) {
		v, err := buildMetricInterpCustomSource(t, records, `-(среднее(огромное) * среднее(огромное))`)
		if err != nil {
			t.Fatalf("ожидалось значение, получена ошибка: %v", err)
		}
		d, ok := v.(value.Дробное)
		if !ok {
			t.Fatalf("тип = %T, хотим value.Дробное", v)
		}
		if !math.IsInf(d.V, -1) {
			t.Errorf("значение = %v, хотим -Inf", d.V)
		}
	})
	t.Run("nan", func(t *testing.T) {
		v, err := buildMetricInterpCustomSource(t, records,
			`(среднее(огромное) * среднее(огромное)) - (среднее(огромное) * среднее(огромное))`)
		if err != nil {
			t.Fatalf("ожидалось значение, получена ошибка: %v", err)
		}
		d, ok := v.(value.Дробное)
		if !ok {
			t.Fatalf("тип = %T, хотим value.Дробное", v)
		}
		if !math.IsNaN(d.V) {
			t.Errorf("значение = %v, хотим NaN", d.V)
		}
	})
}

// A5 (FR-005) — операнд None/Пусто в combineUnary (default) и combineBinary
// (evalAdd type-mismatch). Метрика пусто_м с ПУСТЫМ окном (период: ежедневно →
// value.None по D4-1) читается как глобаль в деривативе внешней метрики на НЕПУСТОМ
// окне (паттерн TestMetricAsValueReentrant). НЕ путать None-операнд с пустым окном.
// Пусто.TypeName() == "Пусто" (подтверждено эмпирически).
func TestMetricNoneOperand(t *testing.T) {
	const head = "источник продажи:\n    файл: \"data/sales.json\"\n\n" +
		"метрика пусто_м:\n    источник: продажи\n    где:      статус == \"оплачен\"\n" +
		"    агрегат:  среднее(сумма_заказа)\n    период:   ежедневно\n    по_дате:  дата(дата_заказа)\n\n"
	cases := []struct {
		name      string
		aggregate string
		wantMsg   string
	}{
		{"unary", `-(пусто_м)`, "унарный '-' нельзя применить к Пусто"},
		{"binary", `сумма(сумма_заказа) + пусто_м`, "'+' нельзя применить к Целое и Пусто"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := head + "метрика m:\n    источник: продажи\n" +
				"    где:      статус == \"оплачен\"\n" +
				"    агрегат:  " + tc.aggregate + "\n" +
				"    период:   ежемесячно\n    по_дате:  дата(дата_заказа)\n"
			i := buildMetricInterp(t, src)
			_, err := i.evalMetricByName("m", ast.Position{Line: 1, Col: 1})
			if err == nil {
				t.Fatalf("ожидалась ОшибкаТипа (операнд Пусто), получено nil")
			}
			if !isType(err) {
				t.Fatalf("ошибка не ОшибкаТипа: %T %v", err, err)
			}
			_, _, msg := evalErr(t, err)
			if msg != tc.wantMsg {
				t.Errorf("msg = %q, хотим %q", msg, tc.wantMsg)
			}
		})
	}
}
