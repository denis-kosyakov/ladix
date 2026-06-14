package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/store"
)

// TestEmitEnqueuesEvent — emit заявка_создана '{"клиент":"ООО"}' → exit 0, одна
// необработанная строка в events с этим payload (проверка через Store). SC-006.
func TestEmitEnqueuesEvent(t *testing.T) {
	db := filepath.Join(t.TempDir(), "emit.db")
	var out, errBuf bytes.Buffer
	code := realMain([]string{"emit", "заявка_создана", `{"клиент":"ООО"}`, "--db", db}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}

	sq, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("открытие БД: %v", err)
	}
	defer sq.Close()
	evs, err := sq.ListUnprocessedEvents()
	if err != nil {
		t.Fatalf("ListUnprocessedEvents: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("ожидалось 1 событие в очереди, получено %d", len(evs))
	}
	if evs[0].Name != "заявка_создана" || evs[0].PayloadJSON != `{"клиент":"ООО"}` {
		t.Fatalf("событие записано неверно: name=%q payload=%q", evs[0].Name, evs[0].PayloadJSON)
	}
	if evs[0].Processed {
		t.Fatalf("новое событие не должно быть processed")
	}
}

// TestEmitTwoEventsFIFO — два emit подряд → две строки, FIFO по CreatedAt/ID
// (ListUnprocessedEvents). ID монотонны e-000001, e-000002. SC-006.
func TestEmitTwoEventsFIFO(t *testing.T) {
	db := filepath.Join(t.TempDir(), "emit.db")
	var out, errBuf bytes.Buffer
	if code := realMain([]string{"emit", "первое", "--db", db}, &out, &errBuf); code != 0 {
		t.Fatalf("emit 1: код=%d stderr=%q", code, errBuf.String())
	}
	if code := realMain([]string{"emit", "второе", "--db", db}, &out, &errBuf); code != 0 {
		t.Fatalf("emit 2: код=%d stderr=%q", code, errBuf.String())
	}

	sq, _ := store.NewSQLiteStore(db)
	defer sq.Close()
	evs, _ := sq.ListUnprocessedEvents()
	if len(evs) != 2 {
		t.Fatalf("ожидалось 2 события, получено %d", len(evs))
	}
	if evs[0].Name != "первое" || evs[1].Name != "второе" {
		t.Fatalf("FIFO нарушен: [%q, %q]", evs[0].Name, evs[1].Name)
	}
	if evs[0].ID != "e-000001" || evs[1].ID != "e-000002" {
		t.Fatalf("ID не монотонны: [%q, %q]", evs[0].ID, evs[1].ID)
	}
}

// TestEmitEmptyPayloadAllowed — emit без payload допустим (пустая строка пишется как
// есть; демон маппит в пустую Запись). exit 0.
func TestEmitEmptyPayloadAllowed(t *testing.T) {
	db := filepath.Join(t.TempDir(), "emit.db")
	var out, errBuf bytes.Buffer
	code := realMain([]string{"emit", "пинг", "--db", db}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	sq, _ := store.NewSQLiteStore(db)
	defer sq.Close()
	evs, _ := sq.ListUnprocessedEvents()
	if len(evs) != 1 || evs[0].PayloadJSON != "" {
		t.Fatalf("ожидалось одно событие с пустым payload, получено %+v", evs)
	}
}

// TestEmitNoNameUsage — emit без имени события → exit 2 (usage). SC-006.
func TestEmitNoNameUsage(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"emit"}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("код = %d, хотим 2", code)
	}
	if errBuf.Len() == 0 {
		t.Fatalf("ожидался usage в stderr")
	}
}

// TestEmitUnknownFlag — неизвестный флаг → exit 2.
func TestEmitUnknownFlag(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := realMain([]string{"emit", "имя", "--bogus"}, &out, &errBuf); code != 2 {
		t.Fatalf("код = %d, хотим 2", code)
	}
}

// TestEmitConfirmationStdoutGolden — единственная строка подтверждения emit в stdout
// (emit.go): exact-match golden. Свежая БД → монотонный e-000001; строка не зависит
// от часов, поэтому детерминирована. exit 0, пустой stderr.
func TestEmitConfirmationStdoutGolden(t *testing.T) {
	db := filepath.Join(t.TempDir(), "emit.db")
	var out, errBuf bytes.Buffer
	code := realMain([]string{"emit", "заявка_создана", "--db", db}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if out.String() != "событие e-000001 'заявка_создана' поставлено в очередь\n" {
		t.Errorf("stdout = %q", out.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
}
