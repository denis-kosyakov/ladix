package value

import (
	"math"
	"strconv"
	"strings"
)

// String — единое строковое представление значения (§7, FR-030, Guardrail 13).
// Используется и встроенной строка(x), и печать. Чистая функция, без побочных
// эффектов; рекурсивна для Список/Запись.
//
//	Целое   → десятичная запись
//	Дробное → formatFloat (кратчайшая обратимая запись с принудительной .0)
//	Строка  → сам текст БЕЗ кавычек (и внутри Список/Запись — тоже)
//	Булево  → истина / ложь
//	Пусто   → пусто
//	Список  → [элементы через ", " рекурсивно]
//	Запись  → {поле: значение через ", " в порядке полей}
func String(v Value) string {
	switch x := v.(type) {
	case Целое:
		return strconv.FormatInt(x.V, 10)
	case Дробное:
		return formatFloat(x.V)
	case Строка:
		return x.V
	case Булево:
		if x.V {
			return "истина"
		}
		return "ложь"
	case Пусто:
		return "пусто"
	case Список:
		parts := make([]string, len(*x.Elems))
		for i, e := range *x.Elems {
			parts[i] = String(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case Запись:
		parts := make([]string, 0, len(x.keys))
		for _, k := range x.keys {
			parts = append(parts, k+": "+String(x.fields[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case Длительность:
		return strconv.FormatInt(x.Amount, 10) + x.Unit
	case Период:
		return x.Name
	case Дата:
		return isoDate(x)
	default:
		return "пусто"
	}
}

// formatFloat реализует политику §7/R1: кратчайшая обратимая запись с
// ПРИНУДИТЕЛЬНОЙ дробной частью .0, если результат «выглядит целым» (не содержит
// ни одного из '.', 'e', 'E', 'n', 'i'). Так Дробное всегда визуально отличимо
// от Целое (2175 → 2175.0), что требуют golden-примеры.
//
// R1 (бизнес-диапазон): для конечных |f| ∈ [1e-4, 1e16) используется фиксированная
// запись 'f' (без экспоненты), иначе 'g' сократил бы 1000000.0 до "1e+06". Для
// очень больших/малых значений и не-конечных — оставляем 'g' (1e20 → "1e+20").
func formatFloat(f float64) string {
	verb := byte('g')
	if abs := math.Abs(f); abs >= 1e-4 && abs < 1e16 {
		verb = 'f'
	}
	s := strconv.FormatFloat(f, verb, -1, 64)
	if !strings.ContainsAny(s, ".eEni") {
		s += ".0"
	}
	return s
}

// isoDate печатает Дата как YYYY-MM-DD (deferred-тип; в 003 не достигается).
func isoDate(d Дата) string {
	return pad4(d.Year) + "-" + pad2(d.Month) + "-" + pad2(d.Day)
}

func pad4(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 4 {
		s = "0" + s
	}
	return s
}

func pad2(n int) string {
	s := strconv.Itoa(n)
	if len(s) < 2 {
		s = "0" + s
	}
	return s
}
