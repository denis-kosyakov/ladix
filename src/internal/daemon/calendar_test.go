package daemon

import (
	"testing"
	"time"
)

// TestShiftEveryFixed — фикс-множитель (сек/мин/час/дн): next = last + amount*unit,
// независимо от календаря (R-6, FR-012). День здесь — ровно 24 часа.
func TestShiftEveryFixed(t *testing.T) {
	last := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	cases := []struct {
		name   string
		amount int64
		unit   string
		want   time.Time
	}{
		{"30сек", 30, "сек", last.Add(30 * time.Second)},
		{"1мин", 1, "мин", last.Add(time.Minute)},
		{"30мин", 30, "мин", last.Add(30 * time.Minute)},
		{"1час", 1, "час", last.Add(time.Hour)},
		{"2час", 2, "час", last.Add(2 * time.Hour)},
		{"1дн", 1, "дн", last.Add(24 * time.Hour)},
		{"3дн", 3, "дн", last.Add(3 * 24 * time.Hour)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shiftEvery(last, c.amount, c.unit); !got.Equal(c.want) {
				t.Fatalf("shiftEvery(%v,%d,%q) = %v, хотим %v", last, c.amount, c.unit, got, c.want)
			}
		})
	}
}

// TestShiftEveryWeek — нед = AddDate(0,0,7*amount), календарный (R-6).
func TestShiftEveryWeek(t *testing.T) {
	last := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	if got, want := shiftEvery(last, 1, "нед"), last.AddDate(0, 0, 7); !got.Equal(want) {
		t.Fatalf("каждые 1 неделя: %v, хотим %v", got, want)
	}
	if got, want := shiftEvery(last, 2, "нед"), last.AddDate(0, 0, 14); !got.Equal(want) {
		t.Fatalf("каждые 2 недели: %v, хотим %v", got, want)
	}
}

// TestShiftEveryMonthClamp — мес: календарный сдвиг с ЗАЖИМОМ конца месяца (SC-004).
// 31 янв +1 мес → 28 фев (невисокосный) / 29 фев (високосный); 31 мар +1 мес → 30 апр;
// обычный день не зажимается; время суток сохраняется.
func TestShiftEveryMonthClamp(t *testing.T) {
	cases := []struct {
		name   string
		last   time.Time
		amount int64
		want   time.Time
	}{
		{
			"31янв+1мес→28фев (2026 невисокосный)",
			time.Date(2026, 1, 31, 9, 0, 0, 0, time.UTC), 1,
			time.Date(2026, 2, 28, 9, 0, 0, 0, time.UTC),
		},
		{
			"31янв+1мес→29фев (2024 високосный)",
			time.Date(2024, 1, 31, 9, 0, 0, 0, time.UTC), 1,
			time.Date(2024, 2, 29, 9, 0, 0, 0, time.UTC),
		},
		{
			"31мар+1мес→30апр",
			time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC), 1,
			time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC),
		},
		{
			"15июн+1мес→15июл (без зажима)",
			time.Date(2026, 6, 15, 8, 30, 0, 0, time.UTC), 1,
			time.Date(2026, 7, 15, 8, 30, 0, 0, time.UTC),
		},
		{
			"31дек+2мес→28фев(след.год)",
			time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC), 2,
			time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			"31янв+1мес сохраняет наносекунды/время",
			time.Date(2026, 1, 31, 23, 59, 58, 123, time.UTC), 1,
			time.Date(2026, 2, 28, 23, 59, 58, 123, time.UTC),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shiftEvery(c.last, c.amount, "мес"); !got.Equal(c.want) {
				t.Fatalf("shiftEvery(%v,%d,мес) = %v, хотим %v", c.last, c.amount, got, c.want)
			}
		})
	}
}

// TestShiftEveryWeekBoundaries — нед-сдвиг через границу МЕСЯЦА и ГОДА (R-6):
// AddDate(0,0,7*amount) корректно переносит дни в следующий месяц/год (Go нормализует).
func TestShiftEveryWeekBoundaries(t *testing.T) {
	cases := []struct {
		name   string
		last   time.Time
		amount int64
		want   time.Time
	}{
		{
			"29янв+1нед→5фев (граница месяца)",
			time.Date(2026, 1, 29, 10, 0, 0, 0, time.UTC), 1,
			time.Date(2026, 2, 5, 10, 0, 0, 0, time.UTC),
		},
		{
			"25дек+1нед→1янв (граница года)",
			time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC), 1,
			time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			"18дек+3нед→8янв (3·7=21дн через границу года)",
			time.Date(2025, 12, 18, 0, 0, 0, 0, time.UTC), 3,
			time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shiftEvery(c.last, c.amount, "нед"); !got.Equal(c.want) {
				t.Fatalf("shiftEvery(%v,%d,нед) = %v, хотим %v", c.last, c.amount, got, c.want)
			}
		})
	}
}

// TestShiftEveryMonthYearBoundary — мес-сдвиг через границу ГОДА (R-6): дек→янв сохраняет
// день (у января он есть), год инкрементируется; от 30 ноя +3 мес зажимается к 28 фев.
func TestShiftEveryMonthYearBoundary(t *testing.T) {
	cases := []struct {
		name   string
		last   time.Time
		amount int64
		want   time.Time
	}{
		{
			"15дек+1мес→15янв (граница года)",
			time.Date(2025, 12, 15, 9, 0, 0, 0, time.UTC), 1,
			time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC),
		},
		{
			"31дек+1мес→31янв (у января 31 день, без зажима)",
			time.Date(2025, 12, 31, 9, 0, 0, 0, time.UTC), 1,
			time.Date(2026, 1, 31, 9, 0, 0, 0, time.UTC),
		},
		{
			"30ноя+3мес→28фев (след.год, зажим)",
			time.Date(2025, 11, 30, 0, 0, 0, 0, time.UTC), 3,
			time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shiftEvery(c.last, c.amount, "мес"); !got.Equal(c.want) {
				t.Fatalf("shiftEvery(%v,%d,мес) = %v, хотим %v", c.last, c.amount, got, c.want)
			}
		})
	}
}
