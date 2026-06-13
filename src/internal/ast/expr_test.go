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

// T033 (часть US4): RunProcessExpr — Pos() = токен запустить; реализует Expression.

func TestRunProcessExprPos(t *testing.T) {
	runPos := Position{Line: 1, Col: 1}
	rpe := NewRunProcessExpr(runPos, *NewIdent(Position{Line: 1, Col: 18}, "Отчёт"),
		[]Expression{NewIntLit(Position{Line: 1, Col: 24}, 2024)})
	if rpe.Pos() != runPos {
		t.Errorf("RunProcessExpr.Pos() = %+v, хотим токен запустить %+v", rpe.Pos(), runPos)
	}
	if rpe.Process.Name != "Отчёт" || len(rpe.Args) != 1 {
		t.Errorf("поля RunProcessExpr: %+v", rpe)
	}
	var _ Expression = rpe
}

// T005 (007a §TR-2): ValueExpr/EventExpr — беспараметрические первичные
// выражения (зеркало NoneLit); Pos() = токен значение/событие; реализуют Expression.

var (
	_ Expression = (*ValueExpr)(nil)
	_ Expression = (*EventExpr)(nil)
)

func TestValueExprPos(t *testing.T) {
	valPos := Position{Line: 1, Col: 15}
	ve := NewValueExpr(valPos)
	if ve.Pos() != valPos {
		t.Errorf("ValueExpr.Pos() = %+v, хотим токен значение %+v", ve.Pos(), valPos)
	}
	var _ Expression = ve
}

func TestEventExprPos(t *testing.T) {
	evtPos := Position{Line: 1, Col: 16}
	ee := NewEventExpr(evtPos)
	if ee.Pos() != evtPos {
		t.Errorf("EventExpr.Pos() = %+v, хотим токен событие %+v", ee.Pos(), evtPos)
	}
	var _ Expression = ee
}
