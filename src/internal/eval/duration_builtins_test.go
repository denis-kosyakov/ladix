package eval

import (
	"bytes"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// runRT исполняет исходник через лексер→парсер→Run с инжектированным
// ProcessRuntime (для процессных встроенных, §EN-9). Лекс/парс-ошибки фатальны.
func runRT(t *testing.T, src string, rt ProcessRuntime) (string, error) {
	t.Helper()
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные лексические/синтаксические ошибки: %v", errList.Error())
	}
	var buf bytes.Buffer
	interp := NewInterpreter(&buf, 0, testClock)
	if rt != nil {
		interp.SetProcessRuntime(rt)
	}
	err := interp.Run(prog)
	return buf.String(), err
}

// T038/§EN-9 — литерал длительности как живое значение (D-7/D-16): печать и
// диапазонная ошибка.
func TestDurationLiteralRuntime(t *testing.T) {
	t.Run("печать(2дн)", func(t *testing.T) {
		out, err := run(t, "пусть d = 2дн\nпечать(d)")
		if err != nil {
			t.Fatalf("ошибка: %v", err)
		}
		if out != "2дн\n" {
			t.Errorf("= %q, хотим %q", out, "2дн\n")
		}
	})
	t.Run("вне диапазона int64", func(t *testing.T) {
		_, err := run(t, "пусть x = 9999999999999999999дн")
		if err == nil {
			t.Fatalf("ожидалась ошибка вне диапазона")
		}
		_, _, msg := evalErr(t, err)
		if want := "литерал длительности вне диапазона типа Целое"; msg != want {
			t.Errorf("msg = %q, хотим %q", msg, want)
		}
		if !isRuntime(err) {
			t.Errorf("категория не ОшибкаВыполнения")
		}
	})
}

// T038/§EN-9 — сравнения Длительности (D-17): == / != / порядок одной единицы /
// разные единицы → TY-BINOP.
func TestDurationComparisonsRuntime(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"2дн == 2дн", "печать(2дн == 2дн)", "истина"},
		{"1час == 60мин", "печать(1час == 60мин)", "ложь"},
		{"2дн < 5дн", "печать(2дн < 5дн)", "истина"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := run(t, tt.src)
			if err != nil {
				t.Fatalf("ошибка: %v", err)
			}
			if out != tt.want+"\n" {
				t.Errorf("= %q, хотим %q", out, tt.want+"\n")
			}
		})
	}
	t.Run("2дн < 5мин разные единицы → TY-BINOP", func(t *testing.T) {
		_, err := run(t, "печать(2дн < 5мин)")
		if err == nil {
			t.Fatalf("ожидалась ОшибкаТипа")
		}
		_, _, msg := evalErr(t, err)
		if want := "'<' нельзя применить к Длительность и Длительность"; msg != want {
			t.Errorf("msg = %q, хотим %q", msg, want)
		}
		if !isType(err) {
			t.Errorf("категория не ОшибкаТипа")
		}
	})
}

// T038/§EN-9 — процессные встроенные через ProcessRuntime (D-15): живой/неизвестный
// id, не-Строка аргумент, задачи_пользователя(""). Покрывает §EN-8.A #1/#2/#3/#4.
func TestProcessBuiltinsRuntime(t *testing.T) {
	t.Run("статус_процесса живой id", func(t *testing.T) {
		rt := &fakeRuntime{statusByID: map[string]string{"p-000001": "ожидает"}}
		out, err := runRT(t, `печать(статус_процесса("p-000001"))`, rt)
		if err != nil {
			t.Fatalf("ошибка: %v", err)
		}
		if out != "ожидает\n" {
			t.Errorf("= %q, хотим %q", out, "ожидает\n")
		}
	})
	t.Run("статус_процесса неизвестный id → #1", func(t *testing.T) {
		rt := &fakeRuntime{} // statusByID пуст → ok=false
		_, err := runRT(t, `печать(статус_процесса("p-999999"))`, rt)
		if err == nil {
			t.Fatalf("ожидалась ОшибкаВыполнения")
		}
		_, _, msg := evalErr(t, err)
		if want := "процесс 'p-999999' не найден"; msg != want {
			t.Errorf("msg = %q, хотим %q", msg, want)
		}
		if !isRuntime(err) {
			t.Errorf("категория не ОшибкаВыполнения")
		}
	})
	t.Run("статус_процесса не-Строка → #2", func(t *testing.T) {
		rt := &fakeRuntime{}
		_, err := runRT(t, `печать(статус_процесса(1))`, rt)
		if err == nil {
			t.Fatalf("ожидалась ОшибкаТипа")
		}
		_, _, msg := evalErr(t, err)
		if want := "статус_процесса: ожидается Строка, получено Целое"; msg != want {
			t.Errorf("msg = %q, хотим %q", msg, want)
		}
		if !isType(err) {
			t.Errorf("категория не ОшибкаТипа")
		}
	})
	t.Run("состояние_процесса не-Строка → #3", func(t *testing.T) {
		rt := &fakeRuntime{}
		_, err := runRT(t, `печать(состояние_процесса(1))`, rt)
		if err == nil {
			t.Fatalf("ожидалась ОшибкаТипа")
		}
		_, _, msg := evalErr(t, err)
		if want := "состояние_процесса: ожидается Строка, получено Целое"; msg != want {
			t.Errorf("msg = %q, хотим %q", msg, want)
		}
		if !isType(err) {
			t.Errorf("категория не ОшибкаТипа")
		}
	})
	t.Run("задачи_пользователя не-Строка → #4", func(t *testing.T) {
		rt := &fakeRuntime{}
		_, err := runRT(t, `печать(задачи_пользователя(1))`, rt)
		if err == nil {
			t.Fatalf("ожидалась ОшибкаТипа")
		}
		_, _, msg := evalErr(t, err)
		if want := "задачи_пользователя: ожидается Строка, получено Целое"; msg != want {
			t.Errorf("msg = %q, хотим %q", msg, want)
		}
		if !isType(err) {
			t.Errorf("категория не ОшибкаТипа")
		}
	})
	t.Run("задачи_пользователя(\"\") → все открытые", func(t *testing.T) {
		one := value.NewRecord([]string{"ид"}, map[string]value.Value{"ид": value.Строка{V: "t-000001"}})
		rt := &fakeRuntime{userTasks: []value.Запись{one}}
		out, err := runRT(t, `печать(длина(задачи_пользователя("")))`, rt)
		if err != nil {
			t.Fatalf("ошибка: %v", err)
		}
		if out != "1\n" {
			t.Errorf("= %q, хотим %q", out, "1\n")
		}
	})
	t.Run("состояние_процесса живой id → Запись", func(t *testing.T) {
		rec := value.NewRecord([]string{"имя"}, map[string]value.Value{"имя": value.Строка{V: "Петров"}})
		rt := &fakeRuntime{varsByID: map[string]value.Запись{"p-000001": rec}}
		out, err := runRT(t, `печать(состояние_процесса("p-000001"))`, rt)
		if err != nil {
			t.Fatalf("ошибка: %v", err)
		}
		if out != "{имя: Петров}\n" {
			t.Errorf("= %q, хотим %q", out, "{имя: Петров}\n")
		}
	})
	t.Run("состояние_процесса неизвестный id → #1", func(t *testing.T) {
		rt := &fakeRuntime{}
		_, err := runRT(t, `печать(состояние_процесса("p-999999"))`, rt)
		if err == nil {
			t.Fatalf("ожидалась ОшибкаВыполнения")
		}
		_, _, msg := evalErr(t, err)
		if want := "процесс 'p-999999' не найден"; msg != want {
			t.Errorf("msg = %q, хотим %q", msg, want)
		}
		if !isRuntime(err) {
			t.Errorf("категория не ОшибкаВыполнения")
		}
	})
}

// T007/§DB-6 — позитив вчера()/завтра() (детерминизм: FixedClock{2026-05-31}, НЕ
// time.Now): −1/+1 день с переходом через границу месяца.
func TestDatetimeRelativeDays(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"вчера", "печать(вчера())", "2026-05-30"},
		{"завтра — граница месяца", "печать(завтра())", "2026-06-01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := run(t, tt.src)
			if err != nil {
				t.Fatalf("ошибка: %v", err)
			}
			if out != tt.want+"\n" {
				t.Errorf("= %q, хотим %q", out, tt.want+"\n")
			}
		})
	}
}

// T007/§DB-6 — тип результата вчера()/завтра() — Дата. «тип» зарезервировано
// лексером (недостижимо из исходника), поэтому проверяем TypeName() белым ящиком
// через evalExpr1 (как TestBuiltinTipDirect).
func TestDatetimeRelativeDaysType(t *testing.T) {
	for _, expr := range []string{"вчера()", "завтра()"} {
		t.Run(expr, func(t *testing.T) {
			v := evalExpr1(t, expr)
			if got := v.TypeName(); got != "Дата" {
				t.Errorf("тип(%s) = %q, хотим Дата", expr, got)
			}
		})
	}
}

// T009/§DB-6 — позитив длительность(...): конструкция без нормализации; равенство по
// {Amount,Unit}; 0/отрицательное/мес — без ошибки.
func TestDatetimeDlitelnost(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"равенство по полям", `печать(длительность(5, "мин") == 5мин)`, "истина"},
		{"разные единицы не равны", `печать(длительность(60, "мин") == длительность(1, "час"))`, "ложь"},
		{"ноль", `печать(длительность(0, "нед"))`, "0нед"},
		{"отрицательное", `печать(длительность(-3, "дн"))`, "-3дн"},
		{"месяц конструируется", `печать(длительность(1, "мес"))`, "1мес"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := run(t, tt.src)
			if err != nil {
				t.Fatalf("ошибка: %v", err)
			}
			if out != tt.want+"\n" {
				t.Errorf("= %q, хотим %q", out, tt.want+"\n")
			}
		})
	}
}

// T011/§DB-6 — позитив конвертеров: множители в_секундах, тождества и усечение к
// нулю (включая отрицательное частное).
func TestDatetimeConverters(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"в_секундах сек", `печать(в_секундах(длительность(1, "сек")))`, "1"},
		{"в_секундах мин", `печать(в_секундах(длительность(1, "мин")))`, "60"},
		{"в_секундах 2мин", `печать(в_секундах(длительность(2, "мин")))`, "120"},
		{"в_секундах час", `печать(в_секундах(длительность(1, "час")))`, "3600"},
		{"в_секундах дн", `печать(в_секундах(длительность(1, "дн")))`, "86400"},
		{"в_секундах нед", `печать(в_секундах(длительность(1, "нед")))`, "604800"},
		{"в_днях нед", `печать(в_днях(длительность(1, "нед")))`, "7"},
		{"в_часах дн", `печать(в_часах(длительность(1, "дн")))`, "24"},
		{"усечение в_минутах 90сек", `печать(в_минутах(длительность(90, "сек")))`, "1"},
		{"усечение в_часах 90мин", `печать(в_часах(длительность(90, "мин")))`, "1"},
		{"усечение в_днях 40час", `печать(в_днях(длительность(40, "час")))`, "1"},
		{"усечение к нулю отрицательного", `печать(в_минутах(длительность(-90, "сек")))`, "-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := run(t, tt.src)
			if err != nil {
				t.Fatalf("ошибка: %v", err)
			}
			if out != tt.want+"\n" {
				t.Errorf("= %q, хотим %q", out, tt.want+"\n")
			}
		})
	}
}

// T012/§DB-5 — негатив: 4 новые строки ошибок БАЙТ-В-БАЙТ (гильемы «», без точки),
// плюс верная категория Go-ошибки.
func TestDatetimeErrorsExact(t *testing.T) {
	tests := []struct {
		name, src, want string
		typ             bool // true → ОшибкаТипа, false → ОшибкаВыполнения
	}{
		{
			"длительность: Дробное вместо Целое",
			`длительность(1.5, "час")`,
			"длительность: ожидается Целое и Строка, получено Дробное и Строка",
			true,
		},
		{
			"длительность: Строка вместо Целое",
			`длительность("5", "мин")`,
			"длительность: ожидается Целое и Строка, получено Строка и Строка",
			true,
		},
		{
			"длительность: неизвестная единица",
			`длительность(5, "XYZ")`,
			"длительность: неизвестная единица «XYZ»",
			false,
		},
		{
			"в_секундах: не Длительность",
			`в_секундах(5)`,
			"в_секундах: ожидается Длительность, получено Целое",
			true,
		},
		{
			"в_секундах: мес не приводится",
			`в_секундах(длительность(1, "мес"))`,
			"в_секундах: месяцы не приводятся без даты-якоря",
			false,
		},
		{
			"в_минутах: мес не приводится (подстановка имени)",
			`в_минутах(длительность(1, "мес"))`,
			"в_минутах: месяцы не приводятся без даты-якоря",
			false,
		},
		{
			"в_часах: мес не приводится (подстановка имени)",
			`в_часах(длительность(1, "мес"))`,
			"в_часах: месяцы не приводятся без даты-якоря",
			false,
		},
		{
			"в_днях: мес не приводится (подстановка имени)",
			`в_днях(длительность(1, "мес"))`,
			"в_днях: месяцы не приводятся без даты-якоря",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := run(t, tt.src)
			if err == nil {
				t.Fatalf("ожидалась ошибка")
			}
			_, _, msg := evalErr(t, err)
			if msg != tt.want {
				t.Errorf("msg = %q, хотим %q", msg, tt.want)
			}
			if tt.typ && !isType(err) {
				t.Errorf("категория не ОшибкаТипа")
			}
			if !tt.typ && !isRuntime(err) {
				t.Errorf("категория не ОшибкаВыполнения")
			}
		})
	}
}

// T013/§DB-6 — переиспользуемые строки (новых НЕ вводить): нарушение арности через
// существующую форму реестра; переполнение конвертера → существующая строка.
func TestDatetimeReusedErrors(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"арность длительность", `длительность(5)`, "'длительность' принимает 2 аргументов, передано 1"},
		{"арность в_секундах", `в_секундах()`, "'в_секундах' принимает 1 аргументов, передано 0"},
		{"переполнение конвертера", `в_секундах(длительность(9223372036854775807, "нед"))`, "переполнение целого числа"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := run(t, tt.src)
			if err == nil {
				t.Fatalf("ожидалась ошибка")
			}
			_, _, msg := evalErr(t, err)
			if msg != tt.want {
				t.Errorf("msg = %q, хотим %q", msg, tt.want)
			}
		})
	}
}
