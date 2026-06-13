package daemon

import (
	stderrors "errors"
	"strconv"
	"strings"
	"time"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/store"
)

// Kind строк trigger_state расписание-триггеров (data-model TriggerState).
const (
	everyKind = "schedule_every"
	atKind    = "schedule_at"
)

// dateLayout — формат YYYY-MM-DD для LastFiredDate (раз-в-сутки якорь AtSchedule).
const dateLayout = "2006-01-02"

// checkSchedules — фаза 3 тика (FR-011…013, EM-17.4, tick-contract.md §фаза3).
// Обходит расписание-триггеры в порядке объявления (interp.Triggers()), исполняет
// тело по календарному графику с durable trigger_state. Часы — d.clock (планировщик,
// в тестах управляемые). Расписание НЕ инжектирует спец-переменную (ни «значение»,
// ни «событие»): тело исполняется как есть.
//
// EverySchedule (Kind="schedule_every"):
//   - промах trigger_state → ЯКОРЬ (FR-011): SaveTriggerState{LastFire:now}, НЕ
//     срабатывать (первое наблюдение — как прайминг метрик, 0 ложных фаеров);
//   - иначе next := shiftEvery(LastFire, amount, unit); если now >= next →
//     SaveTriggerState{LastFire:now} (факт срабатывания от now, дрейф НЕ копим) +
//     исполнить тело под per-триггер recover.
//
// AtSchedule (Kind="schedule_at"):
//   - today := now «YYYY-MM-DD»; target := today в ЧЧ:ММ (формат провалиден статически,
//     SE-TIME-FORMAT слайс 2 — здесь только парс «ЧЧ»/«ММ»);
//   - если (нет состояния ИЛИ LastFiredDate != today) И now >= target →
//     SaveTriggerState{LastFiredDate:today} + исполнить тело (раз в сутки, FR-013).
func (d *Daemon) checkSchedules() {
	now := d.clock.Now()
	for idx, td := range d.interp.Triggers() {
		spec, ok := td.Spec.(*ast.ScheduleTrigger)
		if !ok {
			continue // метрика/событие — другие фазы
		}
		id := triggerID(idx)
		switch sub := spec.Spec.(type) {
		case *ast.EverySchedule:
			d.checkEvery(id, sub, td.Body, now)
		case *ast.AtSchedule:
			d.checkAt(id, sub, td.Body, now)
		}
	}
}

// checkEvery обрабатывает подформу «каждые amount unit» (FR-011/012).
func (d *Daemon) checkEvery(id string, sub *ast.EverySchedule, body *ast.Block, now time.Time) {
	amount, ok := parseAmount(sub.Every.Amount)
	if !ok {
		// Статически невозможно (лексер нормализует число), но не падаем на тике.
		d.logf("триггер '%s': некорректная величина расписания «%s»", id, sub.Every.Amount)
		return
	}

	ts, loadErr := d.st.LoadTriggerState(id)
	if stderrors.Is(loadErr, store.ErrTriggerStateNotFound) {
		// ЯКОРЬ (FR-011): первое наблюдение — зафиксировать LastFire, НЕ срабатывать.
		t := now
		if saveErr := d.st.SaveTriggerState(&store.TriggerState{TriggerID: id, Kind: everyKind, LastFire: &t}); saveErr != nil {
			d.logf("триггер '%s': сбой записи trigger_state (якорь расписания): %s", id, saveErr.Error())
		}
		return
	}
	if loadErr != nil {
		d.logf("триггер '%s': сбой чтения trigger_state: %s", id, loadErr.Error())
		return
	}
	if ts.LastFire == nil {
		// Битое состояние (LastFire не записан) — переякорить, не срабатывать.
		t := now
		if saveErr := d.st.SaveTriggerState(&store.TriggerState{TriggerID: id, Kind: everyKind, LastFire: &t}); saveErr != nil {
			d.logf("триггер '%s': сбой записи trigger_state (переякорь): %s", id, saveErr.Error())
		}
		return
	}

	next := shiftEvery(*ts.LastFire, amount, sub.Every.Unit)
	if now.Before(next) {
		return // ещё рано
	}

	// Факт срабатывания: новый якорь = now (дрейф не копим — следующий next от now).
	t := now
	if saveErr := d.st.SaveTriggerState(&store.TriggerState{TriggerID: id, Kind: everyKind, LastFire: &t}); saveErr != nil {
		d.logf("триггер '%s': сбой записи trigger_state: %s", id, saveErr.Error())
		return
	}
	d.safeFire(func() error {
		return d.fireBody(body, injection{}) // расписание: без инжекции
	})
}

// checkAt обрабатывает подформу «в "ЧЧ:ММ"» — раз в сутки (FR-013).
func (d *Daemon) checkAt(id string, sub *ast.AtSchedule, body *ast.Block, now time.Time) {
	hh, mm, ok := parseHHMM(sub.At.Value)
	if !ok {
		// Формат провалиден статически (SE-TIME-FORMAT); защитный лог на случай дрейфа.
		d.logf("триггер '%s': некорректное время расписания «%s»", id, sub.At.Value)
		return
	}
	today := now.Format(dateLayout)
	target := time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, now.Location())

	ts, loadErr := d.st.LoadTriggerState(id)
	if loadErr != nil && !stderrors.Is(loadErr, store.ErrTriggerStateNotFound) {
		d.logf("триггер '%s': сбой чтения trigger_state: %s", id, loadErr.Error())
		return
	}
	alreadyToday := loadErr == nil && ts.LastFiredDate != nil && *ts.LastFiredDate == today
	if alreadyToday || now.Before(target) {
		return // уже срабатывал сегодня ИЛИ цель ещё не наступила
	}

	day := today
	if saveErr := d.st.SaveTriggerState(&store.TriggerState{TriggerID: id, Kind: atKind, LastFiredDate: &day}); saveErr != nil {
		d.logf("триггер '%s': сбой записи trigger_state: %s", id, saveErr.Error())
		return
	}
	d.safeFire(func() error {
		return d.fireBody(body, injection{}) // расписание: без инжекции
	})
}

// parseAmount разбирает нормализованную лексемой величину DurationLit в int64.
func parseAmount(s string) (int64, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// parseHHMM разбирает «ЧЧ:ММ» в часы/минуты (формат уже провалиден статически,
// SE-TIME-FORMAT слайс 2 — здесь только числовой парс с защитой диапазона).
func parseHHMM(s string) (hh, mm int, ok bool) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, 0, false
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}
