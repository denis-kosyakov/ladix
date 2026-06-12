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

// T038/§EN-9 — deferred-граница 7 имён НЕ сдвинулась: вчера() остаётся deferred.
func TestDeferredDateBuiltinStillDeferred(t *testing.T) {
	_, err := run(t, "печать(вчера())")
	if err == nil {
		t.Fatalf("ожидалась СемантическаяОшибка deferred")
	}
	_, _, msg := evalErr(t, err)
	if want := "функция 'вчера' не поддерживается в этой версии"; msg != want {
		t.Errorf("msg = %q, хотим %q", msg, want)
	}
	if !isSem(err) {
		t.Errorf("категория не СемантическаяОшибка")
	}
}
