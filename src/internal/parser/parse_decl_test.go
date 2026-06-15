package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/lexer"
)

// T034: функции (объявление, параметры, рекурсия, голый возврат), SE-NESTED-FN,
// зарезервированные StepAction/RunProcessExpr, примеры функция/факториал parse-clean.

func TestFunctionDeclWithParam(t *testing.T) {
	prog, el := parseProgramSrc(t, "функция факториал(n):\n    вернуть n\n")
	if !el.Empty() {
		t.Fatalf("ошибки: %v", el.Error())
	}
	fd, ok := prog.Items[0].(*ast.FunctionDecl)
	if !ok {
		t.Fatalf("Items[0] = %T, хотим *FunctionDecl", prog.Items[0])
	}
	if fd.Name.Name != "факториал" || len(fd.Params) != 1 || fd.Params[0].Name != "n" {
		t.Errorf("декларация неверна: name=%q params=%v", fd.Name.Name, fd.Params)
	}
}

func TestFunctionParamsTrailingComma(t *testing.T) {
	prog, el := parseProgramSrc(t, "функция f(a, b, c,):\n    вернуть a\n")
	if !el.Empty() {
		t.Fatalf("ошибки: %v", el.Error())
	}
	fd := prog.Items[0].(*ast.FunctionDecl)
	if len(fd.Params) != 3 {
		t.Fatalf("параметров %d, хотим 3 (висящая запятая)", len(fd.Params))
	}
}

func TestRecursiveCallInReturn(t *testing.T) {
	prog, el := parseProgramSrc(t, "функция факториал(n):\n    вернуть n * факториал(n - 1)\n")
	if !el.Empty() {
		t.Fatalf("ошибки: %v", el.Error())
	}
	fd := prog.Items[0].(*ast.FunctionDecl)
	ret, ok := fd.Body.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("тело[0] = %T, хотим *ReturnStmt", fd.Body.Stmts[0])
	}
	if sexpr(ret.Value) != "(* n (call факториал (- n 1)))" {
		t.Errorf("Value = %s, хотим (* n (call факториал (- n 1)))", sexpr(ret.Value))
	}
}

func TestBareReturnInFunction(t *testing.T) {
	prog, el := parseProgramSrc(t, "функция f():\n    вернуть\n")
	if !el.Empty() {
		t.Fatalf("ошибки: %v", el.Error())
	}
	fd := prog.Items[0].(*ast.FunctionDecl)
	ret := fd.Body.Stmts[0].(*ast.ReturnStmt)
	if ret.Value != nil {
		t.Errorf("голый вернуть: Value = %v, хотим nil", ret.Value)
	}
}

func TestNestedFunctionError(t *testing.T) {
	src := "функция f():\n    функция g():\n        вернуть 1\n"
	prog, el := parseProgramSrc(t, src)
	if el.Len() != 1 {
		t.Fatalf("ошибок %d, хотим 1 (SE-NESTED-FN): %v", el.Len(), el.Error())
	}
	pe := firstParseError(t, el)
	if pe.Msg != msgNestedFn {
		t.Errorf("Msg = %q, хотим %q", pe.Msg, msgNestedFn)
	}
	if pe.Pos.Line != 2 || pe.Pos.Col != 5 {
		t.Errorf("позиция SE-NESTED-FN = %+v, хотим {2,5} (токен функция)", pe.Pos)
	}
	// внешняя функция всё же построена
	if _, ok := prog.Items[0].(*ast.FunctionDecl); !ok {
		t.Errorf("внешняя функция не построена: %T", prog.Items[0])
	}
}

func TestStepActionsBuild(t *testing.T) {
	src := "если c:\n    присвоить x = 5\n    вызвать f(1, 2)\n    уведомить g(a)\n"
	prog, el := parseProgramSrc(t, src)
	if !el.Empty() {
		t.Fatalf("ошибки: %v", el.Error())
	}
	body := prog.Items[0].(*ast.IfStmt).Then.Stmts
	if len(body) != 3 {
		t.Fatalf("операторов в шаге: %d, хотим 3", len(body))
	}
	if aa, ok := body[0].(*ast.AssignAction); !ok || aa.Name.Name != "x" {
		t.Errorf("body[0] не AssignAction(x): %T", body[0])
	}
	if ca, ok := body[1].(*ast.CallAction); !ok || ca.Name.Name != "f" || len(ca.Args) != 2 {
		t.Errorf("body[1] не CallAction(f, 2 арг): %T", body[1])
	}
	if na, ok := body[2].(*ast.NotifyAction); !ok || na.Name.Name != "g" {
		t.Errorf("body[2] не NotifyAction(g): %T", body[2])
	}
}

func TestRunProcessExprBuilds(t *testing.T) {
	// в составе выражения
	prog, el := parseProgramSrc(t, "пусть id = запустить процесс Отчёт(2024)\n")
	if !el.Empty() {
		t.Fatalf("ошибки: %v", el.Error())
	}
	ls := prog.Items[0].(*ast.LetStmt)
	if sexpr(ls.Value) != "(run Отчёт 2024)" {
		t.Errorf("Value = %s, хотим (run Отчёт 2024)", sexpr(ls.Value))
	}

	// без скобок (опциональны)
	prog2, el2 := parseProgramSrc(t, "запустить процесс P\n")
	if !el2.Empty() {
		t.Fatalf("ошибки: %v", el2.Error())
	}
	es := prog2.Items[0].(*ast.ExpressionStmt)
	if _, ok := es.Expr.(*ast.RunProcessExpr); !ok {
		t.Errorf("Expr = %T, хотим *RunProcessExpr", es.Expr)
	}
}

func TestExamplesParseCleanP2b(t *testing.T) {
	for _, name := range []string{"функция.ladix", "факториал.ladix"} {
		t.Run(name, func(t *testing.T) {
			prog, el, lexErrs := parseExampleFile(t, name)
			if !lexErrs.Empty() {
				t.Fatalf("%s: лексические ошибки: %v", name, lexErrs.Error())
			}
			if !el.Empty() {
				t.Fatalf("%s: синтаксические ошибки: %v", name, el.Error())
			}
			if len(prog.Items) == 0 {
				t.Errorf("%s: пустой Program.Items", name)
			}
		})
	}
}

// --- T009/T010/T011/T013: декларативный слой 004 (источник/метрика) ---

// T009: позитивный разбор источника с атрибутом файл.
func TestSourceDeclParse(t *testing.T) {
	prog, el := parseProgramSrc(t, "источник продажи:\n    файл: \"data/sales.json\"\n")
	if !el.Empty() {
		t.Fatalf("ошибки: %v", el.Error())
	}
	sd, ok := prog.Items[0].(*ast.SourceDecl)
	if !ok {
		t.Fatalf("Items[0] = %T, хотим *SourceDecl", prog.Items[0])
	}
	if sd.Name.Name != "продажи" || sd.File.Value != "data/sales.json" {
		t.Errorf("источник неверен: name=%q file=%q", sd.Name.Name, sd.File.Value)
	}
}

// T009: неизвестный атрибут источника → §SM-9.A «неизвестный атрибут '<имя>'».
func TestSourceDeclUnknownAttr(t *testing.T) {
	prog, el := parseProgramSrc(t, "источник продажи:\n    путь: \"x.json\"\n")
	if el.Len() == 0 {
		t.Fatalf("ожидалась ошибка неизвестного атрибута")
	}
	pe := firstParseError(t, el)
	want := "неизвестный атрибут 'путь'"
	if pe.Msg != want {
		t.Errorf("Msg = %q, хотим %q", pe.Msg, want)
	}
	if pe.Pos.Line != 2 || pe.Pos.Col != 5 {
		t.Errorf("позиция = %+v, хотим {2,5} (имя атрибута)", pe.Pos)
	}
	_ = prog
}

// T009: повтор файл → §SM-9.A «атрибут 'файл' уже задан».
func TestSourceDeclDuplicateFile(t *testing.T) {
	src := "источник продажи:\n    файл: \"a.json\"\n    файл: \"b.json\"\n"
	_, el := parseProgramSrc(t, src)
	if el.Len() == 0 {
		t.Fatalf("ожидалась ошибка дубликата файл")
	}
	pe := firstParseError(t, el)
	want := "атрибут 'файл' уже задан"
	if pe.Msg != want {
		t.Errorf("Msg = %q, хотим %q", pe.Msg, want)
	}
	if pe.Pos.Line != 3 || pe.Pos.Col != 5 {
		t.Errorf("позиция = %+v, хотим {3,5} (повтор файл)", pe.Pos)
	}
}

// T006 (010-A1, §SC-D-PARSE-2/3, конституция VI): ТЕСТ-ЗАМОК позитивов парсера
// источника — тип: голый IDENT и поля: вложенный блок объявлений.
func TestSourceDeclTypeAndFieldsParse(t *testing.T) {
	t.Run("тип: csv → Type.Name=csv, TypePos.Line!=0", func(t *testing.T) {
		src := "источник заказы:\n    файл: \"data/orders.csv\"\n    тип: csv\n"
		prog, el := parseProgramSrc(t, src)
		if !el.Empty() {
			t.Fatalf("ошибки: %v", el.Error())
		}
		sd := prog.Items[0].(*ast.SourceDecl)
		if sd.Type.Name != "csv" {
			t.Errorf("Type.Name = %q, хотим \"csv\"", sd.Type.Name)
		}
		if sd.TypePos.Line == 0 {
			t.Errorf("TypePos.Line = 0, хотим presence (!= 0)")
		}
		if sd.TypePos.Line != 3 || sd.TypePos.Col != 5 {
			t.Errorf("TypePos = %+v, хотим {3,5} (слово тип)", sd.TypePos)
		}
	})
	t.Run("поля: блок ≥2 строк → срез FieldDef", func(t *testing.T) {
		src := "источник заказы:\n" +
			"    файл: \"data/orders.csv\"\n" +
			"    тип: csv\n" +
			"    поля:\n" +
			"        сумма: Дробное\n" +
			"        статус: Строка\n"
		prog, el := parseProgramSrc(t, src)
		if !el.Empty() {
			t.Fatalf("ошибки: %v", el.Error())
		}
		sd := prog.Items[0].(*ast.SourceDecl)
		if sd.FieldsPos.Line == 0 {
			t.Errorf("FieldsPos.Line = 0, хотим presence (!= 0)")
		}
		if len(sd.Fields) != 2 {
			t.Fatalf("Fields = %d, хотим 2", len(sd.Fields))
		}
		if sd.Fields[0].Name.Name != "сумма" || sd.Fields[0].TypeName.Name != "Дробное" {
			t.Errorf("Fields[0] = %+v, хотим {сумма, Дробное}", sd.Fields[0])
		}
		if sd.Fields[0].Pos.Line != 5 || sd.Fields[0].Pos.Col != 9 {
			t.Errorf("Fields[0].Pos = %+v, хотим {5,9}", sd.Fields[0].Pos)
		}
		if sd.Fields[1].Name.Name != "статус" || sd.Fields[1].TypeName.Name != "Строка" {
			t.Errorf("Fields[1] = %+v, хотим {статус, Строка}", sd.Fields[1])
		}
		if sd.Fields[1].Pos.Line != 6 {
			t.Errorf("Fields[1].Pos.Line = %d, хотим 6", sd.Fields[1].Pos.Line)
		}
	})
	t.Run("v1-форма (только файл:) → нулевые Type/Fields", func(t *testing.T) {
		src := "источник продажи:\n    файл: \"data/sales.json\"\n"
		prog, el := parseProgramSrc(t, src)
		if !el.Empty() {
			t.Fatalf("ошибки: %v", el.Error())
		}
		sd := prog.Items[0].(*ast.SourceDecl)
		if sd.Type.Name != "" || sd.TypePos.Line != 0 {
			t.Errorf("Type = %+v / TypePos = %+v, хотим нулевые (v1 → json)", sd.Type, sd.TypePos)
		}
		if sd.Fields != nil || sd.FieldsPos.Line != 0 {
			t.Errorf("Fields = %v / FieldsPos = %+v, хотим nil/нулевую", sd.Fields, sd.FieldsPos)
		}
	})
}

// T007 (010-A1, §SC-D-PARSE-3/spec Edge Cases, конституция VI): ТЕСТ-ЗАМОК
// негативов парсера источника — дубль тип:, дубль имени поля, пустой поля:,
// неизвестный атрибут источника. Все тексты — канон §SM-9.A.
func TestSourceDeclNegatives(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		want      string
		line, col int
	}{
		{
			name: "дубль тип:",
			src: "источник з:\n    файл: \"o.csv\"\n" +
				"    тип: csv\n    тип: json\n",
			want: "атрибут 'тип' уже задан",
			line: 4, col: 5,
		},
		{
			name: "дубль имени поля в поля:",
			src: "источник з:\n    файл: \"o.csv\"\n    тип: csv\n" +
				"    поля:\n        сумма: Дробное\n        сумма: Целое\n",
			want: "поле 'сумма' уже объявлено",
			line: 6, col: 9,
		},
		{
			name: "пустой поля: (INDENT сразу DEDENT)",
			src: "источник з:\n    файл: \"o.csv\"\n    тип: csv\n" +
				"    поля:\n    тип: ndjson\n",
			want: msgEmptyBlock,
			line: 5, col: 5,
		},
		{
			name: "неизвестный атрибут источника",
			src:  "источник з:\n    файл: \"o.json\"\n    адрес: \"x\"\n",
			want: "неизвестный атрибут 'адрес'",
			line: 3, col: 5,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, el := parseProgramSrc(t, c.src)
			if el.Len() == 0 {
				t.Fatalf("ожидалась ошибка %q", c.want)
			}
			pe := firstParseError(t, el)
			if pe.Msg != c.want {
				t.Errorf("Msg = %q, хотим %q", pe.Msg, c.want)
			}
			if pe.Pos.Line != c.line || pe.Pos.Col != c.col {
				t.Errorf("позиция = %+v, хотим {%d,%d}", pe.Pos, c.line, c.col)
			}
		})
	}
}

// T010: позитивный разбор полной метрики (5 атрибутов).
func TestMetricDeclParseFull(t *testing.T) {
	src := "метрика выручка_месяца:\n" +
		"    источник: продажи\n" +
		"    где:      статус == \"оплачен\"\n" +
		"    агрегат:  сумма(сумма_заказа)\n" +
		"    период:   ежемесячно\n" +
		"    по_дате:  дата(дата_заказа)\n"
	prog, el := parseProgramSrc(t, src)
	if !el.Empty() {
		t.Fatalf("ошибки: %v", el.Error())
	}
	md, ok := prog.Items[0].(*ast.MetricDecl)
	if !ok {
		t.Fatalf("Items[0] = %T, хотим *MetricDecl", prog.Items[0])
	}
	if md.Name.Name != "выручка_месяца" || md.Source.Name != "продажи" {
		t.Errorf("метрика: name=%q source=%q", md.Name.Name, md.Source.Name)
	}
	if md.Where == nil || md.Aggregate == nil || md.Period == nil || md.ByDate == nil {
		t.Errorf("все 4 выражения должны быть не-nil: %+v", md)
	}
	// Позиции ключевых слов атрибутов заполнены.
	if md.Attrs.SourcePos.Line != 2 || md.Attrs.WherePos.Line != 3 ||
		md.Attrs.AggregatePos.Line != 4 || md.Attrs.PeriodPos.Line != 5 ||
		md.Attrs.ByDatePos.Line != 6 {
		t.Errorf("позиции атрибутов: %+v", md.Attrs)
	}
}

// T010: метрика только с обязательными атрибутами — опциональные nil.
func TestMetricDeclParseMinimal(t *testing.T) {
	src := "метрика всего:\n" +
		"    источник: продажи\n" +
		"    агрегат:  количество(запись)\n"
	prog, el := parseProgramSrc(t, src)
	if !el.Empty() {
		t.Fatalf("ошибки: %v", el.Error())
	}
	md := prog.Items[0].(*ast.MetricDecl)
	if md.Where != nil || md.Period != nil || md.ByDate != nil {
		t.Errorf("опциональные атрибуты должны быть nil: %+v", md)
	}
	if md.Aggregate == nil {
		t.Errorf("агрегат обязателен (не-nil)")
	}
}

// T011: неизвестный атрибут метрики → §SM-9.A.
func TestMetricDeclUnknownAttr(t *testing.T) {
	src := "метрика m:\n    источник: продажи\n    лишний: 1\n"
	_, el := parseProgramSrc(t, src)
	if el.Len() == 0 {
		t.Fatalf("ожидалась ошибка неизвестного атрибута метрики")
	}
	pe := firstParseError(t, el)
	want := "неизвестный атрибут 'лишний'"
	if pe.Msg != want {
		t.Errorf("Msg = %q, хотим %q", pe.Msg, want)
	}
	if pe.Pos.Line != 3 || pe.Pos.Col != 5 {
		t.Errorf("позиция = %+v, хотим {3,5}", pe.Pos)
	}
}

// T011: повтор атрибута метрики → §SM-9.A «атрибут '<имя>' уже задан».
func TestMetricDeclDuplicateAttr(t *testing.T) {
	src := "метрика m:\n    источник: a\n    источник: b\n"
	_, el := parseProgramSrc(t, src)
	if el.Len() == 0 {
		t.Fatalf("ожидалась ошибка дубликата атрибута")
	}
	pe := firstParseError(t, el)
	want := "атрибут 'источник' уже задан"
	if pe.Msg != want {
		t.Errorf("Msg = %q, хотим %q", pe.Msg, want)
	}
	if pe.Pos.Line != 3 || pe.Pos.Col != 5 {
		t.Errorf("позиция = %+v, хотим {3,5} (повтор)", pe.Pos)
	}
}

// T011: источник: не IDENT → §SM-9.A «ожидается имя источника».
func TestMetricDeclSourceNotIdent(t *testing.T) {
	src := "метрика m:\n    источник: \"продажи\"\n    агрегат: сумма(x)\n"
	_, el := parseProgramSrc(t, src)
	if el.Len() == 0 {
		t.Fatalf("ожидалась ошибка «ожидается имя источника»")
	}
	pe := firstParseError(t, el)
	want := "ожидается имя источника"
	if pe.Msg != want {
		t.Errorf("Msg = %q, хотим %q", pe.Msg, want)
	}
	if pe.Pos.Line != 2 {
		t.Errorf("позиция = %+v, хотим строку 2 (токен после источник:)", pe.Pos)
	}
}

// --- 011-A2 (Phase C): спец-парс ветки период: — окна метрик ---

// metricWithPeriod собирает минимальную метрику с заданным значением период:
// (источник + период + агрегат), чтобы изолировать parsePeriodValue.
func metricWithPeriod(periodValue string) string {
	return "метрика m:\n" +
		"    источник: продажи\n" +
		"    период: " + periodValue + "\n" +
		"    агрегат: сумма(x)\n"
}

// T007 (011-A2 §MW-D-PARSE-2/4, FR-001/FR-006/FR-009): ПОЗИТИВНЫЙ разбор трёх форм
// период: — скользящее «последние N<ед>» → WindowPeriodLit, «прошлый|прошлая <noun>»
// → LastCompletedPeriodLit, адверб-константа → Ident (путь v1 неизменен).
func TestMetricPeriodValueForms(t *testing.T) {
	t.Run("последние 30дн → WindowPeriodLit{30,дн}", func(t *testing.T) {
		prog, el := parseProgramSrc(t, metricWithPeriod("последние 30дн"))
		if !el.Empty() {
			t.Fatalf("ошибки: %v", el.Error())
		}
		md := prog.Items[0].(*ast.MetricDecl)
		wp, ok := md.Period.(*ast.WindowPeriodLit)
		if !ok {
			t.Fatalf("Period = %T, хотим *ast.WindowPeriodLit", md.Period)
		}
		if wp.Amount != "30" || wp.Unit != "дн" {
			t.Errorf("WindowPeriodLit = %+v, хотим {Amount:30, Unit:дн}", wp)
		}
		if wp.Pos().Line == 0 {
			t.Errorf("Pos().Line = 0, хотим presence (токен «последние»)")
		}
	})
	t.Run("прошлый месяц → LastCompletedPeriodLit{месяц}", func(t *testing.T) {
		prog, el := parseProgramSrc(t, metricWithPeriod("прошлый месяц"))
		if !el.Empty() {
			t.Fatalf("ошибки: %v", el.Error())
		}
		md := prog.Items[0].(*ast.MetricDecl)
		lc, ok := md.Period.(*ast.LastCompletedPeriodLit)
		if !ok {
			t.Fatalf("Period = %T, хотим *ast.LastCompletedPeriodLit", md.Period)
		}
		if lc.Noun != "месяц" {
			t.Errorf("Noun = %q, хотим \"месяц\"", lc.Noun)
		}
		if lc.Pos().Line == 0 {
			t.Errorf("Pos().Line = 0, хотим presence (токен «прошлый»)")
		}
	})
	t.Run("прошлая неделя → LastCompletedPeriodLit{неделя}", func(t *testing.T) {
		prog, el := parseProgramSrc(t, metricWithPeriod("прошлая неделя"))
		if !el.Empty() {
			t.Fatalf("ошибки: %v", el.Error())
		}
		md := prog.Items[0].(*ast.MetricDecl)
		lc, ok := md.Period.(*ast.LastCompletedPeriodLit)
		if !ok {
			t.Fatalf("Period = %T, хотим *ast.LastCompletedPeriodLit", md.Period)
		}
		if lc.Noun != "неделя" {
			t.Errorf("Noun = %q, хотим \"неделя\"", lc.Noun)
		}
	})
	t.Run("ежемесячно → Ident (путь v1 неизменен)", func(t *testing.T) {
		prog, el := parseProgramSrc(t, metricWithPeriod("ежемесячно"))
		if !el.Empty() {
			t.Fatalf("ошибки: %v", el.Error())
		}
		md := prog.Items[0].(*ast.MetricDecl)
		id, ok := md.Period.(*ast.Ident)
		if !ok {
			t.Fatalf("Period = %T, хотим *ast.Ident (адверб-константа)", md.Period)
		}
		if id.Name != "ежемесячно" {
			t.Errorf("Ident.Name = %q, хотим \"ежемесячно\"", id.Name)
		}
	})
}

// T008 (011-A2 §MW-D-PARSE-2, §MW-8): НЕГАТИВЫ формы период: — «последние» без
// DURATION (в т.ч. спейсовая «30 дн» → INT) и «прошлый» без noun. Тексты — канон
// через expect/msgExpected (byte-exact).
func TestMetricPeriodValueNegatives(t *testing.T) {
	cases := []struct {
		name   string
		period string
		want   string
	}{
		{
			name:   "последние без DURATION (конец строки)",
			period: "последние",
			want:   "ожидалось 'период вида N<ед>, например 30дн', получено 'конец строки'",
		},
		{
			name:   "последние с не-DURATION словом",
			period: "последние нед",
			want:   "ожидалось 'период вида N<ед>, например 30дн', получено 'нед'",
		},
		{
			name:   "спейсовая последние 30 дн (INT после последние)",
			period: "последние 30 дн",
			want:   "ожидалось 'период вида N<ед>, например 30дн', получено '30'",
		},
		{
			name:   "прошлый без noun (конец строки)",
			period: "прошлый",
			want:   "ожидалось 'период: день/неделя/месяц/квартал/год', получено 'конец строки'",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, el := parseProgramSrc(t, metricWithPeriod(c.period))
			if el.Len() == 0 {
				t.Fatalf("ожидалась парс-ошибка %q", c.want)
			}
			pe := firstParseError(t, el)
			if pe.Msg != c.want {
				t.Errorf("Msg = %q, хотим %q", pe.Msg, c.want)
			}
		})
	}
}

// T009 (011-A2 §MW-D-PARSE-1/§MW-3): «последние»/«прошлый»/«прошлая» ВНЕ ветки
// период: остаются обычными IDENT — спец-парс контекстный, не keyword. Регресс
// краснеет, если слова сделать ключевыми. Защита codec-skip (§MW-3): слова НЕ
// предрегистрированы как глобалы (value.PeriodNames — только 5 адвербов) → при
// чтении как переменной дают рантайм «не объявлено …»; скользящий/завершённый
// Период НЕ может стать first-class значением, в Store/codec не попадает.
func TestPeriodKeywordsStayIdentOutsideMetric(t *testing.T) {
	t.Run("присвоить последние = 5 → AssignAction (последние — IDENT)", func(t *testing.T) {
		prog, el := parseProgramSrc(t, "присвоить последние = 5\n")
		if !el.Empty() {
			t.Fatalf("ошибки: %v", el.Error())
		}
		aa, ok := prog.Items[0].(*ast.AssignAction)
		if !ok {
			t.Fatalf("Items[0] = %T, хотим *ast.AssignAction", prog.Items[0])
		}
		if aa.Name.Name != "последние" {
			t.Errorf("цель = %q, хотим \"последние\" (обычный IDENT)", aa.Name.Name)
		}
	})
	t.Run("пусть x = прошлый + 1 → прошлый как Ident в выражении", func(t *testing.T) {
		prog, el := parseProgramSrc(t, "пусть x = прошлый + 1\n")
		if !el.Empty() {
			t.Fatalf("ошибки: %v", el.Error())
		}
		ls, ok := prog.Items[0].(*ast.LetStmt)
		if !ok {
			t.Fatalf("Items[0] = %T, хотим *ast.LetStmt", prog.Items[0])
		}
		be, ok := ls.Value.(*ast.BinaryExpr)
		if !ok {
			t.Fatalf("Value = %T, хотим *ast.BinaryExpr", ls.Value)
		}
		id, ok := be.Left.(*ast.Ident)
		if !ok || id.Name != "прошлый" {
			t.Errorf("левая часть = %T(%v), хотим Ident(прошлый)", be.Left, be.Left)
		}
	})
}

// --- T003: фронтенд процессов 005 — снятие cut только с KW_PROCESS ---

// T003 (D-6, §PM-3) + 007a §TR-10.5 п.1: из top-level-отсечки сняты процесс (005)
// и когда (007a, шов A → parseTriggerDecl); значение/фигурные скобки остаются
// отвергаемыми (значение — выражение шва B, на top-level недопустимо).
func TestTopLevelCutOnlyProcessRemoved(t *testing.T) {
	cases := []struct {
		name string
		tt   lexer.TokenType
		want bool
	}{
		{"процесс снят", lexer.KW_PROCESS, false},
		{"когда снят (007a шов A)", lexer.KW_WHEN, false},
		{"значение остаётся", lexer.KW_VALUE, true},
		{"LBRACE остаётся", lexer.LBRACE, true},
		{"RBRACE остаётся", lexer.RBRACE, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isUnexpectedTopLevel(c.tt); got != c.want {
				t.Errorf("isUnexpectedTopLevel(%v) = %v, хотим %v", c.tt, got, c.want)
			}
		})
	}
}

// T003 (FR-003) + 007a §TR-10.5 п.1: top-level значение … по-прежнему →
// SE-UNEXPECTED (значение — выражение шва B, на top-level недопустимо). Ведущий
// когда более НЕ SE-UNEXPECTED — диспетчеризуется в parseTriggerDecl (007a, шов A;
// негативы форм — TestTriggerKindDiagnostics).
func TestTopLevelWhenValueStillUnexpected(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		firstMsg string
	}{
		{"значение", "значение 5\n", "неожиданный токен 'значение'"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, el := parseProgramSrc(t, c.src)
			if el.Empty() {
				t.Fatalf("ожидался «неожиданный токен», ошибок нет")
			}
			if got := firstParseError(t, el).Msg; got != c.firstMsg {
				t.Errorf("первая ошибка %q, хотим %q", got, c.firstMsg)
			}
		})
	}
}

// T004 (§PM-3, FR-001/FR-005): заголовок процесса — параметры опциональны; обе
// формы дают канонический ProcessDecl (Pos() = токен процесс; без скобок →
// Params=nil). Тела шагов — T005; здесь блоки пусты → единственная ошибка
// msgEmptyBlock и ProcessDecl с пустыми Steps (exact-match негативов — T007).
func TestProcessDeclHeaderForms(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		params []string
	}{
		{"без скобок", "процесс P:\n", nil},
		{"с параметрами", "процесс P(x, y):\n", []string{"x", "y"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, el := parseProgramSrc(t, c.src)
			if el.Len() != 1 {
				t.Fatalf("ошибок %d, хотим 1 (только пустой блок): %v", el.Len(), el.Error())
			}
			if pe := firstParseError(t, el); pe.Msg != msgEmptyBlock {
				t.Fatalf("Msg = %q, хотим %q", pe.Msg, msgEmptyBlock)
			}
			if len(prog.Items) != 1 {
				t.Fatalf("Items = %d, хотим 1", len(prog.Items))
			}
			pd, ok := prog.Items[0].(*ast.ProcessDecl)
			if !ok {
				t.Fatalf("Items[0] = %T, хотим *ProcessDecl", prog.Items[0])
			}
			if pd.Pos() != (ast.Position{Line: 1, Col: 1}) {
				t.Errorf("Pos() = %+v, хотим {1,1} (токен процесс)", pd.Pos())
			}
			if pd.Name.Name != "P" {
				t.Errorf("Name = %q, хотим \"P\"", pd.Name.Name)
			}
			if len(pd.Params) != len(c.params) {
				t.Fatalf("параметров %d, хотим %d", len(pd.Params), len(c.params))
			}
			if c.params == nil && pd.Params != nil {
				t.Errorf("Params = %v, хотим nil (скобок нет)", pd.Params)
			}
			for i, want := range c.params {
				if pd.Params[i].Name != want {
					t.Errorf("Params[%d] = %q, хотим %q", i, pd.Params[i].Name, want)
				}
			}
			if len(pd.Steps) != 0 {
				t.Errorf("Steps = %d, хотим 0 (пустой блок)", len(pd.Steps))
			}
		})
	}
}

// T005 (§PM-3, FR-002): после — 0/1/N имён предшественников; без после After==nil;
// parseAfterList без скобок. Pos() шага = токен шаг.
func TestStepDeclAfterForms(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		steps int
		idx   int      // индекс проверяемого шага
		after []string // ожидаемые имена в After (nil — после отсутствует)
	}{
		{
			name:  "ноль имён (без после)",
			src:   "процесс P:\n    шаг A:\n        печать(1)\n",
			steps: 1, idx: 0, after: nil,
		},
		{
			name: "одно имя",
			src: "процесс P:\n" +
				"    шаг A:\n        печать(1)\n" +
				"    шаг B после A:\n        печать(2)\n",
			steps: 2, idx: 1, after: []string{"A"},
		},
		{
			name: "несколько имён",
			src: "процесс P:\n" +
				"    шаг A:\n        печать(1)\n" +
				"    шаг B:\n        печать(2)\n" +
				"    шаг C после A, B:\n        печать(3)\n",
			steps: 3, idx: 2, after: []string{"A", "B"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, el := parseProgramSrc(t, c.src)
			if !el.Empty() {
				t.Fatalf("ошибки: %v", el.Error())
			}
			pd, ok := prog.Items[0].(*ast.ProcessDecl)
			if !ok {
				t.Fatalf("Items[0] = %T, хотим *ProcessDecl", prog.Items[0])
			}
			if len(pd.Steps) != c.steps {
				t.Fatalf("шагов %d, хотим %d", len(pd.Steps), c.steps)
			}
			sd := pd.Steps[c.idx]
			if c.after == nil && sd.After != nil {
				t.Errorf("After = %v, хотим nil (после отсутствует)", sd.After)
			}
			if len(sd.After) != len(c.after) {
				t.Fatalf("After: %d имён, хотим %d", len(sd.After), len(c.after))
			}
			for i, want := range c.after {
				if sd.After[i].Name != want {
					t.Errorf("After[%d] = %q, хотим %q", i, sd.After[i].Name, want)
				}
			}
		})
	}
}

// T005: Pos() шага = токен шаг (а не имя/после).
func TestStepDeclPosIsStepToken(t *testing.T) {
	prog, el := parseProgramSrc(t, "процесс P:\n    шаг A:\n        печать(1)\n")
	if !el.Empty() {
		t.Fatalf("ошибки: %v", el.Error())
	}
	sd := prog.Items[0].(*ast.ProcessDecl).Steps[0]
	if sd.Pos() != (ast.Position{Line: 2, Col: 5}) {
		t.Errorf("Pos() = %+v, хотим {2,5} (токен шаг)", sd.Pos())
	}
	if sd.Name.Name != "A" {
		t.Errorf("Name = %q, хотим \"A\"", sd.Name.Name)
	}
}

// T005 (§PM-7 P2): атрибуты-только шаг — Assignee:StringLit, Deadline:DurationLit,
// позиции ключевых слов заполнены, Body нет.
func TestStepDeclAttrsOnly(t *testing.T) {
	src := "процесс P(x):\n    шаг A:\n        исполнитель: \"и\"\n        срок: 2дн\n"
	prog, el := parseProgramSrc(t, src)
	if !el.Empty() {
		t.Fatalf("ошибки: %v", el.Error())
	}
	pd := prog.Items[0].(*ast.ProcessDecl)
	if len(pd.Steps) != 1 {
		t.Fatalf("шагов %d, хотим 1", len(pd.Steps))
	}
	sd := pd.Steps[0]
	sl, ok := sd.Assignee.(*ast.StringLit)
	if !ok {
		t.Fatalf("Assignee = %T, хотим *ast.StringLit", sd.Assignee)
	}
	if sl.Value != "и" {
		t.Errorf("Assignee.Value = %q, хотим %q", sl.Value, "и")
	}
	if _, ok := sd.Deadline.(*ast.DurationLit); !ok {
		t.Fatalf("Deadline = %T, хотим *ast.DurationLit", sd.Deadline)
	}
	if sd.Attrs.AssigneePos.Line != 3 || sd.Attrs.AssigneePos.Col != 9 {
		t.Errorf("AssigneePos = %+v, хотим {3,9} (слово исполнитель)", sd.Attrs.AssigneePos)
	}
	if sd.Attrs.DeadlinePos.Line != 4 || sd.Attrs.DeadlinePos.Col != 9 {
		t.Errorf("DeadlinePos = %+v, хотим {4,9} (слово срок)", sd.Attrs.DeadlinePos)
	}
	if sd.Body != nil {
		t.Errorf("Body = %v, хотим nil (атрибуты-только шаг)", sd.Body)
	}
}

// T005 (data-model §2): инвариант пэйринга Attrs.*Pos.Line != 0 ⟺ атрибут
// присутствует (Assignee/Deadline != nil) — на всех четырёх комбинациях.
func TestStepDeclAttrPosPairing(t *testing.T) {
	cases := []struct {
		name                   string
		src                    string
		hasAssignee, hasDeadln bool
	}{
		{
			name: "без атрибутов",
			src:  "процесс P:\n    шаг A:\n        печать(1)\n",
		},
		{
			name:        "только исполнитель",
			src:         "процесс P:\n    шаг A:\n        исполнитель: \"и\"\n",
			hasAssignee: true,
		},
		{
			name:      "только срок",
			src:       "процесс P:\n    шаг A:\n        срок: 2дн\n",
			hasDeadln: true,
		},
		{
			name:        "оба атрибута",
			src:         "процесс P:\n    шаг A:\n        исполнитель: \"и\"\n        срок: 2дн\n",
			hasAssignee: true,
			hasDeadln:   true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, el := parseProgramSrc(t, c.src)
			if !el.Empty() {
				t.Fatalf("ошибки: %v", el.Error())
			}
			sd := prog.Items[0].(*ast.ProcessDecl).Steps[0]
			if got := sd.Assignee != nil; got != c.hasAssignee {
				t.Errorf("Assignee != nil = %v, хотим %v", got, c.hasAssignee)
			}
			if got := sd.Attrs.AssigneePos.Line != 0; got != c.hasAssignee {
				t.Errorf("AssigneePos.Line != 0 = %v, хотим %v (пэйринг)", got, c.hasAssignee)
			}
			if got := sd.Deadline != nil; got != c.hasDeadln {
				t.Errorf("Deadline != nil = %v, хотим %v", got, c.hasDeadln)
			}
			if got := sd.Attrs.DeadlinePos.Line != 0; got != c.hasDeadln {
				t.Errorf("DeadlinePos.Line != 0 = %v, хотим %v (пэйринг)", got, c.hasDeadln)
			}
		})
	}
}

// T006 (FR-009, §PM-2/§PM-3, SC-006): позитивные кейсы тела шага — действия и
// запуск идут через СУЩЕСТВУЮЩИЕ parseStatement/parseStepAction/parseRunProcess;
// формы AssignAction/CallAction/NotifyAction/RunProcessExpr НЕ меняются (D-2/D-10).
// Табличная фиксация канонических форм §PM-7: P1, P3, чередование attr/statement, P4.
func TestProcessStepBodyCanonicalForms(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		check func(t *testing.T, prog *ast.Program)
	}{
		{
			// §PM-7 P1: ProcessDecl{Name:P, Params:nil,
			// Steps:[StepDecl{Name:A, Assignee:nil, Deadline:nil, Body:[ExpressionStmt]}]}.
			name: "P1 оператор-выражение в теле шага",
			src:  "процесс P:\n    шаг A:\n        печать(1)\n",
			check: func(t *testing.T, prog *ast.Program) {
				pd, ok := prog.Items[0].(*ast.ProcessDecl)
				if !ok {
					t.Fatalf("Items[0] = %T, хотим *ProcessDecl", prog.Items[0])
				}
				if pd.Name.Name != "P" || pd.Params != nil {
					t.Errorf("заголовок: name=%q params=%v, хотим P/nil", pd.Name.Name, pd.Params)
				}
				if len(pd.Steps) != 1 {
					t.Fatalf("шагов %d, хотим 1", len(pd.Steps))
				}
				sd := pd.Steps[0]
				if sd.Name.Name != "A" || sd.Assignee != nil || sd.Deadline != nil {
					t.Errorf("шаг: name=%q assignee=%v deadline=%v, хотим A/nil/nil",
						sd.Name.Name, sd.Assignee, sd.Deadline)
				}
				if len(sd.Body) != 1 {
					t.Fatalf("Body: %d операторов, хотим 1", len(sd.Body))
				}
				es, ok := sd.Body[0].(*ast.ExpressionStmt)
				if !ok {
					t.Fatalf("Body[0] = %T, хотим *ast.ExpressionStmt", sd.Body[0])
				}
				if got := sexpr(es.Expr); got != "(call печать 1)" {
					t.Errorf("Expr = %s, хотим (call печать 1)", got)
				}
			},
		},
		{
			// §PM-7 P3: Steps:[A{Body:[AssignAction]}, B{After:[A], Assignee:StringLit}].
			name: "P3 присвоить в A и шаг B после A с исполнителем",
			src: "процесс P:\n" +
				"    шаг A:\n        присвоить y = 1\n" +
				"    шаг B после A:\n        исполнитель: \"и\"\n",
			check: func(t *testing.T, prog *ast.Program) {
				pd, ok := prog.Items[0].(*ast.ProcessDecl)
				if !ok {
					t.Fatalf("Items[0] = %T, хотим *ProcessDecl", prog.Items[0])
				}
				if len(pd.Steps) != 2 {
					t.Fatalf("шагов %d, хотим 2", len(pd.Steps))
				}
				a := pd.Steps[0]
				if len(a.Body) != 1 {
					t.Fatalf("A.Body: %d операторов, хотим 1", len(a.Body))
				}
				aa, ok := a.Body[0].(*ast.AssignAction)
				if !ok {
					t.Fatalf("A.Body[0] = %T, хотим *ast.AssignAction", a.Body[0])
				}
				if aa.Name.Name != "y" {
					t.Errorf("AssignAction.Name = %q, хотим \"y\"", aa.Name.Name)
				}
				b := pd.Steps[1]
				if len(b.After) != 1 || b.After[0].Name != "A" {
					t.Fatalf("B.After = %v, хотим [A]", b.After)
				}
				sl, ok := b.Assignee.(*ast.StringLit)
				if !ok {
					t.Fatalf("B.Assignee = %T, хотим *ast.StringLit", b.Assignee)
				}
				if sl.Value != "и" {
					t.Errorf("B.Assignee.Value = %q, хотим %q", sl.Value, "и")
				}
				if b.Body != nil {
					t.Errorf("B.Body = %v, хотим nil (атрибут-только шаг)", b.Body)
				}
			},
		},
		{
			// Чередование attr/statement в одном шаге: атрибут после оператора
			// синтаксически легален (порядок строк свободный, §PM-3 шаг 6).
			name: "чередование атрибутов и операторов в одном шаге",
			src: "процесс P:\n    шаг A:\n" +
				"        печать(1)\n" +
				"        исполнитель: \"и\"\n" +
				"        присвоить y = 2\n" +
				"        срок: 2дн\n",
			check: func(t *testing.T, prog *ast.Program) {
				pd, ok := prog.Items[0].(*ast.ProcessDecl)
				if !ok {
					t.Fatalf("Items[0] = %T, хотим *ProcessDecl", prog.Items[0])
				}
				sd := pd.Steps[0]
				if len(sd.Body) != 2 {
					t.Fatalf("Body: %d операторов, хотим 2 (атрибуты не в Body)", len(sd.Body))
				}
				if _, ok := sd.Body[0].(*ast.ExpressionStmt); !ok {
					t.Errorf("Body[0] = %T, хотим *ast.ExpressionStmt", sd.Body[0])
				}
				if _, ok := sd.Body[1].(*ast.AssignAction); !ok {
					t.Errorf("Body[1] = %T, хотим *ast.AssignAction", sd.Body[1])
				}
				if _, ok := sd.Assignee.(*ast.StringLit); !ok {
					t.Errorf("Assignee = %T, хотим *ast.StringLit", sd.Assignee)
				}
				if _, ok := sd.Deadline.(*ast.DurationLit); !ok {
					t.Errorf("Deadline = %T, хотим *ast.DurationLit", sd.Deadline)
				}
				if sd.Attrs.AssigneePos.Line != 4 || sd.Attrs.DeadlinePos.Line != 6 {
					t.Errorf("Attrs = %+v, хотим AssigneePos.Line=4, DeadlinePos.Line=6", sd.Attrs)
				}
			},
		},
		{
			// §PM-7 P4: LetStmt{Value: RunProcessExpr{Process:P, Args:[StringLit]}}.
			name: "P4 запустить процесс в пусть",
			src: "процесс P(x):\n    шаг A:\n        печать(1)\n" +
				"пусть id = запустить процесс P(\"Петров\")\n",
			check: func(t *testing.T, prog *ast.Program) {
				if len(prog.Items) != 2 {
					t.Fatalf("Items = %d, хотим 2 (процесс + пусть)", len(prog.Items))
				}
				ls, ok := prog.Items[1].(*ast.LetStmt)
				if !ok {
					t.Fatalf("Items[1] = %T, хотим *ast.LetStmt", prog.Items[1])
				}
				if ls.Name.Name != "id" {
					t.Errorf("LetStmt.Name = %q, хотим \"id\"", ls.Name.Name)
				}
				rp, ok := ls.Value.(*ast.RunProcessExpr)
				if !ok {
					t.Fatalf("Value = %T, хотим *ast.RunProcessExpr", ls.Value)
				}
				if rp.Process.Name != "P" {
					t.Errorf("Process = %q, хотим \"P\"", rp.Process.Name)
				}
				if len(rp.Args) != 1 {
					t.Fatalf("Args: %d, хотим 1", len(rp.Args))
				}
				sl, ok := rp.Args[0].(*ast.StringLit)
				if !ok {
					t.Fatalf("Args[0] = %T, хотим *ast.StringLit", rp.Args[0])
				}
				if sl.Value != "Петров" {
					t.Errorf("Args[0].Value = %q, хотим %q", sl.Value, "Петров")
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, el := parseProgramSrc(t, c.src)
			if !el.Empty() {
				t.Fatalf("ошибки: %v", el.Error())
			}
			c.check(t, prog)
		})
	}
}

// T007 (§PM-6.A, D-8, SC-001): дубль атрибута шага → exact-match payload
// «атрибут '<имя>' уже задан» (msgDuplicateAttr, переиспользование 004 §SM-9.A),
// позиция повторного атрибута; p.error+break строго как parseMetricDecl —
// остаток блока восстанавливается штатно, ровно одна ошибка.
func TestStepDeclDuplicateAttr(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		want      string
		line, col int
	}{
		{
			name: "исполнитель дважды",
			src: "процесс P:\n    шаг A:\n" +
				"        исполнитель: \"x\"\n" +
				"        исполнитель: \"y\"\n",
			want: "атрибут 'исполнитель' уже задан",
			line: 4, col: 9,
		},
		{
			name: "срок дважды",
			src: "процесс P:\n    шаг A:\n" +
				"        срок: 2дн\n" +
				"        срок: 3дн\n",
			want: "атрибут 'срок' уже задан",
			line: 4, col: 9,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, el := parseProgramSrc(t, c.src)
			if el.Len() != 1 {
				t.Fatalf("ошибок %d, хотим 1 (break + штатное восстановление): %v",
					el.Len(), el.Error())
			}
			pe := firstParseError(t, el)
			if pe.Msg != c.want {
				t.Errorf("Msg = %q, хотим %q", pe.Msg, c.want)
			}
			if pe.Pos.Line != c.line || pe.Pos.Col != c.col {
				t.Errorf("позиция = %+v, хотим {%d,%d} (повторный атрибут)",
					pe.Pos, c.line, c.col)
			}
		})
	}
}

// T007 (§PM-6.A, FR-007): пустой блок процесса и пустой блок шага → exact-match
// payload msgEmptyBlock (переиспользование); ровно одна ошибка, узлы построены
// best-effort (ProcessDecl с пустыми Steps / StepDecl без атрибутов и тела).
func TestProcessAndStepEmptyBlock(t *testing.T) {
	const want = "пустой блок не допускается, добавьте хотя бы один оператор"
	cases := []struct {
		name  string
		src   string
		check func(t *testing.T, prog *ast.Program)
	}{
		{
			name: "пустой блок процесса",
			src:  "процесс P:\n",
			check: func(t *testing.T, prog *ast.Program) {
				pd, ok := prog.Items[0].(*ast.ProcessDecl)
				if !ok {
					t.Fatalf("Items[0] = %T, хотим *ProcessDecl", prog.Items[0])
				}
				if len(pd.Steps) != 0 {
					t.Errorf("Steps = %d, хотим 0", len(pd.Steps))
				}
			},
		},
		{
			name: "пустой блок шага",
			src:  "процесс P:\n    шаг A:\n",
			check: func(t *testing.T, prog *ast.Program) {
				pd, ok := prog.Items[0].(*ast.ProcessDecl)
				if !ok {
					t.Fatalf("Items[0] = %T, хотим *ProcessDecl", prog.Items[0])
				}
				if len(pd.Steps) != 1 {
					t.Fatalf("шагов %d, хотим 1 (best-effort)", len(pd.Steps))
				}
				sd := pd.Steps[0]
				if sd.Assignee != nil || sd.Deadline != nil || sd.Body != nil {
					t.Errorf("пустой шаг: assignee=%v deadline=%v body=%v, хотим nil",
						sd.Assignee, sd.Deadline, sd.Body)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, el := parseProgramSrc(t, c.src)
			if el.Len() != 1 {
				t.Fatalf("ошибок %d, хотим 1: %v", el.Len(), el.Error())
			}
			if pe := firstParseError(t, el); pe.Msg != want {
				t.Errorf("Msg = %q, хотим %q", pe.Msg, want)
			}
			c.check(t, prog)
		})
	}
}

// T007 (§PM-6.A, §PM-3 шаг 6, CP-5.2 N3): не-шаг строка в блоке процесса →
// msgUnexpected на ведущем токене строки (peek().Pos ДО advance); печать — IDENT,
// не KW. synchronize съедает строку до NEWLINE → ровно одна ошибка.
func TestProcessBlockNonStepUnexpected(t *testing.T) {
	_, el := parseProgramSrc(t, "процесс P:\n    печать(1)\n")
	if el.Len() != 1 {
		t.Fatalf("ошибок %d, хотим 1: %v", el.Len(), el.Error())
	}
	pe := firstParseError(t, el)
	want := "неожиданный токен 'печать'"
	if pe.Msg != want {
		t.Errorf("Msg = %q, хотим %q", pe.Msg, want)
	}
	if pe.Pos.Line != 2 || pe.Pos.Col != 5 {
		t.Errorf("позиция = %+v, хотим {2,5} (ведущий токен строки)", pe.Pos)
	}
}

// Ревью №1 (M-1/m-2/m-3, §PM-3): восстановление парсера процессов.
// M-1 (п.6): не-шаг строка НЕ обрывает цикл блока процесса — error без break,
// прогресс гарантирует backstop, последующие шаги собираются. m-2 (доктрина
// recover.go): suppress сбрасывается на границе строки блока процесса и
// StepLine — ошибка в теле не глушит последующие диагностики, фантомов
// «конец блока» нет. m-3 (п.3): висящая запятая в 'после' — best-effort стоп
// без ошибки, собранный список остаётся.
// Ревью №2 (пересмотр D-8, §PM-3 п.6): дубль атрибута шага — p.error+continue;
// строка дубля съедена synchronize, следующие строки тела и шаги разбираются
// штатно, первое значение атрибута сохраняется. Кейс — мутационный лок: возврат
// break роняет его (терял следующий шаг из AST).
func TestProcessRecoveryReviewFixes(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		wantMsgs  []string // exact-match, по порядку; пусто → parse-clean
		wantLines []int    // строка i-й ошибки (параллельно wantMsgs)
		check     func(t *testing.T, prog *ast.Program)
	}{
		{
			name:      "M-1: шаги после ошибочной строки собираются",
			src:       "процесс P:\n    мусор\n    шаг A:\n        печать(1)\n",
			wantMsgs:  []string{"неожиданный токен 'мусор'"},
			wantLines: []int{2},
			check: func(t *testing.T, prog *ast.Program) {
				pd, ok := prog.Items[0].(*ast.ProcessDecl)
				if !ok {
					t.Fatalf("Items[0] = %T, хотим *ProcessDecl", prog.Items[0])
				}
				if len(pd.Steps) != 1 {
					t.Fatalf("шагов %d, хотим 1 (цикл продолжился после ошибки)", len(pd.Steps))
				}
				sd := pd.Steps[0]
				if sd.Name.Name != "A" {
					t.Errorf("Name = %q, хотим \"A\"", sd.Name.Name)
				}
				if len(sd.Body) != 1 {
					t.Errorf("Body: %d операторов, хотим 1", len(sd.Body))
				}
			},
		},
		{
			name: "m-2: ошибка в теле не глушит дубль атрибута",
			src: "процесс P:\n    шаг A:\n" +
				"        пусть x = )\n" +
				"        исполнитель: \"и\"\n" +
				"        исполнитель: \"й\"\n",
			wantMsgs: []string{
				"неожиданный токен ')'",
				"атрибут 'исполнитель' уже задан",
			},
			wantLines: []int{3, 5},
		},
		{
			name: "m-2: подряд не-шаг строки — каждая со своей диагностикой",
			src:  "процесс P:\n    мусор\n    хлам\n    шаг A:\n        печать(1)\n",
			wantMsgs: []string{
				"неожиданный токен 'мусор'",
				"неожиданный токен 'хлам'",
			},
			wantLines: []int{2, 3},
			check: func(t *testing.T, prog *ast.Program) {
				pd, ok := prog.Items[0].(*ast.ProcessDecl)
				if !ok {
					t.Fatalf("Items[0] = %T, хотим *ProcessDecl", prog.Items[0])
				}
				if len(pd.Steps) != 1 || pd.Steps[0].Name.Name != "A" {
					t.Fatalf("шагов %d, хотим 1 (A)", len(pd.Steps))
				}
			},
		},
		{
			name: "D-8 ревью №2: дубль атрибута не теряет следующий шаг",
			src: "процесс P:\n    шаг A:\n" +
				"        исполнитель: \"x\"\n" +
				"        исполнитель: \"y\"\n" +
				"        печать(1)\n" +
				"    шаг B:\n        печать(2)\n",
			wantMsgs:  []string{"атрибут 'исполнитель' уже задан"},
			wantLines: []int{4},
			check: func(t *testing.T, prog *ast.Program) {
				pd, ok := prog.Items[0].(*ast.ProcessDecl)
				if !ok {
					t.Fatalf("Items[0] = %T, хотим *ProcessDecl", prog.Items[0])
				}
				if len(pd.Steps) != 2 || pd.Steps[0].Name.Name != "A" || pd.Steps[1].Name.Name != "B" {
					t.Fatalf("шаги = %v, хотим [A, B] (break терял B)", pd.Steps)
				}
				a := pd.Steps[0]
				if len(a.Body) != 1 {
					t.Fatalf("A.Body: %d операторов, хотим 1 (печать(1) после строки дубля)", len(a.Body))
				}
				sl, ok := a.Assignee.(*ast.StringLit)
				if !ok {
					t.Fatalf("A.Assignee = %T, хотим *ast.StringLit", a.Assignee)
				}
				if sl.Value != "x" {
					t.Errorf("A.Assignee.Value = %q, хотим %q (первое значение не перезаписано)", sl.Value, "x")
				}
			},
		},
		{
			name: "m-3: висящая запятая в после",
			src: "процесс P:\n" +
				"    шаг A:\n        печать(1)\n" +
				"    шаг B после A, :\n        печать(2)\n",
			check: func(t *testing.T, prog *ast.Program) {
				pd, ok := prog.Items[0].(*ast.ProcessDecl)
				if !ok {
					t.Fatalf("Items[0] = %T, хотим *ProcessDecl", prog.Items[0])
				}
				if len(pd.Steps) != 2 {
					t.Fatalf("шагов %d, хотим 2", len(pd.Steps))
				}
				b := pd.Steps[1]
				if len(b.After) != 1 || b.After[0].Name != "A" {
					t.Errorf("After = %v, хотим [A]", b.After)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, el := parseProgramSrc(t, c.src)
			if el.Len() != len(c.wantMsgs) {
				t.Fatalf("ошибок %d, хотим %d: %v", el.Len(), len(c.wantMsgs), el.Error())
			}
			for i, want := range c.wantMsgs {
				pe, ok := el.Errors()[i].(errors.ParseError)
				if !ok {
					t.Fatalf("ошибка[%d] не ParseError: %T", i, el.Errors()[i])
				}
				if pe.Msg != want {
					t.Errorf("Msg[%d] = %q, хотим %q", i, pe.Msg, want)
				}
				if pe.Pos.Line != c.wantLines[i] {
					t.Errorf("строка[%d] = %d, хотим %d", i, pe.Pos.Line, c.wantLines[i])
				}
			}
			if c.check != nil {
				c.check(t, prog)
			}
		})
	}
}

// T007 (§PM-3 шаг 3, CP-5.2 N4): после без имени → SE-EXPECTED от
// expect(IDENT, "имя шага") — общий текст msgExpected реестра 002 (НЕ новый текст
// слоя процессов); After пуст, восстановление штатное (шаг и тело построены).
func TestStepAfterWithoutNameExpected(t *testing.T) {
	src := "процесс P:\n    шаг A после :\n        печать(1)\n"
	prog, el := parseProgramSrc(t, src)
	if el.Len() != 1 {
		t.Fatalf("ошибок %d, хотим 1: %v", el.Len(), el.Error())
	}
	pe := firstParseError(t, el)
	want := "ожидалось 'имя шага', получено ':'"
	if pe.Msg != want {
		t.Errorf("Msg = %q, хотим %q", pe.Msg, want)
	}
	if pe.Pos.Line != 2 || pe.Pos.Col != 17 {
		t.Errorf("позиция = %+v, хотим {2,17} (токен ':')", pe.Pos)
	}
	pd, ok := prog.Items[0].(*ast.ProcessDecl)
	if !ok {
		t.Fatalf("Items[0] = %T, хотим *ProcessDecl", prog.Items[0])
	}
	if len(pd.Steps) != 1 {
		t.Fatalf("шагов %d, хотим 1 (восстановление штатное)", len(pd.Steps))
	}
	sd := pd.Steps[0]
	if sd.Name.Name != "A" {
		t.Errorf("Name = %q, хотим \"A\"", sd.Name.Name)
	}
	if len(sd.After) != 0 {
		t.Errorf("After = %v, хотим пустой", sd.After)
	}
	if len(sd.Body) != 1 {
		t.Errorf("Body: %d операторов, хотим 1 (тело разобрано после synchronize)",
			len(sd.Body))
	}
}

// T007 (§PM-3 тонкость, FR-006): «неизвестного атрибута» в шаге НЕТ — foo: bar
// разбирается как выражение-оператор foo, затем ':' даёт общую ошибку реестра 002
// из parseExprStatement (НЕ 'неизвестный атрибут', НЕ реестр процессов §PM-6.A).
func TestStepFooColonBarIsStatementNotAttr(t *testing.T) {
	src := "процесс P:\n    шаг A:\n        foo: bar\n"
	prog, el := parseProgramSrc(t, src)
	if el.Len() != 1 {
		t.Fatalf("ошибок %d, хотим 1: %v", el.Len(), el.Error())
	}
	pe := firstParseError(t, el)
	want := "ожидалось 'конец строки', получено ':'"
	if pe.Msg != want {
		t.Errorf("Msg = %q, хотим %q (общий реестр 002, не слой процессов)", pe.Msg, want)
	}
	sd := prog.Items[0].(*ast.ProcessDecl).Steps[0]
	if sd.Assignee != nil || sd.Deadline != nil {
		t.Errorf("атрибуты не должны выставляться: assignee=%v deadline=%v",
			sd.Assignee, sd.Deadline)
	}
	if len(sd.Body) != 1 {
		t.Fatalf("Body: %d операторов, хотим 1 (foo — выражение-оператор)", len(sd.Body))
	}
	es, ok := sd.Body[0].(*ast.ExpressionStmt)
	if !ok {
		t.Fatalf("Body[0] = %T, хотим *ast.ExpressionStmt", sd.Body[0])
	}
	if got := sexpr(es.Expr); got != "foo" {
		t.Errorf("Expr = %s, хотим foo", got)
	}
}

// T007 (FR-004): вложенный процесс в теле функции — отдельной диагностики НЕТ;
// первый отказ — SE-UNEXPECTED через путь выражения (parsePrimary, msgUnexpected)
// на токене процесс.
func TestNestedProcessInFunctionUnexpected(t *testing.T) {
	src := "функция f():\n    процесс Q:\n        шаг A:\n            печать(1)\n"
	_, el := parseProgramSrc(t, src)
	if el.Len() == 0 {
		t.Fatalf("ожидался «неожиданный токен 'процесс'», ошибок нет")
	}
	pe := firstParseError(t, el)
	want := "неожиданный токен 'процесс'"
	if pe.Msg != want {
		t.Errorf("Msg = %q, хотим %q", pe.Msg, want)
	}
	if pe.Pos.Line != 2 || pe.Pos.Col != 5 {
		t.Errorf("позиция = %+v, хотим {2,5} (токен процесс)", pe.Pos)
	}
}

// T013 (SC-004): метрик-онли фикстура §SM-10 парсится с нулём ошибок.
func TestMetricOnlyFixtureParsesClean(t *testing.T) {
	path := filepath.Join("..", "eval", "testdata", "metric_only.ladix")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("читать фикстуру: %v", err)
	}
	toks, lexErrs := lexer.New(string(data)).Tokenize()
	if !lexErrs.Empty() {
		t.Fatalf("лексические ошибки: %v", lexErrs.Error())
	}
	el := errors.NewErrorList()
	prog := New(toks, el).Parse()
	if !el.Empty() {
		t.Fatalf("синтаксические ошибки: %v", el.Error())
	}
	if len(prog.Items) != 2 {
		t.Fatalf("Items = %d, хотим 2 (источник + метрика)", len(prog.Items))
	}
	if _, ok := prog.Items[0].(*ast.SourceDecl); !ok {
		t.Errorf("Items[0] = %T, хотим *SourceDecl", prog.Items[0])
	}
	if _, ok := prog.Items[1].(*ast.MetricDecl); !ok {
		t.Errorf("Items[1] = %T, хотим *MetricDecl", prog.Items[1])
	}
}

// --- 007a: фронтенд триггеров (T011 — позитивы, негативы, шов B) ---

// triggerBody возвращает тело первого TriggerDecl программы (helper позитивов).
func triggerDecl(t *testing.T, src string) *ast.TriggerDecl {
	t.Helper()
	prog, el := parseProgramSrc(t, src)
	if !el.Empty() {
		t.Fatalf("ошибки: %v", el.Error())
	}
	if len(prog.Items) != 1 {
		t.Fatalf("Items = %d, хотим 1", len(prog.Items))
	}
	td, ok := prog.Items[0].(*ast.TriggerDecl)
	if !ok {
		t.Fatalf("Items[0] = %T, хотим *TriggerDecl", prog.Items[0])
	}
	return td
}

// T011 (§TR-1): метрика-триггер парсится — спека MetricTrigger{Metric, Op, Threshold},
// тело — индентный Block. Pos() = токен когда (1,1).
func TestTriggerMetricParse(t *testing.T) {
	td := triggerDecl(t, "когда метрика выручка < 100:\n    печать(1)\n")
	if td.Pos() != (ast.Position{Line: 1, Col: 1}) {
		t.Errorf("Pos() = %+v, хотим {1,1} (токен когда)", td.Pos())
	}
	mt, ok := td.Spec.(*ast.MetricTrigger)
	if !ok {
		t.Fatalf("Spec = %T, хотим *MetricTrigger", td.Spec)
	}
	if mt.Metric.Name != "выручка" {
		t.Errorf("Metric = %q, хотим \"выручка\"", mt.Metric.Name)
	}
	if mt.Op != ast.CompLt {
		t.Errorf("Op = %v, хотим CompLt", mt.Op)
	}
	if sexpr(mt.Threshold) != "100" {
		t.Errorf("Threshold = %s, хотим 100", sexpr(mt.Threshold))
	}
	if td.Body == nil || len(td.Body.Stmts) != 1 {
		t.Fatalf("Body должно содержать 1 оператор: %+v", td.Body)
	}
}

// T011 (§TR-1, все 6 CompOp): каждый оператор сравнения принимается expectCompOp.
func TestTriggerMetricAllCompOps(t *testing.T) {
	cases := []struct {
		op   string
		want ast.CompOp
	}{
		{"==", ast.CompEq}, {"!=", ast.CompNeq},
		{"<", ast.CompLt}, {"<=", ast.CompLe},
		{">", ast.CompGt}, {">=", ast.CompGe},
	}
	for _, c := range cases {
		t.Run(c.op, func(t *testing.T) {
			td := triggerDecl(t, "когда метрика m "+c.op+" 1:\n    печать(1)\n")
			mt := td.Spec.(*ast.MetricTrigger)
			if mt.Op != c.want {
				t.Errorf("Op = %v, хотим %v", mt.Op, c.want)
			}
		})
	}
}

// T011 (§TR-1): событие-триггер — спека EventTrigger{Event}, тело-Block.
func TestTriggerEventParse(t *testing.T) {
	td := triggerDecl(t, "когда событие заказ_создан:\n    печать(1)\n")
	et, ok := td.Spec.(*ast.EventTrigger)
	if !ok {
		t.Fatalf("Spec = %T, хотим *EventTrigger", td.Spec)
	}
	if et.Event.Name != "заказ_создан" {
		t.Errorf("Event = %q, хотим \"заказ_создан\"", et.Event.Name)
	}
}

// T011 (§TR-1, FR-004): расписание `каждые <D>` — все 6 единиц длительности
// принимаются без валидации (сек/мин/час/дн/нед/мес).
func TestTriggerScheduleEveryAllUnits(t *testing.T) {
	units := []struct {
		lit, amount, unit string
	}{
		{"30сек", "30", "сек"}, {"5мин", "5", "мин"}, {"1час", "1", "час"},
		{"3дн", "3", "дн"}, {"2нед", "2", "нед"}, {"1мес", "1", "мес"},
	}
	for _, u := range units {
		t.Run(u.lit, func(t *testing.T) {
			td := triggerDecl(t, "когда расписание каждые "+u.lit+":\n    печать(1)\n")
			st, ok := td.Spec.(*ast.ScheduleTrigger)
			if !ok {
				t.Fatalf("Spec = %T, хотим *ScheduleTrigger", td.Spec)
			}
			ev, ok := st.Spec.(*ast.EverySchedule)
			if !ok {
				t.Fatalf("Spec.Spec = %T, хотим *EverySchedule", st.Spec)
			}
			if ev.Every.Amount != u.amount || ev.Every.Unit != u.unit {
				t.Errorf("Every = %s%s, хотим %s%s", ev.Every.Amount, ev.Every.Unit, u.amount, u.unit)
			}
		})
	}
}

// T011 (§TR-1, FR-005): расписание `в "ЧЧ:ММ"` — строка принимается как StringLit,
// формат содержимого парсером НЕ проверяется.
func TestTriggerScheduleAtParse(t *testing.T) {
	td := triggerDecl(t, "когда расписание в \"08:30\":\n    печать(1)\n")
	st, ok := td.Spec.(*ast.ScheduleTrigger)
	if !ok {
		t.Fatalf("Spec = %T, хотим *ScheduleTrigger", td.Spec)
	}
	at, ok := st.Spec.(*ast.AtSchedule)
	if !ok {
		t.Fatalf("Spec.Spec = %T, хотим *AtSchedule", st.Spec)
	}
	if at.At.Value != "08:30" {
		t.Errorf("At = %q, хотим \"08:30\"", at.At.Value)
	}
}

// T011 (§TR-4 п.6, FR-021): условие метрики — РОВНО одно сравнение. Плоский разбор
// (parseComparison) НЕ поглощает логический `и` → `метрика X < Y и Z` оставляет `и`,
// далее ошибка ожидания ':'. Это структурная невыразимость составного условия.
//
// T027 усиление: payload + ТОЧНАЯ позиция (строка, колонка в рунах) токена `и` —
// доказывает, что разбор порога остановился ровно на `и` (логика не поглощена), а не
// где-то дальше. `когда метрика m < Y и Z > W:` — `и` на строке 1, колонка 21
// («когда метрика m < 1 » = 20 рун, далее `и`).
func TestTriggerMetricThresholdFlatOneComparison(t *testing.T) {
	_, el := parseProgramSrc(t, "когда метрика m < 1 и 2 > 0:\n    печать(1)\n")
	if el.Len() != 1 {
		t.Fatalf("ошибок %d, хотим 1 (ожидание ':' после порога): %v", el.Len(), el.Error())
	}
	pe := firstParseError(t, el)
	want := "ожидалось ':', получено 'и'"
	if pe.Msg != want {
		t.Errorf("Msg = %q, хотим %q (и не поглощён, одно сравнение)", pe.Msg, want)
	}
	if pe.Pos.Line != 1 || pe.Pos.Col != 21 {
		t.Errorf("позиция = %+v, хотим {1,21} (токен 'и', порог остановлен)", pe.Pos)
	}
}

// T011 (§TR-7.F): негативы диспетчера форм и спек — exact-match SE-TRIGGER-KIND /
// SE-EXPECT-COMPOP / SE-SCHEDULE-SPEC через msgExpected.
func TestTriggerSyntaxDiagnostics(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "SE-TRIGGER-KIND",
			src:  "когда x > 1:\n    печать(1)\n",
			want: "ожидалось 'метрика, событие или расписание', получено 'x'",
		},
		{
			name: "SE-EXPECT-COMPOP",
			src:  "когда метрика m + 1:\n    печать(1)\n",
			want: "ожидалось 'оператор сравнения', получено '+'",
		},
		{
			name: "SE-SCHEDULE-SPEC",
			src:  "когда расписание ночью:\n    печать(1)\n",
			want: "ожидалось 'каждые или в', получено 'ночью'",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, el := parseProgramSrc(t, c.src)
			if el.Len() == 0 {
				t.Fatalf("ожидалась ошибка %q, ошибок нет", c.want)
			}
			if got := firstParseError(t, el).Msg; got != c.want {
				t.Errorf("первая ошибка %q, хотим %q", got, c.want)
			}
		})
	}
}

// T011 (шов B, §TR-10.5 п.3): `значение` — первичное выражение в любой позиции;
// собирается parsePrimary (синтаксически везде; контекст-гард — семпроход).
func TestValueExprAsPrimary(t *testing.T) {
	// аргумент вызова в теле триггера (как в выручка.ladix)
	td := triggerDecl(t, "когда метрика m < 1:\n    запустить процесс P(значение)\n")
	es, ok := td.Body.Stmts[0].(*ast.ExpressionStmt)
	if !ok {
		t.Fatalf("Body[0] = %T, хотим *ExpressionStmt", td.Body.Stmts[0])
	}
	if got := sexpr(es.Expr); got != "(run P значение)" {
		t.Errorf("Expr = %s, хотим (run P значение)", got)
	}
}

// T011 (шов B): `событие.поле` — постфиксный FieldExpr над EventExpr (собирается
// существующим постфикс-парсером, без правок).
func TestEventExprFieldAccess(t *testing.T) {
	td := triggerDecl(t, "когда событие e:\n    печать(событие.клиент)\n")
	es := td.Body.Stmts[0].(*ast.ExpressionStmt)
	if got := sexpr(es.Expr); got != "(call печать (field событие клиент))" {
		t.Errorf("Expr = %s, хотим (call печать (field событие клиент))", got)
	}
}

// T024 (§TR-7.F, diagnostics.md, SC-005/SC-006): exact-match негативы трёх
// СИНТАКСИЧЕСКИХ диагностик форм триггера + переиспользуемого пустого блока —
// payload БАЙТ-В-БАЙТ из contracts/diagnostics.md и точная позиция (строка,
// колонка в рунах) ведущего токена-нарушителя. Дополняет TestTriggerSyntaxDiagnostics
// (только payload): здесь фиксируется И позиция, И ровно-одна-ошибка.
//
//   - SE-TRIGGER-KIND  — нет вида после `когда`: позиция = токен после `когда`.
//   - SE-EXPECT-COMPOP — нет CompOp после `метрика Ident`: позиция = не-CompOp токен.
//   - SE-SCHEDULE-SPEC — нет `каждые`/`в` после `расписание`: позиция = токен после.
//   - msgEmptyBlock    — тело триггера после `:` пусто (переиспользование §PM-6.A).
func TestTriggerNegativesExactPos(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		want      string
		line, col int
	}{
		{
			// тело опущено: default-ветка parseTriggerDecl делает synchronize по
			// строке; индентный блок после неопознанного вида породил бы
			// производные «увеличение отступа»/«конец блока» (не одна ошибка).
			name: "SE-TRIGGER-KIND нет вида после когда",
			src:  "когда X:\n",
			want: "ожидалось 'метрика, событие или расписание', получено 'X'",
			line: 1, col: 7, // токен 'X' после 'когда '
		},
		{
			name: "SE-EXPECT-COMPOP нет оператора после метрика Ident",
			src:  "когда метрика m:\n    печать(1)\n",
			want: "ожидалось 'оператор сравнения', получено ':'",
			line: 1, col: 16, // токен ':' там, где ожидался CompOp
		},
		{
			name: "SE-SCHEDULE-SPEC нет каждые/в после расписание",
			src:  "когда расписание X:\n    печать(1)\n",
			want: "ожидалось 'каждые или в', получено 'X'",
			line: 1, col: 18, // токен 'X' после 'расписание '
		},
		{
			name: "msgEmptyBlock пустое тело триггера",
			src:  "когда метрика m > 1:\nпечать(1)\n",
			want: "пустой блок не допускается, добавьте хотя бы один оператор",
			line: 2, col: 1, // ведущий токен строки на месте отсутствующего INDENT
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, el := parseProgramSrc(t, c.src)
			if el.Len() != 1 {
				t.Fatalf("ошибок %d, хотим 1: %v", el.Len(), el.Error())
			}
			pe := firstParseError(t, el)
			if pe.Msg != c.want {
				t.Errorf("Msg = %q, хотим %q", pe.Msg, c.want)
			}
			if pe.Pos.Line != c.line || pe.Pos.Col != c.col {
				t.Errorf("позиция = %+v, хотим {%d,%d}", pe.Pos, c.line, c.col)
			}
		})
	}
}

// T024 (§8.1, SC-003): три новые синтаксические диагностики форм триггера в ПОЛНОМ
// двухстрочном каноне `Ошибка в строке N, колонка M:\n<payload>` (assertGolden из
// errors_golden_test.go) — БАЙТ-В-БАЙТ из contracts/diagnostics.md §TR-7.F.
func TestGoldenTriggerSyntaxDiagnostics(t *testing.T) {
	assertGolden(t, "когда X:\n",
		"Ошибка в строке 1, колонка 7:\nожидалось 'метрика, событие или расписание', получено 'X'")
	assertGolden(t, "когда метрика m:\n    печать(1)\n",
		"Ошибка в строке 1, колонка 16:\nожидалось 'оператор сравнения', получено ':'")
	assertGolden(t, "когда расписание X:\n    печать(1)\n",
		"Ошибка в строке 1, колонка 18:\nожидалось 'каждые или в', получено 'X'")
	assertGolden(t, "когда метрика m > 1:\nпечать(1)\n",
		"Ошибка в строке 2, колонка 1:\nпустой блок не допускается, добавьте хотя бы один оператор")
}
