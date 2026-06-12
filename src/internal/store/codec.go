package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// Кодек значений Ladix ↔ type-tagged JSON (EM-4 + D-5/D-6/D-21). Внутреннее дело
// SQLiteStore: интерфейс Store принимает/отдаёт map[string]value.Value, MemoryStore
// JSON не трогает. Round-trip честный для всех 10 типов.
//
// Форма тега: {"т":"<TypeName>","зн":<полезная нагрузка>}.
//   Целое       → число
//   Дробное     → число; NaN/+Inf/-Inf — строки "NaN"/"+Inf"/"-Inf" (D-5)
//   Строка      → строка
//   Булево      → true/false
//   Пусто       → null
//   Длительность→ {"значение":N,"единица":"…"}
//   Период      → строка (имя)
//   Дата        → строка "YYYY-MM-DD"
//   Список      → массив (рекурсивно)
//   Запись      → массив пар [["ключ",<значение>],…] в порядке Keys() (D-6)
//
// Верхний уровень Variables — JSON-объект с ключами по возрастанию (D-21).

// taggedTypeName — теги типов в кодеке. Дословно value.TypeName() — единый
// источник имени типа.
const (
	tagЦелое        = "Целое"
	tagДробное      = "Дробное"
	tagСтрока       = "Строка"
	tagБулево       = "Булево"
	tagПусто        = "Пусто"
	tagДлительность = "Длительность"
	tagПериод       = "Период"
	tagДата         = "Дата"
	tagСписок       = "Список"
	tagЗапись       = "Запись"
)

// encodeVariables кодирует карту переменных процесса в type-tagged JSON-объект
// с ключами верхнего уровня по возрастанию (D-21).
func encodeVariables(vars map[string]value.Value) (string, error) {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return "", err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := encodeValue(vars[k])
		if err != nil {
			return "", err
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.String(), nil
}

// encodeValue кодирует одно значение в байты type-tagged JSON.
func encodeValue(v value.Value) ([]byte, error) {
	switch x := v.(type) {
	case value.Целое:
		return tagged(tagЦелое, x.V)
	case value.Дробное:
		return encodeDrobnoe(x.V)
	case value.Строка:
		return tagged(tagСтрока, x.V)
	case value.Булево:
		return tagged(tagБулево, x.V)
	case value.Пусто:
		return tagged(tagПусто, nil)
	case value.Длительность:
		return tagged(tagДлительность, durationPayload{Значение: x.Amount, Единица: x.Unit})
	case value.Период:
		return tagged(tagПериод, x.Name)
	case value.Дата:
		return tagged(tagДата, value.String(x))
	case value.Список:
		return encodeList(x)
	case value.Запись:
		return encodeRecord(x)
	default:
		return nil, fmt.Errorf("кодек: неизвестный тип значения %T", v)
	}
}

// tagged собирает {"т":<tag>,"зн":<payload>} с фиксированным порядком полей.
func tagged(tag string, payload any) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(`{"т":`)
	tb, err := json.Marshal(tag)
	if err != nil {
		return nil, err
	}
	buf.Write(tb)
	buf.WriteString(`,"зн":`)
	pb, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	buf.Write(pb)
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// durationPayload — нагрузка Длительности (D-5/§EN-2: значение+единица).
type durationPayload struct {
	Значение int64  `json:"значение"`
	Единица  string `json:"единица"`
}

// encodeDrobnoe кодирует Дробное; NaN/±Inf — строками (D-5).
func encodeDrobnoe(f float64) ([]byte, error) {
	switch {
	case math.IsNaN(f):
		return tagged(tagДробное, "NaN")
	case math.IsInf(f, +1):
		return tagged(tagДробное, "+Inf")
	case math.IsInf(f, -1):
		return tagged(tagДробное, "-Inf")
	default:
		return tagged(tagДробное, f)
	}
}

func encodeList(x value.Список) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(`{"т":"` + tagСписок + `","зн":[`)
	for i, e := range *x.Elems {
		if i > 0 {
			buf.WriteByte(',')
		}
		eb, err := encodeValue(e)
		if err != nil {
			return nil, err
		}
		buf.Write(eb)
	}
	buf.WriteString(`]}`)
	return buf.Bytes(), nil
}

func encodeRecord(x value.Запись) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(`{"т":"` + tagЗапись + `","зн":[`)
	for i, k := range x.Keys() {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteByte('[')
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(',')
		vb, err := encodeValue(x.Get(k))
		if err != nil {
			return nil, err
		}
		buf.Write(vb)
		buf.WriteByte(']')
	}
	buf.WriteString(`]}`)
	return buf.Bytes(), nil
}

// decodeVariables разбирает type-tagged JSON-объект обратно в карту переменных.
func decodeVariables(s string) (map[string]value.Value, error) {
	if s == "" {
		return map[string]value.Value{}, nil
	}
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader([]byte(s)))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("кодек: разбор переменных: %w", err)
	}
	out := make(map[string]value.Value, len(raw))
	for k, rm := range raw {
		v, err := decodeValue(rm)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

// taggedRaw — общий каркас декодирования тега; «зн» остаётся сырым.
type taggedRaw struct {
	T  string          `json:"т"`
	Зн json.RawMessage `json:"зн"`
}

func decodeValue(rm json.RawMessage) (value.Value, error) {
	var tr taggedRaw
	if err := json.Unmarshal(rm, &tr); err != nil {
		return nil, fmt.Errorf("кодек: разбор тега: %w", err)
	}
	switch tr.T {
	case tagЦелое:
		var n int64
		if err := unmarshalNumber(tr.Зн, &n); err != nil {
			return nil, fmt.Errorf("кодек: Целое: %w", err)
		}
		return value.Целое{V: n}, nil
	case tagДробное:
		return decodeDrobnoe(tr.Зн)
	case tagСтрока:
		var sv string
		if err := json.Unmarshal(tr.Зн, &sv); err != nil {
			return nil, fmt.Errorf("кодек: Строка: %w", err)
		}
		return value.Строка{V: sv}, nil
	case tagБулево:
		var b bool
		if err := json.Unmarshal(tr.Зн, &b); err != nil {
			return nil, fmt.Errorf("кодек: Булево: %w", err)
		}
		return value.Булево{V: b}, nil
	case tagПусто:
		return value.None, nil
	case tagДлительность:
		var dp durationPayload
		if err := json.Unmarshal(tr.Зн, &dp); err != nil {
			return nil, fmt.Errorf("кодек: Длительность: %w", err)
		}
		return value.Длительность{Amount: dp.Значение, Unit: dp.Единица}, nil
	case tagПериод:
		var name string
		if err := json.Unmarshal(tr.Зн, &name); err != nil {
			return nil, fmt.Errorf("кодек: Период: %w", err)
		}
		return value.Период{Name: name}, nil
	case tagДата:
		return decodeDate(tr.Зн)
	case tagСписок:
		return decodeList(tr.Зн)
	case tagЗапись:
		return decodeRecord(tr.Зн)
	default:
		return nil, fmt.Errorf("кодек: неизвестный тег типа %q", tr.T)
	}
}

func decodeDrobnoe(rm json.RawMessage) (value.Value, error) {
	// Спецзначения — строки (D-5).
	var sv string
	if err := json.Unmarshal(rm, &sv); err == nil {
		switch sv {
		case "NaN":
			return value.Дробное{V: math.NaN()}, nil
		case "+Inf":
			return value.Дробное{V: math.Inf(+1)}, nil
		case "-Inf":
			return value.Дробное{V: math.Inf(-1)}, nil
		default:
			return nil, fmt.Errorf("кодек: Дробное: неизвестное спецзначение %q", sv)
		}
	}
	var f float64
	if err := json.Unmarshal(rm, &f); err != nil {
		return nil, fmt.Errorf("кодек: Дробное: %w", err)
	}
	return value.Дробное{V: f}, nil
}

func decodeDate(rm json.RawMessage) (value.Value, error) {
	var s string
	if err := json.Unmarshal(rm, &s); err != nil {
		return nil, fmt.Errorf("кодек: Дата: %w", err)
	}
	var y, m, d int
	if _, err := fmt.Sscanf(s, "%04d-%02d-%02d", &y, &m, &d); err != nil {
		return nil, fmt.Errorf("кодек: Дата %q: %w", s, err)
	}
	return value.Дата{Year: y, Month: m, Day: d}, nil
}

func decodeList(rm json.RawMessage) (value.Value, error) {
	var rawElems []json.RawMessage
	if err := json.Unmarshal(rm, &rawElems); err != nil {
		return nil, fmt.Errorf("кодек: Список: %w", err)
	}
	elems := make([]value.Value, len(rawElems))
	for i, re := range rawElems {
		v, err := decodeValue(re)
		if err != nil {
			return nil, err
		}
		elems[i] = v
	}
	return value.NewList(elems), nil
}

func decodeRecord(rm json.RawMessage) (value.Value, error) {
	var pairs [][]json.RawMessage
	if err := json.Unmarshal(rm, &pairs); err != nil {
		return nil, fmt.Errorf("кодек: Запись: %w", err)
	}
	keys := make([]string, 0, len(pairs))
	fields := make(map[string]value.Value, len(pairs))
	for _, p := range pairs {
		if len(p) != 2 {
			return nil, fmt.Errorf("кодек: Запись: пара ожидает 2 элемента, получено %d", len(p))
		}
		var k string
		if err := json.Unmarshal(p[0], &k); err != nil {
			return nil, fmt.Errorf("кодек: Запись: ключ: %w", err)
		}
		v, err := decodeValue(p[1])
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
		fields[k] = v
	}
	return value.NewRecord(keys, fields), nil
}

// unmarshalNumber декодирует json.Number-совместимое число в int64 без потери
// точности на больших значениях (json по умолчанию читает в float64).
func unmarshalNumber(rm json.RawMessage, out *int64) error {
	dec := json.NewDecoder(bytes.NewReader(rm))
	dec.UseNumber()
	var num json.Number
	if err := dec.Decode(&num); err != nil {
		return err
	}
	n, err := num.Int64()
	if err != nil {
		return err
	}
	*out = n
	return nil
}
