package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/lexer"
)

// parseProgramSrc лексирует и разбирает целую программу; возвращает дерево и
// накопитель синтаксических ошибок (изолированный, без лексических).
func parseProgramSrc(t *testing.T, src string) (*ast.Program, *errors.ErrorList) {
	t.Helper()
	toks := lexTokens(t, src)
	el := errors.NewErrorList()
	return New(toks, el).Parse(), el
}

// parseExampleFile разбирает examples/<name> из корня репозитория; возвращает
// дерево, синтаксические ошибки парсера и лексические ошибки.
func parseExampleFile(t *testing.T, name string) (*ast.Program, *errors.ErrorList, *errors.ErrorList) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", name))
	if err != nil {
		t.Fatalf("читать %s: %v", name, err)
	}
	toks, lexErrs := lexer.New(string(data)).Tokenize()
	el := errors.NewErrorList()
	prog := New(toks, el).Parse()
	return prog, el, lexErrs
}

// T025: каркас программы, let/assign/выражение, печать как обычный CallExpr,
// завершение на EOF, SE-ASSIGN-TARGET, parse-clean примеров.

func TestProgramSkeleton(t *testing.T) {
	src := "пусть a = 2 + 3 * 4\nx = x + 1\nпечать(a, b)\n"
	prog, el := parseProgramSrc(t, src)
	if !el.Empty() {
		t.Fatalf("неожиданные ошибки: %v", el.Error())
	}
	if len(prog.Items) != 3 {
		t.Fatalf("Items = %d, хотим 3", len(prog.Items))
	}
	if prog.EOFPos.Line < 1 || prog.EOFPos.Col < 1 {
		t.Errorf("EOFPos не зафиксирован: %+v", prog.EOFPos)
	}

	ls, ok := prog.Items[0].(*ast.LetStmt)
	if !ok {
		t.Fatalf("Items[0] = %T, хотим *LetStmt", prog.Items[0])
	}
	if ls.Name.Name != "a" || sexpr(ls.Value) != "(+ 2 (* 3 4))" {
		t.Errorf("LetStmt неверен: name=%q value=%s", ls.Name.Name, sexpr(ls.Value))
	}

	as, ok := prog.Items[1].(*ast.AssignStmt)
	if !ok {
		t.Fatalf("Items[1] = %T, хотим *AssignStmt", prog.Items[1])
	}
	if as.Name.Name != "x" || sexpr(as.Value) != "(+ x 1)" {
		t.Errorf("AssignStmt неверен: name=%q value=%s", as.Name.Name, sexpr(as.Value))
	}

	es, ok := prog.Items[2].(*ast.ExpressionStmt)
	if !ok {
		t.Fatalf("Items[2] = %T, хотим *ExpressionStmt", prog.Items[2])
	}
	call, ok := es.Expr.(*ast.CallExpr)
	if !ok {
		t.Fatalf("ExpressionStmt.Expr = %T, хотим *CallExpr (печать — обычный вызов)", es.Expr)
	}
	if id, ok := call.Callee.(*ast.Ident); !ok || id.Name != "печать" {
		t.Errorf("Callee не Ident(печать): %T", call.Callee)
	}
	if len(call.Args) != 2 {
		t.Errorf("аргументов %d, хотим 2", len(call.Args))
	}
}

func TestPrintIsPlainCall(t *testing.T) {
	prog, el := parseProgramSrc(t, `печать("Привет, Уклад!")`+"\n")
	if !el.Empty() {
		t.Fatalf("ошибки: %v", el.Error())
	}
	if len(prog.Items) != 1 {
		t.Fatalf("Items = %d, хотим 1", len(prog.Items))
	}
	es := prog.Items[0].(*ast.ExpressionStmt)
	if got := sexpr(es.Expr); got != `(call печать "Привет, Уклад!")` {
		t.Errorf("дерево = %s, хотим (call печать \"Привет, Уклад!\")", got)
	}
}

func TestAssignTargetError(t *testing.T) {
	tests := []struct {
		src string
		col int // колонка токена '='
	}{
		{"x.поле = 5\n", 8},
		{"x[i] = 5\n", 6},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			_, el := parseProgramSrc(t, tt.src)
			if el.Len() != 1 {
				t.Fatalf("ошибок %d, хотим 1 (SE-ASSIGN-TARGET): %v", el.Len(), el.Error())
			}
			var pe errors.ParseError
			if !errAs(el, &pe) {
				t.Fatalf("не ParseError")
			}
			if pe.Msg != msgAssignTarget {
				t.Errorf("Msg = %q, хотим %q", pe.Msg, msgAssignTarget)
			}
			if pe.Pos.Line != 1 || pe.Pos.Col != tt.col {
				t.Errorf("позиция = %+v, хотим {1,%d} (токен '=')", pe.Pos, tt.col)
			}
		})
	}
}

func TestExamplesParseCleanP1(t *testing.T) {
	for _, name := range []string{"hello.ladix", "арифметика.ladix"} {
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
