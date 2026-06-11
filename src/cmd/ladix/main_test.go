package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func examplePath(name string) string {
	return filepath.Join("..", "..", "..", "examples", name)
}

// deadlineMaskRE — маска времени дедлайна (§EN-9): CLI-тесты не фиксируют абсолютный
// момент (SystemClock), маскируют только «срок до <время>». Формат времени —
// "2006-01-02 15:04" (engine.deadlineLayout).
var deadlineMaskRE = regexp.MustCompile(`срок до \d{4}-\d{2}-\d{2} \d{2}:\d{2}`)

// maskDeadlines заменяет «срок до <время>» на «срок до <DT>» — id детерминированы,
// маскируется только время (§EN-9).
func maskDeadlines(s string) string {
	return deadlineMaskRE.ReplaceAllString(s, "срок до <DT>")
}

// T044/T045: успешный прогон → stdout, код 0.
func TestRunSuccess(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", examplePath("hello.ladix")}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if out.String() != "Привет, Уклад!\n" {
		t.Errorf("stdout = %q", out.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
}

// T047/SC-002: ошибка.ladix → ровно двухстрочный stderr, код 1, без stack trace.
func TestRunErrorExample(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", examplePath("ошибка.ladix")}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1", code)
	}
	want := "Ошибка в строке 5, колонка 14:\nделение на ноль\n"
	if errBuf.String() != want {
		t.Errorf("stderr = %q, хотим %q", errBuf.String(), want)
	}
	if strings.Contains(errBuf.String(), ".go:") || strings.Contains(errBuf.String(), "goroutine") {
		t.Errorf("в stderr просочился Go stack trace: %q", errBuf.String())
	}
}

// Коды использования CLI (код 2): нет подкоманды/файла/неверный флаг/нет файла.
func TestUsageErrors(t *testing.T) {
	cases := [][]string{
		{},                     // нет подкоманды
		{"run"},                // нет файла
		{"run", "--max-depth"}, // флаг без значения
		{"run", "--max-depth", "0", examplePath("hello.ladix")}, // неверное значение
		{"run", "--нечто", examplePath("hello.ladix")},          // неизвестный флаг
		{"run", "нет-такого-файла.ladix"},                       // файл не читается
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := realMain(args, &out, &errBuf)
			if code != 2 {
				t.Errorf("args=%v: код = %d, хотим 2", args, code)
			}
		})
	}
}

// --max-depth пробрасывается в лимит и в текст ошибки RT-DEPTH.
func TestMaxDepthFlag(t *testing.T) {
	// файл с убегающей рекурсией готовим во временном каталоге
	dir := t.TempDir()
	file := filepath.Join(dir, "rec.ladix")
	src := "функция f():\n    вернуть f()\nпечать(f())\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", "--max-depth", "7", file}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1", code)
	}
	if !strings.Contains(errBuf.String(), "превышена максимальная глубина вызовов (7).") {
		t.Errorf("stderr не содержит лимит 7: %q", errBuf.String())
	}
}

// 006/US1 (§EN-9 Сценарий А): онбординг.ladix исполняется через MemoryStore (run
// без --db) — 'запустить процесс' активирован движком. Exit 0, байт-точный golden
// 5 строк. id детерминированы (свежий Store → p-000001/t-000001); маскируется только
// <время> дедлайна (deadlineMaskRE). Сменился вердикт фичи 006 относительно 005,
// где это была рантайм-граница (код 1, §DP-4).
func TestRunOnboardingProcessDeferred(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", examplePath("онбординг.ladix")}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
	want := "" +
		"[уведомление] ИТ: создать учётку для Петров\n" +
		"[задача] t-000001 → руководитель, шаг 'провести_встречу', срок до <DT>\n" +
		"запущен онбординг, id: p-000001\n" +
		"открытых задач: 1\n" +
		"t-000001  p-000001  'провести_встречу'  руководитель  срок до <DT>\n"
	if got := maskDeadlines(out.String()); got != want {
		t.Errorf("stdout (с маской <DT>) байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}
}

// 005/SC-005 (CP-5.3 R1): выручка.ladix остаётся парс-ошибкой код 1, но падение
// сдвинулось — 'процесс' теперь парсится, отвергается top-level 'когда'
// (D-6, триггеры — 007).
func TestRunRevenueUnexpectedWhen(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", examplePath("выручка.ladix")}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1; stderr=%q", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "неожиданный токен 'когда'") {
		t.Errorf("в stderr нет парс-ошибки на 'когда': %q", errBuf.String())
	}
	if strings.Contains(errBuf.String(), "неожиданный токен 'процесс'") {
		t.Errorf("регресс: парсер снова отвергает 'процесс': %q", errBuf.String())
	}
	if out.Len() != 0 {
		t.Errorf("непустой stdout: %q", out.String())
	}
}

// 005/FR-023 (CP-3): программа, ТОЛЬКО объявляющая процесс (без top-level
// 'запустить процесс') → код 0: рантайм отрабатывает, ProcessDecl — Decl,
// не Statement — пропускается циклом Run(); тело шага в 005 не исполняется
// (печать(1) внутри шага не попадает в stdout, §PM-5).
func TestRunProcessDeclOnly(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "процесс.ladix")
	src := "процесс P:\n    шаг A:\n        печать(1)\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", file}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout не пуст — тело шага не должно исполняться: %q", out.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
}

// T023/§EN-9 Сценарий Б — мост SQLite: цепочка из 6 команд на свежей БД. Состояние
// между командами живёт ТОЛЬКО в файле test.db (каждая команда открывает Store
// заново). id детерминированы (свежий Store → p-000001/t-000001…), маска — только
// <время> дедлайнов. Повтор run (шаг 6) даёт p-000002/t-000003 (счётчик персистентен).
func TestScenarioBSQLiteChain(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "test.db")
	file := examplePath("онбординг.ladix")

	type step struct {
		name string
		args []string
		want string
	}
	steps := []step{
		{
			name: "1: run --db",
			args: []string{"run", file, "--db", db},
			want: "" +
				"[уведомление] ИТ: создать учётку для Петров\n" +
				"[задача] t-000001 → руководитель, шаг 'провести_встречу', срок до <DT>\n" +
				"запущен онбординг, id: p-000001\n" +
				"открытых задач: 1\n" +
				"t-000001  p-000001  'провести_встречу'  руководитель  срок до <DT>\n",
		},
		{
			name: "2: tasks --db",
			args: []string{"tasks", "--db", db},
			want: "t-000001  p-000001  'провести_встречу'  руководитель  срок до <DT>\n",
		},
		{
			name: "3: tasks Петров --db",
			args: []string{"tasks", "Петров", "--db", db},
			want: "открытых задач нет\n",
		},
		{
			name: "4: complete t-000001",
			args: []string{"complete", file, "t-000001", "--db", db},
			want: "" +
				"задача t-000001 завершена\n" +
				"[задача] t-000002 → HR, шаг 'закрыть_адаптацию', срок до <DT>\n" +
				"инстанс p-000001: ожидает, шаг 'закрыть_адаптацию'\n",
		},
		{
			name: "5: complete t-000002",
			args: []string{"complete", file, "t-000002", "--db", db},
			want: "" +
				"задача t-000002 завершена\n" +
				"инстанс p-000001: выполнен\n",
		},
		{
			name: "6: run --db (повтор → p-000002/t-000003)",
			args: []string{"run", file, "--db", db},
			want: "" +
				"[уведомление] ИТ: создать учётку для Петров\n" +
				"[задача] t-000003 → руководитель, шаг 'провести_встречу', срок до <DT>\n" +
				"запущен онбординг, id: p-000002\n" +
				"открытых задач: 1\n" +
				"t-000003  p-000002  'провести_встречу'  руководитель  срок до <DT>\n",
		},
	}
	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := realMain(s.args, &out, &errBuf)
			if code != 0 {
				t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
			}
			if errBuf.Len() != 0 {
				t.Errorf("непустой stderr: %q", errBuf.String())
			}
			if got := maskDeadlines(out.String()); got != s.want {
				t.Errorf("stdout (с маской <DT>) байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, s.want)
			}
		})
	}
}

// T047/SC-008: recover-барьер ловит непредвиденную Go-панику → дженерик, код 1.
func TestGuardRecoversPanic(t *testing.T) {
	var errBuf bytes.Buffer
	code := guard(&errBuf, func() int {
		panic("искусственная паника")
	})
	if code != 1 {
		t.Errorf("код = %d, хотим 1", code)
	}
	if errBuf.String() != "внутренняя ошибка интерпретатора\n" {
		t.Errorf("stderr = %q", errBuf.String())
	}
	if strings.Contains(errBuf.String(), "goroutine") {
		t.Errorf("в stderr просочился stack trace")
	}
}
