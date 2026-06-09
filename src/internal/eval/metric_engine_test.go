package eval

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// salesPath — абсолютный путь к data/sales.json (корень репозитория). Тесты eval
// бегут из каталога пакета internal/eval; data/sales.json лежит в корне репо —
// поднимаемся к нему (..\..\..). §SM-10 говорит про «data/sales.json относительно
// cwd», но cwd теста — каталог пакета; чтобы 12 строк golden прошли без os.Chdir,
// подставляем абсолютный путь в SourceDecl.File после парса (loadSource принимает
// абсолютный путь как есть, §SM-8.1).
func salesPath() string { return filepath.Join("..", "..", "..", "data", "sales.json") }

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
