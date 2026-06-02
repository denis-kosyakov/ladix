package parser

import (
	"strconv"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/errors"
)

// parseExprSrc лексирует src и разбирает одно выражение (для табличных тестов
// приоритетов). Возвращает выражение и накопитель синтаксических ошибок.
func parseExprSrc(t *testing.T, src string) (ast.Expression, *errors.ErrorList) {
	t.Helper()
	toks := lexTokens(t, src)
	el := errors.NewErrorList()
	p := New(toks, el)
	return p.parseExpression(), el
}

// sexpr рендерит выражение в нормализованную S-форму для сверки структуры (без
// позиций): приоритеты и ассоциативность видны по вложенности.
func sexpr(e ast.Expression) string {
	switch n := e.(type) {
	case *ast.BinaryExpr:
		return "(" + n.Op.String() + " " + sexpr(n.Left) + " " + sexpr(n.Right) + ")"
	case *ast.UnaryExpr:
		return "(" + n.Op.String() + " " + sexpr(n.Operand) + ")"
	case *ast.CallExpr:
		s := "(call " + sexpr(n.Callee)
		for _, a := range n.Args {
			s += " " + sexpr(a)
		}
		return s + ")"
	case *ast.IndexExpr:
		return "(index " + sexpr(n.Target) + " " + sexpr(n.Index) + ")"
	case *ast.FieldExpr:
		return "(field " + sexpr(n.Target) + " " + n.Field.Name + ")"
	case *ast.ListLit:
		s := "(list"
		for _, el := range n.Elements {
			s += " " + sexpr(el)
		}
		return s + ")"
	case *ast.IntLit:
		return strconv.FormatInt(n.Value, 10)
	case *ast.FloatLit:
		return strconv.FormatFloat(n.Value, 'g', -1, 64)
	case *ast.StringLit:
		return strconv.Quote(n.Value)
	case *ast.BoolLit:
		if n.Value {
			return "истина"
		}
		return "ложь"
	case *ast.NoneLit:
		return "пусто"
	case *ast.DurationLit:
		return n.Amount + n.Unit
	case *ast.Ident:
		return n.Name
	default:
		return "?"
	}
}

func TestPrecedenceTable(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"2 + 3 * 4", "(+ 2 (* 3 4))"},                  // умножение глубже сложения
		{"(2 + 3) * 4", "(* (+ 2 3) 4)"},                // группировка свёрнута
		{"a - b - c", "(- (- a b) c)"},                  // лево-ассоциативность
		{"10 / 2 / 5", "(/ (/ 10 2) 5)"},                // то же
		{"не x и y", "(и (не x) y)"},                    // не выше и
		{"-5", "(- 5)"},                                 // знак не сворачивается
		{"x > -10 и x < 0", "(и (> x (- 10)) (< x 0))"}, // сравнение выше и
		{"данные[i].поле(1, 2,)", "(call (field (index данные i) поле) 1 2)"}, // постфикс + висящая запятая
		{`[1, "две", истина,]`, `(list 1 "две" истина)`},                      // гетерогенность + висящая запятая
		{"[]", "(list)"}, // пустой список
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			expr, el := parseExprSrc(t, tt.src)
			if !el.Empty() {
				t.Fatalf("неожиданные ошибки для %q: %v", tt.src, el.Error())
			}
			if got := sexpr(expr); got != tt.want {
				t.Errorf("%q → %s, хотим %s", tt.src, got, tt.want)
			}
		})
	}
}

func TestChainedComparisonError(t *testing.T) {
	expr, el := parseExprSrc(t, "1 < x < 10")
	if el.Len() != 1 {
		t.Fatalf("ошибок %d, хотим ровно 1 (SE-CHAIN)", el.Len())
	}
	var pe errors.ParseError
	if !errAs(el, &pe) {
		t.Fatalf("ошибка не ParseError")
	}
	if pe.Msg != msgChain {
		t.Errorf("Msg = %q, хотим %q", pe.Msg, msgChain)
	}
	if pe.Pos.Line != 1 || pe.Pos.Col != 7 {
		t.Errorf("позиция SE-CHAIN = %+v, хотим {1,7} (второй CompOp)", pe.Pos)
	}
	// первое сравнение всё же построено (best-effort)
	if got := sexpr(expr); got != "(< 1 x)" {
		t.Errorf("дерево = %s, хотим (< 1 x)", got)
	}
}
