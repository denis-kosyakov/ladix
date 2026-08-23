package eval

import (
	stderrors "errors"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// Процесс с действиями вызвать/уведомить в теле шага (B2, §AU-4.4): обе точки
// (statement-форма) обязаны оборачивать сбой реального драйвера в ОшибкаВыполнения
// с сохранением Cause (активация runtimeErrWrap, D-14).
const procWithCallNotify = `процесс онбординг(сотрудник):
    шаг известить:
        уведомить ИТ(сотрудник)
        вызвать crm(сотрудник)
`

// execFirstAction исполняет тело шага «известить» под данным fakeRuntime, возвращает
// ошибку (первая сбойная точка прервёт исполнение).
func execStepKnown(t *testing.T, rt *fakeRuntime) error {
	t.Helper()
	interp, steps := parseSteps(t, procWithCallNotify, rt)
	processEnv := NewEnvironment(interp.GlobalEnv())
	processEnv.Define("сотрудник", value.Строка{V: "Петров"})
	stepEnv := NewEnvironment(processEnv)
	_, err := interp.ExecStepBody(processEnv, stepEnv, steps[0].Body)
	return err
}

// TestNotifyActionWrapsError — C-ERR-1 (T022): сбой Notify → ОшибкаВыполнения с
// Cause == исходная ошибка и Pos == токен «уведомить» (первое действие в теле).
func TestNotifyActionWrapsError(t *testing.T) {
	boom := stderrors.New("boom-notify")
	rt := &fakeRuntime{notifyErr: boom}
	err := execStepKnown(t, rt)
	if err == nil {
		t.Fatalf("ожидали ОшибкуВыполнения от сбоя Notify, получили nil")
	}
	var re errors.ОшибкаВыполнения
	if !stderrors.As(err, &re) {
		t.Fatalf("категория = %T, хотим errors.ОшибкаВыполнения", err)
	}
	if !stderrors.Is(err, boom) {
		t.Errorf("Cause не сохранён (Unwrap не видит boom-notify)")
	}
	// Pos == токен «уведомить»: строка 3 (тело шага), действие первое.
	if re.Pos.Line != 3 {
		t.Errorf("Pos.Line = %d, хотим 3 (токен «уведомить»)", re.Pos.Line)
	}
}

// TestCallActionWrapsError — C-ERR-1 (T022): сбой CallExternal (statement «вызвать»)
// → ОшибкаВыполнения с Cause и Pos == токен «вызвать». Notify здесь успешен (nil),
// чтобы дойти до второй точки.
func TestCallActionWrapsError(t *testing.T) {
	boom := stderrors.New("boom-call")
	rt := &fakeRuntime{callResultErr: boom}
	err := execStepKnown(t, rt)
	if err == nil {
		t.Fatalf("ожидали ОшибкуВыполнения от сбоя CallExternal, получили nil")
	}
	var re errors.ОшибкаВыполнения
	if !stderrors.As(err, &re) {
		t.Fatalf("категория = %T, хотим errors.ОшибкаВыполнения", err)
	}
	if !stderrors.Is(err, boom) {
		t.Errorf("Cause не сохранён (Unwrap не видит boom-call)")
	}
	// Pos == токен «вызвать»: строка 4 (второе действие).
	if re.Pos.Line != 4 {
		t.Errorf("Pos.Line = %d, хотим 4 (токен «вызвать»)", re.Pos.Line)
	}
}

// TestCallNotifyActionsNoErrorUnderStub — C-ERR-3 регресс: оба действия под
// дефолт-фейком (ошибок нет) исполняются без ошибки; обе точки дёрнуты.
func TestCallNotifyActionsNoErrorUnderStub(t *testing.T) {
	rt := &fakeRuntime{}
	if err := execStepKnown(t, rt); err != nil {
		t.Fatalf("под фейк-стабом ошибок быть не должно: %v", err)
	}
	if len(rt.notifies) != 1 || rt.notifies[0].target != "ИТ" {
		t.Errorf("Notify вызовы = %+v, хотим один для «ИТ»", rt.notifies)
	}
	if len(rt.calls) != 1 || rt.calls[0].target != "crm" {
		t.Errorf("CallExternal вызовы = %+v, хотим один для «crm»", rt.calls)
	}
}
