package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T022 (Phase F, US2, §SC-10 #5, §SC-9) — NEGATIVE-замки ОШИБОК ДАННЫХ И СЕМАНТИКИ
// источника через CLI (`run`). Зеркало assertNegativeExample (golden_test.go), но
// фикстуры пишутся во временный каталог и прогон идёт ИЗ него (cwd = tempdir), чтобы
// относительные пути `файл:` (и embedded-путь в CSV-ошибке заголовка) резолвились и
// были детерминированы.
//
// Каждый кейс: exit РОВНО 1, stderr БАЙТ-В-БАЙТ равен двухстрочному канону §13
// («Ошибка в строке N, колонка M:\n<сообщение §SC-9>\n»), пустой stdout, без Go
// stack trace. Полный байт-пин stderr — намеренно (находка analyze F1): не проходят
// ни пустой stderr+exit1, ни смена класса ошибки, ни сдвиг строки/колонки.
//
// 🔁 ИНВЕРСИЯ: если кейс перестал падать ИМЕННО этой ошибкой (диспетчер/коэрсия/
// валидатор/дата-парс/семпроверка отрегрессировали) — красный.

// assertSourceNegative пишет файлы fixtures (имя→содержимое) и программу prog в
// t.TempDir(), прогоняет `run <prog>` ИЗ этого каталога и утверждает exit 1 + точный
// stderr + пустой stdout + отсутствие стек-трейса.
func assertSourceNegative(t *testing.T, prog string, fixtures map[string]string, wantStderr string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range fixtures {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("запись фикстуры %s: %v", name, err)
		}
	}
	progPath := filepath.Join(dir, "demo.ladix")
	if err := os.WriteFile(progPath, []byte(prog), 0o644); err != nil {
		t.Fatalf("запись программы: %v", err)
	}

	// Прогон из каталога фикстур, чтобы относительные пути `файл:` резолвились и
	// embedded-путь в §SC-9.B (CSV-заголовок) был детерминирован.
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatal(err)
		}
	}()

	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", "demo.ladix"}, &out, &errBuf)
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
}

// runtimeForceTrigger — хвост-триггер, который ФОРСИРУЕТ вычисление метрики m (а
// значит ленивую загрузку источника s → applySchema). Без него рантайм-ошибки данных
// не возникнут (источник грузится лениво при первом обращении).
const runtimeForceTrigger = "\nкогда метрика m < 100:\n    печать(\"v:\", значение)\n"

// (A1-5) отсутствует объявленное поле — JSON-запись без 'b'.
func TestCLINegativeMissingField(t *testing.T) {
	prog := "" +
		"источник s:\n" +
		"    файл: \"m.json\"\n" +
		"    тип: json\n" +
		"    поля:\n" +
		"        a: Целое\n" +
		"        b: Строка\n\n" +
		"метрика m:\n" +
		"    источник: s\n" +
		"    агрегат: сумма(a)\n" + runtimeForceTrigger
	assertSourceNegative(t, prog, map[string]string{"m.json": `[{"a": 1}]`},
		"Ошибка в строке 1, колонка 1:\nисточник 's': запись 1: отсутствует объявленное поле 'b'\n")
}

// (A1-10) несовпадение типа Дробное→Целое — JSON 1.5 в поле Целое.
func TestCLINegativeTypeMismatch(t *testing.T) {
	prog := "" +
		"источник s:\n" +
		"    файл: \"tm.json\"\n" +
		"    тип: json\n" +
		"    поля:\n" +
		"        a: Целое\n\n" +
		"метрика m:\n" +
		"    источник: s\n" +
		"    агрегат: сумма(a)\n" + runtimeForceTrigger
	assertSourceNegative(t, prog, map[string]string{"tm.json": `[{"a": 1.5}]`},
		"Ошибка в строке 1, колонка 1:\nисточник 's': запись 1, поле 'a': ожидался Целое, получено Дробное\n")
}

// (A1-7) невалидная дата — CSV «2026-13-40» в поле Дата.
func TestCLINegativeInvalidDate(t *testing.T) {
	prog := "" +
		"источник s:\n" +
		"    файл: \"bd.csv\"\n" +
		"    тип: csv\n" +
		"    поля:\n" +
		"        d: Дата\n" +
		"        n: Целое\n\n" +
		"метрика m:\n" +
		"    источник: s\n" +
		"    агрегат: сумма(n)\n" + runtimeForceTrigger
	assertSourceNegative(t, prog, map[string]string{"bd.csv": "d,n\n2026-13-40,5\n"},
		"Ошибка в строке 1, колонка 1:\nисточник 's': запись 1, поле 'd': «2026-13-40» не является датой (ожидался формат ГГГГ-ММ-ДД)\n")
}

// CSV без заголовкового столбца объявленного поля.
func TestCLINegativeCSVMissingHeader(t *testing.T) {
	prog := "" +
		"источник s:\n" +
		"    файл: \"ch.csv\"\n" +
		"    тип: csv\n" +
		"    поля:\n" +
		"        a: Целое\n" +
		"        b: Строка\n\n" +
		"метрика m:\n" +
		"    источник: s\n" +
		"    агрегат: сумма(a)\n" + runtimeForceTrigger
	assertSourceNegative(t, prog, map[string]string{"ch.csv": "a\n1\n"},
		"Ошибка в строке 1, колонка 1:\nисточник 's': в заголовке CSV «ch.csv» отсутствует поле 'b'\n")
}

// (Defect 3) Дубликат столбца в заголовке CSV → load-ошибка §SC-9.B (поз. decl).
func TestCLINegativeCSVDuplicateHeader(t *testing.T) {
	prog := "" +
		"источник s:\n" +
		"    файл: \"dup.csv\"\n" +
		"    тип: csv\n" +
		"    поля:\n" +
		"        ид: Целое\n" +
		"        статус: Строка\n\n" +
		"метрика m:\n" +
		"    источник: s\n" +
		"    агрегат: сумма(ид)\n" + runtimeForceTrigger
	assertSourceNegative(t, prog, map[string]string{"dup.csv": "ид,статус,статус\n7,оплачен,новый\n"},
		"Ошибка в строке 1, колонка 1:\nисточник 's': в заголовке CSV «dup.csv» столбец 'статус' объявлен дважды\n")
}

// (Defect 4) CSV-Дробное со значением Inf → «не является дробным», exit 1.
func TestCLINegativeCSVFloatInf(t *testing.T) {
	prog := "" +
		"источник s:\n" +
		"    файл: \"inf.csv\"\n" +
		"    тип: csv\n" +
		"    поля:\n" +
		"        цена: Дробное\n\n" +
		"метрика m:\n" +
		"    источник: s\n" +
		"    агрегат: сумма(цена)\n" + runtimeForceTrigger
	assertSourceNegative(t, prog, map[string]string{"inf.csv": "цена\nInf\n"},
		"Ошибка в строке 1, колонка 1:\nисточник 's': запись 1, поле 'цена': «Inf» не является дробным\n")
}

// (Defect 4) CSV-Дробное с Go hex-float «0x1p4» → «не является дробным», exit 1.
func TestCLINegativeCSVFloatHex(t *testing.T) {
	prog := "" +
		"источник s:\n" +
		"    файл: \"hex.csv\"\n" +
		"    тип: csv\n" +
		"    поля:\n" +
		"        цена: Дробное\n\n" +
		"метрика m:\n" +
		"    источник: s\n" +
		"    агрегат: сумма(цена)\n" + runtimeForceTrigger
	assertSourceNegative(t, prog, map[string]string{"hex.csv": "цена\n0x1p4\n"},
		"Ошибка в строке 1, колонка 1:\nисточник 's': запись 1, поле 'цена': «0x1p4» не является дробным\n")
}

// Семантика: неизвестный `тип:` (Analyze, позиция TypePos).
func TestCLINegativeSemUnknownType(t *testing.T) {
	prog := "" +
		"источник s:\n" +
		"    файл: \"x.json\"\n" +
		"    тип: xml\n" +
		"    поля:\n" +
		"        a: Целое\n\n" +
		"метрика m:\n" +
		"    источник: s\n" +
		"    агрегат: сумма(a)\n"
	assertSourceNegative(t, prog, map[string]string{"x.json": `[{"a": 1}]`},
		"Ошибка в строке 3, колонка 5:\nисточник 's': неизвестный тип источника 'xml' (допустимо: json, csv, ndjson)\n")
}

// Семантика: csv без `поля:` (Analyze, позиция TypePos).
func TestCLINegativeSemCSVNoFields(t *testing.T) {
	prog := "" +
		"источник s:\n" +
		"    файл: \"x.csv\"\n" +
		"    тип: csv\n\n" +
		"метрика m:\n" +
		"    источник: s\n" +
		"    агрегат: сумма(a)\n"
	assertSourceNegative(t, prog, map[string]string{"x.csv": "a\n1\n"},
		"Ошибка в строке 3, колонка 5:\nисточник 's': тип 'csv' требует объявления полей (поля:)\n")
}

// Семантика: неизвестный тип поля (Analyze, позиция FieldDef.Pos).
func TestCLINegativeSemUnknownFieldType(t *testing.T) {
	prog := "" +
		"источник s:\n" +
		"    файл: \"x.json\"\n" +
		"    тип: json\n" +
		"    поля:\n" +
		"        a: Деньги\n\n" +
		"метрика m:\n" +
		"    источник: s\n" +
		"    агрегат: сумма(a)\n"
	assertSourceNegative(t, prog, map[string]string{"x.json": `[{"a": 1}]`},
		"Ошибка в строке 5, колонка 9:\nисточник 's': неизвестный тип поля 'Деньги' (допустимо: Целое, Дробное, Строка, Логическое, Дата)\n")
}
