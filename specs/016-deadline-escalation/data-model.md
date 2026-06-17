# Data Model: B4 эскалация дедлайна

Источник истины §AU-6.1.2 / §AU-2 / §AU-6.2. Ниже — AST-узел (B4a), персистентное поле +
кодек (B4b) и 4-я фаза tick.

## B4a — AST-узел `DeadlineTrigger`

`internal/ast/trigger.go`, рядом с `MetricTrigger`/`EventTrigger`/`ScheduleTrigger`:

```go
type DeadlineTrigger struct {   // четвёртый конкретный тип TriggerSpec
    specBase
    Process Ident
    Step    Ident
}
func NewDeadlineTrigger(pos Position, process, step Ident) *DeadlineTrigger
func (*DeadlineTrigger) triggerSpec() {}   // маркер интерфейса TriggerSpec
```

| Свойство | Значение |
|----------|----------|
| Pos() | токен `задача` (через `specBase`) |
| `Process` | `Ident` — имя процесса (после `в`, до `.`) |
| `Step` | `Ident` — имя шага (после `.`) |
| Маркер | `triggerSpec()` — четвёртый реализатор `TriggerSpec` (после Metric/Event/Schedule) |
| `TriggerDecl.Spec` | новый конкретный вариант |

Грамматика (контекстный разбор, D-AU-4):

```text
TriggerDecl   := "когда" TriggerSpec ":" Block
TriggerSpec   := MetricTrigger | EventTrigger | ScheduleTrigger | DeadlineTrigger
DeadlineTrigger := "задача"ᴵᴰᴱᴺᵀ "просрочена"ᴵᴰᴱᴺᵀ "в" Ident "." Ident
```

`задача`/`просрочена` — IDENT-лексемы (лексер не трогается); распознаёт парсер по позиции
после `когда`.

## B4b — поле `Task.Escalated` + колонка

`internal/store/types.go` — `Task` получает поле (после `CompletedAt`):

```go
type Task struct {
    ID          string
    InstanceID  string
    StepName    string
    Assignee    string
    Deadline    *time.Time
    Status      TaskStatus
    CreatedAt   time.Time
    CompletedAt *time.Time
    Escalated   bool          // НОВОЕ: durable, одноразово (D-AU-5)
}
```

| Свойство | Значение |
|----------|----------|
| Тип | `bool` (Go) / `escalated INTEGER NOT NULL DEFAULT 0` (SQLite) |
| Семантика | задача уже эскалирована — одноразовый durable-фильтр |
| Дефолт | `false` / `0` (новые задачи не эскалированы) |
| Персист | SQLite-колонка `escalated` (durable × рестарт) |
| Без `--db` | MemoryStore — эфемерно (durable НЕ держится; демо ОБЯЗАНО с `--db`, §AU-9) |

### 4 точки SQLite-кодека (§AU-2, ВСЕ обязательны)

| # | Точка | Файл:строка | Правка |
|---|-------|-------------|--------|
| 1 | DDL `tasks` | `sqlite.go:33` | +колонка `escalated INTEGER NOT NULL DEFAULT 0` |
| 2 | `SaveTask` INSERT-список | `sqlite.go:161` | +колонка `escalated` в список + значение `t.Escalated` |
| 3 | `ON CONFLICT(id) DO UPDATE SET` | `sqlite.go:165` | +`escalated = excluded.escalated` (или `?`) |
| 4 | SELECT-читатели → `buildTask`/`scanTask` | `sqlite.go:296/310` | +`escalated` в SELECT-список (`LoadTask :179`, `ListPendingTasks :186`) + параметр `escalated` в сигнатуру `buildTask`/`scanTask`; парс `int→bool` |

`MemoryStore.copyTask` — `cp := *t` несёт `Escalated` тривиально (bool-значение). `UserTasks`
(`engine/runtime.go`) делегирует `ListPendingTasks` → наследует `Escalated` автоматически.

**Пропуск ЛЮБОЙ точки → `Escalated` молча теряется на рестарте** (точка 1 — колонки нет вообще;
2/3 — не пишется при insert/upsert; 4 — пишется, но не читается → скан после рестарта видит
`false`). Замок: durable-тест перечитывает из той же `--db` и проверяет `Escalated==true`.

## B4b — 4-я фаза tick (поведенческая модель)

`tick()` (`daemon/tick.go:10`) — ДО (3 фазы) → ПОСЛЕ (4 фазы):

```go
func (d *Daemon) tick() {
    d.mu.Lock()
    defer d.mu.Unlock()
    d.interp.ResetRunState()   // фаза 0/1
    d.drainEvents()            // фаза 1
    d.evalMetrics()            // фаза 2
    d.checkSchedules()         // фаза 3
    d.checkDeadlines()         // фаза 4 — НОВАЯ (B4b), аддитивна в хвост, тот же d.mu
}
```

`checkDeadlines` (новый `daemon/checkdeadlines.go`):

```text
now := d.clock.Now()
dts := фильтр d.interp.Triggers() по td.Spec.(*ast.DeadlineTrigger)
если dts пуст { return }                                    // нет работы — без листинга
tasks, err := d.st.ListPendingTasks("")                     // ОДИН листинг до циклов (СУЩЕСТВУЮЩИЙ метод)
если err != nil { d.logf("checkDeadlines: листинг задач: %s", err); return }
for t := range tasks:
    если t.Escalated { continue }                           // durable-фильтр (D-AU-5) ← МУТПРОБА
    если !engine.Overdue(t, now) { continue }               // СУЩЕСТВУЮЩИЙ хелпер (format.go:35)
    inst, lerr := d.st.LoadInstance(t.InstanceID); если lerr != nil { continue }
    for td := range dts:
        spec := td.Spec.(*ast.DeadlineTrigger)
        если t.StepName != spec.Step.Name || inst.ProcessName != spec.Process.Name { continue }
        d.safeFire(func() error { return d.fireDeadlineBody(td.Body, inst.Variables) })
        t.Escalated = true; d.st.SaveTask(t)                // durable-персист (UPSERT)
        break                                                // одна эскалация на задачу
```

`fireDeadlineBody` (инжект ВСЕХ `InstanceVariables`, D-AU-6):

```go
func (d *Daemon) fireDeadlineBody(body *ast.Block, vars map[string]value.Value) error {
    env := d.interp.NewTriggerBodyEnv()      // NewEnvironment(global) + markBoundary
    for k, v := range vars {
        env.Define(k, v)                     // факт, текущая_выручка, … как локали барьерного env
    }
    return d.interp.EvalBlockInTrigger(env, body)
}
```

## Жизненный цикл `Escalated`

```text
SaveTask (новая)         → Escalated=false (DEFAULT 0)
checkDeadlines (просрочка, шаг/процесс совпали, !Escalated)
                         → fireDeadlineBody (тело исполнено РАЗ)
                         → Escalated=true, SaveTask (UPSERT)
рестарт демона (та же --db)
                         → ListPendingTasks читает Escalated=true (точка 4 кодека)
                         → continue (durable-фильтр) → повтора НЕТ
complete (до/после эскалации)
                         → задача → "завершена", вне ListPendingTasks
```
