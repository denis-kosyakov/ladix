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

// TriggerState — durable-состояние триггера между тиками и рестартами (EM-17.2,
// 007b, аддитивно). Указатели = трёхзначность: nil ⇒ «нет значения для этого вида
// / не праймлен». Читается/пишется только демоном (FR-025). Заводится только для
// метрика- и расписание-триггеров; событие-триггер строки не имеет (FR-023).
type TriggerState struct {
	TriggerID     string     // ключ: "trg-<N>", N 0-based порядок объявления (EM-17.2.1, FR-023)
	Kind          string     // "metric" | "schedule_every" | "schedule_at"
	LastBool      *bool      // metric: базовая линия edge-детекта; nil = ещё не праймлен (FR-006/007)
	LastFire      *time.Time // schedule_every: момент последнего срабатывания (RFC3339); nil = не зарегистрирован (FR-011)
	LastFiredDate *string    // schedule_at: "YYYY-MM-DD" последнего срабатывания; nil = ещё не срабатывал (FR-013)
}

// Event — запись внешнего события в очереди доставки (EM-17.3, 007b, аддитивно).
// Создаётся командой emit (другой процесс ОС), разбирается демоном на тике FIFO,
// помечается processed ПОСЛЕ исполнения тела (at-least-once, FR-017).
type Event struct {
	ID          string    // opaque, "e-NNNNNN" (mint через NextEventID, зеркало p-/t-)
	Name        string    // имя события — матч с EventTrigger.Event.Name (FR-016)
	PayloadJSON string    // сырой JSON payload; маппится в value.Запись при обработке (FR-016)
	CreatedAt   time.Time // FIFO-порядок разбора (FR-016, SC-006)
	Processed   bool      // false=в очереди; true=обработано/отброшено (FR-017)
}

// Сентинелы Store (D-3, §EN-2). Английские: наружу не печатаются, транслируются
// в русские тексты §EN-8.B на CLI-слое.
var (
	ErrInstanceNotFound     = errors.New("process instance not found")
	ErrTaskNotFound         = errors.New("task not found")
	ErrTaskAlreadyCompleted = errors.New("task already completed")
	// ErrTriggerStateNotFound — LoadTriggerState не нашёл строку (прайминг/первая
	// регистрация, EM-17.2, 007b). Зеркало ErrInstanceNotFound/ErrTaskNotFound;
	// развёртка через errors.Is.
	ErrTriggerStateNotFound = errors.New("trigger state not found")
)
