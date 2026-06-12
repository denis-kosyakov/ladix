package eval

import (
	stderrors "errors"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// Регресс F-LOCK-GAP-1: три процессные read-builtins (статус_процесса /
// состояние_процесса / задачи_пользователя) на ветке err != nil оборачивают сбой
// Store через runtimeErrWrap(pos, err), СОХРАНЯЯ причину в ОшибкаВыполнения.Cause
// (Unwrap). Тип обязан дойти по Unwrap-цепочке до классификатора §EN-8.B B9 на
// пути complete→advance→ExecStepBody (read-builtin легален в теле шага). Под
// уплощением runtimeErr(pos, err.Error()) Cause==nil → errors.Is провалится —
// дискриминирующий ассерт ловит немого мутанта. Граница D-1: причина — обычный
// error через stderrors.New, пакет engine в eval-тест не импортируется.

// TestStatusProtsessaStoreFaultPreservesCause — статус_процесса сохраняет причину сбоя Store.
func TestStatusProtsessaStoreFaultPreservesCause(t *testing.T) {
	sentinel := stderrors.New("сбой хранилища: диск переполнен")
	rt := &fakeRuntime{statusErr: sentinel}
	src := `процесс онбординг(сотрудник):
    шаг s:
        пусть st = статус_процесса("p-1")
`
	interp, steps := parseSteps(t, src, rt)
	processEnv := NewEnvironment(interp.GlobalEnv())
	processEnv.Define("сотрудник", value.Строка{V: "Петров"})
	stepEnv := NewEnvironment(processEnv)

	_, err := interp.ExecStepBody(processEnv, stepEnv, steps[0].Body)
	if !isRuntime(err) {
		t.Fatalf("категория = %T, хотим ОшибкаВыполнения", err)
	}
	// ДИСКРИМИНИРУЮЩИЙ ассерт: причина сохранена через Cause/Unwrap.
	if !stderrors.Is(err, sentinel) {
		t.Errorf("причина потеряна: errors.Is(err, sentinel)=false; " +
			"runtimeErrWrap должен сохранять Cause (под уплощением Cause==nil)")
	}
}

// TestSostoyanieProtsessaStoreFaultPreservesCause — состояние_процесса сохраняет причину сбоя Store.
func TestSostoyanieProtsessaStoreFaultPreservesCause(t *testing.T) {
	sentinel := stderrors.New("сбой хранилища: диск переполнен")
	rt := &fakeRuntime{varsErr: sentinel}
	src := `процесс онбординг(сотрудник):
    шаг s:
        пусть v = состояние_процесса("p-1")
`
	interp, steps := parseSteps(t, src, rt)
	processEnv := NewEnvironment(interp.GlobalEnv())
	processEnv.Define("сотрудник", value.Строка{V: "Петров"})
	stepEnv := NewEnvironment(processEnv)

	_, err := interp.ExecStepBody(processEnv, stepEnv, steps[0].Body)
	if !isRuntime(err) {
		t.Fatalf("категория = %T, хотим ОшибкаВыполнения", err)
	}
	if !stderrors.Is(err, sentinel) {
		t.Errorf("причина потеряна: errors.Is(err, sentinel)=false; " +
			"runtimeErrWrap должен сохранять Cause (под уплощением Cause==nil)")
	}
}

// TestZadachiPolzovatelyaStoreFaultPreservesCause — задачи_пользователя сохраняет причину сбоя Store.
func TestZadachiPolzovatelyaStoreFaultPreservesCause(t *testing.T) {
	sentinel := stderrors.New("сбой хранилища: диск переполнен")
	rt := &fakeRuntime{tasksErr: sentinel}
	src := `процесс онбординг(сотрудник):
    шаг s:
        пусть z = задачи_пользователя("Петров")
`
	interp, steps := parseSteps(t, src, rt)
	processEnv := NewEnvironment(interp.GlobalEnv())
	processEnv.Define("сотрудник", value.Строка{V: "Петров"})
	stepEnv := NewEnvironment(processEnv)

	_, err := interp.ExecStepBody(processEnv, stepEnv, steps[0].Body)
	if !isRuntime(err) {
		t.Fatalf("категория = %T, хотим ОшибкаВыполнения", err)
	}
	if !stderrors.Is(err, sentinel) {
		t.Errorf("причина потеряна: errors.Is(err, sentinel)=false; " +
			"runtimeErrWrap должен сохранять Cause (под уплощением Cause==nil)")
	}
}
