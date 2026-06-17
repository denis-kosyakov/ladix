package daemon

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/store"
)

// Тесты демона — внутри пакета daemon (а не daemon_test): по R-10 они дёргают
// ПРИВАТНЫЙ d.tick() напрямую (без настоящего time.Ticker), с управляемыми часами и
// продвижением источника-фикстуры между тиками. Экспортного Tick() в проде нет —
// тик приватен, наружу только Run(ctx).

// fixedClock — детерминированные часы планировщика (engine.Clock). Момент
// фиксирован/продвигается тестом; конкурентно-безопасен (Run крутит в горутине).
type fixedClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fixedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fixedClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// countWriter — io.Writer, считающий вхождения маркера (детект факта срабатывания
// тела триггера: тело печатает маркер через печать(...)). Конкурентно-безопасен.
type countWriter struct {
	mu     sync.Mutex
	marker string
	buf    bytes.Buffer
}

func (w *countWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *countWriter) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Count(w.buf.String(), w.marker)
}

func (w *countWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func (w *countWriter) contains(sub string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Contains(w.buf.String(), sub)
}

// reset очищает буфер (отбросить системные строки движка, напечатанные при setup-
// eng.Start, до первого тика — чтобы порядок-замок видел только вывод фаз тика).
func (w *countWriter) reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Reset()
}

// panicStore оборачивает Store и паникует на NextInstanceID (моделирует сбой тела
// триггера, чьё «запустить процесс» доходит до движка): per-триггер recover демона
// должен изолировать панику. Прочие методы делегируются.
type panicStore struct {
	store.Store
}

func (s *panicStore) NextInstanceID() (string, error) {
	panic("инъецированная паника тела триггера")
}

// writeFixture пишет JSON-источник во временный файл (фикстура метрики).
func writeFixture(t *testing.T, path, json string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(json), 0o644); err != nil {
		t.Fatalf("запись фикстуры %s: %v", path, err)
	}
}

// buildDaemon компилирует исходник и собирает daemon-стек поверх данного Store с
// фиксированными часами планировщика. interp использует eval.SystemClock (дата
// метрик не важна: источники-фикстуры без окна периода, R-10).
func buildDaemon(t *testing.T, src string, st store.Store, out *countWriter) (*Daemon, *fixedClock) {
	t.Helper()
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные лекс/синт ошибки: %s", errList.Error())
	}
	clk := &fixedClock{t: time.Date(2026, 5, 31, 12, 0, 0, 0, time.Local)}
	interp := eval.NewInterpreter(out, 0, eval.SystemClock{})
	eng := engine.NewEngine(st, interp, out, engine.WithClock(clk))
	interp.SetProcessRuntime(eng)
	if err := interp.Analyze(prog); err != nil {
		t.Fatalf("неожиданная семантическая ошибка: %s", err.Error())
	}
	if err := interp.Run(prog); err != nil {
		t.Fatalf("interp.Run: %s", err.Error())
	}
	d := New(st, eng, interp, clk, time.Minute, out)
	return d, clk
}

// metricSrc собирает программу: источник из файла path, метрика сумма(x), один
// метрика-триггер «метрика m > порог» с телом печать(marker).
func metricSrc(path, marker string, threshold int) string {
	return "источник s:\n    файл: \"" + path + "\"\n" +
		"метрика m:\n    источник: s\n    агрегат: сумма(x)\n" +
		"когда метрика m > " + strconv.Itoa(threshold) + ":\n    печать(\"" + marker + "\")\n"
}

// fixturePath возвращает путь к временной JSON-фикстуре теста.
func fixturePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "data.json")
}
