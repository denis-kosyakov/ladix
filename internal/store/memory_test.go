package store

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/value"
)

func TestMemoryMintIDs(t *testing.T) {
	st := NewMemoryStore()
	for i, want := range []string{"p-000001", "p-000002", "p-000003"} {
		got, err := st.NextInstanceID()
		if err != nil {
			t.Fatalf("NextInstanceID #%d: %v", i, err)
		}
		if got != want {
			t.Errorf("NextInstanceID #%d = %q, want %q", i, got, want)
		}
	}
	for i, want := range []string{"t-000001", "t-000002"} {
		got, err := st.NextTaskID()
		if err != nil {
			t.Fatalf("NextTaskID #%d: %v", i, err)
		}
		if got != want {
			t.Errorf("NextTaskID #%d = %q, want %q", i, got, want)
		}
	}
}

func TestMemoryInstanceRoundTrip(t *testing.T) {
	st := NewMemoryStore()
	now := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	inst := &ProcessInstance{
		ID:          "p-000001",
		ProcessName: "онбординг",
		Status:      StatusWaiting,
		CurrentStep: "провести_встречу",
		Variables:   map[string]value.Value{"имя": value.Строка{V: "Иван"}},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}
	got, err := st.LoadInstance("p-000001")
	if err != nil {
		t.Fatalf("LoadInstance: %v", err)
	}
	if got.ID != inst.ID || got.ProcessName != inst.ProcessName || got.Status != inst.Status ||
		got.CurrentStep != inst.CurrentStep || !got.CreatedAt.Equal(now) || !got.UpdatedAt.Equal(now) {
		t.Errorf("LoadInstance mismatch: got %#v", got)
	}
	if !value.Equal(got.Variables["имя"], value.Строка{V: "Иван"}) {
		t.Errorf("Variables не восстановлены: %#v", got.Variables)
	}
}

func TestMemoryInstanceNotFound(t *testing.T) {
	st := NewMemoryStore()
	_, err := st.LoadInstance("p-999999")
	if !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("LoadInstance unknown: got %v, want ErrInstanceNotFound", err)
	}
}

// Без алиасинга: мутация загруженного инстанса/карты не видна в Store до Save.
func TestMemoryNoAliasingInstance(t *testing.T) {
	st := NewMemoryStore()
	inst := &ProcessInstance{
		ID:          "p-000001",
		ProcessName: "p",
		Status:      StatusRunning,
		CurrentStep: "s1",
		Variables:   map[string]value.Value{"x": value.Целое{V: 1}},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := st.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}

	// Мутация исходного указателя после Save не видна в Store.
	inst.Status = StatusFailed
	inst.CurrentStep = "ПОДМЕНА"
	inst.Variables["x"] = value.Целое{V: 999}

	got, err := st.LoadInstance("p-000001")
	if err != nil {
		t.Fatalf("LoadInstance: %v", err)
	}
	if got.Status != StatusRunning || got.CurrentStep != "s1" {
		t.Errorf("мутация исходного указателя протекла в Store: %#v", got)
	}
	if !value.Equal(got.Variables["x"], value.Целое{V: 1}) {
		t.Errorf("мутация карты исходного указателя протекла: %#v", got.Variables)
	}

	// Мутация загруженной копии не видна в Store до следующего Save.
	got.Status = StatusFailed
	got.Variables["x"] = value.Целое{V: 777}
	again, err := st.LoadInstance("p-000001")
	if err != nil {
		t.Fatalf("LoadInstance again: %v", err)
	}
	if again.Status != StatusRunning {
		t.Errorf("мутация загруженной копии протекла в Store: %#v", again)
	}
	if !value.Equal(again.Variables["x"], value.Целое{V: 1}) {
		t.Errorf("мутация загруженной карты протекла: %#v", again.Variables)
	}
}

func TestMemoryTaskRoundTrip(t *testing.T) {
	st := NewMemoryStore()
	dl := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	task := &Task{
		ID:         "t-000001",
		InstanceID: "p-000001",
		StepName:   "провести_встречу",
		Assignee:   "Петров",
		Deadline:   &dl,
		Status:     TaskPending,
		CreatedAt:  time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC),
	}
	if err := st.SaveTask(task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	got, err := st.LoadTask("t-000001")
	if err != nil {
		t.Fatalf("LoadTask: %v", err)
	}
	if got.ID != task.ID || got.InstanceID != task.InstanceID || got.StepName != task.StepName ||
		got.Assignee != task.Assignee || got.Status != task.Status {
		t.Errorf("LoadTask mismatch: %#v", got)
	}
	if got.Deadline == nil || !got.Deadline.Equal(dl) {
		t.Errorf("Deadline не восстановлен: %v", got.Deadline)
	}
	if got.CompletedAt != nil {
		t.Errorf("CompletedAt должен быть nil: %v", got.CompletedAt)
	}
}

func TestMemoryTaskNotFound(t *testing.T) {
	st := NewMemoryStore()
	_, err := st.LoadTask("t-999999")
	if !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("LoadTask unknown: got %v, want ErrTaskNotFound", err)
	}
}

// Без алиасинга: мутация загруженного Deadline-указателя не видна в Store.
func TestMemoryNoAliasingTask(t *testing.T) {
	st := NewMemoryStore()
	const wantDL = "2026-06-02T00:00:00Z" // эталон, не связанный с указателем task.Deadline
	dl := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	task := &Task{
		ID:         "t-000001",
		InstanceID: "p-000001",
		StepName:   "s",
		Assignee:   "Петров",
		Deadline:   &dl,
		Status:     TaskPending,
		CreatedAt:  time.Now(),
	}
	if err := st.SaveTask(task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	// Мутируем исходные поля И сам указуемый Deadline после Save.
	task.Assignee = "ПОДМЕНА"
	*task.Deadline = time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)

	got, err := st.LoadTask("t-000001")
	if err != nil {
		t.Fatalf("LoadTask: %v", err)
	}
	if got.Assignee != "Петров" {
		t.Errorf("мутация Assignee протекла: %q", got.Assignee)
	}
	if got.Deadline == nil || got.Deadline.Format(time.RFC3339) != wantDL {
		t.Errorf("мутация Deadline-указателя протекла: %v", got.Deadline)
	}
	// Мутация загруженного Deadline тоже изолирована от Store.
	*got.Deadline = time.Date(1888, 1, 1, 0, 0, 0, 0, time.UTC)
	again, _ := st.LoadTask("t-000001")
	if again.Deadline.Format(time.RFC3339) != wantDL {
		t.Errorf("мутация загруженного Deadline протекла: %v", again.Deadline)
	}
}

func TestMemoryMarkTaskCompletedAtomic(t *testing.T) {
	st := NewMemoryStore()
	task := &Task{
		ID:         "t-000001",
		InstanceID: "p-000001",
		StepName:   "s",
		Assignee:   "Петров",
		Status:     TaskPending,
		CreatedAt:  time.Now(),
	}
	if err := st.SaveTask(task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	at := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := st.MarkTaskCompleted("t-000001", at); err != nil {
		t.Fatalf("MarkTaskCompleted: %v", err)
	}
	got, _ := st.LoadTask("t-000001")
	if got.Status != TaskCompleted {
		t.Errorf("status после завершения = %q, want %q", got.Status, TaskCompleted)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(at) {
		t.Errorf("CompletedAt = %v, want %v", got.CompletedAt, at)
	}
	// Повтор → ErrTaskAlreadyCompleted.
	err := st.MarkTaskCompleted("t-000001", at.Add(time.Hour))
	if !errors.Is(err, ErrTaskAlreadyCompleted) {
		t.Errorf("повтор MarkTaskCompleted: got %v, want ErrTaskAlreadyCompleted", err)
	}
	// CompletedAt не перезаписан повтором.
	got2, _ := st.LoadTask("t-000001")
	if !got2.CompletedAt.Equal(at) {
		t.Errorf("повтор перезаписал CompletedAt: %v", got2.CompletedAt)
	}
}

func TestMemoryMarkTaskCompletedNotFound(t *testing.T) {
	st := NewMemoryStore()
	err := st.MarkTaskCompleted("t-999999", time.Now())
	if !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("MarkTaskCompleted unknown: got %v, want ErrTaskNotFound", err)
	}
}

func TestMemoryListPendingTasks(t *testing.T) {
	st := NewMemoryStore()
	mk := func(id, assignee string, status TaskStatus) *Task {
		return &Task{ID: id, InstanceID: "p-000001", StepName: "s", Assignee: assignee, Status: status, CreatedAt: time.Now()}
	}
	// Намеренно вне порядка ID при сохранении.
	for _, tk := range []*Task{
		mk("t-000003", "Петров", TaskPending),
		mk("t-000001", "Петров", TaskPending),
		mk("t-000002", "Сидоров", TaskPending),
		mk("t-000004", "Петров", TaskCompleted), // завершённая — не попадает
	} {
		if err := st.SaveTask(tk); err != nil {
			t.Fatalf("SaveTask %s: %v", tk.ID, err)
		}
	}

	// Все открытые, по возрастанию ID.
	all, err := st.ListPendingTasks("")
	if err != nil {
		t.Fatalf("ListPendingTasks(\"\"): %v", err)
	}
	gotIDs := func(ts []*Task) []string {
		out := make([]string, len(ts))
		for i, t := range ts {
			out[i] = t.ID
		}
		return out
	}
	if want := []string{"t-000001", "t-000002", "t-000003"}; !equalStrings(gotIDs(all), want) {
		t.Errorf("ListPendingTasks(\"\") = %v, want %v", gotIDs(all), want)
	}

	// Фильтр по исполнителю.
	petrov, err := st.ListPendingTasks("Петров")
	if err != nil {
		t.Fatalf("ListPendingTasks(Петров): %v", err)
	}
	if want := []string{"t-000001", "t-000003"}; !equalStrings(gotIDs(petrov), want) {
		t.Errorf("ListPendingTasks(Петров) = %v, want %v", gotIDs(petrov), want)
	}

	// Неизвестный исполнитель → пустой список, не ошибка.
	none, err := st.ListPendingTasks("Неизвестный")
	if err != nil {
		t.Fatalf("ListPendingTasks(Неизвестный): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("ListPendingTasks(Неизвестный) = %v, want []", gotIDs(none))
	}
}

// ListPendingTasks возвращает копии — мутация результата не трогает Store.
func TestMemoryListPendingNoAliasing(t *testing.T) {
	st := NewMemoryStore()
	_ = st.SaveTask(&Task{ID: "t-000001", InstanceID: "p-000001", StepName: "s", Assignee: "Петров", Status: TaskPending, CreatedAt: time.Now()})
	list, _ := st.ListPendingTasks("")
	list[0].Assignee = "ПОДМЕНА"
	got, _ := st.LoadTask("t-000001")
	if got.Assignee != "Петров" {
		t.Errorf("мутация результата ListPendingTasks протекла в Store: %q", got.Assignee)
	}
}

func TestMemoryConcurrentMint(t *testing.T) {
	st := NewMemoryStore()
	const n = 200
	seen := make(map[string]struct{}, n)
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			id, err := st.NextInstanceID()
			if err != nil {
				t.Errorf("NextInstanceID: %v", err)
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

func equalStrings(a, b []string) bool {
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
