package daemon

import (
	stderrors "errors"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/store"
)

// restartScanStatuses — залипшие статусы, обходимые рестарт-сканом при подъёме демона
// (решение #6, FR-019): «выполняется» (активный шаг прерван) и «создан» (первый шаг
// ещё не активирован). «ожидает» НЕ сканируется (корректен — проснётся по complete);
// терминалы (выполнен/провален/отменён) — тоже. Порядок статусов детерминирован.
var restartScanStatuses = []store.Status{store.StatusRunning, store.StatusCreated}

// RunRestartScan восстанавливает залипшие инстансы при подъёме serve, ДО первого тика
// (решение #6, FR-019/020, tick-contract.md §рестарт-скан). Для каждого залипшего
// статуса листит инстансы (ListInstancesByStatus, по возрастанию ID — детерминизм) и
// пытается реактивировать каждый через eng.ReactivateInstance:
//   - успех → инстанс догнан до ожидания/терминала (advance, at-least-once);
//   - дрейф (ErrInstanceDrift: шаг/процесс отсутствует в перезагруженном определении) →
//     лог расхождения, инстанс ОСТАВЛЕН залипшим (шаг НЕ угадывается, Принцип IX);
//   - прочая ошибка (runtime/Store) → лог, инстанс пропущен.
//
// Подъём демона НЕ прерывается ни одним инстансом (US5 №2): сбой ListInstancesByStatus
// логируется и сканирование статуса пропускается; ошибка отдельного инстанса не роняет
// скан. Под d.mu (как tick): рестарт-скан и тики не пересекаются по инстансу (EM-11).
func (d *Daemon) RunRestartScan() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, status := range restartScanStatuses {
		insts, err := d.st.ListInstancesByStatus(string(status))
		if err != nil {
			d.logf("рестарт-скан: сбой листинга инстансов статуса '%s': %s", status, err.Error())
			continue
		}
		for _, inst := range insts {
			d.reactivate(inst)
		}
	}
}

// reactivate реактивирует один залипший инстанс с изоляцией дрейфа и ошибок (FR-020).
// Дрейф → лог расхождения + инстанс залипает; прочая ошибка → лог + пропуск; успех —
// тихо (advance печатает строки §EN-7 сам). Никогда не паникует наружу/не роняет старт.
func (d *Daemon) reactivate(inst *store.ProcessInstance) {
	err := d.eng.ReactivateInstance(inst)
	if err == nil {
		return
	}
	if stderrors.Is(err, engine.ErrInstanceDrift) {
		d.logf("рестарт-скан: инстанс %s залип — шаг '%s' процесса '%s' не найден в определении (дрейф исходника), пропущен",
			inst.ID, inst.CurrentStep, inst.ProcessName)
		return
	}
	d.logf("рестарт-скан: инстанс %s не реактивирован: %s", inst.ID, err.Error())
}
