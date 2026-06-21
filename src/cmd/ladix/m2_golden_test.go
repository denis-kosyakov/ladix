package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/store"
)

// m2_golden_test.go — CLI-форма гейт-сценария вехи M2 (§AU-12.C/B). Companion к
// детерминированному daemon-golden (internal/daemon/m2_endtoend_test.go): здесь
// реальные подкоманды `ladix start`/`complete --data`/`inspect` через realMain на
// временной SQLite-БД, плюс §AU-12.B durable×рестарт В CLI-ФОРМЕ (start → прогон
// демона до эскалации через httptest-вебхук → рестарт store → нет повтора).
//
// Реализованная сигнатура start = `start <файл.ladix> <процесс> [аргументы]` (файл
// первым) — осознанное отклонение от §AU-7, функционально цепочку не ломает.
//
// Прод-логика НЕ тронута: тест только наблюдает CLI-выводы и Store.

// mutClock — мутабельные управляемые часы (engine.Clock) для пошагового продвижения в
// CLI-companion (cmd/ladix.fixedClock неизменяем). Конкурентно-безопасен: d.Run крутит
// тики в горутине, тест продвигает clock.
type mutClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *mutClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *mutClock) set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

// m2CLISrc — исходник §AU-12.C для CLI-companion: процесс эскалация_плана с
// человеческим шагом связаться_с_клиентом (исполнитель+срок 3дн), затем АВТО-шаг
// решение, читающий payload данные.итог (B3 догон). Эскалация-триггер уведомляет
// руководителя переменной факт. Top-level НЕ запускает процесс (старт через CLI start).
const m2CLISrc = `процесс эскалация_плана(факт):
    шаг связаться_с_клиентом:
        исполнитель: "менеджер"
        срок:        3дн
    шаг решение после связаться_с_клиентом:
        присвоить итог = данные.итог
        печать("итог догона:", данные.итог)

когда задача просрочена в эскалация_плана.связаться_с_клиентом:
    уведомить руководитель(факт)
`

// writeM2CLIProg кладёт m2CLISrc во временный файл, возвращает путь к программе и БД.
func writeM2CLIProg(t *testing.T) (prog, db string) {
	t.Helper()
	dir := t.TempDir()
	prog = filepath.Join(dir, "контроль_плана_cli.ladix")
	if err := os.WriteFile(prog, []byte(m2CLISrc), 0o644); err != nil {
		t.Fatal(err)
	}
	db = filepath.Join(dir, "demo.db")
	return prog, db
}

// m2Sink — httptest-приёмник POST (потокобезопасно).
type m2Sink struct {
	mu      sync.Mutex
	bodies  []string
	methods []string
}

func (s *m2Sink) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		s.mu.Lock()
		s.bodies = append(s.bodies, string(b))
		s.methods = append(s.methods, req.Method)
		s.mu.Unlock()
		io.WriteString(w, "")
	}
}

func (s *m2Sink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bodies)
}

func (s *m2Sink) snapshot() ([]string, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.bodies...), append([]string(nil), s.methods...)
}

// TestM2GoldenCLI — §AU-12.C CLI-форма: start → (демон до эскалации через webhook) →
// complete --data (payload виден догоном) → inspect (история с «, эскалирована»).
// Плюс §AU-12.B durable×рестарт в CLI-форме.
func TestM2GoldenCLI(t *testing.T) {
	prog, db := writeM2CLIProg(t)

	sink := &m2Sink{}
	srv := httptest.NewServer(sink.handler())
	defer srv.Close()

	// --- Стадия: B5 `ladix start <файл> эскалация_плана 2500000 --db demo.db` ---
	var so, se bytes.Buffer
	if code := realMain([]string{"start", prog, "эскалация_плана", "2500000", "--db", db}, &so, &se); code != 0 {
		t.Fatalf("start: код=%d stderr=%q", code, se.String())
	}
	if !strings.Contains(so.String(), "запущен инстанс p-000001") {
		t.Fatalf("start не подтвердил инстанс: %q", so.String())
	}

	// Прочитать реальный дедлайн задачи (start использует eval.SystemClock → now+3дн).
	stRead, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("open db для чтения дедлайна: %v", err)
	}
	task, err := stRead.LoadTask("t-000001")
	if err != nil {
		t.Fatalf("LoadTask t-000001: %v", err)
	}
	if task.Deadline == nil || task.Assignee != "менеджер" {
		t.Fatalf("задача не та: %+v", task)
	}
	deadline := *task.Deadline
	stRead.Close()

	// --- Стадия §AU-12.B (CLI durable, прогон 1): serve-демон с вебхуком, часы ЗА срок ---
	afterDL := deadline.Add(time.Minute)
	n1 := driveServeToEscalation(t, prog, db, srv.URL, afterDL, sink, 1)
	if n1 != 1 {
		t.Fatalf("прогон 1: webhook POST = %d, хотим РОВНО 1 (эскалация раз)", n1)
	}
	bodies, methods := sink.snapshot()
	if methods[0] != http.MethodPost {
		t.Fatalf("метод=%q, хотим POST", methods[0])
	}
	if want := `{"цель":"руководитель","данные":[2500000]}`; bodies[0] != want {
		t.Fatalf("тело POST=%q, хотим %q (факт=2500000)", bodies[0], want)
	}

	// --- Стадия §AU-12.B (РЕСТАРТ, прогон 2): новый store на той же --db, часы за срок ---
	// Escalated персистнут → checkDeadlines continue → НЕТ повтора.
	n2 := driveServeToNoRepeat(t, prog, db, srv.URL, afterDL, sink)
	if n2 != 1 {
		t.Fatalf("§AU-12.B рестарт: webhook POST суммарно = %d, хотим РОВНО 1 (нет повтора)", n2)
	}

	// --- Стадия B3: complete --data '{"итог":"перезвонит"}' → авто-шаг догона видит payload ---
	var co, ce bytes.Buffer
	if code := realMain([]string{"complete", prog, "t-000001", "--data", `{"итог":"перезвонит"}`, "--db", db}, &co, &ce); code != 0 {
		t.Fatalf("complete --data: код=%d stderr=%q", code, ce.String())
	}
	if !strings.Contains(co.String(), "итог догона: перезвонит") {
		t.Fatalf("payload --data не дошёл до первого шага догона: %q", co.String())
	}

	// --- Стадия B6: inspect показывает снимок + историю с «, эскалирована» ---
	var io_, ie bytes.Buffer
	if code := realMain([]string{"inspect", "p-000001", "--db", db}, &io_, &ie); code != 0 {
		t.Fatalf("inspect: код=%d stderr=%q", code, ie.String())
	}
	insp := io_.String()
	if !strings.Contains(insp, "инстанс p-000001: процесс эскалация_плана") {
		t.Fatalf("inspect без снимка инстанса: %q", insp)
	}
	if !strings.Contains(insp, "t-000001 шаг 'связаться_с_клиентом' → менеджер") {
		t.Fatalf("inspect без задачи t-000001: %q", insp)
	}
	if !strings.Contains(insp, ", эскалирована") {
		t.Fatalf("inspect не показал суффикс «, эскалирована» в истории: %q", insp)
	}
	// Задача завершена complete → история показывает «завершена».
	if !strings.Contains(insp, ", завершена, эскалирована") {
		t.Fatalf("inspect: задача должна быть завершена+эскалирована: %q", insp)
	}
}

// driveServeToEscalation поднимает serve-демон (buildServeDaemon с httptest-вебхуком и
// мутабельными часами, выставленными за срок), крутит d.Run до wantPOST POST на вебхук,
// затем грациозно останавливает. Возвращает суммарное число POST на sink.
func driveServeToEscalation(t *testing.T, prog, db, webhookURL string, clockAt time.Time, sink *m2Sink, wantPOST int) int {
	t.Helper()
	src, err := os.ReadFile(prog)
	if err != nil {
		t.Fatalf("read prog: %v", err)
	}
	sq, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer sq.Close()
	progAST := parseServeSrc(t, string(src))
	caller, cerr := openExternalCaller(webhookURL)
	if cerr != nil {
		t.Fatalf("openExternalCaller: %v", cerr)
	}
	clk := &mutClock{t: clockAt}
	var out bytes.Buffer
	d, code := buildServeDaemon(progAST, sq, 5*time.Millisecond, 0, "", clk, caller, &out, &out)
	if d == nil {
		t.Fatalf("buildServeDaemon вернул nil, код=%d; out=%q", code, out.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	waitUntil(t, func() bool { return sink.count() >= wantPOST })
	cancel()
	if rerr := <-done; rerr != nil {
		t.Fatalf("Run: %v", rerr)
	}
	return sink.count()
}

// driveServeToNoRepeat имитирует РЕСТАРТ (§AU-12.B): новый store на той же --db,
// RunRestartScan реактивирует инстанс, несколько тиков за срок НЕ повторяют эскалацию
// (Escalated персистнут). Крутит демон фиксированное окно и убеждается, что POST не
// прибавилось.
func driveServeToNoRepeat(t *testing.T, prog, db, webhookURL string, clockAt time.Time, sink *m2Sink) int {
	t.Helper()
	before := sink.count()
	src, err := os.ReadFile(prog)
	if err != nil {
		t.Fatalf("read prog: %v", err)
	}
	sq, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("open db (рестарт): %v", err)
	}
	defer sq.Close()

	// На рестарте Escalated читается true из той же --db (durable-персист).
	rt, err := sq.LoadTask("t-000001")
	if err != nil {
		t.Fatalf("LoadTask после рестарта: %v", err)
	}
	if !rt.Escalated {
		t.Fatalf("§AU-12.B: Escalated НЕ персистнут между прогонами (durable нарушен)")
	}

	progAST := parseServeSrc(t, string(src))
	caller, cerr := openExternalCaller(webhookURL)
	if cerr != nil {
		t.Fatalf("openExternalCaller: %v", cerr)
	}
	clk := &mutClock{t: clockAt}
	var out bytes.Buffer
	d, code := buildServeDaemon(progAST, sq, 5*time.Millisecond, 0, "", clk, caller, &out, &out)
	if d == nil {
		t.Fatalf("buildServeDaemon вернул nil (рестарт), код=%d; out=%q", code, out.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		d.RunRestartScan() // рестарт-скан ДО тиков
		done <- d.Run(ctx)
	}()
	// Дать демону прокрутить достаточно тиков (несколько интервалов), чтобы повтор —
	// если бы фильтр Escalated сняли — успел случиться. Затем грациозно остановить.
	waitUntil(t, func() bool { return d != nil }) // демон поднят
	time.Sleep(60 * time.Millisecond)             // ~12 тиков при 5ms интервале
	cancel()
	if rerr := <-done; rerr != nil {
		t.Fatalf("Run (рестарт): %v", rerr)
	}
	if sink.count() != before {
		t.Fatalf("§AU-12.B: повтор эскалации после рестарта (POST %d→%d); out=%q", before, sink.count(), out.String())
	}
	return sink.count()
}
