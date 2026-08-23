package eval

import (
	"bytes"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
)

// buildTriggerInterp прогоняет лексер→парсер→Analyze→Run на src и возвращает
// готовый интерпретатор (триггеры зарегистрированы Analyze) + буфер stdout тела.
func buildTriggerInterp(t *testing.T, src string) (*Interpreter, *bytes.Buffer) {
	t.Helper()
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("лексические/синтаксические ошибки: %v", errList.Error())
	}
	out := &bytes.Buffer{}
	i := NewInterpreter(out, 0, testClock)
	if err := i.Analyze(prog); err != nil {
		t.Fatalf("Analyze вернул ошибку: %v", err)
	}
	if err := i.Run(prog); err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}
	return i, out
}

// T016 (016 B4a, §AU-6.1): эскалация-триггер под `run` даёт ровно строку-заглушку
// «задача триггер '<процесс>.<шаг>' требует serve (фича 007b)» в w (порядок
// объявления, зеркало событие/расписание); тело НЕ исполняется (out пуст), exit 0.
// 🔁 ИНВЕРСИЯ: если исполнять тело под run → out непуст / w разойдётся → красный;
// если изменить текст заглушки → exact-match w красный.
func TestDeadlineTriggerRunStub(t *testing.T) {
	const proc = "процесс согласование(заявка):\n    шаг проверка:\n        печать(заявка)\n"
	// Тело — канон §AU-11.1 `уведомить руководитель(факт)` со свободным `факт` (резолв
	// в рантайме, B4b). Семпроход принимает `уведомить` в теле триггера (§AU-6.1.3),
	// но под run тело НЕ исполняется (нет таймера) — out пуст, печатается только стаб.
	src := proc + "когда задача просрочена в согласование.проверка:\n    уведомить руководитель(факт)\n"
	i, out := buildTriggerInterp(t, src)

	var w bytes.Buffer
	if err := i.RunTriggers(&w); err != nil {
		t.Fatalf("RunTriggers вернул ошибку: %v", err)
	}
	want := "задача триггер 'согласование.проверка' требует serve (фича 007b)\n"
	if got := w.String(); got != want {
		t.Errorf("заглушка эскалации = %q, хотим %q", got, want)
	}
	if out.Len() != 0 {
		t.Errorf("тело эскалации не должно исполняться под run, out = %q", out.String())
	}
}
