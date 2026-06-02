package ast

import "testing"

// T033 (часть): FunctionDecl — поля и Pos() = токен функция; реализует Decl/TopLevelItem.

func TestFunctionDeclPos(t *testing.T) {
	fnPos := Position{Line: 1, Col: 1}
	body := NewBlock(Position{Line: 2, Col: 5}, []Statement{
		NewReturnStmt(Position{Line: 2, Col: 5}, NewIntLit(Position{Line: 2, Col: 12}, 0)),
	})
	fd := NewFunctionDecl(fnPos, *NewIdent(Position{Line: 1, Col: 9}, "f"),
		[]Ident{*NewIdent(Position{Line: 1, Col: 11}, "n")}, body)

	if fd.Pos() != fnPos {
		t.Errorf("FunctionDecl.Pos() = %+v, хотим токен функция %+v", fd.Pos(), fnPos)
	}
	if fd.Name.Name != "f" || len(fd.Params) != 1 || fd.Params[0].Name != "n" {
		t.Errorf("поля FunctionDecl: %+v", fd)
	}
	var _ Decl = fd
	var _ TopLevelItem = fd
}
