package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/store"
)

// TestOpenStoreSelectsBackend — замок выбора backend по dbPath (инверсия e,
// open-store.md): "" → MemoryStore + no-op close; непустой → SQLiteStore + рабочий
// Close, файл создан. INV-1: НЕ новый метод Store.
func TestOpenStoreSelectsBackend(t *testing.T) {
	// пусто → Memory, no-op close
	st, closeFn, err := openStore("")
	if err != nil {
		t.Fatalf("openStore(\"\") → ошибка %v", err)
	}
	if _, ok := st.(*store.MemoryStore); !ok {
		t.Errorf("openStore(\"\") = %T, хотим *store.MemoryStore", st)
	}
	if err := closeFn(); err != nil {
		t.Errorf("no-op close → ошибка %v", err)
	}

	// непустой → SQLite, рабочий Close, файл существует
	db := filepath.Join(t.TempDir(), "open.db")
	st2, close2, err := openStore(db)
	if err != nil {
		t.Fatalf("openStore(%q) → ошибка %v", db, err)
	}
	if _, ok := st2.(*store.SQLiteStore); !ok {
		t.Errorf("openStore(%q) = %T, хотим *store.SQLiteStore", db, st2)
	}
	if err := close2(); err != nil {
		t.Errorf("SQLite close → ошибка %v", err)
	}
}

// TestStartArityMismatch — замок арности (инверсия a, §AU-7.3/§AU-10.C): процесс
// эскалация_плана(факт) = 1 параметр. 0 арг → «ожидает 1 аргументов, получено 0»;
// 2 арг → «получено 2». exit 2, дословно (НЕ текст движка). Проверка ДО engine.Start.
func TestStartArityMismatch(t *testing.T) {
	prog := examplePath("контроль_плана.ladix")
	cases := []struct {
		args []string
		want string
	}{
		{
			[]string{"start", prog, "эскалация_плана", "--db", filepath.Join(t.TempDir(), "a0.db")},
			"ladix: процесс 'эскалация_плана' ожидает 1 аргументов, получено 0\n",
		},
		{
			[]string{"start", prog, "эскалация_плана", "100", "200", "--db", filepath.Join(t.TempDir(), "a2.db")},
			"ladix: процесс 'эскалация_плана' ожидает 1 аргументов, получено 2\n",
		},
	}
	for _, c := range cases {
		var out, errBuf bytes.Buffer
		code := realMain(c.args, &out, &errBuf)
		if code != 2 {
			t.Errorf("%v: код = %d, хотим 2; stderr=%q", c.args, code, errBuf.String())
		}
		if errBuf.String() != c.want {
			t.Errorf("%v: stderr = %q, хотим %q", c.args, errBuf.String(), c.want)
		}
	}
}

// TestStartUnknownProcess — замок неизв. процесса (инверсия c, §AU-10.C): start с
// необъявленным именем → «процесс '<имя>' не объявлен» exit 2 (НЕ текст движка
// «не найден в определении»). Проверка ДО engine.Start.
func TestStartUnknownProcess(t *testing.T) {
	var out, errBuf bytes.Buffer
	args := []string{"start", examplePath("контроль_плана.ladix"), "неизвестный", "5",
		"--db", filepath.Join(t.TempDir(), "u.db")}
	code := realMain(args, &out, &errBuf)
	if code != 2 {
		t.Fatalf("код = %d, хотим 2; stderr=%q", code, errBuf.String())
	}
	want := "ladix: процесс 'неизвестный' не объявлен\n"
	if errBuf.String() != want {
		t.Errorf("stderr = %q, хотим %q", errBuf.String(), want)
	}
}

// TestStartZeroArity — арность 0=0 проходит (T014): процесс без параметров, start без
// аргументов → exit 0, инстанс создан.
func TestStartZeroArity(t *testing.T) {
	var out, errBuf bytes.Buffer
	args := []string{"start", testdataPath("старт_терминальный.ladix"), "пинг",
		"--db", filepath.Join(t.TempDir(), "z.db")}
	code := realMain(args, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
}

// TestStartGoldenCanon — stdout канон (инверсия d, §AU-10.D): start процесса с
// человеческим первым шагом + срок → ровно две строки, маска <DT>. exit 0,
// детерминизм id на свежей БД (p-000001/t-000001).
func TestStartGoldenCanon(t *testing.T) {
	var out, errBuf bytes.Buffer
	args := []string{"start", examplePath("контроль_плана.ladix"), "эскалация_плана", "2500000",
		"--db", filepath.Join(t.TempDir(), "canon.db")}
	code := realMain(args, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
	want := "" +
		"[задача] t-000001 → менеджер, шаг 'связаться_с_клиентом', срок до <DT>\n" +
		"запущен инстанс p-000001\n"
	if got := maskDeadlines(out.String()); got != want {
		t.Errorf("stdout (с маской <DT>) байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}
}

// TestStartTerminalNoTasks — терминальный процесс без задач (T017, §AU-10.D): движок
// тих по [задача] → stdout = ровно «запущен инстанс <id>».
func TestStartTerminalNoTasks(t *testing.T) {
	var out, errBuf bytes.Buffer
	args := []string{"start", testdataPath("старт_терминальный.ladix"), "пинг",
		"--db", filepath.Join(t.TempDir(), "term.db")}
	code := realMain(args, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if out.String() != "запущен инстанс p-000001\n" {
		t.Errorf("stdout = %q, хотим ровно «запущен инстанс p-000001\\n»", out.String())
	}
}

// TestStartLiteralBinding — связывание типизированного литерала конец-в-конец
// (инверсия b, T018): start приём 2500000 связывает факт с Целое{2500000};
// тело шага печатает `факт * 2` = 5000000 (арифметика возможна ТОЛЬКО для Целое;
// при Строке умножение → ОшибкаТипа, замок краснеет).
func TestStartLiteralBinding(t *testing.T) {
	var out, errBuf bytes.Buffer
	args := []string{"start", testdataPath("старт_литерал.ladix"), "приём", "2500000",
		"--db", filepath.Join(t.TempDir(), "bind.db")}
	code := realMain(args, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	want := "удвоено: 5000000\nзапущен инстанс p-000001\n"
	if out.String() != want {
		t.Errorf("stdout = %q, хотим %q", out.String(), want)
	}
}

// TestStartBadLiteralCLI — плохой литерал на CLI-уровне (§AU-10.C, T019): целое вне
// Int64 → «не удалось разобрать аргумент '<argv>': целое вне диапазона типа Целое»
// exit 2 ДО engine.Start.
func TestStartBadLiteralCLI(t *testing.T) {
	var out, errBuf bytes.Buffer
	args := []string{"start", testdataPath("старт_литерал.ladix"), "приём", "99999999999999999999",
		"--db", filepath.Join(t.TempDir(), "bad.db")}
	code := realMain(args, &out, &errBuf)
	if code != 2 {
		t.Fatalf("код = %d, хотим 2; stderr=%q", code, errBuf.String())
	}
	want := "ladix: не удалось разобрать аргумент '99999999999999999999': целое вне диапазона типа Целое\n"
	if errBuf.String() != want {
		t.Errorf("stderr = %q, хотим %q", errBuf.String(), want)
	}
}

// TestStartBadWebhookURL — вебхук-проводка (§AU-4.5, T020): невалидный --webhook →
// «неверный URL вебхука '<URL>'» exit 2 (через openExternalCaller, B2). Валидный
// путь без вебхука → дефолт-стаб, exit 0.
func TestStartBadWebhookURL(t *testing.T) {
	var out, errBuf bytes.Buffer
	args := []string{"start", testdataPath("старт_терминальный.ladix"), "пинг",
		"--webhook", "://плохо", "--db", filepath.Join(t.TempDir(), "wh.db")}
	code := realMain(args, &out, &errBuf)
	if code != 2 {
		t.Fatalf("код = %d, хотим 2; stderr=%q", code, errBuf.String())
	}
	want := "ladix: неверный URL вебхука '://плохо'\n"
	if errBuf.String() != want {
		t.Errorf("stderr = %q, хотим %q", errBuf.String(), want)
	}

	// без вебхука → дефолт-стаб, старт проходит
	var out2, err2 bytes.Buffer
	ok := realMain([]string{"start", testdataPath("старт_терминальный.ladix"), "пинг",
		"--db", filepath.Join(t.TempDir(), "wh2.db")}, &out2, &err2)
	if ok != 0 {
		t.Errorf("без вебхука: код = %d, хотим 0; stderr=%q", ok, err2.String())
	}
}

// TestStartDefaultDB — дефолт --db = SQLite ladix.db (инверсия e, D-AU-10, T021):
// start БЕЗ --db персистит инстанс в SQLite-файл `ladix.db` в cwd (НЕ эфемерный
// Memory). Изоляция: cwd → t.TempDir() (восстанавливается defer), туда же копия
// фикстуры. После start: файл `ladix.db` существует И задача читается через
// `tasks` (дефолт ladix.db). Дефолт мутирован на "" (Memory) → файл не создан /
// инстанс не персистнут → красный.
func TestStartDefaultDB(t *testing.T) {
	dir := t.TempDir()
	// фикстура (человеческий шаг + срок) рядом, имя без пути — резолв от cwd
	fixture := readFile(t, examplePath("контроль_плана.ladix"))
	writeFile(t, filepath.Join(dir, "fix.ladix"), fixture)

	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(prevWD)

	// start БЕЗ --db → дефолт SQLite ladix.db в cwd
	var out, errBuf bytes.Buffer
	code := realMain([]string{"start", "fix.ladix", "эскалация_плана", "2500000"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("start: код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	// файл ladix.db создан (НЕ Memory)
	if _, serr := os.Stat("ladix.db"); serr != nil {
		t.Fatalf("ladix.db не создан после start без --db (дефолт Memory?): %v", serr)
	}
	// та же дефолт-БД: задача читается → персист подтверждён
	var tout, terr bytes.Buffer
	tcode := realMain([]string{"tasks"}, &tout, &terr)
	if tcode != 0 {
		t.Fatalf("tasks: код = %d, хотим 0; stderr=%q", tcode, terr.String())
	}
	want := "t-000001  p-000001  'связаться_с_клиентом'  менеджер  срок до <DT>\n"
	if got := maskDeadlines(tout.String()); got != want {
		t.Errorf("tasks stdout (маска <DT>) = %q, хотим %q (инстанс не персистнут?)", got, want)
	}
}

// readFile/writeFile — вспомогательные для изоляции cwd-теста (дефолт-БД).
func readFile(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("чтение %q: %v", p, err)
	}
	return b
}

func writeFile(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatalf("запись %q: %v", p, err)
	}
}
