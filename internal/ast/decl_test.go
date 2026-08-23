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

// T004 (010-A1, §SC-2/§SC-D-1, конституция VI): ТЕСТ-ЗАМОК аддитивного расширения
// SourceDecl полями Type/TypePos/Fields/FieldsPos и нового листового узла FieldDef.
// Presence атрибута — Pos.Line != 0 (как MetricAttrPos); v1-форма (только файл:)
// даёт нулевые Type/Fields.
func TestSourceDeclTypeAndFields(t *testing.T) {
	srcPos := Position{Line: 1, Col: 1}
	filePos := Position{Line: 2, Col: 11}
	typePos := Position{Line: 3, Col: 5}
	fieldsPos := Position{Line: 4, Col: 5}

	sd := NewSourceDecl(srcPos, *NewIdent(Position{Line: 1, Col: 10}, "заказы"),
		*NewStringLit(filePos, "data/orders.csv"), filePos)
	// §SC-D-3: Type/Fields заполняются сеттер-присваиванием парсером.
	sd.Type = *NewIdent(typePos, "csv")
	sd.TypePos = typePos
	sd.Fields = []FieldDef{
		{Name: *NewIdent(Position{Line: 5, Col: 9}, "сумма"),
			TypeName: *NewIdent(Position{Line: 5, Col: 16}, "Дробное"),
			Pos:      Position{Line: 5, Col: 9}},
		{Name: *NewIdent(Position{Line: 6, Col: 9}, "статус"),
			TypeName: *NewIdent(Position{Line: 6, Col: 17}, "Строка"),
			Pos:      Position{Line: 6, Col: 9}},
	}
	sd.FieldsPos = fieldsPos

	if sd.Type.Name != "csv" {
		t.Errorf("Type.Name = %q, хотим \"csv\"", sd.Type.Name)
	}
	if sd.TypePos != typePos || sd.TypePos.Line == 0 {
		t.Errorf("TypePos = %+v, хотим %+v (presence Line!=0)", sd.TypePos, typePos)
	}
	if len(sd.Fields) != 2 {
		t.Fatalf("Fields = %d, хотим 2", len(sd.Fields))
	}
	if sd.Fields[0].Name.Name != "сумма" || sd.Fields[0].TypeName.Name != "Дробное" {
		t.Errorf("Fields[0] = %+v, хотим {сумма, Дробное}", sd.Fields[0])
	}
	if sd.Fields[1].Pos != (Position{Line: 6, Col: 9}) {
		t.Errorf("Fields[1].Pos = %+v, хотим {6,9}", sd.Fields[1].Pos)
	}
	if sd.FieldsPos.Line == 0 {
		t.Errorf("FieldsPos.Line = 0, хотим presence (!= 0)")
	}
}

// T004 (§SC-D-1): v1-форма источника (без тип:/поля:) даёт нулевые Type/Fields —
// presence отличает заданный атрибут от нулевого.
func TestSourceDeclV1ZeroTypeFields(t *testing.T) {
	srcPos := Position{Line: 1, Col: 1}
	filePos := Position{Line: 2, Col: 11}
	sd := NewSourceDecl(srcPos, *NewIdent(Position{Line: 1, Col: 10}, "продажи"),
		*NewStringLit(filePos, "data/sales.json"), filePos)

	if sd.Type.Name != "" {
		t.Errorf("Type.Name = %q, хотим пусто (v1 → json)", sd.Type.Name)
	}
	if sd.TypePos.Line != 0 {
		t.Errorf("TypePos.Line = %d, хотим 0 (тип: отсутствует)", sd.TypePos.Line)
	}
	if sd.Fields != nil {
		t.Errorf("Fields = %v, хотим nil (схемы нет)", sd.Fields)
	}
	if sd.FieldsPos.Line != 0 {
		t.Errorf("FieldsPos.Line = %d, хотим 0 (поля: отсутствует)", sd.FieldsPos.Line)
	}
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
