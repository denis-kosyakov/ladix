package parser

import (
	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/lexer"
)

// parseExpression — точка входа каскада приоритетов (низший→высший):
// LogicOr → LogicAnd → LogicNot → Comparison → Additive → Multiplicative →
// Unary → Postfix → Primary (SPEC §5, grammar §9, contracts/ast.md §C-2).
func (p *Parser) parseExpression() ast.Expression {
	return p.parseLogicOr()
}

// startsExpression сообщает, может ли токен начать выражение (FIRST(Expression)).
// Используется для различения голого `вернуть` и `вернуть E`, а также ветки
// statement, начинающейся с выражения.
func startsExpression(t lexer.TokenType) bool {
	switch t {
	case lexer.INT, lexer.FLOAT, lexer.STRING, lexer.BOOL, lexer.NONE, lexer.DURATION,
		lexer.IDENT, lexer.KW_NOT, lexer.MINUS, lexer.LPAREN, lexer.LBRACKET, lexer.KW_RUN:
		return true
	default:
		return false
	}
}

func (p *Parser) parseLogicOr() ast.Expression {
	left := p.parseLogicAnd()
	for p.check(lexer.KW_OR) {
		opTok := p.advance()
		right := p.parseLogicAnd()
		left = ast.NewBinaryExpr(toASTPos(opTok.Pos), ast.OpOr, left, right)
	}
	return left
}

func (p *Parser) parseLogicAnd() ast.Expression {
	left := p.parseLogicNot()
	for p.check(lexer.KW_AND) {
		opTok := p.advance()
		right := p.parseLogicNot()
		left = ast.NewBinaryExpr(toASTPos(opTok.Pos), ast.OpAnd, left, right)
	}
	return left
}

// parseLogicNot — унарное `не` (право-ассоциативно, рекурсией). `не` выше и/или,
// но ниже сравнений: `не x и y` → и(не x, y); `не x == y` → не(x == y).
func (p *Parser) parseLogicNot() ast.Expression {
	if p.check(lexer.KW_NOT) {
		opTok := p.advance()
		operand := p.parseLogicNot()
		return ast.NewUnaryExpr(toASTPos(opTok.Pos), ast.OpNot, operand)
	}
	return p.parseComparison()
}

// parseComparison допускает НЕ более одного оператора сравнения. Повторный CompOp
// подряд → SE-CHAIN на позиции второго оператора (FR-019). Остаток цепочки
// дочитывается best-effort без новых ошибок.
func (p *Parser) parseComparison() ast.Expression {
	left := p.parseAdditive()
	op, ok := compOpOf(p.peek().Type)
	if !ok {
		return left
	}
	opTok := p.advance()
	right := p.parseAdditive()
	result := ast.NewBinaryExpr(toASTPos(opTok.Pos), op, left, right)

	if _, chained := compOpOf(p.peek().Type); chained {
		p.error(p.peek().Pos, msgChain)
		for {
			if _, more := compOpOf(p.peek().Type); !more {
				break
			}
			p.advance()
			p.parseAdditive()
		}
	}
	return result
}

func (p *Parser) parseAdditive() ast.Expression {
	left := p.parseMultiplicative()
	for {
		var op ast.BinOp
		switch p.peek().Type {
		case lexer.PLUS:
			op = ast.OpAdd
		case lexer.MINUS:
			op = ast.OpSub
		default:
			return left
		}
		opTok := p.advance()
		right := p.parseMultiplicative()
		left = ast.NewBinaryExpr(toASTPos(opTok.Pos), op, left, right)
	}
}

func (p *Parser) parseMultiplicative() ast.Expression {
	left := p.parseUnary()
	for {
		var op ast.BinOp
		switch p.peek().Type {
		case lexer.STAR:
			op = ast.OpMul
		case lexer.SLASH:
			op = ast.OpDiv
		case lexer.SLASH_SLASH:
			op = ast.OpFloorDiv
		case lexer.PERCENT:
			op = ast.OpMod
		default:
			return left
		}
		opTok := p.advance()
		right := p.parseUnary()
		left = ast.NewBinaryExpr(toASTPos(opTok.Pos), op, left, right)
	}
}

// parseUnary — унарный минус (право-ассоциативно). Знак НЕ сворачивается в литерал.
func (p *Parser) parseUnary() ast.Expression {
	if p.check(lexer.MINUS) {
		opTok := p.advance()
		operand := p.parseUnary()
		return ast.NewUnaryExpr(toASTPos(opTok.Pos), ast.OpNeg, operand)
	}
	return p.parsePostfix()
}

// parsePostfix навешивает лево-ассоциативную цепочку вызов/индекс/поле на primary.
func (p *Parser) parsePostfix() ast.Expression {
	expr := p.parsePrimary()
	for {
		switch p.peek().Type {
		case lexer.LPAREN:
			p.advance()
			args := p.parseArgList(lexer.RPAREN)
			p.expect(lexer.RPAREN, ")")
			expr = ast.NewCallExpr(expr, args)
		case lexer.LBRACKET:
			p.advance()
			index := p.parseExpression()
			p.expect(lexer.RBRACKET, "]")
			expr = ast.NewIndexExpr(expr, index)
		case lexer.DOT:
			p.advance()
			nameTok, _ := p.expect(lexer.IDENT, "имя поля")
			expr = ast.NewFieldExpr(expr, *p.identFrom(nameTok))
		default:
			return expr
		}
	}
}

// parsePrimary разбирает листовые конструкции: литералы, идентификатор,
// группировку (сворачивается, D5), литерал списка. RunProcessExpr (запустить) —
// добавляется в US4.
func (p *Parser) parsePrimary() ast.Expression {
	t := p.peek()
	switch t.Type {
	case lexer.INT:
		p.advance()
		return p.buildIntLit(t)
	case lexer.FLOAT:
		p.advance()
		return p.buildFloatLit(t)
	case lexer.STRING:
		p.advance()
		return p.buildStringLit(t)
	case lexer.BOOL:
		p.advance()
		return p.buildBoolLit(t)
	case lexer.NONE:
		p.advance()
		return ast.NewNoneLit(toASTPos(t.Pos))
	case lexer.DURATION:
		p.advance()
		return p.buildDurationLit(t)
	case lexer.IDENT:
		p.advance()
		return p.identFrom(t)
	case lexer.LPAREN:
		p.advance()
		expr := p.parseExpression()
		p.expect(lexer.RPAREN, ")")
		return expr // GroupExpr сворачивается (D5)
	case lexer.LBRACKET:
		lbracket := p.advance()
		return p.parseList(lbracket)
	case lexer.KW_RUN:
		return p.parseRunProcess()
	default:
		// Ведущий токен не начинает выражение → SE-UNEXPECTED; узел-заглушка,
		// чтобы вернуть валидное дерево. advance даёт прогресс (до US5 panic-mode).
		p.error(t.Pos, msgUnexpected(t))
		p.advance()
		return ast.NewNoneLit(toASTPos(t.Pos))
	}
}

// parseRunProcess: запустить процесс Ident ("(" ArgList? ")")? — RunProcessExpr.
// Скобки — часть узла (не постфикс-вызов, grammar §9). Pos() = токен запустить.
func (p *Parser) parseRunProcess() ast.Expression {
	runTok := p.advance() // запустить
	p.expect(lexer.KW_PROCESS, "процесс")
	nameTok, _ := p.expect(lexer.IDENT, "имя процесса")
	process := p.identFrom(nameTok)
	var args []ast.Expression
	if p.check(lexer.LPAREN) {
		p.advance()
		args = p.parseArgList(lexer.RPAREN)
		p.expect(lexer.RPAREN, ")")
	}
	return ast.NewRunProcessExpr(toASTPos(runTok.Pos), *process, args)
}

// parseArgList разбирает позиционные аргументы до closer (висящая запятая).
func (p *Parser) parseArgList(closer lexer.TokenType) []ast.Expression {
	var args []ast.Expression
	for !p.check(closer) && !p.check(lexer.EOF) {
		args = append(args, p.parseExpression())
		if !p.match(lexer.COMMA) {
			break
		}
	}
	return args
}

// compOpOf отображает вид токена на оператор сравнения BinOp (CompOp-подмножество).
func compOpOf(t lexer.TokenType) (ast.BinOp, bool) {
	switch t {
	case lexer.EQ:
		return ast.OpEq, true
	case lexer.NEQ:
		return ast.OpNeq, true
	case lexer.LT:
		return ast.OpLt, true
	case lexer.LE:
		return ast.OpLe, true
	case lexer.GT:
		return ast.OpGt, true
	case lexer.GE:
		return ast.OpGe, true
	default:
		return 0, false
	}
}
