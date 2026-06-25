package eval

import (
	stderrors "errors"
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// try_test.go — семантика исполнения пытаться/словить (029, §6.1): REDIRECT на ЛЮБОЙ
// runtime-ошибке тела try → словить; control-flow (вернуть/прервать/продолжить) —
// отдельный канал Signal, проходит НАСКВОЗЬ (словить НЕ исполняется); ошибка ВНУТРИ
// словить терминальна (нет catch-of-catch).

// --- REDIRECT: runtime-ошибки тела try ловятся словить ---

func TestEvalTryCatchesDivByZero(t *testing.T) {
	out, err := run(t, "пытаться:\n    печать(1 / 0)\nсловить:\n    печать(\"поймано\")\n")
	if err != nil {
		t.Fatalf("деление на ноль не поймано словить: %v", err)
	}
	if strings.TrimSpace(out) != "поймано" {
		t.Errorf("out = %q, хотим «поймано» (try прерван на ошибке, словить исполнен)", out)
	}
}

func TestEvalTryCatchesTypeError(t *testing.T) {
	out, err := run(t, "пытаться:\n    печать(1 + \"x\")\nсловить:\n    печать(\"тип пойман\")\n")
	if err != nil {
		t.Fatalf("ошибка типа не поймана словить: %v", err)
	}
	if strings.TrimSpace(out) != "тип пойман" {
		t.Errorf("out = %q, хотим «тип пойман»", out)
	}
}

func TestEvalTryNoErrorSkipsCatch(t *testing.T) {
	// Нет ошибки → словить НЕ исполняется (сигнал try-блока проходит насквозь).
	out, err := run(t, "пытаться:\n    печать(\"ок\")\nсловить:\n    печать(\"не должно\")\n")
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.TrimSpace(out) != "ок" {
		t.Errorf("out = %q, хотим «ок» (словить НЕ исполнен — ошибки не было)", out)
	}
}

// --- Control-flow проходит насквозь (НЕ ловится) ---

func TestEvalTryReturnPassesThrough(t *testing.T) {
	src := "функция f():\n    пытаться:\n        вернуть 42\n    словить:\n        вернуть 99\nпечать(f())\n"
	out, err := run(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.TrimSpace(out) != "42" {
		t.Errorf("out = %q, хотим «42» (вернуть из try прошёл насквозь, словить НЕ исполнен)", out)
	}
}

func TestEvalTryBreakPassesThrough(t *testing.T) {
	// i=1: s=1; i=2: прервать (насквозь, цикл оборван) → s остаётся 1. словить НЕ исполнен.
	src := "пусть s = 0\nдля i в [1, 2, 3]:\n    пытаться:\n        если i == 2:\n            прервать\n        s = s + i\n    словить:\n        s = -100\nпечать(s)\n"
	out, err := run(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.TrimSpace(out) != "1" {
		t.Errorf("out = %q, хотим «1» (прервать из try прошёл насквозь, словить НЕ исполнен)", out)
	}
}

func TestEvalTryContinuePassesThrough(t *testing.T) {
	// i=1: s=1; i=2: продолжить (насквозь, остаток try пропущен); i=3: s=4. словить НЕ исполнен.
	src := "пусть s = 0\nдля i в [1, 2, 3]:\n    пытаться:\n        если i == 2:\n            продолжить\n        s = s + i\n    словить:\n        s = -100\nпечать(s)\n"
	out, err := run(t, src)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if strings.TrimSpace(out) != "4" {
		t.Errorf("out = %q, хотим «4» (продолжить из try прошёл насквозь)", out)
	}
}

// --- analyze рекурсирует в армы (контексты сигналов сохранены) ---

// TestEvalTryAnalyzeRecursesIntoArms — `прервать` вне цикла внутри try ловится
// семантическим проходом (checkStmt рекурсирует в армы с тем же loopDepth → сигнал-
// контекст сохранён). Мутпроба: убрать кейс *ast.TryStmt из checkStmt → армы не
// проверяются → ошибки нет → тест краснеет (err==nil).
func TestEvalTryAnalyzeRecursesIntoArms(t *testing.T) {
	_, err := run(t, "пытаться:\n    прервать\nсловить:\n    печать(1)\n")
	if err == nil {
		t.Fatalf("прервать вне цикла внутри try должен дать семантическую ошибку (analyze в армах)")
	}
	if !strings.Contains(err.Error(), "прервать") {
		t.Errorf("ошибка = %q, хотим про 'прервать' вне цикла", err.Error())
	}
}

// --- ошибка ВНУТРИ словить терминальна (нет catch-of-catch) ---

func TestEvalTryErrorInCatchIsTerminal(t *testing.T) {
	// try падает (деление), словить тоже падает (нет переменной) → ошибка словить всплывает.
	_, err := run(t, "пытаться:\n    печать(1 / 0)\nсловить:\n    печать(нетпеременной)\n")
	if err == nil {
		t.Fatalf("ошибка внутри словить должна всплыть (терминал), получили nil")
	}
	if strings.Contains(err.Error(), "деление на ноль") {
		t.Errorf("всплыла ошибка try, а не словить (catch error должен доминировать): %v", err)
	}
}

// --- сбой вызвать в теле шага ловится словить (домен процессов) ---

const procTryCatchCall = `процесс онбординг:
    шаг отправить:
        пытаться:
            вызвать crm(1)
            присвоить статус = "ок"
        словить:
            присвоить статус = "деградировано"
`

// TestEvalTryCatchesCallFailure — сбой внешнего `вызвать` (драйвер вернул error) в теле
// шага ловится `словить`: ExecStepBody возвращает (SigNormal, nil) → шаг завершается,
// инстанс продвигается (Engine.fail НЕ зовётся, Q7). Остаток try (`присвоить статус=ок`)
// НЕ исполнен (вызвать упал до него); словить дал ровно одну запись статуса.
func TestEvalTryCatchesCallFailure(t *testing.T) {
	rt := &fakeRuntime{callResultErr: stderrors.New("CRM 503")}
	interp, steps := parseSteps(t, procTryCatchCall, rt)
	processEnv := NewEnvironment(interp.GlobalEnv())
	stepEnv := NewEnvironment(processEnv)

	sig, err := interp.ExecStepBody(processEnv, stepEnv, steps[0].Body)
	if err != nil {
		t.Fatalf("сбой вызвать не пойман словить: %v", err)
	}
	if sig.Kind != SigNormal {
		t.Errorf("сигнал = %v, хотим SigNormal (шаг продолжается)", sig.Kind)
	}
	if len(rt.calls) != 1 || rt.calls[0].target != "crm" {
		t.Errorf("вызовы = %+v, хотим один вызов crm (попытка доставки)", rt.calls)
	}
	if len(rt.assigns) != 1 {
		t.Fatalf("assigns = %+v, хотим РОВНО 1 (только словить; try-присвоить после вызвать НЕ исполнен)", rt.assigns)
	}
	if rt.assigns[0].name != "статус" || value.String(rt.assigns[0].v) != "деградировано" {
		t.Errorf("assign = {%s=%s}, хотим {статус=деградировано}", rt.assigns[0].name, value.String(rt.assigns[0].v))
	}
}
