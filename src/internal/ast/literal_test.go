package ast

import "testing"

// T017: конструкторы литералов и Ident; Pos() = свой токен; гетерогенный и
// пустой ListLit (data-model §8).

func TestLiteralConstructorsAndPos(t *testing.T) {
	pos := Position{Line: 1, Col: 1}

	il := NewIntLit(pos, 42)
	if il.Pos() != pos || il.Value != 42 {
		t.Errorf("IntLit: %+v", il)
	}
	fl := NewFloatLit(pos, 3.14)
	if fl.Pos() != pos || fl.Value != 3.14 {
		t.Errorf("FloatLit: %+v", fl)
	}
	sl := NewStringLit(pos, "привет")
	if sl.Pos() != pos || sl.Value != "привет" {
		t.Errorf("StringLit: %+v", sl)
	}
	bl := NewBoolLit(pos, true)
	if bl.Pos() != pos || bl.Value != true {
		t.Errorf("BoolLit: %+v", bl)
	}
	nl := NewNoneLit(pos)
	if nl.Pos() != pos {
		t.Errorf("NoneLit.Pos() = %+v", nl.Pos())
	}
	dl := NewDurationLit(pos, "3", "дн")
	if dl.Pos() != pos || dl.Amount != "3" || dl.Unit != "дн" {
		t.Errorf("DurationLit: %+v", dl)
	}
	id := NewIdent(pos, "x")
	if id.Pos() != pos || id.Name != "x" {
		t.Errorf("Ident: %+v", id)
	}

	// все реализуют Expression
	var _ Expression = il
	var _ Expression = fl
	var _ Expression = sl
	var _ Expression = bl
	var _ Expression = nl
	var _ Expression = dl
	var _ Expression = id
}

func TestListLitHeterogeneousAndEmpty(t *testing.T) {
	pos := Position{Line: 1, Col: 1}
	hetero := NewListLit(pos, []Expression{
		NewIntLit(pos, 1),
		NewStringLit(pos, "две"),
		NewBoolLit(pos, true),
	})
	if hetero.Pos() != pos {
		t.Errorf("ListLit.Pos() = %+v, хотим токен [", hetero.Pos())
	}
	if len(hetero.Elements) != 3 {
		t.Fatalf("ListLit гетерогенный: %d элементов, хотим 3", len(hetero.Elements))
	}
	if _, ok := hetero.Elements[0].(*IntLit); !ok {
		t.Errorf("элемент[0] не IntLit: %T", hetero.Elements[0])
	}
	if _, ok := hetero.Elements[1].(*StringLit); !ok {
		t.Errorf("элемент[1] не StringLit: %T", hetero.Elements[1])
	}

	empty := NewListLit(pos, nil)
	if len(empty.Elements) != 0 {
		t.Errorf("пустой ListLit: %d элементов, хотим 0", len(empty.Elements))
	}
}
