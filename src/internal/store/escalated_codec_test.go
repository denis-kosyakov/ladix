package store

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// storeInterfaceMethodCount — число методов интерфейса Store через рефлексию (замок
// INV-4: счётчик не растёт под B4b).
func storeInterfaceMethodCount() int {
	return reflect.TypeOf((*Store)(nil)).Elem().NumMethod()
}

// TestTaskEscalatedCodec — round-trip + UPSERT поля Task.Escalated через все 4 точки
// SQLite-кодека (016 B4b, §AU-2 / contracts/task-escalated-codec.md). Замок целостности
// кодека: пропуск любой точки (DDL / INSERT-список / ON CONFLICT / SELECT-читатели)
// роняет одну из ветвей этого теста — escalated молча теряется на рестарте.
func TestTaskEscalatedCodec(t *testing.T) {
	path := filepath.Join(t.TempDir(), "escalated.db")
	created := time.Date(2026, 5, 31, 9, 0, 0, 0, time.UTC)

	// (1) Round-trip Escalated=true через ОТДЕЛЬНЫЙ переоткрытый Store на той же --db
	// (персист реальный, не в памяти процесса). Сидим родительский инстанс (FK).
	st1, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore #1: %v", err)
	}
	if err := st1.SaveInstance(&ProcessInstance{
		ID: "p-000001", ProcessName: "p", Status: StatusWaiting, CurrentStep: "s",
		CreatedAt: created, UpdatedAt: created,
	}); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}
	if err := st1.SaveTask(&Task{
		ID: "t-000001", InstanceID: "p-000001", StepName: "s", Assignee: "менеджер",
		Status: TaskPending, CreatedAt: created, Escalated: true,
	}); err != nil {
		t.Fatalf("SaveTask(Escalated:true): %v", err)
	}
	if err := st1.Close(); err != nil {
		t.Fatalf("Close #1: %v", err)
	}

	st2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore #2 (та же --db): %v", err)
	}
	defer st2.Close()

	// Читатель LoadTask (scanTask, точка 4).
	got, err := st2.LoadTask("t-000001")
	if err != nil {
		t.Fatalf("LoadTask: %v", err)
	}
	if !got.Escalated {
		t.Errorf("LoadTask: Escalated = false, хотим true (точка 4 SELECT/scanTask потеряна?)")
	}
	// Читатель ListPendingTasks (главный читатель скана §AU-6.2.2, точка 4).
	pend, err := st2.ListPendingTasks("")
	if err != nil {
		t.Fatalf("ListPendingTasks: %v", err)
	}
	if len(pend) != 1 || !pend[0].Escalated {
		t.Errorf("ListPendingTasks: Escalated не персистнут (точка 4 SELECT потеряна?): %+v", pend)
	}

	// (2) UPSERT false→true тем же ID (точка 3 ON CONFLICT). Свежая БД.
	path2 := filepath.Join(t.TempDir(), "upsert.db")
	su, err := NewSQLiteStore(path2)
	if err != nil {
		t.Fatalf("NewSQLiteStore upsert: %v", err)
	}
	defer su.Close()
	if err := su.SaveInstance(&ProcessInstance{
		ID: "p-000001", ProcessName: "p", Status: StatusWaiting, CurrentStep: "s",
		CreatedAt: created, UpdatedAt: created,
	}); err != nil {
		t.Fatalf("SaveInstance upsert: %v", err)
	}
	base := &Task{
		ID: "t-000001", InstanceID: "p-000001", StepName: "s", Assignee: "менеджер",
		Status: TaskPending, CreatedAt: created, Escalated: false,
	}
	if err := su.SaveTask(base); err != nil {
		t.Fatalf("SaveTask(Escalated:false): %v", err)
	}
	base.Escalated = true
	if err := su.SaveTask(base); err != nil { // UPSERT того же ID
		t.Fatalf("SaveTask UPSERT(Escalated:true): %v", err)
	}
	reread, err := su.LoadTask("t-000001")
	if err != nil {
		t.Fatalf("LoadTask после UPSERT: %v", err)
	}
	if !reread.Escalated {
		t.Errorf("UPSERT не обновил escalated (точка 3 ON CONFLICT потеряна?): Escalated = false")
	}
}

// TestMemoryStoreEscalatedCopy — MemoryStore.copyTask несёт Escalated тривиально
// (cp := *t); паритет с SQLite-кодеком (§AU-2).
func TestMemoryStoreEscalatedCopy(t *testing.T) {
	st := NewMemoryStore()
	now := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	if err := st.SaveTask(&Task{
		ID: "t-000001", InstanceID: "p-000001", StepName: "s", Assignee: "менеджер",
		Status: TaskPending, CreatedAt: now, Escalated: true,
	}); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	got, err := st.LoadTask("t-000001")
	if err != nil {
		t.Fatalf("LoadTask: %v", err)
	}
	if !got.Escalated {
		t.Errorf("MemoryStore: Escalated = false, хотим true (copyTask должен нести bool)")
	}
	pend, _ := st.ListPendingTasks("")
	if len(pend) != 1 || !pend[0].Escalated {
		t.Errorf("MemoryStore ListPendingTasks: Escalated не несётся: %+v", pend)
	}
}

// TestStoreMethodCount16 — интерфейс Store под B6: 15 базовых (006/007b) + аддитивный
// ListTasksByInstance (read-only, §AU-2 15→16). B4b Escalated — колонка, не метод
// (счёт не растёт); B6 добавляет РОВНО один метод. Замок INV-2 §AU-2.
func TestStoreMethodCount16(t *testing.T) {
	// Compile-time: обе реализации удовлетворяют интерфейсу (см. store.go var _).
	var _ Store = (*SQLiteStore)(nil)
	var _ Store = (*MemoryStore)(nil)
	// Ручной счёт методов интерфейса Store через рефлексию.
	const wantMethods = 16
	got := storeInterfaceMethodCount()
	if got != wantMethods {
		t.Errorf("интерфейс Store имеет %d методов, хотим РОВНО %d (15 базовых + ListTasksByInstance B6)", got, wantMethods)
	}
}
