package lexer

import (
	"testing"

	"github.com/denis-kosyakov/ladix/internal/errors"
)

// lexAll прогоняет лексер по src и возвращает поток токенов и агрегат ошибок.
func lexAll(src string) ([]Token, *errors.ErrorList) {
	return New(src).Tokenize()
}

func tokTypes(toks []Token) []TokenType {
	out := make([]TokenType, len(toks))
	for i, tk := range toks {
		out[i] = tk.Type
	}
	return out
}

// requireTypes сверяет последовательность видов токенов с эталоном.
func requireTypes(t *testing.T, toks []Token, want ...TokenType) {
	t.Helper()
	got := tokTypes(toks)
	if len(got) != len(want) {
		t.Fatalf("число токенов = %d %v, хотим %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("токен[%d] = %s, хотим %s\nполный поток: %v", i, got[i], want[i], got)
		}
	}
}

// requireNoErrors требует пустой агрегат.
func requireNoErrors(t *testing.T, el *errors.ErrorList) {
	t.Helper()
	if !el.Empty() {
		t.Fatalf("ожидалось 0 ошибок, получено %d: %v", el.Len(), el.Errors())
	}
}

// onlyError требует ровно одну LexError и возвращает её.
func onlyError(t *testing.T, el *errors.ErrorList) errors.LexError {
	t.Helper()
	if el.Len() != 1 {
		t.Fatalf("ожидалась ровно 1 ошибка, получено %d: %v", el.Len(), el.Errors())
	}
	le, ok := el.Errors()[0].(errors.LexError)
	if !ok {
		t.Fatalf("ошибка не LexError, а %T", el.Errors()[0])
	}
	return le
}

// lastType возвращает вид последнего токена потока.
func lastType(toks []Token) TokenType {
	if len(toks) == 0 {
		return INVALID
	}
	return toks[len(toks)-1].Type
}
