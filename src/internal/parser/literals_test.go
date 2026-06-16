package parser

import (
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/errors"
)

// T019: граница int64, SE-INT-RANGE и чтение готовых значений из Token.Value.

func TestIntLitAtBoundary(t *testing.T) {
	expr, el := parseExprSrc(t, "9223372036854775807") // MaxInt64
	if !el.Empty() {
		t.Fatalf("MaxInt64 не должен давать ошибку: %v", el.Error())
	}
	il, ok := expr.(*ast.IntLit)
	if !ok {
		t.Fatalf("не IntLit: %T", expr)
	}
	if il.Value != 9223372036854775807 {
		t.Errorf("Value = %d, хотим 9223372036854775807", il.Value)
	}
}

func TestIntLitOverflow(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		lexeme string
	}{
		// MaxInt64+1: знак не сворачивается, поэтому MinInt64 невыразим литералом.
		{"MaxInt64+1", "9223372036854775808", "9223372036854775808"},
		{"30 девяток", strings.Repeat("9", 30), strings.Repeat("9", 30)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, el := parseExprSrc(t, tt.src)
			if el.Len() != 1 {
				t.Fatalf("ошибок %d, хотим 1 (SE-INT-RANGE)", el.Len())
			}
			var pe errors.ParseError
			if !errAs(el, &pe) {
				t.Fatalf("не ParseError")
			}
			want := "целое число '" + tt.lexeme + "' вне диапазона типа Целое"
			if pe.Msg != want {
				t.Errorf("Msg = %q, хотим %q", pe.Msg, want)
			}
			if pe.Pos.Line != 1 || pe.Pos.Col != 1 {
				t.Errorf("позиция = %+v, хотим {1,1} (первая руна литерала)", pe.Pos)
			}
			// узел создаётся всё равно (best-effort), чтобы разбор продолжился
			if _, ok := expr.(*ast.IntLit); !ok {
				t.Errorf("узел не IntLit: %T", expr)
			}
		})
	}
}

func TestLiteralsFromTokenValue(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"3.14", "3.14"},
		{`"привет"`, `"привет"`},
		{"истина", "истина"},
		{"ложь", "ложь"},
		{"пусто", "пусто"},
		{"3дн", "3дн"},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			expr, el := parseExprSrc(t, c.src)
			if !el.Empty() {
				t.Fatalf("%q: неожиданные ошибки %v", c.src, el.Error())
			}
			if got := sexpr(expr); got != c.want {
				t.Errorf("%q → %s, хотим %s", c.src, got, c.want)
			}
		})
	}
}
