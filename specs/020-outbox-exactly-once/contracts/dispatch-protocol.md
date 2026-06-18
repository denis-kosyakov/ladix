# Contract: Dispatch-протокол дедупа в 3 effect-методах движка

Источник: §C-2b.2/.4/.5 (D-C-7, D-C-9). Точка: `engine/runtime.go:47-62` (`CallExternal`, `CallExternalResult`, `Notify`). ProcessRuntime сигнатуры НЕ меняются (INV-1).

## Гейт активности

Дедуп активен ⟺ `len(e.active) > 0` (есть кадр шага). Иначе → delegate напрямую в `e.caller` (прежнее поведение). **В каждом из 3 методов независимо** (`CallExternal` зовёт `e.caller.Call` напрямую, не делегирует `CallExternalResult`).

## effect_index (поле кадра)

```go
// advance (engine.go:249), начало каждой итерации шага, перед ExecStepBody:
frame.effectIndex = 0
// effect-метод при len(e.active)>0:
fr  := e.active[len(e.active)-1]
idx := fr.effectIndex
fr.effectIndex++
key := fmt.Sprintf("%s|%s|%d", fr.inst.ID, fr.inst.CurrentStep, idx)
```

## Протокол (deliver-then-record + pre-check)

```
1. key := (inst.ID, CurrentStep, effect_index++)
2. rec, err := e.st.LoadOutbox(key)
   - err==nil && rec.Delivered:
        НЕ звать e.caller; вернуть:
          CallExternalResult → rec.Result, nil
          CallExternal/Notify → nil
        СТОП.
   - ErrOutboxNotFound (или не delivered): продолжаем.
   - иная ошибка Store: вернуть её (шаг провалится; D-14).
3. v, derr := e.caller.Call|Notify(target, args)   // реальная доставка
4. derr != nil → вернуть derr; outbox НЕ помечаем delivered.
5. derr == nil → e.st.SaveOutbox(&OutboxRecord{
        DedupKey:key, InstanceID:inst.ID, StepName:CurrentStep, EffectIndex:idx,
        Kind:"вызвать"|"уведомить", Target:target, Args:args, Result:v(или None),
        Delivered:true, CreatedAt:now, DeliveredAt:&now})   // upsert
6. вернуть v (или nil для statement-формы).
```

| Метод | Kind | Result сохраняемый | Возврат на пропуске |
|---|---|---|---|
| `CallExternalResult` | "вызвать" | результат `v` | `rec.Result` |
| `CallExternal` | "вызвать" | `value.None` (отбрасывается) | `nil` |
| `Notify` | "уведомить" | `value.None` | `nil` |

## Граница at-least-once (осознанная, §C-9)

Окно между успешным шагом 3 (POST) и шагом 5 (SaveOutbox): краш → ключ не delivered → повтор на рестарте = at-least-once. Закрывается лишь идемпотентностью приёмника. Гейт §2 крашится ПОСЛЕ шага 5 → POST ровно 1.

## Контрактные тесты (замки)

- `TestOutboxLedgerSkipsDelivered` (Go-API): дважды `Engine.Notify` под одним кадром+ключом → `e.caller` вызван ОДИН раз; второй — пропуск по `LoadOutbox.Delivered`.
- `TestOutboxResultReplay`: `CallExternalResult` под дедупом возвращает сохранённый `Result` без повторного `Call`.
- `TestDedupOnlyInsideStepBody`: эффект при `len(e.active)==0` (тело триггера/эскалации/top-level) → delegate напрямую, outbox НЕ консультируется (нет записи).
- `TestTwoEffectsIndependentKeys`: два эффекта в одном теле шага → два разных ключа (idx 0 и 1), дедуплицируются независимо.
- **Инверсии (мутпробы):**
  - снять pre-check (`if rec.Delivered → skip`) → доставка дважды → `TestStepEffectExactlyOnceRestart` + `TestOutboxLedgerSkipsDelivered` краснят.
  - сломать `effect_index` (всегда 0 при ≥2 эффектах) → коллизия ключей → второй эффект «съеден» → `TestTwoEffectsIndependentKeys` краснит.
  - record-then-deliver (SaveOutbox до Call) → при ошибке доставки ключ ложно delivered → тест провала доставки краснит.
