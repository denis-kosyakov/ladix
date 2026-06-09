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

// T007: SourceDecl — поля и Pos() = токен источник; реализует Decl/TopLevelItem.

func TestSourceDeclPos(t *testing.T) {
	srcPos := Position{Line: 1, Col: 1}
	filePos := Position{Line: 2, Col: 11}
	sd := NewSourceDecl(srcPos, *NewIdent(Position{Line: 1, Col: 10}, "продажи"),
		*NewStringLit(filePos, "data/sales.json"), filePos)

	if sd.Pos() != srcPos {
		t.Errorf("SourceDecl.Pos() = %+v, хотим токен источник %+v", sd.Pos(), srcPos)
	}
	if sd.Name.Name != "продажи" || sd.File.Value != "data/sales.json" || sd.FilePos != filePos {
		t.Errorf("поля SourceDecl: %+v", sd)
	}
	var _ Decl = sd
	var _ TopLevelItem = sd
}

// T007: MetricDecl — поля, Attrs и Pos() = токен метрика; реализует Decl/TopLevelItem.

func TestMetricDeclPos(t *testing.T) {
	mPos := Position{Line: 4, Col: 1}
	srcAttrPos := Position{Line: 5, Col: 5}
	aggPos := Position{Line: 6, Col: 5}
	aggregate := NewCallExpr(NewIdent(aggPos, "сумма"),
		[]Expression{NewIdent(Position{Line: 6, Col: 11}, "сумма_заказа")})
	attrs := MetricAttrPos{SourcePos: srcAttrPos, AggregatePos: aggPos}
	md := NewMetricDecl(mPos, *NewIdent(Position{Line: 4, Col: 9}, "выручка"),
		*NewIdent(Position{Line: 5, Col: 15}, "продажи"),
		nil, aggregate, nil, nil, attrs)

	if md.Pos() != mPos {
		t.Errorf("MetricDecl.Pos() = %+v, хотим токен метрика %+v", md.Pos(), mPos)
	}
	if md.Name.Name != "выручка" || md.Source.Name != "продажи" {
		t.Errorf("поля MetricDecl: %+v", md)
	}
	if md.Where != nil || md.Period != nil || md.ByDate != nil {
		t.Errorf("опциональные атрибуты должны быть nil: %+v", md)
	}
	if md.Aggregate == nil {
		t.Errorf("Aggregate обязателен (не-nil): %+v", md)
	}
	if md.Attrs.SourcePos != srcAttrPos || md.Attrs.AggregatePos != aggPos {
		t.Errorf("Attrs MetricDecl: %+v", md.Attrs)
	}
	var _ Decl = md
	var _ TopLevelItem = md
}
