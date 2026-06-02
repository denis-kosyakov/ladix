package ast

// IntLit — целочисленный литерал; несёт значение в диапазоне int64. Диапазон
// проверяет парсер при сборке (D2); при выходе узел всё равно создаётся (Value=0),
// чтобы разбор продолжился. Pos() = свой токен.
type IntLit struct {
	exprBase
	Value int64
}

// NewIntLit строит целочисленный литерал.
func NewIntLit(pos Position, value int64) *IntLit {
	return &IntLit{exprBase: exprBase{base{pos}}, Value: value}
}

// FloatLit — дробный литерал (предразобранное float64 из Token.Value).
type FloatLit struct {
	exprBase
	Value float64
}

// NewFloatLit строит дробный литерал.
func NewFloatLit(pos Position, value float64) *FloatLit {
	return &FloatLit{exprBase: exprBase{base{pos}}, Value: value}
}

// StringLit — строковый литерал (развёрнутая строка из Token.Value).
type StringLit struct {
	exprBase
	Value string
}

// NewStringLit строит строковый литерал.
func NewStringLit(pos Position, value string) *StringLit {
	return &StringLit{exprBase: exprBase{base{pos}}, Value: value}
}

// BoolLit — булев литерал (истина/ложь).
type BoolLit struct {
	exprBase
	Value bool
}

// NewBoolLit строит булев литерал.
func NewBoolLit(pos Position, value bool) *BoolLit {
	return &BoolLit{exprBase: exprBase{base{pos}}, Value: value}
}

// NoneLit — литерал пусто.
type NoneLit struct {
	exprBase
}

// NewNoneLit строит литерал пусто.
func NewNoneLit(pos Position) *NoneLit {
	return &NoneLit{exprBase: exprBase{base{pos}}}
}

// DurationLit — литерал длительности: нормализованная величина-лексема и единица
// (сек/мин/час/дн/нед/мес). Диапазон НЕ проверяется (D-R3). Pos() = свой токен.
type DurationLit struct {
	exprBase
	Amount string
	Unit   string
}

// NewDurationLit строит литерал длительности.
func NewDurationLit(pos Position, amount, unit string) *DurationLit {
	return &DurationLit{exprBase: exprBase{base{pos}}, Amount: amount, Unit: unit}
}

// ListLit — литерал списка: гетерогенные элементы, висящая запятая, пустой [].
// Pos() = токен [.
type ListLit struct {
	exprBase
	Elements []Expression
}

// NewListLit строит литерал списка; elements может быть nil (пустой список).
func NewListLit(pos Position, elements []Expression) *ListLit {
	return &ListLit{exprBase: exprBase{base{pos}}, Elements: elements}
}

// Ident — идентификатор: имя переменной/функции/поля/встроенной функции (резолв
// в eval). Pos() = свой токен.
type Ident struct {
	exprBase
	Name string
}

// NewIdent строит идентификатор.
func NewIdent(pos Position, name string) *Ident {
	return &Ident{exprBase: exprBase{base{pos}}, Name: name}
}
