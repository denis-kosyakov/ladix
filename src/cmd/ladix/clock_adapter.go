package main

import (
	"time"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// evalClockFromEngine адаптирует часы планировщика (engine.Clock, time.Time) к
// дневным часам интерпретатора (eval.Clock, value.Дата): усекает момент до
// календарной даты в локальной зоне — ровно как eval.SystemClock (clock.go).
// Так двойные часы (FR-024) едины: и движок процессов (engine.WithClock), и
// интерпретатор (NewInterpreter → пересчёт даты метрик в ResetRunState) читают
// дату ОТ ОДНИХ И ТЕХ ЖЕ инъектированных часов планировщика, а не из независимого
// eval.SystemClock. eval не импортирует engine (слой), потому адаптер живёт здесь,
// где видны оба интерфейса. Значение-обёртка без состояния (Принцип V).
//
// C4 (§C-4.2): вынесен из serve.go в общий файл cmd/ladix, чтобы run/start/
// complete/tasks/metric использовали ТОТ ЖЕ тип, что и serve (buildServeDaemon).
// Поведение байт-идентично прежнему serve.go-локальному объявлению.
type evalClockFromEngine struct{ c engine.Clock }

// Now снимает момент планировщика и усекает до Y/M/D в Local.
func (a evalClockFromEngine) Now() value.Дата {
	t := a.c.Now().In(time.Local)
	return value.Дата{Year: t.Year(), Month: int(t.Month()), Day: t.Day()}
}
