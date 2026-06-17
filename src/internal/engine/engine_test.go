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

// emptyRec — пустая Запись для вызовов Complete без payload (регресс-путь B3, §AU-5.3):
// существующие complete-сценарии передают пустую Запись по умолчанию.
func emptyRec() value.Запись { return value.NewRecord(nil, nil) }

// recOf — Запись из упорядоченных пар (хелпер B3-тестов payload).
func recOf(pairs ...[2]string) value.Запись {
	keys := make([]string, 0, len(pairs))
	fields := make(map[string]value.Value, len(pairs))
	for _, p := range pairs {
		keys = append(keys, p[0])
		fields[p[0]] = value.Строка{V: p[1]}
	}
	return value.NewRecord(keys, fields)
}

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

// buildStackStore собирает стек interp+Engine поверх произвольного Store (для
// фабрикованных/сбойных реализаций); out — внутренний буфер (не возвращается).
func buildStackStore(t *testing.T, src string, now time.Time, st store.Store) *engine.Engine {
	t.Helper()
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные лекс/синт ошибки: %s", errList.Error())
	}
	var out bytes.Buffer
	interp := eval.NewInterpreter(&out, 0, eval.SystemClock{})
	eng := engine.NewEngine(st, interp, &out, engine.WithClock(fixedClock{now}))
	interp.SetProcessRuntime(eng)
	if err := interp.Analyze(prog); err != nil {
		t.Fatalf("неожиданная семантическая ошибка: %s", err.Error())
	}
	return eng
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

// TestNotifyCallFormats — §EN-7 строки 1/1а/2: байт-точная печать стабов действий
// «уведомить»/«вызвать» движком (CallExternal/Notify — реализация
// eval.ProcessRuntime). Покрывает форматы, недостижимые golden-сценарием онбординга
// (он печатает только строку 1): 1а — уведомить БЕЗ аргументов (без двоеточия и
// хвостовых пробелов); 2 — вызвать (разделитель ", "; без аргументов «<имя>()»).
func TestNotifyCallFormats(t *testing.T) {
	_, _, eng, out := buildStack(t, onboardingSrc, goldenMoment())

	// 1. уведомить с ≥1 аргументом (разделитель аргументов — один пробел).
	out.Reset()
	if err := eng.Notify("ИТ", []value.Value{value.Строка{V: "создать учётку для Петров"}}); err != nil {
		t.Fatalf("Notify(1): %v", err)
	}
	if got, want := out.String(), "[уведомление] ИТ: создать учётку для Петров\n"; got != want {
		t.Errorf("формат 1: got %q, want %q", got, want)
	}

	// 1а. уведомить БЕЗ аргументов — без двоеточия и хвостовых пробелов.
	out.Reset()
	if err := eng.Notify("дежурный", nil); err != nil {
		t.Fatalf("Notify(1а): %v", err)
	}
	if got, want := out.String(), "[уведомление] дежурный\n"; got != want {
		t.Errorf("формат 1а: got %q, want %q", got, want)
	}

	// 2. вызвать с аргументами — разделитель ", ".
	out.Reset()
	if err := eng.CallExternal("отправить", []value.Value{value.Строка{V: "адрес"}, value.Целое{V: 7}}); err != nil {
		t.Fatalf("CallExternal(2): %v", err)
	}
	if got, want := out.String(), "[вызов] отправить(адрес, 7)\n"; got != want {
		t.Errorf("формат 2: got %q, want %q", got, want)
	}

	// 2. вызвать без аргументов — «[вызов] <имя>()».
	out.Reset()
	if err := eng.CallExternal("пинг", nil); err != nil {
		t.Fatalf("CallExternal(2 пусто): %v", err)
	}
	if got, want := out.String(), "[вызов] пинг()\n"; got != want {
		t.Errorf("формат 2 (без аргументов): got %q, want %q", got, want)
	}
}

// TestCallExternalDelegatesToResult — C-SEAM-2.1 (B1): statement-форма
// CallExternal делегирует CallExternalResult; печать «[вызов] …» происходит РОВНО
// один раз (нет двойного эффекта). Инверсия: вернуть CallExternal к собственной
// Fprintf И вызову CallExternalResult → две строки.
func TestCallExternalDelegatesToResult(t *testing.T) {
	_, _, eng, out := buildStack(t, onboardingSrc, goldenMoment())

	out.Reset()
	if err := eng.CallExternal("crm", []value.Value{value.Строка{V: "к"}}); err != nil {
		t.Fatalf("CallExternal: %v", err)
	}
	if got, want := out.String(), "[вызов] crm(к)\n"; got != want {
		t.Errorf("CallExternal печать = %q, хотим РОВНО одну строку %q (делегирование)", got, want)
	}
}

// TestCallExternalResultStubPrintsAndReturnsNone — C-SEAM-2.3/2.4 (B1):
// выражение-форма CallExternalResult печатает ту же строку [вызов] (§EN-7 байт-в-
// байт) и возвращает (value.None, nil). Инверсия: вернуть не-None → значение ≠
// Пусто; изменить строку → golden red.
func TestCallExternalResultStubPrintsAndReturnsNone(t *testing.T) {
	_, _, eng, out := buildStack(t, onboardingSrc, goldenMoment())

	out.Reset()
	v, err := eng.CallExternalResult("отправить", []value.Value{value.Строка{V: "адрес"}, value.Целое{V: 7}})
	if err != nil {
		t.Fatalf("CallExternalResult: %v", err)
	}
	if v != value.None {
		t.Errorf("результат = %v (%s), хотим value.None (Пусто)", v, v.TypeName())
	}
	if got, want := out.String(), "[вызов] отправить(адрес, 7)\n"; got != want {
		t.Errorf("печать = %q, хотим %q (§EN-7 байт-в-байт)", got, want)
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

// TestDeadlineTypeGuard — фаза атрибутов: срок не Длительность → ОшибкаТипа §EN-8.A #6,
// инстанс провален (D-18/D-14). Exact-match текста §EN-8.A.
func TestDeadlineTypeGuard(t *testing.T) {
	src := `процесс p(x):
    шаг s:
        исполнитель: "Иванов"
        срок:        "скоро"

пусть id = запустить процесс p(1)
`
	_, st, eng, _ := buildStack(t, src, goldenMoment())
	_, err := eng.Start("p", argsInt(1))
	if err == nil {
		t.Fatalf("ожидали ОшибкуТипа на срок, получили nil")
	}
	want := "шаг 's': срок должен быть Длительность, получено Строка"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("текст = %q, хотим содержащий %q (§EN-8.A #6)", err.Error(), want)
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

// TestEN7FormatRegistry — консолидированная карта покрытия реестра §EN-7 (docs/
// engine-model.md, строки 647–660): ровно 11 stdout-форматов, у каждого ≥1 байт-
// точная (exact-match) проверка. Этот тест НЕ проверяет печать сам по себе — он
// ДОКУМЕНТИРУЕТ «формат № → покрывающий тест» и страхует от выпадения форматов из
// покрытия (как TestErrorsRegistryExactMatch для §8.3). Имена тестов — ссылки на
// фактические exact-match-ассерции (engine_test.go / complete_test.go / main_test.go).
func TestEN7FormatRegistry(t *testing.T) {
	registry := []struct {
		num     string // номер формата §EN-7
		desc    string // дословный шаблон строки
		coverBy string // тест(ы) с exact-match на этот формат
	}{
		{"1", "[уведомление] <получатель>: <арг1 арг2 …>", "engine.TestNotifyCallFormats, engine.TestScenarioA, main.TestRunOnboardingProcessDeferred"},
		{"1а", "[уведомление] <получатель>", "engine.TestNotifyCallFormats"},
		{"2", "[вызов] <имя>(<арг1, арг2, …>)", "engine.TestNotifyCallFormats"},
		{"3", "[задача] <t-id> → <исполнитель>, шаг '<шаг>', срок до <время>", "engine.TestScenarioA, main.TestRunOnboardingProcessDeferred"},
		{"4", "[задача] <t-id> → <исполнитель>, шаг '<шаг>'", "engine.TestCompleteChain, engine.TestCompleteCatchUp"},
		{"5", "открытых задач: <N>", "engine.TestScenarioA, main.TestRunOnboardingProcessDeferred"},
		{"6", "<t-id>  <p-id>  '<шаг>'  <исполнитель>  срок до <время>  ПРОСРОЧЕНА", "engine.TestScenarioA, main.TestScenarioBSQLiteChain"},
		{"7", "задача <t-id> завершена", "engine.TestCompleteChain, engine.TestCompleteTerminalDirect"},
		{"8", "задача <t-id> уже была завершена, инстанс до-продвинут", "engine.TestCompleteCatchUp"},
		{"9", "инстанс <p-id>: ожидает, шаг '<имя>'", "engine.TestCompleteChain, engine.TestCompleteCatchUp"},
		{"10", "инстанс <p-id>: выполнен", "engine.TestCompleteChain, engine.TestCompleteTerminalDirect"},
		{"11", "открытых задач нет", "main.TestScenarioBSQLiteChain"},
	}

	// Канон §EN-7 — 11 НУМЕРОВАННЫХ форматов (1..11); «1а» — помеченный вариант
	// формата №1 (уведомить без аргументов), а не отдельный номер. Считаем по
	// базовому номеру: 12 строк реестра (с «1а») ⇒ ровно 11 уникальных форматов.
	seen := map[string]bool{} // полные метки строк реестра (включая «1а»)
	base := map[string]bool{} // базовые номера 1..11 (без литеры варианта)
	for _, r := range registry {
		if seen[r.num] {
			t.Errorf("строка реестра №%s продублирована", r.num)
		}
		seen[r.num] = true
		base[strings.TrimRight(r.num, "абвг")] = true
		if r.coverBy == "" {
			t.Errorf("формат №%s (%q) не имеет покрывающего теста", r.num, r.desc)
		}
	}
	if len(seen) != 12 {
		t.Errorf("строк реестра §EN-7 = %d, хотим 12 (форматы 1..11 + вариант 1а)", len(seen))
	}
	if len(base) != 11 {
		t.Errorf("уникальных форматов §EN-7 = %d, хотим ровно 11", len(base))
	}
}
