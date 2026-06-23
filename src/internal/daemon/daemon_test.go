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

// TestRunImmediateFirstTick — Run выполняет первый тик НЕМЕДЛЕННО в t=0 (CONC-3), а не
// через полный --interval: событие, уже лежащее в очереди ДО старта, дренится сразу, а
// метрика-edge праймится без фаера. Интервал намеренно БОЛЬШОЙ (10s) — если бы первый
// тик ждал тикер, ни дрена, ни прайминг не случились бы в окне теста (2s).
func TestRunImmediateFirstTick(t *testing.T) {
	path := fixturePath(t)
	out := &countWriter{marker: "EV"}
	st := store.NewMemoryStore()
	// Метрика ИСТИНА на старте (сумма=30 > порог 10): первый тик ПРАЙМИТ её без фаера
	// даже при истинном условии (FR-007). Так ассерт «MFIRE не печатался» различает
	// прайминг от ложного срабатывания: сломанный edge, фаерящий на cur==true, напечатал
	// бы MFIRE на первом же тике — корректный прайминг не печатает.
	writeFixture(t, path, `[{"x":30}]`)

	// trg-0 — событие-триггер «тик» (печать EV); trg-1 — метрика-триггер (печать MFIRE).
	src := "источник s:\n    файл: \"" + path + "\"\n" +
		"метрика m:\n    источник: s\n    агрегат: сумма(x)\n" +
		"когда событие тик:\n    печать(\"EV\")\n" +
		"когда метрика m > 10:\n    печать(\"MFIRE\")\n"
	d, _ := buildDaemon(t, src, st, out)

	// Событие УЖЕ в очереди до старта демона.
	enqueue(t, st, "тик", `{}`, time.Unix(100, 0))

	// Большой интервал: первый тик через тикер случился бы лишь через 10s.
	d.interval = 10 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	// Немедленная дрена в t=0: тело событие-триггера печатает EV задолго до 10s.
	deadline := time.Now().Add(2 * time.Second)
	for out.count() == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("первый тик не выполнен немедленно в t=0: событие не сдренено, out=%q", out.String())
		}
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}

	// Прайминг метрики без фаера: MFIRE не печатался, ХОТЯ условие истинно (база заведена,
	// не сработала — прайминг не фаерит на cur==true, FR-007).
	if out.contains("MFIRE") {
		t.Fatalf("первый тик должен ПРАЙМИТЬ метрику без фаера в t=0 даже при истинном условии, out=%q", out.String())
	}
	// Метрика реально запраймлена первым тиком (контентный ключ метрики: LastBool записан).
	if ts, err := st.LoadTriggerState(trigKey(t, d, 1)); err != nil || ts.LastBool == nil {
		t.Fatalf("метрика не запраймлена немедленным первым тиком: ts=%+v err=%v", ts, err)
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
