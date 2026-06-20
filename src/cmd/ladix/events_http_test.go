package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/store"
)

// Замки трека B «Входящие события» (§IE-7, FR-IE-1..11), httptest, симметрично
// webhook_cli_test.go. fixedClock/parseServeSrc/waitUntil/waitGoroutines — из
// serve_golden_test.go (пакет main).

// testClock — общие детерминированные часы для golden приёма.
var testClock = fixedClock{time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local)}

// httpPost — POST с сырым телом, возврат (код, тело-строка).
func httpPost(t *testing.T, rawurl, body string, headers map[string]string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, rawurl, strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", rawurl, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// failEnqueueStore — Store, чей EnqueueEvent всегда падает (FR-IE-6, R9). Встраивает
// интерфейс → 17 остальных методов делегируются вложенному MemoryStore (NextEventID
// работает, EnqueueEvent — нет).
type failEnqueueStore struct{ store.Store }

func (failEnqueueStore) EnqueueEvent(*store.Event) error {
	return errors.New("диск переполнен")
}

// --- FR-IE-2: статический замок изоляции хендлера от движка ---
// Если в сигнатуру eventsHandler протечёт *engine.Engine/интерпретатор — не скомпилится.
var _ = func() http.Handler {
	return eventsHandler(store.NewMemoryStore(), engine.SystemClock{}, "")
}

// --- FR-IE-3: неразличимость источников (POST кириллица → тик → тело триггера) ---

func TestInboundPostFiresTrigger(t *testing.T) {
	db := filepath.Join(t.TempDir(), "inbound.db")
	sq, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("открытие БД: %v", err)
	}
	defer sq.Close()

	srv := httptest.NewServer(eventsHandler(sq, testClock, ""))
	defer srv.Close()

	// Имя КИРИЛЛИЧЕСКОЕ, percent-кодировано (ловит регресс URL-декода/нормализации).
	code, body := httpPost(t, srv.URL+"/events/"+url.PathEscape("падение_выручки"),
		`{"клиент":"ООО"}`, nil)
	if code != http.StatusAccepted {
		t.Fatalf("код = %d, хотим 202; тело=%q", code, body)
	}
	if want := "событие e-000001 'падение_выручки' принято\n"; body != want {
		t.Errorf("ack = %q, хотим %q", body, want)
	}

	// Тик демона над ТЕМ ЖЕ Store → тело триггера срабатывает с payload-значениями.
	src := "когда событие падение_выручки:\n    печать(событие.клиент)\n"
	var out bytes.Buffer
	prog := parseServeSrc(t, src)
	d, dcode := buildServeDaemon(prog, sq, 5*time.Millisecond, 0, testClock, nil, &out, &out)
	if d == nil {
		t.Fatalf("buildServeDaemon nil, код=%d, out=%q", dcode, out.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }() // НЕ d.tick() — он неэкспортирован (пакет daemon)
	waitUntil(t, func() bool {
		evs, _ := sq.ListUnprocessedEvents()
		return len(evs) == 0
	})
	cancel()
	if rerr := <-done; rerr != nil {
		t.Fatalf("Run: %v", rerr)
	}
	if !strings.Contains(out.String(), "ООО") {
		t.Errorf("тело триггера не сработало с payload: out=%q", out.String())
	}
}

// --- FR-IE-3: эквивалентность HTTP-минта и emit-минта (одинаковая store.Event) ---

func TestInboundEnqueueEquivalentToEmit(t *testing.T) {
	stHTTP := store.NewMemoryStore()
	srv := httptest.NewServer(eventsHandler(stHTTP, testClock, ""))
	defer srv.Close()
	code, _ := httpPost(t, srv.URL+"/events/"+url.PathEscape("заявка_создана"), `{"клиент":"ООО"}`, nil)
	if code != http.StatusAccepted {
		t.Fatalf("HTTP код = %d, хотим 202", code)
	}

	stEmit := store.NewMemoryStore()
	if _, err := enqueueEvent(stEmit, "заявка_создана", `{"клиент":"ООО"}`, testClock); err != nil {
		t.Fatalf("enqueueEvent: %v", err)
	}

	a, _ := stHTTP.ListUnprocessedEvents()
	b, _ := stEmit.ListUnprocessedEvents()
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("по одному событию ожидалось: http=%d emit=%d", len(a), len(b))
	}
	if a[0].ID != b[0].ID || a[0].Name != b[0].Name || a[0].PayloadJSON != b[0].PayloadJSON {
		t.Errorf("события различны: http=%+v emit=%+v", a[0], b[0])
	}
	if !a[0].CreatedAt.Equal(b[0].CreatedAt) {
		t.Errorf("CreatedAt различны (не от общих часов): http=%v emit=%v", a[0].CreatedAt, b[0].CreatedAt)
	}
}

// --- FR-IE-7: битый JSON в теле → 202, тело всё равно исполняется ---

func TestInboundBrokenJSONAccepted(t *testing.T) {
	db := filepath.Join(t.TempDir(), "inbound.db")
	sq, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("открытие БД: %v", err)
	}
	defer sq.Close()
	srv := httptest.NewServer(eventsHandler(sq, testClock, ""))
	defer srv.Close()

	code, body := httpPost(t, srv.URL+"/events/тревога", `{битый`, nil)
	if code != http.StatusAccepted {
		t.Fatalf("битый JSON: код = %d, хотим 202 (приём НЕ валидирует JSON); тело=%q", code, body)
	}

	src := "когда событие тревога:\n    печать(\"сработал\")\n"
	var out bytes.Buffer
	prog := parseServeSrc(t, src)
	d, _ := buildServeDaemon(prog, sq, 5*time.Millisecond, 0, testClock, nil, &out, &out)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	waitUntil(t, func() bool {
		evs, _ := sq.ListUnprocessedEvents()
		return len(evs) == 0
	})
	cancel()
	<-done
	if !strings.Contains(out.String(), "сработал") {
		t.Errorf("тело не исполнилось при битом payload: out=%q", out.String())
	}
}

// --- FR-IE-10: метод/имя ---

func TestInboundMethodAndEmptyName(t *testing.T) {
	srv := httptest.NewServer(eventsHandler(store.NewMemoryStore(), testClock, ""))
	defer srv.Close()

	// GET → 405.
	resp, err := http.Get(srv.URL + "/events/x")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	gb, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET код = %d, хотим 405", resp.StatusCode)
	}
	if want := "ladix: метод не поддерживается, только POST\n"; string(gb) != want {
		t.Errorf("405 тело = %q, хотим %q", string(gb), want)
	}

	// POST /events/ (пустое имя) → 400.
	code, body := httpPost(t, srv.URL+"/events/", `{}`, nil)
	if code != http.StatusBadRequest {
		t.Errorf("пустое имя код = %d, хотим 400", code)
	}
	if want := "ladix: пустое имя события\n"; body != want {
		t.Errorf("400 тело = %q, хотим %q", body, want)
	}
}

// --- FR-IE-6: сбой Store при enqueue → 500, событие не теряется молча ---

func TestInboundStoreFailure500(t *testing.T) {
	mem := store.NewMemoryStore()
	srv := httptest.NewServer(eventsHandler(failEnqueueStore{mem}, testClock, ""))
	defer srv.Close()

	code, body := httpPost(t, srv.URL+"/events/x", `{}`, nil)
	if code != http.StatusInternalServerError {
		t.Fatalf("код = %d, хотим 500", code)
	}
	if want := "ladix: сбой хранилища\n"; body != want {
		t.Errorf("500 тело = %q, хотим %q", body, want)
	}
	// 202 НЕ должен возвращаться до успешного enqueue: события в очереди нет.
	if evs, _ := mem.ListUnprocessedEvents(); len(evs) != 0 {
		t.Errorf("событие просочилось в очередь при сбое: %d", len(evs))
	}
}

// --- FR-IE-9: опциональная аутентификация ---

func TestInboundAuth(t *testing.T) {
	srv := httptest.NewServer(eventsHandler(store.NewMemoryStore(), testClock, "СЕКРЕТ"))
	defer srv.Close()

	// Без заголовка → 401.
	if code, body := httpPost(t, srv.URL+"/events/x", `{}`, nil); code != http.StatusUnauthorized {
		t.Errorf("без токена код = %d, хотим 401; тело=%q", code, body)
	} else if want := "ladix: неверный токен\n"; body != want {
		t.Errorf("401 тело = %q, хотим %q", body, want)
	}
	// Неверный токен → 401.
	if code, _ := httpPost(t, srv.URL+"/events/x", `{}`, map[string]string{"X-Ladix-Token": "мимо"}); code != http.StatusUnauthorized {
		t.Errorf("неверный токен код = %d, хотим 401", code)
	}
	// Верный токен → 202.
	if code, _ := httpPost(t, srv.URL+"/events/x", `{}`, map[string]string{"X-Ladix-Token": "СЕКРЕТ"}); code != http.StatusAccepted {
		t.Errorf("верный токен код = %d, хотим 202", code)
	}

	// Без --token: auth выключен → любой POST 202.
	srv2 := httptest.NewServer(eventsHandler(store.NewMemoryStore(), testClock, ""))
	defer srv2.Close()
	if code, _ := httpPost(t, srv2.URL+"/events/x", `{}`, nil); code != http.StatusAccepted {
		t.Errorf("auth выкл: код = %d, хотим 202", code)
	}
}

// --- FR-IE-8: нет утечки горутин приёмника (Shutdown + join) ---

func TestInboundListenerNoLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	stop := startEventListener(ln, store.NewMemoryStore(), testClock, "")
	// Один POST, чтобы сервер реально обслужил соединение.
	if code, _ := httpPost(t, "http://"+ln.Addr().String()+"/events/x", `{}`, nil); code != http.StatusAccepted {
		t.Fatalf("POST на listener код = %d, хотим 202", code)
	}
	stop() // Shutdown + wg.Wait
	waitGoroutines(t, before)
}

// --- FR-IE-4: --listen требует --db (exit 2, ДО открытия сокета) ---

func TestServeListenRequiresDB(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"serve", filepath.Join("testdata", "inbound.ladix"), "--listen", "127.0.0.1:0"}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("код = %d, хотим 2; stderr=%q", code, errBuf.String())
	}
	if want := "ladix: --listen требует --db\n"; errBuf.String() != want {
		t.Errorf("stderr = %q, хотим %q", errBuf.String(), want)
	}
}

// --- FR-IE-5: занятый порт → exit 2 (bind-ошибка) ---

func TestServeListenBindError(t *testing.T) {
	ln0, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("предварительный net.Listen: %v", err)
	}
	defer ln0.Close()
	addr := ln0.Addr().String()

	db := filepath.Join(t.TempDir(), "serve.db")
	var out, errBuf bytes.Buffer
	code := realMain([]string{"serve", filepath.Join("testdata", "inbound.ladix"), "--db", db, "--listen", addr}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("код = %d, хотим 2; stderr=%q", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "ladix: не удалось открыть сокет '"+addr+"'") {
		t.Errorf("stderr = %q, хотим префикс «не удалось открыть сокет '%s'»", errBuf.String(), addr)
	}
}

// --- §IE-3 / R7: классификация loopback (граница предупреждения) ---

func TestIsLoopbackListen(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:8080", true},
		{"[::1]:8080", true},
		{"localhost:8080", true},
		{"0.0.0.0:8080", false},
		{":8080", false},
		{"1.2.3.4:8080", false},
		{"мусор", false},
	}
	for _, c := range cases {
		if got := isLoopbackListen(c.addr); got != c.want {
			t.Errorf("isLoopbackListen(%q) = %v, хотим %v", c.addr, got, c.want)
		}
	}
}
