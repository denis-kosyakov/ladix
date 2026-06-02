package ast

// FunctionDecl — объявление функции: функция Name(Params): Body. Только верхний
// уровень; вложенные функции запрещены (SE-NESTED-FN, синтаксис grammar §4).
// Параметры позиционные. Pos() = токен функция.
type FunctionDecl struct {
	declBase
	Name   Ident
	Params []Ident
	Body   *Block
}

// NewFunctionDecl строит объявление функции; pos — позиция токена функция.
func NewFunctionDecl(pos Position, name Ident, params []Ident, body *Block) *FunctionDecl {
	return &FunctionDecl{declBase: declBase{base{pos}}, Name: name, Params: params, Body: body}
}
