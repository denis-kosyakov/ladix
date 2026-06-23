package daemon

// tick — один прогон демона: РОВНО ЧЕТЫРЕ фазы в СТРОГОМ порядке (FR-002, EM-17.1,
// 016 §AU-6.2.1). Под d.mu (EM-11: тики не пересекаются по инстансу). ResetRunState ДО
// фаз (решение #2, FR-005/024): без сброса метрика на следующем тике вернёт снимок
// старта и edge-детект молча мёртв (i.today фиксируется один раз, recordCache живёт
// «на запуск»). Порядок drainEvents → evalMetrics → checkSchedules → checkDeadlines
// детерминирован. 4-я фаза (checkDeadlines) аддитивна В ХВОСТ под тем же d.mu: НЕ
// меняет порядок/идемпотентность первых трёх (INV-1 007b).
func (d *Daemon) tick() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.interp.ResetRunState()
	d.drainEvents()    // фаза 1 (FR-016/017)
	d.evalMetrics()    // фаза 2 (FR-006…010)
	d.checkSchedules() // фаза 3 (FR-011…013)
	d.checkDeadlines() // фаза 4 (016 B4b, §AU-6.2.1) — эскалация просроченных задач
}

// drainEvents (фаза 1) реализована в events.go; checkSchedules (фаза 3) — в
// schedule.go; checkDeadlines (фаза 4) — в checkdeadlines.go. tick() хранит только
// оркестрацию и общую изоляцию safeFire.

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
