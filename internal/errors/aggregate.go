package errors

import "strings"

// DefaultErrorBudget — мягкий предел числа накопленных ошибок за один прогон
// статической фазы. Бюджет общий с парсером (A3/FR-025): по достижении предела
// новые ошибки не накапливаются, но поток токенов от этого не зависит и всегда
// завершается EOF.
const DefaultErrorBudget = 20

// ErrorList — агрегат нескольких ошибок, собранных за один прогон (panic-mode).
// Передаётся явно как значение/указатель; пакет-уровневого состояния нет (FR-029).
type ErrorList struct {
	errs   []error
	budget int
}

// NewErrorList создаёт пустой агрегат с бюджетом по умолчанию.
func NewErrorList() *ErrorList {
	return &ErrorList{budget: DefaultErrorBudget}
}

// cap возвращает действующий бюджет; нулевой/отрицательный трактуется как
// DefaultErrorBudget, чтобы и нулевое значение ErrorList{} было пригодно.
func (l *ErrorList) cap() int {
	if l.budget <= 0 {
		return DefaultErrorBudget
	}
	return l.budget
}

// Add добавляет ошибку в порядке обнаружения, если бюджет не исчерпан. По
// достижении предела новые ошибки молча отбрасываются (мягкий предел).
func (l *ErrorList) Add(err error) {
	if err == nil {
		return
	}
	if len(l.errs) >= l.cap() {
		return
	}
	l.errs = append(l.errs, err)
}

// Len возвращает число накопленных ошибок.
func (l *ErrorList) Len() int { return len(l.errs) }

// Empty сообщает, пуст ли агрегат.
func (l *ErrorList) Empty() bool { return len(l.errs) == 0 }

// Errors возвращает накопленные ошибки в порядке обнаружения.
func (l *ErrorList) Errors() []error { return l.errs }

// Unwrap делает агрегат совместимым с errors.As/errors.Is (Go 1.20+): перебор
// отдельных ошибок (например LexError) стандартными средствами.
func (l *ErrorList) Unwrap() []error { return l.errs }

// Error возвращает все сообщения, разделённые пустой строкой. Сводку
// «Найдено K ошибок.» лексер НЕ печатает — это слой-потребитель (FR-026).
func (l *ErrorList) Error() string {
	parts := make([]string, len(l.errs))
	for i, e := range l.errs {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "\n\n")
}
