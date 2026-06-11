// Package store — хранилище состояния движка процессов Ladix (фича 006, §EN-2).
//
// Нарезанный контракт (D-3): типы данных (ProcessInstance/Task/статусы/сентинелы),
// интерфейс Store ровно из 8 методов и две реализации — MemoryStore (эфемерно,
// без алиасинга) и SQLiteStore (персистентно, modernc.org/sqlite). Сентинелы —
// английские: наружу не печатаются, транслируются в русские тексты §EN-8.
package store

import (
	"errors"
	"time"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// Status — статус жизненного цикла инстанса процесса (EM-2, §EN-2).
type Status string

const (
	StatusCreated   Status = "создан"      // персистирован, первый шаг ещё не активирован (транзиентно)
	StatusRunning   Status = "выполняется" // активный шаг исполняет тело
	StatusWaiting   Status = "ожидает"     // активный шаг создал Task, инстанс спит
	StatusDone      Status = "выполнен"    // все шаги готовы (терминал)
	StatusFailed    Status = "провален"    // runtime-ошибка шага/атрибута (терминал)
	StatusCancelled Status = "отменён"     // зарезервирован; в v1 недостижим (SPEC §12)
)

// ProcessInstance — инстанс процесса (EM-2, §EN-2; 7 полей, без изменений).
type ProcessInstance struct {
	ID          string                 // "p-NNNNNN" (D-10)
	ProcessName string                 // имя ProcessDecl
	Status      Status                 //
	CurrentStep string                 // имя активного шага; при терминале — последний обработанный
	Variables   map[string]value.Value // переменные процесса; пусть-локали шага сюда НЕ попадают
	CreatedAt   time.Time              // engine-Clock (D-2)
	UpdatedAt   time.Time              // выставляет движок перед КАЖДЫМ SaveInstance
}

// TaskStatus — статус задачи человека-в-цикле (EM-3, §EN-2).
type TaskStatus string

const (
	TaskPending   TaskStatus = "открыта"
	TaskCompleted TaskStatus = "завершена"
)

// Task — задача человека-в-цикле (EM-3, §EN-2; 8 полей, без изменений).
type Task struct {
	ID          string     // "t-NNNNNN" (D-10)
	InstanceID  string     // → ProcessInstance.ID
	StepName    string     // шаг, породивший задачу
	Assignee    string     // значение «исполнитель» (Строка, D-18)
	Deadline    *time.Time // CreatedAt + «срок» (D-19); nil, если «срок» не задан
	Status      TaskStatus //
	CreatedAt   time.Time  //
	CompletedAt *time.Time // nil, пока открыта; выставляет MarkTaskCompleted (D-12)
}

// Сентинелы Store (D-3, §EN-2). Английские: наружу не печатаются, транслируются
// в русские тексты §EN-8.B на CLI-слое.
var (
	ErrInstanceNotFound     = errors.New("process instance not found")
	ErrTaskNotFound         = errors.New("task not found")
	ErrTaskAlreadyCompleted = errors.New("task already completed")
)
