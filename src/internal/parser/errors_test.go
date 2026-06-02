package parser

import (
	"testing"

	"github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/lexer"
)

// tok — компактный конструктор токена для тестов строителей текста.
func tok(tt lexer.TokenType, lexeme string) lexer.Token {
	return lexer.Token{Type: tt, Lexeme: lexeme, Pos: errors.Position{Line: 1, Col: 1}}
}

// T006: каждый из 7 строителей даёт ДОСЛОВНО текст контракта (3 канона §13.4 +
// 4 эталона), включая псевдо-лексемы виртуальных токенов.

func TestErrorTextsVerbatim(t *testing.T) {
	if msgChain != "сравнения нельзя сцеплять, используйте 'и': 1 < x и x < 10" {
		t.Errorf("SE-CHAIN: %q", msgChain)
	}
	if msgNestedFn != "вложенные функции не поддерживаются в v1" {
		t.Errorf("SE-NESTED-FN: %q", msgNestedFn)
	}
	if got := msgIntRange("99999999999999999999999999999999"); got != "целочисленный литерал вне диапазона типа Целое '99999999999999999999999999999999'" {
		t.Errorf("SE-INT-RANGE: %q", got)
	}
	if got := msgExpected(":", tok(lexer.IDENT, "x")); got != "ожидалось ':', получено 'x'" {
		t.Errorf("SE-EXPECTED (real): %q", got)
	}
	if got := msgUnexpected(tok(lexer.KW_VALUE, "значение")); got != "неожиданный токен 'значение'" {
		t.Errorf("SE-UNEXPECTED: %q", got)
	}
	if msgEmptyBlock != "пустой блок не допускается, добавьте хотя бы один оператор" {
		t.Errorf("SE-EMPTY-BLOCK: %q", msgEmptyBlock)
	}
	if msgAssignTarget != "неверная цель присваивания: слева от '=' допустима только переменная" {
		t.Errorf("SE-ASSIGN-TARGET: %q", msgAssignTarget)
	}
}

func TestExpectedWithVirtualTokens(t *testing.T) {
	// незакрытая скобка на EOF
	if got := msgExpected(")", lexer.Token{Type: lexer.EOF}); got != "ожидалось ')', получено 'конец файла'" {
		t.Errorf("EOF got: %q", got)
	}
	// 'иначеесли' слитно → ожидалось завершение строки
	if got := msgExpected("конец строки", tok(lexer.IDENT, "иначеесли")); got != "ожидалось 'конец строки', получено 'иначеесли'" {
		t.Errorf("NEWLINE expected: %q", got)
	}
}

func TestLexemeOf(t *testing.T) {
	virt := []struct {
		tt   lexer.TokenType
		want string
	}{
		{lexer.NEWLINE, "конец строки"},
		{lexer.INDENT, "увеличение отступа"},
		{lexer.DEDENT, "конец блока"},
		{lexer.EOF, "конец файла"},
	}
	for _, v := range virt {
		if got := lexemeOf(lexer.Token{Type: v.tt}); got != v.want {
			t.Errorf("lexemeOf(%s) = %q, хотим %q", v.tt, got, v.want)
		}
	}
	if got := lexemeOf(tok(lexer.RPAREN, ")")); got != ")" {
		t.Errorf("lexemeOf(RPAREN) = %q, хотим \")\"", got)
	}
}
