package parser

import (
	"strconv"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/lexer"
)

// buildIntLit собирает IntLit из токена INT, проверяя диапазон int64 (D2).
// Лексема нормализована лексером (цифры без '_'), поэтому единственная реальная
// ошибка — выход за диапазон → накапливаемая SE-INT-RANGE. Узел создаётся всегда
// (Value=0 при ошибке), чтобы разбор продолжился.
func (p *Parser) buildIntLit(t lexer.Token) *ast.IntLit {
	v, err := strconv.ParseInt(t.Lexeme, 10, 64)
	if err != nil {
		p.error(t.Pos, msgIntRange(t.Lexeme))
		return ast.NewIntLit(toASTPos(t.Pos), 0)
	}
	return ast.NewIntLit(toASTPos(t.Pos), v)
}

// buildFloatLit берёт предразобранное float64 из Token.Value.
func (p *Parser) buildFloatLit(t lexer.Token) *ast.FloatLit {
	v, _ := t.Value.(float64)
	return ast.NewFloatLit(toASTPos(t.Pos), v)
}

// buildStringLit берёт развёрнутую строку из Token.Value.
func (p *Parser) buildStringLit(t lexer.Token) *ast.StringLit {
	v, _ := t.Value.(string)
	return ast.NewStringLit(toASTPos(t.Pos), v)
}

// buildBoolLit берёт bool из Token.Value.
func (p *Parser) buildBoolLit(t lexer.Token) *ast.BoolLit {
	v, _ := t.Value.(bool)
	return ast.NewBoolLit(toASTPos(t.Pos), v)
}

// buildDurationLit берёт величину и единицу из Token.Value (диапазон не
// проверяется, D-R3).
func (p *Parser) buildDurationLit(t lexer.Token) *ast.DurationLit {
	dv, _ := t.Value.(lexer.DurationValue)
	return ast.NewDurationLit(toASTPos(t.Pos), dv.Amount, dv.Unit)
}

// identFrom строит Ident из токена IDENT (или ведущего токена, использованного
// как имя при восстановлении).
func (p *Parser) identFrom(t lexer.Token) *ast.Ident {
	return ast.NewIdent(toASTPos(t.Pos), t.Lexeme)
}

// parseList разбирает литерал списка: lbracket уже потреблён. Допускает висящую
// запятую, гетерогенность и пустой []. Pos() = токен [.
func (p *Parser) parseList(lbracket lexer.Token) *ast.ListLit {
	var elems []ast.Expression
	for !p.check(lexer.RBRACKET) && !p.check(lexer.EOF) {
		elems = append(elems, p.parseExpression())
		if !p.match(lexer.COMMA) {
			break
		}
	}
	p.expect(lexer.RBRACKET, "]")
	return ast.NewListLit(toASTPos(lbracket.Pos), elems)
}
