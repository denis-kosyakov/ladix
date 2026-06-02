package lexer

import "testing"

// T018 [US1]: отступы — INDENT/DEDENT/NEWLINE, прозрачность пустых/комментарных
// строк и переводов в скобках, хвостовой комментарий, закрытие уровней на EOF.

func TestIndentSingleBlock(t *testing.T) {
	src := "если истина:\n    x = 1\n    y = 2"
	toks, errs := lexAll(src)
	requireNoErrors(t, errs)
	requireTypes(t, toks,
		KW_IF, BOOL, COLON, NEWLINE,
		INDENT, IDENT, ASSIGN, INT, NEWLINE,
		IDENT, ASSIGN, INT, NEWLINE,
		DEDENT, EOF)
}

func TestIndentNestedDedentSeriesAtEOF(t *testing.T) {
	src := "если a:\n    если b:\n        x = 1"
	toks, errs := lexAll(src)
	requireNoErrors(t, errs)
	requireTypes(t, toks,
		KW_IF, IDENT, COLON, NEWLINE,
		INDENT, KW_IF, IDENT, COLON, NEWLINE,
		INDENT, IDENT, ASSIGN, INT, NEWLINE,
		DEDENT, DEDENT, EOF)
}

func TestIndentPositions(t *testing.T) {
	src := "если истина:\n    x = 1"
	toks, _ := lexAll(src)
	// INDENT — на колонке 1 строки 2.
	var indent *Token
	for i := range toks {
		if toks[i].Type == INDENT {
			indent = &toks[i]
			break
		}
	}
	if indent == nil {
		t.Fatal("INDENT не найден")
	}
	if indent.Pos.Line != 2 || indent.Pos.Col != 1 {
		t.Errorf("INDENT.Pos = %+v, хотим {2 1}", indent.Pos)
	}
}

func TestBracketTransparency(t *testing.T) {
	// Переводы строк и отступы внутри [ ... ] прозрачны (C-1.2).
	src := "x = [\n    1,\n    2,\n]"
	toks, errs := lexAll(src)
	requireNoErrors(t, errs)
	requireTypes(t, toks,
		IDENT, ASSIGN, LBRACKET, INT, COMMA, INT, COMMA, RBRACKET,
		NEWLINE, EOF)
}

func TestTrailingComment(t *testing.T) {
	// Хвостовой комментарий: NEWLINE эмитится, `#…` отброшен (FR-014/FR-022).
	toks, errs := lexAll("x = 5 # коммент")
	requireNoErrors(t, errs)
	requireTypes(t, toks, IDENT, ASSIGN, INT, NEWLINE, EOF)
}

func TestCommentOnlyLineTransparent(t *testing.T) {
	src := "# только комментарий\nx = 1"
	toks, errs := lexAll(src)
	requireNoErrors(t, errs)
	requireTypes(t, toks, IDENT, ASSIGN, INT, NEWLINE, EOF)
}

func TestBlankLinesTransparent(t *testing.T) {
	src := "x = 1\n\n\ny = 2"
	toks, errs := lexAll(src)
	requireNoErrors(t, errs)
	requireTypes(t, toks,
		IDENT, ASSIGN, INT, NEWLINE,
		IDENT, ASSIGN, INT, NEWLINE, EOF)
}
