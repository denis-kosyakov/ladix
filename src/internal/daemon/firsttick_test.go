package daemon

import (
	stderrors "errors"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/store"
)

// TestFirstTickPrimesWithoutFire — 🔁 поведенчески-нейтральный первый тик на СВЕЖЕМ
// Store без предыдущего trigger_state (§FR-010): любой ключевой триггер на первом
// наблюдении только ПРАЙМИТ базовую линию, тело НЕ исполняется. Три подкейса —
// метрика-edge, schedule_every, schedule_at.
//
// 🔁 ИНВЕРСИОННЫЙ ЗАМОК: убрать miss-ветку прайминга в checkAt («miss && !now.Before
// (target)») → schedule_at (c) срабатывает на ПЕРВОМ промахе, когда время уже прошло →
// подкейс (c) краснеет (count==1 вместо 0). Зеркально для метрики/every.
func TestFirstTickPrimesWithoutFire(t *testing.T) {
	// (a) метрика истинна уже на первом тике → праймит LastBool=true, тело НЕ фаерит;
	// позже кромка истина→ложь→истина даёт срабатывание (FR-006/007/010).
	t.Run("метрика-edge праймит без фаера", func(t *testing.T) {
		path := fixturePath(t)
		out := &countWriter{marker: "FIRE"}
		st := store.NewMemoryStore()
		d, _ := buildDaemon(t, metricSrc(path, "FIRE", 10), st, out)

		// Первый тик при УЖЕ истинном условии (30 > 10): прайм, 0 срабатываний.
		writeFixture(t, path, `[{"x":30}]`)
		d.tick()
		if got := out.count(); got != 0 {
			t.Fatalf("первый тик (метрика истинна): срабатываний = %d, хотим 0 (прайм)", got)
		}
		ts, err := st.LoadTriggerState(trigKey(t, d, 0))
		if err != nil || ts.LastBool == nil || !*ts.LastBool {
			t.Fatalf("первый тик должен запраймить LastBool=true: ts=%+v err=%v", ts, err)
		}

		// Кромка ложь→истина после прайма даёт срабатывание (поведение не потеряно).
		writeFixture(t, path, `[{"x":1}]`)
		d.tick() // истина→ложь, ре-арм
		writeFixture(t, path, `[{"x":30}]`)
		d.tick() // ложь→истина
		if got := out.count(); got != 1 {
			t.Fatalf("кромка после прайма: срабатываний = %d, хотим 1", got)
		}
	})

	// (b) schedule_every уже «должен» на первом тике → якорит LastFire, НЕ фаерит.
	t.Run("schedule_every якорит без фаера", func(t *testing.T) {
		out := &countWriter{marker: "EV"}
		st := store.NewMemoryStore()
		d, clk := buildDaemon(t, scheduleSrc("EV", "каждые 1дн"), st, out)

		start := time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)
		setClock(clk, start)
		d.tick() // первое наблюдение: якорь, без фаера
		if got := out.count(); got != 0 {
			t.Fatalf("первый тик schedule_every: срабатываний = %d, хотим 0 (якорь)", got)
		}
		ts, err := st.LoadTriggerState(trigKey(t, d, 0))
		if err != nil || ts.Kind != everyKind || ts.LastFire == nil {
			t.Fatalf("первый тик должен заякорить LastFire: ts=%+v err=%v", ts, err)
		}
	})

	// (c) schedule_at с now>=target на первом тике → праймит LastFiredDate=today, НЕ
	// фаерит; тик на СЛЕДУЮЩИЙ день фаерит (поведение восстанавливается).
	t.Run("schedule_at праймит без фаера, фаерит назавтра", func(t *testing.T) {
		out := &countWriter{marker: "AT"}
		st := store.NewMemoryStore()
		d, clk := buildDaemon(t, scheduleSrc("AT", `в "09:00"`), st, out)

		// Первый тик в 09:30 (цель 09:00 уже прошла) → прайм, 0 срабатываний (FR-010).
		setClock(clk, time.Date(2026, 4, 10, 9, 30, 0, 0, time.UTC))
		d.tick()
		if got := out.count(); got != 0 {
			t.Fatalf("первый тик schedule_at (промах, время прошло): срабатываний = %d, хотим 0 (прайм)", got)
		}
		ts, err := st.LoadTriggerState(trigKey(t, d, 0))
		if err != nil || ts.Kind != atKind || ts.LastFiredDate == nil || *ts.LastFiredDate != "2026-04-10" {
			t.Fatalf("первый тик должен запраймить LastFiredDate=2026-04-10: ts=%+v err=%v", ts, err)
		}

		// Тик на следующий день после 09:00 → срабатывание (поведение не потеряно).
		setClock(clk, time.Date(2026, 4, 11, 9, 5, 0, 0, time.UTC))
		d.tick()
		if got := out.count(); got != 1 {
			t.Fatalf("следующий день: срабатываний = %d, хотим 1", got)
		}
	})
}

// TestKeysStoreParity — паритет Memory/SQLite (зеркало restart_test.go eachStore):
// одна и та же программа минтит ОДИН и тот же контентный ключ и даёт одинаковое
// прайм/фаер-поведение на обоих бэкендах Store. Durable-ключ не зависит от движка БД.
func TestKeysStoreParity(t *testing.T) {
	eachStore(t, func(t *testing.T, st store.Store) {
		path := fixturePath(t)
		out := &countWriter{marker: "FIRE"}
		d, _ := buildDaemon(t, metricSrc(path, "FIRE", 10), st, out)

		// Контентный ключ детерминирован и одинаков для обоих Store (выровнен по trg-0).
		keys := buildTriggerKeys(d.interp.Triggers())
		if len(keys) != 1 || keys[0] == "" {
			t.Fatalf("ожидали один непустой контентный ключ, получили %v", keys)
		}

		// Прайм при лжи (3 > 10 == ложь): durable-состояние пишется под контентным ключом.
		writeFixture(t, path, `[{"x":1},{"x":2}]`)
		d.tick()
		if got := out.count(); got != 0 {
			t.Fatalf("прайм-тик: срабатываний = %d, хотим 0", got)
		}
		ts, err := st.LoadTriggerState(keys[0])
		if err != nil {
			t.Fatalf("durable-состояние не записано под контентным ключом %q: %v", keys[0], err)
		}
		if ts.LastBool == nil || *ts.LastBool {
			t.Fatalf("прайм при лжи: LastBool=%v, хотим false", ts.LastBool)
		}

		// Кромка ложь→истина → ровно одно срабатывание (поведение идентично на обоих).
		writeFixture(t, path, `[{"x":30}]`)
		d.tick()
		if got := out.count(); got != 1 {
			t.Fatalf("кромка ложь→истина: срабатываний = %d, хотим 1", got)
		}
		// Старое позиционное состояние «trg-0» не используется как durable-ключ метрики
		// (если контентный ключ ≠ "trg-0", позиционного слота быть не должно).
		if keys[0] != "trg-0" {
			if _, err := st.LoadTriggerState("trg-0"); !stderrors.Is(err, store.ErrTriggerStateNotFound) {
				t.Fatalf("позиционный slot trg-0 не должен существовать при контентном ключе %q, err=%v", keys[0], err)
			}
		}
	})
}
