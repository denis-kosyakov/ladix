package value

import "strings"

// Equal реализует == / != по §3.3 (FR-012, C-3). Чистая функция.
//
// Разные типы → false, КРОМЕ пары Целое↔Дробное (сравниваются численно: 1==1.0).
// В пределах типа: Строка по код-поинтам, Булево по значению, Пусто равно только
// Пусто, Список поэлементно (разная длина → false), Запись структурно (совпадение
// множества полей и значений, порядок не важен).
func Equal(a, b Value) bool {
	switch x := a.(type) {
	case Целое:
		switch y := b.(type) {
		case Целое:
			return x.V == y.V
		case Дробное:
			return float64(x.V) == y.V
		}
		return false
	case Дробное:
		switch y := b.(type) {
		case Дробное:
			return x.V == y.V
		case Целое:
			return x.V == float64(y.V)
		}
		return false
	case Строка:
		y, ok := b.(Строка)
		return ok && x.V == y.V
	case Булево:
		y, ok := b.(Булево)
		return ok && x.V == y.V
	case Пусто:
		_, ok := b.(Пусто)
		return ok
	case Список:
		y, ok := b.(Список)
		if !ok {
			return false
		}
		if len(*x.Elems) != len(*y.Elems) {
			return false
		}
		for i := range *x.Elems {
			if !Equal((*x.Elems)[i], (*y.Elems)[i]) {
				return false
			}
		}
		return true
	case Запись:
		y, ok := b.(Запись)
		if !ok {
			return false
		}
		if len(x.fields) != len(y.fields) {
			return false
		}
		for k, xv := range x.fields {
			yv, has := y.fields[k]
			if !has || !Equal(xv, yv) {
				return false
			}
		}
		return true
	}
	return false
}

// Compare реализует порядок < <= > >= по §3.3 (C-4). Возвращает знак сравнения
// (-1, 0, +1) и признак взаимной упорядочиваемости. Допустимы только: Цел↔Цел,
// Дроб↔Дроб, Цел↔Дроб (промоушен), Строка↔Строка (лексикографически по
// код-поинтам). Прочее → ok == false; вызывающий eval порождает
// «'<оп>' нельзя применить к <тип> и <тип>».
func Compare(a, b Value) (int, bool) {
	switch x := a.(type) {
	case Целое:
		switch y := b.(type) {
		case Целое:
			return cmpInt64(x.V, y.V), true
		case Дробное:
			return cmpFloat(float64(x.V), y.V), true
		}
	case Дробное:
		switch y := b.(type) {
		case Дробное:
			return cmpFloat(x.V, y.V), true
		case Целое:
			return cmpFloat(x.V, float64(y.V)), true
		}
	case Строка:
		if y, ok := b.(Строка); ok {
			return strings.Compare(x.V, y.V), true
		}
	}
	return 0, false
}

func cmpInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
