package main

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// outbox_exactly_once_test.go — durable гейт-тест exactly-once доставки реального
// эффекта ТЕЛА ШАГА (M3-C2b, §C-2b.7 / §C-1). Inline-const источник (образец
// m2CLISrc), изолирован от файловых golden.
//
// Сценарий усиленного §2 (charter §2): человеческий шаг связаться_с_клиентом →
// АВТО-шаг зафиксировать_итог (присвоить итог = данные.итог — durable) → АВТО-шаг
// уведомить_crm (уведомить crm(...) — РЕАЛЬНЫЙ эффект в теле шага, дедуплицируется
// outbox'ом). Гейт: эффект crm доставляется РОВНО один раз через рестарт демона.

// exactlyOnceSrc — усиленный §2 для гейт-теста (зеркало examples/контроль_плана.ladix
// после эволюции T023, но самодостаточный inline-const). itog читается durable на
// рестарте → арг-эвал «итог звонка: » + итог успешен → эффект переисполняется → outbox
// глушит повтор.
const exactlyOnceSrc = `процесс эскалация_плана(факт):
    шаг связаться_с_клиентом:
        исполнитель: "менеджер"
        срок:        3дн
    шаг зафиксировать_итог после связаться_с_клиентом:
        присвоить итог = данные.итог
    шаг уведомить_crm после зафиксировать_итог:
        уведомить crm("итог звонка: " + итог)

когда задача просрочена в эскалация_плана.связаться_с_клиентом:
    уведомить руководитель(факт)
`

// crmPOSTCount считает POST, ушедшие на цель "crm" (тело эффекта шага), отделяя их от
// POST эскалации на "руководитель".
func crmPOSTCount(sink *m2Sink) int {
	bodies, _ := sink.snapshot()
	n := 0
	for _, b := range bodies {
		if strings.Contains(b, `"цель":"crm"`) {
			n++
		}
	}
	return n
}

// TestStepEffectExactlyOnceRestart — ГЕЙТ §2 (durable exactly-once, §C-2b.7).
// (1) start → complete --data: процесс доходит до уведомить_crm, тело POST'ит crm
//
//	РОВНО раз (POST=1), outbox фиксирует delivered.
//
// (2) Имитация краша mid-advance ПОСЛЕ SaveOutbox: инстанс сбрасывается в running на
//
//	шаге уведомить_crm (статус, который рестарт-скан реактивирует) на той же --db.
//
// (3) Новый Store на той же --db → RunRestartScan → реактивация → тики переисполняют
//
//	тело уведомить_crm → outbox видит delivered → доставка ПРОПУЩЕНА → POST остаётся 1.
//
// Зеркало driveServeToNoRepeat / TestDeadlineDurableRestart.
func TestStepEffectExactlyOnceRestart(t *testing.T) {
	dir := t.TempDir()
	prog := filepath.Join(dir, "exactly_once.ladix")
	if err := os.WriteFile(prog, []byte(exactlyOnceSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "demo.db")

	sink := &m2Sink{}
	srv := httptest.NewServer(sink.handler())
	defer srv.Close()

	// --- Стадия 1: start + complete --data доводят инстанс до уведомить_crm ---
	var so, se bytes.Buffer
	if code := realMain([]string{"start", prog, "эскалация_плана", "2500000", "--db", db}, &so, &se); code != 0 {
		t.Fatalf("start: код=%d stderr=%q", code, se.String())
	}

	caller, cerr := openExternalCaller(srv.URL)
	if cerr != nil {
		t.Fatalf("openExternalCaller: %v", cerr)
	}
	var co, ce bytes.Buffer
	// complete провязывается с тем же webhook-драйвером (через openExternalCaller),
	// чтобы реальный эффект crm в теле шага догона ушёл POST'ом на sink.
	if code := completeTask(prog, "t-000001", db, 0, `{"итог":"перезвонит"}`, caller, engine.SystemClock{}, &co, &ce); code != 0 {
		t.Fatalf("complete --data: код=%d stderr=%q out=%q", code, ce.String(), co.String())
	}

	if got := crmPOSTCount(sink); got != 1 {
		t.Fatalf("после complete: POST на crm = %d, хотим РОВНО 1", got)
	}

	// Подтвердить, что outbox зафиксировал доставленную запись (deliver-then-record прошёл).
	verifyStore, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("open db (проверка outbox): %v", err)
	}
	inst, err := verifyStore.LoadInstance("p-000001")
	if err != nil {
		t.Fatalf("LoadInstance: %v", err)
	}
	// Найти запись outbox шага уведомить_crm (effectIndex 0).
	if _, err := verifyStore.LoadOutbox("p-000001|уведомить_crm|0"); err != nil {
		t.Fatalf("outbox-запись уведомить_crm не найдена после доставки: %v", err)
	}
	// Durable: итог персистнут (split-шаг зафиксировать_итог) — без него арг-эвал на
	// рестарте провалился бы (Строка + Пусто), POST=0 (§C-1.2).
	if v, ok := inst.Variables["итог"]; !ok {
		t.Fatalf("итог НЕ персистнут durable: %+v", inst.Variables)
	} else if got := value.String(v); got != "перезвонит" {
		t.Fatalf("итог = %q, хотим \"перезвонит\"", got)
	}

	// --- Стадия 2: имитация краша mid-advance ПОСЛЕ SaveOutbox ---
	// Сбросить инстанс в running на шаге уведомить_crm (как если бы краш произошёл при
	// начале итерации тела уведомить_crm, ПОСЛЕ того как POST уже доставлен и записан).
	inst.Status = store.StatusRunning
	inst.CurrentStep = "уведомить_crm"
	if err := verifyStore.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance (имитация краша): %v", err)
	}
	verifyStore.Close()

	// --- Стадия 3: рестарт — новый Store на той же --db, RunRestartScan, тики ---
	src, _ := os.ReadFile(prog)
	sq, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("open db (рестарт): %v", err)
	}
	defer sq.Close()
	progAST := parseServeSrc(t, string(src))
	rcaller, _ := openExternalCaller(srv.URL)
	clk := &mutClock{t: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)}
	var out bytes.Buffer
	d, code := buildServeDaemon(progAST, sq, 5*time.Millisecond, 0, clk, rcaller, &out, &out)
	if d == nil {
		t.Fatalf("buildServeDaemon вернул nil (рестарт), код=%d; out=%q", code, out.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		d.RunRestartScan() // реактивирует p-000001 на уведомить_crm → переисполняет тело
		done <- d.Run(ctx)
	}()
	time.Sleep(60 * time.Millisecond) // ~12 тиков при 5ms интервале — повтор успел бы случиться
	cancel()
	if rerr := <-done; rerr != nil {
		t.Fatalf("Run (рестарт): %v", rerr)
	}

	// ГЕЙТ: эффект crm доставлен exactly-once несмотря на переисполнение тела на рестарте.
	if got := crmPOSTCount(sink); got != 1 {
		t.Fatalf("§C-2b.7: POST на crm суммарно = %d, хотим РОВНО 1 (exactly-once через рестарт); out=%q", got, out.String())
	}
}
