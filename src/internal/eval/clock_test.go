package eval

import (
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// FixedClock детерминирован: Now() всегда возвращает заданную дату (§SM-7, CK-2).
func TestFixedClockDeterministic(t *testing.T) {
	c := FixedClock{D: value.Дата{Year: 2026, Month: 5, Day: 31}}
	want := value.Дата{Year: 2026, Month: 5, Day: 31}
	for k := 0; k < 3; k++ {
		if got := c.Now(); got != want {
			t.Fatalf("FixedClock.Now() = %+v, хотим %+v", got, want)
		}
	}
}

// Clock — интерфейс; и FixedClock, и SystemClock его реализуют (compile-time +
// runtime). SystemClock.Now() усечён до Y/M/D и согласован с time.Now() в Local.
func TestSystemClockTruncated(t *testing.T) {
	var _ Clock = FixedClock{}
	var _ Clock = SystemClock{}

	got := SystemClock{}.Now()
	now := time.Now().In(time.Local)
	want := value.Дата{Year: now.Year(), Month: int(now.Month()), Day: now.Day()}
	if got != want {
		t.Fatalf("SystemClock.Now() = %+v, хотим %+v (усечение до даты)", got, want)
	}
}
