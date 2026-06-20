package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/store"
)

// webhookRecorder — httptest-сервер, фиксирующий полученные POST-тела (потокобезопасно).
type webhookRecorder struct {
	mu      sync.Mutex
	bodies  []string
	methods []string
}

func (r *webhookRecorder) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.bodies = append(r.bodies, string(b))
		r.methods = append(r.methods, req.Method)
		r.mu.Unlock()
		io.WriteString(w, "") // пустое тело ответа → Пусто
	}
}

func (r *webhookRecorder) snapshot() ([]string, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.bodies...), append([]string(nil), r.methods...)
}

func notifyFixture() string { return filepath.Join("testdata", "webhook_notify.ladix") }

// --- C-CLI-1 (T027): флаг + env активируют POST ---

// TestRunWebhookFlagPosts — run --webhook <httptest> → POST тела уведомить, стаб НЕ печатается.
func TestRunWebhookFlagPosts(t *testing.T) {
	rec := &webhookRecorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", notifyFixture(), "--webhook", srv.URL}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, stderr=%q", code, errBuf.String())
	}
	bodies, methods := rec.snapshot()
	if len(bodies) != 1 {
		t.Fatalf("получено POST = %d, хотим 1; тела=%v", len(bodies), bodies)
	}
	if methods[0] != http.MethodPost {
		t.Errorf("метод = %q, хотим POST", methods[0])
	}
	if want := `{"цель":"ИТ","данные":["заявка от Петров"]}`; bodies[0] != want {
		t.Errorf("тело = %q, хотим %q", bodies[0], want)
	}
	if strings.Contains(out.String(), "[уведомление]") {
		t.Errorf("стаб печатался под --webhook: %q", out.String())
	}
}

// TestRunWebhookEnvPosts — env LADIX_WEBHOOK без флага → POST.
func TestRunWebhookEnvPosts(t *testing.T) {
	rec := &webhookRecorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	t.Setenv("LADIX_WEBHOOK", srv.URL)
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", notifyFixture()}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, stderr=%q", code, errBuf.String())
	}
	bodies, _ := rec.snapshot()
	if len(bodies) != 1 {
		t.Fatalf("env: получено POST = %d, хотим 1", len(bodies))
	}
	if strings.Contains(out.String(), "[уведомление]") {
		t.Errorf("стаб печатался под env-вебхуком: %q", out.String())
	}
}

// TestWebhookFlagBeatsEnv — оба заданы → POST идёт на флаг-URL, не env.
func TestWebhookFlagBeatsEnv(t *testing.T) {
	flagRec := &webhookRecorder{}
	flagSrv := httptest.NewServer(flagRec.handler())
	defer flagSrv.Close()
	envRec := &webhookRecorder{}
	envSrv := httptest.NewServer(envRec.handler())
	defer envSrv.Close()

	t.Setenv("LADIX_WEBHOOK", envSrv.URL)
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", notifyFixture(), "--webhook", flagSrv.URL}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, stderr=%q", code, errBuf.String())
	}
	if fb, _ := flagRec.snapshot(); len(fb) != 1 {
		t.Errorf("флаг-сервер получил POST = %d, хотим 1 (флаг приоритетнее env)", len(fb))
	}
	if eb, _ := envRec.snapshot(); len(eb) != 0 {
		t.Errorf("env-сервер получил POST = %d, хотим 0", len(eb))
	}
}

// --- C-CLI-2 (T028): ошибка неверного URL, ДОСЛОВНО ---

// TestWebhookInvalidURL — run --webhook '://мусор' → stderr ровно §AU-10.C, exit 2, stdout пуст.
func TestWebhookInvalidURL(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", notifyFixture(), "--webhook", "://мусор"}, &out, &errBuf)
	if code != 2 {
		t.Errorf("код = %d, хотим 2", code)
	}
	if want := "ladix: неверный URL вебхука '://мусор'\n"; errBuf.String() != want {
		t.Errorf("stderr = %q, хотим %q", errBuf.String(), want)
	}
	if out.Len() != 0 {
		t.Errorf("stdout не пуст: %q (движок не должен запускаться)", out.String())
	}
}

// --- C-CLI-4 (T030): без флага → стаб ---

// TestRunNoWebhookUsesStub — run без флага/env → стаб печатает §EN-7, сеть не трогается.
func TestRunNoWebhookUsesStub(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", notifyFixture()}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, stderr=%q", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "[уведомление] ИТ: заявка от Петров") {
		t.Errorf("стаб не напечатал уведомление: %q", out.String())
	}
}

// --- C-CLI-3 (T029): serve = единый движок доставляет на вебхук ---

// TestServeWebhookEscalationPosts — событие-триггер в serve запускает процесс, чей
// авто-шаг шлёт уведомить; serve строит ОДИН движок (buildServeDaemon) с вебхуком →
// тело уходит POST на вебхук (не стаб). Доказывает single-engine wiring serve→webhook.
func TestServeWebhookEscalationPosts(t *testing.T) {
	rec := &webhookRecorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	db := filepath.Join(t.TempDir(), "serve.db")
	// Положить событие в очередь.
	var eo, ee bytes.Buffer
	if code := realMain([]string{"emit", "пуск", `{"кто":"ООО"}`, "--db", db}, &eo, &ee); code != 0 {
		t.Fatalf("emit: код=%d stderr=%q", code, ee.String())
	}

	src := `процесс известить(кто):
    шаг сообщить:
        уведомить ИТ("пуск от " + кто)

когда событие пуск:
    запустить процесс известить(событие.кто)
`
	sq, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("открытие БД: %v", err)
	}
	defer sq.Close()
	var out bytes.Buffer
	prog := parseServeSrc(t, src)
	caller, cerr := openExternalCaller(srv.URL)
	if cerr != nil {
		t.Fatalf("openExternalCaller: %v", cerr)
	}
	d, code := buildServeDaemon(prog, sq, 5*time.Millisecond, 0,
		fixedClock{time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local)}, caller, &out, &out)
	if d == nil {
		t.Fatalf("buildServeDaemon вернул nil, код=%d; out=%q", code, out.String())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	waitUntil(t, func() bool {
		evs, _ := sq.ListUnprocessedEvents()
		return len(evs) == 0
	})
	// Дать тику доставить тело на вебхук.
	waitUntil(t, func() bool {
		b, _ := rec.snapshot()
		return len(b) >= 1
	})
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}

	bodies, _ := rec.snapshot()
	if len(bodies) != 1 {
		t.Fatalf("вебхук получил POST = %d, хотим 1; out=%q", len(bodies), out.String())
	}
	if want := `{"цель":"ИТ","данные":["пуск от ООО"]}`; bodies[0] != want {
		t.Errorf("тело serve→вебхук = %q, хотим %q", bodies[0], want)
	}
	if strings.Contains(out.String(), "[уведомление]") {
		t.Errorf("serve печатал стаб вместо POST: %q", out.String())
	}
}
