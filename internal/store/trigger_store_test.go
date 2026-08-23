package store

import (
	"errors"
	"testing"
	"time"
)

// TestTriggerStoreContract — ОБЩИЙ поведенческий контракт 7 триггерных методов
// (007b), прогоняемый на ОБЕИХ реализациях Store (MemoryStore + SQLiteStore).
// Паритет Memory/SQLite фиксируется единой таблицей impl × case — зеркало
// TestStoreContract. Спека требует поведенческой идентичности (FR-021..023).
func TestTriggerStoreContract(t *testing.T) {
	impls := []struct {
		name     string
		newStore func(*testing.T) Store
	}{
		{"memory", func(*testing.T) Store { return NewMemoryStore() }},
		{"sqlite", func(t *testing.T) Store { return openSQLite(t) }},
	}
	cases := []struct {
		name string
		run  func(*testing.T, Store)
	}{
		{"LoadMissSentinel", contractTriggerLoadMiss},
		{"SaveLoadRoundTrip", contractTriggerRoundTrip},
		{"SaveUpsert", contractTriggerUpsert},
		{"NextEventIDMonotonic", contractNextEventIDMonotonic},
		{"EnqueueListFIFO", contractEnqueueListFIFO},
		{"MarkProcessedIdempotent", contractMarkProcessedIdempotent},
		{"ListUnprocessedEmpty", contractListUnprocessedEmpty},
		{"ListInstancesByStatus", contractListInstancesByStatus},
	}
	for _, impl := range impls {
		for _, c := range cases {
			t.Run(impl.name+"/"+c.name, func(t *testing.T) {
				c.run(t, impl.newStore(t))
			})
		}
	}
}

// contractTriggerLoadMiss — LoadTriggerState на пустом Store даёт ErrTriggerStateNotFound
// (errors.Is), а не произвольную ошибку (прайминг, FR-007).
func contractTriggerLoadMiss(t *testing.T, st Store) {
	if _, err := st.LoadTriggerState("trg-0"); !errors.Is(err, ErrTriggerStateNotFound) {
		t.Errorf("LoadTriggerState(нет) err = %v, хотим ErrTriggerStateNotFound", err)
	}
}

func boolPtr(b bool) *bool           { return &b }
func strPtr(s string) *string        { return &s }
func timePtr(t time.Time) *time.Time { return &t }

// contractTriggerRoundTrip — Save→Load по каждому виду триггера сохраняет все поля,
// включая трёхзначность *bool/*time.Time/*string (nil vs 0 vs 1).
func contractTriggerRoundTrip(t *testing.T, st Store) {
	fire := time.Date(2026, 6, 13, 9, 30, 0, 0, time.UTC)
	cases := []*TriggerState{
		// metric, LastBool=nil (не праймлен)
		{TriggerID: "trg-0", Kind: "metric", LastBool: nil},
		// metric, LastBool=false
		{TriggerID: "trg-1", Kind: "metric", LastBool: boolPtr(false)},
		// metric, LastBool=true
		{TriggerID: "trg-2", Kind: "metric", LastBool: boolPtr(true)},
		// schedule_every
		{TriggerID: "trg-3", Kind: "schedule_every", LastFire: timePtr(fire)},
		// schedule_at
		{TriggerID: "trg-4", Kind: "schedule_at", LastFiredDate: strPtr("2026-06-13")},
	}
	for _, want := range cases {
		if err := st.SaveTriggerState(want); err != nil {
			t.Fatalf("SaveTriggerState(%s): %v", want.TriggerID, err)
		}
	}
	for _, want := range cases {
		got, err := st.LoadTriggerState(want.TriggerID)
		if err != nil {
			t.Fatalf("LoadTriggerState(%s): %v", want.TriggerID, err)
		}
		if got.TriggerID != want.TriggerID || got.Kind != want.Kind {
			t.Errorf("%s: id/kind = %q/%q, хотим %q/%q", want.TriggerID, got.TriggerID, got.Kind, want.TriggerID, want.Kind)
		}
		if !eqBoolPtr(got.LastBool, want.LastBool) {
			t.Errorf("%s: LastBool = %v, хотим %v", want.TriggerID, fmtBoolPtr(got.LastBool), fmtBoolPtr(want.LastBool))
		}
		if !eqTimePtr(got.LastFire, want.LastFire) {
			t.Errorf("%s: LastFire = %v, хотим %v", want.TriggerID, got.LastFire, want.LastFire)
		}
		if !eqStrPtr(got.LastFiredDate, want.LastFiredDate) {
			t.Errorf("%s: LastFiredDate = %v, хотим %v", want.TriggerID, got.LastFiredDate, want.LastFiredDate)
		}
	}
}

// contractTriggerUpsert — повторный Save с новым LastBool обновляет, не дублирует
// (persist edge-базы, FR-008/010).
func contractTriggerUpsert(t *testing.T, st Store) {
	if err := st.SaveTriggerState(&TriggerState{TriggerID: "trg-0", Kind: "metric", LastBool: boolPtr(false)}); err != nil {
		t.Fatalf("SaveTriggerState #1: %v", err)
	}
	if err := st.SaveTriggerState(&TriggerState{TriggerID: "trg-0", Kind: "metric", LastBool: boolPtr(true)}); err != nil {
		t.Fatalf("SaveTriggerState #2: %v", err)
	}
	got, err := st.LoadTriggerState("trg-0")
	if err != nil {
		t.Fatalf("LoadTriggerState: %v", err)
	}
	if got.LastBool == nil || *got.LastBool != true {
		t.Errorf("после upsert LastBool = %v, хотим true", fmtBoolPtr(got.LastBool))
	}
}

// contractNextEventIDMonotonic — минт e-NNNNNN монотонен и zero-padded (D-10).
func contractNextEventIDMonotonic(t *testing.T, st Store) {
	for i, want := range []string{"e-000001", "e-000002", "e-000003"} {
		got, err := st.NextEventID()
		if err != nil {
			t.Fatalf("NextEventID #%d: %v", i, err)
		}
		if got != want {
			t.Errorf("NextEventID #%d = %q, хотим %q", i, got, want)
		}
	}
}

// contractEnqueueListFIFO — EnqueueEvent ×3 → ListUnprocessedEvents отдаёт 3 в
// FIFO-порядке по CreatedAt (FR-016, SC-006).
func contractEnqueueListFIFO(t *testing.T, st Store) {
	base := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	// Намеренно вне порядка вставки, но CreatedAt задаёт FIFO.
	mk := func(id string, dt time.Duration) *Event {
		return &Event{ID: id, Name: "заявка_создана", PayloadJSON: `{"клиент":"ООО"}`, CreatedAt: base.Add(dt)}
	}
	for _, e := range []*Event{mk("e-000002", time.Minute), mk("e-000001", 0), mk("e-000003", 2*time.Minute)} {
		if err := st.EnqueueEvent(e); err != nil {
			t.Fatalf("EnqueueEvent %s: %v", e.ID, err)
		}
	}
	got, err := st.ListUnprocessedEvents()
	if err != nil {
		t.Fatalf("ListUnprocessedEvents: %v", err)
	}
	want := []string{"e-000001", "e-000002", "e-000003"}
	if len(got) != len(want) {
		t.Fatalf("ListUnprocessedEvents len = %d, хотим %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Errorf("FIFO[%d] = %q, хотим %q", i, got[i].ID, want[i])
		}
	}
	// Поля сохранены.
	if got[0].Name != "заявка_создана" || got[0].PayloadJSON != `{"клиент":"ООО"}` || got[0].Processed {
		t.Errorf("поля события искажены: %+v", got[0])
	}
}

// contractMarkProcessedIdempotent — MarkEventProcessed исключает событие из листинга;
// повтор — no-op без ошибки (at-least-once, FR-017).
func contractMarkProcessedIdempotent(t *testing.T, st Store) {
	base := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	for i, id := range []string{"e-000001", "e-000002"} {
		if err := st.EnqueueEvent(&Event{ID: id, Name: "n", PayloadJSON: "{}", CreatedAt: base.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatalf("EnqueueEvent %s: %v", id, err)
		}
	}
	if err := st.MarkEventProcessed("e-000001"); err != nil {
		t.Fatalf("MarkEventProcessed: %v", err)
	}
	got, err := st.ListUnprocessedEvents()
	if err != nil {
		t.Fatalf("ListUnprocessedEvents: %v", err)
	}
	if len(got) != 1 || got[0].ID != "e-000002" {
		t.Errorf("после пометки осталось %v, хотим [e-000002]", ids(got))
	}
	// Повтор по уже помеченному — no-op без ошибки.
	if err := st.MarkEventProcessed("e-000001"); err != nil {
		t.Errorf("повтор MarkEventProcessed err = %v, хотим nil", err)
	}
	got2, _ := st.ListUnprocessedEvents()
	if len(got2) != 1 || got2[0].ID != "e-000002" {
		t.Errorf("повтор изменил листинг: %v", ids(got2))
	}
}

// contractListUnprocessedEmpty — пустая очередь даёт пустой срез, не ошибку.
func contractListUnprocessedEmpty(t *testing.T, st Store) {
	got, err := st.ListUnprocessedEvents()
	if err != nil {
		t.Fatalf("ListUnprocessedEvents(пусто) err = %v, хотим nil", err)
	}
	if len(got) != 0 {
		t.Errorf("ListUnprocessedEvents(пусто) = %v, хотим []", ids(got))
	}
}

// contractListInstancesByStatus — фильтр по статусу, сортировка по возрастанию ID;
// инстансы другого статуса не попадают (рестарт-скан, FR-019).
func contractListInstancesByStatus(t *testing.T, st Store) {
	now := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	mk := func(id string, status Status) *ProcessInstance {
		return &ProcessInstance{ID: id, ProcessName: "p", Status: status, CurrentStep: "s", CreatedAt: now, UpdatedAt: now}
	}
	// Намеренно вне порядка id; смешанные статусы.
	for _, inst := range []*ProcessInstance{
		mk("p-000003", StatusRunning),
		mk("p-000001", StatusRunning),
		mk("p-000002", StatusWaiting),
		mk("p-000004", StatusRunning),
	} {
		if err := st.SaveInstance(inst); err != nil {
			t.Fatalf("SaveInstance %s: %v", inst.ID, err)
		}
	}
	got, err := st.ListInstancesByStatus(string(StatusRunning))
	if err != nil {
		t.Fatalf("ListInstancesByStatus: %v", err)
	}
	want := []string{"p-000001", "p-000003", "p-000004"}
	gotIDs := make([]string, len(got))
	for i, inst := range got {
		gotIDs[i] = inst.ID
		if inst.Status != StatusRunning {
			t.Errorf("в выборке статус %q, хотим только %q", inst.Status, StatusRunning)
		}
	}
	if !eqStrs(gotIDs, want) {
		t.Errorf("ListInstancesByStatus(выполняется) = %v, хотим %v (по возрастанию, без «ожидает»)", gotIDs, want)
	}
	// Пустой статус → пустой срез.
	if got, err := st.ListInstancesByStatus("провален"); err != nil || len(got) != 0 {
		t.Errorf("ListInstancesByStatus(провален) = %v, %v, хотим [], nil", got, err)
	}
}

// --- хелперы сравнения трёхзначных указателей ---

func eqBoolPtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func eqStrPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func eqTimePtr(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func fmtBoolPtr(b *bool) string {
	if b == nil {
		return "<nil>"
	}
	if *b {
		return "true"
	}
	return "false"
}

func ids(es []*Event) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.ID
	}
	return out
}
