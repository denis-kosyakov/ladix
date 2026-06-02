package parser

import (
	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/lexer"
)

// Parser — ручной recursive-descent разборщик. Создаётся явно (New); пакет-
// уровневого изменяемого состояния нет (FR-029, guardrail 9). Вход — поток
// токенов лексера (контракт token-stream.md: ровно один EOF, баланс INDENT/
// DEDENT, INT строкой). Выход Parse() — *ast.Program (всегда, best-effort) и
// накопленные ошибки в общем *errors.ErrorList.
type Parser struct {
	tokens   []lexer.Token
	pos      int               // индекс текущего токена
	errs     *errors.ErrorList // общий накопитель (лексика+синтаксис)
	suppress bool              // panic-mode: подавление новых ошибок до синхронизации (US5)
}

// New создаёт парсер для потока токенов. Если errs == nil, создаётся свой
// накопитель (изоляция для тестов); в собранном пайплайне сюда протаскивается
// общий *ErrorList лексера (общий бюджет ≈20, §13.2).
func New(tokens []lexer.Token, errs *errors.ErrorList) *Parser {
	if errs == nil {
		errs = errors.NewErrorList()
	}
	if len(tokens) == 0 {
		// Защита: контракт лексера гарантирует завершающий EOF, но изолированный
		// вызов с пустым срезом не должен паниковать.
		tokens = []lexer.Token{{Type: lexer.EOF, Pos: errors.Position{Line: 1, Col: 1}}}
	}
	return &Parser{tokens: tokens, errs: errs}
}

// peek возвращает текущий токен без продвижения; на конце потока — EOF.
func (p *Parser) peek() lexer.Token {
	if p.pos >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[p.pos]
}

// peekAt смотрит на o токенов вперёд (o ≥ 0); за концом потока — завершающий EOF.
func (p *Parser) peekAt(o int) lexer.Token {
	i := p.pos + o
	if i >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[i]
}

// advance возвращает текущий токен и сдвигает курсор; на завершающем EOF курсор
// не двигается (повторный peek даёт EOF).
func (p *Parser) advance() lexer.Token {
	tok := p.peek()
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return tok
}

// check сообщает, имеет ли текущий токен вид t.
func (p *Parser) check(t lexer.TokenType) bool { return p.peek().Type == t }

// match потребляет текущий токен, если он вида t, и сообщает об этом.
func (p *Parser) match(t lexer.TokenType) bool {
	if p.check(t) {
		p.advance()
		return true
	}
	return false
}

// expect потребляет токен вида t. При несовпадении эмитит SE-EXPECTED
// (ожидалось expected) на позиции текущего токена и возвращает ok=false БЕЗ
// продвижения (вызвавший решает про восстановление — US5).
func (p *Parser) expect(t lexer.TokenType, expected string) (lexer.Token, bool) {
	if p.check(t) {
		return p.advance(), true
	}
	p.error(p.peek().Pos, msgExpected(expected, p.peek()))
	return p.peek(), false
}

// error регистрирует синтаксическую ошибку в общем накопителе. В Фазе 0 флаг
// подавления — заглушка (всегда false); полное panic-mode-восстановление со
// сбросом suppress в точке синхронизации подключается в US5 (recover.go).
func (p *Parser) error(pos errors.Position, msg string) {
	if p.suppress {
		return
	}
	p.errs.Add(errors.ParseError{Pos: pos, Msg: msg})
}

// Parse — точка входа: строит Program из top-level-элементов до EOF и фиксирует
// EOFPos (FR-007). Program возвращается всегда (best-effort даже при ошибках).
func (p *Parser) Parse() *ast.Program {
	prog := &ast.Program{}
	for !p.check(lexer.EOF) {
		if item := p.parseTopLevelItem(); item != nil {
			prog.Items = append(prog.Items, item)
		}
	}
	prog.EOFPos = toASTPos(p.peek().Pos)
	return prog
}
