package engine_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// argsInt — список аргументов из одного Целое (для прямых вызовов Start в тестах).
func argsInt(n int64) []value.Value { return []value.Value{value.Целое{V: n}} }

// fixedClock — детерминированные часы движка для golden-сценариев (D-2, §EN-9).
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// goldenMoment — фиксированный момент сценария А (§EN-9): 2026-05-31 00:00:00 Local.
func goldenMoment() time.Time {
	return time.Date(2026, 5, 31, 0, 0, 0, 0, time.Local)
}

// buildStack компилирует исходник и собирает стек interp+Store+Engine с
// фиксированными часами; возвращает interp, st, eng и общий out-буфер.
func buildStack(t *testing.T, src string, now time.Time) (*eval.Interpreter, store.Store, *engine.Engine, *bytes.Buffer) {
	t.Helper()
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные лекс/синт ошибки: %s", errList.Error())
	}
	var out bytes.Buffer
	interp := eval.NewInterpreter(&out, 0, eval.SystemClock{})
	st := store.NewMemoryStore()
	eng := engine.NewEngine(st, interp, &out, engine.WithClock(fixedClock{now}))
	interp.SetProcessRuntime(eng)
	if err := interp.Analyze(prog); err != nil {
		t.Fatalf("неожиданная семантическая ошибка: %s", err.Error())
	}
	return interp, st, eng, &out
}

const onboardingSrc = `процесс онбординг(сотрудник):
    шаг завести_доступы:
        присвоить имя = сотрудник
        уведомить ИТ("создать учётку для " + сотрудник)
    шаг провести_встречу после завести_доступы:
        исполнитель: "руководитель"
        срок:        3дн
    шаг закрыть_адаптацию после провести_встречу:
        исполнитель: "HR"
        срок:        5дн

пусть id = запустить процесс онбординг("Петров")
печать("запущен онбординг, id:", id)
`

// TestScenarioA — байт-точный сценарий А (§EN-9): run онбординга (MemoryStore) →
// 5 строк stdout + состояние Store на выходе.
func TestScenarioA(t *testing.T) {
	interp, st, _, out := buildStack(t, onboardingSrc, goldenMoment())

	tokens, errList := lexer.New(onboardingSrc).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if err := interp.Run(prog); err != nil {
		t.Fatalf("неожиданная ошибка Run: %s", err.Error())
	}
	// Сводка висящих задач — как делает CLI run (§EN-6 шаг 4, строки 5/6).
	pending, err := st.ListPendingTasks("")
	if err != nil {
		t.Fatalf("ListPendingTasks: %v", err)
	}
	if len(pending) > 0 {
		fmt.Fprintf(out, "открытых задач: %d\n", len(pending))
		for _, tk := range pending {
			fmt.Fprintln(out, engine.FormatTaskLine(tk, goldenMoment()))
		}
	}

	want := "" +
		"[уведомление] ИТ: создать учётку для Петров\n" +
		"[задача] t-000001 → руководитель, шаг 'провести_встречу', срок до 2026-06-03 00:00\n" +
		"запущен онбординг, id: p-000001\n" +
		"открытых задач: 1\n" +
		"t-000001  p-000001  'провести_встречу'  руководитель  срок до 2026-06-03 00:00\n"
	if got := out.String(); got != want {
		t.Errorf("stdout байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}

	// Состояние Store: инстанс p-000001 ожидает/провести_встречу, Variables {имя,сотрудник}.
	inst, err := st.LoadInstance("p-000001")
	if err != nil {
		t.Fatalf("LoadInstance: %v", err)
	}
	if inst.Status != store.StatusWaiting {
		t.Errorf("статус инстанса = %q, хотим %q", inst.Status, store.StatusWaiting)
	}
	if inst.CurrentStep != "провести_встречу" {
		t.Errorf("CurrentStep = %q, хотим %q", inst.CurrentStep, "провести_встречу")
	}
	if got := value.String(inst.Variables["имя"]); got != "Петров" {
		t.Errorf("Variables[имя] = %q, хотим %q", got, "Петров")
	}
	if got := value.String(inst.Variables["сотрудник"]); got != "Петров" {
		t.Errorf("Variables[сотрудник] = %q, хотим %q", got, "Петров")
	}
	if len(inst.Variables) != 2 {
		t.Errorf("Variables имеет %d ключей, хотим 2: %v", len(inst.Variables), inst.Variables)
	}

	// Задача t-000001 открыта.
	task, err := st.LoadTask("t-000001")
	if err != nil {
		t.Fatalf("LoadTask: %v", err)
	}
	if task.Status != store.TaskPending {
		t.Errorf("статус задачи = %q, хотим %q", task.Status, store.TaskPending)
	}
	if task.Assignee != "руководитель" {
		t.Errorf("Assignee = %q, хотим %q", task.Assignee, "руководитель")
	}
	if task.StepName != "провести_встречу" {
		t.Errorf("StepName = %q, хотим %q", task.StepName, "провести_встречу")
	}
}

// TestAttributeTypeGuard — фаза атрибутов: исполнитель не Строка → ОшибкаТипа §EN-8.A,
// инстанс провален (D-18/D-14).
func TestAttributeTypeGuard(t *testing.T) {
	src := `процесс p(x):
    шаг s:
        исполнитель: 42
        срок:        3дн

пусть id = запустить процесс p(1)
`
	_, st, eng, _ := buildStack(t, src, goldenMoment())
	_, err := eng.Start("p", argsInt(1))
	if err == nil {
		t.Fatalf("ожидали ОшибкуТипа, получили nil")
	}
	want := "шаг 's': исполнитель должен быть Строка, получено Целое"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("текст ошибки = %q, хотим содержащий %q", err.Error(), want)
	}
	inst, lerr := st.LoadInstance("p-000001")
	if lerr != nil {
		t.Fatalf("LoadInstance: %v", lerr)
	}
	if inst.Status != store.StatusFailed {
		t.Errorf("статус = %q, хотим %q", inst.Status, store.StatusFailed)
	}
}

// TestBodyDivisionByZero — тело шага падает (деление на ноль) → инстанс провален (D-14).
func TestBodyDivisionByZero(t *testing.T) {
	src := `процесс p(x):
    шаг s:
        присвоить y = 1 / 0

пусть id = запустить процесс p(1)
`
	_, st, eng, _ := buildStack(t, src, goldenMoment())
	_, err := eng.Start("p", argsInt(1))
	if err == nil {
		t.Fatalf("ожидали ОшибкуВыполнения, получили nil")
	}
	if !strings.Contains(err.Error(), "деление на ноль") {
		t.Errorf("текст = %q, хотим содержащий 'деление на ноль'", err.Error())
	}
	inst, lerr := st.LoadInstance("p-000001")
	if lerr != nil {
		t.Fatalf("LoadInstance: %v", lerr)
	}
	if inst.Status != store.StatusFailed {
		t.Errorf("статус = %q, хотим %q", inst.Status, store.StatusFailed)
	}
}

// TestDeadlineAbsolutization — абсолютизация срока D-19: множители единиц и календарный мес.
func TestDeadlineAbsolutization(t *testing.T) {
	base := time.Date(2026, 1, 31, 12, 0, 0, 0, time.Local)
	cases := []struct {
		amount int64
		unit   string
		want   time.Time
	}{
		{10, "сек", base.Add(10 * time.Second)},
		{5, "мин", base.Add(5 * time.Minute)},
		{3, "час", base.Add(3 * time.Hour)},
		{2, "дн", base.Add(2 * 24 * time.Hour)},
		{1, "нед", base.Add(168 * time.Hour)},
		{1, "мес", base.AddDate(0, 1, 0)}, // 2026-02-28 (календарно)
	}
	for _, c := range cases {
		got := engine.AddDuration(base, c.amount, c.unit)
		if !got.Equal(c.want) {
			t.Errorf("%d%s: получено %v, хотим %v", c.amount, c.unit, got, c.want)
		}
	}
}
