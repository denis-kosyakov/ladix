package parser

import (
	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/lexer"
)

// parseTopLevelItem разбирает один элемент верхнего уровня. Декларация функция →
// FunctionDecl подключается в US4 (parse_decl.go); остальное — statements.
func (p *Parser) parseTopLevelItem() ast.TopLevelItem {
	return p.parseStatement()
}

// parseStatement — диспетчер по ведущему токену. Ветви управления (US3) и
// зарезервированные действия (US4) подключаются в своих фазах; по умолчанию —
// statement, начинающийся с выражения (присваивание либо выражение-оператор).
func (p *Parser) parseStatement() ast.Statement {
	switch p.peek().Type {
	case lexer.KW_LET:
		return p.parseLet()
	default:
		return p.parseExprStatement()
	}
}

// parseLet: пусть Ident "=" Expression NEWLINE. Pos() = токен пусть.
func (p *Parser) parseLet() ast.Statement {
	letTok := p.advance() // пусть
	nameTok, _ := p.expect(lexer.IDENT, "имя переменной")
	name := p.identFrom(nameTok)
	p.expect(lexer.ASSIGN, "=")
	value := p.parseExpression()
	p.expect(lexer.NEWLINE, "конец строки")
	return ast.NewLetStmt(toASTPos(letTok.Pos), *name, value)
}

// parseExprStatement разбирает statement, начинающийся с выражения (D-R11):
// разобрать выражение; если следом "=" — это присваивание, lvalue ОБЯЗАН быть
// *ast.Ident (иначе SE-ASSIGN-TARGET на токене "="); иначе — выражение-оператор.
// Оба завершаются NEWLINE.
func (p *Parser) parseExprStatement() ast.Statement {
	expr := p.parseExpression()
	if p.check(lexer.ASSIGN) {
		assignTok := p.advance()
		value := p.parseExpression()
		p.expect(lexer.NEWLINE, "конец строки")
		if id, ok := expr.(*ast.Ident); ok {
			return ast.NewAssignStmt(*id, value)
		}
		p.error(assignTok.Pos, msgAssignTarget)
		return ast.NewExpressionStmt(expr) // best-effort: дерево остаётся валидным
	}
	p.expect(lexer.NEWLINE, "конец строки")
	return ast.NewExpressionStmt(expr)
}
