package value

import "testing"

// Equal — §3.3/C-3: численное Целое↔Дробное, поэлементные списки, структурные
// записи, разные типы → false.
func TestEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b Value
		want bool
	}{
		{"1 == 1.0 численно", Целое{V: 1}, Дробное{V: 1.0}, true},
		{"1.0 == 1 численно", Дробное{V: 1.0}, Целое{V: 1}, true},
		{"1 == 2", Целое{V: 1}, Целое{V: 2}, false},
		{"строки равны", Строка{V: "а"}, Строка{V: "а"}, true},
		{"строки разные", Строка{V: "а"}, Строка{V: "б"}, false},
		{"булево", Булево{V: true}, Булево{V: true}, true},
		{"пусто == пусто", None, Пусто{}, true},
		{"пусто != целое", None, Целое{V: 0}, false},
		{"строка != целое", Строка{V: "1"}, Целое{V: 1}, false},
		{
			"списки поэлементно равны",
			NewList([]Value{Целое{V: 1}, Целое{V: 2}}),
			NewList([]Value{Целое{V: 1}, Целое{V: 2}}),
			true,
		},
		{
			"списки разной длины",
			NewList([]Value{Целое{V: 1}}),
			NewList([]Value{Целое{V: 1}, Целое{V: 2}}),
			false,
		},
		{
			"список с численным совпадением",
			NewList([]Value{Целое{V: 1}}),
			NewList([]Value{Дробное{V: 1.0}}),
			true,
		},
		{"даты равны", Дата{Year: 2026, Month: 5, Day: 17}, Дата{Year: 2026, Month: 5, Day: 17}, true},
		{"даты разные", Дата{Year: 2026, Month: 5, Day: 3}, Дата{Year: 2026, Month: 5, Day: 17}, false},
		{"дата != целое", Дата{Year: 2026, Month: 5, Day: 3}, Целое{V: 20260503}, false},
		{"периоды равны", Период{Name: "ежемесячно"}, Период{Name: "ежемесячно"}, true},
		{"периоды разные", Период{Name: "ежемесячно"}, Период{Name: "ежегодно"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Equal(tt.a, tt.b); got != tt.want {
				t.Errorf("Equal(%v, %v) = %v, хотим %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// Compare — §3.3/C-4: знак сравнения и взаимная упорядочиваемость. Списки/булево/
// пусто не упорядочиваются (ok == false).
func TestCompare(t *testing.T) {
	tests := []struct {
		name    string
		a, b    Value
		wantCmp int
		wantOK  bool
	}{
		{"1 < 2", Целое{V: 1}, Целое{V: 2}, -1, true},
		{"2 > 1", Целое{V: 2}, Целое{V: 1}, 1, true},
		{"равны", Целое{V: 1}, Целое{V: 1}, 0, true},
		{"промоушен 1 < 1.5", Целое{V: 1}, Дробное{V: 1.5}, -1, true},
		{"дробные", Дробное{V: 2.5}, Дробное{V: 2.5}, 0, true},
		{"строки лексикографически", Строка{V: "а"}, Строка{V: "б"}, -1, true},
		{"даты по возрастанию", Дата{Year: 2026, Month: 5, Day: 3}, Дата{Year: 2026, Month: 5, Day: 17}, -1, true},
		{"даты по убыванию", Дата{Year: 2026, Month: 6, Day: 1}, Дата{Year: 2026, Month: 5, Day: 31}, 1, true},
		{"даты равны", Дата{Year: 2026, Month: 5, Day: 17}, Дата{Year: 2026, Month: 5, Day: 17}, 0, true},
		{"год важнее месяца", Дата{Year: 2025, Month: 12, Day: 31}, Дата{Year: 2026, Month: 1, Day: 1}, -1, true},
		{"список не упорядочивается", NewList(nil), NewList(nil), 0, false},
		{"булево не упорядочивается", Булево{V: true}, Булево{V: false}, 0, false},
		{"разные типы не упорядочиваются", Строка{V: "1"}, Целое{V: 1}, 0, false},
		{"дата↔целое несравнимы", Дата{Year: 2026, Month: 5, Day: 3}, Целое{V: 20260503}, 0, false},
		{"период не упорядочивается", Период{Name: "ежемесячно"}, Период{Name: "ежегодно"}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCmp, gotOK := Compare(tt.a, tt.b)
			if gotOK != tt.wantOK {
				t.Fatalf("Compare ok = %v, хотим %v", gotOK, tt.wantOK)
			}
			if gotOK && gotCmp != tt.wantCmp {
				t.Errorf("Compare = %d, хотим %d", gotCmp, tt.wantCmp)
			}
		})
	}
}
