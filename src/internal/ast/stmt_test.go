package ast

import "testing"

// T024 (подмножество US2): конструкторы LetStmt/AssignStmt/ExpressionStmt и Pos()
// (ведущий токен / lvalue / Expr.Pos()) — data-model §4.

func TestLetStmtPos(t *testing.T) {
	letPos := Position{Line: 1, Col: 1}
	name := *NewIdent(Position{Line: 1, Col: 7}, "a")
	value := NewIntLit(Position{Line: 1, Col: 11}, 5)
	ls := NewLetStmt(letPos, name, value)
	if ls.Pos() != letPos {
		t.Errorf("LetStmt.Pos() = %+v, хотим токен пусть %+v", ls.Pos(), letPos)
	}
	if ls.Name.Name != "a" || ls.Value != Expression(value) {
		t.Errorf("поля LetStmt: %+v", ls)
	}
	var _ Statement = ls
	var _ TopLevelItem = ls
}

func TestAssignStmtPosIsLvalue(t *testing.T) {
	namePos := Position{Line: 2, Col: 1}
	name := *NewIdent(namePos, "x")
	as := NewAssignStmt(name, NewIntLit(Position{Line: 2, Col: 5}, 1))
	if as.Pos() != namePos {
		t.Errorf("AssignStmt.Pos() = %+v, хотим позицию lvalue %+v", as.Pos(), namePos)
	}
	var _ Statement = as
}

func TestExpressionStmtPosIsExpr(t *testing.T) {
	exprPos := Position{Line: 3, Col: 1}
	call := NewCallExpr(NewIdent(exprPos, "печать"), nil)
	es := NewExpressionStmt(call)
	if es.Pos() != exprPos {
		t.Errorf("ExpressionStmt.Pos() = %+v, хотим Expr.Pos() %+v", es.Pos(), exprPos)
	}
	var _ Statement = es
}
