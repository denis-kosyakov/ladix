package store

import (
	"path/filepath"
	"testing"
	"time"
)

// Счётный замок Store=16 (T-STORE-COUNT-16, §AU-2 15→16) живёт в
// escalated_codec_test.go::TestStoreMethodCount16 (там уже helper storeInterfaceMethodCount).

// TestListTasksByInstanceContract — поведенческий контракт ListTasksByInstance
// (C1–C6, store-list-tasks-by-instance.md) на ОБЕИХ реализациях. Зеркало
// TestStoreContract: таблица impl × проверка фиксирует паритет Memory/SQLite.
func TestListTasksByInstanceContract(t *testing.T) {
	impls := []struct {
		name     string
		newStore func(*testing.T) Store
	}{
		{"memory", func(*testing.T) Store { return NewMemoryStore() }},
		{"sqlite", func(t *testing.T) Store { return openSQLite(t) }},
	}
	for _, impl := range impls {
		t.Run(impl.name, func(t *testing.T) {
			st := impl.newStore(t)
			seedInstance(t, st, "p-000001")
			seedInstance(t, st, "p-000002")
			now := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
			mk := func(id, inst string, status TaskStatus, escalated bool) *Task {
				return &Task{
					ID: id, InstanceID: inst, StepName: "s", Assignee: "менеджер",
					Status: status, CreatedAt: now, Escalated: escalated,
				}
			}
			// Намеренно вне порядка ID при сохранении (C3 ORDER); смешанные статусы (C1 MIXED);
			// одна задача чужого инстанса p-000002 (C2 FILTER); одна Escalated (C5).
			for _, tk := range []*Task{
				mk("t-000003", "p-000001", TaskPending, false),
				mk("t-000001", "p-000001", TaskPending, true), // эскалированная
				mk("t-000002", "p-000001", TaskCompleted, false),
				mk("t-000010", "p-000002", TaskPending, false), // чужой инстанс
			} {
				if err := st.SaveTask(tk); err != nil {
					t.Fatalf("SaveTask %s: %v", tk.ID, err)
				}
			}

			got, err := st.ListTasksByInstance("p-000001")
			if err != nil {
				t.Fatalf("ListTasksByInstance(p-000001): %v", err)
			}
			// C3 ORDER + C1 MIXED + C2 FILTER: ровно [t-000001,t-000002,t-000003] (ID ASC,
			// открытые И завершённые, чужой t-000010 отфильтрован).
			ids := make([]string, len(got))
			for i, tk := range got {
				ids[i] = tk.ID
			}
			if want := []string{"t-000001", "t-000002", "t-000003"}; !eqStrs(ids, want) {
				t.Errorf("ListTasksByInstance(p-000001) = %v, хотим %v (ID ASC, MIXED, FILTER)", ids, want)
			}
			// C5 ESCALATED: поле Escalated сохранено в возвращённом *Task (t-000001 эскалирована).
			if len(got) > 0 && !got[0].Escalated {
				t.Errorf("t-000001.Escalated = false, хотим true (поле сохранено, C5)")
			}
			if len(got) > 1 && got[1].Escalated {
				t.Errorf("t-000002.Escalated = true, хотим false (не эскалирована, C5)")
			}

			// C4 EMPTY: инстанс без задач → len==0, err==nil (без паники).
			empty, err := st.ListTasksByInstance("p-999999")
			if err != nil {
				t.Fatalf("ListTasksByInstance(p-999999): %v, хотим nil (C4)", err)
			}
			if len(empty) != 0 {
				t.Errorf("ListTasksByInstance(p-999999) = %v, хотим [] (C4 EMPTY)", empty)
			}
		})
	}
}

// TestListTasksByInstanceSQLiteEscalatedPersist — замок FR-013: escalated ОБЯЗАН быть
// в SELECT-списке ListTasksByInstance и пройти через buildTask. Персист реальный:
// сохраняем Escalated=true, переоткрываем БД, читаем через ListTasksByInstance.
// Инверсия: убрать escalated из SELECT → buildTask получит 0 → Escalated=false → красный.
func TestListTasksByInstanceSQLiteEscalatedPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ltbi_escalated.db")
	created := time.Date(2026, 5, 31, 9, 0, 0, 0, time.UTC)

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
	got, err := st2.ListTasksByInstance("p-000001")
	if err != nil {
		t.Fatalf("ListTasksByInstance: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListTasksByInstance вернул %d задач, хотим 1", len(got))
	}
	if !got[0].Escalated {
		t.Errorf("ListTasksByInstance: Escalated = false, хотим true (escalated выпал из SELECT? FR-013)")
	}
}

// TestListTasksByInstanceNoAliasing — результат — копии (C6 read-only): мутация
// возвращённого *Task не протекает в Store.
func TestListTasksByInstanceNoAliasing(t *testing.T) {
	st := NewMemoryStore()
	seedInstance(t, st, "p-000001")
	_ = st.SaveTask(&Task{ID: "t-000001", InstanceID: "p-000001", StepName: "s", Assignee: "менеджер", Status: TaskPending, CreatedAt: time.Now()})
	list, _ := st.ListTasksByInstance("p-000001")
	list[0].Assignee = "ПОДМЕНА"
	got, _ := st.LoadTask("t-000001")
	if got.Assignee != "менеджер" {
		t.Errorf("мутация результата ListTasksByInstance протекла в Store: %q", got.Assignee)
	}
}
