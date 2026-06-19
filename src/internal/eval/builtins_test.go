package eval

import (
	"math"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// Закрытый реестр: РОВНО 35 активных, 0 deferred = 35 (D6/§DB-6; 008 активировал 7
// дата/время-функций, ранее 004 — дата/сегодня §SM-6, 006 — 3 процессных §EN-0/D-15);
// длина в счёте ×1.
func TestBuiltinRegistryClosed(t *testing.T) {
	m := registerBuiltins()
	if len(m) != 35 {
		t.Fatalf("всего встроенных = %d, хотим 35", len(m))
	}
	active, deferred := 0, 0
	for _, b := range m {
		if b.Deferred {
			deferred++
		} else {
			active++
		}
	}
	if active != 35 {
		t.Errorf("активных = %d, хотим 35", active)
	}
	if deferred != 0 {
		t.Errorf("deferred = %d, хотим 0", deferred)
	}
}

// булево(x) — единственная точка truthy (B-5).
func TestBuiltinBulevoTruthy(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"булево(0)", "ложь"},
		{"булево(0.0)", "ложь"},
		{`булево("")`, "ложь"},
		{"булево([])", "ложь"},
		{"булево(пусто)", "ложь"},
		{"булево(ложь)", "ложь"},
		{"булево(1)", "истина"},
		{`булево("x")`, "истина"},
		{"булево([0])", "истина"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			out, err := run(t, "печать("+tt.src+")")
			if err != nil {
				t.Fatalf("ошибка: %v", err)
			}
			if out != tt.want+"\n" {
				t.Errorf("= %q, хотим %q", out, tt.want)
			}
		})
	}
}

// тип(x) недостижим из исходника (с 010-A1 «тип» — ключевое слово KW_TYPE, не
// начинающее выражение → парс-ошибка; ранее — резерв лексера) — проверяем функцию
// напрямую: каноническое имя типа.
func TestBuiltinTipDirect(t *testing.T) {
	v, err := builtinTip(nil, []value.Value{value.Дробное{V: 3.0}}, ast.Position{})
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if !value.Equal(v, value.Строка{V: "Дробное"}) {
		t.Errorf("тип(3.0) = %v, хотим Дробное", v)
	}
}

// Преобразования и агрегаты/списки/строки через исходник.
func TestBuiltinBehaviors(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"строка числа", `строка(42)`, "42"},
		{"целое из строки", `целое("42")`, "42"},
		{"дробное из целого", `дробное(3)`, "3.0"},
		{"число авто-целое", `число("7")`, "7"},
		{"число авто-дробное", `число("7.5")`, "7.5"},
		{"сумма", `сумма([1, 2, 3])`, "6"},
		{"сумма пустого = 0", `сумма([])`, "0"},
		{"сумма с дробным", `сумма([1, 2.0])`, "3.0"},
		{"количество", `количество([1, 2, 3])`, "3"},
		{"среднее", `среднее([2, 4])`, "3.0"},
		{"мин", `мин([3, 1, 2])`, "1"},
		{"макс", `макс([3, 1, 2])`, "3"},
		{"длина списка", `длина([1, 2, 3])`, "3"},
		{"длина строки в рунах", `длина("привет")`, "6"},
		{"соединить", `соединить([1], [2, 3])`, "[1, 2, 3]"},
		{"срез", `срез([10, 20, 30, 40], 1, 3)`, "[20, 30]"},
		{"содержит истина", `содержит([1, 2], 2)`, "истина"},
		{"найти индекс", `найти([10, 20], 20)`, "1"},
		{"найти нет = -1", `найти([10], 99)`, "-1"},
		{"обратить", `обратить([1, 2, 3])`, "[3, 2, 1]"},
		{"сортировать", `сортировать([3, 1, 2])`, "[1, 2, 3]"},
		{"диапазон 1 арг", `диапазон(4)`, "[0, 1, 2, 3]"},
		{"диапазон 2 арг", `диапазон(1, 4)`, "[1, 2, 3]"},
		{"подстрока в рунах", `подстрока("привет", 0, 3)`, "при"},
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

// добавить — единственный мутатор: меняет список на месте, возвращает Пусто.
func TestBuiltinDobavitMutates(t *testing.T) {
	out, err := run(t, `пусть s = [1, 2]
добавить(s, 3)
печать(s)`)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if out != "[1, 2, 3]\n" {
		t.Errorf("= %q", out)
	}
}

// копия не трогает оригинал (поверхностная копия).
func TestBuiltinKopiyaIndependent(t *testing.T) {
	out, err := run(t, `пусть a = [1, 2]
пусть b = копия(a)
добавить(b, 3)
печать(a)
печать(b)`)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if out != "[1, 2]\n[1, 2, 3]\n" {
		t.Errorf("= %q", out)
	}
}

// 7 дата/время-функций активированы (008/§DB-6): минимальная sanity-проверка
// вызываемости каждого имени без ошибки (детерминизм через FixedClock; матрица
// значений/усечения/ошибок — в duration_builtins_test.go).
func TestBuiltinDatetimeCallable(t *testing.T) {
	srcs := []string{
		"печать(вчера())",
		"печать(завтра())",
		`печать(длительность(5, "мин"))`,
		`печать(в_секундах(длительность(1, "мин")))`,
		`печать(в_минутах(длительность(60, "сек")))`,
		`печать(в_часах(длительность(3600, "сек")))`,
		`печать(в_днях(длительность(1, "нед")))`,
	}
	for _, src := range srcs {
		t.Run(src, func(t *testing.T) {
			if _, err := run(t, src); err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
		})
	}
}

// Фикс C: дробное/число отвергают нечисловые ±Inf/NaN и hex-формы (0x1p4),
// которые ParseFloat принимает молча → ОшибкаВыполнения «не является конечным
// числом». Непарсимые строки («abc») по-прежнему дают ОшибкаТипа «не является
// числом» (сообщение не смешивать). Валидные строки конвертируются штатно.
func TestConvertNonFinite(t *testing.T) {
	// runtime-ветка: успешный ParseFloat, но не конечное/hex → ОшибкаВыполнения.
	nonfinite := []struct{ name, src, msg string }{
		{"дробное nan", `дробное("nan")`, "дробное: «nan» не является конечным числом"},
		{"дробное +inf", `дробное("+inf")`, "дробное: «+inf» не является конечным числом"},
		{"дробное 0x1p4", `дробное("0x1p4")`, "дробное: «0x1p4» не является конечным числом"},
		{"число inf", `число("inf")`, "число: «inf» не является конечным числом"},
	}
	for _, tt := range nonfinite {
		t.Run(tt.name, func(t *testing.T) {
			_, err := run(t, "печать("+tt.src+")")
			if err == nil {
				t.Fatalf("%s: ожидалась ошибка", tt.src)
			}
			_, _, msg := evalErr(t, err)
			if msg != tt.msg {
				t.Errorf("msg = %q, хотим %q", msg, tt.msg)
			}
			if !isRuntime(err) {
				t.Errorf("%s: категория не ОшибкаВыполнения", tt.src)
			}
		})
	}

	// type-ветка: непарсимая строка → ОшибкаТипа «не является числом» (сохранено).
	typecases := []struct{ name, src, msg string }{
		{"дробное abc", `дробное("abc")`, "дробное: строка «abc» не является числом"},
		{"число abc", `число("abc")`, "число: строка «abc» не является числом"},
	}
	for _, tt := range typecases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := run(t, "печать("+tt.src+")")
			if err == nil {
				t.Fatalf("%s: ожидалась ошибка", tt.src)
			}
			_, _, msg := evalErr(t, err)
			if msg != tt.msg {
				t.Errorf("msg = %q, хотим %q", msg, tt.msg)
			}
			if !isType(err) {
				t.Errorf("%s: категория не ОшибкаТипа", tt.src)
			}
		})
	}

	// happy-путь: конечные строки конвертируются штатно.
	ok := []struct{ name, src, want string }{
		{"дробное 3.5", `дробное("3.5")`, "3.5"},
		{"число 42", `число("42")`, "42"},
	}
	for _, tt := range ok {
		t.Run(tt.name, func(t *testing.T) {
			out, err := run(t, "печать("+tt.src+")")
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if out != tt.want+"\n" {
				t.Errorf("= %q, хотим %q", out, tt.want+"\n")
			}
		})
	}
}

// Фикс E: сумма со смешанным Дробным НЕ срабатывает на int-overflow-гарде —
// гард активен только когда все элементы Целые. Смешанный список с краевыми
// Целыми + Дробным суммируется во float → Дробное (без ложного «переполнения»).
// Регресс: чисто-целый список с переполнением по-прежнему даёт ОшибкаВыполнения.
func TestSummaOverflowGatedByFloat(t *testing.T) {
	// Смешанное: 2×MaxInt64 (целая сумма переполнилась бы) + 1.5 → float-путь, ОК.
	out, err := run(t, "печать(сумма([9223372036854775807, 9223372036854775807, 1.5]))")
	if err != nil {
		t.Fatalf("смешанная сумма: неожиданная ошибка: %v", err)
	}
	want := value.String(value.Дробное{V: float64(math.MaxInt64) + float64(math.MaxInt64) + 1.5}) + "\n"
	if out != want {
		t.Errorf("смешанная сумма = %q, хотим %q", out, want)
	}

	// Регресс: чисто целочисленное переполнение → ОшибкаВыполнения.
	_, err = run(t, "печать(сумма([9223372036854775807, 1]))")
	if err == nil {
		t.Fatal("целочисленное переполнение: ожидалась ошибка")
	}
	_, _, msg := evalErr(t, err)
	if msg != "переполнение целого числа" {
		t.Errorf("msg = %q, хотим «переполнение целого числа»", msg)
	}
	if !isRuntime(err) {
		t.Errorf("категория не ОшибкаВыполнения")
	}
}

// Фикс B (eval-уровень): после фиксов C/D ±Inf/NaN недостижимы из ИСХОДНИКА
// (дробное/число их отвергают, source_loader тоже; деление на 0 — ошибка; нет
// sqrt/log/степени). Поэтому NaN конструируется белым ящиком (как
// TestBuiltinTipDirect) и подаётся в сортировать/мин напрямую: value.Compare
// возвращает ok==false для NaN → ОшибкаТипа «<имя>: элементы несравнимы».
// Замыкает сиблинг-фикс value/equal.go (NaN-гард в Compare).
func TestSortMinNaNIncomparable(t *testing.T) {
	nan := value.Дробное{V: math.NaN()}
	lst := value.NewList([]value.Value{value.Целое{V: 1}, nan})

	if _, err := builtinSortirovat(nil, []value.Value{lst}, ast.Position{}); err == nil {
		t.Error("сортировать(NaN): ожидалась ошибка несравнимости")
	} else if !isType(err) {
		t.Errorf("сортировать(NaN): категория не ОшибкаТипа: %v", err)
	} else if _, _, msg := evalErr(t, err); msg != "сортировать: элементы несравнимы" {
		t.Errorf("сортировать(NaN): msg = %q", msg)
	}

	if _, err := builtinMin(nil, []value.Value{lst}, ast.Position{}); err == nil {
		t.Error("мин(NaN): ожидалась ошибка несравнимости")
	} else if !isType(err) {
		t.Errorf("мин(NaN): категория не ОшибкаТипа: %v", err)
	} else if _, _, msg := evalErr(t, err); msg != "мин: элементы несравнимы" {
		t.Errorf("мин(NaN): msg = %q", msg)
	}
}
