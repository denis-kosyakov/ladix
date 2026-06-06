package errors

import (
	stderrors "errors"
	"testing"
)

// Три категории eval (стадии 3/4) дают тот же двухстрочный канон §8.1, что и
// LexError/ParseError, и развёртываются через errors.As по значению (Guardrail 14).

func TestEvalErrorsTwoLineFormat(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			"СемантическаяОшибка",
			СемантическаяОшибка{Pos: Position{Line: 3, Col: 1}, Msg: "функция 'f' не объявлена"},
			"Ошибка в строке 3, колонка 1:\nфункция 'f' не объявлена",
		},
		{
			"ОшибкаТипа",
			ОшибкаТипа{Pos: Position{Line: 2, Col: 7}, Msg: "условие должно быть Булево, получено Целое"},
			"Ошибка в строке 2, колонка 7:\nусловие должно быть Булево, получено Целое",
		},
		{
			"ОшибкаВыполнения позиция в рунах (5,14)",
			ОшибкаВыполнения{Pos: Position{Line: 5, Col: 14}, Msg: "деление на ноль"},
			"Ошибка в строке 5, колонка 14:\nделение на ноль",
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

func TestEvalErrorsAs(t *testing.T) {
	var sem error = СемантическаяОшибка{Pos: Position{Line: 1, Col: 1}, Msg: "x"}
	var ty error = ОшибкаТипа{Pos: Position{Line: 1, Col: 1}, Msg: "x"}
	var rt error = ОшибкаВыполнения{Pos: Position{Line: 1, Col: 1}, Msg: "x"}

	var asSem СемантическаяОшибка
	var asTy ОшибкаТипа
	var asRt ОшибкаВыполнения
	if !stderrors.As(sem, &asSem) {
		t.Errorf("errors.As не нашёл СемантическаяОшибка")
	}
	if !stderrors.As(ty, &asTy) {
		t.Errorf("errors.As не нашёл ОшибкаТипа")
	}
	if !stderrors.As(rt, &asRt) {
		t.Errorf("errors.As не нашёл ОшибкаВыполнения")
	}
	// категории различимы: ОшибкаТипа — не ОшибкаВыполнения
	var wrongCat ОшибкаВыполнения
	if stderrors.As(ty, &wrongCat) {
		t.Errorf("ОшибкаТипа ошибочно распозналась как ОшибкаВыполнения")
	}
}
