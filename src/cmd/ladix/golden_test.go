package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

// T048/SC-001: сквозной CLI-golden всех 6 обязательных примеров — байт-в-байт
// stdout (§10.3) и код возврата 0. выручка/онбординг НЕ включены (отдельный трек).
func TestCLIGoldenStdout(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"hello.ladix", "Привет, Уклад!\n"},
		{"арифметика.ladix", "14 20\n3 2\n3.4\n25\nистина\nистина\n"},
		{"условие.ladix", "категория: средний\nчётная сумма\n"},
		{"цикл.ladix", "сумма чётных: 30\nдорогие: [250, 400]\nпервая степень двойки > 16: 32\n"},
		{"функция.ladix", "2175.0\n0\n"},
		{"факториал.ladix", "120\n1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := realMain([]string{"run", examplePath(tt.name)}, &out, &errBuf)
			if code != 0 {
				t.Fatalf("%s: код = %d, хотим 0; stderr=%q", tt.name, code, errBuf.String())
			}
			if out.String() != tt.want {
				t.Errorf("%s:\nполучено %q\nхотим   %q", tt.name, out.String(), tt.want)
			}
		})
	}
}

// T016 (US3, класс 1) — событие-триггер `когда событие <Ident>:` под `run` даёт
// строку-заглушку «… требует serve (фича 007b)» (паттерн TestRunTriggerEventScheduleStubGolden),
// exit 0, тело НЕ исполняется. Прогон витрины примера examples/событие.ladix напрямую
// через examplePath/realMain — байт-точный stdout, чистый stderr.
// ИНВЕРСИЯ: если пример сломан/вывод разошёлся/тело начало исполняться — красный.
func TestCLIGoldenEvent(t *testing.T) {
	want := "событие триггер 'заявка_создана' требует serve (фича 007b)\n"
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", examplePath("событие.ladix")}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
	if out.String() != want {
		t.Errorf("stdout байт-не-точен:\nполучено %q\nхотим   %q", out.String(), want)
	}
}

// T018 (US3, класс 2) — расписание-триггеры обеих форм (`каждые Nдн`, `в "ЧЧ:ММ"`)
// под `run` дают строки-заглушки «… требует serve (фича 007b)» в порядке объявления,
// exit 0, тело НЕ исполняется. Прогон examples/расписание.ladix напрямую.
// ИНВЕРСИЯ: пример сломан/порядок/вывод разошёлся → красный.
func TestCLIGoldenSchedule(t *testing.T) {
	want := "" +
		"расписание триггер '1дн' требует serve (фича 007b)\n" +
		"расписание триггер '09:00' требует serve (фича 007b)\n"
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", examplePath("расписание.ladix")}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
	if out.String() != want {
		t.Errorf("stdout байт-не-точен:\nполучено %q\nхотим   %q", out.String(), want)
	}
}

// T020 (US3, класс 3) — мультиисточник/мультиметрика: examples/метрики.ladix объявляет
// два ИСТОЧНИКА (заказы=data/sales.json, расходы=data/costs.json) и три МЕТРИКИ БЕЗ
// периода/по_дате, поэтому значения детерминированы (не зависят от сегодня()): на
// фиксированных данных метрики печатаются метрика-триггерами (форма 007a) одними и
// теми же числами в порядке объявления. Источники адресуют data/*.json относительно
// cwd → прогон из корня репо (withRepoRoot, зеркало metric_test.go), путь к примеру
// также из корня (examples/метрики.ladix). Байт-точный stdout.
// ИНВЕРСИЯ: данные/метрики/порядок изменились → вывод разойдётся → красный.
func TestCLIGoldenMetrics(t *testing.T) {
	want := "" +
		"выручка оплаченных: 2000000\n" +
		"всего заказов: 3\n" +
		"расходы оплаченных: 1000000\n"
	withRepoRoot(t, func() {
		var out, errBuf bytes.Buffer
		code := realMain([]string{"run", filepath.Join("examples", "метрики.ladix")}, &out, &errBuf)
		if code != 0 {
			t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
		}
		if errBuf.Len() != 0 {
			t.Errorf("непустой stderr: %q", errBuf.String())
		}
		if out.String() != want {
			t.Errorf("stdout байт-не-точен:\nполучено %q\nхотим   %q", out.String(), want)
		}
	})
}
