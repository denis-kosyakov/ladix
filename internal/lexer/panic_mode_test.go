package lexer

import (
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/errors"
)

// T033 [US3]: panic-mode (вариант a) — несколько ошибок за прогон без фантомов;
// проблемная лексема пропускается, валидные токены остатка эмитятся; поток → EOF;
// мягкий предел ≈20 (SC-005).

func TestPanicModeTwoIndependentErrors(t *testing.T) {
	// US3.1: ошибки на строках 1 и 3 → ровно две с верными позициями, поток → EOF.
	src := "пусть x = @\nпусть y = 5\nпусть z = $"
	toks, errs := lexAll(src)
	if errs.Len() != 2 {
		t.Fatalf("число ошибок = %d, хотим 2: %v", errs.Len(), errs.Errors())
	}
	e0 := errs.Errors()[0].(errors.LexError)
	e1 := errs.Errors()[1].(errors.LexError)
	if e0.Pos.Line != 1 || e0.Pos.Col != 11 {
		t.Errorf("ошибка[0] позиция = %+v, хотим {1 11}", e0.Pos)
	}
	if e1.Pos.Line != 3 || e1.Pos.Col != 11 {
		t.Errorf("ошибка[1] позиция = %+v, хотим {3 11}", e1.Pos)
	}
	if lastType(toks) != EOF {
		t.Errorf("последний токен = %s, хотим EOF", lastType(toks))
	}
}

func TestPanicModeBestEffortRemainder(t *testing.T) {
	// US3.2: проблемный токен '@' пропускается, валидный остаток строки эмитится.
	src := "пусть x = @ + 5"
	toks, errs := lexAll(src)
	le := onlyError(t, errs)
	if le.Pos.Col != 11 || le.Msg != "неожиданный символ '@'" {
		t.Errorf("ошибка = %+v / %q, хотим {1 11} / неожиданный символ '@'", le.Pos, le.Msg)
	}
	requireTypes(t, toks, KW_LET, IDENT, ASSIGN, PLUS, INT, NEWLINE, EOF)
}

func TestPanicModeSoftBudget(t *testing.T) {
	// US3.3: ошибок больше лимита → накопитель ≤ ≈20, поток → EOF.
	src := strings.Repeat("@\n", 30)
	toks, errs := lexAll(src)
	if errs.Len() != errors.DefaultErrorBudget {
		t.Errorf("число ошибок = %d, хотим мягкий предел %d", errs.Len(), errors.DefaultErrorBudget)
	}
	if lastType(toks) != EOF {
		t.Errorf("последний токен = %s, хотим EOF", lastType(toks))
	}
}

func TestPanicModeNoPhantomCascade(t *testing.T) {
	// После первой ошибки в строке новые ошибки той же строки подавляются.
	// "@ @ @" на одной строке → ровно одна ошибка (без фантомного каскада).
	_, errs := lexAll("@ @ @")
	if errs.Len() != 1 {
		t.Errorf("число ошибок = %d, хотим 1 (подавление до конца строки)", errs.Len())
	}
}
