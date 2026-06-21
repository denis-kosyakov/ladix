package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// fixedClock — детерминированные часы планировщика для CLI golden (engine.Clock).
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// readFixture читает testdata-фикстуру относительно cmd/ladix.
func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("чтение фикстуры %s: %v", name, err)
	}
	return string(data)
}

// --- SE-TIME-FORMAT в serve: компиляция отклоняется, демон не стартует ---

// TestServeBadTimeFormat — файл serve с «в "25:99"» → двухстрочная диагностика
// SE-TIME-FORMAT в stderr, exit 1, демон НЕ стартует (serve-command.md §тесты, FR-014).
func TestServeBadTimeFormat(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"serve", filepath.Join("testdata", "bad_time.ladix")}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1; stderr=%q", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "Ошибка в строке") {
		t.Fatalf("ожидалась каноническая двухстрочная диагностика, stderr=%q", errBuf.String())
	}
	if strings.Contains(errBuf.String(), ".go:") || strings.Contains(errBuf.String(), "goroutine") {
		t.Fatalf("в stderr просочился Go stack trace: %q", errBuf.String())
	}
}

// --- Разбор флагов serve (без входа в блокирующий Run) ---

// TestServeFlagBadInterval — невалидный --interval → exit 2 (до сборки демона).
func TestServeFlagBadInterval(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"serve", "--interval", "тик", filepath.Join("testdata", "schedule.ladix")}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("код = %d, хотим 2; stderr=%q", code, errBuf.String())
	}
	// Exact-match канона: для формы `--interval <мусор>` достижимо РОВНО одно сообщение
	// (значение присутствует, парс не прошёл) — не «требует значение». Мутация текста в
	// serve.go роняет тест.
	if got, want := errBuf.String(), "ladix: неверное значение --interval\n"; got != want {
		t.Fatalf("stderr=%q, хотим %q", got, want)
	}
}

// TestServeFlagBadMaxDepth — невалидный --max-depth → exit 2.
func TestServeFlagBadMaxDepth(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := realMain([]string{"serve", "--max-depth", "0", filepath.Join("testdata", "schedule.ladix")}, &out, &errBuf); code != 2 {
		t.Fatalf("код = %d, хотим 2; stderr=%q", code, errBuf.String())
	}
}

// TestServeNoFile — нет файла → exit 2 (usage).
func TestServeNoFile(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := realMain([]string{"serve", "--interval", "5s"}, &out, &errBuf); code != 2 {
		t.Fatalf("код = %d, хотим 2", code)
	}
}

// TestServeUnknownFlag — неизвестный флаг → exit 2.
func TestServeUnknownFlag(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := realMain([]string{"serve", "--bogus", filepath.Join("testdata", "schedule.ladix")}, &out, &errBuf); code != 2 {
		t.Fatalf("код = %d, хотим 2", code)
	}
}

// TestServeMissingFile — несуществующий файл → exit 2 (ошибка чтения, использование).
func TestServeMissingFile(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := realMain([]string{"serve", filepath.Join("testdata", "нет-такого.ladix")}, &out, &errBuf); code != 2 {
		t.Fatalf("код = %d, хотим 2; stderr=%q", code, errBuf.String())
	}
}

// --- Интеграция emit → serve(drainEvents) → процесс создан, at-least-once ---

// TestServeEmitDrainFires — emit кладёт событие в --db; затем демон, собранный прод-путём
// buildServeDaemon на той же БД, дренит очередь коротким Run и исполняет тело
// (запускает процесс). Доказывает сквозняк emit→serve через общий Store (SC-006, US4).
func TestServeEmitDrainFires(t *testing.T) {
	db := filepath.Join(t.TempDir(), "serve.db")

	// emit заявка_создана '{"клиент":"ООО"}' → событие в очереди.
	var eo, ee bytes.Buffer
	if code := realMain([]string{"emit", "заявка_создана", `{"клиент":"ООО"}`, "--db", db}, &eo, &ee); code != 0 {
		t.Fatalf("emit: код=%d stderr=%q", code, ee.String())
	}

	// Собрать демон прод-путём на той же БД и продренить коротким Run.
	src := readFixture(t, "event.ladix")
	sq, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("открытие БД: %v", err)
	}
	defer sq.Close()
	var out bytes.Buffer
	prog := parseServeSrc(t, src)
	d, code := buildServeDaemon(prog, sq, 5*time.Millisecond, 0, "",
		fixedClock{time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local)}, nil, &out, &out)
	if d == nil {
		t.Fatalf("buildServeDaemon вернул nil, код=%d; out=%q", code, out.String())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	// Ждём, пока событие обработается (processed) — детерминированный сигнал доставки.
	waitUntil(t, func() bool {
		evs, _ := sq.ListUnprocessedEvents()
		return len(evs) == 0
	})
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}

	// Тело исполнено: процесс создан, его автоматический шаг напечатал маркер.
	if !strings.Contains(out.String(), "заявка принята от ООО") {
		t.Fatalf("тело событие-триггера не исполнено; out=%q", out.String())
	}
	insts, _ := sq.ListInstancesByStatus(string(store.StatusDone))
	if len(insts) != 1 {
		t.Fatalf("ожидался 1 завершённый инстанс, получено %d", len(insts))
	}
}

// --- Метрика-триггер через прод-путь serve: фикстура metric_edge.ladix ---

// TestServeMetricPrimingNoFalsePositive — метрика-триггер фикстуры metric_edge.ladix
// при УЖЕ истинном условии (сумма(вес)=15 > 10): первый тик = ПРАЙМИНГ без срабатывания
// (FR-007). Доказывает, что метрика-триггер компилируется и проходит через прод-путь
// serve (buildServeDaemon), а edge-детект не даёт ложного срабатывания на первом тике.
// Многотиковый edge-детект с мутацией источника — в daemon/metrics_test (прямой tick).
func TestServeMetricPrimingNoFalsePositive(t *testing.T) {
	dir := t.TempDir()
	srcJSON := filepath.Join(dir, "src.json")
	if err := os.WriteFile(srcJSON, []byte(`[{"вес":7},{"вес":8}]`), 0o644); err != nil {
		t.Fatalf("запись источника: %v", err)
	}
	src := strings.ReplaceAll(readFixture(t, "metric_edge.ladix"), "__SOURCE__", srcJSON)

	st := store.NewMemoryStore()
	var out bytes.Buffer
	prog := parseServeSrc(t, src)
	d, code := buildServeDaemon(prog, st, 5*time.Millisecond, 0, "",
		fixedClock{time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local)}, nil, &out, &out)
	if d == nil {
		t.Fatalf("buildServeDaemon вернул nil, код=%d; out=%q", code, out.String())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	// Ждём, пока триггер запраймится (LastBool записан) — детерминированный сигнал.
	waitUntil(t, func() bool {
		ts, err := st.LoadTriggerState("trg-0")
		return err == nil && ts.LastBool != nil
	})
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}

	// Прайминг: тело НЕ исполнено (нет процесса эскалация), несмотря на истинное условие.
	if strings.Contains(out.String(), "порог заявок превышен") {
		t.Fatalf("прайминг при cur==true дал ложное срабатывание; out=%q", out.String())
	}
	insts, _ := st.ListInstancesByStatus(string(store.StatusDone))
	if len(insts) != 0 {
		t.Fatalf("прайминг не должен создавать инстансы, получено %d", len(insts))
	}
}

// TestServeMetricDateFollowsSchedulerClock — двойные часы (FR-024): дата вычисления
// метрик интерпретатора, перевычисляемая в ResetRunState на каждом тике, идёт ОТ
// ЧАСОВ ПЛАНИРОВЩИКА (инъекция через buildServeDaemon → evalClockFromEngine), а НЕ
// от собственного eval.SystemClock интерпретатора.
//
// Доказательство: метрика с ОКОННЫМ периодом (ежемесячно + по_дате) над записями,
// датированными маем 2026. Часы планировщика = FixedClock 2026-05-31 → окно = май
// 2026 → записи в окне → сумма=100 > 0 → метрика ИСТИНА → прайминг сохраняет
// LastBool=true. Если бы дата бралась из системных часов интерпретатора (сегодня —
// НЕ май 2026), майские записи выпали бы из окна → сумма=0 → метрика ЛОЖЬ →
// LastBool=false. Так LastBool==true однозначно различает «дата от планировщика».
func TestServeMetricDateFollowsSchedulerClock(t *testing.T) {
	dir := t.TempDir()
	srcJSON := filepath.Join(dir, "src.json")
	// Записи датированы маем 2026 (в окне FixedClock 2026-05-31, вне любого иного месяца).
	if err := os.WriteFile(srcJSON,
		[]byte(`[{"дата_заказа":"2026-05-10","сумма_заказа":60},{"дата_заказа":"2026-05-20","сумма_заказа":40}]`),
		0o644); err != nil {
		t.Fatalf("запись источника: %v", err)
	}
	src := strings.ReplaceAll(readFixture(t, "metric_dated.ladix"), "__SOURCE__", srcJSON)

	st := store.NewMemoryStore()
	var out bytes.Buffer
	prog := parseServeSrc(t, src)
	d, code := buildServeDaemon(prog, st, 5*time.Millisecond, 0, "",
		fixedClock{time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local)}, nil, &out, &out)
	if d == nil {
		t.Fatalf("buildServeDaemon вернул nil, код=%d; out=%q", code, out.String())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	// Ждём прайминга (LastBool записан первым тиком).
	waitUntil(t, func() bool {
		ts, err := st.LoadTriggerState("trg-0")
		return err == nil && ts.LastBool != nil
	})
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}

	ts, err := st.LoadTriggerState("trg-0")
	if err != nil || ts.LastBool == nil {
		t.Fatalf("trigger_state не записан: err=%v ts=%+v", err, ts)
	}
	if !*ts.LastBool {
		t.Fatalf("LastBool=false: майские записи выпали из окна — дата метрик НЕ от " +
			"часов планировщика (FR-024 нарушен: интерпретатор использует свои часы, не " +
			"инъектированные)")
	}
}

// --- Рестарт-скан через прод-путь сборки ---

// TestServeRestartScanLiftsStuck — залипший инстанс «выполняется» с валидным шагом в
// БД → демон, собранный прод-путём, реактивирует его при RunRestartScan (US5, SC-008).
func TestServeRestartScanLiftsStuck(t *testing.T) {
	db := filepath.Join(t.TempDir(), "serve.db")
	sq, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("открытие БД: %v", err)
	}
	defer sq.Close()
	// Фабрикуем залипший инстанс процесса обработать_заявку на первом шаге.
	inst := &store.ProcessInstance{
		ID:          "p-000001",
		ProcessName: "обработать_заявку",
		Status:      store.StatusRunning,
		CurrentStep: "зафиксировать",
		Variables:   map[string]value.Value{"клиент": value.Строка{V: "ООО"}},
		CreatedAt:   time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local),
	}
	if err := sq.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}

	src := readFixture(t, "event.ladix")
	var out bytes.Buffer
	prog := parseServeSrc(t, src)
	d, _ := buildServeDaemon(prog, sq, time.Minute, 0, "",
		fixedClock{time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local)}, nil, &out, &out)
	if d == nil {
		t.Fatalf("buildServeDaemon вернул nil; out=%q", out.String())
	}

	d.RunRestartScan()

	got, _ := sq.LoadInstance("p-000001")
	if got.Status != store.StatusDone {
		t.Fatalf("залипший инстанс не догнан рестарт-сканом: статус=%q", got.Status)
	}
	if !strings.Contains(out.String(), "заявка принята от ООО") {
		t.Fatalf("тело шага не исполнено при реактивации; out=%q", out.String())
	}
}

// --- Грациозная остановка без утечки горутин ---

// TestServeGracefulShutdownNoLeak — демон, собранный прод-путём, запущен Run(ctx) в
// горутине; cancel() → Run возвращает nil без утечки тикер-горутины (SC-007, FR-003).
func TestServeGracefulShutdownNoLeak(t *testing.T) {
	src := readFixture(t, "schedule.ladix")
	var out bytes.Buffer
	prog := parseServeSrc(t, src)
	d, _ := buildServeDaemon(prog, store.NewMemoryStore(), 5*time.Millisecond, 0, "",
		fixedClock{time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local)}, nil, &out, &out)
	if d == nil {
		t.Fatalf("buildServeDaemon вернул nil; out=%q", out.String())
	}

	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()
	time.Sleep(30 * time.Millisecond) // дать тикеру стартовать/тикнуть
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}
	waitGoroutines(t, before)
}

// --- хелперы ---

// parseServeSrc лексирует+парсит исходник для buildServeDaemon (фикстуры обязаны быть
// чистыми по лексеру/парсеру).
func parseServeSrc(t *testing.T, src string) *ast.Program {
	t.Helper()
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные лекс/синт ошибки: %s", errList.Error())
	}
	return prog
}

// waitUntil опрашивает условие до 2s (детерминированный сигнал вместо фикс-сна).
func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("условие не выполнилось за отведённое время")
}

// waitGoroutines ждёт, пока число горутин стабилизируется к базовому (допуск на
// планировщик Go), иначе фейл (детект утечки тикер-горутины).
func waitGoroutines(t *testing.T, before int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("горутины не вернулись к базовому уровню: before=%d after=%d", before, runtime.NumGoroutine())
}

// TestServeBrokenDBExit2 — `serve --db <битый-путь>`: открытие SQLite вне guard →
// ошибка использования CLI, exit 2 (демон не стартует, открытие синхронно).
func TestServeBrokenDBExit2(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"serve", "--db", "нет/такого/каталога.db",
		filepath.Join("testdata", "schedule.ladix")}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("код = %d, хотим 2; stderr=%q", code, errBuf.String())
	}
	if !strings.HasPrefix(errBuf.String(), "ladix: не удалось открыть хранилище 'нет/такого/каталога.db': ") {
		t.Errorf("stderr=%q", errBuf.String())
	}
}

// TestServeInlineBrokenDBExit2 — то же через inline-форму `--db=<битый-путь>`
// (ветка strings.HasPrefix(a, "--db=")): exit 2, тот же префикс.
func TestServeInlineBrokenDBExit2(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"serve", "--db=нет/такого/каталога.db",
		filepath.Join("testdata", "schedule.ladix")}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("код = %d, хотим 2; stderr=%q", code, errBuf.String())
	}
	if !strings.HasPrefix(errBuf.String(), "ladix: не удалось открыть хранилище 'нет/такого/каталога.db': ") {
		t.Errorf("stderr=%q", errBuf.String())
	}
}

// TestServeInlineBadIntervalExit2 — inline-форма `--interval=<мусор>` (ветка
// strings.HasPrefix(a, "--interval=")): неверное значение → exit 2.
func TestServeInlineBadIntervalExit2(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"serve", "--interval=тик",
		filepath.Join("testdata", "schedule.ladix")}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("код = %d, хотим 2; stderr=%q", code, errBuf.String())
	}
	// Exact-match канона: inline-форма `--interval=<мусор>` достижима только сообщением
	// «неверное значение --interval» (serve.go:100). Мутация текста роняет тест.
	if got, want := errBuf.String(), "ladix: неверное значение --interval\n"; got != want {
		t.Errorf("stderr=%q, хотим %q", got, want)
	}
}

// TestServeTopLevelParseErrorExit1 — top-level лекс/синт ошибка в файле (ветка
// serve.go: !errList.Empty()) → exit 1, канонический двухстрочный Error() (§13),
// без утечки Go-стека в stderr. Дополняет TestServeBadTimeFormat (семантическая ветка).
func TestServeTopLevelParseErrorExit1(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"serve", filepath.Join("testdata", "сломанный.ladix")}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1; stderr=%q", code, errBuf.String())
	}
	if !strings.HasPrefix(errBuf.String(), "Ошибка в строке ") {
		t.Errorf("stderr=%q, хотим канон §13", errBuf.String())
	}
	if strings.Contains(errBuf.String(), ".go:") || strings.Contains(errBuf.String(), "goroutine") {
		t.Errorf("в stderr просочился Go stack trace: %q", errBuf.String())
	}
}
