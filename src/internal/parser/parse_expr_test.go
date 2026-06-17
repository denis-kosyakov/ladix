package parser

import (
	"strconv"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/lexer"
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
	case *ast.RunProcessExpr:
		s := "(run " + n.Process.Name
		for _, a := range n.Args {
			s += " " + sexpr(a)
		}
		return s + ")"
	case *ast.CallExternalExpr:
		s := "(call-ext " + n.Target.Name
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
	case *ast.ValueExpr:
		return "значение"
	case *ast.EventExpr:
		return "событие"
	default:
		return "?"
	}
}

// T008 (010-A1, §SC-D-RESERVE/§SC-10 #7, ИНВЕРСИЯ, конституция VI): ТЕСТ-ЗАМОК
// нижнего слоя инварианта «тип(x) недостижим». KW_TYPE НЕ начинает выражение
// (его НЕТ в parsePrimary/startsExpression) → `тип(5)` = парс-ошибка
// «неожиданный элемент 'тип'», а НЕ успешный разбор вызова builtin `тип`. Замок
// краснеет, если KW_TYPE попадёт в parsePrimary (builtin `тип` стал бы достижим).
func TestTypeKeywordNotExpression(t *testing.T) {
	_, el := parseExprSrc(t, "тип(5)")
	if el.Len() == 0 {
		t.Fatalf("ожидалась парс-ошибка: KW_TYPE не начинает выражение")
	}
	pe, ok := el.Errors()[0].(errors.ParseError)
	if !ok {
		t.Fatalf("ошибка не ParseError: %T", el.Errors()[0])
	}
	want := "неожиданный элемент 'тип'"
	if pe.Msg != want {
		t.Errorf("Msg = %q, хотим %q", pe.Msg, want)
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

// C-PARSE-2/3 (B1, 013): вызвать в позиции выражения → CallExternalExpr с
// целью-именем и аргументами; НЕ CallExpr, НЕ CallAction. Инверсия: убрать
// case KW_CALL из parsePrimary → вызвать уходит в default-ветку (ошибка).
func TestParseCallExprInExpressionPosition(t *testing.T) {
	expr, el := parseExprSrc(t, `вызвать crm("к")`)
	if !el.Empty() {
		t.Fatalf("неожиданные ошибки разбора: %v", el.Error())
	}
	ce, ok := expr.(*ast.CallExternalExpr)
	if !ok {
		t.Fatalf("выражение типа %T, хотим *ast.CallExternalExpr", expr)
	}
	if ce.Target.Name != "crm" {
		t.Errorf("Target.Name = %q, хотим \"crm\"", ce.Target.Name)
	}
	if len(ce.Args) != 1 {
		t.Errorf("Args = %d, хотим 1", len(ce.Args))
	}
	if ce.Pos().Line != 1 || ce.Pos().Col != 1 {
		t.Errorf("Pos() = %+v, хотим токен вызвать {1,1}", ce.Pos())
	}
	// Без скобок — Args пуст (скобки опциональны, как у RunProcessExpr).
	bare, el2 := parseExprSrc(t, `вызвать сервис`)
	if !el2.Empty() {
		t.Fatalf("неожиданные ошибки (без скобок): %v", el2.Error())
	}
	if bc, ok := bare.(*ast.CallExternalExpr); !ok || len(bc.Args) != 0 {
		t.Errorf("вызвать сервис → %T, Args не пуст; хотим CallExternalExpr с 0 арг", bare)
	}
}

// C-PARSE-2 FIRST-set (B1, 013): startsExpression(KW_CALL)==true; вызвать
// распознаётся как начало выражения в позиции аргумента и элемента списка.
// Инверсия: не добавлять KW_CALL в startsExpression / parsePrimary → red.
func TestStartsExpressionCall(t *testing.T) {
	if !startsExpression(lexer.KW_CALL) {
		t.Fatalf("startsExpression(KW_CALL) = false, хотим true (FIRST-set)")
	}
	// аргумент: печать(вызвать сервис())
	arg, el := parseExprSrc(t, `печать(вызвать сервис())`)
	if !el.Empty() {
		t.Fatalf("ошибки (аргумент): %v", el.Error())
	}
	call, ok := arg.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		t.Fatalf("печать(...) → %T с %d арг, хотим CallExpr с 1", arg, len(call.Args))
	}
	if _, ok := call.Args[0].(*ast.CallExternalExpr); !ok {
		t.Errorf("аргумент типа %T, хотим *ast.CallExternalExpr", call.Args[0])
	}
	// элемент списка: [вызвать a(), 1]
	lst, el2 := parseExprSrc(t, `[вызвать a(), 1]`)
	if !el2.Empty() {
		t.Fatalf("ошибки (элемент списка): %v", el2.Error())
	}
	list, ok := lst.(*ast.ListLit)
	if !ok || len(list.Elements) != 2 {
		t.Fatalf("[...] → %T, хотим ListLit с 2 элементами", lst)
	}
	if _, ok := list.Elements[0].(*ast.CallExternalExpr); !ok {
		t.Errorf("элемент[0] типа %T, хотим *ast.CallExternalExpr", list.Elements[0])
	}
}

// C-PARSE-3 постфикс (B1, 013): вызвать crm(x).статус → FieldExpr{Target:
// CallExternalExpr} — постфикс навешивается цепочкой parsePostfix, скобки —
// часть узла вызова. Инверсия: поглотить .статус внутрь parseCallExternalExpr
// → структура ≠ FieldExpr{Target:CallExternalExpr}.
func TestParseCallExprPostfix(t *testing.T) {
	expr, el := parseExprSrc(t, `вызвать crm(x).статус`)
	if !el.Empty() {
		t.Fatalf("неожиданные ошибки разбора: %v", el.Error())
	}
	fld, ok := expr.(*ast.FieldExpr)
	if !ok {
		t.Fatalf("выражение типа %T, хотим *ast.FieldExpr (постфикс .статус)", expr)
	}
	if fld.Field.Name != "статус" {
		t.Errorf("Field.Name = %q, хотим \"статус\"", fld.Field.Name)
	}
	if _, ok := fld.Target.(*ast.CallExternalExpr); !ok {
		t.Errorf("Target типа %T, хотим *ast.CallExternalExpr", fld.Target)
	}
}
