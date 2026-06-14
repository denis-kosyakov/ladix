package daemon

import "fmt"

// tick — один прогон демона: РОВНО три фазы в СТРОГОМ порядке (FR-002, EM-17.1).
// Под d.mu (EM-11: тики не пересекаются по инстансу). ResetRunState ДО фаз (решение
// #2, FR-005/024): без сброса метрика на следующем тике вернёт снимок старта и
// edge-детект молча мёртв (i.today фиксируется один раз, recordCache живёт «на
// запуск»). Порядок drainEvents → evalMetrics → checkSchedules детерминирован.
func (d *Daemon) tick() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.interp.ResetRunState()
	d.drainEvents()    // фаза 1 (FR-016/017)
	d.evalMetrics()    // фаза 2 (FR-006…010)
	d.checkSchedules() // фаза 3 (FR-011…013)
}

// drainEvents (фаза 1) реализована в events.go; checkSchedules (фаза 3) — в
// schedule.go. tick() хранит только оркестрацию и общую изоляцию safeFire.

// safeFire изолирует исполнение тела одного триггера (FR-004, EM-17.6): паника или
// рантайм-ошибка логируется по-русски (§VIII) и НЕ роняет тик/демон/прочие триггеры
// (SC-007). Без stack trace. Отдельный per-триггер recover ВНУТРИ тика — не отменяет
// CLI-границу guard() в serveMain (Принцип III).
func (d *Daemon) safeFire(fn func() error) {
	defer func() {
		if r := recover(); r != nil {
			d.logf("сбой триггера изолирован: %v", r)
		}
	}()
	if err := fn(); err != nil {
		d.logf("ошибка триггера: %s", err.Error())
	}
}

// triggerID — durable-ключ триггера по его 0-based индексу объявления (EM-17.2.1,
// FR-023): "trg-<N>", N — позиция в interp.Triggers() (порядок объявления).
func triggerID(idx int) string {
	return fmt.Sprintf("trg-%d", idx)
}
