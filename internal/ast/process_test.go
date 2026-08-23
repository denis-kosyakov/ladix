package ast

import "testing"

// T001: ProcessDecl/StepDecl/StepAttrPos — поля, Pos() = ведущий токен,
// участие ProcessDecl в union Decl/TopLevelItem; StepDecl — только Node.

// Компайл-тайм маркеры (§PM-2, data-model §4): ProcessDecl — top-level
// декларация; StepDecl — узел (Pos()), но НЕ Decl/TopLevelItem/Statement
// (отрицательный факт обеспечивается отсутствием маркеров — компилятор не
// даст var _ Decl = (*StepDecl)(nil)).
var _ Decl = (*ProcessDecl)(nil)
var _ TopLevelItem = (*ProcessDecl)(nil)
var _ Node = (*StepDecl)(nil)

func TestProcessDeclPos(t *testing.T) {
	procPos := Position{Line: 1, Col: 1}
	stepPos := Position{Line: 2, Col: 5}
	step := NewStepDecl(stepPos, *NewIdent(Position{Line: 2, Col: 10}, "проверка"),
		[]Ident{*NewIdent(Position{Line: 2, Col: 25}, "старт")},
		nil, nil, StepAttrPos{}, nil)
	pd := NewProcessDecl(procPos, *NewIdent(Position{Line: 1, Col: 9}, "онбординг"),
		[]Ident{
			*NewIdent(Position{Line: 1, Col: 19}, "x"),
			*NewIdent(Position{Line: 1, Col: 22}, "y"),
		},
		[]*StepDecl{step})

	if pd.Pos() != procPos {
		t.Errorf("ProcessDecl.Pos() = %+v, хотим токен процесс %+v", pd.Pos(), procPos)
	}
	if pd.Name.Name != "онбординг" {
		t.Errorf("ProcessDecl.Name: %+v", pd.Name)
	}
	if len(pd.Params) != 2 || pd.Params[0].Name != "x" || pd.Params[1].Name != "y" {
		t.Errorf("ProcessDecl.Params: %+v", pd.Params)
	}
	if len(pd.Steps) != 1 || pd.Steps[0].Name.Name != "проверка" {
		t.Errorf("ProcessDecl.Steps: %+v", pd.Steps)
	}
	if pd.Steps[0].After[0].Name != "старт" {
		t.Errorf("ProcessDecl.Steps[0].After: %+v", pd.Steps[0].After)
	}
	var _ Decl = pd
	var _ TopLevelItem = pd

	// процесс P: без скобок → Params == nil.
	pd2 := NewProcessDecl(procPos, *NewIdent(Position{Line: 1, Col: 9}, "P"),
		nil, []*StepDecl{step})
	if pd2.Params != nil {
		t.Errorf("процесс P: без скобок → Params == nil, получили %+v", pd2.Params)
	}
}

func TestStepDeclPos(t *testing.T) {
	stepPos := Position{Line: 2, Col: 5}
	assigneePos := Position{Line: 3, Col: 9}
	deadlinePos := Position{Line: 4, Col: 9}
	assignee := NewStringLit(Position{Line: 3, Col: 22}, "hr@x.ru")
	deadline := NewDurationLit(Position{Line: 4, Col: 15}, "3", "дней")
	attrs := StepAttrPos{AssigneePos: assigneePos, DeadlinePos: deadlinePos}

	// Атрибуты-только шаг (§PM-7 P2): Assignee/Deadline заполнены, Body nil.
	sd := NewStepDecl(stepPos, *NewIdent(Position{Line: 2, Col: 10}, "оформление"),
		[]Ident{
			*NewIdent(Position{Line: 2, Col: 27}, "старт"),
			*NewIdent(Position{Line: 2, Col: 34}, "анкета"),
		},
		assignee, deadline, attrs, nil)

	if sd.Pos() != stepPos {
		t.Errorf("StepDecl.Pos() = %+v, хотим токен шаг %+v", sd.Pos(), stepPos)
	}
	if sd.Name.Name != "оформление" {
		t.Errorf("StepDecl.Name: %+v", sd.Name)
	}
	if len(sd.After) != 2 || sd.After[0].Name != "старт" || sd.After[1].Name != "анкета" {
		t.Errorf("StepDecl.After: %+v", sd.After)
	}
	if sd.Assignee == nil || sd.Deadline == nil {
		t.Errorf("Assignee/Deadline должны быть заполнены: %+v", sd)
	}
	if sd.Attrs.AssigneePos != assigneePos || sd.Attrs.DeadlinePos != deadlinePos {
		t.Errorf("StepDecl.Attrs: %+v", sd.Attrs)
	}
	// Инвариант пэйринга: атрибут присутствует ⟺ Attrs.*Pos.Line != 0.
	if sd.Attrs.AssigneePos.Line == 0 || sd.Attrs.DeadlinePos.Line == 0 {
		t.Errorf("Attrs.*Pos.Line != 0 при присутствующем атрибуте: %+v", sd.Attrs)
	}
	if sd.Body != nil {
		t.Errorf("атрибуты-только шаг → Body nil: %+v", sd.Body)
	}
	var _ Node = sd

	// Шаг без атрибутов: Assignee/Deadline nil, нулевая StepAttrPos, тело — операторы.
	body := []Statement{
		NewAssignAction(Position{Line: 6, Col: 9},
			*NewIdent(Position{Line: 6, Col: 19}, "статус"),
			NewStringLit(Position{Line: 6, Col: 28}, "готово")),
	}
	sd2 := NewStepDecl(stepPos, *NewIdent(Position{Line: 5, Col: 10}, "старт"),
		nil, nil, nil, StepAttrPos{}, body)
	if sd2.Assignee != nil || sd2.Deadline != nil {
		t.Errorf("Assignee/Deadline без атрибутов должны быть nil: %+v", sd2)
	}
	if sd2.After != nil {
		t.Errorf("After без после должен быть nil: %+v", sd2.After)
	}
	if sd2.Attrs.AssigneePos.Line != 0 || sd2.Attrs.DeadlinePos.Line != 0 {
		t.Errorf("нулевая StepAttrPos ⟺ атрибут отсутствует: %+v", sd2.Attrs)
	}
	if sd2.Attrs != (StepAttrPos{}) {
		t.Errorf("StepAttrPos без атрибутов должна быть нулевой: %+v", sd2.Attrs)
	}
	if len(sd2.Body) != 1 {
		t.Errorf("StepDecl.Body: %+v", sd2.Body)
	}
}
