package daemon

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/store"
)

// TestRunGracefulShutdown — Run(ctx) с отменяемым ctx возвращается оперативно по
// cancel() БЕЗ утечки горутин (FR-003, SC-007). Сравниваем runtime.NumGoroutine
// до/после (с допуском на стабилизацию рантайма) — тикер-горутина не должна остаться.
func TestRunGracefulShutdown(t *testing.T) {
	path := fixturePath(t)
	out := &countWriter{marker: "FIRE"}
	st := store.NewMemoryStore()
	writeFixture(t, path, `[{"x":1}]`)
	d, _ := buildDaemon(t, metricSrc(path, "FIRE", 10), st, out)
	// Короткий интервал, чтобы тикер реально срабатывал во время теста.
	d.interval = 5 * time.Millisecond

	before := stableGoroutines()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	// Дать демону покрутиться несколько тиков.
	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run вернул ошибку: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run не вернулся оперативно после cancel() — возможна утечка/блокировка")
	}

	after := stableGoroutines()
	if after > before {
		t.Fatalf("утечка горутин: до=%d после=%d", before, after)
	}
}

// TestRunZeroIntervalDefaultsToMinute — конструктор защищает от interval ≤ 0
// (дефолт 1m), чтобы NewTicker не паниковал.
func TestRunZeroIntervalDefaultsToMinute(t *testing.T) {
	path := fixturePath(t)
	out := &countWriter{marker: "FIRE"}
	writeFixture(t, path, `[{"x":1}]`)
	d, _ := buildDaemon(t, metricSrc(path, "FIRE", 10), store.NewMemoryStore(), out)
	d2 := New(d.st, d.eng, d.interp, d.clock, 0, out)
	if d2.interval != time.Minute {
		t.Fatalf("interval при 0 = %v, хотим 1m", d2.interval)
	}
}

// stableGoroutines — счётчик горутин после короткой стабилизации рантайма (GC +
// планировщик догоняют завершившиеся горутины). Снимает флак NumGoroutine.
func stableGoroutines() int {
	prev := -1
	for i := 0; i < 50; i++ {
		runtime.GC()
		n := runtime.NumGoroutine()
		if n == prev {
			return n
		}
		prev = n
		time.Sleep(5 * time.Millisecond)
	}
	return runtime.NumGoroutine()
}
