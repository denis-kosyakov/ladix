package engine

import (
	"strings"
	"time"

	"github.com/denis-kosyakov/ladix/internal/store"
)

// deadlineLayout — формат времени дедлайна в выводе (§EN-7): локальная зона.
const deadlineLayout = "2006-01-02 15:04"

// FormatTaskLine — ЕДИНСТВЕННЫЙ источник формата строки задачи (D-22, §EN-7 строка
// 6). Поля через ДВА пробела: <t-id>  <p-id>  '<шаг>'  <исполнитель>. Хвост
// «срок до <время>» добавляется только при дедлайне; хвост «ПРОСРОЧЕНА» — только при
// Overdue(t, now). Используется и `ladix tasks`, и сводкой `run`.
func FormatTaskLine(t *store.Task, now time.Time) string {
	parts := []string{
		t.ID,
		t.InstanceID,
		"'" + t.StepName + "'",
		t.Assignee,
	}
	if t.Deadline != nil {
		parts = append(parts, "срок до "+t.Deadline.Format(deadlineLayout))
	}
	if Overdue(t, now) {
		parts = append(parts, "ПРОСРОЧЕНА")
	}
	return strings.Join(parts, "  ")
}

// Overdue сообщает, просрочена ли задача: now.After(*t.Deadline). При nil-дедлайне —
// false (EM-13).
func Overdue(t *store.Task, now time.Time) bool {
	if t.Deadline == nil {
		return false
	}
	return now.After(*t.Deadline)
}
