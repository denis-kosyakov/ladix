package store

import (
	"errors"
	"math"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// openSQLite открывает SQLiteStore во временном файле и регистрирует Close.
func openSQLite(t *testing.T) *SQLiteStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ladix.db")
	st, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore(%q): %v", path, err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestSQLiteMintIDs(t *testing.T) {
	st := openSQLite(t)
	for i, want := range []string{"p-000001", "p-000002"} {
		got, err := st.NextInstanceID()
		if err != nil {
			t.Fatalf("NextInstanceID #%d: %v", i, err)
		}
		if got != want {
			t.Errorf("NextInstanceID #%d = %q, want %q", i, got, want)
		}
	}
	for i, want := range []string{"t-000001", "t-000002", "t-000003"} {
		got, err := st.NextTaskID()
		if err != nil {
			t.Fatalf("NextTaskID #%d: %v", i, err)
		}
		if got != want {
			t.Errorf("NextTaskID #%d = %q, want %q", i, got, want)
		}
	}
}

// D-10: счётчик персистентен через Close+переоткрытие — нумерация продолжается.
func TestSQLiteCounterPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ladix.db")
	st, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for _, want := range []string{"p-000001", "p-000002"} {
		got, _ := st.NextInstanceID()
		if got != want {
			t.Fatalf("mint = %q, want %q", got, want)
		}
	}
	if got, _ := st.NextTaskID(); got != "t-000001" {
		t.Fatalf("task mint = %q, want t-000001", got)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Переоткрытие — нумерация продолжается, не сбрасывается.
	st2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st2.Close()
	if got, _ := st2.NextInstanceID(); got != "p-000003" {
		t.Errorf("после reopen NextInstanceID = %q, want p-000003", got)
	}
	if got, _ := st2.NextTaskID(); got != "t-000002" {
		t.Errorf("после reopen NextTaskID = %q, want t-000002", got)
	}
}

// Идемпотентность схемы: повторное открытие того же файла безвредно.
func TestSQLiteSchemaIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ladix.db")
	st, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("open #1: %v", err)
	}
	inst := &ProcessInstance{
		ID: "p-000001", ProcessName: "p", Status: StatusWaiting, CurrentStep: "s",
		Variables: map[string]value.Value{"x": value.Целое{V: 1}},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := st.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}
	st.Close()

	st2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("open #2 (idempotent CREATE): %v", err)
	}
	defer st2.Close()
	got, err := st2.LoadInstance("p-000001")
	if err != nil {
		t.Fatalf("LoadInstance после reopen: %v", err)
	}
	if got.ProcessName != "p" {
		t.Errorf("данные потеряны при reopen: %#v", got)
	}
}

func TestSQLiteRoundTripAllTypes(t *testing.T) {
	st := openSQLite(t)
	vars := map[string]value.Value{
		"цел":   value.Целое{V: 42},
		"дроб":  value.Дробное{V: 3.14},
		"стр":   value.Строка{V: "привет"},
		"бул":   value.Булево{V: true},
		"пусто": value.None,
		"длит":  value.Длительность{Amount: 5, Unit: "мин"},
		"пер":   value.Период{Name: "ежемесячно"},
		"дата":  value.Дата{Year: 2026, Month: 5, Day: 31},
		"спис":  value.NewList([]value.Value{value.Целое{V: 1}, value.Строка{V: "x"}}),
		"зап": value.NewRecord(
			[]string{"номер", "сумма"},
			map[string]value.Value{"номер": value.Целое{V: 7}, "сумма": value.Целое{V: 1000}},
		),
	}
	inst := &ProcessInstance{
		ID: "p-000001", ProcessName: "p", Status: StatusRunning, CurrentStep: "s",
		Variables: vars,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := st.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}
	got, err := st.LoadInstance("p-000001")
	if err != nil {
		t.Fatalf("LoadInstance: %v", err)
	}
	for k, want := range vars {
		gv, ok := got.Variables[k]
		if !ok {
			t.Errorf("переменная %q потеряна", k)
			continue
		}
		if !valEqual(want, gv) {
			t.Errorf("переменная %q: got %#v, want %#v", k, gv, want)
		}
	}
}

// D-5 через SQLite round-trip.
func TestSQLiteRoundTripSpecialFloats(t *testing.T) {
	st := openSQLite(t)
	inst := &ProcessInstance{
		ID: "p-000001", ProcessName: "p", Status: StatusRunning, CurrentStep: "s",
		Variables: map[string]value.Value{
			"nan":  value.Дробное{V: math.NaN()},
			"pinf": value.Дробное{V: math.Inf(+1)},
			"ninf": value.Дробное{V: math.Inf(-1)},
		},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := st.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}
	got, err := st.LoadInstance("p-000001")
	if err != nil {
		t.Fatalf("LoadInstance: %v", err)
	}
	if !math.IsNaN(got.Variables["nan"].(value.Дробное).V) {
		t.Errorf("nan не восстановлен: %v", got.Variables["nan"])
	}
	if !math.IsInf(got.Variables["pinf"].(value.Дробное).V, +1) {
		t.Errorf("+Inf не восстановлен: %v", got.Variables["pinf"])
	}
	if !math.IsInf(got.Variables["ninf"].(value.Дробное).V, -1) {
		t.Errorf("-Inf не восстановлен: %v", got.Variables["ninf"])
	}
}

func TestSQLiteInstanceNotFound(t *testing.T) {
	st := openSQLite(t)
	_, err := st.LoadInstance("p-999999")
	if !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("LoadInstance unknown: got %v, want ErrInstanceNotFound", err)
	}
}

func TestSQLiteTaskNotFound(t *testing.T) {
	st := openSQLite(t)
	_, err := st.LoadTask("t-999999")
	if !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("LoadTask unknown: got %v, want ErrTaskNotFound", err)
	}
}

// Времена в RFC3339 (секундная точность, не Nano): субсекунда отбрасывается.
func TestSQLiteTimeRFC3339(t *testing.T) {
	st := openSQLite(t)
	created := time.Date(2026, 5, 31, 9, 30, 15, 123456789, time.UTC)
	dl := time.Date(2026, 6, 2, 9, 30, 15, 987654321, time.UTC)
	inst := &ProcessInstance{
		ID: "p-000001", ProcessName: "p", Status: StatusWaiting, CurrentStep: "s",
		Variables: map[string]value.Value{}, CreatedAt: created, UpdatedAt: created,
	}
	if err := st.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}
	task := &Task{
		ID: "t-000001", InstanceID: "p-000001", StepName: "s", Assignee: "Петров",
		Deadline: &dl, Status: TaskPending, CreatedAt: created,
	}
	if err := st.SaveTask(task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}

	gotInst, _ := st.LoadInstance("p-000001")
	wantSec := created.Truncate(time.Second)
	if !gotInst.CreatedAt.Equal(wantSec) {
		t.Errorf("CreatedAt = %v, want секундную точность %v", gotInst.CreatedAt, wantSec)
	}
	gotTask, _ := st.LoadTask("t-000001")
	if !gotTask.Deadline.Equal(dl.Truncate(time.Second)) {
		t.Errorf("Deadline = %v, want секундную точность %v", gotTask.Deadline, dl.Truncate(time.Second))
	}
}

// Task с nil-Deadline сохраняется/читается как NULL.
func TestSQLiteTaskNilDeadline(t *testing.T) {
	st := openSQLite(t)
	_ = st.SaveInstance(&ProcessInstance{
		ID: "p-000001", ProcessName: "p", Status: StatusWaiting, CurrentStep: "s",
		Variables: map[string]value.Value{}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	task := &Task{
		ID: "t-000001", InstanceID: "p-000001", StepName: "s", Assignee: "Петров",
		Deadline: nil, Status: TaskPending, CreatedAt: time.Now(),
	}
	if err := st.SaveTask(task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	got, err := st.LoadTask("t-000001")
	if err != nil {
		t.Fatalf("LoadTask: %v", err)
	}
	if got.Deadline != nil {
		t.Errorf("nil-Deadline восстановлен как %v", got.Deadline)
	}
	if got.CompletedAt != nil {
		t.Errorf("nil-CompletedAt восстановлен как %v", got.CompletedAt)
	}
}

func TestSQLiteMarkTaskCompletedAtomic(t *testing.T) {
	st := openSQLite(t)
	_ = st.SaveInstance(&ProcessInstance{
		ID: "p-000001", ProcessName: "p", Status: StatusWaiting, CurrentStep: "s",
		Variables: map[string]value.Value{}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	_ = st.SaveTask(&Task{
		ID: "t-000001", InstanceID: "p-000001", StepName: "s", Assignee: "Петров",
		Status: TaskPending, CreatedAt: time.Now(),
	})
	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := st.MarkTaskCompleted("t-000001", at); err != nil {
		t.Fatalf("MarkTaskCompleted: %v", err)
	}
	got, _ := st.LoadTask("t-000001")
	if got.Status != TaskCompleted {
		t.Errorf("status = %q, want %q", got.Status, TaskCompleted)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(at) {
		t.Errorf("CompletedAt = %v, want %v", got.CompletedAt, at)
	}
	// Повтор → ErrTaskAlreadyCompleted.
	if err := st.MarkTaskCompleted("t-000001", at.Add(time.Hour)); !errors.Is(err, ErrTaskAlreadyCompleted) {
		t.Errorf("повтор: got %v, want ErrTaskAlreadyCompleted", err)
	}
}

func TestSQLiteMarkTaskCompletedNotFound(t *testing.T) {
	st := openSQLite(t)
	if err := st.MarkTaskCompleted("t-999999", time.Now()); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("MarkTaskCompleted unknown: got %v, want ErrTaskNotFound", err)
	}
}

func TestSQLiteListPendingTasksOrder(t *testing.T) {
	st := openSQLite(t)
	_ = st.SaveInstance(&ProcessInstance{
		ID: "p-000001", ProcessName: "p", Status: StatusWaiting, CurrentStep: "s",
		Variables: map[string]value.Value{}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	mk := func(id, assignee string, status TaskStatus) *Task {
		return &Task{ID: id, InstanceID: "p-000001", StepName: "s", Assignee: assignee, Status: status, CreatedAt: time.Now()}
	}
	for _, tk := range []*Task{
		mk("t-000003", "Петров", TaskPending),
		mk("t-000001", "Петров", TaskPending),
		mk("t-000002", "Сидоров", TaskPending),
		mk("t-000004", "Петров", TaskCompleted),
	} {
		if err := st.SaveTask(tk); err != nil {
			t.Fatalf("SaveTask %s: %v", tk.ID, err)
		}
	}
	ids := func(ts []*Task) []string {
		out := make([]string, len(ts))
		for i, t := range ts {
			out[i] = t.ID
		}
		return out
	}
	all, err := st.ListPendingTasks("")
	if err != nil {
		t.Fatalf("ListPendingTasks(\"\"): %v", err)
	}
	if want := []string{"t-000001", "t-000002", "t-000003"}; !equalStrings(ids(all), want) {
		t.Errorf("ListPendingTasks(\"\") = %v, want %v", ids(all), want)
	}
	petrov, err := st.ListPendingTasks("Петров")
	if err != nil {
		t.Fatalf("ListPendingTasks(Петров): %v", err)
	}
	if want := []string{"t-000001", "t-000003"}; !equalStrings(ids(petrov), want) {
		t.Errorf("ListPendingTasks(Петров) = %v, want %v", ids(petrov), want)
	}
}

// SaveInstance — upsert: повторный Save обновляет.
func TestSQLiteSaveInstanceUpsert(t *testing.T) {
	st := openSQLite(t)
	inst := &ProcessInstance{
		ID: "p-000001", ProcessName: "p", Status: StatusRunning, CurrentStep: "s1",
		Variables: map[string]value.Value{}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := st.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance #1: %v", err)
	}
	inst.Status = StatusWaiting
	inst.CurrentStep = "s2"
	if err := st.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance #2 (upsert): %v", err)
	}
	got, _ := st.LoadInstance("p-000001")
	if got.Status != StatusWaiting || got.CurrentStep != "s2" {
		t.Errorf("upsert не обновил: %#v", got)
	}
}

// Конкурентный mint в рамках одного процесса не даёт дубликатов.
func TestSQLiteConcurrentMint(t *testing.T) {
	st := openSQLite(t)
	const n = 50
	seen := make(map[string]struct{}, n)
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			id, err := st.NextTaskID()
			if err != nil {
				t.Errorf("NextTaskID: %v", err)
				return
			}
			mu.Lock()
			seen[id] = struct{}{}
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(seen) != n {
		t.Errorf("конкурентный mint дал дубликаты: уникальных %d из %d", len(seen), n)
	}
}
