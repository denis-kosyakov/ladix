package daemon

import (
	stderrors "errors"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/store"
)

// metricKind — Kind строки trigger_state метрика-триггера (data-model TriggerState).
const metricKind = "metric"

// valueName — предопределённое имя, инжектируемое в тело метрика-триггера на момент
// срабатывания (§TR-5: «значение» = снимок метрики).
const valueName = "значение"

// evalMetrics — фаза 2 тика (FR-006…010, EM-17.2, tick-contract.md §фаза2). Обходит
// метрика-триггеры в порядке объявления (interp.Triggers()), детектит фронт
// ложь→истина по durable trigger_state с гарантией at-most-once (персист ДО тела).
//
// Для каждого метрика-триггера (TriggerID="trg-<N>", N — индекс по ВСЕМ триггерам):
//   - вычислить текущий булев cur (+ снимок) через interp.EvalMetricCondition;
//   - НЕ ok (метрика пуста / сравнение не-Булево / ошибка вычисления) → ЗАМОРОЗКА
//     (FR-009): ничего не персистить, тело не исполнять, продолжить;
//   - LoadTriggerState промах (ErrTriggerStateNotFound) → ПРАЙМИНГ (FR-007):
//     SaveTriggerState{LastBool:cur}, тело НЕ исполнять (0 ложных срабатываний);
//   - иначе fired := (LastBool==false && cur==true) (фронт ложь→истина, FR-006);
//     persist нового LastBool ДО тела (at-most-once, FR-008: краш после persist →
//     пропуск, не дубль; сбойный триггер не зацикливается); затем при fired —
//     исполнить тело со снимком «значение» под per-триггер recover (изоляция).
func (d *Daemon) evalMetrics() {
	for idx, td := range d.interp.Triggers() {
		spec, ok := td.Spec.(*ast.MetricTrigger)
		if !ok {
			continue // событие/расписание — другие фазы
		}
		id := triggerID(idx)

		cur, snapshot, computable, err := d.interp.EvalMetricCondition(spec)
		if err != nil {
			// Нештатная невычислимость (цикл метрик/сбой загрузки): логируем и
			// замораживаем — тик не падает (Принцип III, FR-009 трактовка).
			d.logf("триггер '%s': метрика не вычислена: %s", id, err.Error())
			continue
		}
		if !computable {
			continue // заморозка (FR-009): ничего не персистить, тело не исполнять
		}

		ts, loadErr := d.st.LoadTriggerState(id)
		if stderrors.Is(loadErr, store.ErrTriggerStateNotFound) {
			// ПРАЙМИНГ (FR-007): записать базовую линию, НЕ срабатывать (даже если cur==true).
			b := cur
			if saveErr := d.st.SaveTriggerState(&store.TriggerState{TriggerID: id, Kind: metricKind, LastBool: &b}); saveErr != nil {
				d.logf("триггер '%s': сбой записи trigger_state (прайминг): %s", id, saveErr.Error())
			}
			continue
		}
		if loadErr != nil {
			d.logf("триггер '%s': сбой чтения trigger_state: %s", id, loadErr.Error())
			continue
		}

		fired := ts.LastBool != nil && !*ts.LastBool && cur // фронт ложь→истина (FR-006)

		// persist ДО тела (at-most-once, FR-008): ре-арм базовой линии независимо от
		// исхода тела. Краш после persist → следующий тик не ре-файрит.
		b := cur
		if saveErr := d.st.SaveTriggerState(&store.TriggerState{TriggerID: id, Kind: metricKind, LastBool: &b}); saveErr != nil {
			d.logf("триггер '%s': сбой записи trigger_state: %s", id, saveErr.Error())
			continue
		}

		if fired {
			body := td.Body
			d.safeFire(func() error {
				return d.fireBody(body, injection{name: valueName, val: snapshot})
			})
		}
	}
}
