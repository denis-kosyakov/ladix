package main

import (
	"bytes"
	"path/filepath"
	"strings"
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

// assertNegativeExample прогоняет негатив-пример витрины через realMain и утверждает:
// (а) код выхода РОВНО 1; (б) stderr БАЙТ-В-БАЙТ равен канону §13 этого класса
// (две строки: «Ошибка в строке N, колонка M:\n<сообщение>\n»), пинённому из
// фактического вывода бинаря; (в) пустой stdout; (г) НЕТ Go stack trace.
//
// Полный байт-пин stderr — намеренно (находка analyze F1): он не даёт пройти ни
// пустому stderr+exit1, ни смене класса ошибки, ни сдвигу строки/колонки. Различение
// классов — по фактическому тексту сообщения, заодно с каноном, а не вместо него.
func assertNegativeExample(t *testing.T, name, wantStderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", examplePath(name)}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("%s: код = %d, хотим 1; stderr=%q", name, code, errBuf.String())
	}
	if got := errBuf.String(); got != wantStderr {
		t.Errorf("%s: stderr байт-не-точен:\nполучено %q\nхотим   %q", name, got, wantStderr)
	}
	if out.Len() != 0 {
		t.Errorf("%s: stdout не пуст: %q", name, out.String())
	}
	if s := errBuf.String(); strings.Contains(s, ".go:") || strings.Contains(s, "goroutine") {
		t.Errorf("%s: в stderr просочился Go stack trace: %q", name, s)
	}
}

// T028 (010-A1, Phase F, SC-003) — Golden CSV-демо: прогон examples/источник_csv.ladix
// через CLI `run`. Метрика БЕЗ периода/по_дате (фильтр `где` + сумма) → дата-независима,
// поэтому байт-точный stdout детерминирован под прод-Clock (без FixedClock). Источник
// адресует data/orders.csv относительно cwd → прогон из корня репо (withRepoRoot).
// Оплаченные заказы CSV: 1200000 + 800000.50 + 300000 = 2300000.5 (отменённый 450000
// отфильтрован `где статус == "оплачен"`).
// 🔁 ИНВЕРСИЯ: если CSV-загрузка/коэрсия/фильтр разошлись → stdout разойдётся → красный.
func TestCLIGoldenSourceCSV(t *testing.T) {
	want := "выручка оплаченных (CSV): 2300000.5\n"
	withRepoRoot(t, func() {
		var out, errBuf bytes.Buffer
		code := realMain([]string{"run", filepath.Join("examples", "источник_csv.ladix")}, &out, &errBuf)
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

// T029 (010-A1, Phase F, US2, §SC-10 #6) — NEGATIVE-замок examples/ошибочная.ladix:
// объявленный тип поля `сумма_заказа: Целое` НЕ совпадает с дробным 800000.50 в
// data/orders.json (запись 2) → §SC-9.B «ожидался Целое, получено Дробное», exit 1,
// канон §13, без Go stack trace. Источник адресует data/orders.json относительно cwd →
// прогон из корня репо (withRepoRoot); assertNegativeExample неприменим (его examplePath
// адресует из каталога пакета, а пример требует cwd=корень для резолва data/orders.json).
// stderr пиннится из T026 (фактический прогон бинаря).
// 🔁 ИНВЕРСИЯ: если коэрсия начнёт демоутить Дробное→Целое или сменит класс ошибки →
// красный.
func TestCLINegativeSourceSchema(t *testing.T) {
	wantStderr := "Ошибка в строке 9, колонка 1:\n" +
		"источник 'заказы': запись 2, поле 'сумма_заказа': ожидался Целое, получено Дробное\n"
	withRepoRoot(t, func() {
		var out, errBuf bytes.Buffer
		code := realMain([]string{"run", filepath.Join("examples", "ошибочная.ladix")}, &out, &errBuf)
		if code != 1 {
			t.Fatalf("код = %d, хотим 1; stderr=%q", code, errBuf.String())
		}
		if got := errBuf.String(); got != wantStderr {
			t.Errorf("stderr байт-не-точен:\nполучено %q\nхотим   %q", got, wantStderr)
		}
		if out.Len() != 0 {
			t.Errorf("stdout не пуст: %q", out.String())
		}
		if s := errBuf.String(); strings.Contains(s, ".go:") || strings.Contains(s, "goroutine") {
			t.Errorf("в stderr просочился Go stack trace: %q", s)
		}
	})
}

// T026 (US3, класс 5 — СИНТАКСИЧЕСКАЯ ошибка) — examples/ошибка_синтаксис.ladix
// падает на ПАРСИНГЕ (незакрытая скобка вызова) → канон §13 «ожидалось ')'…», exit 1.
// Пример НЕ входит в TestExamplesParseCleanSet (он обязан НЕ парситься).
// ИНВЕРСИЯ: если перестал быть синтаксической ошибкой / stderr опустел / код ≠ 1 — красный.
func TestCLINegativeSyntax(t *testing.T) {
	assertNegativeExample(t, "ошибка_синтаксис.ladix",
		"Ошибка в строке 5, колонка 1:\nожидалось ')', получено 'конец файла'\n")
}

// T026 (US3, класс 6 — ТИПОВАЯ/семантическая ошибка) — examples/ошибка_тип.ladix
// парсится ЧИСТО, падает в РАНТАЙМЕ на несовместимости типов (Целое + Строка) →
// канон §13 «'+' нельзя применить к Целое и Строка», exit 1.
// ИНВЕРСИЯ: смена класса (напр. парс-ошибка) / иной текст / код ≠ 1 — красный.
func TestCLINegativeType(t *testing.T) {
	assertNegativeExample(t, "ошибка_тип.ladix",
		"Ошибка в строке 5, колонка 19:\n'+' нельзя применить к Целое и Строка\n")
}

// T026 (US3, класс 7 — ошибка ПРОЦЕССА) — examples/ошибка_процесс.ladix парсится
// ЧИСТО, падает в РАНТАЙМЕ на процессной операции «запустить процесс» с необъявленным
// процессом → канон §13 «процесс 'выдача_заказа' не объявлен», exit 1.
// ИНВЕРСИЯ: смена класса / иной текст / код ≠ 1 — красный.
func TestCLINegativeProcess(t *testing.T) {
	assertNegativeExample(t, "ошибка_процесс.ladix",
		"Ошибка в строке 3, колонка 12:\nпроцесс 'выдача_заказа' не объявлен\n")
}

// 012-mdx-diagnostics (US1 витрина DX1) — examples/ошибка_каскад.ladix: ведущее
// ключевое слово в позиции выражения. Раньше — каскад из 2 фантомных диагностик;
// после DX1 — РОВНО ОДНА (полный байт-пин stderr ловит вторую/иной текст/код≠1).
func TestCLINegativeCascade(t *testing.T) {
	assertNegativeExample(t, "ошибка_каскад.ladix",
		"Ошибка в строке 4, колонка 15:\nнеожиданный элемент 'если'\n")
}

// 012-mdx-diagnostics (US2 витрина DX2) — examples/ошибка_подсказка.ladix: опечатка
// имени атрибута источника → бизнес-понятный текст + дешёвая подсказка Левенштейна
// «возможно, вы имели в виду 'файл'?». ИНВЕРСИЯ: пропажа подсказки / жаргон / код≠1.
func TestCLINegativeSuggestion(t *testing.T) {
	assertNegativeExample(t, "ошибка_подсказка.ladix",
		"Ошибка в строке 7, колонка 5:\nнеизвестный атрибут 'фал'; возможно, вы имели в виду 'файл'?\n")
}
