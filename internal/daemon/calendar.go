package daemon

import "time"

// shiftEvery вычисляет следующий момент срабатывания расписания «каждые amount unit»
// от момента last (R-6, FR-012). Две арифметики:
//
//   - ФИКС-множитель (сек/мин/час/дн): целое кратное фиксированной длительности —
//     last.Add(amount*unitDur). День здесь — ровно 24 часа (без учёта DST/секунд
//     координации: часы планировщика монотонны по контракту).
//   - КАЛЕНДАРНЫЙ сдвиг (нед/мес): нед → AddDate(0,0,7*amount); мес → AddDate(0,amount,0)
//     с ЗАЖИМОМ конца месяца. Go AddDate нормализует переполнение (31 янв +1 мес → 3 мар),
//     что для расписания неверно — зажимаем день к последнему дню целевого месяца
//     (31 янв +1 мес → 28/29 фев) паттерном lastDayOfMonth (зеркало eval/window.go:69,
//     арифметику не дублируем сверх служебного зажима).
//
// Неизвестная unit (статически невозможна — лексер допускает только 6 единиц,
// keywords.go:38) трактуется как фикс-день (безопасный дефолт, тик не падает).
func shiftEvery(last time.Time, amount int64, unit string) time.Time {
	switch unit {
	case "сек":
		return last.Add(time.Duration(amount) * time.Second)
	case "мин":
		return last.Add(time.Duration(amount) * time.Minute)
	case "час":
		return last.Add(time.Duration(amount) * time.Hour)
	case "дн":
		return last.Add(time.Duration(amount) * 24 * time.Hour)
	case "нед":
		return last.AddDate(0, 0, int(7*amount))
	case "мес":
		return addMonthsClamped(last, int(amount))
	default:
		return last.Add(time.Duration(amount) * 24 * time.Hour)
	}
}

// addMonthsClamped прибавляет months календарных месяцев к t, ЗАЖИМАЯ день к
// последнему дню целевого месяца, если исходный день в нём отсутствует (31 янв +1
// мес → 28/29 фев; 31 мар +1 мес → 30 апр). Без зажима time.AddDate переполнил бы в
// следующий месяц. Год/месяц цели берём через служебный AddDate первого числа (он
// переполнения по дню не даёт), затем зажимаем день. Время суток/наносекунды/зона
// сохраняются от t.
func addMonthsClamped(t time.Time, months int) time.Time {
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()).AddDate(0, months, 0)
	y, m := first.Year(), int(first.Month())
	day := t.Day()
	if last := lastDayOfMonth(y, m); day > last {
		day = last
	}
	return time.Date(y, time.Month(m), day, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
}

// lastDayOfMonth возвращает последний день месяца m года y (фев=29 в високосный по
// григорианскому правилу). time.Date(y, m+1, 0, …) нормализуется в последний день
// месяца m (паттерн eval/window.go:69; служебная арифметика, не доменная).
func lastDayOfMonth(y, m int) int {
	return time.Date(y, time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
