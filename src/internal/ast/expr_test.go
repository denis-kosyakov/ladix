package ast

import "testing"

// T016: конструкторы выражений и конвенция Pos() (D4, contracts/ast.md §C-4).

func TestBinaryExprPosIsOperator(t *testing.T) {
	opPos := Position{Line: 2, Col: 5}
	left := NewIntLit(Position{Line: 2, Col: 3}, 1)
	right := NewIntLit(Position{Line: 2, Col: 7}, 2)
	be := NewBinaryExpr(opPos, OpAdd, left, right)

	if be.Pos() != opPos {
		t.Errorf("BinaryExpr.Pos() = %+v, хотим токен оператора %+v", be.Pos(), opPos)
	}
	if be.Op != OpAdd || be.Left != Expression(left) || be.Right != Expression(right) {
		t.Errorf("поля BinaryExpr заполнены неверно: %+v", be)
	}
	var _ Expression = be // реализует Expression
}

func TestUnaryExprPosIsOperator(t *testing.T) {
	opPos := Position{Line: 1, Col: 1}
	ue := NewUnaryExpr(opPos, OpNeg, NewIntLit(Position{Line: 1, Col: 2}, 5))
	if ue.Pos() != opPos {
		t.Errorf("UnaryExpr.Pos() = %+v, хотим токен оператора %+v", ue.Pos(), opPos)
	}
	if ue.Op != OpNeg {
		t.Errorf("UnaryExpr.Op = %v, хотим OpNeg", ue.Op)
	}
}

func TestCallIndexFieldPos(t *testing.T) {
	calleePos := Position{Line: 3, Col: 1}
	callee := NewIdent(calleePos, "f")
	call := NewCallExpr(callee, []Expression{NewIntLit(Position{Line: 3, Col: 3}, 1)})
	if call.Pos() != calleePos {
		t.Errorf("CallExpr.Pos() = %+v, хотим позицию Callee %+v", call.Pos(), calleePos)
	}
	if len(call.Args) != 1 {
		t.Errorf("CallExpr.Args = %d, хотим 1", len(call.Args))
	}

	targetPos := Position{Line: 4, Col: 2}
	target := NewIdent(targetPos, "данные")
	idx := NewIndexExpr(target, NewIdent(Position{Line: 4, Col: 9}, "i"))
	if idx.Pos() != targetPos {
		t.Errorf("IndexExpr.Pos() = %+v, хотим позицию Target %+v", idx.Pos(), targetPos)
	}
	fld := NewFieldExpr(target, *NewIdent(Position{Line: 4, Col: 12}, "поле"))
	if fld.Pos() != targetPos {
		t.Errorf("FieldExpr.Pos() = %+v, хотим позицию Target %+v", fld.Pos(), targetPos)
	}
	if fld.Field.Name != "поле" {
		t.Errorf("FieldExpr.Field.Name = %q, хотим \"поле\"", fld.Field.Name)
	}
}
