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

// T028 (часть US3): If/ElseClause/While/For/Return/Break/Continue.

func TestIfStmtAndElseClause(t *testing.T) {
	ifPos := Position{Line: 1, Col: 1}
	cond := NewBoolLit(Position{Line: 1, Col: 6}, true)
	then := NewBlock(Position{Line: 2, Col: 5}, []Statement{NewBreakStmt(Position{Line: 2, Col: 5})})

	finalElse := NewElseBlock(Position{Line: 5, Col: 1}, then)
	if !finalElse.IsFinal() {
		t.Errorf("NewElseBlock должен быть финальным иначе")
	}
	elseIf := NewElseIf(Position{Line: 3, Col: 1}, cond, then, finalElse)
	if elseIf.IsFinal() {
		t.Errorf("NewElseIf не должен быть финальным")
	}
	if elseIf.Else != finalElse {
		t.Errorf("цепочка ElseClause разорвана")
	}

	is := NewIfStmt(ifPos, cond, then, elseIf)
	if is.Pos() != ifPos {
		t.Errorf("IfStmt.Pos() = %+v, хотим токен если %+v", is.Pos(), ifPos)
	}
	var _ Statement = is
}

func TestWhileForReturnBreakContinue(t *testing.T) {
	wp := Position{Line: 1, Col: 1}
	body := NewBlock(Position{Line: 2, Col: 5}, []Statement{NewContinueStmt(Position{Line: 2, Col: 5})})
	ws := NewWhileStmt(wp, NewBoolLit(Position{Line: 1, Col: 6}, true), body)
	if ws.Pos() != wp {
		t.Errorf("WhileStmt.Pos() = %+v, хотим токен пока", ws.Pos())
	}

	fp := Position{Line: 3, Col: 1}
	fs := NewForStmt(fp, *NewIdent(Position{Line: 3, Col: 5}, "i"), NewIdent(Position{Line: 3, Col: 9}, "r"), body)
	if fs.Pos() != fp || fs.Var.Name != "i" {
		t.Errorf("ForStmt неверен: pos=%+v var=%q", fs.Pos(), fs.Var.Name)
	}

	rp := Position{Line: 4, Col: 1}
	bare := NewReturnStmt(rp, nil)
	if bare.Pos() != rp || bare.Value != nil {
		t.Errorf("голый ReturnStmt: pos=%+v value=%v", bare.Pos(), bare.Value)
	}
	withVal := NewReturnStmt(rp, NewIntLit(Position{Line: 4, Col: 9}, 0))
	if withVal.Value == nil {
		t.Errorf("ReturnStmt со значением: Value == nil")
	}

	if NewBreakStmt(rp).Pos() != rp {
		t.Errorf("BreakStmt.Pos()")
	}
	if NewContinueStmt(rp).Pos() != rp {
		t.Errorf("ContinueStmt.Pos()")
	}

	var _ Statement = ws
	var _ Statement = fs
	var _ Statement = bare
}
