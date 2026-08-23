package main

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// seedInspectFixture создаёт в свежей SQLite-БД инстанс p-000001 процесса
// эскалация_плана (статус ожидает, шаг 'связаться_с_клиентом', переменная факт=2500000)
// и одну задачу t-000001 с заданными статусом/дедлайном/эскалацией. Прямой SaveInstance/
// SaveTask — детерминированный контроль каждого варианта строки задачи (§AU-10.D).
func seedInspectFixture(t *testing.T, db string, status store.TaskStatus, withDeadline, escalated bool) {
	t.Helper()
	st, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer st.Close()
	created := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	inst := &store.ProcessInstance{
		ID:          "p-000001",
		ProcessName: "эскалация_плана",
		Status:      store.StatusWaiting,
		CurrentStep: "связаться_с_клиентом",
		Variables:   map[string]value.Value{"факт": value.Целое{V: 2500000}},
		CreatedAt:   created,
		UpdatedAt:   created,
	}
	if err := st.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}
	task := &store.Task{
		ID: "t-000001", InstanceID: "p-000001", StepName: "связаться_с_клиентом",
		Assignee: "менеджер", Status: status, CreatedAt: created, Escalated: escalated,
	}
	if withDeadline {
		d := created.Add(72 * time.Hour)
		task.Deadline = &d
	}
	if status == store.TaskCompleted {
		c := created.Add(time.Hour)
		task.CompletedAt = &c
	}
	if err := st.SaveTask(task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
}

// TestInspectGoldenCanon — stdout канон inspect (инверсия b, §AU-10.D): снимок +
// переменные + одна открытая эскалированная задача с дедлайном. exact-match, маска <DT>.
// Любой сдвиг разделителя/слова/отступа → красный.
func TestInspectGoldenCanon(t *testing.T) {
	db := filepath.Join(t.TempDir(), "canon.db")
	seedInspectFixture(t, db, store.TaskPending, true, true)

	var out, errBuf bytes.Buffer
	code := inspectMain([]string{"p-000001", "--db", db}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
	want := "" +
		"инстанс p-000001: процесс эскалация_плана, статус ожидает, шаг 'связаться_с_клиентом'\n" +
		"переменные:\n" +
		"  факт = 2500000\n" +
		"задачи:\n" +
		"  t-000001 шаг 'связаться_с_клиентом' → менеджер, срок до <DT>, открыта, эскалирована\n"
	if got := maskDeadlines(out.String()); got != want {
		t.Errorf("stdout (с маской <DT>) байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}
}

// TestInspectEscalatedSuffix — замок суффикса «, эскалирована» (инверсия c, §AU-10.D):
// (c1) Escalated=true → строка оканчивается «, открыта, эскалирована»;
// (c2) Escalated=false → «, открыта» БЕЗ суффикса. Ловит и пропуск, и ложное появление.
func TestInspectEscalatedSuffix(t *testing.T) {
	t.Run("escalated", func(t *testing.T) {
		db := filepath.Join(t.TempDir(), "esc.db")
		seedInspectFixture(t, db, store.TaskPending, true, true)
		var out, errBuf bytes.Buffer
		if code := inspectMain([]string{"p-000001", "--db", db}, &out, &errBuf); code != 0 {
			t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
		}
		want := "  t-000001 шаг 'связаться_с_клиентом' → менеджер, срок до <DT>, открыта, эскалирована\n"
		if got := lastTaskLine(maskDeadlines(out.String())); got != want {
			t.Errorf("строка задачи = %q, хотим %q (c1)", got, want)
		}
	})
	t.Run("not-escalated", func(t *testing.T) {
		db := filepath.Join(t.TempDir(), "noesc.db")
		seedInspectFixture(t, db, store.TaskPending, true, false)
		var out, errBuf bytes.Buffer
		if code := inspectMain([]string{"p-000001", "--db", db}, &out, &errBuf); code != 0 {
			t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
		}
		want := "  t-000001 шаг 'связаться_с_клиентом' → менеджер, срок до <DT>, открыта\n"
		if got := lastTaskLine(maskDeadlines(out.String())); got != want {
			t.Errorf("строка задачи = %q, хотим %q (c2, БЕЗ суффикса)", got, want)
		}
	})
}

// TestInspectCompletedTask — завершённая задача → «, завершена» (НЕ «, открыта»).
func TestInspectCompletedTask(t *testing.T) {
	db := filepath.Join(t.TempDir(), "done.db")
	seedInspectFixture(t, db, store.TaskCompleted, true, false)
	var out, errBuf bytes.Buffer
	if code := inspectMain([]string{"p-000001", "--db", db}, &out, &errBuf); code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	want := "  t-000001 шаг 'связаться_с_клиентом' → менеджер, срок до <DT>, завершена\n"
	if got := lastTaskLine(maskDeadlines(out.String())); got != want {
		t.Errorf("строка задачи = %q, хотим %q (завершена, НЕ открыта)", got, want)
	}
}

// TestInspectNoDeadline — задача без дедлайна → хвост «, срок до <DT>» ОТСУТСТВУЕТ.
func TestInspectNoDeadline(t *testing.T) {
	db := filepath.Join(t.TempDir(), "nodl.db")
	seedInspectFixture(t, db, store.TaskPending, false, false)
	var out, errBuf bytes.Buffer
	if code := inspectMain([]string{"p-000001", "--db", db}, &out, &errBuf); code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	want := "  t-000001 шаг 'связаться_с_клиентом' → менеджер, открыта\n"
	if got := lastTaskLine(out.String()); got != want {
		t.Errorf("строка задачи = %q, хотим %q (БЕЗ срок до)", got, want)
	}
}

// TestInspectEmptyVarsAndTasks — инстанс без переменных и без задач → блоки
// «переменные:» и «задачи:» печатаются БЕЗ строк под ними (§AU-10.D). exit 0.
func TestInspectEmptyVarsAndTasks(t *testing.T) {
	db := filepath.Join(t.TempDir(), "empty.db")
	st, err := store.NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	created := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	if err := st.SaveInstance(&store.ProcessInstance{
		ID: "p-000001", ProcessName: "пусто_проц", Status: store.StatusDone,
		CurrentStep: "финал", CreatedAt: created, UpdatedAt: created,
	}); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}
	st.Close()

	var out, errBuf bytes.Buffer
	if code := inspectMain([]string{"p-000001", "--db", db}, &out, &errBuf); code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	want := "" +
		"инстанс p-000001: процесс пусто_проц, статус выполнен, шаг 'финал'\n" +
		"переменные:\n" +
		"задачи:\n"
	if got := out.String(); got != want {
		t.Errorf("stdout байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}
}

// TestInspectUnknownInstance — неизв.инстанс (инверсия d, §AU-10.C): stderr РОВНО
// «ladix: инстанс 'p-999999' не найден\n», exit 2, stdout пуст.
func TestInspectUnknownInstance(t *testing.T) {
	db := filepath.Join(t.TempDir(), "unknown.db")
	// Создаём БД, но без p-999999.
	seedInspectFixture(t, db, store.TaskPending, true, false)

	var out, errBuf bytes.Buffer
	code := inspectMain([]string{"p-999999", "--db", db}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("код = %d, хотим 2; stderr=%q", code, errBuf.String())
	}
	if out.Len() != 0 {
		t.Errorf("непустой stdout: %q (хотим пусто)", out.String())
	}
	want := "ladix: инстанс 'p-999999' не найден\n"
	if errBuf.String() != want {
		t.Errorf("stderr = %q, хотим %q (дословно, английский сентинел НЕ печатается)", errBuf.String(), want)
	}
}

// TestInspectDemoPairB5toB6 — демо-пара B5→B6 через реальный start (детерминизм id на
// свежей БД: p-000001/t-000001), затем inspect показывает снимок+историю. Замок INV-4
// (единая --db, start и inspect делят одну БД).
func TestInspectDemoPairB5toB6(t *testing.T) {
	db := filepath.Join(t.TempDir(), "demo.db")
	var sOut, sErr bytes.Buffer
	startArgs := []string{"start", examplePath("контроль_плана.ladix"), "эскалация_плана", "2500000", "--db", db}
	if code := realMain(startArgs, &sOut, &sErr); code != 0 {
		t.Fatalf("start код = %d, хотим 0; stderr=%q", code, sErr.String())
	}

	var iOut, iErr bytes.Buffer
	if code := inspectMain([]string{"p-000001", "--db", db}, &iOut, &iErr); code != 0 {
		t.Fatalf("inspect код = %d, хотим 0; stderr=%q", code, iErr.String())
	}
	want := "" +
		"инстанс p-000001: процесс эскалация_плана, статус ожидает, шаг 'связаться_с_клиентом'\n" +
		"переменные:\n" +
		"  факт = 2500000\n" +
		"задачи:\n" +
		"  t-000001 шаг 'связаться_с_клиентом' → менеджер, срок до <DT>, открыта\n"
	if got := maskDeadlines(iOut.String()); got != want {
		t.Errorf("inspect после start байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}
}

// lastTaskLine возвращает последнюю непустую строку (с переводом строки) вывода inspect —
// строку задачи (помощник для замков суффикса/статуса/дедлайна).
func lastTaskLine(s string) string {
	lines := bytes.Split([]byte(s), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		if len(lines[i]) > 0 {
			return string(lines[i]) + "\n"
		}
	}
	return ""
}
