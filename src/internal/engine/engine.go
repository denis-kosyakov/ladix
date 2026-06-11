// Package engine — движок исполнения процессов Ladix (фича 006, §EN-3).
//
// Lifecycle инстанса: Start/advance/Complete (EM-9 с правками D-4/D-8/D-9),
// засыпание/пробуждение (EM-10), engine-Clock (D-2), реализация eval.ProcessRuntime
// (мост §EN-4). Граф пакетов без циклов: engine → eval, store, ast, value, errors;
// eval НЕ импортирует engine/store (цикл разорван интерфейсом ProcessRuntime в eval,
// D-1). Всё синхронно в одной горутине — mutex не нужен (конкурентность между
// процессами ОС — WAL + busy_timeout, EM-11).
package engine

import (
	stderrors "errors"
	"fmt"
	"io"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// activeFrame — пара «инстанс + его processEnv» в стеке активных инстансов
// (атрибуция хука присвоить, §EN-3/§EN-4). Push при входе в advance, pop при выходе;
// вложенный «запустить процесс» из тела шага кладёт новый кадр поверх.
type activeFrame struct {
	inst       *store.ProcessInstance
	processEnv *eval.Environment
}

// Engine — движок исполнения процессов над Store и интерпретатором (§EN-3).
type Engine struct {
	st     store.Store
	interp *eval.Interpreter
	out    io.Writer
	clock  Clock
	active []*activeFrame // стек активных инстансов (атрибуция хука присвоить)
}

// NewEngine строит движок над Store и интерпретатором. out — канал системных строк
// stdout (§EN-7); в CLI совпадает с out интерпретатора (печать программы и движка
// перемешиваются в порядке исполнения — всё синхронно). Дефолтные часы — SystemClock.
func NewEngine(st store.Store, interp *eval.Interpreter, out io.Writer, opts ...Option) *Engine {
	e := &Engine{st: st, interp: interp, out: out, clock: SystemClock{}}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Start запускает процесс по имени (реализация eval.ProcessRuntime.StartProcess);
// запуск ТИХИЙ (D-13): mint id, bind параметров, статус «создан» ▼, синхронный
// advance до первого ожидания/терминала, возврат id. Ошибка advance всплывает наверх
// (инстанс уже персистирован «провален», D-14).
func (e *Engine) Start(name string, args []value.Value) (string, error) {
	pd, ok := e.interp.Process(name)
	if !ok {
		// Семпроход резолвит имя (§PM-4) — недостижимо в норме; защитно.
		return "", fmt.Errorf("процесс '%s' не найден в определении", name)
	}
	id, err := e.st.NextInstanceID()
	if err != nil {
		return "", NewStoreError(err)
	}
	now := e.clock.Now()
	inst := &store.ProcessInstance{
		ID:          id,
		ProcessName: name,
		Status:      store.StatusCreated,
		CurrentStep: pd.Steps[0].Name.Name,
		Variables:   bindParams(pd.Params, args),
		CreatedAt:   now,
	}
	if err := e.save(inst); err != nil {
		return "", err
	}
	if err := e.advance(inst); err != nil {
		return id, err
	}
	return id, nil
}

// CompleteResult — итог Complete для CLI (маппинг exit-кодов и тесты). Печать строк
// 7-10 (§EN-7) делает САМ Complete в e.out; CLI по этой структуре ничего не печатает.
type CompleteResult struct {
	Instance *store.ProcessInstance // состояние инстанса после продвижения
	CaughtUp bool                   // true = гард-догон D-4 (US3); в US2 всегда false
}

// Complete завершает задачу человека-в-цикле и продвигает инстанс — ПОЛНАЯ машина
// состояний §EN-3 с порядком гардов (US3):
//
//	LoadTask → LoadInstance → дрейф-гарды Q3 (процесс/шаг в определении) →
//	гард-догон D-4 (задача завершена) → гарды D-8 (статус/шаг) →
//	MarkTaskCompleted (проигравший гонку D-12 → ветка догона) →
//	печать строки 7 → продвижение (next==∅ → выполнен + строка 10; иначе advance +
//	строка 9).
//
// Любое нарушение гарда → типизированная ошибка (*GuardError, тексты §EN-8.B минус
// префикс) И инстанс НЕ тронут (до мутаций). Сбой Store на этом CLI-пути → *StoreError
// (CLI → B9, exit 2). Runtime-ошибка тела/атрибута из advance — как есть (канон §13,
// exit 1, D-14). Владелец печати строк 7-10 (§EN-7) — сам Complete (пишет в e.out).
func (e *Engine) Complete(taskID string) (CompleteResult, error) {
	t, err := e.st.LoadTask(taskID)
	if err != nil {
		return CompleteResult{}, err // ErrTaskNotFound — CLI → §EN-8.B B1; иначе сбой Store
	}
	inst, err := e.st.LoadInstance(t.InstanceID)
	if err != nil {
		if stderrors.Is(err, store.ErrInstanceNotFound) {
			// Битая/чужая БД: задача есть, инстанса нет (B3). Несём id для CLI.
			return CompleteResult{}, GuardInstanceNotFound(t.InstanceID)
		}
		return CompleteResult{}, NewStoreError(err)
	}

	// Дрейф-гарды Q3 (ДО любых мутаций): процесс инстанса есть в файле; CurrentStep ∈
	// pd.Steps. Идут раньше гарда «уже завершена» (§EN-9 негатив (4)).
	pd, ok := e.interp.Process(inst.ProcessName)
	if !ok {
		return CompleteResult{}, &GuardError{Kind: GuardProcessNotInDefKind, ProcName: inst.ProcessName}
	}
	if !stepInDef(pd, inst.CurrentStep) {
		return CompleteResult{}, &GuardError{
			Kind: GuardStepNotInDefKind, StepName: inst.CurrentStep, ProcName: inst.ProcessName,
		}
	}

	// Гард-догон D-4: задача УЖЕ завершена. Если инстанс ожидает на том же шаге —
	// хвост сбойного окна: идемпотентное до-продвижение БЕЗ MarkTaskCompleted, строка 8.
	if t.Status == store.TaskCompleted {
		return e.catchUp(inst, t)
	}

	// Гарды D-8 (открытая задача): статус ожидает; соответствие текущему шагу.
	if inst.Status != store.StatusWaiting {
		return CompleteResult{}, &GuardError{
			Kind: GuardInstanceNotWaitingKind, InstanceID: inst.ID, Status: string(inst.Status),
		}
	}
	if inst.CurrentStep != t.StepName {
		return CompleteResult{}, &GuardError{
			Kind: GuardStepMismatchKind, TaskID: t.ID, InstanceID: inst.ID,
		}
	}

	// Завершение задачи (D-12: проигравший гонку получит ErrTaskAlreadyCompleted и
	// уходит в ветку догона — инстанс перечитываем).
	if err := e.st.MarkTaskCompleted(taskID, e.clock.Now()); err != nil {
		if stderrors.Is(err, store.ErrTaskAlreadyCompleted) {
			fresh, lerr := e.st.LoadInstance(t.InstanceID)
			if lerr != nil {
				return CompleteResult{}, NewStoreError(lerr)
			}
			t.Status = store.TaskCompleted
			return e.catchUp(fresh, t)
		}
		return CompleteResult{}, NewStoreError(err)
	}
	// Печать строки 7 ДО продвижения: задача уже завершена фактом (§EN-7 строка 7).
	fmt.Fprintf(e.out, "задача %s завершена\n", taskID)
	return e.advanceAfterComplete(inst, false)
}

// catchUp — ветка гард-догона D-4: задача УЖЕ была завершена. Применима, только если
// инстанс ожидает на том же шаге (хвост сбойного окна «MarkTaskCompleted прошёл,
// advance не успел»); иначе → «уже завершена», exit 2. Печатает строку 8 §EN-7 ВМЕСТО
// строки 7, далее до-продвигает БЕЗ повторного MarkTaskCompleted (CaughtUp=true).
func (e *Engine) catchUp(inst *store.ProcessInstance, t *store.Task) (CompleteResult, error) {
	if inst.Status != store.StatusWaiting || inst.CurrentStep != t.StepName {
		return CompleteResult{}, &GuardError{Kind: GuardAlreadyCompletedKind, TaskID: t.ID}
	}
	// Строка 8 §EN-7 (вместо строки 7).
	fmt.Fprintf(e.out, "задача %s уже была завершена, инстанс до-продвинут\n", t.ID)
	return e.advanceAfterComplete(inst, true)
}

// advanceAfterComplete продвигает инстанс после (до-)завершения задачи и печатает
// итоговую строку §EN-7 (9 или 10). next==∅ → выполнен (строка 10); иначе advance
// (может снова заснуть → строка 9, или провалиться → D-14, итоговой строки НЕТ).
func (e *Engine) advanceAfterComplete(inst *store.ProcessInstance, caughtUp bool) (CompleteResult, error) {
	next, ok := e.nextStep(inst.ProcessName, inst.CurrentStep)
	if !ok {
		// Терминал: следующего шага нет → выполнен (§EN-7 строка 10).
		inst.Status = store.StatusDone
		if serr := e.save(inst); serr != nil {
			return CompleteResult{CaughtUp: caughtUp}, serr
		}
		fmt.Fprintf(e.out, "инстанс %s: выполнен\n", inst.ID)
		return CompleteResult{Instance: inst, CaughtUp: caughtUp}, nil
	}
	// Есть следующий шаг: продвигаемся (advance может снова заснуть или провалиться).
	inst.CurrentStep = next
	if err := e.advance(inst); err != nil {
		// Провал продвижения (D-14): инстанс уже провален внутри advance; итоговой
		// строки 9/10 НЕТ. Сбой Store → *StoreError (CLI → B9); runtime → канон §13.
		return CompleteResult{Instance: inst, CaughtUp: caughtUp}, err
	}
	// Итог: инстанс либо снова ожидает (строка 9), либо выполнен (строка 10).
	if inst.Status == store.StatusDone {
		fmt.Fprintf(e.out, "инстанс %s: выполнен\n", inst.ID)
	} else {
		fmt.Fprintf(e.out, "инстанс %s: %s, шаг '%s'\n", inst.ID, inst.Status, inst.CurrentStep)
	}
	return CompleteResult{Instance: inst, CaughtUp: caughtUp}, nil
}

// stepInDef сообщает, есть ли шаг с именем stepName в определении процесса (дрейф Q3).
func stepInDef(pd *ast.ProcessDecl, stepName string) bool {
	for _, s := range pd.Steps {
		if s.Name.Name == stepName {
			return true
		}
	}
	return false
}

// bindParams связывает параметры процесса с позиционными аргументами (§EN-3).
// Лишние параметры без аргумента остаются неопределёнными (семпроход гарантирует
// совпадение арности, checkRunProcess).
func bindParams(params []ast.Ident, args []value.Value) map[string]value.Value {
	m := make(map[string]value.Value, len(params))
	for k, p := range params {
		if k < len(args) {
			m[p.Name] = args[k]
		}
	}
	return m
}

// advance крутит шаги до ожидания/терминала (машина состояний §EN-3). processEnv —
// ОДИН на весь прогон; stepEnv — свой на каждый шаг. Перед каждым ▼ выставляется
// UpdatedAt = clock.Now().
func (e *Engine) advance(inst *store.ProcessInstance) error {
	processEnv := eval.NewEnvironment(e.interp.GlobalEnv())
	for name, v := range inst.Variables {
		processEnv.Define(name, v)
	}
	// Push кадра в стек active для атрибуции хука присвоить (§EN-4).
	frame := &activeFrame{inst: inst, processEnv: processEnv}
	e.active = append(e.active, frame)
	defer func() { e.active = e.active[:len(e.active)-1] }()

	for {
		step, ok := e.lookupStep(inst.ProcessName, inst.CurrentStep)
		if !ok {
			// CurrentStep не найден в определении — защитно (норма §11.2).
			return fmt.Errorf("шаг '%s' не найден в определении процесса '%s'", inst.CurrentStep, inst.ProcessName)
		}
		inst.Status = store.StatusRunning
		if err := e.save(inst); err != nil {
			return err
		}
		stepEnv := eval.NewEnvironment(processEnv)

		// (1) фаза атрибутов ДО тела (D-9): исполнитель → Строка, срок → Длительность.
		assignee, hasAssignee, err := e.evalAssignee(step, stepEnv)
		if err != nil {
			return e.fail(inst, err)
		}
		deadlineDur, hasDeadline, err := e.evalDeadline(step, stepEnv)
		if err != nil {
			return e.fail(inst, err)
		}

		// (2) тело шага: каждое присвоить внутри ▼ персистит (хук §EN-4).
		if _, err := e.interp.ExecStepBody(processEnv, stepEnv, step.Body); err != nil {
			return e.fail(inst, err)
		}

		// (3) развилка: человеческий шаг → заснуть; иначе продвижение/терминал.
		if hasAssignee {
			now := e.clock.Now()
			tid, err := e.st.NextTaskID()
			if err != nil {
				return NewStoreError(err)
			}
			task := &store.Task{
				ID:         tid,
				InstanceID: inst.ID,
				StepName:   step.Name.Name,
				Assignee:   assignee,
				Status:     store.TaskPending,
				CreatedAt:  now,
			}
			if hasDeadline {
				dl := AddDuration(now, deadlineDur.Amount, deadlineDur.Unit)
				task.Deadline = &dl
			}
			if err := e.st.SaveTask(task); err != nil {
				return NewStoreError(err)
			}
			e.printTaskCreated(task)
			inst.Status = store.StatusWaiting
			if err := e.save(inst); err != nil {
				return err
			}
			return nil // засыпание (EM-10)
		}

		next, ok := e.nextStep(inst.ProcessName, inst.CurrentStep)
		if !ok {
			inst.Status = store.StatusDone
			if err := e.save(inst); err != nil {
				return err
			}
			return nil // терминал; печати нет (тихо)
		}
		inst.CurrentStep = next
	}
}

// evalAssignee вычисляет атрибут исполнитель (если задан) и проверяет тип Строка
// (D-18). Возвращает значение, флаг наличия атрибута и ошибку.
func (e *Engine) evalAssignee(step *ast.StepDecl, stepEnv *eval.Environment) (string, bool, error) {
	if step.Assignee == nil {
		return "", false, nil
	}
	v, err := e.interp.EvalExpr(stepEnv, step.Assignee)
	if err != nil {
		return "", false, err
	}
	s, ok := v.(value.Строка)
	if !ok {
		return "", false, typeErr(step.Assignee.Pos(),
			fmt.Sprintf("шаг '%s': исполнитель должен быть Строка, получено %s", step.Name.Name, v.TypeName()))
	}
	return s.V, true, nil
}

// evalDeadline вычисляет атрибут срок (если задан) и проверяет тип Длительность
// (D-18). Возвращает значение, флаг наличия атрибута и ошибку.
func (e *Engine) evalDeadline(step *ast.StepDecl, stepEnv *eval.Environment) (value.Длительность, bool, error) {
	if step.Deadline == nil {
		return value.Длительность{}, false, nil
	}
	v, err := e.interp.EvalExpr(stepEnv, step.Deadline)
	if err != nil {
		return value.Длительность{}, false, err
	}
	d, ok := v.(value.Длительность)
	if !ok {
		return value.Длительность{}, false, typeErr(step.Deadline.Pos(),
			fmt.Sprintf("шаг '%s': срок должен быть Длительность, получено %s", step.Name.Name, v.TypeName()))
	}
	return d, true, nil
}

// fail переводит инстанс в провален (D-14), персистит и возвращает исходную ошибку
// (всплывает как обычная ошибка программы — канон §13, exit 1).
func (e *Engine) fail(inst *store.ProcessInstance, cause error) error {
	inst.Status = store.StatusFailed
	if serr := e.save(inst); serr != nil {
		return serr
	}
	return cause
}

// save выставляет UpdatedAt = clock.Now() и персистит инстанс (▼, EM-9). Сбой Store
// заворачивается в *StoreError (§EN-8/FR-018: на путях Ladix-узла — §EN-8.A exit 1,
// на CLI-путях complete — §EN-8.B B9 exit 2; различие — по инициатору, в вызывающем).
func (e *Engine) save(inst *store.ProcessInstance) error {
	inst.UpdatedAt = e.clock.Now()
	if err := e.st.SaveInstance(inst); err != nil {
		return NewStoreError(err)
	}
	return nil
}

// lookupStep ищет шаг по имени в определении процесса (порядок исходника, §11.2).
func (e *Engine) lookupStep(procName, stepName string) (*ast.StepDecl, bool) {
	pd, ok := e.interp.Process(procName)
	if !ok {
		return nil, false
	}
	for _, s := range pd.Steps {
		if s.Name.Name == stepName {
			return s, true
		}
	}
	return nil, false
}

// nextStep возвращает имя следующего шага по порядку исходника (§11.2); ok=false на
// последнем шаге.
func (e *Engine) nextStep(procName, stepName string) (string, bool) {
	pd, ok := e.interp.Process(procName)
	if !ok {
		return "", false
	}
	for k, s := range pd.Steps {
		if s.Name.Name == stepName {
			if k+1 < len(pd.Steps) {
				return pd.Steps[k+1].Name.Name, true
			}
			return "", false
		}
	}
	return "", false
}

// typeErr строит ОшибкуТипа фазы атрибутов шага (D-18, §EN-8.A) с позицией выражения
// атрибута. Движок — владелец этих диагностик (семпроход атрибуты не обходит).
func typeErr(p ast.Position, msg string) error {
	return errors.ОшибкаТипа{Pos: errors.Position{Line: p.Line, Col: p.Col}, Msg: msg}
}

// printTaskCreated печатает строку создания задачи (§EN-7, строки 3/4).
func (e *Engine) printTaskCreated(t *store.Task) {
	if t.Deadline != nil {
		fmt.Fprintf(e.out, "[задача] %s → %s, шаг '%s', срок до %s\n",
			t.ID, t.Assignee, t.StepName, t.Deadline.Format(deadlineLayout))
		return
	}
	fmt.Fprintf(e.out, "[задача] %s → %s, шаг '%s'\n", t.ID, t.Assignee, t.StepName)
}
