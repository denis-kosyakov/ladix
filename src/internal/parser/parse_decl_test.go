package parser

import (
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
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
