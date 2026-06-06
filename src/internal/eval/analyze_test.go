package eval

import "testing"

// Семпроход — fail-fast стадия 3: статически разрешимые ошибки.
func TestAnalyzeSemanticErrors(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantMsg string
		sem     bool // ожидается СемантическаяОшибка
	}{
		{
			"повтор пусть в одной области",
			"пусть x = 1\nпусть x = 2",
			"'x' уже объявлено в строке 1",
			true,
		},
		{
			"повтор пусть в обеих ветках если",
			"если 1 < 2:\n    пусть к = 1\nиначе:\n    пусть к = 2",
			"'к' уже объявлено в строке 2",
			true,
		},
		{
			"повтор функции",
			"функция f():\n    вернуть 1\nфункция f():\n    вернуть 2",
			"функция 'f' уже объявлена в строке 1",
			true,
		},
		{
			"вернуть вне функции",
			"вернуть 1",
			"'вернуть' допустимо только внутри функции",
			true,
		},
		{
			"прервать вне цикла",
			"прервать",
			"'прервать' допустимо только внутри 'пока' или 'для'",
			true,
		},
		{
			"продолжить вне цикла",
			"продолжить",
			"'продолжить' допустимо только внутри 'пока' или 'для'",
			true,
		},
		{
			"арность пользовательской функции",
			"функция f(a, b):\n    вернуть a\nпечать(f(1))",
			"'f' принимает 2 аргументов, передано 1",
			true,
		},
		{
			"арность встроенной фикс.",
			"печать(срез([1], 0))",
			"'срез' принимает 3 аргументов, передано 2",
			true,
		},
		{
			"необъявленная функция",
			"печать(нету(1))",
			"функция 'нету' не объявлена",
			true,
		},
		{
			"deferred-встроенная",
			"печать(сегодня())",
			"функция 'сегодня' не поддерживается в этой версии",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := analyzeSrc(t, tt.src)
			if err == nil {
				t.Fatalf("ожидалась ошибка семпрохода")
			}
			_, _, msg := evalErr(t, err)
			if msg != tt.wantMsg {
				t.Errorf("msg = %q, хотим %q", msg, tt.wantMsg)
			}
			if tt.sem && !isSem(err) {
				t.Errorf("категория не СемантическаяОшибка")
			}
		})
	}
}

// Два «для x» подряд в одной области — НЕ повтор (D-R13, «если имени ещё нет»).
func TestAnalyzeRepeatedForVarOK(t *testing.T) {
	src := `для x в [1, 2]:
    печать(x)
для x в [3, 4]:
    печать(x)`
	if err := analyzeSrc(t, src); err != nil {
		t.Fatalf("два «для x» не должны давать ошибку: %v", err)
	}
}

// для x при объявленном пусть x → повтор.
func TestAnalyzeForVarOverLet(t *testing.T) {
	src := `пусть x = 0
для x в [1, 2]:
    печать(x)`
	err := analyzeSrc(t, src)
	_, _, msg := evalErr(t, err)
	if msg != "'x' уже объявлено в строке 1" {
		t.Errorf("msg = %q", msg)
	}
}

// Граница FR-035: declaredness переменной в позиции значения семпроход НЕ ловит
// (печать(b) до «пусть b» проходит Analyze, падает в рантайме).
func TestAnalyzeBoundaryValueIdentNotChecked(t *testing.T) {
	src := `печать(b)
пусть b = 1`
	if err := analyzeSrc(t, src); err != nil {
		t.Errorf("семпроход не должен ловить declaredness переменной: %v", err)
	}
}

// Граница: арность вариативных/перегруженных (печать/диапазон/длина) НЕ
// проверяется на семпроходе.
func TestAnalyzeBoundaryVariadicArityNotChecked(t *testing.T) {
	for _, src := range []string{
		`печать(1, 2, 3)`,
		`пусть r = диапазон(1, 5)`,
		`пусть n = длина([1, 2])`,
	} {
		if err := analyzeSrc(t, src); err != nil {
			t.Errorf("семпроход ошибочно проверил арность %q: %v", src, err)
		}
	}
}

// Forward-вызов функции, объявленной ниже, — разрешён.
func TestAnalyzeForwardCall(t *testing.T) {
	src := `печать(g())
функция g():
    вернуть 1`
	if err := analyzeSrc(t, src); err != nil {
		t.Errorf("forward-вызов должен быть разрешён: %v", err)
	}
}
