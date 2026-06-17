# Contract: 4-я фаза tick + checkDeadlines + fireDeadlineBody (B4b)

Источник §AU-6.2. Группа: **B4b-бэкенд**.

## 4-я фаза `tick()` (daemon/tick.go:10) — аддитивна в хвост

ДО (3 фазы) / ПОСЛЕ (4 фазы):
```go
func (d *Daemon) tick() {
    d.mu.Lock()
    defer d.mu.Unlock()
    d.interp.ResetRunState()
    d.drainEvents()
    d.evalMetrics()
    d.checkSchedules()
    d.checkDeadlines()   // НОВАЯ 4-я фаза — В ХВОСТ, под тем же d.mu
}
```

**Инвариант 007b (INV-1):** порядок и идемпотентность первых трёх фаз НЕ меняются;
`checkDeadlines` под тем же `d.mu`; использует ту же машинерию `safeFire`. `ResetRunState`
уже сбросил кеши. **Верифицировать живым daemon-тестом.**

## `checkDeadlines` (новый daemon/checkdeadlines.go)

```text
now := d.clock.Now()
dts := фильтр d.interp.Triggers() по td.Spec.(*ast.DeadlineTrigger)
если dts пуст { return }                              // нет работы — БЕЗ листинга
tasks, err := d.st.ListPendingTasks("")               // ОДИН листинг до циклов (СУЩЕСТВУЮЩИЙ метод)
если err != nil { d.logf("checkDeadlines: листинг задач: %s", err); return }
for t := range tasks:
    если t.Escalated { continue }                     // durable-фильтр (D-AU-5) ← МУТПРОБА INV-2
    если !engine.Overdue(t, now) { continue }         // СУЩЕСТВУЮЩИЙ хелпер (format.go:35)
    inst, lerr := d.st.LoadInstance(t.InstanceID); если lerr != nil { continue }
    for td := range dts:
        spec := td.Spec.(*ast.DeadlineTrigger)
        если t.StepName != spec.Step.Name || inst.ProcessName != spec.Process.Name { continue }
        d.safeFire(func() error { return d.fireDeadlineBody(td.Body, inst.Variables) })
        t.Escalated = true; d.st.SaveTask(t)          // durable-персист (UPSERT)
        break                                          // одна эскалация на задачу
```

**Решения (закрыты §AU-6.2.2, не гадать):** (1) `ListPendingTasks` ОДИН раз до циклов; копии
безопасны. (2) Просрочка — `engine.Overdue(t, now)` (единый источник, без off-by-one на
`now==Deadline`). (3) Ошибка листинга → лог + выход из фазы (изоляция как первые три). (4)
Инжект — напрямую из `inst.Variables`, без round-trip `InstanceVariables`. `SaveTask` после
fire — at-least-once допустим до M3 (идемпотентно по цели).

## `fireDeadlineBody` (инжект ВСЕХ InstanceVariables, D-AU-6)

```go
func (d *Daemon) fireDeadlineBody(body *ast.Block, vars map[string]value.Value) error {
    env := d.interp.NewTriggerBodyEnv()      // NewEnvironment(global)+markBoundary (trigger_daemon.go:60)
    for k, v := range vars {
        env.Define(k, v)                     // ВСЕ переменные инстанса как локали барьерного env
    }
    return d.interp.EvalBlockInTrigger(env, body)
}
```

В отличие от `fireBody` (один `injection{name,val}`, `fire.go:22`), эскалация инжектит ВСЕ
переменные ПРЯМЫМ циклом `Define`, минуя struct `injection` (он НЕ расширяется). Read-only
барьер тела (TR-BODY-RO) наследуется от `NewTriggerBodyEnv`+`markBoundary`: тело читает
инжект, зовёт `уведомить`/`вызвать`/`запустить процесс`, но не перепривязывает глобали.

## Тесты (B4b-группа)

- Живой daemon-тест: метрика+расписание+эскалация в одном `tick()`; порядок фаз
  `ResetRunState→drainEvents→evalMetrics→checkSchedules→checkDeadlines`, все под одним `d.mu`.
  Первые три отрабатывают как до B4 (идемпотентность).
- Эскалация: инстанс+задача с дедлайном, Clock за срок → `fireDeadlineBody` исполнено,
  `[уведомление] руководитель: <факт>`, `Escalated=true`.
- Не-просрочена: Clock до срока → тишина.
- Нет эскалация-триггеров → ранний `return` (без листинга — проверить, что `ListPendingTasks`
  не вызван).
- Инжект: тело читает `факт` из `inst.Variables` → корректное значение в уведомлении.
- Инверсия INV-1: поменять порядок (`checkDeadlines` ПЕРЕД `checkSchedules`) → живой
  daemon-тест ловит сдвиг (если есть зависимость) / порядок-замок красный.
- Инверсия INV-2 (durable): снять `if t.Escalated { continue }` → durable-golden красный
  (см. `durable-restart.md`).
