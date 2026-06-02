package lexer

import "testing"

// T017 [US1]: операторы (жадное макс-совпадение), разделители и скобки.

func TestOperatorsGreedy(t *testing.T) {
	src := "+ - * / // % = == != < <= > >= ."
	toks, errs := lexAll(src)
	requireNoErrors(t, errs)
	requireTypes(t, toks,
		PLUS, MINUS, STAR, SLASH, SLASH_SLASH, PERCENT, ASSIGN, EQ, NEQ,
		LT, LE, GT, GE, DOT, NEWLINE, EOF)
}

func TestOperatorLexemes(t *testing.T) {
	want := map[string]struct {
		typ TokenType
		lex string
	}{
		"+":  {PLUS, "+"},
		"-":  {MINUS, "-"},
		"*":  {STAR, "*"},
		"/":  {SLASH, "/"},
		"//": {SLASH_SLASH, "//"},
		"%":  {PERCENT, "%"},
		"=":  {ASSIGN, "="},
		"==": {EQ, "=="},
		"!=": {NEQ, "!="},
		"<":  {LT, "<"},
		"<=": {LE, "<="},
		">":  {GT, ">"},
		">=": {GE, ">="},
		".":  {DOT, "."},
	}
	for src, exp := range want {
		t.Run(src, func(t *testing.T) {
			toks, errs := lexAll(src)
			requireNoErrors(t, errs)
			if toks[0].Type != exp.typ || toks[0].Lexeme != exp.lex {
				t.Errorf("%q → (%s,%q), хотим (%s,%q)", src, toks[0].Type, toks[0].Lexeme, exp.typ, exp.lex)
			}
		})
	}
}

func TestMaxMunchBoundaries(t *testing.T) {
	tests := []struct {
		src  string
		want []TokenType
	}{
		{"===", []TokenType{EQ, ASSIGN, NEWLINE, EOF}},
		{"///", []TokenType{SLASH_SLASH, SLASH, NEWLINE, EOF}},
		{"<=<", []TokenType{LE, LT, NEWLINE, EOF}},
		{">=>", []TokenType{GE, GT, NEWLINE, EOF}},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			toks, errs := lexAll(tt.src)
			requireNoErrors(t, errs)
			requireTypes(t, toks, tt.want...)
		})
	}
}

func TestDelimitersAndBrackets(t *testing.T) {
	src := ", : ( ) [ ] { }"
	toks, errs := lexAll(src)
	requireNoErrors(t, errs)
	requireTypes(t, toks,
		COMMA, COLON, LPAREN, RPAREN, LBRACKET, RBRACKET, LBRACE, RBRACE,
		NEWLINE, EOF)
}
