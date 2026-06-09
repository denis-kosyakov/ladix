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
