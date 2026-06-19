# Implementation Plan: Бэкенд триггеров — демон `serve`, события и edge-детект (007b)

**Branch**: `007b-trigger-backend` | **Date**: 2026-06-13 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/007b-trigger-backend/spec.md`

> **Связывающий якорь — источник истины.** Авторитетные источники для слоя 007b —
> `docs/trigger-model.md §TR-11` (9 аддитивных швов), `docs/execution-model.md` EM-17
> (три фазы тика, edge-детект, расписание, события, рестарт-скан, режим `run`) и EM-11
> (конкурентность демона), плюс обещание `Store` в `docs/engine-model.md` (D-3/D-4).
> Спецификация выведена из якорей и не вводит требований сверх них; при расхождении любого
> источника (SPEC, README, engine-model) для 007b побеждает якорь. Инвариант границы §TR-11:
> всё ложится **аддитивно** — синтаксис, AST-узлы и реестр диагностик 007a не меняются (кроме
> одной новой семош формата времени FR-014). Открытых `[NEEDS CLARIFICATION]` **нет** — обе
> развилки §TR-11 (#2 пересчёт метрик, #6 рестарт-скан) закрыты решениями в research.md.

## Summary

Фича реализует **бэкенд исполнения триггеров**: долгоживущий демон `ladix serve` с тикером
(`time.Ticker`, флаг `--interval`, дефолт `1m`) и грациозной остановкой по `context.Context`/
`ctx.Done()`; durable edge-детект «ложь→истина» для метрика-триггеров через таблицу `trigger_state`;
календарное исполнение расписания (`каждые` для 6 единиц, `в "ЧЧ:ММ"` раз в сутки) + новая семош
валидации формата времени; источник событий `ladix emit` с очередью `events` и доставкой
at-least-once; рестарт-скан залипших инстансов. Хранилище `Store` расширяется аддитивно на **+7
методов** (6 триггерных + `ListInstancesByStatus`) и две таблицы, с паритетом SQLite/Memory.
Поведение команды `run` (007a fire-if-true) **не меняется**.

Тик последователен (EM-17.1/EM-11): один поток исполнения движка, строго `drainEvents → evalMetrics
→ checkSchedules`, триггеры одного вида — в порядке объявления. Сработавший триггер исполняет тело
**штатным путём движка 006** (`запустить процесс` → `Engine.Start` → инстанс `p-NNNNNN`,
fire-and-forget). Между тиками состояние интерпретатора, влияющее на пересчёт метрик
(`i.today` + `i.recordCache`), сбрасывается — иначе edge-детект мёртв (метрика вернёт снимок старта).

**Технический подход.** 0 новых пакетов уровня языка; **1 новый пакет** `internal/daemon`
(оркестратор тика — он импортирует `engine`+`store`+`eval`+`ast`, поэтому НЕ может жить ни в `eval`,
ни в `engine` без цикла, см. Structure Decision). Расширяются: `internal/store` (контракт +7 методов,
две таблицы в обоих бэкендах), `internal/eval` (метод `ResetRunState` на интерпретаторе + публичный
доступ к реестру триггеров), `internal/engine` (экспортный метод реактивации инстанса для
рестарт-скана), `internal/cmd/ladix` (две новые подкоманды `serve`/`emit` + диспетчер). Новая семош
формата `"ЧЧ:ММ"` ложится в семпроход `eval/analyze.go` аддитивно. Календарный сдвиг нед/мес —
**новый** хелпер в `daemon` (зажим конца месяца через `time.AddDate` + паттерн `lastDayOfMonth` из
`eval/window.go`); существующего `Дата ± Длит`-оператора в `eval` НЕТ (SPEC §10.4 относит его в v2),
поэтому переиспользуется только **паттерн зажима**, а не несуществующая операция.

**Что НЕ входит (§TR-11 / Out of Scope):** cron-выражения, приём событий по сети, exactly-once,
кеш метрик между тиками, стабильный ключ триггера при правке исходника, FixedClock-CLI-флаг,
составное условие метрики.

## Technical Context

**Language/Version**: Go 1.22+ (модуль в `src/`); идиоматичный код, `gofmt`/`go vet` чисто.

**Primary Dependencies**: stdlib (`time`, `context`, `os/signal`, `sync`, `database/sql`, `encoding/json`).
`modernc.org/sqlite` уже введена 006 — **новых зависимостей 007b не добавляет** (Принцип I).

**Storage**: `internal/store` (Memory / SQLite) расширяется **аддитивно**: две новые таблицы
(`trigger_state`, `events`) и +7 методов; 8 существующих методов и 3 таблицы (instances/tasks/
counters) 006 **не меняются** (FR-021/FR-022, SC-009). `trigger_state` читается/пишется только
демоном; `events` пишет `emit` (другой процесс ОС), читает демон — сериализация WAL + busy_timeout.

**Testing**: три стратегии (как 001–007a) + новый класс «демон с управляемыми часами».
table-driven юнит (Store-паритет Memory+SQLite, edge-детект, календарный сдвиг, валидация `"ЧЧ:ММ"`),
golden байт-точный stdout (`serve`/`emit` на детерминированных фикстурах `cmd/ladix/testdata/`),
exact-match новой диагностики. **Часы инъектируются** (никогда `time.Now()` в коде демона напрямую):
двойные часы — engine/scheduler (`time.Time`) и eval-метрики (`value.Дата` через `Clock`), оба
управляемые в тестах. Фикстуру `выручка.ladix` НЕ переиспользовать (оконная по дате,
недетерминирована) — отдельные детерминированные `.ladix`+`.json` фикстуры.

**Target Platform**: один статический бинарник `ladix`, кросс-компиляция Windows/macOS/Linux.

**Project Type**: интерпретатор/компилятор DSL с CLI-подкомандами; 007b добавляет демон-планировщик.

**Performance Goals**: N/A (масштаб DSL; каждый тик пересчитывает все метрики с нуля — EM-17.9,
кеш между тиками — v2).

**Constraints**: CGO запрещён; без глобального изменяемого состояния — демон держит свои поля,
прогон движка сериализуется `sync.Mutex` (EM-11), часы инъектируются; позиции в рунах для новой
диагностики; recover-барьер на каждый триггер тика (EM-17.6) И на границе CLI-подкоманды (Принцип
III); тексты сообщений — русские, дословно, новые тексты фиксируются по факту при реализации.

**Scale/Scope**: **1 новый пакет** (`internal/daemon`); расширяются `store`/`eval`/`engine`/`cmd`.
+7 методов Store, 2 таблицы. 2 новые подкоманды (`serve`/`emit`). 1 новая семош (формат `"ЧЧ:ММ"`).
Открытых `[NEEDS CLARIFICATION]` нет.

## Constitution Check

*GATE: пройден до Phase 0, перепроверен после Phase 1. Нарушений нет — `Complexity Tracking` пуст.*

| Принцип | Требование | Как план соблюдает | Статус |
|---|---|---|---|
| **I. Язык и сборка** | Go 1.22+, CGO запрещён, без новых тяжёлых зависимостей | Только stdlib + уже имеющаяся `modernc.org/sqlite`; 0 новых зависимостей; один бинарник | ✅ PASS |
| **II. Парсинг — ручной** | recursive descent, без генераторов/regex | Синтаксис 007a не трогается (§TR-11). Валидация `"ЧЧ:ММ"` — посимвольная проверка (5 рун, цифры+`:`), **без regex** (Принцип II) | ✅ PASS |
| **III. Ошибки — явные типы** | типы из `internal/errors`, recover-барьер на CLI | Новая семош → `СемантическаяОшибка`/`semErr`. Новый сентинел `store.ErrTriggerStateNotFound` объявляется явно, развёртка `errors.Is`. recover-барьер `guard()` на `serve`/`emit` (как `run`); ВНУТРИ тика — отдельный per-триггер `recover` (EM-17.6), который ловит панику тела, логирует и НЕ роняет демон (изоляция сбоя — не утечка stack trace) | ✅ PASS |
| **IV. Позиции — сквозные** | `Position{Line,Col}` (руны, с 1) на каждом узле/токене | Новая семош формата времени печатает строку/колонку токена-строки `AtSchedule.At` (позиция уже есть в AST 007a); подсчёт в рунах | ✅ PASS |
| **V. Без глобального состояния** | нет изменяемого состояния пакета; инъекция зависимостей | Демон — структура `Daemon`/`Scheduler` (поля: Store, Engine, Interpreter, Clock, mutex, ctx), создаётся явно; Store/Clock инжектируются; прогон движка под `sync.Mutex` (EM-11), НЕ глобал. `ResetRunState` — метод интерпретатора над его полями. Пакетных `var` с изменяемым состоянием нет | ✅ PASS |
| **VI. Тесты — вперёд (лексер/парсер)** | table-driven тесты, включая негативы, до/вместе с кодом | Лексер/парсер не меняются. Новая семош формата времени покрывается table-driven позитивами+негативами (валидные/`25:99`/`9:05`/`09:5`/мусор) в той же фазе; Store-методы — контрактные table-driven (Memory+SQLite) | ✅ PASS |
| **VII. Раскладка проекта** | стандартная раскладка, граф без циклов, `ast`/`value`/`errors` листовые | Новый `internal/daemon` импортирует `engine`/`store`/`eval`/`ast`/`errors`; **никто из них не импортирует `daemon`** → цикла нет (зеркало `engine → eval`, D-1). `ast`/`value`/`errors` остаются листовыми (007b их не трогает по импортам). Граф фиксируется в `ARCHITECTURE.md` (синк FR-027) | ✅ PASS |
| **VIII. Язык сообщений** | русский, двухстрочный канон, тексты дословно | Новая семош — русская, двухстрочный канон §13. Логи демона (срабатывание, «событие <имя> без триггеров», изоляция сбоя, дрейф рестарт-скана) — русские; точные тексты фиксируются по факту (импл-факты §TR-11, не пробел спеки) | ✅ PASS |
| **IX. Спека — источник истины** | поведение из доков; пробел → остановка, не догадка | Якорь — §TR-11 / EM-17 / EM-11 / engine-model. Обе развилки §TR-11 закрыты решениями владельца в research.md (#2, #6). Отступление «+6 → +7» (`ListInstancesByStatus`) зафиксировано явно как deviation (FR-022), не молчком; обещание Store приводится в соответствие при реализации | ✅ PASS |

**Complexity Tracking**: нарушений конституции нет — таблица пуста. Введение пакета `internal/daemon`
оправдано раскладкой (Принцип VII): оркестратор тика обязан видеть и `engine`, и `store`, и `eval`
одновременно — он не помещается ни в `eval` (который не знает `engine`/`store`, D-1), ни в `engine`
(который не должен расти до планировщика). Седьмой метод Store (`ListInstancesByStatus`) — осознанное
отступление от «+6», задокументированное в FR-022/research.md, а не нарушение принципа.

## Project Structure

### Documentation (this feature)

```text
specs/007b-trigger-backend/
├── spec.md              # Спецификация (готова: фаза specify + чеклист requirements)
├── plan.md              # Этот файл (/speckit-plan)
├── research.md          # Phase 0 — Decision/Rationale/Alternatives (вкл. РЕШЕНИЯ #2 и #6)
├── data-model.md        # Phase 1 — TriggerState/Event + DDL 2 таблиц + сигнатуры 7 Store-методов
├── quickstart.md        # Phase 1 — гейты, сценарии US1…US5, негативы, фикстуры
├── contracts/           # Phase 1 — to-code контракты
│   ├── store-methods.md #   7 методов Store: сигнатуры, семантика, паритет Memory/SQLite, DDL
│   ├── serve-command.md #   Подкоманда serve: флаги, жизненный цикл демона, остановка
│   ├── emit-command.md  #   Подкоманда emit: запись события, JSON-payload, exit-коды
│   ├── tick-contract.md #   Контракт тика: 3 фазы, edge-детект, расписание, события, рестарт-скан, recover
│   └── diagnostics.md   #   Новая семош формата "ЧЧ:ММ" байт-точно (правила 00–23:00–59, ведущие нули)
├── checklists/          # из фазы specify (requirements.md)
└── tasks.md             # Phase 2 (/speckit-tasks — НЕ создаётся этим планом)
```

### Source Code (repository root)

Корень модуля — `src/` (там `go.mod`). 007b добавляет **один** пакет `internal/daemon`; остальное
расширяет существующие.

```text
src/
├── cmd/
│   └── ladix/
│       ├── main.go                 # ИЗМЕНЯЕТСЯ — диспетчер +case "serve"/"emit"; usage; (run/metric/complete/tasks неизменны)
│       ├── serve.go                # НОВЫЙ — serveMain: разбор флагов (--db/--interval/--max-depth),
│       │                           #   открытие Store, сборка Daemon, signal.NotifyContext, запуск, guard-барьер (FR-001/003)
│       ├── emit.go                 # НОВЫЙ — emitMain: <событие> [json] → Store.EnqueueEvent, exit 0 (FR-015)
│       ├── serve_golden_test.go    # НОВЫЙ — golden serve на детерминир. фикстуре (управляемые часы), Memory+SQLite (US1/US3/US4)
│       └── testdata/               # НОВЫЙ — детерминированные фикстуры (НЕ выручка.ladix):
│           ├── metric_edge.ladix   #   метрика-триггер + источник-фикстура для edge-детекта
│           ├── schedule.ladix      #   расписание каждые/в "ЧЧ:ММ"
│           ├── event.ladix         #   событие-триггер
│           └── *.json              #   источники без окна периода (детерминизм)
├── internal/
│   ├── daemon/                     # НОВЫЙ ПАКЕТ — оркестратор тика + планировщик (импортирует engine/store/eval/ast/errors)
│   │   ├── daemon.go               #   тип Daemon/Scheduler: поля (Store, Engine, Interpreter, Clock, mu, triggers);
│   │   │                           #     Run(ctx) — цикл time.Ticker + ctx.Done() (FR-002/003), грациозная остановка
│   │   ├── tick.go                 #   tick(): drainEvents → evalMetrics → checkSchedules; per-триггер recover (EM-17.1/.6, FR-002/004)
│   │   ├── metrics.go              #   evalMetrics: edge-детект ложь→истина, прайминг, заморозка, persist-до-тела (FR-005…010)
│   │   ├── schedule.go             #   checkSchedules: каждые (фикс сек/мин/час/дн + календарный нед/мес с зажимом), в "ЧЧ:ММ" (FR-011…013)
│   │   ├── calendar.go             #   НОВЫЙ хелпер shiftEvery: фикс-множитель vs календарный сдвиг нед/мес + зажим (паттерн lastDayOfMonth)
│   │   ├── events.go               #   drainEvents: FIFO, match по имени, payload JSON→Запись, исполнить тело, mark-после-тела (FR-016/017)
│   │   ├── restart.go              #   рестарт-скан: ListInstancesByStatus → реактивация/дрейф-лог (FR-019/020)
│   │   ├── fire.go                 #   общий исполнитель тела триггера (env-scope §TR-5, инжекция значение/событие) → Engine
│   │   └── *_test.go               #   юнит: edge-детект, календарный сдвиг (31янв+1мес→28/29фев), FIFO, рестарт-скан, recover, graceful-stop
│   ├── store/                      # РАСШИРЯЕТСЯ аддитивно (контракт +7, обе реализации)
│   │   ├── store.go                #   ИЗМЕНЯЕТСЯ — +7 методов в интерфейс; +ErrTriggerStateNotFound (комментарий «+6» уточняется на «+6 триггерных + ListInstancesByStatus»)
│   │   ├── types.go                #   ИЗМЕНЯЕТСЯ — +TriggerState, +Event (структуры EM-17.2/.3)
│   │   ├── sqlite.go               #   ИЗМЕНЯЕТСЯ — DDL +trigger_state/+events (+idx_events_pending), +sid counter 'event'; 7 методов
│   │   ├── memory.go               #   ИЗМЕНЯЕТСЯ — +карты triggerState/events + eventSeq под mu; 7 методов (паритет)
│   │   └── *_test.go               #   ИЗМЕНЯЕТСЯ/НОВЫЙ — контрактные table-driven тесты паритета Memory+SQLite на 7 методов
│   ├── eval/                       # РАСШИРЯЕТСЯ (сброс состояния + доступ к реестру + новая семош)
│   │   ├── interpreter.go          #   ИЗМЕНЯЕТСЯ — +ResetRunState() (i.today=nil; i.recordCache=make(...); reset clock-date); +Triggers() аксессор; SetClock/SetToday для двойных часов
│   │   ├── analyze.go              #   ИЗМЕНЯЕТСЯ — новая семош валидации формата "ЧЧ:ММ" в checkTrigger (AtSchedule), аддитивно
│   │   └── trigger_run.go          #   БЕЗ изменений по run (007a fire-if-true неизменен, FR-001); демон НЕ зовёт RunTriggers
│   └── engine/                     # РАСШИРЯЕТСЯ (точка входа для рестарт-скана)
│       └── engine.go               #   ИЗМЕНЯЕТСЯ — +экспортный Resume(inst)/ReactivateInstance: реактивировать залипший инстанс (advance), дрейф CurrentStep → ошибка/skip-сигнал
├── examples/
│   └── MANIFEST.md                 # ИЗМЕНЯЕТСЯ — +serve/emit demo-фикстуры (FR-027, доковый синк)
└── docs/ + SPEC.md + README.md     # ИЗМЕНЯЮТСЯ при реализации — синк §TR-11/EM-17, README CLI serve/emit/--interval, обещание Store «+6→+7» (FR-027)
```

**Structure Decision**: вводится **один** новый пакет `internal/daemon` — это единственное
архитектурно-чистое место для оркестратора тика. Граф зависимостей 006 разорван интерфейсом
`ProcessRuntime`: `eval` не знает `engine`/`store` (D-1). Планировщик же обязан одновременно держать
`Store` (читать `trigger_state`/`events`, листать инстансы), `*engine.Engine` (исполнять тело через
`Start`/реактивацию) и `*eval.Interpreter` (реестр триггеров, метрики, `ResetRunState`). Положить его
в `eval` → цикл `eval → engine`; в `engine` → разрастание движка до планировщика и связь с сигналами
ОС. Поэтому новый листовой-сверху пакет `daemon`, который импортируют только `cmd/ladix` (зеркало
того, как `cmd` уже собирает стек `interp+engine`). `ResetRunState` — метод над полями
интерпретатора (Принцип V); рестарт-скан реактивирует инстанс экспортным методом `engine`
(инкапсуляция lifecycle сохранена в `engine`). Прогон движка на тике сериализуется `sync.Mutex`
внутри `Daemon` (EM-11), без глобалов.

## Phasing — порядок поставки

Фазы зависимы: A → B → C → D → E → F. Каждая завершается гейтом `go build ./... && go vet ./... &&
gofmt -l && go test ./...` (зелёный, включая регресс 001–007a). Тексты диагностик — дословно из
contracts/diagnostics.md; точные тексты логов демона — импл-факты, фиксируются в фазе.

- **Фаза A (Store: +7 методов, 2 таблицы, паритет).** 🎯 `TriggerState`/`Event` в `types.go`;
  `ErrTriggerStateNotFound`; интерфейс +7 методов; DDL `trigger_state`/`events` (+индекс, +sid
  counter `event`) в SQLite; карты+`eventSeq` под `mu` в Memory. **Гейт:** контрактные table-driven
  тесты паритета Memory+SQLite на все 7 методов (load-miss→ErrTriggerStateNotFound, save/load
  round-trip, FIFO ListUnprocessedEvents, mark-processed идемпотентно, ListInstancesByStatus по
  статусам). 8 методов и 3 таблицы 006 не тронуты. *(FR-021/022/023, SC-009)*

- **Фаза B (eval: сброс состояния + доступ + двойные часы).** 🎯 `ResetRunState()` (i.today=nil;
  i.recordCache=make; переустановка даты вычисления от часов планировщика — двойные часы FR-024);
  аксессор `Triggers()`; (если нужно для часов) `SetClock`. **Гейт:** юнит — после `ResetRunState`
  метрика пересчитывается со свежим `сегодня()` и пустым `recordCache` (мутационная проверка:
  изменили источник-фикстуру → новый снимок). 007a `run`/fire-if-true регресс зелёный. *(FR-005/024,
  SC-001/004 — спинной блокер edge-детекта)*

- **Фаза C (engine: реактивация инстанса).** 🎯 Экспортный `ReactivateInstance(inst)` (или `Resume`):
  для залипшего «выполняется»/«создан» — найти `CurrentStep` в `ProcessDecl`, прогнать `advance`;
  `CurrentStep` отсутствует → вернуть распознаваемую ошибку дрейфа (демон логирует, инстанс залипает).
  **Гейт:** юнит — инстанс «выполняется» с валидным шагом догоняется до ожидания/терминала; дрейф →
  сигнал дрейфа без паники; инстанс «ожидает» методом не трогается (вызывается только из рестарт-скана
  по статусам). *(FR-019/020, SC-008)*

- **Фаза D (daemon: тик, edge-детект, расписание, события, изоляция).** 🎯 Тип `Daemon`; `Run(ctx)`
  (ticker + ctx.Done + graceful); `tick()` строго `drainEvents → evalMetrics → checkSchedules` с
  per-триггер `recover`; `evalMetrics` (прайминг/edge/заморозка/persist-до-тела); `checkSchedules`
  (`каждые` фикс vs календарный нед/мес+зажим в `calendar.go`; `в "ЧЧ:ММ"` раз/сутки); `drainEvents`
  (FIFO, payload→Запись, mark-после-тела); общий `fire` (env-scope §TR-5, инжекция `значение`/
  `событие`, прогон через mutex). **Гейт:** юнит на управляемых часах — edge ровно один раз (3 тика),
  прайминг 0 ложных, заморозка при невычислимой метрике, `каждые 1мес` от 31янв→28/29фев,
  `в "09:30"` раз/сутки, FIFO двух событий, событие без триггеров→лог+processed, паника одного
  триггера изолирована, graceful-stop без утечки горутин. *(FR-002…013/016…018/024/025, SC-001…007)*

- **Фаза E (CLI: serve + emit + рестарт-скан + семош формата).** 🎯 `serveMain` (флаги,
  `signal.NotifyContext`, рестарт-скан при старте → `daemon.Run`, guard-барьер); `emitMain`
  (`EnqueueEvent`, exit 0); диспетчер `main.go` +case; новая семош валидации `"ЧЧ:ММ"` в `analyze.go`
  (00–23:00–59, ведущие нули, посимвольно, без regex). **Гейт:** golden serve/emit на
  детерминированных testdata-фикстурах (Memory+SQLite паритет); exact-match новой диагностики
  (`25:99`/`9:05`/`09:5`/мусор → exit 1; `09:05`/`14:30` ок); рестарт-скан подбирает залипший,
  дрейф пропускается с логом. *(FR-001/014/015/019/020, SC-005/006/008)*

- **Фаза F (Синки и регресс).** 🎯 Доковый синк §TR-11/EM-17/EM-11, обещание Store «+6→+7»
  (`engine-model.md`), README (CLI `serve`/`emit`/`--interval`), SPEC §12, `MANIFEST.md`
  (+serve/emit demo). **Гейт SC-010:** `go build/vet/gofmt` чисто, `go test ./...` зелёное включая
  весь регресс 001–007a (поведение `run` и реестр диагностик 007a неизменны, кроме аддитивной семош
  формата времени). *(FR-026/027, SC-009/010)*

## Phase 0 — Research & Decisions

[research.md](./research.md) консолидирует решения в формате **Decision / Rationale / Alternatives**,
выведенные из якорей и кросс-слинкованные с FR-номерами. Включает **два обязательных решения §TR-11**:

- **#2 — стратегия пересчёта метрик между тиками.** Принят вариант **A**: явный метод
  `Interpreter.ResetRunState()` (сброс `i.today=nil` + `i.recordCache=make(...)` + переустановка даты
  вычисления от часов планировщика), зовётся в начале каждого тика. Без сброса edge-детект молча
  никогда не срабатывает (спинной блокер). Альтернативы B (свежий Interpreter на тик) и C (tickID)
  отвергнуты с обоснованием.
- **#6 — семантика рестарт-скана.** Сигнатура `ListInstancesByStatus(status string) ([]*ProcessInstance,
  error)`. При дрейфе (`CurrentStep` залипшего инстанса не найден в перезагруженном `ProcessDecl`) —
  лог расхождения + инстанс остаётся залипшим (не угадывать шаг, не падать, демон стартует). Седьмой
  метод Store зафиксирован как **осознанное отступление** от «+6» (`engine-model.md`), названо deviation.

Прочие решения: размещение пакета `daemon` (граф без циклов), двойные часы и точка их инъекции,
порядок «persist trigger_state ДО тела» (at-most-once) против «mark event ПОСЛЕ тела» (at-least-once),
календарный сдвиг нед/мес без существующего `Дата±Длит` (новый хелпер, паттерн зажима из window.go),
mutex вокруг прогона движка (EM-11), сериализация emit↔serve через WAL+busy_timeout.

## Phase 1 — Design & Contracts

[data-model.md](./data-model.md) фиксирует сущности `TriggerState` (поля Kind/LastBool/LastFire/
LastFiredDate, ключ `trg-<N>`, инварианты прайминга/заморозки) и `Event` (id `e-NNNNNN`, имя,
payload, created_at, processed), DDL обеих таблиц (зеркало EM-17.3) и **точные сигнатуры 7
Store-методов** с семантикой и контрактом паритета Memory/SQLite.

Контракты в `contracts/` — готовые to-code: `store-methods.md` (7 сигнатур, DDL, сентинел, паритет),
`serve-command.md` (флаги, жизненный цикл, грациозная остановка по сигналу), `emit-command.md`
(запись события, JSON-payload, exit-коды), `tick-contract.md` (3 фазы строго `drainEvents→
evalMetrics→checkSchedules`, edge-детект, прайминг, persist-до-тела, mark-после-тела, календарный
сдвиг, рестарт-скан, per-триггер recover, graceful-stop), `diagnostics.md` (новая семош формата
`"ЧЧ:ММ"` байт-точно: правила 00–23 : 00–59, ведущие нули).

[quickstart.md](./quickstart.md) — как собрать и прогнать приёмку: гейты качества (SC-010),
детерминированные testdata-фикстуры (Clock-инъекция, НЕ `выручка.ladix`), сценарии US1 (edge ровно
раз + прайминг + рестарт), US2 (graceful-stop + изоляция сбоя), US3 (расписание `каждые`/`в` +
зажим конца месяца + негатив формата), US4 (emit→событие FIFO at-least-once), US5 (рестарт-скан +
дрейф), паритет Memory+SQLite.

## Phase 2 — Tasks

`tasks.md` **не создаётся этим планом** — генерируется отдельно командой `/speckit-tasks` на основе
артефактов Phase 0/1.
