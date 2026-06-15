package eval

import (
	"time"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// periodWindow вычисляет полный календарный период [начало, конец] (включительно
// обе границы), содержащий дату d, по предопределённому имени периода p (§SM-8.2,
// контракт ME-6). d — текущая дата (D = clock.Now()), окно НЕ усекается по d.
//
// Возвращает ok=false, если имя периода неизвестно (движок метрики обработает —
// период: должен давать один из пяти value.PeriodNames). Арифметика дат —
// служебная Go (time.Date в UTC), результат-границы суть value.Дата, сравниваемые
// value.Compare.
//
// 011-A2 (§MW-6) расширяет switch АДДИТИВНО:
//   - p.Name=="последние" (§MW-D-WINDOW-SLIDE): скользящее окно (d−N ед, d],
//     возвращаемое инклюзивно-эквивалентно (конец=d; начало=(d−N ед)+1день), чтобы
//     инклюзивный recordSurvives работал БЕЗ изменения;
//   - p.Offset!=0 для календарного Name (§MW-D-WINDOW-COMPLETED): якорь d сдвигается
//     назад на |Offset| периодов базовой единицы, затем считается существующее
//     календарное окно сдвинутой даты (5 ветвей ниже).
//
// Всё считается от d=i.now() (заморожен, §MW-6 детерминизм) — никогда time.Now().
func periodWindow(p value.Период, d value.Дата) (начало, конец value.Дата, ok bool) {
	// §MW-D-WINDOW-SLIDE: скользящее окно «последние N <ед>».
	if p.Name == "последние" {
		n := int(p.Amount)
		var нижняя_искл time.Time
		t := dateToTime(d)
		switch p.Unit {
		case "дн":
			нижняя_искл = t.AddDate(0, 0, -n)
		case "нед":
			нижняя_искл = t.AddDate(0, 0, -7*n)
		case "мес":
			нижняя_искл = t.AddDate(0, -n, 0)
		default:
			return value.Дата{}, value.Дата{}, false
		}
		// Инклюзивная нижняя граница ≡ полуинтервал (нижняя_искл, d]: начало = искл+1день.
		начало = timeToDate(нижняя_искл.AddDate(0, 0, 1))
		return начало, d, true
	}

	// §MW-D-WINDOW-COMPLETED (ИСПРАВЛЕНО после self-check): «последний завершённый
	// период» — «шаг назад от НАЧАЛА текущего периода». Сырой сдвиг даты
	// d.AddDate(0,Offset,0) на day-31 ПЕРЕПОЛНЯЛСЯ вперёд (2026-05-31.AddDate(0,-1,0)
	// = 2026-05-01, апрель 30 дней → 31 апр = 1 мая = ВСЁ ЕЩЁ МАЙ) → «прошлый месяц»
	// на 31-м давал ТЕКУЩИЙ месяц. Корректно: для КАЖДОГО шага вычислить окно текущего
	// периода базовым Период{Name} (Offset:0 → обычная календарная ветка ниже, без
	// рекурсии в этот блок), затем якорь = начало_тек − 1 день — гарантированно в
	// ПРЕДЫДУЩЕМ периоде для любого Name (день/неделя/месяц/квартал/год) БЕЗ
	// переполнения дня. В A2 Offset == −1 (один шаг); цикл — на будущий |Offset|>1.
	if p.Offset != 0 {
		base := value.Период{Name: p.Name}
		anchor := d
		for i := 0; i < -p.Offset; i++ {
			start, _, ok := periodWindow(base, anchor)
			if !ok {
				return value.Дата{}, value.Дата{}, false
			}
			anchor = timeToDate(dateToTime(start).AddDate(0, 0, -1)) // на день раньше начала текущего периода → предыдущий период
		}
		return periodWindow(base, anchor)
	}

	switch p.Name {
	case "ежедневно":
		return d, d, true

	case "еженедельно":
		// ISO-неделя пн–вс, содержащая d. Go time.Weekday: Sun=0..Sat=6;
		// преобразуем в ISO Mon=1..Sun=7, затем сдвигаем назад на (ISO−1) дней.
		t := dateToTime(d)
		isoDow := int(t.Weekday())
		if isoDow == 0 { // воскресенье → 7 по ISO
			isoDow = 7
		}
		start := t.AddDate(0, 0, -(isoDow - 1))
		end := start.AddDate(0, 0, 6)
		return timeToDate(start), timeToDate(end), true

	case "ежемесячно":
		начало = value.Дата{Year: d.Year, Month: d.Month, Day: 1}
		конец = value.Дата{Year: d.Year, Month: d.Month, Day: lastDayOfMonth(d.Year, d.Month)}
		return начало, конец, true

	case "ежеквартально":
		// Квартал по месяцу d.Month: Q1[1..3] Q2[4..6] Q3[7..9] Q4[10..12].
		qStart := ((d.Month-1)/3)*3 + 1 // 1, 4, 7 или 10
		qEnd := qStart + 2              // 3, 6, 9 или 12
		начало = value.Дата{Year: d.Year, Month: qStart, Day: 1}
		конец = value.Дата{Year: d.Year, Month: qEnd, Day: lastDayOfMonth(d.Year, qEnd)}
		return начало, конец, true

	case "ежегодно":
		начало = value.Дата{Year: d.Year, Month: 1, Day: 1}
		конец = value.Дата{Year: d.Year, Month: 12, Day: 31}
		return начало, конец, true
	}
	return value.Дата{}, value.Дата{}, false
}

// dateToTime переводит value.Дата в time.Time (полночь UTC) для служебной
// календарной арифметики окон.
func dateToTime(d value.Дата) time.Time {
	return time.Date(d.Year, time.Month(d.Month), d.Day, 0, 0, 0, 0, time.UTC)
}

// timeToDate переводит time.Time обратно в value.Дата (Y/M/D).
func timeToDate(t time.Time) value.Дата {
	return value.Дата{Year: t.Year(), Month: int(t.Month()), Day: t.Day()}
}

// lastDayOfMonth возвращает последний день месяца m года y (фев=29 в високосный по
// григорианскому правилу). time.Date(y, m+1, 0, …) нормализуется в последний день
// месяца m.
func lastDayOfMonth(y, m int) int {
	t := time.Date(y, time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC)
	return t.Day()
}
