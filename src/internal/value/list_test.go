package value

import "testing"

// Список ссылочен: NewList над тем же срезом делит хранилище; мутация *Elems
// видна через алиас (контракт b := a, FR-003, C-6).
func TestListAliasingSharesStorage(t *testing.T) {
	a := NewList([]Value{Целое{V: 1}, Целое{V: 2}})
	b := a // присваивание делит *Elems

	*b.Elems = append(*b.Elems, Целое{V: 3})

	if len(*a.Elems) != 3 {
		t.Fatalf("после добавления через алиас len(a) = %d, хотим 3", len(*a.Elems))
	}
	if !Equal((*a.Elems)[2], Целое{V: 3}) {
		t.Errorf("a[2] = %v, хотим Целое 3", (*a.Elems)[2])
	}
}

// NewList(nil) даёт пустой список с собственным непустым *Elems.
func TestNewListNil(t *testing.T) {
	l := NewList(nil)
	if l.Elems == nil {
		t.Fatalf("Elems == nil")
	}
	if len(*l.Elems) != 0 {
		t.Errorf("len = %d, хотим 0", len(*l.Elems))
	}
}
