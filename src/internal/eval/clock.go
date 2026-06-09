package eval

import (
	"time"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// Clock — инъекция времени (§SM-7, D-7). Метрики с период: считаются относительно
// сегодня(); прямой time.Now() в eval/движке запрещён — только через Clock. Это
// делает golden детерминированным (FixedClock в тестах, SystemClock в проде).
type Clock interface {
	Now() value.Дата
}

// SystemClock — продовый Clock: time.Now() в локальной зоне, усечённый до Y/M/D.
// Это ЕДИНСТВЕННЫЙ легальный вызов time.Now() во всей цепочке eval/движка (SC-006).
// Таймзоны — v2 (SPEC §12); для v1 «сегодня» = календарная дата time.Now() в Local.
type SystemClock struct{}

// Now возвращает текущую календарную дату (без времени) в локальной зоне.
func (SystemClock) Now() value.Дата {
	t := time.Now().In(time.Local)
	return value.Дата{Year: t.Year(), Month: int(t.Month()), Day: t.Day()}
}

// FixedClock — детерминированный Clock для тестов и golden-приёмки (§SM-10).
// Now() всегда возвращает D (например Дата{2026,5,31}).
type FixedClock struct {
	D value.Дата
}

// Now возвращает фиксированную дату D.
func (c FixedClock) Now() value.Дата { return c.D }
