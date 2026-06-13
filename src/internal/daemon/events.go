package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// eventName — предопределённое имя, инжектируемое в тело событие-триггера на момент
// доставки (§TR-5: «событие» = Запись из payload).
const eventName = "событие"

// drainEvents — фаза 1 тика (FR-016/017, EM-17.3, tick-contract.md §фаза1). Разбирает
// очередь необработанных событий в FIFO-порядке (ListUnprocessedEvents по CreatedAt),
// матчит событие-триггеры по имени (EventTrigger.Event.Name == e.Name), исполняет их
// тела с инжектом read-only «событие» (Запись из payload) и помечает событие
// обработанным ПОСЛЕ тела (at-least-once, FR-017): краш между телом и пометкой →
// переобработка на следующем drain (повтор безвреден на уровне at-least-once).
//
// Событие без подходящих триггеров → MarkEventProcessed + лог «без триггеров» (FR-017:
// не копится «мусорная» очередь). Невалидный/пустой payload → пустая Запись (лог),
// тело всё равно исполняется (толерантный доступ событие.поле → None). Каждое событие
// и каждый триггер — под per-триггер recover (изоляция, EM-17.6). Пустая очередь → no-op.
func (d *Daemon) drainEvents() {
	events, err := d.st.ListUnprocessedEvents()
	if err != nil {
		d.logf("сбой чтения очереди событий: %s", err.Error())
		return
	}
	for _, e := range events {
		matched := d.eventTriggers(e.Name)
		if len(matched) == 0 {
			d.markProcessed(e.ID, e.Name)
			d.logf("событие '%s' без триггеров", e.Name)
			continue
		}

		rec, perr := payloadToRecord(e.PayloadJSON)
		if perr != nil {
			// Невалидный payload не роняет доставку: пустая Запись + лог (импл-факт).
			d.logf("событие '%s': некорректный payload, поля пусты: %s", e.Name, perr.Error())
		}

		for _, mt := range matched {
			body := mt.body
			d.safeFire(func() error {
				return d.fireBody(body, injection{name: eventName, val: rec})
			})
		}

		// MarkEventProcessed ПОСЛЕ тела (at-least-once, FR-017).
		d.markProcessed(e.ID, e.Name)
	}
}

// matchedTrigger — событие-триггер, чьё имя совпало с именем события.
type matchedTrigger struct {
	body *ast.Block
}

// eventTriggers возвращает событие-триггеры (в порядке объявления), чьё имя события
// равно name (FR-016). Пусто → событие без обработчиков.
func (d *Daemon) eventTriggers(name string) []matchedTrigger {
	var out []matchedTrigger
	for _, td := range d.interp.Triggers() {
		spec, ok := td.Spec.(*ast.EventTrigger)
		if !ok {
			continue
		}
		if spec.Event.Name == name {
			out = append(out, matchedTrigger{body: td.Body})
		}
	}
	return out
}

// markProcessed помечает событие обработанным, логируя сбой (не роняет тик).
func (d *Daemon) markProcessed(id, name string) {
	if err := d.st.MarkEventProcessed(id); err != nil {
		d.logf("событие '%s': сбой пометки обработанным: %s", name, err.Error())
	}
}

// payloadToRecord конвертирует сырой JSON-payload события в value.Запись с сохранением
// порядка ключей (маппинг источников §9: null→Пусто, bool→Булево, строка→Строка,
// число без '.'/'e'/'E' → Целое, иначе Дробное, массив→Список, объект→Запись).
// Пустой payload → пустая Запись (не ошибка). Верхний уровень обязан быть JSON-объектом;
// иначе — ошибка (поля события доступны через событие.поле, §TR-5).
func payloadToRecord(payload string) (value.Запись, error) {
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
		v, err := decodeValue(dec)
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

// decodeValue читает одно JSON-значение (§9.3) из потокового декодера.
func decodeValue(dec *json.Decoder) (value.Value, error) {
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

// decodeArray читает тело JSON-массива (открывающая «[» уже прочитана) → value.Список.
func decodeArray(dec *json.Decoder) (value.Value, error) {
	elems := []value.Value{}
	for dec.More() {
		ev, err := decodeValue(dec)
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
// (payload толерантен: лучше приблизительное число, чем сбой доставки события).
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
