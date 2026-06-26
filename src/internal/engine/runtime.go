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
	if name == payloadName {
		// payload «данные» (B3, §AU-5.3) read-only: присвоить данные = … запрещено
		// (как тело триггера). Чтобы сохранить — присвоить факт = данные.поле (своя
		// переменная). Ошибка всплывает как ОшибкаВыполнения шага (D-14, провален).
		return fmt.Errorf("'%s' доступно только для чтения (payload задачи): присвойте свою переменную, напр. присвоить факт = %s.поле", payloadName, payloadName)
	}
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

// outboxPrecheck — общий пре-чек дедупа эффектов тела шага (M3-C2b, §C-2b.5, D-C-9;
// 029 Уровень 2). Активен только при len(e.active)>0 (внутри тела шага). Возвращает:
//   - active=false: нет активного кадра → НЕ дедупим, эффект-метод делегирует напрямую.
//   - skip=true: ключ уже delivered → пропустить доставку, вернуть rec.Result (реплей).
//   - frozen!=nil: запись есть, Delivered=false → ЗАМОРОЖЕННЫЙ ВЕРДИКТ СБОЯ (029):
//     пере-бросить ошибку с текстом ErrorText БЕЗ повторной доставки (детерминированный
//     реплей try/catch — упавший эффект падает идентично, catch исполнится так же).
//   - skip=false, frozen=nil, active=true: доставлять; key/idx — для outboxRecord.
//
// derr (Store-сбой при LoadOutbox) всплывает наверх (шаг провалится, D-14).
func (e *Engine) outboxPrecheck() (fr *activeFrame, key string, idx int, active bool, skip bool, replay value.Value, frozen error, derr error) {
	if len(e.active) == 0 {
		return nil, "", 0, false, false, nil, nil, nil
	}
	fr = e.active[len(e.active)-1]
	idx = fr.effectIndex
	fr.effectIndex++
	key = fmt.Sprintf("%s|%s|%d", fr.inst.ID, fr.inst.CurrentStep, idx)
	rec, err := e.st.LoadOutbox(key)
	if err != nil {
		if stderrors.Is(err, store.ErrOutboxNotFound) {
			return fr, key, idx, true, false, nil, nil, nil
		}
		return fr, key, idx, true, false, nil, nil, NewStoreError(err)
	}
	if rec.Delivered {
		return fr, key, idx, true, true, rec.Result, nil, nil
	}
	// 029 Уровень 2: запись есть, но Delivered=false → замороженный вердикт сбоя.
	return fr, key, idx, true, false, nil, frozenVerdict(rec.ErrorText), nil
}

// frozenVerdict — плоский error из замороженного вердикта сбоя (029 Уровень 2).
// Eval-обёртка (evalCallAction/evalNotifyAction/вызвать-выражение) добавит позицию
// текущего call-site через runtimeErrWrap → ОшибкаВыполнения; для голого `словить`
// содержимое не важно, но текст/позиция сохраняются для непойманного (провален) кейса.
func frozenVerdict(text string) error {
	if text == "" {
		text = "замороженный вердикт сбоя"
	}
	return stderrors.New(text)
}

// outboxRecord персистит запись делёджа ПОСЛЕ успешной доставки (deliver-then-record,
// §C-2b.5 шаг 5): Delivered=true, штампы — часы движка. Зазор между доставкой и этим
// SaveOutbox — осознанная at-least-once граница (§C-9): краш здесь → повтор на рестарте.
func (e *Engine) outboxRecord(fr *activeFrame, key string, idx int, kind, target string, args []value.Value, result value.Value) error {
	now := e.clock.Now()
	return e.st.SaveOutbox(&store.OutboxRecord{
		DedupKey:    key,
		InstanceID:  fr.inst.ID,
		StepName:    fr.inst.CurrentStep,
		EffectIndex: idx,
		Kind:        kind,
		Target:      target,
		Args:        args,
		Result:      result,
		Delivered:   true,
		CreatedAt:   now,
		DeliveredAt: &now,
	})
}

// outboxRecordFailure замораживает вердикт сбоя доставки (029 Уровень 2,
// deliver-then-record-outcome; outcome ∈ {delivered, failed}): Delivered=false,
// ErrorText=cause.Error(), Result=None, DeliveredAt=nil. На реплее outboxPrecheck
// находит !Delivered → пере-бросает ErrorText БЕЗ доставки → детерминированный путь
// try/catch. Зазор доставка→эта запись — остаточное крэш-окно (§C-9): краш здесь →
// на реплее эффект пере-доставляется (как Уровень 1), НИКОГДА не дубль записанного.
func (e *Engine) outboxRecordFailure(fr *activeFrame, key string, idx int, kind, target string, args []value.Value, cause error) error {
	now := e.clock.Now()
	return e.st.SaveOutbox(&store.OutboxRecord{
		DedupKey:    key,
		InstanceID:  fr.inst.ID,
		StepName:    fr.inst.CurrentStep,
		EffectIndex: idx,
		Kind:        kind,
		Target:      target,
		Args:        args,
		Result:      value.None,
		Delivered:   false,
		ErrorText:   cause.Error(),
		CreatedAt:   now,
	})
}

// CallExternalResult — выражение-форма «вызвать» (B1, §AU-3): ДЕЛЕГИРУЕТ драйверу
// e.caller.Call (B2, §AU-4.1). В теле шага (len(e.active)>0) — через outbox-дедуп
// (M3-C2b): на пропуске-по-дедупу возвращает СОХРАНЁННЫЙ результат без повторного Call
// (иначе логика процесса разошлась бы). Вне тела шага — прежнее прямое делегирование.
func (e *Engine) CallExternalResult(target string, args []value.Value) (value.Value, error) {
	fr, key, idx, active, skip, replay, frozen, derr := e.outboxPrecheck()
	if derr != nil {
		return nil, derr
	}
	if !active {
		return e.caller.Call(target, args)
	}
	if frozen != nil {
		return nil, frozen // 029: замороженный вердикт сбоя → пере-брос без доставки
	}
	if skip {
		return replay, nil
	}
	v, err := e.caller.Call(target, args)
	if err != nil {
		// 029 Уровень 2: заморозить вердикт сбоя, затем вернуть ошибку наверх (try/catch
		// её поймает; на реплее — пере-брос без доставки). Сбой записи вердикта (Store)
		// доминирует — провалит шаг (D-14), вердикт не заморожен → реплей пере-доставит.
		if rerr := e.outboxRecordFailure(fr, key, idx, "вызвать", target, args, err); rerr != nil {
			return nil, rerr
		}
		return nil, err
	}
	if err := e.outboxRecord(fr, key, idx, "вызвать", target, args, v); err != nil {
		return nil, err
	}
	return v, nil
}

// CallExternal — statement-форма «вызвать» (D-13): делегирует e.caller.Call, отбрасывая
// значение (эффект РОВНО один раз). В теле шага — outbox-дедуп НЕЗАВИСИМО (зовёт
// e.caller.Call напрямую, не делегирует CallExternalResult); Result хранится как None.
func (e *Engine) CallExternal(target string, args []value.Value) error {
	fr, key, idx, active, skip, _, frozen, derr := e.outboxPrecheck()
	if derr != nil {
		return derr
	}
	if !active {
		_, err := e.caller.Call(target, args)
		return err
	}
	if frozen != nil {
		return frozen // 029: замороженный вердикт сбоя → пере-брос без доставки
	}
	if skip {
		return nil
	}
	if _, err := e.caller.Call(target, args); err != nil {
		if rerr := e.outboxRecordFailure(fr, key, idx, "вызвать", target, args, err); rerr != nil {
			return rerr
		}
		return err
	}
	return e.outboxRecord(fr, key, idx, "вызвать", target, args, value.None)
}

// Notify — «уведомить» (D-13, §EN-7.1/1а): ДЕЛЕГИРУЕТ e.caller.Notify (best-effort).
// В теле шага — outbox-дедуп НЕЗАВИСИМО; Result=None; на пропуске вернуть nil.
func (e *Engine) Notify(target string, args []value.Value) error {
	fr, key, idx, active, skip, _, frozen, derr := e.outboxPrecheck()
	if derr != nil {
		return derr
	}
	if !active {
		return e.caller.Notify(target, args)
	}
	if frozen != nil {
		return frozen // 029: замороженный вердикт сбоя → пере-брос без доставки
	}
	if skip {
		return nil
	}
	if err := e.caller.Notify(target, args); err != nil {
		if rerr := e.outboxRecordFailure(fr, key, idx, "уведомить", target, args, err); rerr != nil {
			return rerr
		}
		return err
	}
	return e.outboxRecord(fr, key, idx, "уведомить", target, args, value.None)
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
