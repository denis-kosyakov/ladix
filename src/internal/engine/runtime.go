package engine

import (
	stderrors "errors"
	"fmt"
	"sort"

	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// Engine реализует eval.ProcessRuntime (мост §EN-4, D-1). Compile-time проверка
// соответствия (engine импортирует eval; цикла нет — eval не импортирует engine).
var _ eval.ProcessRuntime = (*Engine)(nil)

// StartProcess запускает процесс по имени (§EN-4): делегирует Start.
func (e *Engine) StartProcess(name string, args []value.Value) (string, error) {
	return e.Start(name, args)
}

// AssignProcessVar — хук персиста «присвоить» (§EN-4): значение уже записано в
// processEnv интерпретатором; движок обновляет Variables активного инстанса (вершина
// стека active) и персистит (▼SaveInstance).
func (e *Engine) AssignProcessVar(name string, v value.Value) error {
	if len(e.active) == 0 {
		// Защитно: присвоить вне тела шага невозможно (семпроход, §PM-1).
		return fmt.Errorf("присвоить вне активного инстанса")
	}
	frame := e.active[len(e.active)-1]
	if frame.inst.Variables == nil {
		frame.inst.Variables = make(map[string]value.Value)
	}
	frame.inst.Variables[name] = v
	return e.save(frame.inst)
}

// CallExternalResult — выражение-форма «вызвать» (B1, §AU-3): ДЕЛЕГИРУЕТ драйверу
// e.caller.Call (B2, §AU-4.1). Под дефолт-стабом — печать §EN-7.2 + (value.None, nil);
// под webhookCaller — POST + декодированный ответ (или ошибка доставки).
func (e *Engine) CallExternalResult(target string, args []value.Value) (value.Value, error) {
	return e.caller.Call(target, args)
}

// CallExternal — statement-форма «вызвать» (D-13): делегирует e.caller.Call, отбрасывая
// значение (эффект РОВНО один раз — нет двойной печати/POST).
func (e *Engine) CallExternal(target string, args []value.Value) error {
	_, err := e.caller.Call(target, args)
	return err
}

// Notify — «уведомить» (D-13, §EN-7.1/1а): ДЕЛЕГИРУЕТ e.caller.Notify (best-effort).
// Под дефолт-стабом — печать; под webhookCaller — POST (ответ игнорируется).
func (e *Engine) Notify(target string, args []value.Value) error {
	return e.caller.Notify(target, args)
}

// InstanceStatus — статус инстанса по id (§EN-4); ok=false → builtin даёт «процесс
// '<id>' не найден» (D-15). err — только сбой Store.
func (e *Engine) InstanceStatus(id string) (string, bool, error) {
	inst, err := e.st.LoadInstance(id)
	if err != nil {
		if stderrors.Is(err, store.ErrInstanceNotFound) {
			return "", false, nil
		}
		return "", false, NewStoreError(err)
	}
	return string(inst.Status), true, nil
}

// InstanceVariables — переменные инстанса как Запись, ключи по возрастанию (D-21, §EN-4).
func (e *Engine) InstanceVariables(id string) (value.Запись, bool, error) {
	inst, err := e.st.LoadInstance(id)
	if err != nil {
		if stderrors.Is(err, store.ErrInstanceNotFound) {
			return value.Запись{}, false, nil
		}
		return value.Запись{}, false, NewStoreError(err)
	}
	keys := make([]string, 0, len(inst.Variables))
	for k := range inst.Variables {
		keys = append(keys, k)
	}
	sort.Strings(keys) // ключи по возрастанию (D-21)
	fields := make(map[string]value.Value, len(keys))
	for _, k := range keys {
		fields[k] = inst.Variables[k]
	}
	return value.NewRecord(keys, fields), true, nil
}

// UserTasks — открытые задачи исполнителя (""=все), по возрастанию id (D-15, §EN-4);
// поля Записи — таблица «Task → Запись» (ARCH §7.7): ид/процесс(=InstanceID)/шаг/
// исполнитель/статус/просрочена.
func (e *Engine) UserTasks(assignee string) ([]value.Запись, error) {
	tasks, err := e.st.ListPendingTasks(assignee)
	if err != nil {
		return nil, NewStoreError(err)
	}
	now := e.clock.Now()
	out := make([]value.Запись, 0, len(tasks))
	for _, t := range tasks {
		keys := []string{"ид", "процесс", "шаг", "исполнитель", "статус", "просрочена"}
		fields := map[string]value.Value{
			"ид":          value.Строка{V: t.ID},
			"процесс":     value.Строка{V: t.InstanceID},
			"шаг":         value.Строка{V: t.StepName},
			"исполнитель": value.Строка{V: t.Assignee},
			"статус":      value.Строка{V: string(t.Status)},
			"просрочена":  value.Булево{V: Overdue(t, now)},
		}
		out = append(out, value.NewRecord(keys, fields))
	}
	return out, nil
}
