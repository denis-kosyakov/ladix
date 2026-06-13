package store

import "time"

// Store — нарезанный контракт хранилища состояния движка (D-3, §EN-2). 8 методов
// 006 над ProcessInstance/Task + 6 триггерных методов и ListInstancesByStatus
// (рестарт-скан, осознанное отступление «+6→+7», deviation FR-022, 007b).
// Две реализации за одним интерфейсом — MemoryStore и SQLiteStore.
//
// Транзакционного комбо-метода «завершить + продвинуть» нет: корректность сбойного
// окна обеспечивает идемпотентный гард-догон D-4.
type Store interface {
	SaveInstance(inst *ProcessInstance) error         // upsert: создание и обновление
	LoadInstance(id string) (*ProcessInstance, error) // не найден → ErrInstanceNotFound

	SaveTask(t *Task) error
	LoadTask(id string) (*Task, error)                        // не найдена → ErrTaskNotFound
	ListPendingTasks(assignee string) ([]*Task, error)        // assignee=="" → все открытые; порядок — по возрастанию ID (D-15)
	MarkTaskCompleted(id string, completedAt time.Time) error // атомарно открыта→завершена (D-12); повтор → ErrTaskAlreadyCompleted

	NextInstanceID() (string, error) // mint "p-NNNNNN" (D-10)
	NextTaskID() (string, error)     // mint "t-NNNNNN"

	// --- триггерные методы (007b, аддитивно; обещание engine-model «+6») ---
	LoadTriggerState(triggerID string) (*TriggerState, error) // не найдено → ErrTriggerStateNotFound
	SaveTriggerState(ts *TriggerState) error                  // upsert
	NextEventID() (string, error)                             // mint "e-NNNNNN"
	EnqueueEvent(e *Event) error                              // запись в очередь (Processed=false)
	ListUnprocessedEvents() ([]*Event, error)                 // FIFO по CreatedAt; пусто → []
	MarkEventProcessed(id string) error                       // идемпотентно

	// --- рестарт-скан (007b, ОСОЗНАННОЕ ОТСТУПЛЕНИЕ «+6→+7», FR-022) ---
	ListInstancesByStatus(status string) ([]*ProcessInstance, error) // по возрастанию ID; пусто → []
}

// Проверки соответствия реализаций контракту (compile-time).
var (
	_ Store = (*MemoryStore)(nil)
	_ Store = (*SQLiteStore)(nil)
)
