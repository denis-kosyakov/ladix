package store

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// MemoryStore — эфемерная реализация Store (FR-007, §EN-2). Карты инстансов/задач
// и счётчики id — под одним sync.Mutex. Сериализация не используется: value.Value
// лежат как есть (JSON не трогается). Назначение — ladix run без --db, metric и
// тесты lifecycle.
//
// Без алиасинга указателей: Save/Load копируют ProcessInstance и карту Variables
// (значения разделяются — ссылочность Список/Запись как в Locals()); Task —
// аналогично (Deadline/CompletedAt копируются как новые указатели). Load
// возвращает копию: мутации снаружи не видны в Store до следующего Save.
type MemoryStore struct {
	mu           sync.Mutex
	instances    map[string]*ProcessInstance
	tasks        map[string]*Task
	triggerState map[string]*TriggerState // 007b: ключ TriggerID
	events       []*Event                 // 007b: FIFO-срез (порядок = порядок Enqueue ≈ CreatedAt)
	outbox       map[string]*OutboxRecord // M3-C2b: ключ DedupKey; эфемерный леджер дедупа
	instSeq      int64
	taskSeq      int64
	eventSeq     int64 // 007b: счётчик e-NNNNNN
}

// NewMemoryStore создаёт пустой MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		instances:    make(map[string]*ProcessInstance),
		tasks:        make(map[string]*Task),
		triggerState: make(map[string]*TriggerState),
		outbox:       make(map[string]*OutboxRecord),
	}
}

func (s *MemoryStore) SaveInstance(inst *ProcessInstance) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.instances[inst.ID] = copyInstance(inst)
	return nil
}

func (s *MemoryStore) LoadInstance(id string) (*ProcessInstance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inst, ok := s.instances[id]
	if !ok {
		return nil, ErrInstanceNotFound
	}
	return copyInstance(inst), nil
}

func (s *MemoryStore) SaveTask(t *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[t.ID] = copyTask(t)
	return nil
}

func (s *MemoryStore) LoadTask(id string) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, ErrTaskNotFound
	}
	return copyTask(t), nil
}

func (s *MemoryStore) ListPendingTasks(assignee string) ([]*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Task
	for _, t := range s.tasks {
		if t.Status != TaskPending {
			continue
		}
		if assignee != "" && t.Assignee != assignee {
			continue
		}
		out = append(out, copyTask(t))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ListTasksByInstance — открытые И завершённые задачи инстанса, порядок ID ASC
// (read-only; 018 B6). Зеркало ListPendingTasks, но фильтр по InstanceID, БЕЗ фильтра
// статуса. copyTask копирует Escalated тривиально (cp := *t). Без задач → nil, nil.
func (s *MemoryStore) ListTasksByInstance(instanceID string) ([]*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Task
	for _, t := range s.tasks {
		if t.InstanceID != instanceID {
			continue
		}
		out = append(out, copyTask(t))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *MemoryStore) MarkTaskCompleted(id string, completedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return ErrTaskNotFound
	}
	if t.Status != TaskPending {
		return ErrTaskAlreadyCompleted
	}
	t.Status = TaskCompleted
	at := completedAt
	t.CompletedAt = &at
	return nil
}

func (s *MemoryStore) NextInstanceID() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.instSeq++
	return fmt.Sprintf("p-%06d", s.instSeq), nil
}

func (s *MemoryStore) NextTaskID() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskSeq++
	return fmt.Sprintf("t-%06d", s.taskSeq), nil
}

// --- триггерные методы (007b, паритет; всё под mu, без алиасинга) ---

func (s *MemoryStore) LoadTriggerState(triggerID string) (*TriggerState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ts, ok := s.triggerState[triggerID]
	if !ok {
		return nil, ErrTriggerStateNotFound
	}
	return copyTriggerState(ts), nil
}

func (s *MemoryStore) SaveTriggerState(ts *TriggerState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.triggerState[ts.TriggerID] = copyTriggerState(ts)
	return nil
}

// --- outbox-леджер (M3-C2b, аддитивно; эфемерный — дедуп процесса-жизни) ---

func (s *MemoryStore) LoadOutbox(dedupKey string) (*OutboxRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.outbox[dedupKey]
	if !ok {
		return nil, ErrOutboxNotFound
	}
	return copyOutboxRecord(rec), nil
}

func (s *MemoryStore) SaveOutbox(rec *OutboxRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outbox[rec.DedupKey] = copyOutboxRecord(rec)
	return nil
}

func (s *MemoryStore) NextEventID() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventSeq++
	return fmt.Sprintf("e-%06d", s.eventSeq), nil
}

func (s *MemoryStore) EnqueueEvent(e *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Дубль ID недостижим по построению: ID выдаёт монотонный NextEventID (e-%06d), и
	// другого источника ID у событий нет. Здесь Memory тихо append, тогда как SQLite на
	// том же дубле упал бы на UNIQUE-constraint — расхождение осознанно НЕ выравнивается
	// в v1 (недостижимый путь; выравнивание добавило бы проверку ради мёртвой ветки).
	s.events = append(s.events, copyEvent(e))
	return nil
}

func (s *MemoryStore) ListUnprocessedEvents() ([]*Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Event
	for _, e := range s.events {
		if e.Processed {
			continue
		}
		out = append(out, copyEvent(e))
	}
	// FIFO по CreatedAt, затем по ID для стабильности при равных штампах —
	// паритет с SQLite (ORDER BY created_at, id; FR-016/SC-006).
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *MemoryStore) MarkEventProcessed(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Идемпотентно: повтор по уже помеченному — no-op без ошибки (FR-017).
	for _, e := range s.events {
		if e.ID == id {
			e.Processed = true
		}
	}
	return nil
}

func (s *MemoryStore) ListInstancesByStatus(status string) ([]*ProcessInstance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*ProcessInstance
	for _, inst := range s.instances {
		if string(inst.Status) != status {
			continue
		}
		out = append(out, copyInstance(inst))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// copyTriggerState — копия состояния триггера с новыми указателями (без алиасинга,
// зеркало copyTask).
func copyTriggerState(ts *TriggerState) *TriggerState {
	cp := *ts
	if ts.LastBool != nil {
		b := *ts.LastBool
		cp.LastBool = &b
	}
	if ts.LastFire != nil {
		f := *ts.LastFire
		cp.LastFire = &f
	}
	if ts.LastFiredDate != nil {
		d := *ts.LastFiredDate
		cp.LastFiredDate = &d
	}
	return &cp
}

// copyEvent — копия события (значимые поля; алиасинга указателей нет).
func copyEvent(e *Event) *Event {
	cp := *e
	return &cp
}

// copyInstance возвращает глубокую копию инстанса: новая структура + новая карта
// Variables (значения value.Value разделяются — ссылочность Список/Запись как в
// Locals(); скаляры неизменяемы).
func copyInstance(inst *ProcessInstance) *ProcessInstance {
	cp := *inst
	cp.Variables = make(map[string]value.Value, len(inst.Variables))
	for k, v := range inst.Variables {
		cp.Variables[k] = v
	}
	return &cp
}

// copyTask возвращает копию задачи: новая структура + новые указатели на времена
// (мутация *Deadline/*CompletedAt снаружи не видна в Store).
func copyTask(t *Task) *Task {
	cp := *t
	if t.Deadline != nil {
		d := *t.Deadline
		cp.Deadline = &d
	}
	if t.CompletedAt != nil {
		c := *t.CompletedAt
		cp.CompletedAt = &c
	}
	return &cp
}

// copyOutboxRecord — глубокая копия записи леджера (M3-C2b): новый срез Args
// (значения value.Value делятся ссылочно — паритет copyInstance/Variables) и новый
// указатель DeliveredAt, чтобы мутация значений/времён в движке не протекла в леджер.
func copyOutboxRecord(rec *OutboxRecord) *OutboxRecord {
	cp := *rec
	if rec.Args != nil {
		cp.Args = make([]value.Value, len(rec.Args))
		copy(cp.Args, rec.Args)
	}
	if rec.DeliveredAt != nil {
		d := *rec.DeliveredAt
		cp.DeliveredAt = &d
	}
	return &cp
}
