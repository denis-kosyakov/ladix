package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// outbox_test.go — контракт Store.LoadOutbox/SaveOutbox (M3-C2b, §C-2b.6,
// contracts/store-outbox-methods.md). Прогон на ОБЕИХ реализациях (Memory + SQLite):
// round-trip всех полей, ErrOutboxNotFound, upsert, глубокая копия Memory.

// outboxStores строит обе реализации для табличного прогона. SQLite — на временном
// файле (миграция C2a создаёт таблицу outbox при открытии).
func outboxStores(t *testing.T) map[string]Store {
	t.Helper()
	sq, err := NewSQLiteStore(filepath.Join(t.TempDir(), "outbox.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = sq.Close() })
	return map[string]Store{
		"memory": NewMemoryStore(),
		"sqlite": sq,
	}
}

func sampleOutboxRecord() *OutboxRecord {
	created := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	delivered := time.Date(2026, 6, 18, 10, 0, 1, 0, time.UTC)
	return &OutboxRecord{
		DedupKey:    "p-000001|уведомить_crm|0",
		InstanceID:  "p-000001",
		StepName:    "уведомить_crm",
		EffectIndex: 0,
		Kind:        "уведомить",
		Target:      "crm",
		Args:        []value.Value{value.Строка{V: "итог звонка: 2500000"}},
		Result:      value.None,
		Delivered:   true,
		CreatedAt:   created,
		DeliveredAt: &delivered,
	}
}

func TestSaveLoadOutboxRoundTrip(t *testing.T) {
	for name, st := range outboxStores(t) {
		t.Run(name, func(t *testing.T) {
			rec := sampleOutboxRecord()
			if err := st.SaveOutbox(rec); err != nil {
				t.Fatalf("SaveOutbox: %v", err)
			}
			got, err := st.LoadOutbox(rec.DedupKey)
			if err != nil {
				t.Fatalf("LoadOutbox: %v", err)
			}
			if got.DedupKey != rec.DedupKey || got.InstanceID != rec.InstanceID ||
				got.StepName != rec.StepName || got.EffectIndex != rec.EffectIndex ||
				got.Kind != rec.Kind || got.Target != rec.Target {
				t.Errorf("скалярные поля разошлись: got %+v", got)
			}
			if got.Delivered != rec.Delivered {
				t.Errorf("Delivered: got %v, want %v", got.Delivered, rec.Delivered)
			}
			if len(got.Args) != 1 || value.String(got.Args[0]) != "итог звонка: 2500000" {
				t.Errorf("Args round-trip: got %#v", got.Args)
			}
			if _, ok := got.Result.(value.Пусто); !ok {
				t.Errorf("Result None round-trip: got %T (%#v)", got.Result, got.Result)
			}
			if !got.CreatedAt.Equal(rec.CreatedAt) {
				t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, rec.CreatedAt)
			}
			if got.DeliveredAt == nil || !got.DeliveredAt.Equal(*rec.DeliveredAt) {
				t.Errorf("DeliveredAt: got %v, want %v", got.DeliveredAt, rec.DeliveredAt)
			}
		})
	}
}

func TestLoadOutboxNotFound(t *testing.T) {
	for name, st := range outboxStores(t) {
		t.Run(name, func(t *testing.T) {
			_, err := st.LoadOutbox("нет|такого|99")
			if !errors.Is(err, ErrOutboxNotFound) {
				t.Fatalf("LoadOutbox(несуществующий): err=%v, want ErrOutboxNotFound", err)
			}
		})
	}
}

func TestSaveOutboxUpsert(t *testing.T) {
	for name, st := range outboxStores(t) {
		t.Run(name, func(t *testing.T) {
			rec := sampleOutboxRecord()
			rec.Kind = "вызвать"
			rec.Result = value.Целое{V: 1}
			rec.Delivered = false
			rec.DeliveredAt = nil
			if err := st.SaveOutbox(rec); err != nil {
				t.Fatalf("SaveOutbox #1: %v", err)
			}
			// Повторный Save тем же ключом — upsert (последнее значение).
			rec2 := sampleOutboxRecord()
			rec2.Kind = "вызвать"
			rec2.Result = value.Целое{V: 999}
			rec2.Delivered = true
			if err := st.SaveOutbox(rec2); err != nil {
				t.Fatalf("SaveOutbox #2: %v", err)
			}
			got, err := st.LoadOutbox(rec.DedupKey)
			if err != nil {
				t.Fatalf("LoadOutbox: %v", err)
			}
			gi, ok := got.Result.(value.Целое)
			if !ok || gi.V != 999 {
				t.Errorf("upsert Result: got %#v, want Целое{999}", got.Result)
			}
			if !got.Delivered {
				t.Errorf("upsert Delivered: got false, want true")
			}
		})
	}
}

// TestMemoryOutboxDeepCopy — мутация Args[0]/*DeliveredAt в исходном rec ПОСЛЕ Save
// не протекает в леджер (глубокая копия среза + указателей времён, как copyTask).
func TestMemoryOutboxDeepCopy(t *testing.T) {
	st := NewMemoryStore()
	rec := sampleOutboxRecord()
	rec.Args = []value.Value{value.Строка{V: "оригинал"}}
	if err := st.SaveOutbox(rec); err != nil {
		t.Fatalf("SaveOutbox: %v", err)
	}
	// Мутируем срез и указатель времени снаружи.
	rec.Args[0] = value.Строка{V: "ПОДМЕНА"}
	*rec.DeliveredAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	got, err := st.LoadOutbox(rec.DedupKey)
	if err != nil {
		t.Fatalf("LoadOutbox: %v", err)
	}
	if value.String(got.Args[0]) != "оригинал" {
		t.Errorf("Args мутация протекла: got %q, want \"оригинал\"", value.String(got.Args[0]))
	}
	if got.DeliveredAt.Year() == 2000 {
		t.Errorf("DeliveredAt мутация протекла: got %v", got.DeliveredAt)
	}
	// Повторный Load тоже изолирован от мутации возвращённой копии.
	got.Args[0] = value.Строка{V: "вторая подмена"}
	again, _ := st.LoadOutbox(rec.DedupKey)
	if value.String(again.Args[0]) != "оригинал" {
		t.Errorf("Load не вернул свежую копию: got %q", value.String(again.Args[0]))
	}
}
