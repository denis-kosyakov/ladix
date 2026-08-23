package ast

import "testing"

// T033 (часть): зарезервированные StepAction — поля, Pos() = ведущий токен,
// реализуют Statement.

func TestStepActions(t *testing.T) {
	pos := Position{Line: 1, Col: 5}

	aa := NewAssignAction(pos, *NewIdent(Position{Line: 1, Col: 14}, "x"), NewIntLit(Position{Line: 1, Col: 18}, 5))
	if aa.Pos() != pos || aa.Name.Name != "x" {
		t.Errorf("AssignAction: %+v", aa)
	}

	ca := NewCallAction(pos, *NewIdent(Position{Line: 1, Col: 12}, "f"), []Expression{NewIntLit(Position{Line: 1, Col: 14}, 1)})
	if ca.Pos() != pos || len(ca.Args) != 1 {
		t.Errorf("CallAction: %+v", ca)
	}

	na := NewNotifyAction(pos, *NewIdent(Position{Line: 1, Col: 15}, "g"), nil)
	if na.Pos() != pos {
		t.Errorf("NotifyAction.Pos() = %+v", na.Pos())
	}

	var _ Statement = aa
	var _ Statement = ca
	var _ Statement = na
}
