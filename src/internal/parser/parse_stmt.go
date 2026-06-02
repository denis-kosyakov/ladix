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
	case lexer.KW_IF:
		return p.parseIf()
	case lexer.KW_WHILE:
		return p.parseWhile()
	case lexer.KW_FOR:
		return p.parseFor()
	case lexer.KW_RETURN:
		return p.parseReturn()
	case lexer.KW_BREAK:
		return p.parseBreak()
	case lexer.KW_CONTINUE:
		return p.parseContinue()
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

// parseBlock разбирает тело конструкции: NEWLINE INDENT Statement+ DEDENT
// (двоеточие потреблено вызывающим). Если после NEWLINE нет INDENT — лексер
// INDENT при пустом теле не эмитит — это пустой блок → SE-EMPTY-BLOCK на реально
// присутствующем токене (D-R12). Pos() блока = токен INDENT.
func (p *Parser) parseBlock() *ast.Block {
	p.expect(lexer.NEWLINE, "конец строки")
	if !p.check(lexer.INDENT) {
		p.error(p.peek().Pos, msgEmptyBlock)
		return ast.NewBlock(toASTPos(p.peek().Pos), nil)
	}
	indent := p.advance() // INDENT
	var stmts []ast.Statement
	for !p.check(lexer.DEDENT) && !p.check(lexer.EOF) {
		stmts = append(stmts, p.parseStatement())
	}
	p.expect(lexer.DEDENT, "конец блока")
	return ast.NewBlock(toASTPos(indent.Pos), stmts)
}

// parseIf: если Cond ":" Block [ElseClause]. Pos() = токен если.
func (p *Parser) parseIf() ast.Statement {
	ifTok := p.advance() // если
	cond := p.parseExpression()
	p.expect(lexer.COLON, ":")
	then := p.parseBlock()
	els := p.parseElse()
	return ast.NewIfStmt(toASTPos(ifTok.Pos), cond, then, els)
}

// parseElse разбирает цепочку иначе/иначе если. nil, если иначе отсутствует.
// `иначе если` — два отдельных ключевых слова (KW_ELSE KW_IF); слитное
// `иначеесли` — IDENT и сюда не попадает (даёт SE-EXPECTED в ветке выражения).
func (p *Parser) parseElse() *ast.ElseClause {
	if !p.check(lexer.KW_ELSE) {
		return nil
	}
	elseTok := p.advance() // иначе
	if p.check(lexer.KW_IF) {
		p.advance() // если
		cond := p.parseExpression()
		p.expect(lexer.COLON, ":")
		then := p.parseBlock()
		next := p.parseElse()
		return ast.NewElseIf(toASTPos(elseTok.Pos), cond, then, next)
	}
	p.expect(lexer.COLON, ":")
	body := p.parseBlock()
	return ast.NewElseBlock(toASTPos(elseTok.Pos), body)
}

// parseWhile: пока Cond ":" Block. Pos() = токен пока.
func (p *Parser) parseWhile() ast.Statement {
	tok := p.advance() // пока
	cond := p.parseExpression()
	p.expect(lexer.COLON, ":")
	body := p.parseBlock()
	return ast.NewWhileStmt(toASTPos(tok.Pos), cond, body)
}

// parseFor: для Ident "в" Expression ":" Block. Pos() = токен для.
func (p *Parser) parseFor() ast.Statement {
	tok := p.advance() // для
	varTok, _ := p.expect(lexer.IDENT, "имя переменной")
	variable := p.identFrom(varTok)
	p.expect(lexer.KW_IN, "в")
	iterable := p.parseExpression()
	p.expect(lexer.COLON, ":")
	body := p.parseBlock()
	return ast.NewForStmt(toASTPos(tok.Pos), *variable, iterable, body)
}

// parseReturn: вернуть [Expression] NEWLINE. Голый вернуть (Value=nil), если за
// ним сразу конец строки; иначе — выражение по множеству FIRST(Expression).
func (p *Parser) parseReturn() ast.Statement {
	tok := p.advance() // вернуть
	var value ast.Expression
	if startsExpression(p.peek().Type) {
		value = p.parseExpression()
	}
	p.expect(lexer.NEWLINE, "конец строки")
	return ast.NewReturnStmt(toASTPos(tok.Pos), value)
}

// parseBreak: прервать NEWLINE.
func (p *Parser) parseBreak() ast.Statement {
	tok := p.advance()
	p.expect(lexer.NEWLINE, "конец строки")
	return ast.NewBreakStmt(toASTPos(tok.Pos))
}

// parseContinue: продолжить NEWLINE.
func (p *Parser) parseContinue() ast.Statement {
	tok := p.advance()
	p.expect(lexer.NEWLINE, "конец строки")
	return ast.NewContinueStmt(toASTPos(tok.Pos))
}
