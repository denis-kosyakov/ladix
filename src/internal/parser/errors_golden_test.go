package parser

import "testing"

// T041: полный golden-каталог — все 7 кодов SE ДОСЛОВНО в двухстрочном формате с
// корректной (строка, колонка) 1-based в рунах (SC-003, FR-017/FR-024).

// assertGolden разбирает src, требует РОВНО одну ошибку и сверяет её двухстрочный
// вид с want.
func assertGolden(t *testing.T, src, want string) {
	t.Helper()
	_, el := parseProgramSrc(t, src)
	if el.Len() != 1 {
		t.Fatalf("src %q: ошибок %d, хотим 1:\n%s", src, el.Len(), el.Error())
	}
	if got := el.Errors()[0].Error(); got != want {
		t.Errorf("src %q:\n got  = %q\nхотим = %q", src, got, want)
	}
}

func TestGoldenSEChain(t *testing.T) {
	assertGolden(t, "1 < y < 10\n",
		"Ошибка в строке 1, колонка 7:\nсравнения нельзя сцеплять, используйте 'и': 1 < x и x < 10")
}

func TestGoldenSENestedFn(t *testing.T) {
	assertGolden(t, "функция f():\n    функция g():\n        вернуть 1\n",
		"Ошибка в строке 2, колонка 5:\nвложенные функции не поддерживаются в v1")
}

func TestGoldenSEIntRange(t *testing.T) {
	assertGolden(t, "99999999999999999999\n",
		"Ошибка в строке 1, колонка 1:\nцелочисленный литерал вне диапазона типа Целое '99999999999999999999'")
}

func TestGoldenSEExpectedEOF(t *testing.T) {
	// незакрытый вызов на EOF (внутри скобок NEWLINE прозрачен)
	assertGolden(t, "печать(1, 2",
		"Ошибка в строке 1, колонка 12:\nожидалось ')', получено 'конец файла'")
}

func TestGoldenSEExpectedColon(t *testing.T) {
	// если без двоеточия → ожидалось ':', получено 'конец строки'
	assertGolden(t, "если x\n    a = 1\n",
		"Ошибка в строке 1, колонка 7:\nожидалось ':', получено 'конец строки'")
}

func TestGoldenSEEmptyBlock(t *testing.T) {
	assertGolden(t, "если x:\nпечать(1)\n",
		"Ошибка в строке 2, колонка 1:\nпустой блок не допускается, добавьте хотя бы один оператор")
}

func TestGoldenSEAssignTarget(t *testing.T) {
	assertGolden(t, "x.поле = 5\n",
		"Ошибка в строке 1, колонка 8:\nневерная цель присваивания: слева от '=' допустима только переменная")
}

func TestGoldenSEUnexpectedTopLevel(t *testing.T) {
	leads := []string{"источник", "метрика", "процесс", "когда", "значение", "{"}
	for _, lead := range leads {
		t.Run(lead, func(t *testing.T) {
			assertGolden(t, lead+"\n",
				"Ошибка в строке 1, колонка 1:\nнеожиданный токен '"+lead+"'")
		})
	}
}
