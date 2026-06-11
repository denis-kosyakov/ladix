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
	mu        sync.Mutex
	instances map[string]*ProcessInstance
	tasks     map[string]*Task
	instSeq   int64
	taskSeq   int64
}

// NewMemoryStore создаёт пустой MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		instances: make(map[string]*ProcessInstance),
		tasks:     make(map[string]*Task),
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
