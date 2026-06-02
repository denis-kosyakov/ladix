package errors

import (
	stderrors "errors"
	"strings"
	"testing"
)

// T004: агрегат ErrorList — накопление в порядке обнаружения, Unwrap()/errors.As,
// мягкий предел ≈20.
func TestErrorListAccumulationOrder(t *testing.T) {
	l := NewErrorList()
	l.Add(LexError{Pos: Position{Line: 1, Col: 1}, Msg: "первая"})
	l.Add(LexError{Pos: Position{Line: 2, Col: 1}, Msg: "вторая"})

	if l.Len() != 2 {
		t.Fatalf("Len = %d, хотим 2", l.Len())
	}
	got := l.Errors()
	if le, ok := got[0].(LexError); !ok || le.Msg != "первая" {
		t.Errorf("ошибка[0] = %v, хотим Msg=первая", got[0])
	}
	if le, ok := got[1].(LexError); !ok || le.Msg != "вторая" {
		t.Errorf("ошибка[1] = %v, хотим Msg=вторая", got[1])
	}
}

func TestErrorListUnwrapAs(t *testing.T) {
	l := NewErrorList()
	l.Add(LexError{Pos: Position{Line: 4, Col: 7}, Msg: "ошибка"})

	var target LexError
	if !stderrors.As(l, &target) {
		t.Fatalf("errors.As по агрегату не нашёл LexError")
	}
	if target.Pos.Col != 7 {
		t.Errorf("Col = %d, хотим 7", target.Pos.Col)
	}
}

func TestErrorListSoftBudget(t *testing.T) {
	l := NewErrorList()
	for i := 0; i < DefaultErrorBudget+10; i++ {
		l.Add(LexError{Pos: Position{Line: i + 1, Col: 1}, Msg: "x"})
	}
	if l.Len() != DefaultErrorBudget {
		t.Errorf("Len = %d, хотим мягкий предел %d", l.Len(), DefaultErrorBudget)
	}
}

func TestErrorListNilAndEmpty(t *testing.T) {
	l := NewErrorList()
	if !l.Empty() {
		t.Errorf("новый агрегат должен быть пуст")
	}
	l.Add(nil)
	if !l.Empty() {
		t.Errorf("nil-ошибка не должна накапливаться")
	}
}

// TestErrorListErrorRender фиксирует ErrorList.Error() (FR-026): сообщения
// склеиваются РОВНО через один пустой разделитель "\n\n" и лексер НЕ печатает
// сводку «Найдено K ошибок.» — это слой-потребитель.
func TestErrorListErrorRender(t *testing.T) {
	l := NewErrorList()
	l.Add(LexError{Pos: Position{Line: 1, Col: 1}, Msg: "первая"})
	l.Add(LexError{Pos: Position{Line: 2, Col: 3}, Msg: "вторая"})

	got := l.Error()
	want := "Ошибка в строке 1, колонка 1:\nпервая\n\nОшибка в строке 2, колонка 3:\nвторая"
	if got != want {
		t.Errorf("Error() = %q,\nхотим = %q", got, want)
	}
	if strings.Contains(got, "Найдено") {
		t.Errorf("агрегат не должен печатать сводку «Найдено …»: %q", got)
	}

	if e := NewErrorList().Error(); e != "" {
		t.Errorf("пустой агрегат: Error() = %q, хотим \"\"", e)
	}
}
