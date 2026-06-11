package parser

import (
	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/lexer"
)

// parseTopLevelItem разбирает один элемент верхнего уровня: функция/источник/
// метрика/процесс → декларация; отложенные триггеры (когда)/значение/{ } →
// SE-UNEXPECTED; иначе — statement. nil-результат (поглощённая ошибочная
// конструкция) вызывающий пропускает.
func (p *Parser) parseTopLevelItem() ast.TopLevelItem {
	if p.check(lexer.KW_FUNC) {
		return p.parseFunctionDecl()
	}
	if p.check(lexer.KW_SOURCE) {
		return p.parseSourceDecl()
	}
	if p.check(lexer.KW_METRIC) {
		return p.parseMetricDecl()
	}
	if p.check(lexer.KW_PROCESS) {
		return p.parseProcessDecl()
	}
	if isUnexpectedTopLevel(p.peek().Type) {
		bad := p.advance() // потребляем ведущий токен (прогресс), затем синхронизация
		p.error(bad.Pos, msgUnexpected(bad))
		return nil
	}
	if s := p.parseStatement(); s != nil {
		return s
	}
	return nil
}

// isUnexpectedTopLevel — ведущие токены, недопустимые на верхнем уровне: отложенные
// триггеры (когда), значение, фигурные скобки. Все дают SE-UNEXPECTED (FR-017,
// guardrail 12). источник/метрика парсятся с 004 (§SM-3), процесс — с 005
// (§PM-3, D-6) — здесь их нет.
func isUnexpectedTopLevel(t lexer.TokenType) bool {
	switch t {
	case lexer.KW_WHEN, lexer.KW_VALUE,
		lexer.LBRACE, lexer.RBRACE:
		return true
	default:
		return false
	}
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
	case lexer.KW_FUNC:
		return p.parseNestedFunc()
	case lexer.KW_SET, lexer.KW_CALL, lexer.KW_NOTIFY:
		return p.parseStepAction()
	default:
		return p.parseExprStatement()
	}
}

// parseNestedFunc обрабатывает функцию внутри блока: вложенные функции запрещены
// синтаксически (grammar §4) → SE-NESTED-FN на токене функция. Структура
// поглощается best-effort; statement не порождается (вызывающий пропускает nil).
func (p *Parser) parseNestedFunc() ast.Statement {
	p.errorLocal(p.peek().Pos, msgNestedFn)
	p.parseFunctionDecl() // поглотить тело, результат отбросить
	return nil
}

// parseStepAction разбирает зарезервированные действия шага (grammar §7). Гард
// «только в шаге процесса» НЕ проверяется (guardrail 6) — узлы строятся всюду.
func (p *Parser) parseStepAction() ast.Statement {
	switch p.peek().Type {
	case lexer.KW_SET: // присвоить Ident "=" Expression NEWLINE
		tok := p.advance()
		nameTok, _ := p.expect(lexer.IDENT, "имя переменной")
		name := p.identFrom(nameTok)
		p.expect(lexer.ASSIGN, "=")
		value := p.parseExpression()
		p.expect(lexer.NEWLINE, "конец строки")
		return ast.NewAssignAction(toASTPos(tok.Pos), *name, value)
	case lexer.KW_CALL: // вызвать Ident "(" ArgList? ")" NEWLINE
		tok := p.advance()
		name, args := p.parseActionCall()
		p.expect(lexer.NEWLINE, "конец строки")
		return ast.NewCallAction(toASTPos(tok.Pos), name, args)
	default: // KW_NOTIFY: уведомить Ident "(" ArgList? ")" NEWLINE
		tok := p.advance()
		name, args := p.parseActionCall()
		p.expect(lexer.NEWLINE, "конец строки")
		return ast.NewNotifyAction(toASTPos(tok.Pos), name, args)
	}
}

// parseActionCall разбирает общий хвост вызвать/уведомить: Ident "(" ArgList? ")".
func (p *Parser) parseActionCall() (ast.Ident, []ast.Expression) {
	nameTok, _ := p.expect(lexer.IDENT, "имя")
	name := p.identFrom(nameTok)
	p.expect(lexer.LPAREN, "(")
	args := p.parseArgList(lexer.RPAREN)
	p.expect(lexer.RPAREN, ")")
	return *name, args
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
		p.errorLocal(assignTok.Pos, msgAssignTarget)
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
		p.errorLocal(p.peek().Pos, msgEmptyBlock)
		return ast.NewBlock(toASTPos(p.peek().Pos), nil)
	}
	indent := p.advance() // INDENT
	var stmts []ast.Statement
	for !p.check(lexer.DEDENT) && !p.check(lexer.EOF) {
		p.suppress = false // граница оператора внутри блока
		before := p.pos
		if s := p.parseStatement(); s != nil {
			stmts = append(stmts, s)
		}
		if p.pos == before {
			p.advance() // backstop: гарантия прогресса
		}
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
