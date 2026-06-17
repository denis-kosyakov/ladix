package engine_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/store"
)

// twoHumanSrc — процесс из двух человеческих шагов: после Start инстанс ожидает на
// первом шаге; Complete первой задачи → создаётся вторая задача, инстанс снова
// ожидает (переход ожидает→выполняется→ожидает); Complete второй → терминал.
const twoHumanSrc = `процесс p(x):
    шаг первый:
        исполнитель: "Иванов"
        срок:        2дн
    шаг второй после первый:
        исполнитель: "Петров"

пусть id = запустить процесс p(1)
`

// TestCompleteChain — цепочка Complete: пробуждение → следующая задача → терминал.
// Проверяет переход ожидает→выполнен через выполняется при наличии следующего шага.
func TestCompleteChain(t *testing.T) {
	_, st, eng, out := buildStack(t, twoHumanSrc, goldenMoment())
	if _, err := eng.Start("p", argsInt(1)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// После Start: инстанс ожидает на шаге 'первый', задача t-000001 открыта.
	inst, err := st.LoadInstance("p-000001")
	if err != nil {
		t.Fatalf("LoadInstance: %v", err)
	}
	if inst.Status != store.StatusWaiting || inst.CurrentStep != "первый" {
		t.Fatalf("после Start: статус=%q шаг=%q, хотим ожидает/первый", inst.Status, inst.CurrentStep)
	}
	out.Reset()

	// Complete t-000001: задача завершена → продвижение к шагу 'второй' (есть
	// следующий шаг) → новая задача t-000002 → инстанс снова ожидает.
	res, err := eng.Complete("t-000001", emptyRec())
	if err != nil {
		t.Fatalf("Complete t-000001: %v", err)
	}
	if res.CaughtUp {
		t.Errorf("CaughtUp=true на штатном пути (не догон)")
	}
	if res.Instance == nil || res.Instance.Status != store.StatusWaiting || res.Instance.CurrentStep != "второй" {
		t.Errorf("после Complete t-000001: инстанс=%+v, хотим ожидает/второй", res.Instance)
	}
	want1 := "" +
		"задача t-000001 завершена\n" +
		"[задача] t-000002 → Петров, шаг 'второй'\n" +
		"инстанс p-000001: ожидает, шаг 'второй'\n"
	if got := out.String(); got != want1 {
		t.Errorf("stdout после Complete t-000001 байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want1)
	}
	// Первая задача завершена в Store.
	t1, _ := st.LoadTask("t-000001")
	if t1.Status != store.TaskCompleted {
		t.Errorf("t-000001 статус=%q, хотим завершена", t1.Status)
	}
	out.Reset()

	// Complete t-000002: задача завершена → нет следующего шага → терминал (выполнен).
	res2, err := eng.Complete("t-000002", emptyRec())
	if err != nil {
		t.Fatalf("Complete t-000002: %v", err)
	}
	if res2.Instance.Status != store.StatusDone {
		t.Errorf("после Complete t-000002: статус=%q, хотим выполнен", res2.Instance.Status)
	}
	want2 := "" +
		"задача t-000002 завершена\n" +
		"инстанс p-000001: выполнен\n"
	if got := out.String(); got != want2 {
		t.Errorf("stdout после Complete t-000002 байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want2)
	}
}

// oneHumanSrc — процесс из единственного человеческого шага: переход
// ожидает→выполнен НАПРЯМУЮ (нет следующего шага → терминал без выполняется).
const oneHumanSrc = `процесс solo(x):
    шаг единственный:
        исполнитель: "Сидоров"

пусть id = запустить процесс solo(1)
`

// TestCompleteTerminalDirect — Complete единственной задачи → терминал напрямую
// (ожидает→выполнен), без промежуточного выполняется/новой задачи.
func TestCompleteTerminalDirect(t *testing.T) {
	_, st, eng, out := buildStack(t, oneHumanSrc, goldenMoment())
	if _, err := eng.Start("solo", argsInt(1)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	out.Reset()

	res, err := eng.Complete("t-000001", emptyRec())
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if res.Instance.Status != store.StatusDone {
		t.Errorf("статус=%q, хотим выполнен", res.Instance.Status)
	}
	want := "" +
		"задача t-000001 завершена\n" +
		"инстанс p-000001: выполнен\n"
	if got := out.String(); got != want {
		t.Errorf("stdout байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}
	// Никакой второй задачи не создано; инстанс выполнен в Store.
	if _, lerr := st.LoadTask("t-000002"); lerr == nil {
		t.Errorf("создана лишняя задача t-000002")
	}
	inst, _ := st.LoadInstance("p-000001")
	if inst.Status != store.StatusDone {
		t.Errorf("Store: инстанс статус=%q, хотим выполнен", inst.Status)
	}
}

// TestCompleteNotFound — отсутствующая задача → ошибка (для CLI → §EN-8.B).
func TestCompleteNotFound(t *testing.T) {
	_, _, eng, _ := buildStack(t, oneHumanSrc, goldenMoment())
	if _, err := eng.Start("solo", argsInt(1)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, err := eng.Complete("t-999999", emptyRec())
	if err == nil {
		t.Fatalf("ожидали ошибку на несуществующую задачу")
	}
	if !strings.Contains(err.Error(), "t-999999") && !strings.Contains(err.Error(), "not found") {
		// Точная Go-форма на усмотрение импл (§EN-3); CLI формирует текст §EN-8.B.
		t.Logf("ошибка Complete несуществующей задачи: %v", err)
	}
}

// fabricate кладёт в свежий MemoryStore руками заданные инстанс и задачу (без
// Start), чтобы воспроизвести состояния гардов D-8/D-4, недостижимые штатной
// CLI-цепочкой (один активный шаг = одна открытая задача). twoHumanSrc компилируется,
// чтобы движок знал определение процесса 'p' (шаги первый/второй), но advance не
// гоняется до явного Complete. Счётчики id двигаются через NextInstanceID/NextTaskID
// до фабрикуемых p-000001/t-000001, чтобы догон D-4 минтил следующую задачу t-000002
// (а не перетирал фабрикованную). Возвращает Store, Engine и общий out-буфер движка.
func fabricate(t *testing.T, inst *store.ProcessInstance, task *store.Task) (store.Store, *engine.Engine, *bytes.Buffer) {
	t.Helper()
	_, st, eng, out := buildStack(t, twoHumanSrc, goldenMoment())
	if _, err := st.NextInstanceID(); err != nil { // → p-000001, счётчик инстансов = 1
		t.Fatalf("NextInstanceID: %v", err)
	}
	if _, err := st.NextTaskID(); err != nil { // → t-000001, счётчик задач = 1
		t.Fatalf("NextTaskID: %v", err)
	}
	if err := st.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}
	if err := st.SaveTask(task); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	return st, eng, out
}

// TestCompleteGuardInstanceNotWaiting — гард D-8: открытая задача при инстансе со
// статусом 'выполняется' → ошибка «инстанс '<p-id>' не ожидает (статус 'выполняется')»
// (§EN-8.B минус префикс ladix:), инстанс НЕ тронут. Уровень engine: через CLI-цепочку
// кейс недостижим (§EN-9). Ошибка различима errors.As как GuardError.
func TestCompleteGuardInstanceNotWaiting(t *testing.T) {
	inst := &store.ProcessInstance{
		ID: "p-000001", ProcessName: "p", Status: store.StatusRunning, CurrentStep: "первый",
	}
	task := &store.Task{
		ID: "t-000001", InstanceID: "p-000001", StepName: "первый",
		Assignee: "Иванов", Status: store.TaskPending,
	}
	st, eng, _ := fabricate(t, inst, task)
	_, err := eng.Complete("t-000001", emptyRec())
	if err == nil {
		t.Fatalf("ожидали ошибку гарда D-8 (инстанс не ожидает)")
	}
	want := "инстанс 'p-000001' не ожидает (статус 'выполняется')"
	if err.Error() != want {
		t.Errorf("текст ошибки гарда = %q, хотим %q (§EN-8.B минус префикс)", err.Error(), want)
	}
	var ge *engine.GuardError
	if !errors.As(err, &ge) {
		t.Errorf("ошибка не различима errors.As как *engine.GuardError: %v", err)
	}
	// Инстанс НЕ тронут: статус как был (выполняется), задача — открыта.
	got, _ := st.LoadInstance("p-000001")
	if got.Status != store.StatusRunning {
		t.Errorf("инстанс тронут: статус=%q, хотим выполняется", got.Status)
	}
	gt, _ := st.LoadTask("t-000001")
	if gt.Status != store.TaskPending {
		t.Errorf("задача тронута: статус=%q, хотим открыта", gt.Status)
	}
}

// TestCompleteGuardStepMismatch — гард D-8: открытая задача со StepName ≠ CurrentStep
// ожидающего инстанса → ошибка «задача '<t-id>' не соответствует текущему шагу
// инстанса '<p-id>'» (§EN-8.B минус префикс), инстанс НЕ тронут.
func TestCompleteGuardStepMismatch(t *testing.T) {
	inst := &store.ProcessInstance{
		ID: "p-000001", ProcessName: "p", Status: store.StatusWaiting, CurrentStep: "второй",
	}
	task := &store.Task{
		ID: "t-000001", InstanceID: "p-000001", StepName: "первый",
		Assignee: "Иванов", Status: store.TaskPending,
	}
	st, eng, _ := fabricate(t, inst, task)
	_, err := eng.Complete("t-000001", emptyRec())
	if err == nil {
		t.Fatalf("ожидали ошибку гарда D-8 (несоответствие шагу)")
	}
	want := "задача 't-000001' не соответствует текущему шагу инстанса 'p-000001'"
	if err.Error() != want {
		t.Errorf("текст ошибки гарда = %q, хотим %q (§EN-8.B минус префикс)", err.Error(), want)
	}
	var ge *engine.GuardError
	if !errors.As(err, &ge) {
		t.Errorf("ошибка не различима errors.As как *engine.GuardError: %v", err)
	}
	got, _ := st.LoadInstance("p-000001")
	if got.Status != store.StatusWaiting || got.CurrentStep != "второй" {
		t.Errorf("инстанс тронут: %+v, хотим ожидает/второй", got)
	}
}

// TestCompleteCatchUp — гард-догон D-4: задача уже 'завершена', инстанс 'ожидает' на
// том же шаге (хвост сбойного окна «complete завершил задачу, advance не успел») →
// печать строки 8 §EN-7 ВМЕСТО строки 7 + идемпотентное до-продвижение БЕЗ
// MarkTaskCompleted, CaughtUp=true, exit 0.
func TestCompleteCatchUp(t *testing.T) {
	completedAt := goldenMoment()
	inst := &store.ProcessInstance{
		ID: "p-000001", ProcessName: "p", Status: store.StatusWaiting, CurrentStep: "первый",
	}
	task := &store.Task{
		ID: "t-000001", InstanceID: "p-000001", StepName: "первый",
		Assignee: "Иванов", Status: store.TaskCompleted, CompletedAt: &completedAt,
	}
	st, eng, out := fabricate(t, inst, task)
	res, err := eng.Complete("t-000001", emptyRec())
	if err != nil {
		t.Fatalf("Complete (догон): %v", err)
	}
	if !res.CaughtUp {
		t.Errorf("CaughtUp=false, хотим true (гард-догон D-4)")
	}
	// До-продвижение: шаг 'первый' → 'второй' (есть следующий шаг) → новая задача
	// t-000002 → инстанс снова ожидает на 'второй'.
	if res.Instance == nil || res.Instance.Status != store.StatusWaiting || res.Instance.CurrentStep != "второй" {
		t.Errorf("после догона: инстанс=%+v, хотим ожидает/второй", res.Instance)
	}
	// Строка 8 §EN-7 ВМЕСТО строки 7 (задача УЖЕ была завершена — догон).
	want := "" +
		"задача t-000001 уже была завершена, инстанс до-продвинут\n" +
		"[задача] t-000002 → Петров, шаг 'второй'\n" +
		"инстанс p-000001: ожидает, шаг 'второй'\n"
	if got := out.String(); got != want {
		t.Errorf("stdout догона байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}
	// MarkTaskCompleted НЕ вызывался повторно: CompletedAt не сдвинулся.
	gt, _ := st.LoadTask("t-000001")
	if gt.CompletedAt == nil || !gt.CompletedAt.Equal(completedAt) {
		t.Errorf("CompletedAt сдвинут (повторный MarkTaskCompleted?): %v", gt.CompletedAt)
	}
}

// TestCompleteAlreadyCompletedNoCatchUp — задача 'завершена', но догон D-4 НЕприменим
// (инстанс уже 'выполнен', не ожидает) → ошибка «задача '<id>' уже завершена»
// (§EN-8.B минус префикс), exit 2. Различима errors.As как *engine.GuardError.
func TestCompleteAlreadyCompletedNoCatchUp(t *testing.T) {
	completedAt := goldenMoment()
	inst := &store.ProcessInstance{
		ID: "p-000001", ProcessName: "p", Status: store.StatusDone, CurrentStep: "второй",
	}
	task := &store.Task{
		ID: "t-000001", InstanceID: "p-000001", StepName: "первый",
		Assignee: "Иванов", Status: store.TaskCompleted, CompletedAt: &completedAt,
	}
	st, eng, _ := fabricate(t, inst, task)
	_, err := eng.Complete("t-000001", emptyRec())
	if err == nil {
		t.Fatalf("ожидали ошибку «уже завершена» (догон неприменим)")
	}
	want := "задача 't-000001' уже завершена"
	if err.Error() != want {
		t.Errorf("текст = %q, хотим %q (§EN-8.B минус префикс)", err.Error(), want)
	}
	var ge *engine.GuardError
	if !errors.As(err, &ge) {
		t.Errorf("ошибка не различима errors.As как *engine.GuardError: %v", err)
	}
	got, _ := st.LoadInstance("p-000001")
	if got.Status != store.StatusDone {
		t.Errorf("инстанс тронут: статус=%q, хотим выполнен", got.Status)
	}
}

// failingStore — обёртка Store, дающая ошибку на NextInstanceID (имитация сбоя
// хранилища на пути, инициированном Ladix-узлом «запустить процесс»). Прочие методы
// делегируются вложенному MemoryStore.
type failingStore struct {
	store.Store
	err error
}

func (s failingStore) NextInstanceID() (string, error) { return "", s.err }

// TestEngineStorageFailureDiagnostic — §EN-8.A #8 exact-match: сбой Store на пути от
// Ladix-узла (NextInstanceID внутри Start) → обёртка «сбой хранилища: <причина>»
// (ОшибкаВыполнения, FR-018: §EN-8.A, exit 1 — не CLI-ошибка). buildStackStore
// собирает стек поверх произвольного Store.
func TestEngineStorageFailureDiagnostic(t *testing.T) {
	fs := failingStore{Store: store.NewMemoryStore(), err: errors.New("диск переполнен")}
	eng := buildStackStore(t, oneHumanSrc, goldenMoment(), fs)
	_, err := eng.Start("solo", argsInt(1))
	if err == nil {
		t.Fatalf("ожидали сбой хранилища")
	}
	want := "сбой хранилища: диск переполнен"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("текст = %q, хотим содержащий %q (§EN-8.A #8)", err.Error(), want)
	}
}
