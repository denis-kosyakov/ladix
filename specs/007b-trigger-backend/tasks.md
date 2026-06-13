---
description: "Task list — 007b-trigger-backend"
---

# Tasks: 007b — Бэкенд триггеров: демон `serve`, события, edge-детект

**Input**: Design documents from `specs/007b-trigger-backend/`
**Dev root**: `/Users/denis/dev/ladix` (модуль в `src/`); пути ниже — относительно этого корня.
**Prerequisites**: plan.md, spec.md, research.md (решения #2/#6 + R-3…R-10), data-model.md,
contracts/ (5: store-methods/serve-command/emit-command/tick-contract/diagnostics), quickstart.md,
docs/trigger-model.md §TR-11 / docs/execution-model.md EM-17/EM-11 / docs/engine-model.md (обещание Store).

**Tests**: ВКЛЮЧЕНЫ. Контракты (`contracts/*.md`) и `quickstart.md` определяют приёмку через
table-driven юнит (Store-паритет Memory+SQLite, edge-детект, календарный сдвиг, валидация `"ЧЧ:ММ"`),
golden байт-точный stdout (`serve`/`emit` на детерминированных фикстурах) и exact-match новой
диагностики — это интринсик фичи и устоявшаяся конвенция фич 001–007a. **Часы инъектируются**:
никогда `time.Now()` напрямую в коде демона; в тестах — управляемые часы (R-10).

**Organization**: задачи сгруппированы по user story. Из-за слоистости (Store → eval/engine → daemon →
CLI) стории **инкрементальны**: каждая независимо *тестируема*, но опирается на код предыдущих фаз
(зависимости явно указаны). Соответствие фаз плана: Phase A→Foundational (Store), Phase B/C→US1-блокеры
(eval/engine), Phase D→US1+US2+US3+US4 (тик/edge/расписание/события), Phase E→CLI (serve/emit/семош),
Phase F→Polish/регресс.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: можно выполнять параллельно (разные файлы, нет незавершённых зависимостей)
- **[Story]**: US1 / US2 / US3 / US4 / US5 (только для фаз стори)
- Точные пути файлов — в каждой задаче

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: базовая готовность ветки и сборки. **1 новый пакет** (`internal/daemon`); остальное
расширяет существующие.

- [ ] T001 Подтвердить ветку `007b-trigger-backend` и зелёный бейзлайн: из `/Users/denis/dev/ladix` выполнить `cd src && go build ./... && go vet ./... && go test ./...`; зафиксировать, что рабочее дерево чистое, собирается и весь регресс 001–007a зелёный ДО правок (бейзлайн для SC-010)

---

## Phase 2: Foundational (Blocking Prerequisites) — Phase A плана: Store +7 методов, 2 таблицы, паритет

**Purpose**: durable-носитель `TriggerState`/`Event` и листинг инстансов. ⚠️ Блокирует ВСЕ стории —
демон не может тикать без `LoadTriggerState`/`SaveTriggerState`, события без `EnqueueEvent`/
`ListUnprocessedEvents`/`MarkEventProcessed`/`NextEventID`, рестарт-скан без `ListInstancesByStatus`.
8 методов и 3 таблицы (instances/tasks/counters) 006 — НЕ трогать (FR-021/022, SC-009).

- [ ] T002 [P] Добавить структуры `TriggerState` (поля `TriggerID/Kind/LastBool *bool/LastFire *time.Time/LastFiredDate *string`) и `Event` (поля `ID/Name/PayloadJSON/CreatedAt/Processed`) в `src/internal/store/types.go` (аддитивно; форма зеркалит `ProcessInstance`/`Task`, data-model.md §1/§2). 3 существующие структуры не меняются (FR-021)
- [ ] T003 [P] Объявить сентинел `ErrTriggerStateNotFound = stderrors.New("состояние триггера не найдено")` в `src/internal/store/store.go` (явный тип, Принцип III; зеркало `ErrInstanceNotFound`/`ErrTaskNotFound`, развёртка `errors.Is`, data-model.md §сентинел)
- [ ] T004 Расширить интерфейс `Store` в `src/internal/store/store.go` на 7 методов: `LoadTriggerState(triggerID string) (*TriggerState, error)`, `SaveTriggerState(ts *TriggerState) error`, `NextEventID() (string, error)`, `EnqueueEvent(e *Event) error`, `ListUnprocessedEvents() ([]*Event, error)`, `MarkEventProcessed(id string) error`, `ListInstancesByStatus(status string) ([]*ProcessInstance, error)` (точные сигнатуры — contracts/store-methods.md, data-model.md §контракт). Уточнить комментарий «+6» → «6 триггерных + ListInstancesByStatus (рестарт-скан, deviation FR-022)». Зависит от T002, T003
- [ ] T005 Реализовать 7 методов в `src/internal/store/sqlite.go`: DDL `trigger_state`/`events` (+`CREATE INDEX IF NOT EXISTS idx_events_pending ON events(processed, created_at)`) добавить к константе `ddl` (`IF NOT EXISTS`, без правки instances/tasks/counters); `INSERT OR IGNORE INTO counters VALUES ('event', 0)`; `NextEventID` → `e-NNNNNN` (зеркало `NextInstanceID`, счётчик `event`); `LoadTriggerState` SELECT по PK → не найдено = `ErrTriggerStateNotFound`; `SaveTriggerState` upsert (`INSERT … ON CONFLICT(trigger_id) DO UPDATE`); `EnqueueEvent` INSERT; `ListUnprocessedEvents` `WHERE processed=0 ORDER BY created_at, id` (FIFO, стабильность по rowid/id); `MarkEventProcessed` UPDATE идемпотентно; `ListInstancesByStatus` `WHERE status=? ORDER BY id`. Времена RFC3339 (как 006). DDL — точно data-model.md §DDL. Зависит от T004
- [ ] T006 Реализовать 7 методов в `src/internal/store/memory.go` (паритет): добавить поля `triggerState map[string]*TriggerState`, `events []*Event`, `eventSeq int64` в `MemoryStore`; все 7 под `mu`; копирование при Save/Load (без алиасинга, зеркало `copyInstance`/`copyTask`); `ListUnprocessedEvents` фильтрует `!Processed`, отдаёт копии в порядке среза (FIFO); `ListInstancesByStatus` сортирует по ID; `LoadTriggerState` промах → `ErrTriggerStateNotFound`. Семантика идентична SQLite (data-model.md §MemoryStore). Зависит от T004
- [ ] T007 Контрактные table-driven тесты паритета на ВСЕ 7 методов в `src/internal/store/trigger_store_test.go` (новый), прогон на ОБОИХ бэкендах (Memory + SQLite через общую тест-функцию, как 006): load-miss → `ErrTriggerStateNotFound` (`errors.Is`); save/load round-trip по каждому виду (`metric`/`schedule_every`/`schedule_at`, корректная трёхзначность `*bool`/`*time.Time`/`*string`); upsert перезаписывает; `NextEventID` монотонен (`e-000001`, `e-000002`); FIFO `ListUnprocessedEvents` по `CreatedAt`; `MarkEventProcessed` идемпотентно (повтор — no-op без ошибки); `ListInstancesByStatus` фильтрует по статусу и сортирует по ID; пустая очередь/пустой статус → пустой срез, не ошибка. Зависит от T005, T006
- [ ] T008 [P] Регресс-замок Store: тест/проверка в `src/internal/store/store_test.go` (или существующем), что 8 методов и 3 таблицы (instances/tasks/counters) 006 не изменены по сигнатуре/поведению — все существующие store-тесты 006 зелёные (FR-021, SC-009). Зависит от T005, T006

**Checkpoint**: Store расширен аддитивно с паритетом Memory+SQLite — демон может персистить состояние.

---

## Phase 3: User Story 1 — Edge-детект метрики ложь→истина ровно один раз (Priority: P1) 🎯 MVP

**Goal**: демон на каждом тике переоценивает метрика-триггеры со сбросом состояния интерпретатора,
сравнивает с durable-базой `LastBool`, срабатывает РОВНО на переходе ложь→истина; первая оценка =
прайминг без срабатывания; persist ДО тела (at-most-once); заморозка при невычислимой метрике.

**Independent Test** (R-10, tick-contract.md §тесты): на фикстуре с метрика-триггером и управляемым
источником — тик1 (ложь) → база записана, тело не исполнено; тик2 (истина, источник изменён) → ровно
одно срабатывание, процесс создан; тик3 (истина) → НЕТ повторного. Рестарт демона → прайминг не
повторяется. Мутация источника между тиками доказывает, что `ResetRunState` активен.

### Phase B плана — eval: сброс состояния + доступ + двойные часы (спинной блокер edge-детекта)

- [ ] T009 [US1] Реализовать `func (i *Interpreter) ResetRunState()` в `src/internal/eval/interpreter.go` (решение #2): `i.today = nil` (следующий `now()` снимет свежую дату через `i.clock`); `i.recordCache = make(map[string][]value.Запись)` (отбросить кеш-на-запуск §9.6); переустановка даты вычисления метрик от часов планировщика (двойные часы FR-024). Без сброса edge-детект молча мёртв (research.md решение #2). Метод над полями интерпретатора (Принцип V). Зависит от T001
- [ ] T010 [P] [US1] Добавить аксессор `func (i *Interpreter) Triggers() []*ast.TriggerDecl` (возвращает реестр в порядке объявления, interpreter.go) и, если требуется для управляемых часов в тестах, `SetClock(eval.Clock)`/доступ к eval-Clock в `src/internal/eval/interpreter.go` (R-4 двойные часы; реестр `i.triggers` собран 007a). Зависит от T001
- [ ] T011 [US1] Юнит-тест `ResetRunState` в `src/internal/eval/interpreter_reset_test.go` (новый, мутационная проверка): после `ResetRunState` метрика пересчитывается со свежим `сегодня()` и пустым `recordCache` — изменили источник-фикстуру между «тиками» → новый снимок (доказывает, что сброс активен, US1 №4, SC-001/SC-004); БЕЗ сброса вернулся бы старый снимок. Зависит от T009, T010
- [ ] T012 [P] [US1] Регресс-замок eval: подтвердить, что `run`/fire-if-true 007a (`trigger_run.go`, `RunTriggers`) НЕ меняется и все 007a-тесты eval зелёные — `ResetRunState`/`Triggers()` аддитивны, демон НЕ зовёт `RunTriggers` (FR-001, plan.md trigger_run.go «без изменений по run»). Зависит от T009, T010

### Phase D плана (US1-срез) — daemon: тип, тик, evalMetrics edge-детект

- [ ] T013 [US1] Создать тип `Daemon` в `src/internal/daemon/daemon.go` (новый пакет, импортирует engine/store/eval/ast/errors): поля `st store.Store`, `eng *engine.Engine`, `interp *eval.Interpreter`, `clock engine.Clock`, `interval time.Duration`, `mu sync.Mutex`, `out io.Writer` (tick-contract.md §тип; Принцип V — без глобалов); конструктор `New(st, eng, interp, clock, interval, out) *Daemon`; хелпер `logf` (русские системные строки). Зависит от T004 (Store-интерфейс), T010
- [ ] T014 [US1] Реализовать `tick()` (строгий порядок 3 фаз) и `safeFire` в `src/internal/daemon/tick.go` (новый): `tick()` берёт `d.mu.Lock()` (EM-11, тики не пересекаются), зовёт `d.interp.ResetRunState()` ДО фаз, затем строго `drainEvents() → evalMetrics() → checkSchedules()` (FR-002, tick-contract.md §tick); `safeFire(fn func() error)` — per-триггер `recover` + лог ошибки/паники (FR-004, EM-17.6; НЕ роняет тик/демон, без stack trace). Зависит от T013, T009
- [ ] T015 [US1] Реализовать `evalMetrics()` в `src/internal/daemon/metrics.go` (новый): обойти метрика-триггеры `interp.Triggers()` в порядке объявления; вычислить булев результат метрики (переиспользовать путь `i.evalMetricByName`+`compareValues` из 007a trigger_run.go); если невычислимо (пусто/не-Булево) → заморозка: ничего не персистить, тело не исполнять (FR-009); `LoadTriggerState` промах (`errors.Is ErrTriggerStateNotFound`) → ПРАЙМИНГ: `SaveTriggerState{Kind:"metric", LastBool:&cur}`, тело НЕ исполнять (FR-007, 0 ложных срабатываний); иначе `fired := *ts.LastBool==false && cur==true` (edge FR-006), `SaveTriggerState{LastBool:&cur}` persist ДО тела (at-most-once FR-008), если `fired` → `safeFire(fire метрики со снимком)`. Точный алгоритм — tick-contract.md §фаза2. Зависит от T014, T016
- [ ] T016 [US1] Реализовать общий исполнитель тела `fire.go` в `src/internal/daemon/fire.go` (новый): `env := eval.NewEnvironment(interp.GlobalEnv())` + `markBoundary` (read-only глобалы, env-барьер 007a §TR-5/TR-BODY-RO); для метрики `env.Define("значение", snapshot)`, для события `env.Define("событие", rec)`, для расписания ничего; исполнить тело `interp.EvalBlockInTrigger(env, td.Body)` штатным путём движка 006 (`запустить процесс` → `Engine.Start`, инстанс `p-NNNNNN`, fire-and-forget; несколько — последовательно, FR-018). Зависит от T013, T010
- [ ] T017 [US1] Вывести `Kind` и `TriggerID` из AST в `src/internal/daemon/` (хелпер в daemon.go или metrics.go): `TriggerID = "trg-<N>"` (N — 0-based индекс по ВСЕМ `TriggerDecl`, EM-17.2.1/FR-023); `Kind` из типа `td.Spec` (`*ast.MetricTrigger`→"metric", `*ast.EverySchedule`→"schedule_every", `*ast.AtSchedule`→"schedule_at", `*ast.EventTrigger`→нет строки в trigger_state, R-9/FR-023). Зависит от T013
- [ ] T018 [US1] Юнит-тесты edge-детекта на управляемых часах в `src/internal/daemon/metrics_test.go` (новый): прямой вызов `d.tick()` с продвижением источника/часов — тик1 ложь→прайминг (тело не исполнено, база записана, SC-002); тик2 истина→РОВНО одно срабатывание (процесс создан, persist ДО тела, SC-001/SC-003); тик3 истина→0 срабатываний (нет перехода, FR-006); заморозка: метрика пусто → persist пропущен, тело не исполнено (FR-009); рестарт (новый `Daemon` на той же БД) → прайминг НЕ повторяется (FR-010, SC-002). Memory+SQLite паритет. Зависит от T015, T016, T017

**Checkpoint**: US1 функциональна — edge-детект ловит ложь→истина ровно раз, прайминг без ложных,
заморозка, durable-рестарт; ядро фичи поставляемо.

---

## Phase 4: User Story 2 — Грациозная остановка без утечки горутин + изоляция сбоя триггера (Priority: P1)

**Goal**: `Run(ctx)` — цикл `time.Ticker` + `ctx.Done()`, чистый выход без зависших горутин; фазы строго
`drainEvents → evalMetrics → checkSchedules`, внутри фазы — порядок объявления; per-триггер `recover`
изолирует панику/рантайм-ошибку (тик и демон живы); персист метрики ДО тела → сбойный триггер не
зацикливается.

**Independent Test** (tick-contract.md §тесты, SC-007): `Run(ctx)` + `cancel()` → возврат без утечки
тикер-горутины (счётчик горутин до/после); фикстура с паникующим телом + корректным вторым триггером →
сбой первого изолирован и залогирован, второй исполнен, демон тикает дальше.

- [ ] T019 [US2] Реализовать `func (d *Daemon) Run(ctx context.Context) error` в `src/internal/daemon/daemon.go`: `ticker := time.NewTicker(d.interval); defer ticker.Stop()`; `for { select { case <-ctx.Done(): return nil; case <-ticker.C: d.tick() } }`; отмена ловится в `select` МЕЖДУ тиками (полу-записанного состояния нет, `tick()` синхронен под `d.mu`); тикер-горутина не утекает (`defer Stop()` + выход по `ctx.Done()`, FR-003, SC-007, tick-contract.md §Run). Зависит от T014
- [ ] T020 [US2] Юнит-тест graceful-stop в `src/internal/daemon/daemon_test.go` (новый): запустить `Run(ctx)` в горутине, `cancel()` → `Run` возвращает nil без утечки горутин (счётчик `runtime.NumGoroutine` до/после, допуск на стабилизацию); состояние не повреждено (SC-007). Зависит от T019
- [ ] T021 [US2] Юнит-тест изоляции сбоя в `src/internal/daemon/tick_test.go` (новый): фикстура с триггером, чьё тело паникует, и вторым корректным; `d.tick()` → паника первого изолирована `safeFire`+залогирована, второй триггер исполнен, демон жив и тикает дальше (FR-004, SC-007); строгий порядок фаз `drainEvents→evalMetrics→checkSchedules` детерминирован (Acceptance US2 №3); сбойный метрика-триггер (база сдвинута ДО тела) не зацикливается на повторном ложь→истина (US2 №4). Зависит от T014, T015

**Checkpoint**: US1 + US2 — операционный фундамент демона (чистый lifecycle + изоляция сбоев).

---

## Phase 5: User Story 3 — Расписание: `каждые` (фикс/календарный) и `в "ЧЧ:ММ"` раз в сутки (Priority: P1)

**Goal**: `checkSchedules()` исполняет `каждые` (фикс-множитель сек/мин/час/дн; календарный сдвиг
нед/мес с зажимом конца месяца) и `в "ЧЧ:ММ"` (раз в сутки: `LastFiredDate != today && now >= target`);
якорь `каждые` при первой регистрации = старт без срабатывания, дрейф не копится.

**Independent Test** (tick-contract.md §тесты, SC-004/SC-005): управляемые часы — `каждые 1дн` ровно
раз в сутки от якоря; `каждые 1мес` от 31янв → 28/29фев (зажим); `в "09:30"` старт после 09:30 →
срабатывание в день старта, повтор в тот же день → 0; файл `в "25:99"`/`в "9:05"` → SE-TIME-FORMAT exit 1.

- [ ] T022 [P] [US3] Реализовать хелпер `shiftEvery(last time.Time, amount int64, unit string) time.Time` в `src/internal/daemon/calendar.go` (новый, R-6): сек/мин/час/дн → `last.Add(amount*unitDur)` (фикс-множитель); нед → `last.AddDate(0,0,7*amount)`; мес → `last.AddDate(0,amount,0)` с зажимом конца месяца через паттерн `lastDayOfMonth` (зеркало `eval/window.go:69`; 31янв+1мес→28/29фев). НЕ переиспользует несуществующий `Дата±Длит`, только паттерн зажима (FR-012, SC-004). Зависит от T013
- [ ] T023 [P] [US3] Юнит-тесты `shiftEvery` (table-driven, чистая функция) в `src/internal/daemon/calendar_test.go` (новый): фикс-множитель сек/мин/час/дн; нед = `AddDate(0,0,7)`; мес зажим конца месяца (31янв+1мес→28фев; в високосный год→29фев; 31мар+1мес→30апр); `amount>1`. SC-004. Зависит от T022
- [ ] T024 [US3] Реализовать `checkSchedules()` в `src/internal/daemon/schedule.go` (новый): `now := d.clock.Now()`; обойти расписание-триггеры в порядке объявления; `EverySchedule` (Kind="schedule_every"): промах → якорь `SaveTriggerState{LastFire:&now}` без срабатывания (FR-011); иначе `next := shiftEvery(*ts.LastFire, amount, unit)`, если `now >= next` → `SaveTriggerState{LastFire:&now}` (факт, дрейф не копится) + `safeFire(fire расписания, без инжекции)`; `AtSchedule` (Kind="schedule_at"): `today := now.date`, `target := now.date в ЧЧ:ММ`, если `(ts==nil || *ts.LastFiredDate != today) && now >= target` → `SaveTriggerState{LastFiredDate:&today}` + `safeFire` (раз в сутки FR-013). Точный алгоритм — tick-contract.md §фаза3. Зависит от T014, T016, T022, T017
- [ ] T025 [US3] Юнит-тесты расписания на управляемых часах в `src/internal/daemon/schedule_test.go` (новый): `каждые 1дн` — якорь при первой регистрации (без срабатывания), затем срабатывание при `now>=LastFire+1дн`, `LastFire` сдвигается на факт (дрейф не копится, FR-011); `каждые 1мес` от 31янв → срабатывание 28/29фев (зажим, SC-004); `в "09:30"` — старт после 09:30 → срабатывание в день старта (FR-013), повтор в тот же день → 0, следующие сутки → снова (SC-005); расписание-тело без инжекции `значение`/`событие`. Memory+SQLite паритет. Зависит от T024

**Checkpoint**: US1 + US2 + US3 — расписание исполняется по календарю (валидация формата — фаза E).

---

## Phase 6: User Story 4 — Эмитировать события и доставлять at-least-once FIFO (Priority: P1)

**Goal**: `drainEvents()` разбирает очередь FIFO, матчит событие-триггеры по имени, парсит `payload`
JSON→`Запись`, инжектирует `событие`, исполняет тело, помечает processed ПОСЛЕ тела (at-least-once);
событие без обработчиков → processed + лог «без триггеров»; команда `emit` пишет событие и выходит.

**Independent Test** (tick-contract.md §тесты, SC-006): `emit заявка_создана '{"клиент":"ООО"}'` →
exit 0, строка в `events`; демон с `когда событие заявка_создана:` → тик привязывает `событие.клиент`,
исполняет тело, помечает processed; два события подряд → FIFO; событие без триггеров → лог+processed;
краш между телом и пометкой → переобработка (at-least-once).

- [ ] T026 [US4] Реализовать `drainEvents()` в `src/internal/daemon/events.go` (новый): `events := ListUnprocessedEvents()` (FIFO по CreatedAt); для каждого `e` — найти событие-триггеры с `EventTrigger.Event.Name == e.Name`; если пусто → `MarkEventProcessed(e.ID)` + `logf("событие '%s' без триггеров", e.Name)` (FR-017); иначе `rec := payloadJSON→value.Запись` (маппинг источников §9; невалидный JSON → лог+skip, импл-факт), для каждого matched (порядок объявления) `safeFire(fire события с инжекцией событие=rec)`, затем `MarkEventProcessed(e.ID)` ПОСЛЕ тела (at-least-once FR-016/017). Пустая очередь → no-op. Точно — tick-contract.md §фаза1. Зависит от T014, T016
- [ ] T027 [US4] Юнит-тесты `drainEvents` на управляемых часах в `src/internal/daemon/events_test.go` (новый): `когда событие заявка_создана:` + событие в очереди → тело исполнено, `событие.клиент="ООО"` (толерантный доступ), processed ПОСЛЕ тела; два события подряд → FIFO-порядок (SC-006); событие без триггеров → processed + лог «без триггеров»; краш между телом и пометкой (имитация: пометка не вызвана) → событие переобрабатывается на следующем `drainEvents` (at-least-once, FR-017); пустая очередь → no-op. Memory+SQLite паритет. Зависит от T026

**Checkpoint**: US1–US4 — P1-набор полон (edge-детект + lifecycle + расписание + события доставляются).

---

## Phase 7: User Story 5 — Рестарт-скан залипших инстансов (Priority: P2)

**Goal**: при старте `serve` рестарт-скан листает инстансы в «выполняется»/«создан», сверяет
`CurrentStep` с перезагруженным `ProcessDecl`, реактивирует (`advance`, at-least-once); дрейф (шаг не
найден) → лог + инстанс залипает, демон стартует; «ожидает» не трогается.

**Independent Test** (tick-contract.md §рестарт, SC-008): БД с инстансом «выполняется» + валидный
`CurrentStep` → подъём → инстанс догнан до ожидания/терминала; тот же с отсутствующим `CurrentStep`
(дрейф) → лог расхождения, инстанс залип, демон стартует штатно; «ожидает» не тронут.

### Phase C плана — engine: реактивация инстанса

- [ ] T028 [US5] Реализовать экспортный `func (e *Engine) ReactivateInstance(inst *store.ProcessInstance) error` в `src/internal/engine/engine.go` (решение #6): для залипшего инстанса найти `inst.CurrentStep` в `ProcessDecl`, прогнать `advance` (at-least-once — повтор шага безвреден, D-11/EM-12); `CurrentStep` не найден → вернуть распознаваемую ошибку дрейфа (демон логирует, инстанс залипает); инкапсуляция lifecycle в engine. Зависит от T001
- [ ] T029 [P] [US5] Юнит-тесты `ReactivateInstance` в `src/internal/engine/reactivate_test.go` (новый): инстанс «выполняется» с валидным шагом → догнан до ожидания/терминала (advance); дрейф (`CurrentStep` отсутствует) → ошибка дрейфа без паники; метод не трогает инстанс «ожидает» (вызывается только по статусам залипших). FR-019/020, SC-008. Зависит от T028

### Phase D плана (US5-срез) — daemon: рестарт-скан

- [ ] T030 [US5] Реализовать рестарт-скан `func (d *Daemon) RunRestartScan()` в `src/internal/daemon/restart.go` (новый): для `status` в {`"выполняется"`,`"создан"`} → `ListInstancesByStatus(status)`; для каждого `inst` — `pd, ok := interp.Process(inst.ProcessName)`; если `!ok || inst.CurrentStep не найден в pd.Steps` → `logf` расхождения + пропустить (НЕ реактивировать, дрейф FR-020, демон стартует); иначе `eng.ReactivateInstance(inst)` (ошибка → лог, не падать); `"ожидает"` НЕ сканируется (FR-019). Точно — tick-contract.md §рестарт-скан. Зависит от T028, T013
- [ ] T031 [US5] Юнит-тесты рестарт-скана на детерминированной БД в `src/internal/daemon/restart_test.go` (новый): инстанс «выполняется» + валидный `CurrentStep` → реактивирован (advance, догнан); инстанс с дрейфом (`CurrentStep` отсутствует/`ProcessDecl` не найден) → лог + залипает, `RunRestartScan` не паникует и не прерывает старт; «ожидает» НЕ трогается; несколько инстансов — детерминированный порядок (по ID). Memory+SQLite паритет. FR-019/020, SC-008. Зависит от T030

**Checkpoint**: US1–US5 — рестарт-скан восстанавливает залипшие инстансы поверх живого исполнения.

---

## Phase 8: Phase E плана — CLI: serve + emit + новая семош формата + golden

**Purpose**: связать демон с CLI; новая семош `"ЧЧ:ММ"`; байт-точная приёмка serve/emit на
детерминированных фикстурах. Эта фаза охватывает FR-001/014/015 (CLI-поверхность всех сторий).

### Новая семош формата `"ЧЧ:ММ"` (FR-014, R-8, diagnostics.md)

- [ ] T032 [US3] Добавить семош валидации формата `"ЧЧ:ММ"` (SE-TIME-FORMAT) в `src/internal/eval/analyze.go`, ветка `*ast.AtSchedule` в `checkTrigger` (точка, где 007a отложил проверку, analyze.go:223): посимвольно (руны, БЕЗ regex — Принцип II) — ровно 5 рун, руна[2]==`:`, руны [0][1][3][4] — цифры, часы 00–23, минуты 00–59 (обязательные ведущие нули); нарушение → `СемантическаяОшибка` (semErr) с позицией токена `AtSchedule.At` (Принцип IV, двухстрочный канон §13). Точный русский текст фиксируется дословно при реализации (импл-факт, рекомендация — diagnostics.md). Реестр 007a НЕ меняется (FR-026). Зависит от T001
- [ ] T033 [US3] Exact-match golden-тест SE-TIME-FORMAT в `src/internal/eval/analyze_time_test.go` (новый): валидно `"09:05"`/`"00:00"`/`"14:30"`/`"23:59"` → чисто; невалидно `"25:99"`/`"9:05"`/`"09:5"`/`"9:5"`/`"24:00"`/`"12:60"`/`"ab:cd"`/`"12-30"`/`"012:30"`/`""` → SE-TIME-FORMAT (двухстрочный канон + позиция), exit 1 (таблица — diagnostics.md). Зависит от T032

### Команды serve / emit + диспетчер

- [ ] T034 [US4] Реализовать `emitMain(args, stdout, stderr) int` в `src/cmd/ladix/emit.go` (новый, emit-command.md): разбор `<событие>` (обязателен; нет имени → stderr usage, exit 2), `[json]` (опционален), `--db` (дефолт `defaultDBPath`); открыть Store (сбой → exit 2); `guard()` (CLI-барьер Принцип III) → `id := NextEventID()`, `EnqueueEvent{ID:id, Name, PayloadJSON:json, CreatedAt:clock.Now(), Processed:false}`, exit 0; НЕ запускает демон (FR-015). Зависит от T004
- [ ] T035 [US1] Реализовать `serveMain(args, stdout, stderr) int` в `src/cmd/ladix/serve.go` (новый, serve-command.md): разбор `<файл>`+`--db`/`--interval`(дефолт `1m`, `time.ParseDuration`)/`--max-depth` (невалидное/нет файла → exit 2); прочитать+лексировать+распарсить (ошибка → двухстрочный Error(), exit 1); `guard()` → собрать `interp`+`eng`, `interp.Analyze(prog)` (вкл. SE-TIME-FORMAT → exit 1, демон не стартует), `interp.Run(prog)` (связать глобалы), `d := daemon.New(...)`, `ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`, `d.RunRestartScan()` ДО тиков (FR-019), `d.Run(ctx)`; выход 0 по сигналу (FR-001/003). Зависит от T019, T030, T032, T013
- [ ] T036 [US1] Добавить диспетчер в `src/cmd/ladix/main.go`: `case "serve": return serveMain(args[1:], stdout, stderr)`, `case "emit": return emitMain(args[1:], stdout, stderr)`; дополнить `usage` формами `serve`/`emit`. Поведение `run`/`metric`/`complete`/`tasks` НЕ меняется (FR-001/026). Зависит от T034, T035

### Детерминированные фикстуры + golden serve/emit

- [ ] T037 [P] [US1] Создать детерминированные фикстуры в `src/cmd/ladix/testdata/` (новый каталог; НЕ переиспускать `выручка.ladix` — оконная по дате): `metric_edge.ladix` + `*.json` (метрика-триггер + источник БЕЗ `период:`/`по_дате:` для детерминизма), `schedule.ladix` (`каждые`/`в "ЧЧ:ММ"`), `event.ladix` (событие-триггер). Источники без окна периода (R-10). Зависит от T001
- [ ] T038 [US1] Golden-тест `serve` на детерминированных фикстурах (управляемые часы, прямой `d.tick()`) в `src/cmd/ladix/serve_golden_test.go` (новый): edge-детект (US1: тик1/2/3), расписание (US3: зажим+раз в сутки), события через `emit`→`drainEvents` (US4: `событие.клиент`+FIFO+processed), Memory+SQLite паритет; рестарт-скан подбирает залипший, дрейф пропускается с логом (US5). Зависит от T036, T037, T031
- [ ] T039 [P] [US4] Golden/Store-тест `emit` в `src/cmd/ladix/emit_golden_test.go` (новый): `emit заявка_создана '{"клиент":"ООО"}'` → exit 0, одна необработанная строка в `events` (через Store); два `emit` подряд → FIFO по CreatedAt (`ListUnprocessedEvents`); `emit` без имени → exit 2 (emit-command.md §тесты, SC-006). Зависит от T034

**Checkpoint**: CLI-поверхность готова — `serve`/`emit` работают на детерминированных фикстурах с паритетом.

---

## Phase 9: Polish & Cross-Cutting Concerns — Phase F плана: синки и регресс

- [ ] T040 ⚠️ GUARD-аудит SE-TIME-FORMAT (статическая семош analyze.go НЕ должна ломать 001–007a): из `/Users/denis/dev/ladix` проверить, что НИ ОДНА 007a-фикстура/пример/тест (`examples/*.ladix`, `src/internal/parser/testdata`, eval-тесты) не содержит вне-диапазонный `в "ЧЧ:ММ"` с ожиданием parse-clean/semantic-clean; при РЕАЛЬНОМ конфликте — инвертировать замок (стал SE-TIME-FORMAT) ЛИБО рассмотреть перенос валидации на serve-time (diagnostics.md §тонкость регресса). Поведение `run` на ВАЛИДНЫХ программах неизменно (событие/расписание в `run` — no-op+лог, FR-001/026). Зависит от T032, T033
- [ ] T041 [P] Прогнать сценарии `quickstart.md` end-to-end (US1 edge+прайминг+рестарт; US2 graceful-stop+изоляция; US3 расписание+зажим+негатив формата; US4 emit→событие FIFO at-least-once; US5 рестарт-скан+дрейф; паритет Memory+SQLite) и подтвердить прохождение всех гейтов. Зависит от T038, T039
- [ ] T042 [P] Синхронизировать доки (FR-027): `docs/trigger-model.md §TR-11`, `docs/execution-model.md` EM-17/EM-11, обещание Store `docs/engine-model.md` «+6 → 6 триггерных + ListInstancesByStatus (deviation FR-022)»; SPEC §4/§10.4/§12; README (CLI `serve`/`emit`/`--interval`); `examples/MANIFEST.md` (+serve/emit demo-фикстуры); указатель плана в `CLAUDE.md` при дрейфе. Зависит от T038
- [ ] T043 Гейт SC-010 — полный прогон: из `/Users/denis/dev/ladix` `cd src && go build ./... && go vet ./... && go test ./...` зелёный, `gofmt -l` без правок; ЯВНЫЙ ЗАМОК: все тесты 001/002/003/004/005/006/007a остаются зелёными (поведение `run` и реестр диагностик 007a неизменны, кроме аддитивной SE-TIME-FORMAT). Зависит от T040, T041, T042

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: без зависимостей
- **Foundational / Store (Phase 2)**: после Setup — БЛОКИРУЕТ все стории (durable-носитель + листинг)
- **US1 (Phase 3)**: после Foundational (нужны Store-методы + `ResetRunState`/`Triggers()` + `Daemon`/`fire`)
- **US2 (Phase 4)**: после US1 (нужны `tick()`/`safeFire`/`Daemon` из US1)
- **US3 (Phase 5)**: после US1 (нужны `tick()`/`fire`/`Daemon`); `shiftEvery` независим (другой файл)
- **US4 (Phase 6)**: после US1 (нужны `tick()`/`fire`/Store-события)
- **US5 (Phase 7)**: после Foundational (`ListInstancesByStatus`) + US1 (`Daemon`); engine-часть независима
- **CLI (Phase 8)**: после US1–US5 (serve собирает `Run`/`RunRestartScan`); семош independent от daemon
- **Polish (Phase 9)**: после всех желаемых сторий

> ⚠️ Из-за слоистости (Store → eval/engine → daemon → CLI) стории НЕ полностью независимы — они
> инкрементальны. Каждая независимо *тестируема*, но опирается на код US1 (daemon-скелет). Это
> осознанное отклонение от «полностью параллельных сторий», как в 007a.
>
> ⚠️ **GUARD T040 (SE-TIME-FORMAT)**: статическая семош в `analyze.go` затрагивает и `run`, и `serve`.
> Аудит 007a-фикстур обязателен ДО гейта T043 — иначе скрытый регресс на валидных программах.

### Within Each User Story

- **Store (Phase 2)**: типы/сентинел (T002∥T003) → интерфейс (T004) → реализации (T005 SQLite ∥ T006 Memory) → паритет-тест (T007) ∥ регресс-замок (T008)
- **US1**: eval-блокеры (T009→T010→T011, T012 регресс) → daemon-скелет (T013→T014) → fire (T016) + evalMetrics (T015) + Kind/ID (T017) → тесты (T018)
- **US2**: `Run` (T019) → graceful-тест (T020) ∥ изоляция-тест (T021)
- **US3**: `shiftEvery` (T022→T023) ∥ `checkSchedules` (T024→T025)
- **US4**: `drainEvents` (T026) → тесты (T027)
- **US5**: engine (T028→T029) → рестарт-скан daemon (T030→T031)
- **CLI**: семош (T032→T033) ∥ emit (T034) ∥ serve (T035) → диспетчер (T036) → фикстуры (T037) → golden (T038/T039)
- Тесты — после соответствующего кода

### Parallel Opportunities

- Store: T002 ∥ T003; T005 (SQLite) ∥ T006 (Memory); T007 ∥ T008
- US1: T010 ∥ T012 (после T009); T015 ∥ T016 ∥ T017 (разные файлы, после T014)
- US3: T022/T023 (calendar.go) ∥ T024/T025 (schedule.go)
- US5: T028/T029 (engine) ∥ начало daemon-части после T013
- CLI: T032/T033 (eval) ∥ T034 (emit) ∥ T035 (serve, после daemon); T037 ∥ T039
- Polish: T041 ∥ T042

---

## Parallel Example: Phase 2 (Store Foundational)

```bash
# Параллельно (разные файлы / реализации):
Task: "T002 Структуры TriggerState/Event в src/internal/store/types.go"
Task: "T003 Сентинел ErrTriggerStateNotFound в src/internal/store/store.go"
# Затем интерфейс T004, потом параллельно реализации:
Task: "T005 7 методов + DDL в src/internal/store/sqlite.go"
Task: "T006 7 методов + карты в src/internal/store/memory.go"
# Затем:
Task: "T007 Контрактный паритет-тест Memory+SQLite в src/internal/store/trigger_store_test.go"
Task: "T008 Регресс-замок 8 методов/3 таблиц 006 в src/internal/store/store_test.go"
```

## Parallel Example: US3 (расписание — два независимых файла)

```bash
Task: "T022 shiftEvery (фикс/календарный+зажим) в src/internal/daemon/calendar.go"
Task: "T024 checkSchedules в src/internal/daemon/schedule.go"  # после T022
```

---

## Implementation Strategy

### MVP (P1)

1. Phase 1 Setup → Phase 2 Foundational (Store +7 методов, 2 таблицы, паритет)
2. **US1** (Phase 3): eval-сброс + daemon-скелет + edge-детект → ВАЛИДАЦИЯ: тик1/2/3 ложь→истина ровно раз, прайминг, durable-рестарт (T018)
3. **US2** (Phase 4): `Run(ctx)` + изоляция → ВАЛИДАЦИЯ: graceful-stop без утечки, паника изолирована (T020/T021)
4. **US3** (Phase 5): расписание `каждые`/`в` + зажим → ВАЛИДАЦИЯ: 31янв+1мес→28фев, раз в сутки (T025)
5. **US4** (Phase 6): события FIFO at-least-once → ВАЛИДАЦИЯ: emit→drainEvents (T027)
6. P1-набор (US1–US4) + CLI (serve/emit) = поставляемый MVP фичи

### Incremental Delivery

1. Foundational → Store готов (durable-носитель)
2. + US1 → edge-детект живой (демо: тик ловит ложь→истина ровно раз)
3. + US2 → демон эксплуатируем (демо: graceful-stop + изоляция)
4. + US3 → расписание исполняется (демо: зажим конца месяца + раз в сутки)
5. + US4 → события доставляются (демо: emit→событие)
6. + US5 (P2) → рестарт-скан (демо: догон залипшего + дрейф)
7. + CLI + семош формата → полная CLI-поверхность serve/emit + SE-TIME-FORMAT

### Границы 007b (НЕ делать здесь — отложено в v2)

Cron-выражения; приём событий по сети; exactly-once; кеш метрик между тиками; стабильный ключ триггера
при правке исходника (хеш условия); FixedClock-CLI-флаг; составное условие метрики; листинг инстансов
сверх рестарт-скана как первоклассный запрос; поля миграций схемы SQLite.

---

## Notes

- [P] = разные файлы, нет незавершённых зависимостей
- Тесты включены: приёмка фичи — контрактный паритет Store (Memory+SQLite), управляемые часы (R-10,
  прямой `d.tick()`, НЕ реальный `time.Ticker`), golden stdout serve/emit, exact-match SE-TIME-FORMAT
- **Часы инъектируются ВСЕГДА**: никогда `time.Now()` напрямую в коде демона; двойные часы (engine-Clock
  `time.Time` / eval-Clock `value.Дата`) управляемы в тестах (R-4/FR-024)
- **Детерминированные фикстуры** (`cmd/ladix/testdata/`) — источники без окна периода; `выручка.ladix`
  НЕ переиспользуется (оконная по дате, недетерминирована, R-10)
- **GUARD T040**: SE-TIME-FORMAT статическая (analyze.go) → аудит 007a-фикстур на вне-диапазонный
  `"ЧЧ:ММ"`; поведение `run` на валидных программах неизменно (событие/расписание в `run` — no-op+лог)
- **Седьмой метод `ListInstancesByStatus`** — осознанное отступление от «+6» (FR-022, deviation),
  синк обещания Store в T042
- **Инвариант границы §TR-11** (FR-026): синтаксис трёх форм, AST-узлы и реестр диагностик 007a
  НЕ меняются (кроме одной аддитивной SE-TIME-FORMAT); программа, валидная в 007a без вне-диапазонного
  `"ЧЧ:ММ"`, остаётся валидной в 007b
- Гарантии доставки асимметричны: метрика at-most-once (persist ДО тела), событие at-least-once
  (mark ПОСЛЕ тела); SC-003 измеряет обе
- Коммит после каждой задачи или логической группы
