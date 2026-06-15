package value

import "strings"

// Equal реализует == / != по §3.3 (FR-012, C-3). Чистая функция.
//
// Разные типы → false, КРОМЕ пары Целое↔Дробное (сравниваются численно: 1==1.0).
// В пределах типа: Строка по код-поинтам, Булево по значению, Пусто равно только
// Пусто, Список поэлементно (разная длина → false), Запись структурно (совпадение
// множества полей и значений, порядок не важен). Длительность — по паре
// (единица, значение) БЕЗ нормализации (D-17: 1час != 60мин).
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
	case Дата:
		y, ok := b.(Дата)
		return ok && x.Year == y.Year && x.Month == y.Month && x.Day == y.Day
	case Период:
		// §MW-D-VALUE-EQ: равенство по ВСЕМ полям (Name + скользящие Amount/Unit +
		// сдвиг Offset завершённого). 5 календарных констант — нулевые новые поля.
		y, ok := b.(Период)
		return ok && x.Name == y.Name && x.Amount == y.Amount && x.Unit == y.Unit && x.Offset == y.Offset
	case Длительность:
		// D-17: равенство по паре (единица, значение) БЕЗ нормализации
		// (1час != 60мин). Разные единицы → false.
		y, ok := b.(Длительность)
		return ok && x.Unit == y.Unit && x.Amount == y.Amount
	}
	return false
}

// Compare реализует порядок < <= > >= по §3.3 (C-4). Возвращает знак сравнения
// (-1, 0, +1) и признак взаимной упорядочиваемости. Допустимы только: Цел↔Цел,
// Дроб↔Дроб, Цел↔Дроб (промоушен), Строка↔Строка (лексикографически по
// код-поинтам), Дата↔Дата, Длительность↔Длительность ОДНОЙ единицы (D-17: по
// значению; разные единицы → ok == false). Прочее → ok == false; вызывающий eval порождает
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
	case Дата:
		if y, ok := b.(Дата); ok {
			return cmpDate(x, y), true
		}
	case Длительность:
		// D-17: порядок только в пределах ОДНОЙ единицы — по значению. Разные
		// единицы → ok == false (вызывающий eval даёт TY-BINOP «'<оп>' нельзя
		// применить к Длительность и Длительность»). Нормализации нет.
		if y, ok := b.(Длительность); ok && x.Unit == y.Unit {
			return cmpInt64(x.Amount, y.Amount), true
		}
	}
	return 0, false
}

// cmpDate сравнивает две даты лексикографически по (Year, Month, Day).
func cmpDate(a, b Дата) int {
	if c := cmpInt(a.Year, b.Year); c != 0 {
		return c
	}
	if c := cmpInt(a.Month, b.Month); c != 0 {
		return c
	}
	return cmpInt(a.Day, b.Day)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
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
