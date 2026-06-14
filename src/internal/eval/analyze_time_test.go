package eval

import "testing"

// TestScheduleTimeFormatOK — позитивы SE-TIME-FORMAT (T033, R-8/FR-014): валидные
// «в "ЧЧ:ММ"» проходят семпроход чисто. Тело валидируется штатно (печать(1)).
func TestScheduleTimeFormatOK(t *testing.T) {
	valid := []string{"00:00", "09:05", "14:30", "23:59", "23:00", "00:59"}
	for _, hhmm := range valid {
		t.Run(hhmm, func(t *testing.T) {
			src := "когда расписание в \"" + hhmm + "\":\n    печать(1)\n"
			if err := analyzeSrc(t, src); err != nil {
				t.Errorf("формат %q должен быть валиден, получено: %v", hhmm, err)
			}
		})
	}
}

// TestScheduleTimeFormatNegatives — exact-match golden SE-TIME-FORMAT (T033):
// невалидный формат «в "ЧЧ:ММ"» → СемантическаяОшибка с дословным текстом и
// позицией токена строкового литерала (двухстрочный канон §13, Принцип IV).
//
// Префикс «когда расписание в » фиксирован (19 рун: когда=5, ' '=1, расписание=10,
// ' '=1, в=1, ' '=1) → токен строкового литерала всегда в строке 1, колонке 20.
func TestScheduleTimeFormatNegatives(t *testing.T) {
	const wantLine, wantCol = 1, 20
	tests := []struct {
		name string
		hhmm string
	}{
		{"часы и минуты вне диапазона", "25:99"},
		{"нет ведущего нуля часов", "9:05"},
		{"нет ведущего нуля минут", "09:5"},
		{"обе группы одна цифра", "9:5"},
		{"часы 24", "24:00"},
		{"минуты 60", "12:60"},
		{"не цифры", "ab:cd"},
		{"дефис вместо двоеточия", "12-30"},
		{"три цифры часов", "012:30"},
		{"пусто", ""},
		{"только цифры без двоеточия", "1230"},
		{"лишний символ в конце", "09:05 "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "когда расписание в \"" + tt.hhmm + "\":\n    печать(1)\n"
			err := analyzeSrc(t, src)
			if err == nil {
				t.Fatalf("ожидалась SE-TIME-FORMAT для %q, получено nil", tt.hhmm)
			}
			if !isSem(err) {
				t.Errorf("категория не СемантическаяОшибка: %T", err)
			}
			line, col, msg := evalErr(t, err)
			want := "неверный формат времени '" + tt.hhmm + "': ожидается \"ЧЧ:ММ\" (часы 00–23, минуты 00–59)"
			if msg != want {
				t.Errorf("msg = %q, хотим %q", msg, want)
			}
			if line != wantLine || col != wantCol {
				t.Errorf("позиция = (%d,%d), хотим (%d,%d)", line, col, wantLine, wantCol)
			}
		})
	}
}

// TestScheduleEveryZeroInterval — exact-match golden для нулевого интервала
// расписания «каждые 0<ед>» (SCHED-1): интервал должен быть строго положительным,
// иначе next==LastFire → busy-fire на каждом тике демона. Нарушение →
// СемантическаяОшибка с дословным текстом и позицией токена длительности.
//
// Позиция: ошибка указывает на токен DurationLit «0мин» (зеркало SE-TIME-FORMAT,
// указывающего на под-узел строки). Префикс «когда расписание каждые » — 24 руны
// (когда=5, ' '=1, расписание=10, ' '=1, каждые=6, ' '=1) → «0мин» в строке 1,
// колонке 25. Единиценезависимо: проверяем все формы нуля и разные единицы.
func TestScheduleEveryZeroInterval(t *testing.T) {
	const wantLine, wantCol = 1, 25
	tests := []struct {
		name string
		dur  string // amount+unit как во входе
	}{
		{"ноль минут", "0мин"},
		{"ноль секунд", "0сек"},
		{"ноль дней", "0дн"},
		{"ноль недель (календарная единица)", "0нед"},
		{"ноль месяцев (календарная единица)", "0мес"},
		{"ведущие нули остаются нулём", "00мин"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "когда расписание каждые " + tt.dur + ":\n    печать(1)\n"
			err := analyzeSrc(t, src)
			if err == nil {
				t.Fatalf("ожидалась семантическая ошибка для «каждые %s», получено nil", tt.dur)
			}
			if !isSem(err) {
				t.Errorf("категория не СемантическаяОшибка: %T", err)
			}
			line, col, msg := evalErr(t, err)
			want := "интервал расписания должен быть положительным: 'каждые " + tt.dur + "'"
			if msg != want {
				t.Errorf("msg = %q, хотим %q", msg, want)
			}
			if line != wantLine || col != wantCol {
				t.Errorf("позиция = (%d,%d), хотим (%d,%d)", line, col, wantLine, wantCol)
			}
		})
	}
}

// TestScheduleEveryPositiveOK — положительные интервалы «каждые N<ед>» проходят
// семпроход чисто (SCHED-1 не ложно-срабатывает). Тело валидируется штатно.
func TestScheduleEveryPositiveOK(t *testing.T) {
	valid := []string{"1мин", "30сек", "1час", "3дн", "2нед", "1мес", "999мин"}
	for _, dur := range valid {
		t.Run(dur, func(t *testing.T) {
			src := "когда расписание каждые " + dur + ":\n    печать(1)\n"
			if err := analyzeSrc(t, src); err != nil {
				t.Errorf("«каждые %s» должно быть валидно, получено: %v", dur, err)
			}
		})
	}
}
