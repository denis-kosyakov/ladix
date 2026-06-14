package daemon

import (
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// enqueue кладёт событие в очередь Store с заданным именем/payload/штампом FIFO.
func enqueue(t *testing.T, st store.Store, name, payload string, createdAt time.Time) {
	t.Helper()
	id, err := st.NextEventID()
	if err != nil {
		t.Fatalf("NextEventID: %v", err)
	}
	if err := st.EnqueueEvent(&store.Event{ID: id, Name: name, PayloadJSON: payload, CreatedAt: createdAt}); err != nil {
		t.Fatalf("EnqueueEvent: %v", err)
	}
}

// eventSrc — программа с одним событие-триггером «когда событие <name>:» и телом
// печать(событие.<field>) (доступ к полю payload через событие.поле, §TR-5).
func eventSrc(name, field string) string {
	return "когда событие " + name + ":\n    печать(событие." + field + ")\n"
}

// failOnceMarkStore оборачивает Store и проваливает ПЕРВЫЙ MarkEventProcessed
// (имитация краша/сбоя коммита РОВНО на точке пометки, ПОСЛЕ исполнения тела): событие
// остаётся необработанным → следующий drain переисполняет тело (at-least-once, FR-017).
type failOnceMarkStore struct {
	store.Store
	failed bool
}

func (s *failOnceMarkStore) MarkEventProcessed(id string) error {
	if !s.failed {
		s.failed = true
		return errMarkFailed
	}
	return s.Store.MarkEventProcessed(id)
}

var errMarkFailed = &markErr{}

type markErr struct{}

func (*markErr) Error() string { return "имитация сбоя пометки" }

// TestDrainEventsFieldInjection — payload JSON→Запись: поле доступно как событие.поле
// в теле; событие помечено processed ПОСЛЕ исполнения тела.
func TestDrainEventsFieldInjection(t *testing.T) {
	out := &countWriter{marker: "ООО"}
	st := store.NewMemoryStore()
	d, _ := buildDaemon(t, eventSrc("заявка_создана", "клиент"), st, out)

	enqueue(t, st, "заявка_создана", `{"клиент":"ООО"}`, time.Unix(100, 0))
	d.tick()

	if got := out.count(); got != 1 {
		t.Fatalf("инжект событие.клиент: вхождений «ООО» = %d, хотим 1", got)
	}
	// processed ПОСЛЕ тела: очередь пуста.
	rest, _ := st.ListUnprocessedEvents()
	if len(rest) != 0 {
		t.Fatalf("после drain необработанных = %d, хотим 0", len(rest))
	}
}

// TestDrainEventsFIFO — три события одного имени обрабатываются в FIFO-порядке
// (по CreatedAt): тело печатает поле «n», порядок вывода = порядок enqueue (SC-006).
func TestDrainEventsFIFO(t *testing.T) {
	out := &countWriter{marker: "n"}
	st := store.NewMemoryStore()
	d, _ := buildDaemon(t, eventSrc("обновление", "n"), st, out)

	enqueue(t, st, "обновление", `{"n":"A"}`, time.Unix(100, 0))
	enqueue(t, st, "обновление", `{"n":"B"}`, time.Unix(200, 0))
	enqueue(t, st, "обновление", `{"n":"C"}`, time.Unix(300, 0))
	d.tick()

	// Тело печатает «A»/«B»/«C» с переводом строки; порядок строго A→B→C.
	want := "A\nB\nC\n"
	if got := out.String(); got != want {
		t.Fatalf("FIFO порядок: out=%q, хотим %q", got, want)
	}
}

// TestDrainEventsAtLeastOnce — пометка ПОСЛЕ тела: сбой пометки (имитация краша на
// точке mark, после исполнения тела) → событие остаётся в очереди → следующий drain
// переисполняет тело (at-least-once, FR-017). Тело исполнено ДВАЖДЫ.
func TestDrainEventsAtLeastOnce(t *testing.T) {
	out := &countWriter{marker: "ООО"}
	st := &failOnceMarkStore{Store: store.NewMemoryStore()}
	d, _ := buildDaemon(t, eventSrc("заявка_создана", "клиент"), st, out)

	enqueue(t, st, "заявка_создана", `{"клиент":"ООО"}`, time.Unix(100, 0))

	// drain №1: тело исполнено, но пометка провалена → событие НЕ processed.
	d.tick()
	if got := out.count(); got != 1 {
		t.Fatalf("drain1: тело исполнено %d раз, хотим 1", got)
	}
	rest, _ := st.ListUnprocessedEvents()
	if len(rest) != 1 {
		t.Fatalf("drain1: пометка провалена, необработанных = %d, хотим 1 (at-least-once)", len(rest))
	}

	// drain №2: пометка теперь успешна → событие обработано, тело исполнено повторно.
	d.tick()
	if got := out.count(); got != 2 {
		t.Fatalf("drain2: всего исполнений тела = %d, хотим 2 (переобработка)", got)
	}
	rest, _ = st.ListUnprocessedEvents()
	if len(rest) != 0 {
		t.Fatalf("drain2: необработанных = %d, хотим 0", len(rest))
	}
}

// TestDrainEventsNoMatchProcessed — событие без подходящего триггера: помечается
// processed без исполнения тела + лог «без триггеров» (FR-017).
func TestDrainEventsNoMatchProcessed(t *testing.T) {
	out := &countWriter{marker: "X"}
	st := store.NewMemoryStore()
	// Триггер на ДРУГОЕ имя — событие «иное» не матчится.
	d, _ := buildDaemon(t, eventSrc("заявка_создана", "клиент"), st, out)

	enqueue(t, st, "иное", `{"a":1}`, time.Unix(100, 0))
	d.tick()

	if !out.contains("событие 'иное' без триггеров") {
		t.Fatalf("ожидали лог «без триггеров», out=%q", out.String())
	}
	rest, _ := st.ListUnprocessedEvents()
	if len(rest) != 0 {
		t.Fatalf("событие без триггеров должно быть processed: необработанных = %d, хотим 0", len(rest))
	}
}

// TestDrainEventsEmptyQueueNoOp — пустая очередь → no-op (без логов/паники).
func TestDrainEventsEmptyQueueNoOp(t *testing.T) {
	out := &countWriter{marker: "X"}
	st := store.NewMemoryStore()
	d, _ := buildDaemon(t, eventSrc("заявка_создана", "клиент"), st, out)
	d.tick() // очередь пуста
	if s := out.String(); s != "" {
		t.Fatalf("пустая очередь: ожидали no-op, out=%q", s)
	}
}

// TestDrainEventsEmptyPayloadTolerant — пустой payload → пустая Запись; событие.поле
// → Пусто (печатается «пусто»), тело исполняется без сбоя, событие помечено.
func TestDrainEventsEmptyPayloadTolerant(t *testing.T) {
	out := &countWriter{marker: "пусто"}
	st := store.NewMemoryStore()
	d, _ := buildDaemon(t, eventSrc("заявка_создана", "клиент"), st, out)

	enqueue(t, st, "заявка_создана", ``, time.Unix(100, 0))
	d.tick()

	if got := out.count(); got != 1 {
		t.Fatalf("пустой payload: событие.клиент → «пусто», вхождений = %d, хотим 1", got)
	}
	rest, _ := st.ListUnprocessedEvents()
	if len(rest) != 0 {
		t.Fatalf("после drain необработанных = %d, хотим 0", len(rest))
	}
}

// TestPayloadToRecordValueTypes — декод JSON-payload в value-типы (§9.3): различение
// Целое/Дробное по форме токена, деградация int64-overflow → Дробное (numberToValue),
// построение Списка из JSON-массива (decodeArray), доступ к полю/None у открытой Записи.
func TestPayloadToRecordValueTypes(t *testing.T) {
	rec, err := payloadToRecord(`{"n":3,"f":1.5,"xs":[1,2],"big":99999999999999999999}`)
	if err != nil {
		t.Fatalf("payloadToRecord: %v", err)
	}

	// Целое: число без '.'/'e' в пределах int64 → value.Целое.
	if got, ok := rec.Get("n").(value.Целое); !ok || got.V != 3 {
		t.Errorf("поле n = %#v, хотим value.Целое{V:3}", rec.Get("n"))
	}
	// Дробное: число с '.' → value.Дробное (различение по форме токена, не по величине).
	if got, ok := rec.Get("f").(value.Дробное); !ok || got.V != 1.5 {
		t.Errorf("поле f = %#v, хотим value.Дробное{V:1.5}", rec.Get("f"))
	}
	// int64-overflow деградирует в Дробное (payload толерантен: приближение > сбой доставки).
	if _, ok := rec.Get("big").(value.Дробное); !ok {
		t.Errorf("поле big (вне int64) = %#v, хотим value.Дробное (деградация)", rec.Get("big"))
	}
	// Список: JSON-массив → value.Список с поэлементным декодом.
	xs, ok := rec.Get("xs").(value.Список)
	if !ok {
		t.Fatalf("поле xs = %#v, хотим value.Список", rec.Get("xs"))
	}
	elems := *xs.Elems
	if len(elems) != 2 {
		t.Fatalf("список xs: длина = %d, хотим 2", len(elems))
	}
	if e0, ok := elems[0].(value.Целое); !ok || e0.V != 1 {
		t.Errorf("xs[0] = %#v, хотим value.Целое{V:1}", elems[0])
	}
	if e1, ok := elems[1].(value.Целое); !ok || e1.V != 2 {
		t.Errorf("xs[1] = %#v, хотим value.Целое{V:2}", elems[1])
	}
	// Открытая Запись: отсутствующее поле → None (value.Пусто), без ошибки.
	if _, ok := rec.Get("нет_такого").(value.Пусто); !ok {
		t.Errorf("отсутствующее поле = %#v, хотим value.Пусто (None)", rec.Get("нет_такого"))
	}
}
