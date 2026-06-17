package jsonval

import (
	"testing"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// TestEncodeBodyPlainJSON — тело вебхука байт-в-байт (§AU-4.3): нетегированный
// объект {"цель":…,"данные":[…]}, фиксированный порядок ключей.
func TestEncodeBodyPlainJSON(t *testing.T) {
	got := string(EncodeBody("crm", []value.Value{value.Строка{V: "клиент"}}))
	want := `{"цель":"crm","данные":["клиент"]}`
	if got != want {
		t.Errorf("EncodeBody = %q, хотим %q", got, want)
	}
}

// TestEncodeBodyEmptyArgs — без аргументов → пустой массив данных.
func TestEncodeBodyEmptyArgs(t *testing.T) {
	got := string(EncodeBody("пинг", nil))
	want := `{"цель":"пинг","данные":[]}`
	if got != want {
		t.Errorf("EncodeBody(пусто) = %q, хотим %q", got, want)
	}
}

// TestEncodeValueTypes — таблица типов value → plain-JSON БЕЗ обёртки {"т","зн"}.
func TestEncodeValueTypes(t *testing.T) {
	cases := []struct {
		name string
		in   value.Value
		want string
	}{
		{"Целое", value.Целое{V: 42}, `42`},
		{"Дробное", value.Дробное{V: 1.5}, `1.5`},
		{"Строка", value.Строка{V: "клиент"}, `"клиент"`},
		{"Строка-экран", value.Строка{V: `с "кавычкой"`}, `"с \"кавычкой\""`},
		{"Булево-истина", value.Булево{V: true}, `true`},
		{"Булево-ложь", value.Булево{V: false}, `false`},
		{"Пусто", value.None, `null`},
		{"Список", value.NewList([]value.Value{value.Целое{V: 1}, value.Строка{V: "a"}}), `[1,"a"]`},
		{"Длительность", value.Длительность{Amount: 3, Unit: "дн"}, `"3дн"`},
		{"Дата", value.Дата{Year: 2025, Month: 1, Day: 15}, `"2025-01-15"`},
		{"Период-скользящий", value.Период{Name: "последние", Amount: 7, Unit: "дн"}, `"последние 7дн"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(EncodeValue(c.in))
			if got != c.want {
				t.Errorf("EncodeValue(%s) = %q, хотим %q", c.name, got, c.want)
			}
		})
	}
}

// TestEncodeRecordKeyOrder — Запись кодируется с ключами в ПОРЯДКЕ появления (НЕ
// сортировка encoding/json по map): детерминизм тела вебхука.
func TestEncodeRecordKeyOrder(t *testing.T) {
	rec := value.NewRecord(
		[]string{"я", "а", "м"},
		map[string]value.Value{
			"я": value.Целое{V: 1},
			"а": value.Целое{V: 2},
			"м": value.Целое{V: 3},
		},
	)
	got := string(EncodeValue(rec))
	want := `{"я":1,"а":2,"м":3}`
	if got != want {
		t.Errorf("EncodeValue(Запись) = %q, хотим %q (порядок появления, НЕ сортировка)", got, want)
	}
}

// TestEncodeBodyRecordArg — аргумент-Запись внутри данных тоже нетегированный объект.
func TestEncodeBodyRecordArg(t *testing.T) {
	rec := value.NewRecord(
		[]string{"имя", "сумма"},
		map[string]value.Value{
			"имя":   value.Строка{V: "Пётр"},
			"сумма": value.Целое{V: 100},
		},
	)
	got := string(EncodeBody("платёж", []value.Value{rec}))
	want := `{"цель":"платёж","данные":[{"имя":"Пётр","сумма":100}]}`
	if got != want {
		t.Errorf("EncodeBody(Запись) = %q, хотим %q", got, want)
	}
}
