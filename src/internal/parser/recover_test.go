package parser

import (
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/lexer"
)

// T042: panic-mode восстановление — несколько независимых ошибок без фантомного
// каскада, бюджет, отсутствие Go stack trace, best-effort Program (SC-005, FR-025).

func TestMultipleIndependentErrors(t *testing.T) {
	// две независимые ошибки на разных строках → ровно две диагностики
	prog, el := parseProgramSrc(t, "значение\n{\n")
	if prog == nil {
		t.Fatalf("Program == nil; ожидается best-effort дерево")
	}
	if el.Len() != 2 {
		t.Fatalf("ошибок %d, хотим 2 (без фантомного каскада):\n%s", el.Len(), el.Error())
	}
	e0 := el.Errors()[0].(errors.ParseError)
	e1 := el.Errors()[1].(errors.ParseError)
	if e0.Pos.Line != 1 || e1.Pos.Line != 2 {
		t.Errorf("строки ошибок = %d,%d, хотим 1,2", e0.Pos.Line, e1.Pos.Line)
	}
	if e0.Msg != "неожиданный токен 'значение'" || e1.Msg != "неожиданный токен '{'" {
		t.Errorf("сообщения: %q / %q", e0.Msg, e1.Msg)
	}
}

func TestErrorOnOneLineDoesNotCascadeNextValidLine(t *testing.T) {
	// ошибка на строке 1 не мешает корректно разобрать строку 2
	prog, el := parseProgramSrc(t, "1 < y < 10\nпечать(42)\n")
	if el.Len() != 1 {
		t.Fatalf("ошибок %d, хотим 1 (без каскада на валидную строку):\n%s", el.Len(), el.Error())
	}
	if len(prog.Items) != 2 {
		t.Errorf("Items = %d, хотим 2 (ошибочное выражение + печать)", len(prog.Items))
	}
}

func TestErrorBudgetShared(t *testing.T) {
	// много ошибочных строк → накопитель не превышает мягкий бюджет (общий с лексером)
	var b strings.Builder
	for i := 0; i < errors.DefaultErrorBudget+10; i++ {
		b.WriteString("значение\n")
	}
	_, el := parseProgramSrc(t, b.String())
	if el.Len() != errors.DefaultErrorBudget {
		t.Errorf("накоплено %d, хотим мягкий предел %d", el.Len(), errors.DefaultErrorBudget)
	}
}

func TestNoGoStackTrace(t *testing.T) {
	_, el := parseProgramSrc(t, "значение\nx.поле = 5\n1 < y < 10\n")
	out := el.Error()
	for _, marker := range []string{"goroutine", ".go:", "panic:", "runtime."} {
		if strings.Contains(out, marker) {
			t.Errorf("в выводе обнаружен Go stack trace (%q):\n%s", marker, out)
		}
	}
}

// T043: прямой тест synchronize() — потребляет NEWLINE/DEDENT, останавливается на
// ведущем ключевом слове, не потребляя его.

func TestSynchronizeConsumesNewline(t *testing.T) {
	// junk... NEWLINE пусть : отбросить до NEWLINE включительно, остановиться на пусть
	toks := []lexer.Token{
		{Type: lexer.RPAREN, Lexeme: ")", Pos: errors.Position{Line: 1, Col: 1}},
		{Type: lexer.COMMA, Lexeme: ",", Pos: errors.Position{Line: 1, Col: 2}},
		{Type: lexer.NEWLINE, Pos: errors.Position{Line: 1, Col: 3}},
		{Type: lexer.KW_LET, Lexeme: "пусть", Pos: errors.Position{Line: 2, Col: 1}},
		{Type: lexer.EOF, Pos: errors.Position{Line: 2, Col: 6}},
	}
	p := New(toks, nil)
	p.synchronize()
	if !p.check(lexer.KW_LET) {
		t.Errorf("после synchronize peek = %s, хотим KW_LET (NEWLINE потреблён, ключевое слово — нет)", p.peek().Type)
	}
}

// T045: examples/ошибка.ladix синтаксически валиден — дефект (деление на ноль) —
// рантайм, парсер ошибок не даёт.
func TestErrorExampleParsesClean(t *testing.T) {
	prog, el, lexErrs := parseExampleFile(t, "ошибка.ladix")
	if !lexErrs.Empty() {
		t.Fatalf("ошибка.ladix: лексические ошибки: %v", lexErrs.Error())
	}
	if !el.Empty() {
		t.Fatalf("ошибка.ladix: синтаксические ошибки (дефект — рантайм): %v", el.Error())
	}
	if len(prog.Items) == 0 {
		t.Errorf("ошибка.ladix: пустой Program.Items")
	}
}

func TestSynchronizeStopsAtLeadKeyword(t *testing.T) {
	// текущий токен уже ведущее ключевое слово → синхронизация не двигается
	toks := []lexer.Token{
		{Type: lexer.KW_IF, Lexeme: "если", Pos: errors.Position{Line: 1, Col: 1}},
		{Type: lexer.EOF, Pos: errors.Position{Line: 1, Col: 5}},
	}
	p := New(toks, nil)
	p.synchronize()
	if !p.check(lexer.KW_IF) {
		t.Errorf("synchronize не должен потреблять ведущее ключевое слово, peek = %s", p.peek().Type)
	}
}
