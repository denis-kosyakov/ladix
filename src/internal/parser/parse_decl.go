package parser

import (
	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/lexer"
)

// parseFunctionDecl: функция Ident "(" ParamList? ")" ":" Block. Только верхний
// уровень (grammar §4). Pos() = токен функция.
func (p *Parser) parseFunctionDecl() *ast.FunctionDecl {
	fnTok := p.advance() // функция
	nameTok, _ := p.expect(lexer.IDENT, "имя функции")
	name := p.identFrom(nameTok)
	p.expect(lexer.LPAREN, "(")
	params := p.parseParamList()
	p.expect(lexer.RPAREN, ")")
	p.expect(lexer.COLON, ":")
	body := p.parseBlock()
	return ast.NewFunctionDecl(toASTPos(fnTok.Pos), *name, params, body)
}

// parseParamList разбирает позиционные параметры до ")" (висящая запятая).
func (p *Parser) parseParamList() []ast.Ident {
	var params []ast.Ident
	for !p.check(lexer.RPAREN) && !p.check(lexer.EOF) {
		nameTok, ok := p.expect(lexer.IDENT, "имя параметра")
		if !ok {
			break
		}
		params = append(params, *p.identFrom(nameTok))
		if !p.match(lexer.COMMA) {
			break
		}
	}
	return params
}
