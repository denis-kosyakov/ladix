package store

import (
	"path/filepath"
	"testing"
	"time"
)

// TestSQLiteEventCounterPersists — счётчик event персистентен через Close+
// переоткрытие, нумерация e-NNNNNN продолжается (D-10, зеркало
// TestSQLiteCounterPersists для instance/task).
func TestSQLiteEventCounterPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ladix.db")
	st, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for _, want := range []string{"e-000001", "e-000002"} {
		got, _ := st.NextEventID()
		if got != want {
			t.Fatalf("mint = %q, want %q", got, want)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	st2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st2.Close()
	if got, _ := st2.NextEventID(); got != "e-000003" {
		t.Errorf("после reopen NextEventID = %q, want e-000003", got)
	}
}

// TestSQLiteTriggerStateAndEventsPersist — trigger_state и events переживают
// Close+переоткрытие (durable-состояние демона, EM-17.2/17.3).
func TestSQLiteTriggerStateAndEventsPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ladix.db")
	st, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	fire := time.Date(2026, 6, 13, 9, 30, 0, 0, time.UTC)
	b := true
	if err := st.SaveTriggerState(&TriggerState{TriggerID: "trg-0", Kind: "metric", LastBool: &b}); err != nil {
		t.Fatalf("SaveTriggerState metric: %v", err)
	}
	if err := st.SaveTriggerState(&TriggerState{TriggerID: "trg-1", Kind: "schedule_every", LastFire: &fire}); err != nil {
		t.Fatalf("SaveTriggerState every: %v", err)
	}
	if err := st.EnqueueEvent(&Event{ID: "e-000001", Name: "заявка_создана", PayloadJSON: `{"к":"О"}`, CreatedAt: fire}); err != nil {
		t.Fatalf("EnqueueEvent: %v", err)
	}
	st.Close()

	st2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st2.Close()

	ts, err := st2.LoadTriggerState("trg-0")
	if err != nil {
		t.Fatalf("LoadTriggerState после reopen: %v", err)
	}
	if ts.Kind != "metric" || ts.LastBool == nil || *ts.LastBool != true {
		t.Errorf("trigger_state потеряно при reopen: %+v", ts)
	}
	ev, err := st2.ListUnprocessedEvents()
	if err != nil {
		t.Fatalf("ListUnprocessedEvents после reopen: %v", err)
	}
	if len(ev) != 1 || ev[0].ID != "e-000001" || ev[0].Name != "заявка_создана" {
		t.Errorf("events потеряны при reopen: %v", ev)
	}
}
