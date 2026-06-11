package engine

import "fmt"

// Типизированные ошибки движка для CLI-маршрутизации (§EN-3, §EN-8, FR-018).
//
// Граница eval↔engine и CLI различают три класса ошибок Complete:
//   - *GuardError — нарушение гарда Complete (дрейф Q3 / D-8 / «уже завершена» /
//     инстанс не найден). Текст УЖЕ совпадает с §EN-8.B минус префикс «ladix: »;
//     CLI добавляет префикс и даёт exit 2. Инстанс при отказе гарда НЕ тронут.
//   - *StoreError — не-сентинельный сбой Store (обёртка «сбой хранилища: <причина>»,
//     EM-16). На CLI-путях complete/tasks → §EN-8.B B9 (ladix: …, exit 2); на путях
//     Ladix-узла (запуск/присвоить/builtins внутри Start) — §EN-8.A #8, exit 1
//     (различие — по инициатору, FR-018; см. main.go и runtime-обёртку eval).
//   - прочее (ОшибкаТипа/ОшибкаВыполнения тела/атрибута) — канон §13, exit 1 (D-14).
//
// Сентинелы Store (ErrTaskNotFound/ErrTaskAlreadyCompleted) НЕ заворачиваются в
// GuardError: CLI распознаёт их errors.Is и формирует B1/B2 сам (taskID известен CLI).

// GuardKind различает тексты §EN-8.B на уровне гарда (CLI формирует строку по полям).
type GuardKind int

const (
	// GuardInstanceNotFoundKind — LoadInstance → ErrInstanceNotFound (битая/чужая БД), B3.
	GuardInstanceNotFoundKind GuardKind = iota
	// GuardProcessNotInDefKind — дрейф Q3: ProcessName инстанса отсутствует в файле, B6.
	GuardProcessNotInDefKind
	// GuardStepNotInDefKind — дрейф Q3: CurrentStep отсутствует в ProcessDecl, B7.
	GuardStepNotInDefKind
	// GuardAlreadyCompletedKind — задача завершена И догон D-4 неприменим, B2.
	GuardAlreadyCompletedKind
	// GuardInstanceNotWaitingKind — гард D-8: Status != ожидает, B4.
	GuardInstanceNotWaitingKind
	// GuardStepMismatchKind — гард D-8: CurrentStep != task.StepName, B5.
	GuardStepMismatchKind
)

// GuardError — нарушение гарда Complete (§EN-3). Текст Error() совпадает с §EN-8.B
// минус префикс «ladix: » — CLI добавляет префикс и подбирает exit 2. Поля несут
// данные для формирования (id инстанса/задачи, статус, шаг), чтобы CLI не лез в Store.
type GuardError struct {
	Kind       GuardKind
	InstanceID string // id инстанса (B3/B4/B5/B6 — где уместно)
	TaskID     string // id задачи (B2/B5)
	Status     string // статус инстанса (B4)
	StepName   string // имя шага (B7)
	ProcName   string // имя процесса (B7)
}

func (e *GuardError) Error() string {
	switch e.Kind {
	case GuardInstanceNotFoundKind:
		return fmt.Sprintf("инстанс '%s' не найден", e.InstanceID)
	case GuardProcessNotInDefKind:
		return fmt.Sprintf("процесс '%s' не найден в определении", e.ProcName)
	case GuardStepNotInDefKind:
		return fmt.Sprintf("шаг '%s' не найден в определении процесса '%s'", e.StepName, e.ProcName)
	case GuardAlreadyCompletedKind:
		return fmt.Sprintf("задача '%s' уже завершена", e.TaskID)
	case GuardInstanceNotWaitingKind:
		return fmt.Sprintf("инстанс '%s' не ожидает (статус '%s')", e.InstanceID, e.Status)
	case GuardStepMismatchKind:
		return fmt.Sprintf("задача '%s' не соответствует текущему шагу инстанса '%s'", e.TaskID, e.InstanceID)
	default:
		return "нарушение гарда"
	}
}

// GuardInstanceNotFound — конструктор GuardError для B3 (используется и тестами CLI).
func GuardInstanceNotFound(instanceID string) *GuardError {
	return &GuardError{Kind: GuardInstanceNotFoundKind, InstanceID: instanceID}
}

// StoreError — не-сентинельный сбой Store (обёртка «сбой хранилища: <причина>», EM-16).
// Error() даёт текст БЕЗ префикса «ladix: » (его добавляет CLI на путях complete/tasks).
// Unwrap сохраняет исходную ошибку (errors.Is/As по причине). Один тип для обоих
// маршрутов §EN-8.A/§EN-8.B — различие по инициатору решает вызывающий слой (FR-018).
type StoreError struct{ cause error }

func (e *StoreError) Error() string { return "сбой хранилища: " + e.cause.Error() }
func (e *StoreError) Unwrap() error { return e.cause }

// NewStoreError оборачивает сбой Store (используется движком и тестами CLI).
func NewStoreError(cause error) *StoreError { return &StoreError{cause: cause} }
