package engine

import "time"

// AddDuration абсолютизирует срок (D-19, §EN-3): Deadline = base + срок. Единицы →
// Go-время: сек/мин/час/дн(24ч)/нед(168ч) — фиксированные множители time.Duration;
// мес — календарный AddDate(0, n, 0). Это внутренняя Go-механика дедлайна, НЕ
// Ladix-арифметика (clamp-семантика SPEC §4 не распространяется). Неизвестная
// единица возвращает base без изменения (защитно; лексер не порождает других).
func AddDuration(base time.Time, amount int64, unit string) time.Time {
	switch unit {
	case "сек":
		return base.Add(time.Duration(amount) * time.Second)
	case "мин":
		return base.Add(time.Duration(amount) * time.Minute)
	case "час":
		return base.Add(time.Duration(amount) * time.Hour)
	case "дн":
		return base.Add(time.Duration(amount) * 24 * time.Hour)
	case "нед":
		return base.Add(time.Duration(amount) * 168 * time.Hour)
	case "мес":
		return base.AddDate(0, int(amount), 0)
	}
	return base
}
