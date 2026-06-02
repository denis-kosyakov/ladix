package lexer

import "testing"

// T019 [US1]: сквозные потоки приёмки US1; поток ВСЕГДА завершается одним EOF
// (SC-001/SC-003).

func TestStreamLetStatement(t *testing.T) {
	toks, errs := lexAll("пусть x = 5_000")
	requireNoErrors(t, errs)
	requireTypes(t, toks, KW_LET, IDENT, ASSIGN, INT, NEWLINE, EOF)
	if toks[1].Lexeme != "x" {
		t.Errorf("IDENT.Lexeme = %q, хотим x", toks[1].Lexeme)
	}
	if toks[3].Lexeme != "5000" {
		t.Errorf("INT.Lexeme = %q, хотим 5000", toks[3].Lexeme)
	}
}

func TestStreamFunctionDecl(t *testing.T) {
	src := "функция f(a, b):\n    вернуть a + b"
	toks, errs := lexAll(src)
	requireNoErrors(t, errs)
	requireTypes(t, toks,
		KW_FUNC, IDENT, LPAREN, IDENT, COMMA, IDENT, RPAREN, COLON, NEWLINE,
		INDENT, KW_RETURN, IDENT, PLUS, IDENT, NEWLINE,
		DEDENT, EOF)
}

func TestStreamAlwaysExactlyOneEOF(t *testing.T) {
	// SC-003: даже на ошибочных и пустых входах — ровно один завершающий EOF.
	inputs := []string{
		"",
		"x",
		"x = 1\n",
		"если a:\n    x = 1",
		"(",              // незакрытая скобка — не диагностируется лексером
		"\"unterminated", // незакрытая строка
		"@ @ @",          // несколько ошибок
	}
	for _, src := range inputs {
		toks, _ := lexAll(src)
		if lastType(toks) != EOF {
			t.Errorf("%q: последний токен = %s, хотим EOF", src, lastType(toks))
		}
		count := 0
		for _, tk := range toks {
			if tk.Type == EOF {
				count++
			}
		}
		if count != 1 {
			t.Errorf("%q: число EOF = %d, хотим 1", src, count)
		}
	}
}
