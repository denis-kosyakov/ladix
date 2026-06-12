package store

import (
	"errors"
	"testing"
	"time"
)

// TestStoreContract — ОБЩИЙ поведенческий контракт §EN-2, прогоняемый на ОБЕИХ
// реализациях Store (MemoryStore + SQLiteStore). Спека требует поведенческой
// идентичности; ранее Memory и SQLite проверялись параллельно вручную, паритет ничем
// не фиксировался. Таблица impl × case даёт единый источник истины по контракту.
func TestStoreContract(t *testing.T) {
	impls := []struct {
		name     string
		newStore func(*testing.T) Store
	}{
		{"memory", func(*testing.T) Store { return NewMemoryStore() }},
		{"sqlite", func(t *testing.T) Store { return openSQLite(t) }},
	}
	cases := []struct {
		name string
		run  func(*testing.T, Store)
	}{
		{"NotFoundSentinels", contractNotFoundSentinels},
		{"MarkCompletedIdempotent", contractMarkCompletedIdempotent},
		{"ListPendingOrderFilter", contractListPendingOrderFilter},
		{"MonotonicMint", contractMonotonicMint},
	}
	for _, impl := range impls {
		for _, c := range cases {
			t.Run(impl.name+"/"+c.name, func(t *testing.T) {
				c.run(t, impl.newStore(t))
			})
		}
	}
}

// contractNotFoundSentinels — LoadInstance/LoadTask/MarkTaskCompleted на неизвестном id
// дают именно сентинелы (errors.Is), а не произвольную ошибку.
func contractNotFoundSentinels(t *testing.T, st Store) {
	if _, err := st.LoadInstance("p-999999"); !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("LoadInstance(нет) err = %v, хотим ErrInstanceNotFound", err)
	}
	if _, err := st.LoadTask("t-999999"); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("LoadTask(нет) err = %v, хотим ErrTaskNotFound", err)
	}
	if err := st.MarkTaskCompleted("t-999999", time.Now()); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("MarkTaskCompleted(нет) err = %v, хотим ErrTaskNotFound", err)
	}
}

// seedInstance создаёт родительский инстанс — валидное предусловие SaveTask (SQLite
// enforces FK tasks.instance_id→instances.id; движок всегда создаёт инстанс до задачи).
func seedInstance(t *testing.T, st Store, id string) {
	t.Helper()
	now := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	if err := st.SaveInstance(&ProcessInstance{
		ID: id, ProcessName: "p", Status: StatusWaiting, CurrentStep: "s",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveInstance %s: %v", id, err)
	}
}

// contractMarkCompletedIdempotent — завершение атомарно и одноразово: первый раз ok,
// повтор → ErrTaskAlreadyCompleted, CompletedAt не перезаписан (D-12).
func contractMarkCompletedIdempotent(t *testing.T, st Store) {
	seedInstance(t, st, "p-000001")
	task := &Task{
		ID: "t-000001", InstanceID: "p-000001", StepName: "s",
		Assignee: "Петров", Status: TaskPending,
		CreatedAt: time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC),
	}
	if err := st.SaveTask(task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := st.MarkTaskCompleted("t-000001", at); err != nil {
		t.Fatalf("MarkTaskCompleted: %v", err)
	}
	got, err := st.LoadTask("t-000001")
	if err != nil {
		t.Fatalf("LoadTask: %v", err)
	}
	if got.Status != TaskCompleted {
		t.Errorf("статус = %q, хотим %q", got.Status, TaskCompleted)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(at) {
		t.Errorf("CompletedAt = %v, хотим %v", got.CompletedAt, at)
	}
	if err := st.MarkTaskCompleted("t-000001", at.Add(time.Hour)); !errors.Is(err, ErrTaskAlreadyCompleted) {
		t.Errorf("повтор MarkTaskCompleted err = %v, хотим ErrTaskAlreadyCompleted", err)
	}
	got2, _ := st.LoadTask("t-000001")
	if got2.CompletedAt == nil || !got2.CompletedAt.Equal(at) {
		t.Errorf("повтор перезаписал CompletedAt: %v, хотим %v", got2.CompletedAt, at)
	}
}

// contractListPendingOrderFilter — открытые задачи по возрастанию id; завершённая
// исключена; фильтр по исполнителю (включая НЕПУСТОЕ совпадение и пустой результат).
func contractListPendingOrderFilter(t *testing.T, st Store) {
	seedInstance(t, st, "p-000001")
	now := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	mk := func(id, assignee string) *Task {
		return &Task{ID: id, InstanceID: "p-000001", StepName: "s", Assignee: assignee, Status: TaskPending, CreatedAt: now}
	}
	// Намеренно вне порядка id при сохранении.
	for _, tk := range []*Task{mk("t-000003", "Петров"), mk("t-000001", "Петров"), mk("t-000002", "Сидоров"), mk("t-000004", "Петров")} {
		if err := st.SaveTask(tk); err != nil {
			t.Fatalf("SaveTask %s: %v", tk.ID, err)
		}
	}
	// t-000004 → завершена (реальный путь MarkTaskCompleted), должна выпасть из открытых.
	if err := st.MarkTaskCompleted("t-000004", now.Add(time.Hour)); err != nil {
		t.Fatalf("MarkTaskCompleted t-000004: %v", err)
	}

	ids := func(assignee string) []string {
		ts, err := st.ListPendingTasks(assignee)
		if err != nil {
			t.Fatalf("ListPendingTasks(%q): %v", assignee, err)
		}
		out := make([]string, len(ts))
		for i, tk := range ts {
			out[i] = tk.ID
		}
		return out
	}
	if got, want := ids(""), []string{"t-000001", "t-000002", "t-000003"}; !eqStrs(got, want) {
		t.Errorf("ListPendingTasks(\"\") = %v, хотим %v (по возрастанию, завершённая исключена)", got, want)
	}
	if got, want := ids("Петров"), []string{"t-000001", "t-000003"}; !eqStrs(got, want) {
		t.Errorf("ListPendingTasks(Петров) = %v, хотим %v (непустой фильтр)", got, want)
	}
	if got, want := ids("Сидоров"), []string{"t-000002"}; !eqStrs(got, want) {
		t.Errorf("ListPendingTasks(Сидоров) = %v, хотим %v", got, want)
	}
	if got := ids("Неизвестный"); len(got) != 0 {
		t.Errorf("ListPendingTasks(Неизвестный) = %v, хотим []", got)
	}
}

// contractMonotonicMint — минт id монотонен и формат «p-/t-NNNNNN» (D-10); счётчики
// инстансов и задач независимы.
func contractMonotonicMint(t *testing.T, st Store) {
	for i, want := range []string{"p-000001", "p-000002", "p-000003"} {
		got, err := st.NextInstanceID()
		if err != nil {
			t.Fatalf("NextInstanceID #%d: %v", i, err)
		}
		if got != want {
			t.Errorf("NextInstanceID #%d = %q, хотим %q", i, got, want)
		}
	}
	for i, want := range []string{"t-000001", "t-000002"} {
		got, err := st.NextTaskID()
		if err != nil {
			t.Fatalf("NextTaskID #%d: %v", i, err)
		}
		if got != want {
			t.Errorf("NextTaskID #%d = %q, хотим %q", i, got, want)
		}
	}
}

func eqStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
