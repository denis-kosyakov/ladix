package eval

import "testing"

// Вызов пользовательской функции, ранний и обычный возврат, forward-вызов.
func TestUserFunctionReturn(t *testing.T) {
	src := `функция средний_чек(заказы):
    если длина(заказы) == 0:
        вернуть 0
    вернуть сумма(заказы) / длина(заказы)
печать(средний_чек([1500, 2300, 4100, 800]))
печать(средний_чек([]))`
	out, err := run(t, src)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if out != "2175.0\n0\n" {
		t.Errorf("= %q, хотим 2175.0/0", out)
	}
}

// Рекурсия резолвится через таблицу funcs независимо от кадра.
func TestRecursion(t *testing.T) {
	src := `функция факт(n):
    если n <= 1:
        вернуть 1
    вернуть n * факт(n - 1)
печать(факт(5))
печать(факт(0))`
	out, err := run(t, src)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if out != "120\n1\n" {
		t.Errorf("= %q, хотим 120/1", out)
	}
}

// Голый вернуть → Пусто.
func TestBareReturnNone(t *testing.T) {
	src := `функция f():
    вернуть
печать(f())`
	out, err := run(t, src)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if out != "пусто\n" {
		t.Errorf("= %q, хотим пусто", out)
	}
}

// Функция без вернуть → Пусто.
func TestNoReturnNone(t *testing.T) {
	src := `функция f():
    пусть x = 1
печать(f())`
	out, err := run(t, src)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if out != "пусто\n" {
		t.Errorf("= %q", out)
	}
}

// Убегающая рекурсия → RT-DEPTH; --max-depth меняет число в тексте.
func TestDepthLimit(t *testing.T) {
	src := `функция беск(n):
    вернуть беск(n + 1)
печать(беск(1))`
	_, err := runDepth(t, src, 50)
	_, _, msg := evalErr(t, err)
	want := "превышена максимальная глубина вызовов (50). Возможна бесконечная рекурсия."
	if msg != want {
		t.Errorf("msg = %q, хотим %q", msg, want)
	}
	if !isRuntime(err) {
		t.Errorf("категория не ОшибкаВыполнения")
	}
}

// Ссылочные аргументы: мутация Список видна вызывающему.
func TestReferenceArgMutation(t *testing.T) {
	src := `функция доб(s):
    добавить(s, 99)
пусть a = [1]
доб(a)
печать(a)`
	out, err := run(t, src)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if out != "[1, 99]\n" {
		t.Errorf("= %q, хотим [1, 99]", out)
	}
}

// Затенение пусть: имя — значение, не функция → RT-NOT-FUNC.
func TestShadowedNameNotFunction(t *testing.T) {
	src := `пусть длина = 5
печать(длина(2))`
	_, err := run(t, src)
	_, _, msg := evalErr(t, err)
	if msg != "'длина' — не функция (Целое), вызов невозможен" {
		t.Errorf("msg = %q", msg)
	}
	if !isRuntime(err) {
		t.Errorf("категория не ОшибкаВыполнения")
	}
}

// Имя функции в позиции значения → RT-FN-AS-VALUE.
func TestFunctionAsValue(t *testing.T) {
	src := `функция f():
    вернуть 1
печать(f)`
	_, err := run(t, src)
	_, _, msg := evalErr(t, err)
	if msg != "'f' — функция, её нельзя использовать как значение" {
		t.Errorf("msg = %q", msg)
	}
	if !isRuntime(err) {
		t.Errorf("категория не ОшибкаВыполнения")
	}
}
