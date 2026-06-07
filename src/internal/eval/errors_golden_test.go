package eval

import "testing"

// Исчерпывающее exact-match покрытие реестра §8.3 (contracts/runtime-errors.md
// RE-1): по одному golden-кейсу на каждый из 30 кодов — категория (тип Go-ошибки),
// позиция (строка + колонка в РУНАХ) и ДОСЛОВНЫЙ текст (SC-003).
// Вариант «значение — не функция …» (не-Ident callee, тот же код RT-NOT-FUNC, но
// субъект — «значение») покрыт отдельно в TestNonIdentCalleeNotFunc.
func TestErrorsRegistryExactMatch(t *testing.T) {
	const (
		sem = "sem"
		ty  = "type"
		rt  = "runtime"
	)
	tests := []struct {
		code      string
		src       string
		maxDepth  int
		line, col int
		msg, cat  string
	}{
		{
			code: "RT-DIV-ZERO", src: `печать(1 / 0)`,
			line: 1, col: 10, cat: rt, msg: "деление на ноль",
		},
		{
			code: "RT-OVERFLOW", src: `печать(9999999999 * 9999999999)`,
			line: 1, col: 19, cat: rt, msg: "переполнение целого числа",
		},
		{
			code: "RT-INDEX-RANGE", src: `печать([1, 2][5])`,
			line: 1, col: 8, cat: rt, msg: "индекс 5 вне диапазона (длина 2)",
		},
		{
			code: "RT-DEPTH",
			src: `функция f():
    вернуть f()
печать(f())`,
			maxDepth: 10, line: 2, col: 13, cat: rt,
			msg: "превышена максимальная глубина вызовов (10). Возможна бесконечная рекурсия.",
		},
		{
			code: "TY-ADD", src: `печать("a" + 1)`,
			line: 1, col: 12, cat: ty, msg: "'+' нельзя применить к Строка и Целое",
		},
		{
			code: "TY-BINOP", src: `печать("a" - "b")`,
			line: 1, col: 12, cat: ty, msg: "'-' нельзя применить к Строка и Строка",
		},
		{
			code: "TY-NEG", src: `печать(-"a")`,
			line: 1, col: 8, cat: ty, msg: "унарный '-' нельзя применить к Строка",
		},
		{
			code: "TY-NOT", src: `печать(не 1)`,
			line: 1, col: 8, cat: ty, msg: "'не' нельзя применить к Целое",
		},
		{
			code: "TY-LOGIC", src: `печать(1 и истина)`,
			line: 1, col: 10, cat: ty, msg: "'и' требует Булево, получено Целое",
		},
		{
			code: "TY-COND",
			src: `если 1:
    печать(1)`,
			line: 1, col: 6, cat: ty, msg: "условие должно быть Булево, получено Целое",
		},
		{
			code: "TY-FOR",
			src: `для x в 5:
    печать(x)`,
			line: 1, col: 9, cat: ty, msg: "'для' требует Список, получено Целое",
		},
		{
			code: "TY-INDEX-NONIDX",
			src: `пусть n = 5
печать(n[0])`,
			line: 2, col: 8, cat: ty, msg: "значение типа Целое не индексируется",
		},
		{
			code: "TY-INDEX-TYPE", src: `печать([1, 2]["x"])`,
			line: 1, col: 8, cat: ty, msg: "индекс должен быть Целое, получено Строка",
		},
		{
			code: "TY-FIELD",
			src: `пусть s = [1]
печать(s.поле)`,
			line: 2, col: 8, cat: ty, msg: "значение типа Список не имеет полей",
		},
		{
			code: "SEM-REDECL-VAR",
			src: `пусть x = 1
пусть x = 2`,
			line: 2, col: 7, cat: sem, msg: "'x' уже объявлено в строке 1",
		},
		{
			code: "RT-VAR-UNDECLARED", src: `печать(b)`,
			line: 1, col: 8, cat: rt, msg: "'b' не объявлено",
		},
		{
			code: "SEM-FN-UNDECLARED", src: `печать(нету(1))`,
			line: 1, col: 8, cat: sem, msg: "функция 'нету' не объявлена",
		},
		{
			code: "SEM-REDECL-FN",
			src: `функция f():
    вернуть 1
функция f():
    вернуть 2`,
			line: 3, col: 9, cat: sem, msg: "функция 'f' уже объявлена в строке 1",
		},
		{
			code: "SEM-RETURN-CTX", src: `вернуть 1`,
			line: 1, col: 1, cat: sem, msg: "'вернуть' допустимо только внутри функции",
		},
		{
			code: "SEM-BREAK-CTX", src: `прервать`,
			line: 1, col: 1, cat: sem, msg: "'прервать' допустимо только внутри 'пока' или 'для'",
		},
		{
			code: "SEM-CONTINUE-CTX", src: `продолжить`,
			line: 1, col: 1, cat: sem, msg: "'продолжить' допустимо только внутри 'пока' или 'для'",
		},
		{
			code: "SEM-ARITY",
			src: `функция f(a, b):
    вернуть a
печать(f(1))`,
			line: 3, col: 8, cat: sem, msg: "'f' принимает 2 аргументов, передано 1",
		},
		{
			code: "RT-NOT-FUNC",
			src: `пусть длина = 5
печать(длина(2))`,
			line: 2, col: 8, cat: rt, msg: "'длина' — не функция (Целое), вызов невозможен",
		},
		{
			code: "RT-FN-AS-VALUE",
			src: `функция f():
    вернуть 1
печать(f)`,
			line: 3, col: 8, cat: rt, msg: "'f' — функция, её нельзя использовать как значение",
		},
		{
			code: "RT-VARIADIC-ARITY", src: `печать(диапазон(1, 2, 3))`,
			line: 1, col: 8, cat: rt, msg: "'диапазон': неверное число аргументов",
		},
		{
			code: "RT-EMPTY-LIST", src: `печать(среднее([]))`,
			line: 1, col: 8, cat: rt, msg: "среднее: список пуст",
		},
		{
			code: "RT-SLICE-RANGE", src: `печать(срез([1], 0, 5))`,
			line: 1, col: 8, cat: rt, msg: "срез: индексы вне диапазона",
		},
		{
			code: "RT-LIST-MUTATED",
			src: `пусть s = [1]
для x в s:
    добавить(s, x)`,
			line: 3, col: 5, cat: rt, msg: "список изменён во время итерации",
		},
		{
			code: "SEM-DEFERRED-CONSTRUCT", src: `пусть x = 3дн`,
			line: 1, col: 11, cat: sem, msg: "конструкция литерал длительности не поддерживается в этой версии",
		},
		{
			code: "SEM-DEFERRED-BUILTIN", src: `печать(сегодня())`,
			line: 1, col: 8, cat: sem, msg: "функция 'сегодня' не поддерживается в этой версии",
		},
	}

	catOf := func(err error) string {
		switch {
		case isSem(err):
			return "sem"
		case isType(err):
			return "type"
		case isRuntime(err):
			return "runtime"
		}
		return "?"
	}

	seen := map[string]bool{}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			seen[tt.code] = true
			_, err := runDepth(t, tt.src, tt.maxDepth)
			if err == nil {
				t.Fatalf("%s: ожидалась ошибка", tt.code)
			}
			line, col, msg := evalErr(t, err)
			if msg != tt.msg {
				t.Errorf("%s: msg = %q, хотим %q", tt.code, msg, tt.msg)
			}
			if got := catOf(err); got != tt.cat {
				t.Errorf("%s: категория = %s, хотим %s", tt.code, got, tt.cat)
			}
			if line != tt.line || col != tt.col {
				t.Errorf("%s: позиция = (%d,%d), хотим (%d,%d)", tt.code, line, col, tt.line, tt.col)
			}
		})
	}
	if len(seen) != 30 {
		t.Errorf("покрыто кодов = %d, хотим 30", len(seen))
	}
}

// Не-Ident callee (§8.3, строка «значение — не функция (<тип>), вызов невозможен»):
// у произвольного выражения нет имени, поэтому субъект — «значение», а не «'имя'».
// Тот же код RT-NOT-FUNC, что у Ident-ветки; позиция = CallExpr.Pos() (= Callee.Pos()).
func TestNonIdentCalleeNotFunc(t *testing.T) {
	const src = `печать((5)(3))`
	_, err := run(t, src)
	if err == nil {
		t.Fatal("ожидалась ОшибкаВыполнения")
	}
	line, col, msg := evalErr(t, err)
	if msg != "значение — не функция (Целое), вызов невозможен" {
		t.Errorf("msg = %q", msg)
	}
	if !isRuntime(err) {
		t.Errorf("категория не ОшибкаВыполнения")
	}
	// Callee — литерал 5 внутри скобок; CallExpr.Pos() = его позиция.
	if line != 1 || col != 9 {
		t.Errorf("позиция = (%d,%d), хотим (1,9)", line, col)
	}
}
