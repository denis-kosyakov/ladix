package store

import "time"

// Store — нарезанный контракт хранилища состояния движка (D-3, §EN-2). Ровно
// 8 методов над ProcessInstance/Task. Две реализации за одним интерфейсом —
// MemoryStore и SQLiteStore.
//
// Триггерные методы (LoadTriggerState/SaveTriggerState/NextEventID/EnqueueEvent/
// ListUnprocessedEvents/MarkEventProcessed) и ErrTriggerStateNotFound в 006 НЕ
// объявляются — 007 добавит аддитивно. Методов листинга инстансов нет намеренно
// (D-4). Транзакционного комбо-метода «завершить + продвинуть» нет: корректность
// сбойного окна обеспечивает идемпотентный гард-догон D-4.
type Store interface {
	SaveInstance(inst *ProcessInstance) error         // upsert: создание и обновление
	LoadInstance(id string) (*ProcessInstance, error) // не найден → ErrInstanceNotFound

	SaveTask(t *Task) error
	LoadTask(id string) (*Task, error)                        // не найдена → ErrTaskNotFound
	ListPendingTasks(assignee string) ([]*Task, error)        // assignee=="" → все открытые; порядок — по возрастанию ID (D-15)
	MarkTaskCompleted(id string, completedAt time.Time) error // атомарно открыта→завершена (D-12); повтор → ErrTaskAlreadyCompleted

	NextInstanceID() (string, error) // mint "p-NNNNNN" (D-10)
	NextTaskID() (string, error)     // mint "t-NNNNNN"
}

// Проверки соответствия реализаций контракту (compile-time).
var (
	_ Store = (*MemoryStore)(nil)
	_ Store = (*SQLiteStore)(nil)
)
