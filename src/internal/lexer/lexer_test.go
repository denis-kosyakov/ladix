package lexer

import "testing"

// T012: ридер — отбрасывание ведущего BOM, нормализация \r\n и одиночного \r → \n
// (FR-011), пустой ввод → ровно один EOF в позиции (1,1) (SC-003, C-5).

func TestEmptyInputSingleEOF(t *testing.T) {
	toks, errs := lexAll("")
	requireNoErrors(t, errs)
	requireTypes(t, toks, EOF)
	if toks[0].Pos.Line != 1 || toks[0].Pos.Col != 1 {
		t.Errorf("EOF Pos = %+v, хотим {1 1}", toks[0].Pos)
	}
}

func TestOnlyWhitespaceAndCommentsSingleEOF(t *testing.T) {
	// Пустые строки и строки-комментарии прозрачны: остаётся только EOF.
	toks, errs := lexAll("\n\n   \n# только комментарий\n")
	requireNoErrors(t, errs)
	requireTypes(t, toks, EOF)
}

func TestLeadingBOMStripped(t *testing.T) {
	toks, errs := lexAll("\uFEFFx")
	requireNoErrors(t, errs)
	requireTypes(t, toks, IDENT, NEWLINE, EOF)
	if toks[0].Pos.Col != 1 {
		t.Errorf("после отброшенного BOM IDENT.Col = %d, хотим 1", toks[0].Pos.Col)
	}
}

func TestCRLFNormalization(t *testing.T) {
	toks, errs := lexAll("a\r\nb")
	requireNoErrors(t, errs)
	requireTypes(t, toks, IDENT, NEWLINE, IDENT, NEWLINE, EOF)
	// 'b' должен оказаться на строке 2 (CRLF → один \n).
	if toks[2].Pos.Line != 2 {
		t.Errorf("'b' Line = %d, хотим 2", toks[2].Pos.Line)
	}
}

func TestLoneCRNormalization(t *testing.T) {
	toks, errs := lexAll("a\rb")
	requireNoErrors(t, errs)
	requireTypes(t, toks, IDENT, NEWLINE, IDENT, NEWLINE, EOF)
	if toks[2].Pos.Line != 2 {
		t.Errorf("'b' Line = %d, хотим 2", toks[2].Pos.Line)
	}
}

func TestEOFAfterTrailingNewline(t *testing.T) {
	// После финального \n EOF встаёт на следующую строку, колонка 1 (D-R11).
	toks, _ := lexAll("abc\n\n\n")
	last := toks[len(toks)-1]
	if last.Type != EOF {
		t.Fatalf("последний токен = %s, хотим EOF", last.Type)
	}
	if last.Pos.Line != 4 || last.Pos.Col != 1 {
		t.Errorf("EOF Pos = %+v, хотим {4 1} (\"abc\\n\\n\\n\")", last.Pos)
	}
}
