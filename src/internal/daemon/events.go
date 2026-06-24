package daemon

import (
	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/jsonval"
)

// eventName — предопределённое имя, инжектируемое в тело событие-триггера на момент
// доставки (§TR-5: «событие» = Запись из payload).
const eventName = "событие"

// drainEvents — фаза 1 тика (FR-016/017, EM-17.3, tick-contract.md §фаза1). Разбирает
// очередь необработанных событий в FIFO-порядке (ListUnprocessedEvents по CreatedAt),
// матчит событие-триггеры по имени (EventTrigger.Event.Name == e.Name), исполняет их
// тела с инжектом read-only «событие» (Запись из payload) и помечает событие
// обработанным ПОСЛЕ тела (at-least-once, FR-017): краш между телом и пометкой →
// переобработка на следующем drain. Повтор тела ВОЗМОЖЕН: при неидемпотентной побочке
// («запустить процесс» → второй инстанс p-NNNNNN) эффект может задвоиться. at-least-once
// — осознанный выбор v1 (доставка важнее дедупликации, FR-017), а не «безвредность».
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

		rec, perr := jsonval.PayloadToRecord(e.PayloadJSON)
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

// Декодер payload (payloadToRecord/decodeObject/decodeValue/decodeArray/payloadNumberToValue)
// ЛИФТНУТ в internal/jsonval (B2, §AU-5.2): daemon делегирует туда (jsonval.PayloadToRecord
// в drainEvents), дубля декодера здесь больше нет. jsonval — листовой (value+stdlib),
// engine/cmd тоже потребляют его без цикла engine→daemon. Маппинг §9 неизменен; golden
// событий 007b байт-точен (TestPayloadToRecordValueTypes переехал в jsonval/decode_test.go).
