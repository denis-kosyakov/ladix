package parser

import (
	stderrors "errors"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/lexer"
)

// lexTokens прогоняет лексер и возвращает поток токенов (для тестов «вход → AST»).
func lexTokens(t *testing.T, src string) []lexer.Token {
	t.Helper()
	toks, _ := lexer.New(src).Tokenize()
	return toks
}

// T014: каркас Parser — пустой/только-EOF ввод, базовый курсор, expect.

func TestParseEmptyProgram(t *testing.T) {
	for _, src := range []string{"", "   ", "\n\n", "# только комментарий\n"} {
		toks := lexTokens(t, src)
		p := New(toks, nil)
		prog := p.Parse()
		if len(prog.Items) != 0 {
			t.Errorf("src %q: Items = %d, хотим 0", src, len(prog.Items))
		}
		if prog.EOFPos.Line < 1 || prog.EOFPos.Col < 1 {
			t.Errorf("src %q: EOFPos = %+v, хотим валидную позицию", src, prog.EOFPos)
		}
	}
}

func TestPeekAdvanceCheck(t *testing.T) {
	toks := []lexer.Token{
		{Type: lexer.INT, Lexeme: "1", Pos: errors.Position{Line: 1, Col: 1}},
		{Type: lexer.PLUS, Lexeme: "+", Pos: errors.Position{Line: 1, Col: 3}},
		{Type: lexer.INT, Lexeme: "2", Pos: errors.Position{Line: 1, Col: 5}},
		{Type: lexer.EOF, Pos: errors.Position{Line: 1, Col: 6}},
	}
	p := New(toks, nil)
	if !p.check(lexer.INT) {
		t.Fatalf("check(INT) ложно на старте, peek=%s", p.peek().Type)
	}
	if first := p.advance(); first.Type != lexer.INT {
		t.Errorf("advance().Type = %s, хотим INT", first.Type)
	}
	if !p.check(lexer.PLUS) {
		t.Errorf("после advance ждём PLUS, peek=%s", p.peek().Type)
	}
	if p.peekAt(1).Type != lexer.INT {
		t.Errorf("peekAt(1) = %s, хотим INT", p.peekAt(1).Type)
	}
	// advance на завершающем EOF не выходит за границы
	for i := 0; i < 10; i++ {
		p.advance()
	}
	if p.peek().Type != lexer.EOF {
		t.Errorf("после переполнения advance peek = %s, хотим EOF", p.peek().Type)
	}
}

func TestExpectMismatchEmitsExpected(t *testing.T) {
	toks := []lexer.Token{
		{Type: lexer.INT, Lexeme: "1", Pos: errors.Position{Line: 1, Col: 1}},
		{Type: lexer.EOF, Pos: errors.Position{Line: 1, Col: 2}},
	}
	el := errors.NewErrorList()
	p := New(toks, el)
	p.advance() // съели INT, текущий — EOF
	if _, ok := p.expect(lexer.RPAREN, ")"); ok {
		t.Fatalf("expect(RPAREN) должен был не совпасть на EOF")
	}
	if el.Len() != 1 {
		t.Fatalf("ожидалась 1 ошибка SE-EXPECTED, накоплено %d", el.Len())
	}
	var pe errors.ParseError
	if !stderrors.As(el, &pe) {
		t.Fatalf("ошибка не ParseError")
	}
	if pe.Msg != "ожидалось ')', получено 'конец файла'" {
		t.Errorf("Msg = %q, хотим \"ожидалось ')', получено 'конец файла'\"", pe.Msg)
	}
	// успешный expect потребляет и возвращает ok
	if _, ok := p.expect(lexer.EOF, "конец файла"); !ok {
		t.Errorf("expect(EOF) должен совпасть")
	}
}
