package eval

import "testing"

// печать — единственный канал вывода: представления §7 через пробел + \n.
func TestPrintChannel(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"несколько аргументов", `печать(1, "две", истина)`, "1 две истина\n"},
		{"пустая печать", `печать()`, "\n"},
		{"список без кавычек", `печать([1, "две"])`, "[1, две]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := run(t, tt.src)
			if err != nil {
				t.Fatalf("ошибка: %v", err)
			}
			if out != tt.want {
				t.Errorf("= %q, хотим %q", out, tt.want)
			}
		})
	}
}

// Функциональный scope: ветка если НЕ создаёт области — присваивание мутирует
// внешнюю переменную (контракт условие.ladix); чтение до объявления → рантайм.
func TestFunctionalScope(t *testing.T) {
	src := `пусть категория = ""
если 1 < 2:
    категория = "да"
печать(категория)`
	out, err := run(t, src)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if out != "да\n" {
		t.Errorf("= %q, хотим %q", out, "да\n")
	}
}

// печать(b) до «пусть b» — рантайм RT-VAR-UNDECLARED (flow-зависимо, НЕ семпроход).
func TestUseBeforeDeclareRuntime(t *testing.T) {
	src := `печать(b)
пусть b = 1`
	_, err := run(t, src)
	line, col, msg := evalErr(t, err)
	if msg != "'b' не объявлено" {
		t.Errorf("msg = %q", msg)
	}
	if !isRuntime(err) {
		t.Errorf("категория не ОшибкаВыполнения")
	}
	if line != 1 || col != 8 {
		t.Errorf("позиция = (%d,%d), хотим (1,8)", line, col)
	}
}

// Присваивание необъявленной → RT-VAR-UNDECLARED на позиции lvalue.
func TestAssignUndeclared(t *testing.T) {
	_, err := run(t, `x = 5`)
	_, _, msg := evalErr(t, err)
	if msg != "'x' не объявлено" {
		t.Errorf("msg = %q", msg)
	}
}

// если/иначе если/иначе по IsFinal(); strict-Булево.
func TestIfChain(t *testing.T) {
	src := `пусть n = 4200
пусть к = ""
если n >= 10000:
    к = "крупный"
иначе если n >= 3000:
    к = "средний"
иначе:
    к = "мелкий"
печать(к)`
	out, err := run(t, src)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if out != "средний\n" {
		t.Errorf("= %q", out)
	}
}

func TestStrictBoolCondition(t *testing.T) {
	_, err := run(t, `если 1:
    печать("нет")`)
	line, _, msg := evalErr(t, err)
	if msg != "условие должно быть Булево, получено Целое" {
		t.Errorf("msg = %q", msg)
	}
	if !isType(err) {
		t.Errorf("категория не ОшибкаТипа")
	}
	if line != 1 {
		t.Errorf("строка = %d, хотим 1", line)
	}
}

// пока с прервать/продолжить (поглощаются); вернуть пробрасывается выше (в функции).
func TestWhileBreakContinue(t *testing.T) {
	src := `пусть n = 1
пока истина:
    если n > 16:
        прервать
    n = n * 2
печать(n)`
	out, err := run(t, src)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if out != "32\n" {
		t.Errorf("= %q, хотим 32", out)
	}
}

// для x видна после цикла (последний элемент); на пустом списке не создаётся.
func TestForVariableVisibleAfter(t *testing.T) {
	out, err := run(t, `для x в [10, 20, 30]:
    печать(x)
печать(x)`)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if out != "10\n20\n30\n30\n" {
		t.Errorf("= %q", out)
	}
}

func TestForEmptyListVarNotCreated(t *testing.T) {
	_, err := run(t, `для x в []:
    печать(x)
печать(x)`)
	_, _, msg := evalErr(t, err)
	if msg != "'x' не объявлено" {
		t.Errorf("msg = %q (x не должна создаваться на пустом списке)", msg)
	}
}

// для над не-Список → TY-FOR на позиции Iterable.
func TestForNonList(t *testing.T) {
	_, err := run(t, `для x в 5:
    печать(x)`)
	_, _, msg := evalErr(t, err)
	if msg != "'для' требует Список, получено Целое" {
		t.Errorf("msg = %q", msg)
	}
	if !isType(err) {
		t.Errorf("категория не ОшибкаТипа")
	}
}

// Защита от мутации списка во время итерации (добавить на «итерируется»).
func TestListMutatedDuringIteration(t *testing.T) {
	_, err := run(t, `пусть s = [1, 2, 3]
для x в s:
    добавить(s, x)`)
	_, _, msg := evalErr(t, err)
	if msg != "список изменён во время итерации" {
		t.Errorf("msg = %q", msg)
	}
	if !isRuntime(err) {
		t.Errorf("категория не ОшибкаВыполнения")
	}
}

// Снимок через копия — итерация по копии, мутация оригинала допустима.
func TestForOverCopyAllowsMutation(t *testing.T) {
	out, err := run(t, `пусть s = [1, 2]
для x в копия(s):
    добавить(s, x)
печать(длина(s))`)
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if out != "4\n" {
		t.Errorf("= %q, хотим 4", out)
	}
}
