// Package daemon — ядро демона `serve` Ladix (фича 007b, §EM-17). Над Store/Engine/
// Interpreter крутит периодический тик из трёх фаз (drainEvents → evalMetrics →
// checkSchedules), детектит фронт ложь→истина метрик с durable trigger_state
// (at-most-once), исполняет тела триггеров штатным движком 006 и грациозно
// останавливается по контексту.
//
// Принцип V (без глобального изменяемого состояния): всё состояние — поля Daemon;
// прогон тика сериализован d.mu (EM-11: тики не пересекаются по инстансу). Часы
// инъектируются (engine.Clock) — в тестах фиксированные, прод — SystemClock.
package daemon

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/store"
)

// Daemon — состояние демона serve (tick-contract.md §тип, Принцип V — без глобалов).
type Daemon struct {
	st       store.Store       // trigger_state, события, инстансы
	eng      *engine.Engine    // исполнение тела → Engine.Start/реактивация
	interp   *eval.Interpreter // реестр триггеров, метрики, ResetRunState
	clock    engine.Clock      // часы планировщика (time.Time): когда тикать, target/LastFire
	interval time.Duration     // период тика (--interval, дефолт 1m)
	mu       sync.Mutex        // сериализация прогона движка (EM-11): тики не пересекаются
	out      io.Writer         // системные строки/логи демона (русские, §VIII)
}

// New строит демон с явной инъекцией зависимостей (serveMain, слайс 5). Без скрытого
// глобального состояния (Принцип V). interval ≤ 0 трактуется как 1m (дефолт).
func New(st store.Store, eng *engine.Engine, interp *eval.Interpreter, clock engine.Clock, interval time.Duration, out io.Writer) *Daemon {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Daemon{
		st:       st,
		eng:      eng,
		interp:   interp,
		clock:    clock,
		interval: interval,
		out:      out,
	}
}

// logf печатает системную строку демона в d.out с переводом строки (русские тексты,
// §VIII). Единственный канал диагностики демона (без stack trace).
func (d *Daemon) logf(format string, args ...any) {
	if d.out == nil {
		return
	}
	fmt.Fprintf(d.out, format+"\n", args...)
}

// Run крутит цикл тиков с грациозной остановкой (FR-003, tick-contract.md §Run).
// Тикер на d.interval; отмена ctx (SIGINT/SIGTERM ловит serveMain через
// signal.NotifyContext) ловится в select МЕЖДУ тиками — полу-записанного состояния
// нет, т.к. tick() синхронен под d.mu. Тикер-горутина не утекает: defer ticker.Stop()
// + выход по ctx.Done() (SC-007). Возвращает nil при штатной остановке.
func (d *Daemon) Run(ctx context.Context) error {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			d.tick()
		}
	}
}
