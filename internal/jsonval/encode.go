package jsonval

import (
	"bytes"
	"encoding/json"
	"strconv"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// EncodeBody строит тело HTTP-вебхука (§AU-4.3): plain-JSON-объект
// {"цель":<target>,"данные":[<args>]}. НЕтегированный (БЕЗ обёртки {"т","зн"}
// store/codec): получатель вебхука — внешняя система, ждёт обычный JSON. Порядок
// ключей фиксирован ("цель" перед "данные") + порядок аргументов сохранён →
// детерминированное тело (golden/httptest-замки).
func EncodeBody(target string, args []value.Value) []byte {
	var b bytes.Buffer
	b.WriteString(`{"цель":`)
	b.Write(encodeString(target))
	b.WriteString(`,"данные":[`)
	for i, a := range args {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(EncodeValue(a))
	}
	b.WriteString(`]}`)
	return b.Bytes()
}

// EncodeValue кодирует одно value.Value в plain-JSON (нетегированный, §AU-4.3):
//
//	Целое        → число (десятичное)
//	Дробное      → число (кратчайшая обратимая запись)
//	Строка       → JSON-строка в кавычках
//	Булево       → true/false
//	Пусто        → null
//	Список       → array (поэлементно)
//	Запись       → object (ключи в ПОРЯДКЕ появления — НЕ сортируются, в отличие от
//	               encoding/json по map; строим объект вручную ради детерминизма)
//	Дата         → строковая форма "YYYY-MM-DD"  (решение impl, см. ниже)
//	Длительность → строковая форма "3дн"
//	Период       → строковая форма ("последние 7дн" / "прошлый месяц" / адверб)
//
// РАЗВИЛКА §AU-4.3 (Дата/Длительность/Период): нет канонического JSON-представления
// доменных типов, поэтому B2 выбирает СТРОКОВУЮ форму value.String(v) — ту же, что
// видит пользователь в печать/строка(x). Альтернатива (структурный объект {год,…})
// отвергнута: усложняет контракт вебхука без потребителя, а строковая форма
// человекочитаема и обратима внешним парсером по известному формату. Выбор
// задокументирован здесь как требует tasks T005.
func EncodeValue(v value.Value) []byte {
	switch x := v.(type) {
	case value.Целое:
		return []byte(strconv.FormatInt(x.V, 10))
	case value.Дробное:
		// Кратчайшая обратимая запись; +Inf/-Inf/NaN недостижимы в обычном payload,
		// но Marshal на них ошибётся — оставляем числовую форму как json.
		bts, err := json.Marshal(x.V)
		if err != nil {
			return []byte("null")
		}
		return bts
	case value.Строка:
		return encodeString(x.V)
	case value.Булево:
		if x.V {
			return []byte("true")
		}
		return []byte("false")
	case value.Пусто:
		return []byte("null")
	case value.Список:
		var b bytes.Buffer
		b.WriteByte('[')
		for i, e := range *x.Elems {
			if i > 0 {
				b.WriteByte(',')
			}
			b.Write(EncodeValue(e))
		}
		b.WriteByte(']')
		return b.Bytes()
	case value.Запись:
		var b bytes.Buffer
		b.WriteByte('{')
		for i, k := range x.Keys() {
			if i > 0 {
				b.WriteByte(',')
			}
			b.Write(encodeString(k))
			b.WriteByte(':')
			b.Write(EncodeValue(x.Get(k)))
		}
		b.WriteByte('}')
		return b.Bytes()
	default:
		// Дата/Длительность/Период (и защитный fallback): строковая форма §AU-4.3.
		return encodeString(value.String(v))
	}
}

// encodeString кодирует строку как JSON-строку (экранирование/кавычки) через
// encoding/json — единый источник правил экранирования.
func encodeString(s string) []byte {
	bts, err := json.Marshal(s)
	if err != nil {
		return []byte(`""`)
	}
	return bts
}
