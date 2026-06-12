package engine_test

import (
	"bytes"
	stderrors "errors"
	"reflect"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// bareEngine собирает Engine поверх заданного Store с фиксированными часами, БЕЗ
// компиляции исходника: методы ProcessRuntime (InstanceStatus/Variables/UserTasks) и
// ранний LoadTask в Complete не трогают interp.Process — достаточно живого интерпретатора.
func bareEngine(t *testing.T, now time.Time, st store.Store) *engine.Engine {
	t.Helper()
	var out bytes.Buffer
	interp := eval.NewInterpreter(&out, 0, eval.SystemClock{})
	return engine.NewEngine(st, interp, &out, engine.WithClock(fixedClock{now}))
}

// --- fault-инжектирующие Store-дабли (не-сентинельный сбой на одном методе) ---

type loadInstFail struct {
	store.Store
	err error
}

func (s loadInstFail) LoadInstance(string) (*store.ProcessInstance, error) { return nil, s.err }

type listTasksFail struct {
	store.Store
	err error
}

func (s listTasksFail) ListPendingTasks(string) ([]*store.Task, error) { return nil, s.err }

type loadTaskFail struct {
	store.Store
	err error
}

func (s loadTaskFail) LoadTask(string) (*store.Task, error) { return nil, s.err }

func strVal(v value.Value) string {
	if s, ok := v.(value.Строка); ok {
		return s.V
	}
	return "<не Строка: " + v.TypeName() + ">"
}

func boolVal(v value.Value) bool {
	b, ok := v.(value.Булево)
	return ok && b.V
}

func checkField(t *testing.T, r value.Запись, field, want string) {
	t.Helper()
	if got := strVal(r.Get(field)); got != want {
		t.Errorf("поле %q = %q, хотим %q", field, got, want)
	}
}

// TestEngineInstanceStatus — T1: реальный InstanceStatus (runtime.go) на готовом Store.
// Существует → (статус,true,nil); отсутствует → ("",false,nil) (D-15); не-сентинельный
// сбой Store → *engine.StoreError.
func TestEngineInstanceStatus(t *testing.T) {
	now := goldenMoment()
	st := store.NewMemoryStore()
	if err := st.SaveInstance(&store.ProcessInstance{
		ID: "p-000001", ProcessName: "p", Status: store.StatusWaiting,
		CurrentStep: "s", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}
	eng := bareEngine(t, now, st)

	status, ok, err := eng.InstanceStatus("p-000001")
	if err != nil || !ok || status != string(store.StatusWaiting) {
		t.Errorf("InstanceStatus(существует) = (%q,%v,%v), хотим (%q,true,nil)", status, ok, err, store.StatusWaiting)
	}
	if _, ok, err := eng.InstanceStatus("p-999999"); ok || err != nil {
		t.Errorf("InstanceStatus(нет) = (_,%v,%v), хотим (_,false,nil)", ok, err)
	}
	engFail := bareEngine(t, now, loadInstFail{Store: store.NewMemoryStore(), err: stderrors.New("бд повреждена")})
	_, _, err = engFail.InstanceStatus("p-000001")
	var se *engine.StoreError
	if !stderrors.As(err, &se) {
		t.Errorf("InstanceStatus(сбой) err = %v (%T), хотим *engine.StoreError", err, err)
	}
}

// TestEngineInstanceVariables — T1: реальный InstanceVariables. Ключи Записи по
// ВОЗРАСТАНИЮ (D-21) независимо от порядка вставки; отсутствует → ok=false; сбой → StoreError.
func TestEngineInstanceVariables(t *testing.T) {
	now := goldenMoment()
	st := store.NewMemoryStore()
	if err := st.SaveInstance(&store.ProcessInstance{
		ID: "p-000001", ProcessName: "p", Status: store.StatusRunning, CurrentStep: "s",
		Variables: map[string]value.Value{
			"гамма": value.Целое{V: 3},
			"альфа": value.Целое{V: 1},
			"бета":  value.Целое{V: 2},
		},
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}
	eng := bareEngine(t, now, st)

	rec, ok, err := eng.InstanceVariables("p-000001")
	if err != nil || !ok {
		t.Fatalf("InstanceVariables = (_,%v,%v), хотим (_,true,nil)", ok, err)
	}
	if want := []string{"альфа", "бета", "гамма"}; !reflect.DeepEqual(rec.Keys(), want) {
		t.Errorf("ключи = %v, хотим %v (по возрастанию, D-21)", rec.Keys(), want)
	}
	if rec.Get("бета") != (value.Целое{V: 2}) {
		t.Errorf("бета = %v, хотим Целое 2", rec.Get("бета"))
	}
	if _, ok, err := eng.InstanceVariables("p-999999"); ok || err != nil {
		t.Errorf("InstanceVariables(нет) = (_,%v,%v), хотим (_,false,nil)", ok, err)
	}
	engFail := bareEngine(t, now, loadInstFail{Store: store.NewMemoryStore(), err: stderrors.New("бд повреждена")})
	_, _, err = engFail.InstanceVariables("p-000001")
	var se *engine.StoreError
	if !stderrors.As(err, &se) {
		t.Errorf("InstanceVariables(сбой) err = %v (%T), хотим *engine.StoreError", err, err)
	}
}

// TestEngineUserTasks — T1+T2: реальный UserTasks. Только открытые, по возрастанию id
// (D-15); поля Записи задачи (ARCH §7.7); просрочена=true у задачи с дедлайном в прошлом
// (§EN-7 стр.6); непустой фильтр исполнителя; не-сентинельный сбой → *engine.StoreError.
func TestEngineUserTasks(t *testing.T) {
	now := goldenMoment()
	st := store.NewMemoryStore()
	past := now.Add(-time.Hour)
	for _, tk := range []*store.Task{
		{ID: "t-000002", InstanceID: "p-000001", StepName: "проверка", Assignee: "Сидоров", Status: store.TaskPending, Deadline: &past, CreatedAt: now},
		{ID: "t-000001", InstanceID: "p-000001", StepName: "встреча", Assignee: "Петров", Status: store.TaskPending, CreatedAt: now},
		{ID: "t-000003", InstanceID: "p-000001", StepName: "закрытие", Assignee: "HR", Status: store.TaskCompleted, CreatedAt: now},
	} {
		if err := st.SaveTask(tk); err != nil {
			t.Fatalf("SaveTask %s: %v", tk.ID, err)
		}
	}
	eng := bareEngine(t, now, st)

	recs, err := eng.UserTasks("")
	if err != nil {
		t.Fatalf("UserTasks: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("len = %d, хотим 2 (открытые по возрастанию, t-000003 завершена → исключена)", len(recs))
	}
	if got := strVal(recs[0].Get("ид")); got != "t-000001" {
		t.Errorf("recs[0].ид = %q, хотим t-000001 (порядок)", got)
	}
	if got := strVal(recs[1].Get("ид")); got != "t-000002" {
		t.Errorf("recs[1].ид = %q, хотим t-000002 (порядок)", got)
	}
	r0 := recs[0]
	checkField(t, r0, "процесс", "p-000001")
	checkField(t, r0, "шаг", "встреча")
	checkField(t, r0, "исполнитель", "Петров")
	checkField(t, r0, "статус", string(store.TaskPending))
	if boolVal(r0.Get("просрочена")) {
		t.Errorf("t-000001 просрочена=true, хотим false (нет дедлайна)")
	}
	if !boolVal(recs[1].Get("просрочена")) {
		t.Errorf("t-000002 просрочена=false, хотим true (дедлайн в прошлом, §EN-7 стр.6)")
	}

	petrov, err := eng.UserTasks("Петров")
	if err != nil {
		t.Fatalf("UserTasks(Петров): %v", err)
	}
	if len(petrov) != 1 || strVal(petrov[0].Get("ид")) != "t-000001" {
		t.Errorf("UserTasks(Петров) = %d записей, хотим [t-000001] (непустой фильтр)", len(petrov))
	}

	engFail := bareEngine(t, now, listTasksFail{Store: store.NewMemoryStore(), err: stderrors.New("бд повреждена")})
	if _, err := engFail.UserTasks(""); !stderrorsAsStore(err) {
		t.Errorf("UserTasks(сбой) err = %v (%T), хотим *engine.StoreError", err, err)
	}
}

// stderrorsAsStore — компактная проверка «err оборачивает *engine.StoreError».
func stderrorsAsStore(err error) bool {
	var se *engine.StoreError
	return stderrors.As(err, &se)
}

// TestFormatTaskLineOverdue — T2: ветка Overdue==true даёт хвост «  ПРОСРОЧЕНА»
// (§EN-7 стр.6); граница now==deadline → НЕ просрочена (строгий After).
func TestFormatTaskLineOverdue(t *testing.T) {
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.Local)
	deadline := time.Date(2026, 6, 1, 9, 0, 0, 0, time.Local)
	task := &store.Task{
		ID: "t-000007", InstanceID: "p-000007", StepName: "проверка",
		Assignee: "Иванов", Deadline: &deadline, Status: store.TaskPending,
	}
	got := engine.FormatTaskLine(task, now)
	want := "t-000007  p-000007  'проверка'  Иванов  срок до 2026-06-01 09:00  ПРОСРОЧЕНА"
	if got != want {
		t.Errorf("FormatTaskLine(просрочена) =\n%q\nхотим\n%q (§EN-7 стр.6)", got, want)
	}
	if engine.Overdue(task, deadline) {
		t.Errorf("Overdue при now==deadline = true, хотим false (строгий After)")
	}
}

// TestCompleteLoadTaskStoreFailure — F3: не-сентинельная ошибка LoadTask в Complete
// классифицируется как *engine.StoreError (CLI → §EN-8.B B9, exit 2), а НЕ возвращается
// сырой (иначе CLI-классификатор дал бы exit 1). Сентинел ErrTaskNotFound не примешан.
func TestCompleteLoadTaskStoreFailure(t *testing.T) {
	now := goldenMoment()
	st := loadTaskFail{Store: store.NewMemoryStore(), err: stderrors.New("бд повреждена")}
	eng := bareEngine(t, now, st)

	_, err := eng.Complete("t-000001")
	if err == nil {
		t.Fatalf("ожидали *engine.StoreError, получили nil")
	}
	var se *engine.StoreError
	if !stderrors.As(err, &se) {
		t.Fatalf("Complete(LoadTask-сбой) err = %v (%T), хотим *engine.StoreError (B9)", err, err)
	}
	if se.Error() != "сбой хранилища: бд повреждена" {
		t.Errorf("текст = %q, хотим 'сбой хранилища: бд повреждена'", se.Error())
	}
	if stderrors.Is(err, store.ErrTaskNotFound) {
		t.Errorf("не-сентинельный сбой ошибочно опознан как ErrTaskNotFound (B1)")
	}
}
