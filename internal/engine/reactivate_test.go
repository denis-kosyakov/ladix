package engine_test

import (
	stderrors "errors"
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// reactSrc — процесс с автоматическим первым шагом (печать, без исполнителя) и
// человеческим вторым (исполнитель → задача). Top-level НЕ запускает процесс: инстанс
// фабрикуется тестом напрямую (имитация залипшего инстанса в БД до подъёма демона).
const reactSrc = `процесс заявка(клиент):
    шаг подготовить:
        печать("готовим заявку для " + клиент)
    шаг согласовать после подготовить:
        исполнитель: "менеджер"
`

// stuckInstance фабрикует инстанс в данном статусе на данном шаге (имитация записи в
// БД, оставшейся от прерванного прогона).
func stuckInstance(id, step string, status store.Status) *store.ProcessInstance {
	return &store.ProcessInstance{
		ID:          id,
		ProcessName: "заявка",
		Status:      status,
		CurrentStep: step,
		Variables:   map[string]value.Value{"клиент": value.Строка{V: "ООО"}},
		CreatedAt:   goldenMoment(),
	}
}

// TestReactivateInstanceValidStep — инстанс «выполняется» с валидным CurrentStep →
// прогон advance от шага догоняет инстанс до ожидания (человеческий второй шаг создаёт
// задачу). FR-019, SC-008.
func TestReactivateInstanceValidStep(t *testing.T) {
	_, st, eng, out := buildStack(t, reactSrc, goldenMoment())
	inst := stuckInstance("p-000001", "подготовить", store.StatusRunning)
	if err := st.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}

	if err := eng.ReactivateInstance(inst); err != nil {
		t.Fatalf("ReactivateInstance: неожиданная ошибка %v", err)
	}

	// Автоматический шаг исполнен (печать), человеческий создал задачу → ожидает.
	got, err := st.LoadInstance("p-000001")
	if err != nil {
		t.Fatalf("LoadInstance: %v", err)
	}
	if got.Status != store.StatusWaiting {
		t.Fatalf("статус после реактивации = %q, ожидался %q", got.Status, store.StatusWaiting)
	}
	if got.CurrentStep != "согласовать" {
		t.Fatalf("шаг после реактивации = %q, ожидался 'согласовать'", got.CurrentStep)
	}
	if !strings.Contains(out.String(), "готовим заявку для ООО") {
		t.Fatalf("тело шага не исполнено; out=%q", out.String())
	}
	tasks, _ := st.ListPendingTasks("")
	if len(tasks) != 1 {
		t.Fatalf("ожидалась 1 открытая задача, получено %d", len(tasks))
	}
}

// TestReactivateInstanceIdempotentTask — инстанс «выполняется» уже на человеческом шаге
// «согласовать» с УЖЕ сохранённой открытой задачей (рестарт демона повторно дошёл до
// шага) → реактивация НЕ создаёт дубль: ровно 1 открытая задача, id прежней не изменился
// (Фикс A, идемпотентность создания задачи).
func TestReactivateInstanceIdempotentTask(t *testing.T) {
	_, st, eng, _ := buildStack(t, reactSrc, goldenMoment())
	inst := stuckInstance("p-000010", "согласовать", store.StatusRunning)
	if err := st.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}
	// Предзаливаем открытую задачу на этом же шаге (остаток прерванного прогона).
	tid, err := st.NextTaskID()
	if err != nil {
		t.Fatalf("NextTaskID: %v", err)
	}
	preTask := &store.Task{
		ID:         tid,
		InstanceID: inst.ID,
		StepName:   "согласовать",
		Assignee:   "менеджер",
		Status:     store.TaskPending,
		CreatedAt:  goldenMoment(),
	}
	if err := st.SaveTask(preTask); err != nil {
		t.Fatalf("SaveTask: %v", err)
	}

	if err := eng.ReactivateInstance(inst); err != nil {
		t.Fatalf("ReactivateInstance: неожиданная ошибка %v", err)
	}

	tasks, _ := st.ListPendingTasks("")
	if len(tasks) != 1 {
		t.Fatalf("ожидалась 1 открытая задача (без дубля), получено %d", len(tasks))
	}
	if tasks[0].ID != tid {
		t.Fatalf("id задачи изменился: было %q, стало %q (минт дубля)", tid, tasks[0].ID)
	}
	got, _ := st.LoadInstance("p-000010")
	if got.Status != store.StatusWaiting {
		t.Fatalf("статус после реактивации = %q, ожидался %q", got.Status, store.StatusWaiting)
	}
}

// TestReactivateInstanceCreatedStatus — инстанс «создан» (первый шаг не активирован) →
// реактивация прогоняет с первого шага так же, как «выполняется» (оба залипших статуса
// сканируются демоном).
func TestReactivateInstanceCreatedStatus(t *testing.T) {
	_, st, eng, _ := buildStack(t, reactSrc, goldenMoment())
	inst := stuckInstance("p-000002", "подготовить", store.StatusCreated)
	if err := st.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}
	if err := eng.ReactivateInstance(inst); err != nil {
		t.Fatalf("ReactivateInstance: неожиданная ошибка %v", err)
	}
	got, _ := st.LoadInstance("p-000002")
	if got.Status != store.StatusWaiting {
		t.Fatalf("статус = %q, ожидался ожидает", got.Status)
	}
}

// TestReactivateInstanceDrift — CurrentStep отсутствует в перезагруженном определении
// (дрейф исходника) → ErrInstanceDrift без паники, инстанс НЕ тронут (шаг не угадан,
// Принцип IX). FR-020, SC-008.
func TestReactivateInstanceDrift(t *testing.T) {
	_, st, eng, _ := buildStack(t, reactSrc, goldenMoment())
	inst := stuckInstance("p-000003", "удалённый_шаг", store.StatusRunning)
	if err := st.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}

	err := eng.ReactivateInstance(inst)
	if !stderrors.Is(err, engine.ErrInstanceDrift) {
		t.Fatalf("ожидался ErrInstanceDrift, получено %v", err)
	}
	// Инстанс не тронут: статус и шаг сохранены как были.
	got, _ := st.LoadInstance("p-000003")
	if got.Status != store.StatusRunning || got.CurrentStep != "удалённый_шаг" {
		t.Fatalf("дрейф-инстанс изменён: статус=%q шаг=%q", got.Status, got.CurrentStep)
	}
}

// TestReactivateInstanceUnknownProcess — процесс инстанса отсутствует в исходнике
// (дрейф процесса целиком) → ErrInstanceDrift.
func TestReactivateInstanceUnknownProcess(t *testing.T) {
	_, st, eng, _ := buildStack(t, reactSrc, goldenMoment())
	inst := &store.ProcessInstance{
		ID:          "p-000004",
		ProcessName: "несуществующий",
		Status:      store.StatusRunning,
		CurrentStep: "шаг1",
		CreatedAt:   goldenMoment(),
	}
	if err := st.SaveInstance(inst); err != nil {
		t.Fatalf("SaveInstance: %v", err)
	}
	if err := eng.ReactivateInstance(inst); !stderrors.Is(err, engine.ErrInstanceDrift) {
		t.Fatalf("ожидался ErrInstanceDrift для неизвестного процесса, получено %v", err)
	}
}
