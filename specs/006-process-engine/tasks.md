# Tasks: Движок исполнения процессов + человек-в-цикле

**Input**: Design documents from `/specs/006-process-engine/`

**Prerequisites**: plan.md (фазы A–E), spec.md (4 user story P1/P1/P2/P2, 43 FR, SC-001…009),
research.md, data-model.md, quickstart.md, contracts/{store,engine,eval-runtime,cli,diagnostics}.md

> **Связывающий якорь — источник истины.** Поведение, контракты, кадр шага, машина состояний, гарды
> `complete`, активация deferred-веток, CLI и **байт-точные** реестры stdout (§EN-7) / диагностик
> (§EN-8) — `docs/engine-model.md §EN-0…§EN-10`; решения Q1–Q3 / D-1…D-22 (§EN-1) приняты и **не
> переоткрываются**. Приёмочная таблица — §EN-9. При расхождении задачи и якоря побеждает якорь.

**Tests**: tests-first ОБЯЗАТЕЛЬНЫ (конституция VI): для каждой story тестовая(ые) задача(и) идут ПЕРЕД
имплементацией; тесты пишутся так, чтобы **падать** до имплементации. Стиль — table-driven +
golden/exact-match, co-located `*_test.go`. Вывод — через инжектированный `out` (`bytes.Buffer`); часы
движка — через `WithClock`.

**Organization**: задачи сгруппированы по user stories (US1–US4) поверх Setup + Foundational; каждая
story независимо реализуема и проверяема как MVP-инкремент.

## Format: `[ID] [P?] [Story] Описание`

- **[P]**: можно параллелить (другой файл, нет зависимости от незавершённых задач).
- **[US#]**: к какой user story принадлежит задача (только для фаз user story).
- Каждая задача несёт точный путь к файлу и привязку к FR-NNN / §EN-N / D-N.

## Path Conventions

- Корень Go-модуля — `src/` (модуль `github.com/denis-kosyakov/ladix`, `go 1.22`); все `go`-команды от
  `src/`. Абсолютная база путей кода — `/Users/denis/dev/ladix/src/` (зеркало `…/ladix/src/`).
- `examples/` и `.gitignore` — на **корне репозитория** (сиблинг `src/`).

---

## Phase 1: Setup (Shared Infrastructure) = Фаза A (зависимость)

**Purpose**: первая внешняя зависимость проекта + игнор файла БД; отдельным коммитом (FR-041).

- [X] T001 Подключить `modernc.org/sqlite` (чистый Go без CGO, первая внешняя зависимость): из `src/`
  выполнить `go get modernc.org/sqlite@latest` + `go mod tidy`, зафиксировать правку `src/go.mod`
  (`require modernc.org/sqlite`) и новый `src/go.sum` **отдельным коммитом** (FR-041). Проверить
  `go build ./...` собирается с новой зависимостью.
- [X] T002 [P] Добавить `ladix.db` в `.gitignore` на корне репозитория (FR-040, GAP-9).

**Checkpoint**: зависимость подключена отдельным коммитом, `ladix.db` игнорируется — фундамент готов.

---

## Phase 2: Foundational (Blocking Prerequisites) = Фаза A (хранилище)

**Purpose**: пакет `internal/store` — типы, контракт из 8 методов, две реализации, кодек, mint id.
Блокирует ВСЕ user stories (движок и CLI строятся над Store).

**⚠️ CRITICAL**: ни одна user story не может начаться, пока эта фаза не завершена.

### Тесты Foundational (tests-first — написать и убедиться, что падают) ⚠️

- [X] T003 [P] Написать `src/internal/store/codec_test.go` — round-trip кодека type-tagged JSON для всех
  10 типов значений (Целое/Дробное/Строка/Булево/Пусто/Длительность/Период/Дата/Список/Запись) через
  `SaveInstance`/`LoadInstance`; включить D-5 (`NaN`/`+Inf`/`-Inf` строками), D-6 (Запись массивом пар,
  порядок `Keys()`), D-21 (ключи верхнего уровня по возрастанию), вложенность Список/Запись; убедиться
  что падает (FR-006, §EN-2).
- [X] T004 [P] Написать `src/internal/store/memory_test.go` — lifecycle `MemoryStore`, **без алиасинга**
  (мутация загруженного инстанса/карты/задачи не видна в Store до `Save`), mint `p-000001`/`t-000001`,
  `MarkTaskCompleted` атомарность (повтор → `ErrTaskAlreadyCompleted`), `ListPendingTasks` фильтр и
  порядок по возрастанию id, сентинелы через `errors.Is`; убедиться что падает (FR-007, FR-001…004).
- [X] T005 [P] Написать `src/internal/store/sqlite_test.go` — round-trip `SQLiteStore` (10 типов),
  персист счётчика через `Close`+переоткрытие (продолжение нумерации, D-10), идемпотентность схемы
  (повторное открытие безвредно), `MarkTaskCompleted` атомарность, `ListPendingTasks` порядок; времена
  `time.RFC3339` (не Nano); убедиться что падает (FR-002, FR-005, FR-006).

### Имплементация Foundational

- [X] T006 [P] Создать `src/internal/store/types.go` — `ProcessInstance` (7 полей), `Status` (6 констант
  `создан`/`выполняется`/`ожидает`/`выполнен`/`провален`/`отменён`), `Task` (8 полей), `TaskStatus`
  (`открыта`/`завершена`), 3 сентинела (`ErrInstanceNotFound`/`ErrTaskNotFound`/`ErrTaskAlreadyCompleted`,
  английские) — дословно §EN-2 (FR-001…004).
- [X] T007 Создать `src/internal/store/store.go` — интерфейс `Store` ровно из 8 методов (D-3):
  `SaveInstance`/`LoadInstance`/`SaveTask`/`LoadTask`/`ListPendingTasks`/`MarkTaskCompleted`/
  `NextInstanceID`/`NextTaskID`; листингов инстансов и триггерных методов НЕ объявлять (зависит от T006).
- [X] T008 Создать `src/internal/store/codec.go` — кодек type-tagged JSON (10 типов; `NaN`/`±Inf`
  строками D-5; Запись массивом пар, порядок `Keys()` D-6; верхний уровень `Variables` по возрастанию
  ключей D-21; round-trip честный) — внутреннее дело `SQLiteStore`; зелёный `codec_test.go` (FR-006,
  зависит от T006).
- [X] T009 Создать `src/internal/store/memory.go` — `MemoryStore` + `NewMemoryStore()`; карты
  инстансов/задач + счётчики под одним `sync.Mutex`; **без алиасинга** (`Save`/`Load` копируют инстанс,
  карту `Variables` и задачу; значения разделяются); `MarkTaskCompleted` под mutex; mint
  `p-%06d`/`t-%06d`; зелёный `memory_test.go` (FR-007, зависит от T006/T007).
- [X] T010 Создать `src/internal/store/sqlite.go` — `SQLiteStore` + `NewSQLiteStore(path)`/`Close()`
  (`modernc.org/sqlite`); явный `Exec` DDL §EN-2 (`instances`/`tasks`/`counters` + индекс
  `idx_tasks_pending` + сид `counters` `INSERT OR IGNORE`); PRAGMA EM-7 (`journal_mode=WAL`,
  `busy_timeout=5000`, `foreign_keys=ON`); mint в одной транзакции (D-10); времена `time.RFC3339`;
  использовать `codec` для `variables`; зелёный `sqlite_test.go` (FR-002, FR-005, FR-006, зависит от
  T006/T007/T008).

**Checkpoint**: гейт фазы A — round-trip 10 типов честный, `p-000001`/`t-000001` детерминированы,
счётчик персистентен, `MarkTaskCompleted` атомарен, схема идемпотентна; `go test ./internal/store/...`
зелёный (FR-001…007, FR-040/041). Фундамент готов — US можно начинать.

---

## Phase 3: User Story 1 — Запустить процесс до точки ожидания человека (Priority: P1) 🎯 MVP

= **Фаза B** (движок + интеграция eval).

**Goal**: `ladix run examples/онбординг.ladix` (MemoryStore) синхронно прогоняет шаги, на человеческом
шаге создаёт задачу и засыпает; байт-точный stdout сценария А (5 строк), exit 0.

**Independent Test**: `Start` с `WithClock(2026-05-31 00:00:00)` → 5 строк stdout + состояние Store
(`p-000001` `ожидает`/`провести_встречу`, `{имя,сотрудник}`, `t-000001` `открыта`); `run` без `--db`
зелёный (SC-001).

### Тесты US1 (tests-first — написать и убедиться, что падают) ⚠️

- [X] T011 [P] [US1] Написать `src/internal/engine/engine_test.go` — сценарий А байт-точный (5 строк,
  `WithClock(2026-05-31 00:00:00 Local)`, `out` = `bytes.Buffer`) + проверка состояния Store на выходе;
  фаза атрибутов: `исполнитель: 42` → ОшибкаТипа §EN-8.A `шаг '<имя>': исполнитель должен быть Строка,
  получено Целое`, инстанс `провален`; тело `присвоить x = 1/0` → `деление на ноль`, инстанс
  `провален`; абсолютизация дедлайна D-19 (множители единиц, `мес` календарно); убедиться что падает
  (SC-001, FR-008…014, FR-017).
- [X] T012 [P] [US1] Написать тесты границы `eval↔engine` в `src/internal/eval/` (новый
  `runtime_test.go` или в существующем `stmt_test.go`/`expr_test.go`): `ExecStepBody` трёхслойный кадр
  `global→processEnv→stepEnv`; `присвоить` пишет в `procEnv` (виден в `Locals()`) + дёргает хук;
  `x = E` мутирует слой без хука; пусть-локаль шага в `procEnv` не утекает; `RunProcessExpr` →
  `value.Строка{V:id}` в трёх позициях; nil-runtime → `внутренняя ошибка: движок процессов не подключён`;
  убедиться что падает (FR-019…023).

### Имплементация US1

- [X] T013 [US1] Создать `src/internal/engine/clock.go` — интерфейс `Clock`, `SystemClock` (единственное
  `time.Now()` движка, D-2), `WithClock(c Clock) Option`; `time.Now()` только здесь (FR-017, зависит
  от T010).
- [X] T014 [P] [US1] Создать `src/internal/engine/deadline.go` — абсолютизация срока D-19: множители
  `сек`/`мин`/`час`/`дн`(24ч)/`нед`(168ч), `мес` через `CreatedAt.AddDate(0,n,0)`; шаг без срока →
  `Deadline==nil` (FR-012).
- [X] T015 [P] [US1] Создать `src/internal/engine/format.go` — `FormatTaskLine(t,now)` (единый источник
  формата строки задачи, D-22, §EN-7 строка 6: поля через 2 пробела, хвост `срок до <время>` только при
  дедлайне, `ПРОСРОЧЕНА` только при просрочке) и `Overdue(t,now)` (`now.After(*Deadline)`; nil →
  false, EM-13) (FR-017, FR-035).
- [X] T016 [US1] Объявить интерфейс `ProcessRuntime` в `src/internal/eval/` (новый файл, напр.
  `runtime.go`): 7 методов `StartProcess`/`AssignProcessVar`/`CallExternal`/`Notify`/`InstanceStatus`/
  `InstanceVariables`/`UserTasks` — дословно §EN-4 (D-1; eval НЕ импортирует store/engine).
- [X] T017 [US1] Добавить экспорт `eval` в `src/internal/eval/`: `SetProcessRuntime(rt)` (поле
  `i.runtime`, защитный nil → §EN-8.A `внутренняя ошибка: движок процессов не подключён`), `Process(name)`,
  `GlobalEnv()`, `EvalExpr(env,e)`, `ExecStepBody(processEnv,stepEnv,body)` (поле `i.procEnv`,
  save/restore реентерабельно), `Locals()` на `Environment` (снапшот локального слоя) — §EN-4
  (FR-019, FR-023, зависит от T016).
- [X] T018 [US1] Активировать `RunProcessExpr` в `src/internal/eval/expr.go:48-49` (заменить
  `deferredConstruct` на: вычислить аргументы → `i.runtime.StartProcess(P,args)` → `value.Строка{V:id}`);
  активировать `AssignAction`/`CallAction`/`NotifyAction` в `src/internal/eval/stmt.go:63-64`
  (запись в `procEnv` + хук `AssignProcessVar`; стабы `CallExternal`/`Notify`) — §EN-5 (FR-020…022,
  зависит от T017).
- [X] T019 [US1] Создать `src/internal/engine/engine.go` — `Engine`, `NewEngine(st,interp,out,opts...)`,
  поле `active []*activeFrame` (стек инстансов для атрибуции хука `присвоить`), `Start(name,args)`
  (mint id, bind params, `создан` ▼, `advance`, тихий запуск D-13), приватный `advance` (машина
  состояний §EN-3: `выполняется` ▼, кадр `processEnv` один на прогон, фаза атрибутов до тела D-9,
  тип-гарды D-18, тело `ExecStepBody`, развилка задача/продвижение/терминал, провал D-14,
  гранулярность персиста EM-9, `UpdatedAt=clock.Now()` перед каждым ▼) (FR-008…014, FR-017…019,
  зависит от T013/T014/T015/T017).
- [X] T020 [US1] Создать `src/internal/engine/runtime.go` — реализация `eval.ProcessRuntime`:
  `StartProcess`→`Start`; `AssignProcessVar` (обновить `Variables` вершины стека `active` + ▼SaveInstance);
  `CallExternal`/`Notify` (печать строк §EN-7.2 / 1–1а в `e.out`, всегда nil); `InstanceStatus`/
  `InstanceVariables`/`UserTasks` (для US4-builtins; реализовать сейчас, активация builtins — фаза E)
  (FR-018, FR-020…022, FR-026, зависит от T019).
- [X] T021 [US1] Связать `MemoryStore`-путь `run` без `--db` в `src/cmd/ladix/main.go`: собрать стек
  `interp`+`MemoryStore`+`engine.NewEngine`+`interp.SetProcessRuntime(eng)` для существующей команды
  `run` (без флага `--db` пока), чтобы `run examples/онбординг.ladix` исполнял процесс (§EN-6; полный
  CLI-разрез — US2; зависит от T019/T020).

**Checkpoint**: гейт фазы B — сценарий А байт-точный (5 строк, exit 0), состояние Store как в SC-001;
тип-гард `исполнитель:42` и провал `1/0` → канон §13 + `провален` (SC-004 частично); `run` без `--db`
зелёный. **MVP достижим.** (FR-008…014, FR-017…020, FR-022, SC-001)

---

## Phase 4: User Story 2 — Персистентная цепочка run --db → tasks → complete (Priority: P1)

= **Фаза C** (CLI-разрез).

**Goal**: мост в SQLite (`run --db`), просмотр задач (`tasks [исполнитель]`), завершение (`complete
<file> <task-id>`); состояние переживает завершение процесса ОС; повтор `run --db` даёт новые id.

**Independent Test**: цепочка из 6 команд сценария Б на свежей БД → ожидаемый stdout каждого шага и
exit 0; повтор → `p-000002`/`t-000003`; состояние только в файле БД (SC-002).

### Тесты US2 (tests-first — написать и убедиться, что падают) ⚠️

- [ ] T022 [US2] Перевернуть замок в `src/cmd/ladix/main_test.go:91 TestRunOnboardingProcessDeferred`:
  было «код 1, рантайм-граница 005» → стало **код 0 + golden сценария А** (5 строк, маска только
  `<время>` дедлайнов; id детерминированы); поправить стейл-докстринг теста; убедиться что падает до
  имплементации CLI (SC-002, FR-028).
- [ ] T023 [US2] Написать сценарий Б в `src/cmd/ladix/main_test.go` — цепочка из 6 команд на свежей БД
  (`run --db` → `tasks` → `tasks Петров` → `complete t-000001` → `complete t-000002` → повторный
  `run --db`); каждый шаг — ожидаемый stdout (маска только `<время>`) + exit 0; повтор даёт
  `p-000002`/`t-000003`; состояние между командами — только в файле БД; убедиться что падает
  (SC-002, FR-029…034).
- [ ] T024 [P] [US2] Написать `src/internal/engine/complete_test.go` — цепочка `Complete`:
  пробуждение → следующая задача → терминал (`выполнен`); проверить переход `ожидает`→`выполнен`
  напрямую при отсутствии следующего шага и через `выполняется` при наличии; убедиться что падает
  (FR-009, FR-013, FR-015, SC-002).

### Имплементация US2

- [ ] T025 [US2] Добавить `Complete(taskID)` + `CompleteResult{Instance,CaughtUp}` в
  `src/internal/engine/engine.go` (basic-путь без полного набора гардов — гарды/догон оформляются в US3):
  `LoadTask`/`LoadInstance` → `MarkTaskCompleted` → печать строки 7 → продвижение (`next==∅`→`выполнен`;
  иначе `advance`) → печать строк 9/10 (владелец печати — сам `Complete`, §EN-3); зависит от T019.
- [ ] T026 [US2] Обновить usage-строку в `src/cmd/ladix/main.go:31` до 4 команд (дословно cli.md):
  `использование: ladix run [--max-depth N] [--db путь] <файл> | ladix metric [--max-depth N] <файл>
  <имя> | ladix complete [--db путь] [--max-depth N] <файл> <task-id> | ladix tasks [--db путь]
  [исполнитель]`; ручной разбор флага `--db` (`--db значение` и `--db=значение`); существующие
  CLI-тексты без изменений + общий разбор флагов обеих форм (`--флаг значение` и `--флаг=значение`) и
  отказ на неизвестный флаг (CLI-ошибка `ladix: …`, §EN-8.B, exit 2) (FR-032).
- [ ] T027 [US2] Реализовать `run --db` в `src/cmd/ladix/main.go`: без `--db` — `MemoryStore` (как
  US1); с `--db` — `SQLiteStore` (`defer Close()` после успешного открытия; ошибка открытия →
  `ladix: не удалось открыть хранилище '<путь>': <причина>`, exit 2); сводка висящих задач
  `ListPendingTasks("")` → строки 5–6 §EN-7 **только при N≥1**, exit 0; повторный `run --db` создаёт
  новые инстансы (FR-029, FR-034, зависит от T021/T025).
- [ ] T028 [US2] Реализовать подкоманду `complete <file.ladix> <task-id> [--db] [--max-depth]` в
  `src/cmd/ladix/main.go` (Q3): ровно 2 позиционных (иначе usage exit 2); компиляция файла чисто
  (лексер→парсер→`Analyze`; ошибка → канон §13 exit 1), **`interp.Run` НЕ вызывается** (top-level не
  исполняется); `SQLiteStore` дефолт `ladix.db`; собрать стек + `eng.Complete(taskID)`; печать строк
  7–10 делает сам `Complete`; провал продвижения → канон §13 exit 1; обернуть подкоманду в
  guard/recover-барьер (конституция III) (FR-030, зависит от T025/T026).
- [ ] T029 [US2] Реализовать подкоманду `tasks [исполнитель] [--db]` в `src/cmd/ladix/main.go`
  (FR-031): файл НЕ принимать (всё из БД, движок/интерпретатор НЕ строятся); `SQLiteStore` дефолт
  `ladix.db`; `ListPendingTasks(фильтр)` → `engine.FormatTaskLine(t, engine.SystemClock{}.Now())` на
  задачу (строка 6 §EN-7), пусто → `открытых задач нет` (строка 11); exit 0 в обоих случаях; экспорт
  `engine.SystemClock{}` (инвариант D-2); обернуть подкоманду в guard/recover-барьер (конституция III)
  (зависит от T015/T026).

**Checkpoint**: гейт фазы C — сценарий Б на свежей БД, повтор даёт `p-000002`/`t-000003`, состояние
только в файле (SC-002); все 11 stdout-форматов §EN-7 exact-match (SC-005); несуществующий файл БД →
пустая БД со схемой (FR-034). US1+US2 работают независимо. (FR-029…036, SC-002, SC-005)

---

## Phase 5: User Story 3 — Гарды завершения: некорректный complete не ломает состояние (Priority: P2)

= **Фаза D** (гарды + диагностики).

**Goal**: полный порядок гардов `Complete` (§EN-3) + гард-догон D-4; CLI-ошибки §EN-8.B (одна строка
`ladix: <текст>`, exit 2); парс-ошибка → канон §13 exit 1; инстанс при отказе гарда не тронут.

**Independent Test**: на БД после сценария Б — 6 негативов §EN-9 (дословные stderr, коды 2/1); 2
фабрикованных кейса гардов D-8 на уровне движка; гард-догон D-4 (строка 8 + exit 0).

### Тесты US3 (tests-first — написать и убедиться, что падают) ⚠️

- [ ] T030 [US3] Написать 6 негативов §EN-9 в `src/cmd/ladix/main_test.go` (после шага 5 сценария Б):
  несуществующая задача → `ladix: задача 't-999999' не найдена` (2); повтор при `выполнен` →
  `ladix: задача 't-000001' уже завершена` (2); файл без процесса (`examples/hello.ladix`) →
  `ladix: процесс 'онбординг' не найден в определении` (2); дрейф шага (`testdata/онбординг-дрейф.ladix`)
  → `ladix: шаг 'закрыть_адаптацию' не найден в определении процесса 'онбординг'` (2); неоткрываемый
  путь БД → `ladix: не удалось открыть хранилище '<путь>': <причина>` (2); парс-ошибка
  (`testdata/сломанный.ladix`) → канон §13 (1); убедиться что падает (SC-003, FR-038).
- [ ] T031 [US3] Расширить `src/internal/engine/complete_test.go` двумя фабрикованными кейсами гардов
  D-8 (Store-состояние руками): открытая задача при инстансе `выполняется` →
  `инстанс '<p-id>' не ожидает (статус 'выполняется')`, инстанс не тронут; открытая задача с
  `StepName≠CurrentStep` ожидающего инстанса → `задача '<t-id>' не соответствует текущему шагу инстанса
  '<p-id>'`, инстанс не тронут; гард-догон D-4 (задача `завершена` + инстанс `ожидает` на том же шаге →
  строка 8 §EN-7 + до-продвижение, `CaughtUp=true`, exit 0); убедиться что падает (SC-003, FR-015/016).
- [ ] T032 [P] [US3] Написать exact-match-тесты диагностик §EN-8.A (9 текстов, канон §13, позиция,
  exit 1) — в `src/internal/engine/engine_test.go` / `src/internal/eval/*_test.go` где уместно; и
  §EN-8.B (10 текстов, одна строка `ladix: <текст>`, exit 2) — в `src/cmd/ladix/main_test.go`; убедиться
  что падает (SC-006, FR-037/038).

### Имплементация US3

- [ ] T033 [P] [US3] Создать testdata-фикстуры (FR-042): `src/cmd/ladix/testdata/онбординг-дрейф.ladix`
  (процесс `онбординг` есть, шаг `закрыть_адаптацию` переименован — дрейф Q3) и
  `src/cmd/ladix/testdata/сломанный.ladix` (файл с **парс**-ошибкой — `complete` файл не исполняет,
  его компиляция падает).
- [ ] T034 [US3] Реализовать полный порядок гардов `Complete` строго §EN-3 в
  `src/internal/engine/engine.go` (правка `Complete` из T025): задача → инстанс → **дрейф-гарды Q3**
  (процесс инстанса в файле; `CurrentStep ∈ pd.Steps`) → **гард-догон D-4** (`t.Status==завершена` +
  инстанс `ожидает` на том же шаге → строка 8 вместо 7, до-продвижение без `MarkTaskCompleted`,
  `CaughtUp=true`, exit 0; иначе → `уже завершена` exit 2) → **гарды D-8** (`Status!=ожидает`;
  `CurrentStep!=t.StepName`) → `MarkTaskCompleted` (проигравший гонку D-12 → ветка догона) → печать →
  продвижение; любое нарушение гарда → типизированная ошибка для CLI, инстанс НЕ тронут (FR-015/016,
  зависит от T025).
- [ ] T035 [US3] Замаппить гарды `Complete` на CLI-тексты §EN-8.B в `src/cmd/ladix/main.go`: трансляция
  типизированных ошибок гардов / сентинелов Store в 10 текстов `ladix: <текст>` (B1…B10), exit 2;
  компиляционные/runtime-ошибки → канон §13 exit 1 (FR-038, D-20, зависит от T028/T034).
- [ ] T036 [US3] Реализовать маршрутизацию сбоев Store по инициатору (FR-018): пути от Ladix-узла
  (`запустить процесс`/`присвоить`/process-builtins, весь `advance` внутри `Start`) → §EN-8.A
  `сбой хранилища: <причина>` с позицией узла, exit 1 (в `engine.go`/`runtime.go`); CLI-пути
  `complete`/`tasks` (загрузки/`MarkTaskCompleted`/`ListPendingTasks`, ▼ внутри `advance` из `Complete`,
  декод битой БД) → §EN-8.B `ladix: сбой хранилища: <причина>`, exit 2 (в `main.go`) (зависит от
  T034/T035).

**Checkpoint**: гейт фазы D — все 6 негативов §EN-9 дословны (коды 2/1) (SC-003); 2 фабрикованных кейса
гардов D-8 (инстанс не тронут) + догон D-4; 19 текстов §EN-8.A/B байт-в-байт (SC-006). US1–US3 работают
независимо. (FR-003/004, FR-015/016, FR-018, FR-037/038/039, FR-042, SC-003, SC-006)

---

## Phase 6: User Story 4 — Длительности и процессные встроенные как живые значения (Priority: P2)

= **Фаза E** (длительности + процессные встроенные + переворот замков). Зависит от US1 (движок), по
коду `value`/`eval` параллелизуема с US2/US3, но gate (переворот замков + golden А в CLI) опирается на US2.

**Goal**: литерал длительности (`2дн`) — живое значение в любой позиции; сравнения `==`/`!=`/порядок в
одной единице; 3 процессные встроенные опрашивают движок; реестр 28 активных + 7 deferred; все замки
инвентаря §EN-0 перевёрнуты.

**Independent Test**: точечные приёмки §EN-9 (печать `2дн`; сравнения; вне диапазона; `статус_процесса`
живой/неизвестный/не-Строка; `задачи_пользователя("")`; `вчера()` остаётся deferred).

### Тесты US4 (tests-first — написать и убедиться, что падают) ⚠️

- [ ] T037 [P] [US4] Написать `src/internal/value/equal_test.go` (или расширить существующий) — case
  Длительность: `2дн == 2дн` → истина, `1час == 60мин` → ложь (D-17, единица+значение, без
  нормализации); `2дн < 5дн` → истина; `2дн < 5мин` → `Compare ok==false` (существующий текст
  ОшибкиТипа `'<оп>' нельзя применить к Длительность и Длительность`); убедиться что падает (SC-004,
  FR-025).
- [ ] T038 [P] [US4] Написать точечные приёмки слоя §EN-9 в `src/internal/eval/*_test.go`: печать `2дн`
  (D-7/D-16); литерал вне диапазона → `литерал длительности вне диапазона типа Целое` (D-16);
  `статус_процесса` на живом/неизвестном (`процесс '<id>' не найден`)/не-Строка
  (`статус_процесса: ожидается Строка, получено Целое`); `задачи_пользователя("")` → все открытые;
  `вчера()` остаётся deferred (`функция 'вчера' не поддерживается в этой версии`); убедиться что падает
  (SC-004, FR-024/026/027).
- [ ] T039 [US4] Адаптировать замки-тесты под новое поведение (должны падать до имплементации):
  `src/internal/eval/builtins_test.go:146 TestBuiltinDeferredAll` (итерировать 7 оставшихся имён,
  комментарий «10»→«7»), `:12 TestBuiltinRegistryClosed` (счёт 28+7),
  `src/internal/eval/analyze_decl_test.go:753 TestAnalyzeDeferredBoundaryUnchanged` (`2дн` и
  `статус_процесса(1)` — семантика чиста, проверка переезжает в рантайм) (SC-008, FR-028).

### Имплементация US4

- [ ] T040 [P] [US4] Активировать case `Длительность` в `src/internal/value/equal.go`: `Equal`
  (`==`/`!=` по паре единица+значение) и `Compare` (`<`/`<=`/`>`/`>=` только одна единица → по
  значению; разные единицы → `ok==false`) — D-17; зелёный `equal_test.go` (FR-025).
- [ ] T041 [US4] Активировать рантайм `DurationLit` в `src/internal/eval/expr.go:50-51`: заменить
  `deferredConstruct` на `strconv.ParseInt(Amount,10,64)` → `value.Длительность{Amount,Unit}`; вне
  диапазона → ОшибкаВыполнения `литерал длительности вне диапазона типа Целое` с позицией литерала
  (§EN-8.A, D-16); снять семантический deferred — удалить `case *ast.DurationLit` в
  `src/internal/eval/analyze.go:429-430` (литерал валиден в любой позиции, D-7) (FR-024, зависит от T040).
- [ ] T042 [US4] Активировать 3 процессные встроенные в `src/internal/eval/builtins.go`: убрать
  `статус_процесса`/`состояние_процесса`/`задачи_пользователя` из `deferredNames`
  (`builtins.go:49-52`, 10→7) и зарегистрировать как `fixed(имя, 1, …)` активные через `i.runtime`
  (`InstanceStatus`/`InstanceVariables`/`UserTasks`); аргумент не-Строка → ОшибкаТипа §EN-8.A (позиция
  вызова); неизвестный id → `процесс '<id>' не найден` (D-15) (FR-026, зависит от T020).
- [ ] T043 [US4] Удалить мёртвую рантайм-заглушку: убрать `deferredConstruct`
  (`src/internal/eval/interpreter.go:148-150`) — после активации `RunProcessExpr`/`DurationLit`/действий
  остаётся без вызовов (staticcheck U1000); `constructName` оставить (живой у контекст-гарда
  `analyze.go:371`) (§EN-5, FR-028, зависит от T018/T041/T042).
- [ ] T044 [US4] Перевернуть замки-комментарии инвентаря §EN-0 (FR-028): счёт `interpreter.go:20`
  («23 активных + 12» → «28 активных + 7»), `builtins.go:55` («РОВНО 25 активных + 10 deferred = 35» →
  «28 активных + 7 deferred = 35»), комментарий `deferredNames` `builtins.go:47` («10 отложенных» →
  «7»); шапка `src/internal/value/deferred.go:3-7` (Длительность ТЕПЕРЬ конструируется через
  `DurationLit`); стейл `src/internal/eval/clock.go:16-17` («ЕДИНСТВЕННЫЙ легальный вызов time.Now()»
  → единственный в **eval**; у движка свой `engine.SystemClock`, D-2) (SC-008, SC-009, зависит от
  T039…T043).

**Checkpoint**: гейт фазы E — точечные приёмки §EN-9 целиком (SC-004); реестр 28+7, счёт-комментарии
согласованы (SC-008); инвариант D-2 grep-ом — два легальных `time.Now()` (SC-009); все тесты 001–005
зелёные, golden 004 неизменен (FR-043). US4 работает. (FR-024…028, FR-043, SC-004, SC-008, SC-009)

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: финальный синк документации и сквозные гейты SC-007.

- [ ] T045 [P] Синхронизировать `examples/MANIFEST.md` (строка онбординга): код 0, golden сценария А,
  id детерминированы, маска только `<время>` (FR-042, SC-008).
- [ ] T046 Прогнать финальные гейты SC-007 из `src/`: `go build ./...`, `go vet ./...`, `gofmt -l .`
  (пусто), `go test ./...` (зелёное, включая регрессы 001–005), `go test -race ./...`; проверить, что
  guard/recover-барьер оборачивает КАЖДУЮ подкоманду (`run`/`metric`/`complete`/`tasks`) — конституция
  III; устранить любые замечания.
- [ ] T047 Проверить инвариант D-2 (SC-009): `grep -rn "time.Now()" internal --include='*.go' | grep -v
  '_test.go'` → ровно 2 места (`internal/eval/clock.go` дневные, `internal/engine/clock.go` lifecycle);
  любое третье вхождение — нарушение.
- [ ] T048 Подтвердить регрессы: golden-вывод 004 (`TestGoldenSM10`, §SM-10) не изменился ни на байт;
  все тесты 001–005 зелёные (FR-043, SC-007).
- [ ] T049 [P] Тест-реестр всех 11 stdout-форматов §EN-7 exact-match: явная карта формат→покрывающий
  тест (1/1а уведомить, 2 вызвать, 3 задача-со-сроком, 4 задача-без-срока, 5 заголовок сводки, 6 строка
  задачи, 7 завершена, 8 до-продвинута, 9 инстанс-ожидает, 10 инстанс-выполнен, 11 открытых-задач-нет);
  добавить недостающие ассерции (1а, 8, 11), чтобы каждый из 11 форматов имел хотя бы одну
  exact-match-проверку, в `src/cmd/ladix/main_test.go`, `src/internal/engine/engine_test.go`,
  `src/internal/eval/*_test.go` (SC-005, FR-035, FR-036, §EN-7).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1 = Фаза A зависимость)**: без зависимостей — стартует сразу.
- **Foundational (Phase 2 = Фаза A хранилище)**: после Setup — **БЛОКИРУЕТ все user stories**.
- **US1 (Phase 3 = Фаза B)**: после Foundational (Store для lifecycle).
- **US2 (Phase 4 = Фаза C)**: после US1 (движок для CLI-моста).
- **US3 (Phase 5 = Фаза D)**: после US2 (CLI-цепочка как поверхность для гардов).
- **US4 (Phase 6 = Фаза E)**: после US1 (движок для process-builtins); по коду `value`/`eval`
  параллелизуема с US2/US3, но её gate (переворот замков + golden А в CLI) опирается на US2.
- **Polish (Phase 7)**: после всех желаемых user stories.

### User Story Dependencies

- **US1 (P1)**: после Foundational — без зависимостей от других stories. **MVP.**
- **US2 (P1)**: после US1 — расширяет CLI до SQLite-моста и `complete`/`tasks`.
- **US3 (P2)**: после US2 — гарды поверх CLI-цепочки `complete`.
- **US4 (P2)**: после US1 — активация языковых конструкций; gate опирается на US2 (golden А в CLI).

### Within Each User Story

- Тесты пишутся и **падают** ДО имплементации (конституция VI).
- Типы/модели до сервисов; сервисы до CLI-эндпоинтов; ядро до интеграции.
- Story завершена и зелёная до перехода к следующему приоритету.

### Parallel Opportunities

- Setup: T002 [P] параллельно T001.
- Foundational: тесты T003/T004/T005 [P] вместе; `types.go` (T006) [P] первым, затем T007→T008/T009/T010.
- US1: тесты T011/T012 [P]; `deadline.go` (T014) и `format.go` (T015) [P] независимы друг от друга.
- US3: testdata-фикстуры (T033) [P] и exact-match-тесты диагностик (T032) [P].
- US4: тесты T037/T038 [P]; `value.Equal/Compare` (T040) [P] независим от eval-правок до T041.
- Polish: T045 [P] и T049 [P] независимы от гейтов T046/T047/T048.
- Разные user stories разными разработчиками — US3 и US4 после US2 могут идти параллельно (US4 не
  зависит от US3; общий лишь финальный gate).

---

## Parallel Example: Foundational (Phase 2)

```bash
# Тесты Foundational вместе (tests-first, должны падать):
Task: "codec_test.go — round-trip 10 типов (T003)"
Task: "memory_test.go — lifecycle + без алиасинга (T004)"
Task: "sqlite_test.go — round-trip + персист счётчика (T005)"

# После types.go (T006) — кодек и реализации:
Task: "codec.go — type-tagged JSON (T008)"
Task: "memory.go — MemoryStore (T009)"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1 Setup (T001–T002).
2. Phase 2 Foundational (T003–T010) — **КРИТИЧНО: блокирует все stories**.
3. Phase 3 US1 (T011–T021).
4. **STOP и проверить**: сценарий А байт-точный, exit 0, состояние Store (SC-001).
5. Демо MVP: `ladix run examples/онбординг.ladix` исполняет процесс.

### Incremental Delivery

1. Setup + Foundational → фундамент готов.
2. + US1 → сценарий А (SC-001) → демо MVP.
3. + US2 → сценарий Б, SQLite-мост (SC-002, SC-005) → демо человек-в-цикле.
4. + US3 → гарды `complete` (SC-003, SC-006).
5. + US4 → длительности и встроенные (SC-004, SC-008, SC-009).
6. Polish → финальные гейты (SC-007).

### Parallel Team Strategy

1. Команда вместе закрывает Setup + Foundational.
2. Затем US1 (P1) — критический путь к MVP.
3. После US1: US2 (P1) на критическом пути; US4 можно вести параллельно по `value`/`eval`.
4. После US2: US3 и US4 независимы (общий лишь финальный gate); сходятся в Polish.

---

## Notes

- [P] = другой файл, нет зависимости от незавершённых задач.
- [US#] связывает задачу с user story для трассируемости (только фазы user story).
- Каждая story независимо завершаема и проверяема.
- Проверять, что тесты падают до имплементации (tests-first, конституция VI).
- Все системные строки и диагностики — **дословно** §EN-7/§EN-8 (конституция VIII); exact-match.
- Часы движка `engine.Clock` — отдельны от дневных `eval.Clock` (golden 004 не трогать, FR-043, D-2).
- Коммитить после задачи или логической группы; `modernc.org/sqlite` — отдельным коммитом (T001).
- Граф пакетов без циклов: `engine → eval, store, ast, value, errors`; `eval` НЕ импортирует
  store/engine (цикл разорван интерфейсом `ProcessRuntime` в eval, D-1).
