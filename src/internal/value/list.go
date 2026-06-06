package value

// Список — ссылочная последовательность значений. Несёт *[]Value: алиас делит
// хранилище (пусть b = a → добавить(a, x) видно в b), а единственный мутатор
// добавить меняет *Elems на месте (§2.1, FR-003, C-6).
type Список struct {
	Elems *[]Value
}

func (Список) TypeName() string { return "Список" }

// NewList строит Список поверх переданного среза (ссылочно: тот же backing
// array). elems == nil → пустой список с собственным хранилищем.
func NewList(elems []Value) Список {
	if elems == nil {
		elems = []Value{}
	}
	return Список{Elems: &elems}
}
