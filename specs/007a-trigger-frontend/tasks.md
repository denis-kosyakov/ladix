---
description: "Task list — 007a-trigger-frontend"
---

# Tasks: 007a — Фронтенд триггеров `когда …:` + метрика-fire-if-true в `run`

**Input**: Design documents from `specs/007a-trigger-frontend/`
**Dev root**: `/Users/denis/dev/ladix` (модуль в `src/`); пути ниже — относительно этого корня.
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/ (5), quickstart.md, docs/trigger-model.md §TR-10 «Каскад синков»

**Tests**: ВКЛЮЧЕНЫ. Контракты (`contracts/*.md`) и `quickstart.md` определяют приёмку через
table-driven позитивы/негативы и golden stdout (байт-точные диагностики) — это интринсик фичи и
устоявшаяся конвенция фич 001–006.

**Organization**: задачи сгруппированы по user story. Из-за слоистости компилятора (AST → парсер →
семантика → run) стории **инкрементальны**: каждая независимо *тестируема*, но опирается на код
предыдущих (зависимости явно указаны).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: можно выполнять параллельно (разные файлы, нет незавершённых зависимостей)
- **[Story]**: US1 / US2 / US3 / US4 (только для фаз стори)
- Точные пути файлов — в каждой задаче

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: базовая готовность ветки и сборки. 0 новых пакетов — расширяются существующие.

- [X] T001 Подтвердить ветку `007a-trigger-frontend` и зелёный бейзлайн: из `/Users/denis/dev/ladix` выполнить `cd src && go build ./... && go test ./...`; зафиксировать, что рабочее дерево чистое и собирается до начала правок

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: AST-узлы и токены, на которые ссылаются ВСЕ стории. ⚠️ Ни одна стори не может начаться до завершения этой фазы.

- [X] T002 [P] Подтвердить токены лексера триггеров в `src/internal/lexer/token.go` — все 7 видов уже присутствуют (`KW_WHEN :57`, `KW_METRIC :44`, `KW_EVENT :58`, `KW_SCHEDULE :60`, `KW_EVERY :61`, **`KW_IN :62`** для ключевого слова `в` (НЕ `KW_AT`), `KW_VALUE :59`) и 6 единиц длительности (`сек/мин/час/дн/нед/мес`) тоже есть. Задача **confirm-only** — новый вид токена заводить НЕ надо, правок лексера фича не требует
- [X] T003 [P] Создать AST-узлы триггеров в `src/internal/ast/trigger.go` (новый файл): `TriggerDecl` (встраивает `declBase`; поля `Spec TriggerSpec`, `Body *Block`; `NewTriggerDecl`); маркер-интерфейс `TriggerSpec` (метод `triggerSpec()`); `MetricTrigger` (`specBase`; `Metric Ident`, `Op CompOp`, `Threshold Expression`; `NewMetricTrigger`); `EventTrigger` (`Event Ident`; `NewEventTrigger`); `ScheduleTrigger` (`Spec ScheduleSpec`; `NewScheduleTrigger`); маркер-интерфейс `ScheduleSpec` (метод `scheduleSpec()`); `EverySchedule` (`Every *DurationLit`; `NewEverySchedule`); `AtSchedule` (`At StringLit` по значению; `NewAtSchedule`). Переиспользовать существующий `CompOp` из `src/internal/ast/op.go`. Пакет `ast` остаётся листовым (без импорта `errors`). Позиции узлов = токен-якорь (`когда`/`метрика`/`событие`/`расписание`/`каждые`/`в`)
- [X] T004 [P] Добавить первичные выражения `ValueExpr` и `EventExpr` в `src/internal/ast/expr.go`: встраивают `exprBase`, беспараметрические (по образцу `NoneLit`), `NewValueExpr`/`NewEventExpr`; позиция = токен `значение`/`событие`
- [X] T005 [P] Юнит-тесты новых AST-узлов в `src/internal/ast/trigger_test.go` и `src/internal/ast/expr_test.go`: конструкторы проставляют `Pos`; реализация интерфейсов (`TriggerDecl` ⇒ `Decl`+`TopLevelItem`; три спеки ⇒ `TriggerSpec`; `EverySchedule`/`AtSchedule` ⇒ `ScheduleSpec`; `ValueExpr`/`EventExpr` ⇒ `Expression`); проверка, что пакет `ast` не импортирует `errors`

**Checkpoint**: AST + токены готовы — стории могут начинаться.

---

## Phase 3: User Story 1 — Объявить триггер в трёх формах и статически проверить (Priority: P1) 🎯 MVP

**Goal**: автор `.ladix` объявляет `когда метрика|событие|расписание …:` с индентным телом; лексер+парсер+семпроход принимают все три формы единообразно, без ошибок.

**Independent Test**: `examples/выручка.ladix` (одна метрика-форма `когда метрика … < … : запустить процесс …(значение)`, §TR-9/SC-001) → `run` даёт exit 0 и ноль диагностик. Покрытие трёх форм parse-clean и всех 6 единиц `каждые` — за table-тестом парсера T011, НЕ в самом `выручка.ladix`.

### Парсер (швы D-TR-1)

- [X] T006 [US1] Шов A — top-level диспетчер в `src/internal/parser/parse_stmt.go`: убрать `KW_WHEN` из `isUnexpectedTopLevel` (`:40-48`; `KW_VALUE` и `{` ОСТАЮТСЯ отвергаемыми — §TR-10.5 п.1); в `parseTopLevelItem` (`:12-34`) добавить ветку `if p.check(lexer.KW_WHEN) { return p.parseTriggerDecl() }` перед вызовом `isUnexpectedTopLevel` (§TR-10.5 п.2)
- [X] T007 [US1] Добавить `parseTriggerDecl` + диспетчер форм в **существующий** файл `src/internal/parser/parse_decl.go` (добавить функции, файл НЕ новый — §TR-10.5 п.2): поглотить `когда`; по следующему токену `KW_METRIC`/`KW_EVENT`/`KW_SCHEDULE` → `parseMetricTrigger`/`parseEventTrigger`/`parseScheduleTrigger`, иначе диагностика **SE-TRIGGER-KIND**; затем ожидать `:` и `parseBlock()` → `Body`. Зависит от T003, T006
- [X] T008 [US1] Реализовать `parseMetricTrigger` и `expectCompOp` в `src/internal/parser/parse_decl.go`: `метрика Ident expectCompOp() <порог>`; `expectCompOp` — exact-match 6 токенов сравнения (через `compOpOf`), иначе **SE-EXPECT-COMPOP**. Порог разбирать так, чтобы логический `и`/`или` НЕ поглощался (плоский разбор, одно сравнение — §TR-4 п.6), оставляя `и` неразобранным для последующей ошибки ожидания `:`. Зависит от T007
- [X] T009 [US1] Реализовать `parseEventTrigger` (`событие Ident` → `EventTrigger`) и `parseScheduleTrigger`+`parseScheduleSpec` (`расписание`; `каждые DurationLit` → `EverySchedule` | `в StringLit` → `AtSchedule`; иначе **SE-SCHEDULE-SPEC**) в `src/internal/parser/parse_decl.go`. Все 6 единиц `каждые` и любая строка `в "…"` принимаются без валидации содержимого. Зависит от T007
- [X] T010 [P] [US1] Шов B — Primary `значение`/`событие` в `src/internal/parser/parse_expr.go`: в `parsePrimary` добавить `case lexer.KW_VALUE: return ast.NewValueExpr(...)` и `case lexer.KW_EVENT: return ast.NewEventExpr(...)` (после `KW_RUN`, перед `default`); добавить `KW_VALUE`/`KW_EVENT` в `startsExpression` (`:18-26`, FIRST(Expression)) — §TR-10.5 п.3. `событие.поле` собирается даром существующим постфиксным `FieldExpr`. Зависит от T004
- [X] T011 [US1] Table-тесты парсера (позитивы) в `src/internal/parser/parse_decl_test.go`: три формы парсятся; все 6 единиц `каждые` (`сек/мин/час/дн/нед/мес`); `в "08:30"`; `событие.поле` даёт `FieldExpr` над `EventExpr`; `значение` как первичное в выражении. Это canonical-покрытие трёх форм и 6 единиц (НЕ через фикстуру `выручка.ladix`). Зависит от T007–T010

### Инверсия регресс-замков (co-land с T006 — §TR-10.5 п.5/п.6)

> ⚠️ Удаление `KW_WHEN` из `isUnexpectedTopLevel` (T006) ломает ДВА существующих негативных теста.
> Обе инверсии **обязаны попасть в один коммит с T006**, иначе `go test ./...` краснеет.

- [X] T012 [P] [US1] Инвертировать замок `src/internal/parser/examples_test.go` (§TR-10.5 п.5, FR-024): `TestDeclarativeExamplesUnexpected` (`:40-58`) держит единственный кейс `{"выручка.ladix", "неожиданный токен 'когда'"}` (`:45`). После T006 `выручка.ladix` больше не падает на `когда` → **удалить запись `выручка.ladix`**; набор пустеет (единственный кейс) → **предпочтительно удалить таблицу-кейс/тест целиком** (других declarative-примеров, ожидающих parse-error в v1, нет), либо — по §TR-10.5 п.5 — наполнить новым негативом, остающимся parse-error (`значение` на верхнем уровне). Co-land с T006. Зависит от T006
- [X] T013 [P] [US1] Инвертировать golden-замок `src/internal/parser/errors_golden_test.go:61` (§TR-10.5 п.6 — НОВЫЙ, FR-020): `TestGoldenSEUnexpectedTopLevel` держит `leads := []string{"когда", "значение", "{"}` и ждёт `SE-UNEXPECTED` на каждый ведущий токен. После T006 ведущий `когда` уходит в `parseTriggerDecl` и `SE-UNEXPECTED` не даёт → **убрать `"когда"` из `leads`** (оставить `"значение"` и `"{"` — оба остаются `неожиданный токен …`, т.к. `KW_VALUE` и `{` сохранены в `isUnexpectedTopLevel` по §TR-10.5 п.1). Отдельный файл от T012 — инвертировать ОБА. Co-land с T006. Зависит от T006 (контекст T007)

### Семантика (семпроход)

- [X] T014 [US1] Расширить сигнатуры обхода в `src/internal/eval/analyze.go`: протянуть флаги `inMetricTrigger`/`inEventTrigger` через `checkStmts`/`checkStmt`/`checkExpr`/`checkElse` (зеркало существующего `inStep`)
- [X] T015 [US1] Регистрация триггеров — Шаг 1d в `Analyze` (`src/internal/eval/analyze.go`): обойти `prog.Items`, для каждого `*ast.TriggerDecl` вызвать `checkTrigger`, затем добавить в `i.triggers` в порядке объявления; завести поле `i.triggers []*ast.TriggerDecl` в `src/internal/eval/interpreter.go`. Зависит от T003
- [X] T016 [US1] Диспетчер `checkTrigger(td)` в `src/internal/eval/analyze.go`: `MetricTrigger` → резолв имени метрики в `i.metrics` (**TR-MET-UNDECL**/**TR-MET-NOTMETRIC**, образец `checkMetricDecl`), `checkExpr(Threshold)`, `checkTriggerBody(inMetricTrigger=true)`; `EventTrigger` → `checkTriggerBody(inEventTrigger=true)` (имя события не резолвится — реестра нет в 007a); `ScheduleTrigger` → `checkTriggerBody(оба false)` (содержимое строки не анализируется). Зависит от T014, T015
- [X] T017 [US1] Контекст-гарды в `checkExpr` (`src/internal/eval/analyze.go`): `*ast.ValueExpr` требует `inMetricTrigger`, иначе **TR-VAL-CTX**; `*ast.EventExpr` требует `inEventTrigger`, иначе **TR-EVT-CTX**. Переиспользовать в теле триггера существующие гарды: `inStep=false` для действий-шага (§PM-6.B), `checkRunProcess` для `запустить процесс` (§PM-6.C), `inFunction=false` для `вернуть`. Зависит от T014
- [X] T018 [P] [US1] Позитивные семантические тесты в `src/internal/eval/analyze_trigger_test.go`: валидные триггеры всех трёх форм проходят чисто; `значение` в теле метрики — OK; `событие` в теле события — OK; `запустить процесс` с верной арностью — OK; метрика резолвится. Зависит от T016, T017

### Приёмка фикстуры (независимый тест US1)

- [X] T019 [US1] Перевести **существующую** фикстуру `examples/выручка.ladix` (одна метрика-форма `когда метрика выручка_месяца < 3_000_000: запустить процесс разбор_падения(значение)` — синтаксис файла НЕ меняется, §TR-9/§TR-10.5 п.4/SC-001) из parse-error в parse-clean: добавить приёмочный тест (exit 0, ноль диагностик). Покрытие трёх форм и 6 единиц — за T011, НЕ в этом файле. Комплементарно T012 (снятие негативного ожидания). **Независимый тест US1**. Зависит от T011, T018

**Checkpoint**: US1 функциональна — валидные триггеры парсятся и проходят семпроход; оба регресс-замка инвертированы, `go test ./...` зелёный.

---

## Phase 4: User Story 2 — Запустить процесс из метрики через fire-if-true в `run` (Priority: P1)

**Goal**: `run <файл>`; после исполнения top-level и до сводки задач — проход по метрика-триггерам: вычислить метрику → порог → сравнить; если истинно → инжектировать read-only `значение`, исполнить тело, запустить процесс штатным путём движка 006.

**Independent Test**: демо-программа, собираемая ИНЛАЙН golden-тестом в `t.TempDir()` (`trigger_golden_test.go`, без testdata-/`examples/`-фикстуры; метрика 2M, порог `< 3_000_000`) → процесс создан, `значение`=2M, задача в сводке; контр-демо (`< 1_000_000`) → молчание, пустая сводка, exit 0.

- [X] T020 [US2] Реализовать `func (i *Interpreter) RunTriggers(w io.Writer) error` в `src/internal/eval/trigger_run.go` (новый): обойти `i.triggers` в порядке объявления; для `*ast.MetricTrigger` — вычислить значение метрики, вычислить порог в `i.global`, сравнить существующим оператором сравнения 003 (`i.compareValues(ast.BinOp(spec.Op), …)`) → Булево; база ЛОЖЬ эфемерно ⇒ если истинно: новый `env := NewEnvironment(i.global)`, `env.Define("значение", metricVal)` (read-only обеспечено тем, что `значение` — ключевое слово `KW_VALUE`, не присваиваемое в синтаксисе), исполнить тело; `*ast.EventTrigger`/`*ast.ScheduleTrigger` — no-op + строка-заглушка в `w` («…требует serve (фича 007b)»). Зависит от T015, T016
- [X] T021 [US2] Вспомогательные пути исполнения в `src/internal/eval/` (`trigger_run.go`/`interpreter.go`): `evalMetricByName` (или переиспользование пути вычисления метрики) и `evalBlockInTrigger`; обеспечить, что `запустить процесс` в теле доходит до `Engine.StartProcess` (штатный путь 006, id `p-NNNNNN`) и персистится под `--db`. Состояние триггеров НЕ читается/НЕ пишется (нет `trigger_state`). Зависит от T020
- [X] T022 [US2] Врезка вызова `interp.RunTriggers(stdout)` в `src/cmd/ladix/main.go` между `interp.Run(prog)` и `st.ListPendingTasks("")` (§TR-8.1): при ошибке — печать в stderr, exit 1. Зависит от T020
- [X] T023 [P] [US2] Golden-тест `run` в `src/cmd/ladix/trigger_golden_test.go`; программа строится ИНЛАЙН в `t.TempDir()` (а не из testdata-фикстур): `демо.ladix` (истина → процесс + `значение`=2M + задача в сводке); контр-демо (`< 1_000_000` → молчание, пустая сводка, exit 0); повторный `run --db` того же файла идентичен (база-ложь эфемерна, оба прогона срабатывают, id сдвигаются, `trigger_state` отсутствует). Зависит от T022

**Checkpoint**: US1 + US2 — P1-набор полон (фронтенд принимает + метрика-триггер запускает процесс).

---

## Phase 5: User Story 3 — Точные диагностики и негативы фронтенда (Priority: P2)

**Goal**: байт-точные диагностики с позицией при нарушении правил: контекст `значение`/`событие`, неизвестная/не-метрика, отсутствие вида/оператора/формы расписания, действия-шага, арность процесса, `вернуть`.

**Independent Test**: негативные фикстуры дают байт-точные диагностики (двухстрочный канон `Ошибка в строке N, колонка M:` + payload), exit 1.

- [X] T024 [P] [US3] Негативные тесты парсера в `src/internal/parser/parse_decl_test.go`: **SE-TRIGGER-KIND** (нет вида после `когда`), **SE-EXPECT-COMPOP** (нет оператора после `метрика Ident`), **SE-SCHEDULE-SPEC** (нет `каждые`/`в` после `расписание`), пустой блок после `:` (переиспользуемая `msgEmptyBlock`) — точные тексты per `contracts/diagnostics.md`. Зависит от T007–T009
- [X] T025 [P] [US3] Негативные семантические тесты в `src/internal/eval/analyze_trigger_test.go`: **TR-VAL-CTX** (`значение` вне тела метрики), **TR-EVT-CTX** (`событие` вне тела события), **TR-MET-UNDECL**, **TR-MET-NOTMETRIC**; переиспользуемые — действие-шага в теле триггера, неверная арность/вид `запустить процесс`, `вернуть` вне функции. Зависит от T016, T017
- [X] T026 [US3] Синхронизировать реестр диагностик: централизовать/сверить тексты сообщений всех 7 новых (4 сем: TR-VAL-CTX/TR-EVT-CTX/TR-MET-UNDECL/TR-MET-NOTMETRIC; 3 синт: SE-TRIGGER-KIND/SE-EXPECT-COMPOP/SE-SCHEDULE-SPEC) и переиспользуемых семейств с `contracts/diagnostics.md` и `docs/trigger-model.md §TR-7` — байт-в-байт. Зависит от T024, T025

**Checkpoint**: все диагностики фронтенда покрыты и байт-точны.

---

## Phase 6: User Story 4 — Отказ на составном условии и невалидный токен на верхнем уровне (Priority: P3)

**Goal**: составное условие (`X < Y и Z > W`) → синтаксическая ошибка про ожидание `:` (грамматика плоская); `значение`/`событие`/`{`/`}` как ведущий top-level токен → `неожиданный токен`; ведущий `когда` теперь принимается.

**Independent Test**: фикстуры составного условия и невалидного top-level токена дают ожидаемые синт-ошибки; `когда` как ведущий токен — OK.

- [X] T027 [P] [US4] Тест плоской грамматики (одно сравнение) в `src/internal/parser/parse_decl_test.go`: `когда метрика X < Y и Z > W:` → синт-ошибка ожидания `:` (логика не собирается). Если `parseExpression` поглощает `и` — ограничить разбор порога per §TR-4 п.6 (правка в T008). Зависит от T008
- [X] T028 [P] [US4] Тест невалидного ведущего top-level токена в `src/internal/parser/parse_stmt_test.go` (или общий parser-тест): `значение`/`событие`/`{`/`}` в начале top-level → `неожиданный токен '<лексема>'` (`msgUnexpected`); подтвердить, что ведущий `когда` больше НЕ отвергается `isUnexpectedTopLevel`. Комплементарно инвертированному golden-замку T013 (там значение/`{` остаются SE-UNEXPECTED; здесь добавляется `событие` и подтверждение приёма `когда`). Зависит от T006

**Checkpoint**: краевые случаи v1-грамматики зафиксированы.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [X] T029 [P] Прогнать сценарии `quickstart.md` end-to-end (§3 A, §4 Б + контр-демо + повторный `run --db`, §5 негативы) и подтвердить прохождение всех
- [X] T030 [P] Синхронизировать доки: `docs/trigger-model.md §TR-0…§TR-11` и указатель плана в `CLAUDE.md` при дрейфе; сверить примеры в `spec.md` с реализованными диагностиками; **`examples/MANIFEST.md` синк** — перевести запись `выручка.ladix` (`:77`) из «отложенный синтаксис/демо» в **parse-clean + run-demo** (FR-023, §TR-10.5 п.8)
- [X] T031 Полный прогон: из `/Users/denis/dev/ladix` `cd src && go build ./... && go test ./...` зелёный; `gofmt -l` и `go vet ./...` чисты

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: без зависимостей
- **Foundational (Phase 2)**: после Setup — БЛОКИРУЕТ все стории (AST-узлы + токены)
- **US1 (Phase 3)**: после Foundational
- **US2 (Phase 4)**: после US1 (нужны `i.triggers` из T015 и `checkTrigger` из T016)
- **US3 (Phase 5)**: после US1 (диагностики производит код парсера T007–T009 и семантики T016–T017; US3 добавляет негативное покрытие)
- **US4 (Phase 6)**: после US1 (швы парсера T006, T008)
- **Polish (Phase 7)**: после всех желаемых сторий

> ⚠️ Из-за слоистости компилятора стории НЕ полностью независимы — они инкрементальны. Каждая независимо *тестируема*, но опирается на код US1. Это осознанное отклонение от «полностью параллельных сторий».
>
> ⚠️ **Регресс-замки T012/T013 co-land с T006** (один коммит): удаление `KW_WHEN` из `isUnexpectedTopLevel` ломает `examples_test.go` и `errors_golden_test.go` одновременно — без обеих инверсий гейт T031 `go test ./...` недостижим.

### Within Each User Story

- **US1 парсер**: шов A (T006) → диспетчер/формы (T007→T008/T009); шов B (T010) независим (другой файл). **Замки T012/T013 co-land с T006** (инверсия двух регресс-тестов, разные файлы).
- **US1 семантика**: сигнатуры (T014) → регистрация (T015) → диспетчер (T016) → гарды (T017)
- Тесты после соответствующего кода; фикстура-приёмка (T019) — последней в US1

### Parallel Opportunities

- Foundational: T002 / T003 / T004 параллельны (разные файлы); T005 после T003+T004
- US1: T010 (шов B) ∥ T006–T009 (шов A — иной файл); T012 ∥ T013 (разные файлы, оба после T006); T018 ∥ после T016/T017
- US3: T024 ∥ T025 (разные файлы)
- US4: T027 ∥ T028 (разные кейсы/файлы)
- Polish: T029 ∥ T030

---

## Parallel Example: Phase 2 (Foundational)

```bash
# Запустить параллельно (разные файлы):
Task: "T002 Подтвердить токены лексера в src/internal/lexer/token.go"
Task: "T003 Создать AST-узлы триггеров в src/internal/ast/trigger.go"
Task: "T004 Добавить ValueExpr/EventExpr в src/internal/ast/expr.go"
# Затем:
Task: "T005 Юнит-тесты AST-узлов в src/internal/ast/trigger_test.go и expr_test.go"
```

## Parallel Example: US1 инверсия замков (после T006)

```bash
# Оба регресс-замка — разные файлы, co-land с T006:
Task: "T012 Инвертировать замок src/internal/parser/examples_test.go (убрать выручка.ladix)"
Task: "T013 Инвертировать golden-замок src/internal/parser/errors_golden_test.go:61 (убрать «когда» из leads)"
```

## Parallel Example: User Story 3 (негативы)

```bash
Task: "T024 Негативные тесты парсера в src/internal/parser/parse_decl_test.go"
Task: "T025 Негативные семантические тесты в src/internal/eval/analyze_trigger_test.go"
```

---

## Implementation Strategy

### MVP (P1)

1. Phase 1 Setup → Phase 2 Foundational (AST + токены)
2. **US1** (Phase 3): парсер + инверсия замков T012/T013 + семпроход принимают три формы → ВАЛИДАЦИЯ: `examples/выручка.ladix` exit 0 (T019), `go test ./...` зелёный
3. **US2** (Phase 4): fire-if-true в `run` → ВАЛИДАЦИЯ: `демо.ladix` запускает процесс (T023)
4. P1-набор (US1+US2) = поставляемый MVP фичи

### Incremental Delivery

1. Foundational → AST готов
2. + US1 → фронтенд принимает (демо: parse-clean три формы; оба замка инвертированы, гейт зелёный)
3. + US2 → метрика-триггер запускает процесс (демо: fire-if-true)
4. + US3 → байт-точные диагностики (демо: негативы)
5. + US4 → краевые случаи грамматики

### Границы 007a (НЕ делать здесь — отложено в 007b)

Демон `serve`; доставка событий/`emit`/очередь; edge-детект ложь→истина и персист `trigger_state`;
исполнение расписания (`каждые`/`в`) и календарный сдвиг `нед`/`мес`; валидация формата `"ЧЧ:ММ"`;
рестарт-скан; составное условие метрики (v2). Событие/расписание в `run` — no-op строка-заглушка.

---

## Notes

- [P] = разные файлы, нет незавершённых зависимостей
- Тесты включены: приёмка фичи определена байт-точными диагностиками и golden stdout (конвенция 001–006)
- **`выручка.ladix` — одноформенный** (одна метрика-форма, §TR-9/SC-001); синтаксис файла НЕ меняется, меняется лишь статус parse-error → parse-clean. Три формы и 6 единиц покрываются table-тестом T011, событие/расписание — ручные дописки/table-кейсы
- **Два регресс-замка** (`examples_test.go` §TR-10.5 п.5/FR-024, `errors_golden_test.go` §TR-10.5 п.6/FR-020) инвертируются в US1 (T012/T013), co-land с T006 — иначе гейт T031 недостижим
- Коммит после каждой задачи или логической группы
- Инвариант границы (§TR-11): программа, валидная в 007a, остаётся валидной в 007b без правок синтаксиса
