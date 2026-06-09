package eval

import (
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// Закрытый реестр: РОВНО 25 активных + 10 deferred = 35 (D6, после активации
// дата/сегодня в 004 — §SM-6); длина в счёте ×1.
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
	if active != 25 {
		t.Errorf("активных = %d, хотим 25", active)
	}
	if deferred != 10 {
		t.Errorf("deferred = %d, хотим 10", deferred)
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

// тип(x) недостижим из исходника (лексер резервирует «тип») — проверяем функцию
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

// Все 10 deferred → SEM-DEFERRED-BUILTIN (отлично от «не объявлена»).
func TestBuiltinDeferredAll(t *testing.T) {
	for _, name := range deferredNames {
		t.Run(name, func(t *testing.T) {
			_, err := run(t, "печать("+name+"())")
			if err == nil {
				t.Fatalf("deferred %s не дал ошибку", name)
			}
			_, _, msg := evalErr(t, err)
			want := "функция '" + name + "' не поддерживается в этой версии"
			if msg != want {
				t.Errorf("msg = %q, хотим %q", msg, want)
			}
			if !isSem(err) {
				t.Errorf("категория не СемантическаяОшибка")
			}
		})
	}
}
