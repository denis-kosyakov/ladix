package errors

import (
	stderrors "errors"
	"testing"
)

// T005: двухстрочный канон ParseError.Error(), развёртка errors.As и складывание
// в общий ErrorList рядом с LexError (FR-023, data-model §10).

func TestParseErrorTwoLineFormat(t *testing.T) {
	tests := []struct {
		name string
		err  ParseError
		want string
	}{
		{
			name: "канон §13.4, позиция (1,1)",
			err:  ParseError{Pos: Position{Line: 1, Col: 1}, Msg: "вложенные функции не поддерживаются в v1"},
			want: "Ошибка в строке 1, колонка 1:\nвложенные функции не поддерживаются в v1",
		},
		{
			name: "позиция в рунах (5,14)",
			err:  ParseError{Pos: Position{Line: 5, Col: 14}, Msg: "неожиданный элемент 'значение'"},
			want: "Ошибка в строке 5, колонка 14:\nнеожиданный элемент 'значение'",
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

func TestParseErrorAs(t *testing.T) {
	var err error = ParseError{Pos: Position{Line: 2, Col: 5}, Msg: "тест"}
	var target ParseError
	if !stderrors.As(err, &target) {
		t.Fatalf("errors.As не нашёл ParseError в %v", err)
	}
	if target.Pos.Line != 2 || target.Pos.Col != 5 {
		t.Errorf("Pos = %+v, хотим {Line:2 Col:5}", target.Pos)
	}
}

// TestParseErrorInSharedErrorList: лексическая и синтаксическая ошибки копятся в
// ОДНОМ ErrorList (общий бюджет, §13.2), и errors.As находит каждую категорию.
func TestParseErrorInSharedErrorList(t *testing.T) {
	l := NewErrorList()
	l.Add(LexError{Pos: Position{Line: 1, Col: 1}, Msg: "незакрытый строковый литерал"})
	l.Add(ParseError{Pos: Position{Line: 2, Col: 3}, Msg: "неожиданный элемент '{'"})

	if l.Len() != 2 {
		t.Fatalf("Len = %d, хотим 2 (лексическая+синтаксическая в общем накопителе)", l.Len())
	}
	var pe ParseError
	if !stderrors.As(l, &pe) {
		t.Fatalf("errors.As по агрегату не нашёл ParseError")
	}
	if pe.Msg != "неожиданный элемент '{'" {
		t.Errorf("Msg = %q, хотим \"неожиданный элемент '{'\"", pe.Msg)
	}
	var le LexError
	if !stderrors.As(l, &le) {
		t.Fatalf("errors.As по агрегату не нашёл LexError")
	}
}
