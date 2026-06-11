package store

import (
	"math"
	"reflect"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// roundTrip кодирует карту переменных и декодирует обратно, возвращая результат.
// Кодек — внутреннее дело SQLiteStore; тестируем его напрямую (white-box).
func roundTrip(t *testing.T, vars map[string]value.Value) map[string]value.Value {
	t.Helper()
	enc, err := encodeVariables(vars)
	if err != nil {
		t.Fatalf("encodeVariables: %v", err)
	}
	dec, err := decodeVariables(enc)
	if err != nil {
		t.Fatalf("decodeVariables(%q): %v", enc, err)
	}
	return dec
}

// valEqual — структурное сравнение значений для round-trip-проверок кодека.
// Не опирается на value.Equal, поскольку в 006-foundational ещё нет case
// Длительность (он приходит в US4/T040) и value.Equal даёт NaN!=NaN. Кодек —
// ниже по слою, его round-trip обязан быть честным независимо от состояния
// value.Equal.
func valEqual(a, b value.Value) bool {
	switch x := a.(type) {
	case value.Дробное:
		y, ok := b.(value.Дробное)
		if !ok {
			return false
		}
		if math.IsNaN(x.V) || math.IsNaN(y.V) {
			return math.IsNaN(x.V) && math.IsNaN(y.V)
		}
		return x.V == y.V
	case value.Длительность:
		y, ok := b.(value.Длительность)
		return ok && x.Amount == y.Amount && x.Unit == y.Unit
	case value.Список:
		y, ok := b.(value.Список)
		if !ok || len(*x.Elems) != len(*y.Elems) {
			return false
		}
		for i := range *x.Elems {
			if !valEqual((*x.Elems)[i], (*y.Elems)[i]) {
				return false
			}
		}
		return true
	case value.Запись:
		y, ok := b.(value.Запись)
		if !ok {
			return false
		}
		xk, yk := x.Keys(), y.Keys()
		if len(xk) != len(yk) {
			return false
		}
		for _, k := range xk {
			if !valEqual(x.Get(k), y.Get(k)) {
				return false
			}
		}
		return true
	default:
		return value.Equal(a, b)
	}
}

func TestCodecRoundTripAllTypes(t *testing.T) {
	cases := []struct {
		name string
		v    value.Value
	}{
		{"Целое", value.Целое{V: 42}},
		{"Целое-отриц", value.Целое{V: -7}},
		{"Дробное", value.Дробное{V: 3.14}},
		{"Дробное-целоподобное", value.Дробное{V: 2175}},
		{"Строка", value.Строка{V: "привет"}},
		{"Строка-пустая", value.Строка{V: ""}},
		{"Булево-истина", value.Булево{V: true}},
		{"Булево-ложь", value.Булево{V: false}},
		{"Пусто", value.None},
		{"Длительность", value.Длительность{Amount: 5, Unit: "мин"}},
		{"Длительность-дн", value.Длительность{Amount: 2, Unit: "дн"}},
		{"Период", value.Период{Name: "ежемесячно"}},
		{"Дата", value.Дата{Year: 2026, Month: 5, Day: 31}},
		{"Список", value.NewList([]value.Value{value.Целое{V: 1}, value.Строка{V: "x"}})},
		{"Список-пустой", value.NewList(nil)},
		{"Запись", value.NewRecord(
			[]string{"номер", "сумма"},
			map[string]value.Value{"номер": value.Целое{V: 7}, "сумма": value.Целое{V: 1000}},
		)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dec := roundTrip(t, map[string]value.Value{"k": c.v})
			got, ok := dec["k"]
			if !ok {
				t.Fatalf("ключ k потерян")
			}
			if !valEqual(c.v, got) {
				t.Errorf("round-trip %s: got %#v, want %#v", c.name, got, c.v)
			}
		})
	}
}

// D-5: NaN/+Inf/-Inf кодируются строками и честно восстанавливаются.
func TestCodecDrobnoeSpecials(t *testing.T) {
	cases := []struct {
		name string
		v    float64
	}{
		{"NaN", math.NaN()},
		{"+Inf", math.Inf(+1)},
		{"-Inf", math.Inf(-1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dec := roundTrip(t, map[string]value.Value{"k": value.Дробное{V: c.v}})
			got, ok := dec["k"].(value.Дробное)
			if !ok {
				t.Fatalf("got %#v, не Дробное", dec["k"])
			}
			switch c.name {
			case "NaN":
				if !math.IsNaN(got.V) {
					t.Errorf("got %v, want NaN", got.V)
				}
			case "+Inf":
				if !math.IsInf(got.V, +1) {
					t.Errorf("got %v, want +Inf", got.V)
				}
			case "-Inf":
				if !math.IsInf(got.V, -1) {
					t.Errorf("got %v, want -Inf", got.V)
				}
			}
		})
	}
}

// D-5 проверка байтов: спецзначения именно строки в JSON.
func TestCodecDrobnoeSpecialBytes(t *testing.T) {
	cases := map[string]string{
		`{"k":{"т":"Дробное","зн":"NaN"}}`:  "NaN",
		`{"k":{"т":"Дробное","зн":"+Inf"}}`: "+Inf",
		`{"k":{"т":"Дробное","зн":"-Inf"}}`: "-Inf",
	}
	for wantJSON := range cases {
		var v float64
		switch cases[wantJSON] {
		case "NaN":
			v = math.NaN()
		case "+Inf":
			v = math.Inf(+1)
		case "-Inf":
			v = math.Inf(-1)
		}
		enc, err := encodeVariables(map[string]value.Value{"k": value.Дробное{V: v}})
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		if enc != wantJSON {
			t.Errorf("encode %s: got %q, want %q", cases[wantJSON], enc, wantJSON)
		}
	}
}

// D-6: Запись кодируется массивом пар в порядке Keys(); порядок сохраняется.
func TestCodecRecordPairsOrder(t *testing.T) {
	rec := value.NewRecord(
		[]string{"я", "б", "а"}, // намеренно НЕ отсортированный порядок появления
		map[string]value.Value{
			"я": value.Целое{V: 1},
			"б": value.Строка{V: "two"},
			"а": value.Булево{V: true},
		},
	)
	enc, err := encodeVariables(map[string]value.Value{"r": rec})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	want := `{"r":{"т":"Запись","зн":[["я",{"т":"Целое","зн":1}],["б",{"т":"Строка","зн":"two"}],["а",{"т":"Булево","зн":true}]]}}`
	if enc != want {
		t.Errorf("encode Записи (порядок пар):\n got %q\nwant %q", enc, want)
	}

	dec := roundTrip(t, map[string]value.Value{"r": rec})
	gotRec, ok := dec["r"].(value.Запись)
	if !ok {
		t.Fatalf("got %#v, не Запись", dec["r"])
	}
	if !reflect.DeepEqual(gotRec.Keys(), []string{"я", "б", "а"}) {
		t.Errorf("порядок Keys() не сохранён: got %v", gotRec.Keys())
	}
	if !value.Equal(rec, gotRec) {
		t.Errorf("Запись round-trip: got %#v", gotRec)
	}
}

// D-21: ключи верхнего уровня Variables пишутся по возрастанию.
func TestCodecTopLevelKeysAscending(t *testing.T) {
	enc, err := encodeVariables(map[string]value.Value{
		"я":         value.Целое{V: 1},
		"абвг":      value.Целое{V: 2},
		"сотрудник": value.Строка{V: "Петров"},
		"имя":       value.Строка{V: "Иван"},
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	want := `{"абвг":{"т":"Целое","зн":2},"имя":{"т":"Строка","зн":"Иван"},"сотрудник":{"т":"Строка","зн":"Петров"},"я":{"т":"Целое","зн":1}}`
	if enc != want {
		t.Errorf("верхний уровень не по возрастанию:\n got %q\nwant %q", enc, want)
	}
}

// Вложенность Список/Запись (рекурсия кодека).
func TestCodecNested(t *testing.T) {
	inner := value.NewRecord(
		[]string{"q"},
		map[string]value.Value{"q": value.NewList([]value.Value{value.Целое{V: 9}, value.Пусто{}})},
	)
	lst := value.NewList([]value.Value{
		value.Длительность{Amount: 3, Unit: "час"},
		inner,
		value.NewList([]value.Value{value.Строка{V: "вложенный"}}),
	})
	dec := roundTrip(t, map[string]value.Value{"top": lst})
	if !valEqual(lst, dec["top"]) {
		t.Errorf("вложенный round-trip: got %#v", dec["top"])
	}
}

// Пустая карта Variables → честный round-trip пустой карты.
func TestCodecEmptyVariables(t *testing.T) {
	dec := roundTrip(t, map[string]value.Value{})
	if len(dec) != 0 {
		t.Errorf("пустая карта round-trip: got %#v", dec)
	}
}
