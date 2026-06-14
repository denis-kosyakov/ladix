# Contract — Тик демона: 3 фазы, edge-детект, расписание, события, рестарт-скан (007b)

**Anchor**: EM-17.1 (3 фазы), EM-17.2 (edge/прайминг/заморозка), EM-17.3 (события), EM-17.4
(расписание), EM-17.5 (исполнение тела), EM-17.6 (recover), EM-17.7/EM-11 (конкурентность).
FR-002…013, FR-016…020, FR-024/025. Пакет `internal/daemon`.

## Тип `Daemon` (поля, Принцип V — без глобалов)

```go
type Daemon struct {
    st       store.Store        // Store: trigger_state, events, инстансы
    eng      *engine.Engine     // исполнение тела → Start/реактивация
    interp   *eval.Interpreter  // реестр триггеров, метрики, ResetRunState
    clock    engine.Clock       // часы планировщика (time.Time): когда тикать, LastFire, target
    interval time.Duration      // --interval (дефолт 1m)
    mu       sync.Mutex         // сериализация прогона движка (EM-11): тики не пересекаются
    out      io.Writer          // системные строки/логи демона (русские)
}
```
Создаётся явно в `serveMain` (инъекция Store/Engine/Interpreter/Clock). Часы — управляемые в тестах.

## `Run(ctx context.Context) error` — цикл с грациозной остановкой (FR-003)

```
тикер := time.NewTicker(d.interval); defer тикер.Stop()
рестарт-скан при старте (см. ниже, до первого тика)  // FR-019
for {
    select {
    case <-ctx.Done():        // SIGINT/SIGTERM (signal.NotifyContext в serveMain)
        return nil            // завершить текущий тик уже не идёт — select между тиками; чистый выход
    case <-тикер.C:
        d.tick()              // последовательно, под d.mu внутри
    }
}
```
- Тикер-горутина не утекает: `defer тикер.Stop()` + выход по `ctx.Done()` (SC-007).
- Полу-записанного состояния нет: `tick()` синхронен; отмена ловится в `select` МЕЖДУ тиками.

## `tick()` — строгий порядок трёх фаз (FR-002, EM-17.1)

```go
func (d *Daemon) tick() {
    d.mu.Lock(); defer d.mu.Unlock()   // EM-11: один прогон движка за раз
    d.interp.ResetRunState()           // СБРОС метрик-состояния (решение #2, FR-005/024) — ДО фаз
    d.drainEvents()                    // фаза 1 (FR-016/017)
    d.evalMetrics()                    // фаза 2 (FR-006…010)
    d.checkSchedules()                 // фаза 3 (FR-011…013)
}
```
Порядок `drainEvents → evalMetrics → checkSchedules` — **строгий** (FR-002, SC детерминизма). Внутри
фазы триггеры обрабатываются в **текстовом порядке объявления** (реестр `interp.Triggers()` уже в
порядке объявления, interpreter.go:32).

## Per-триггер изоляция сбоя (FR-004, EM-17.6)

Каждое исполнение тела триггера обёрнуто `recover`:
```go
func (d *Daemon) safeFire(fn func() error) {
    defer func() {
        if r := recover(); r != nil { d.logf("сбой триггера изолирован: %v", r) } // русский лог, без stack trace
    }()
    if err := fn(); err != nil { d.logf("ошибка триггера: %s", err.Error()) }
}
```
Паника/рантайм-ошибка одного триггера логируется и НЕ роняет тик/демон; остальные триггеры тика и
последующие тики работают (SC-007). Это отдельный per-триггер `recover` ВНУТРИ тика — он не отменяет
CLI-границу `guard()` в `serveMain` (Принцип III; guard ловит панику ВНЕ цикла тика).

## Фаза 1 — `drainEvents` (FR-016/017, EM-17.3)

```
events := ListUnprocessedEvents()              // FIFO по CreatedAt
для каждого e в events:
    matched := событие-триггеры с EventTrigger.Event.Name == e.Name
    если matched пусто:
        MarkEventProcessed(e.ID); logf("событие '%s' без триггеров", e.Name); продолжить
    rec := payloadJSON→value.Запись (маппинг источников §9; невалидный JSON — лог+skip, импл-факт)
    для каждого td в matched (порядок объявления):
        safeFire(исполнить тело td с инжекцией «событие»=rec)   // EM-17.5
    MarkEventProcessed(e.ID)                    // ПОСЛЕ тела (at-least-once, FR-017)
```
Краш между телом и `MarkEventProcessed` → переобработка тела на следующем старте/тике. Повтор
ВОЗМОЖЕН: неидемпотентная побочка («запустить процесс» → второй p-NNNNNN) может задвоиться —
at-least-once осознанный выбор v1 (доставка > дедупликация, FR-017), не «безвредность». Пустая
очередь → no-op.

## Фаза 2 — `evalMetrics` (FR-006…010, EM-17.2)

```
для каждого td-метрика-триггера (порядок объявления):
    ts, err := LoadTriggerState(td.TriggerID)
    cur, ok := вычислить булев результат метрики (значение метрики → порог → compareValues)
              // переиспользует i.evalMetricByName + compareValues (trigger_run.go:54–109)
    если НЕ ok (метрика пусто / сравнение не-Булево):           // заморозка FR-009
        ничего не персистить, тело НЕ исполнять; продолжить
    если errors.Is(err, ErrTriggerStateNotFound):               // ПРАЙМИНГ FR-007
        SaveTriggerState{Kind:"metric", LastBool:&cur}; тело НЕ исполнять; продолжить
    fired := (ts.LastBool != nil && *ts.LastBool == false && cur == true)  // edge ложь→истина FR-006
    SaveTriggerState{LastBool:&cur}                             // persist ДО тела (at-most-once FR-008)
    если fired:
        snapshot := снимок метрики (value.Value)
        safeFire(исполнить тело td с инжекцией «значение»=snapshot)   // EM-17.5
```
- Прайминг: 0 ложных срабатываний (SC-002), даже если `cur==true`.
- persist ДО тела: краш после persist → пропуск, не дубль; сбойный триггер не зацикливается (FR-008).
- Заморозка: persist пропущен, тело не исполнено (FR-009).
- Сброс между тиками (`ResetRunState`) гарантирует, что `cur` — свежий снимок (FR-005, SC-001).

## Фаза 3 — `checkSchedules` (FR-011…013, EM-17.4)

```
now := d.clock.Now()  // часы планировщика (time.Time)
для каждого td-расписание-триггера (порядок объявления):
    ts, err := LoadTriggerState(td.TriggerID)
    EverySchedule (Kind="schedule_every"):
        если ErrTriggerStateNotFound:                           // якорь FR-011
            SaveTriggerState{Kind:"schedule_every", LastFire:&now}; продолжить (без срабатывания)
        next := shiftEvery(*ts.LastFire, amount, unit)          // R-6: фикс vs календарный нед/мес
        если now >= next:
            SaveTriggerState{LastFire:&now}                     // факт срабатывания (дрейф не копим)
            safeFire(исполнить тело td)                          // у расписания «значение»/«событие» НЕ инжектируются
    AtSchedule (Kind="schedule_at"):
        today := now.date "YYYY-MM-DD"; target := now.date в ЧЧ:ММ
        если (ts==nil || *ts.LastFiredDate != today) И now >= target:    // раз в сутки FR-013
            SaveTriggerState{Kind:"schedule_at", LastFiredDate:&today}
            safeFire(исполнить тело td)
```
`shiftEvery` (daemon/calendar.go, R-6): сек/мин/час/дн → `last.Add(amount*unitDur)`; нед →
`AddDate(0,0,7*amount)`; мес → `AddDate(0,amount,0)` + зажим `lastDayOfMonth` (паттерн window.go:69).

## Исполнение тела триггера (EM-17.5, §TR-5) — `fire.go`

```
env := eval.NewEnvironment(interp.GlobalEnv()); env.markBoundary()   // read-only глобалы (TR-BODY-RO)
для метрики: env.Define("значение", snapshot)
для события: env.Define("событие", rec)
для расписания: (ничего не инжектируется)
interp.EvalBlockInTrigger(env, td.Body)   // штатный исполнитель блока; «запустить процесс» → Engine.Start
```
Несколько `запустить процесс` в одном теле — последовательно (FR-018). Инстанс `p-NNNNNN` доходит до
ожидания/терминала (fire-and-forget). Под `--db` персистится штатно. Read-only глобалов — env-барьер
007a (`markBoundary`, trigger_run.go:78–79) переиспользуется.

## Рестарт-скан при старте (FR-019/020, решение #6) — `restart.go`

```
для status в {"выполняется", "создан"}:
    для inst в ListInstancesByStatus(status):
        pd, ok := interp.Process(inst.ProcessName)
        если !ok ИЛИ inst.CurrentStep не найден в pd.Steps:     // ДРЕЙФ FR-020
            logf("рестарт-скан: инстанс '%s' — шаг '%s' не найден (дрейф), пропущен", inst.ID, inst.CurrentStep)
            продолжить                                          // НЕ переактивировать, демон стартует
        eng.ReactivateInstance(inst)                            // advance заново (at-least-once); ошибка → лог, не падать
"ожидает"-инстансы НЕ сканируются (корректны, FR-019)
```

## Конкурентность (EM-11/EM-17.7, FR-025)

- Прогон движка под `d.mu` (тики не пересекаются по инстансу).
- `emit` пишет в `events` из другого процесса ОС; демон читает — сериализация WAL+busy_timeout
  (sqlite.go:52–56), НЕ блокировки в коде Ladix.
- `trigger_state` читается/пишется только демоном.

## Тесты (управляемые часы, R-10)

Прямой вызов `d.tick()` с продвижением часов между вызовами:
- edge: тик1 ложь→прайм; тик2 истина→1 срабатывание; тик3 истина→0 (SC-001/002).
- заморозка: метрика пусто → persist пропущен, тело не исполнено (FR-009).
- `каждые 1мес` от 31 янв → срабатывание 28/29 фев (SC-004).
- `в "09:30"`: старт после 09:30 → срабатывание в день старта; повтор в тот же день → 0 (FR-013).
- FIFO: 2 события подряд → обработаны в порядке; событие без триггеров → лог+processed (SC-006).
- recover: тело паникует → изолировано, второй триггер исполнен, демон жив (SC-007).
- graceful: `Run(ctx)` + `cancel()` → выход без утечки горутин (счётчик до/после, SC-007).
- рестарт-скан: «выполняется»+валидный шаг → догон; дрейф → лог+пропуск; «ожидает» не тронут (SC-008).
