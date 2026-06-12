package eval

import (
	stderrors "errors"
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// fakeRuntime — тестовый ProcessRuntime (граница eval↔engine, D-1). Записывает
// вызовы хуков, отдаёт детерминированный id из StartProcess.
type fakeRuntime struct {
	assigns    []assignRec
	startCalls []startRec
	notifies   []callRec
	calls      []callRec
	startID    string
	startErr   error
	assignErr  error // ошибка хука AssignProcessVar (имитация StoreError движка)
	statusErr  error // ошибка InstanceStatus (имитация StoreError движка)
	varsErr    error // ошибка InstanceVariables
	tasksErr   error // ошибка UserTasks
	// process-builtins (D-15): сценарные ответы для InstanceStatus/InstanceVariables/UserTasks.
	statusByID map[string]string // id → статус (отсутствует → ok=false)
	varsByID   map[string]value.Запись
	userTasks  []value.Запись
}

type assignRec struct {
	name string
	v    value.Value
}
type startRec struct {
	name string
	args []value.Value
}
type callRec struct {
	target string
	args   []value.Value
}

func (f *fakeRuntime) StartProcess(name string, args []value.Value) (string, error) {
	f.startCalls = append(f.startCalls, startRec{name, args})
	if f.startErr != nil {
		return "", f.startErr
	}
	return f.startID, nil
}
func (f *fakeRuntime) AssignProcessVar(name string, v value.Value) error {
	f.assigns = append(f.assigns, assignRec{name, v})
	return f.assignErr
}
func (f *fakeRuntime) CallExternal(target string, args []value.Value) error {
	f.calls = append(f.calls, callRec{target, args})
	return nil
}
func (f *fakeRuntime) Notify(target string, args []value.Value) error {
	f.notifies = append(f.notifies, callRec{target, args})
	return nil
}
func (f *fakeRuntime) InstanceStatus(id string) (string, bool, error) {
	if f.statusErr != nil {
		return "", false, f.statusErr
	}
	s, ok := f.statusByID[id]
	return s, ok, nil
}
func (f *fakeRuntime) InstanceVariables(id string) (value.Запись, bool, error) {
	if f.varsErr != nil {
		return value.Запись{}, false, f.varsErr
	}
	v, ok := f.varsByID[id]
	return v, ok, nil
}
func (f *fakeRuntime) UserTasks(assignee string) ([]value.Запись, error) {
	if f.tasksErr != nil {
		return nil, f.tasksErr
	}
	return f.userTasks, nil
}

// parseSteps парсит процесс и возвращает интерпретатор (после Analyze) и шаги.
func parseSteps(t *testing.T, src string, rt ProcessRuntime) (*Interpreter, []*ast.StepDecl) {
	t.Helper()
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные ошибки разбора: %v", errList.Error())
	}
	interp := NewInterpreter(&stringsBuilderWriter{}, 0, testClock)
	if rt != nil {
		interp.SetProcessRuntime(rt)
	}
	if err := interp.Analyze(prog); err != nil {
		t.Fatalf("неожиданная семантическая ошибка: %v", err.Error())
	}
	pd, ok := interp.Process("онбординг")
	if !ok {
		t.Fatalf("процесс онбординг не зарегистрирован")
	}
	return interp, pd.Steps
}

type stringsBuilderWriter struct{ b strings.Builder }

func (w *stringsBuilderWriter) Write(p []byte) (int, error) { return w.b.Write(p) }
func (w *stringsBuilderWriter) String() string              { return w.b.String() }

const procWithAssign = `процесс онбординг(сотрудник):
    шаг завести_доступы:
        присвоить имя = сотрудник
        пусть локаль = 7
`

// TestExecStepBodyThreeLayerFrame — трёхслойный кадр global→processEnv→stepEnv:
// присвоить пишет в processEnv (виден в Locals()) и дёргает хук AssignProcessVar;
// пусть-локаль шага в processEnv НЕ утекает.
func TestExecStepBodyThreeLayerFrame(t *testing.T) {
	rt := &fakeRuntime{}
	interp, steps := parseSteps(t, procWithAssign, rt)

	processEnv := NewEnvironment(interp.GlobalEnv())
	processEnv.Define("сотрудник", value.Строка{V: "Петров"})
	stepEnv := NewEnvironment(processEnv)

	sig, err := interp.ExecStepBody(processEnv, stepEnv, steps[0].Body)
	if err != nil {
		t.Fatalf("ExecStepBody: %v", err)
	}
	if sig.Kind != SigNormal {
		t.Errorf("сигнал = %v, хотим SigNormal", sig.Kind)
	}

	// присвоить имя = сотрудник → процессный слой.
	locals := processEnv.Locals()
	if got := value.String(locals["имя"]); got != "Петров" {
		t.Errorf("processEnv.Locals()[имя] = %q, хотим Петров", got)
	}
	// пусть локаль = 7 → слой шага, НЕ процессный.
	if _, ok := locals["локаль"]; ok {
		t.Errorf("пусть-локаль шага утекла в processEnv: %v", locals)
	}
	if _, ok := stepEnv.Locals()["локаль"]; !ok {
		t.Errorf("пусть-локаль шага не в stepEnv: %v", stepEnv.Locals())
	}

	// Хук AssignProcessVar дёрнут ровно для «имя».
	if len(rt.assigns) != 1 || rt.assigns[0].name != "имя" {
		t.Fatalf("AssignProcessVar вызовы = %+v, хотим один для «имя»", rt.assigns)
	}
	if value.String(rt.assigns[0].v) != "Петров" {
		t.Errorf("хук получил %q, хотим Петров", value.String(rt.assigns[0].v))
	}
}

// TestAssignActionStorageFailureSinglePrefix — регресс §EN-8.A: ошибка хука присвоить
// НЕ дублирует префикс «сбой хранилища:». Движок отдаёт уже обёрнутую StoreError
// («сбой хранилища: <причина>»), eval добавляет лишь позицию узла (§13), не повторяя
// префикс (как builtins_process.go). До фикса Msg удваивался.
func TestAssignActionStorageFailureSinglePrefix(t *testing.T) {
	rt := &fakeRuntime{assignErr: stderrors.New("сбой хранилища: диск переполнен")}
	interp, steps := parseSteps(t, procWithAssign, rt)

	processEnv := NewEnvironment(interp.GlobalEnv())
	processEnv.Define("сотрудник", value.Строка{V: "Петров"})
	stepEnv := NewEnvironment(processEnv)

	_, err := interp.ExecStepBody(processEnv, stepEnv, steps[0].Body)
	if !isRuntime(err) {
		t.Fatalf("категория ошибки = %T, хотим ОшибкаВыполнения", err)
	}
	line, _, msg := evalErr(t, err)
	if msg != "сбой хранилища: диск переполнен" {
		t.Errorf("Msg = %q, хотим единичный префикс «сбой хранилища: диск переполнен» (§EN-8.A)", msg)
	}
	if line != 3 {
		t.Errorf("строка позиции = %d, хотим 3 (узел присвоить)", line)
	}
}

const procWithPlainAssign = `процесс онбординг(сотрудник):
    шаг завести_доступы:
        сотрудник = "Иванов"
`

// TestPlainAssignNoHook — x = E (обычный AssignStmt) мутирует слой процесса БЕЗ
// персист-хука (канал персиста — только присвоить).
func TestPlainAssignNoHook(t *testing.T) {
	rt := &fakeRuntime{}
	interp, steps := parseSteps(t, procWithPlainAssign, rt)

	processEnv := NewEnvironment(interp.GlobalEnv())
	processEnv.Define("сотрудник", value.Строка{V: "Петров"})
	stepEnv := NewEnvironment(processEnv)

	if _, err := interp.ExecStepBody(processEnv, stepEnv, steps[0].Body); err != nil {
		t.Fatalf("ExecStepBody: %v", err)
	}
	if got := value.String(processEnv.Locals()["сотрудник"]); got != "Иванов" {
		t.Errorf("processEnv[сотрудник] = %q, хотим Иванов (Assign вверх по цепочке)", got)
	}
	if len(rt.assigns) != 0 {
		t.Errorf("x = E не должен дёргать хук, получили %+v", rt.assigns)
	}
}

// TestRunProcessExprReturnsString — запустить процесс P(args) → value.Строка{V:id}.
func TestRunProcessExprReturnsString(t *testing.T) {
	rt := &fakeRuntime{startID: "p-000042"}
	src := `процесс онбординг(сотрудник):
    шаг s:
        исполнитель: "x"

пусть id = запустить процесс онбординг("Петров")
`
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("ошибки разбора: %v", errList.Error())
	}
	interp := NewInterpreter(&stringsBuilderWriter{}, 0, testClock)
	interp.SetProcessRuntime(rt)
	if err := interp.Run(prog); err != nil {
		t.Fatalf("Run: %v", err)
	}
	v, ok := interp.GlobalEnv().Lookup("id")
	if !ok {
		t.Fatalf("id не привязан")
	}
	s, ok := v.(value.Строка)
	if !ok {
		t.Fatalf("id типа %s, хотим Строка", v.TypeName())
	}
	if s.V != "p-000042" {
		t.Errorf("id = %q, хотим p-000042", s.V)
	}
	if len(rt.startCalls) != 1 || rt.startCalls[0].name != "онбординг" {
		t.Errorf("StartProcess вызовы = %+v", rt.startCalls)
	}
	if len(rt.startCalls[0].args) != 1 || value.String(rt.startCalls[0].args[0]) != "Петров" {
		t.Errorf("аргументы StartProcess = %+v, хотим [Петров]", rt.startCalls[0].args)
	}
}

// TestNilRuntimeGuard — runtime==nil → ОшибкаВыполнения §EN-8.A.
func TestNilRuntimeGuard(t *testing.T) {
	src := `процесс онбординг(x):
    шаг s:
        исполнитель: "y"

пусть id = запустить процесс онбординг(1)
`
	_, err := run(t, src) // SetProcessRuntime НЕ вызван
	if err == nil {
		t.Fatalf("ожидали ошибку nil-runtime")
	}
	if !strings.Contains(err.Error(), "внутренняя ошибка: движок процессов не подключён") {
		t.Errorf("текст = %q, хотим содержащий 'внутренняя ошибка: движок процессов не подключён'", err.Error())
	}
}
