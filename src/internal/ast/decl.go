package ast

// FunctionDecl — объявление функции: функция Name(Params): Body. Только верхний
// уровень; вложенные функции запрещены (SE-NESTED-FN, синтаксис grammar §4).
// Параметры позиционные. Pos() = токен функция.
type FunctionDecl struct {
	declBase
	Name   Ident
	Params []Ident
	Body   *Block
}

// NewFunctionDecl строит объявление функции; pos — позиция токена функция.
func NewFunctionDecl(pos Position, name Ident, params []Ident, body *Block) *FunctionDecl {
	return &FunctionDecl{declBase: declBase{base{pos}}, Name: name, Params: params, Body: body}
}

// SourceDecl — объявление источника: источник Name: файл "<путь>". Только верхний
// уровень (§SM-2/§SM-3). File — строковый литерал пути (не Expression); FilePos —
// позиция самого литерала пути. Pos() = токен источник.
type SourceDecl struct {
	declBase
	Name    Ident
	File    StringLit
	FilePos Position
}

// NewSourceDecl строит объявление источника; pos — позиция токена источник,
// filePos — позиция строкового литерала пути.
func NewSourceDecl(pos Position, name Ident, file StringLit, filePos Position) *SourceDecl {
	return &SourceDecl{declBase: declBase{base{pos}}, Name: name, File: file, FilePos: filePos}
}

// MetricDecl — объявление метрики: метрика Name: блок атрибутов
// {источник/где/агрегат/период/по_дате} (§SM-2). Source — Ident (D-1, не
// Expression). Aggregate обязателен (не-nil после успешного парса); Where/Period/
// ByDate = nil при отсутствии атрибута. Attrs несёт позиции ключевых слов
// присутствующих атрибутов для точной диагностики. Pos() = токен метрика.
type MetricDecl struct {
	declBase
	Name      Ident
	Source    Ident
	Where     Expression
	Aggregate Expression
	Period    Expression
	ByDate    Expression
	Attrs     MetricAttrPos
}

// NewMetricDecl строит объявление метрики; pos — позиция токена метрика.
func NewMetricDecl(pos Position, name Ident, source Ident,
	where Expression, aggregate Expression,
	period Expression, byDate Expression,
	attrs MetricAttrPos) *MetricDecl {
	return &MetricDecl{
		declBase:  declBase{base{pos}},
		Name:      name,
		Source:    source,
		Where:     where,
		Aggregate: aggregate,
		Period:    period,
		ByDate:    byDate,
		Attrs:     attrs,
	}
}

// MetricAttrPos — позиции ключевых слов присутствующих атрибутов метрики (D-2).
// Нулевая Position{} означает отсутствующий атрибут. Вспомогательная структура,
// НЕ Node (без Pos()/маркеров).
type MetricAttrPos struct {
	SourcePos    Position
	WherePos     Position
	AggregatePos Position
	PeriodPos    Position
	ByDatePos    Position
}
