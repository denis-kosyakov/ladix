package errors

import (
	stderrors "errors"
	"testing"
)

// T003: двухстрочный канонический формат LexError.Error() (SPEC §13) и развёртка
// через errors.As.
func TestLexErrorTwoLineFormat(t *testing.T) {
	tests := []struct {
		name string
		err  LexError
		want string
	}{
		{
			name: "простое описание, позиция (1,1)",
			err:  LexError{Pos: Position{Line: 1, Col: 1}, Msg: "незакрытый строковый литерал"},
			want: "Ошибка в строке 1, колонка 1:\nнезакрытый строковый литерал",
		},
		{
			name: "позиция в рунах (3,12), подстановка символа",
			err:  LexError{Pos: Position{Line: 3, Col: 12}, Msg: "неожиданный символ '@'"},
			want: "Ошибка в строке 3, колонка 12:\nнеожиданный символ '@'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, хотим %q", got, tt.want)
			}
		})
	}
}

func TestLexErrorAs(t *testing.T) {
	var err error = LexError{Pos: Position{Line: 2, Col: 5}, Msg: "тест"}
	var target LexError
	if !stderrors.As(err, &target) {
		t.Fatalf("errors.As не нашёл LexError в %v", err)
	}
	if target.Pos.Line != 2 || target.Pos.Col != 5 {
		t.Errorf("Pos = %+v, хотим {Line:2 Col:5}", target.Pos)
	}
}

func TestWarningHeader(t *testing.T) {
	w := Warning{Pos: Position{Line: 7, Col: 3}, Msg: "теневое имя"}
	want := "Предупреждение в строке 7, колонка 3:\nтеневое имя"
	if got := w.Error(); got != want {
		t.Errorf("Warning.Error() = %q, хотим %q", got, want)
	}
}
