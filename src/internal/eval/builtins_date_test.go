package eval

import "testing"

// T015 — позитив сегодня()/дата(...) (§SM-6). Прогон через исходник; сегодня() при
// testClock = FixedClock{2026,5,31}.
func TestBuiltinDatePositive(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"дата строка", `дата("2026-05-03")`, "2026-05-03"},
		{"дата пусто", `дата(пусто)`, "пусто"},
		{"дата тройка", `дата(2026, 5, 3)`, "2026-05-03"},
		{"дата високосный фев", `дата("2024-02-29")`, "2024-02-29"},
		{"дата тройка високосный", `дата(2024, 2, 29)`, "2024-02-29"},
		{"сегодня фикс-дата", `сегодня()`, "2026-05-31"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := run(t, "печать("+tt.src+")")
			if err != nil {
				t.Fatalf("ошибка: %v", err)
			}
			if out != tt.want+"\n" {
				t.Errorf("= %q, хотим %q", out, tt.want+"\n")
			}
		})
	}
}

// T016 — exact-match негатив дата(...) (§SM-9.C, тексты байт-точно). Позиция всех = CallExpr.Pos().
func TestBuiltinDateErrors(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantMsg string
		isType  bool // true → ОшибкаТипа, иначе ОшибкаВыполнения
	}{
		{
			name:    "невалидный календарь",
			src:     `дата("2026-13-40")`,
			wantMsg: "дата: «2026-13-40» не является датой",
		},
		{
			name:    "не високосный фев 29",
			src:     `дата("2026-02-29")`,
			wantMsg: "дата: «2026-02-29» не является датой",
		},
		{
			name:    "строка со временем T",
			src:     `дата("2026-05-03T10:00")`,
			wantMsg: "дата: «2026-05-03T10:00» не является датой",
		},
		{
			name:    "тройка невалидный месяц",
			src:     `дата(2026, 13, 1)`,
			wantMsg: "дата: некорректные год/месяц/день",
		},
		{
			name:    "1 арг неверного типа",
			src:     `дата(истина)`,
			wantMsg: "дата: ожидается Строка или Пусто, получено Булево",
			isType:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := run(t, "печать("+tt.src+")")
			if err == nil {
				t.Fatalf("ожидалась ошибка для %s", tt.src)
			}
			_, _, msg := evalErr(t, err)
			if msg != tt.wantMsg {
				t.Errorf("msg = %q, хотим %q", msg, tt.wantMsg)
			}
			if tt.isType {
				if !isType(err) {
					t.Errorf("категория не ОшибкаТипа: %T", err)
				}
			} else {
				if !isRuntime(err) {
					t.Errorf("категория не ОшибкаВыполнения: %T", err)
				}
			}
		})
	}
}

// Арность дата(...): 0/2/≥4 → «'дата': неверное число аргументов» (ОшибкаВыполнения).
func TestBuiltinDateArity(t *testing.T) {
	for _, src := range []string{`дата()`, `дата(1, 2)`, `дата(1, 2, 3, 4)`} {
		t.Run(src, func(t *testing.T) {
			_, err := run(t, "печать("+src+")")
			if err == nil {
				t.Fatalf("ожидалась ошибка для %s", src)
			}
			_, _, msg := evalErr(t, err)
			if msg != "'дата': неверное число аргументов" {
				t.Errorf("msg = %q, хотим %q", msg, "'дата': неверное число аргументов")
			}
			if !isRuntime(err) {
				t.Errorf("категория не ОшибкаВыполнения: %T", err)
			}
		})
	}
}
