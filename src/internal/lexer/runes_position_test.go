package lexer

import "testing"

// T026 [US2]: колонка считается в РУНАХ на кириллическом префиксе, не в байтах
// (SC-002, US2.10).

func TestRuneColumnOnCyrillicPrefix(t *testing.T) {
	// "мама мыла @": '@' — 11-я руна (4+1+4+1), но 19-й байт (кириллица 2 байта).
	_, errs := lexAll("мама мыла @")
	le := onlyError(t, errs)
	if le.Pos.Line != 1 || le.Pos.Col != 11 {
		t.Errorf("позиция '@' = %+v, хотим {1 11} (рунная колонка)", le.Pos)
	}
	if le.Msg != "неожиданный символ '@'" {
		t.Errorf("Msg = %q", le.Msg)
	}
}

func TestRuneColumnOfTokenAfterCyrillic(t *testing.T) {
	// ASSIGN после 10-рунного имени — на колонке 12.
	toks, errs := lexAll("переменная = 5")
	requireNoErrors(t, errs)
	requireTypes(t, toks, IDENT, ASSIGN, INT, NEWLINE, EOF)
	if toks[1].Pos.Col != 12 {
		t.Errorf("ASSIGN.Col = %d, хотим 12 (в рунах)", toks[1].Pos.Col)
	}
}
