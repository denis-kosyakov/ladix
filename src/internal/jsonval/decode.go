// Package jsonval — нейтральный кодек «JSON ↔ value.Value» (B2, §AU-5.2).
//
// Создан фичей 014 (B2): декодер ЛИФТНУТ из daemon (раньше payloadToRecord и
// помощники жили в daemon/events.go), плюс НОВЫЙ нетегированный энкодер
// value→plain-JSON для тела HTTP-вебхука. jsonval — ЕДИНСТВЕННЫЙ источник этого
// декодера: daemon ДЕЛЕГИРУЕТ сюда (без дубля), engine.webhookCaller декодирует
// ответ вебхука через DecodeValue, B3 (015) потребит PayloadToRecord для
// --данные→Запись. Импортирует только value+stdlib (листовой пакет: НЕ тянет
// eval/engine/store/ast/parser/lexer — иначе engine→daemon циклы; D-AU-2).
//
// ВНИМАНИЕ: jsonval НЕ сливается со вторым декодером источников M1
// (eval/source_loader.go, §SM-9.B): у источников строгая семантика overflow
// (int64-overflow → ОШИБКА), а здесь payload толерантен (overflow → Дробное,
// деградация лучше сбоя доставки). Разная семантика — разные декодеры.
package jsonval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// PayloadToRecord конвертирует сырой JSON-payload в value.Запись с сохранением
// порядка ключей (маппинг §9: null→Пусто, bool→Булево, строка→Строка, число без
// '.'/'e'/'E' → Целое, иначе Дробное, массив→Список, объект→Запись). Пустой
// payload → пустая Запись (не ошибка). Верхний уровень обязан быть JSON-объектом;
// иначе — ошибка (поля доступны через запись.поле). Лифтнут из daemon (007b).
func PayloadToRecord(payload string) (value.Запись, error) {
	if strings.TrimSpace(payload) == "" {
		return value.NewRecord(nil, map[string]value.Value{}), nil
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(payload)))
	dec.UseNumber()

	tok, err := dec.Token()
	if err != nil {
		return value.NewRecord(nil, map[string]value.Value{}), err
	}
	d, ok := tok.(json.Delim)
	if !ok || d != '{' {
		return value.NewRecord(nil, map[string]value.Value{}),
			fmt.Errorf("payload не является JSON-объектом")
	}
	rec, err := decodeObject(dec)
	if err != nil {
		return value.NewRecord(nil, map[string]value.Value{}), err
	}
	return rec, nil
}

// DecodeValue читает одно JSON-значение из потокового декодера (§9.3). Экспортирован
// для engine.webhookCaller: ответ вебхука может быть скаляром, массивом ИЛИ объектом
// (PayloadToRecord потребовал бы верхний объект — для ответа это слишком строго).
// Вызывающий обязан сам подать декодер с UseNumber() (см. NewDecoder ниже) ИЛИ
// использовать DecodeReader.
func DecodeValue(dec *json.Decoder) (value.Value, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			return decodeObject(dec)
		case '[':
			return decodeArray(dec)
		default:
			return nil, fmt.Errorf("неожиданный токен '%c'", rune(t))
		}
	case nil:
		return value.None, nil // null
	case bool:
		return value.Булево{V: t}, nil
	case string:
		return value.Строка{V: t}, nil
	case json.Number:
		return numberToValue(t), nil
	default:
		return nil, fmt.Errorf("неподдерживаемое значение")
	}
}

// decodeObject читает тело JSON-объекта (открывающая «{» уже прочитана) → value.Запись,
// сохраняя порядок ключей (дубликат ключа — побеждает последний, как §9.2).
func decodeObject(dec *json.Decoder) (value.Запись, error) {
	keys := []string{}
	fields := map[string]value.Value{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return value.Запись{}, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return value.Запись{}, fmt.Errorf("ожидался ключ объекта")
		}
		v, err := DecodeValue(dec)
		if err != nil {
			return value.Запись{}, err
		}
		if _, exists := fields[key]; !exists {
			keys = append(keys, key)
		}
		fields[key] = v
	}
	if _, err := dec.Token(); err != nil { // закрывающая «}»
		return value.Запись{}, err
	}
	return value.NewRecord(keys, fields), nil
}

// decodeArray читает тело JSON-массива (открывающая «[» уже прочитана) → value.Список.
func decodeArray(dec *json.Decoder) (value.Value, error) {
	elems := []value.Value{}
	for dec.More() {
		ev, err := DecodeValue(dec)
		if err != nil {
			return nil, err
		}
		elems = append(elems, ev)
	}
	if _, err := dec.Token(); err != nil { // закрывающая «]»
		return nil, err
	}
	return value.NewList(elems), nil
}

// numberToValue различает Целое/Дробное по форме токена JSON (§9.3): наличие
// '.'/'e'/'E' → Дробное; иначе Целое. Целое вне int64 деградирует в Дробное
// (payload толерантен: лучше приблизительное число, чем сбой доставки).
func numberToValue(n json.Number) value.Value {
	s := string(n)
	if !strings.ContainsAny(s, ".eE") {
		if v, err := n.Int64(); err == nil {
			return value.Целое{V: v}
		}
	}
	if f, err := n.Float64(); err == nil {
		return value.Дробное{V: f}
	}
	return value.None
}

// NewDecoder строит потоковый JSON-декодер с UseNumber() (различение Целое/Дробное
// по форме токена). Удобный конструктор для вызывающих DecodeValue над io.Reader
// (engine.webhookCaller: тело ответа вебхука).
func NewDecoder(r *bytes.Reader) *json.Decoder {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	return dec
}
