package daemon

import (
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/store"
)

// setClock устанавливает абсолютный момент управляемых часов планировщика (тесты
// расписания требуют конкретных дат/времён, а не относительного advance).
func setClock(c *fixedClock, t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

// scheduleSrc собирает программу с ОДНИМ расписание-триггером и телом печать(marker).
// schedExpr — например `каждые 1дн` или `в "09:30"`.
func scheduleSrc(marker, schedExpr string) string {
	return "когда расписание " + schedExpr + ":\n    печать(\"" + marker + "\")\n"
}

// TestCheckSchedulesEveryDayPrimingAndFire — `каждые 1дн`: первая регистрация —
// ЯКОРЬ (без срабатывания), затем срабатывание при now>=LastFire+1дн; LastFire
// сдвигается на ФАКТ (дрейф не копится, FR-011).
func TestCheckSchedulesEveryDayPrimingAndFire(t *testing.T) {
	out := &countWriter{marker: "TICKDAY"}
	st := store.NewMemoryStore()
	d, clk := buildDaemon(t, scheduleSrc("TICKDAY", "каждые 1дн"), st, out)

	start := time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)
	setClock(clk, start)

	// Якорь: первое наблюдение НЕ срабатывает (FR-011).
	d.tick()
	if got := out.count(); got != 0 {
		t.Fatalf("якорь расписания: срабатываний = %d, хотим 0", got)
	}
	ts, err := st.LoadTriggerState("trg-0")
	if err != nil || ts.Kind != everyKind || ts.LastFire == nil {
		t.Fatalf("якорь должен записать schedule_every+LastFire: ts=%+v err=%v", ts, err)
	}
	if !ts.LastFire.Equal(start) {
		t.Fatalf("якорь LastFire=%v, хотим %v", ts.LastFire, start)
	}

	// Ещё рано (через 12 часов): 0 срабатываний.
	setClock(clk, start.Add(12*time.Hour))
	d.tick()
	if got := out.count(); got != 0 {
		t.Fatalf("до интервала: срабатываний = %d, хотим 0", got)
	}

	// Через 25 часов (>= 24): ровно 1 срабатывание, LastFire := now (факт).
	fireMoment := start.Add(25 * time.Hour)
	setClock(clk, fireMoment)
	d.tick()
	if got := out.count(); got != 1 {
		t.Fatalf("после интервала: срабатываний = %d, хотим 1", got)
	}
	ts, _ = st.LoadTriggerState("trg-0")
	if ts.LastFire == nil || !ts.LastFire.Equal(fireMoment) {
		t.Fatalf("дрейф: LastFire=%v, хотим %v (якорь = факт, не +1дн от старта)", ts.LastFire, fireMoment)
	}
}

// TestCheckSchedulesEveryHourMinuteFixed — фикс-множитель: `каждые 2 часа` и
// `каждые 30 минут` срабатывают по точному интервалу (не календарю).
func TestCheckSchedulesEveryHourMinuteFixed(t *testing.T) {
	t.Run("каждые 2час", func(t *testing.T) {
		out := &countWriter{marker: "H2"}
		st := store.NewMemoryStore()
		d, clk := buildDaemon(t, scheduleSrc("H2", "каждые 2час"), st, out)
		start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
		setClock(clk, start)
		d.tick() // якорь
		setClock(clk, start.Add(90*time.Minute))
		d.tick() // рано (1.5ч < 2ч)
		if got := out.count(); got != 0 {
			t.Fatalf("1.5ч < 2ч: %d, хотим 0", got)
		}
		setClock(clk, start.Add(2*time.Hour))
		d.tick()
		if got := out.count(); got != 1 {
			t.Fatalf("2ч: %d, хотим 1", got)
		}
	})
	t.Run("каждые 30мин", func(t *testing.T) {
		out := &countWriter{marker: "M30"}
		st := store.NewMemoryStore()
		d, clk := buildDaemon(t, scheduleSrc("M30", "каждые 30мин"), st, out)
		start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
		setClock(clk, start)
		d.tick() // якорь
		setClock(clk, start.Add(29*time.Minute))
		d.tick()
		if got := out.count(); got != 0 {
			t.Fatalf("29мин < 30мин: %d, хотим 0", got)
		}
		setClock(clk, start.Add(30*time.Minute))
		d.tick()
		if got := out.count(); got != 1 {
			t.Fatalf("30мин: %d, хотим 1", got)
		}
	})
}

// TestCheckSchedulesEveryWeekCalendar — `каждые 1 неделя`: срабатывание ровно через
// 7 календарных дней от якоря.
func TestCheckSchedulesEveryWeekCalendar(t *testing.T) {
	out := &countWriter{marker: "WK"}
	st := store.NewMemoryStore()
	d, clk := buildDaemon(t, scheduleSrc("WK", "каждые 1нед"), st, out)
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	setClock(clk, start)
	d.tick() // якорь
	setClock(clk, start.AddDate(0, 0, 6))
	d.tick()
	if got := out.count(); got != 0 {
		t.Fatalf("6 дней < недели: %d, хотим 0", got)
	}
	setClock(clk, start.AddDate(0, 0, 7))
	d.tick()
	if got := out.count(); got != 1 {
		t.Fatalf("7 дней = неделя: %d, хотим 1", got)
	}
}

// TestCheckSchedulesEveryMonthEndClamp — `каждые 1 месяц` от 31 января → срабатывание
// 28 февраля (зажим конца месяца, SC-004). Старт 31 янв; в 28 фев (<29) уже время.
func TestCheckSchedulesEveryMonthEndClamp(t *testing.T) {
	out := &countWriter{marker: "MO"}
	st := store.NewMemoryStore()
	d, clk := buildDaemon(t, scheduleSrc("MO", "каждые 1мес"), st, out)
	start := time.Date(2026, 1, 31, 9, 0, 0, 0, time.UTC) // 2026 невисокосный
	setClock(clk, start)
	d.tick() // якорь LastFire=31 янв

	// 27 фев 09:00 — ещё рано (next = зажим к 28 фев).
	setClock(clk, time.Date(2026, 2, 27, 9, 0, 0, 0, time.UTC))
	d.tick()
	if got := out.count(); got != 0 {
		t.Fatalf("27 фев < зажатого 28 фев: %d, хотим 0", got)
	}

	// 28 фев 09:00 — наступил зажатый конец месяца → срабатывание.
	setClock(clk, time.Date(2026, 2, 28, 9, 0, 0, 0, time.UTC))
	d.tick()
	if got := out.count(); got != 1 {
		t.Fatalf("28 фев (зажим 31 янв +1 мес): %d, хотим 1", got)
	}
}

// TestCheckSchedulesAtOncePerDay — `в "09:00"`: несколько тиков в один день → РОВНО
// 1 срабатывание; тик на следующий день → снова срабатывание (FR-013, SC-005).
func TestCheckSchedulesAtOncePerDay(t *testing.T) {
	out := &countWriter{marker: "AT"}
	st := store.NewMemoryStore()
	d, clk := buildDaemon(t, scheduleSrc("AT", `в "09:00"`), st, out)

	// 08:00 первого дня — до цели: 0.
	setClock(clk, time.Date(2026, 4, 10, 8, 0, 0, 0, time.UTC))
	d.tick()
	if got := out.count(); got != 0 {
		t.Fatalf("08:00 до 09:00: %d, хотим 0", got)
	}

	// 09:30 — после цели → 1 срабатывание; LastFiredDate=2026-04-10.
	setClock(clk, time.Date(2026, 4, 10, 9, 30, 0, 0, time.UTC))
	d.tick()
	if got := out.count(); got != 1 {
		t.Fatalf("09:30 первый раз: %d, хотим 1", got)
	}
	ts, err := st.LoadTriggerState("trg-0")
	if err != nil || ts.Kind != atKind || ts.LastFiredDate == nil || *ts.LastFiredDate != "2026-04-10" {
		t.Fatalf("AtSchedule persist: ts=%+v err=%v", ts, err)
	}

	// 12:00 того же дня — повтор в тот же день → 0 новых.
	setClock(clk, time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC))
	d.tick()
	if got := out.count(); got != 1 {
		t.Fatalf("повтор в тот же день: всего %d, хотим 1 (раз в сутки)", got)
	}

	// 09:05 следующего дня → снова срабатывание.
	setClock(clk, time.Date(2026, 4, 11, 9, 5, 0, 0, time.UTC))
	d.tick()
	if got := out.count(); got != 2 {
		t.Fatalf("следующий день: всего %d, хотим 2", got)
	}
}

// TestCheckSchedulesNoInjection — тело расписания исполняется БЕЗ инжекции спец-
// переменных «значение»/«событие» (injection{} в checkEvery/checkAt): обычное тело
// (печать литерала, чтение глобала) отрабатывает штатно и БЕЗ лога ошибки. Контекст-
// гарды 007a статически запрещают «значение»/«событие» вне их триггеров, поэтому
// расписание-тело их и не содержит — здесь проверяем чистое исполнение без спец-env.
func TestCheckSchedulesNoInjection(t *testing.T) {
	out := &countWriter{marker: "SCHED"}
	st := store.NewMemoryStore()
	// Глобал + тело расписания, читающее его (read-only поднятие через env-барьер).
	src := "пусть порог = 42\n" +
		"когда расписание каждые 1дн:\n    печать(\"SCHED\")\n    печать(порог)\n"
	d, clk := buildDaemon(t, src, st, out)
	start := time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)
	setClock(clk, start)
	d.tick() // якорь
	setClock(clk, start.Add(25*time.Hour))
	d.tick() // срабатывание: тело отрабатывает штатно, без спец-переменных

	if got := out.count(); got != 1 {
		t.Fatalf("расписание-тело: срабатываний = %d, хотим 1", got)
	}
	if out.contains("ошибка триггера") || out.contains("сбой триггера изолирован") {
		t.Fatalf("расписание-тело без инжекции должно отработать чисто, out=%q", out.String())
	}
	// Глобал прочитан в теле (read-only поднятие, env-барьер 007a): «42» напечатано.
	if !out.contains("42") {
		t.Fatalf("ожидали чтение глобала «порог»=42 в теле расписания, out=%q", out.String())
	}
}
