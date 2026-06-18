---
description: "Task list — C5 человеко-explain срабатывания"
---

# Tasks: C5 — человеко-explain срабатывания (наблюдаемость «почему»)

**Input**: Design documents from `/specs/022-human-explain-fire/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/ (eval-metric-condition.md,
explain-strings.md, golden-churn.md), quickstart.md

**Anchor**: `docs/reliability-model.md` §C-5 + `.m3-ledger/digest-anchor.md` (C5) + digest-seams.md.
Веха M3, пункт C5 (размер M). Предшественники C2a/C2b/C4 смержены.

**Tests**: Tests-first где применимо (конституция VI). C5 — почти-целиком тест-замки + golden co-land
(always-on explain ломает exact-match → co-land ОБЯЗАТЕЛЕН).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: можно параллельно (разные файлы, нет зависимостей)
- **[US1]**: единственная пользовательская история (P1)
- Пути — реальные (`src/internal/...`, `src/cmd/ladix/...`); build = `cd src && go build ./...`

## Path Conventions

Single project, стандартная Go-раскладка. Корень `src/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: подтвердить базу и швы перед правками.

- [ ] T001 Подтвердить базу: на ветке `022-human-explain-fire`, дерево чисто, `cd src && go build ./...` → BUILD_OK; зафиксировать текущую сигнатуру `EvalMetricCondition` (`src/internal/eval/trigger_daemon.go:31` = `(cur bool, snapshot value.Value, ok bool, err error)`) и единственный call-site `src/internal/daemon/metrics.go:39` (grep `EvalMetricCondition` по `src/` → ровно 1 определение + 1 вызов).
- [ ] T002 [P] Сверить швы печати по contracts: run fire-if-true `src/internal/eval/trigger_run.go:78-92` (писатель тела `i.out`, НЕ `w`); serve `src/internal/daemon/metrics.go` (снимок :39, ребро `fired` :72, ветка `if fired {` :82, писатель `d.logf` из `daemon.go:53`); форматтеры `value.String` (`src/internal/value/repr.go:20`) и `BinOp.String` (`src/internal/ast/op.go:35`).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: расширить eval-вычислитель, чтобы порог стал доступен serve-печати. БЛОКИРУЕТ serve-печать (T009) и serve-замки (T006). Прод-дифф пакета `eval`; `ProcessRuntime` не затрагивается.

- [ ] T003 Расширить СВОБОДНУЮ функцию `EvalMetricCondition` в `src/internal/eval/trigger_daemon.go:31`: добавить возвращаемое `threshold value.Value` → `(cur bool, snapshot value.Value, threshold value.Value, ok bool, err error)`. Возвращать `threshold` во ВСЕХ ветках: success → вычисленный порог; пустая метрика / несравнимые типы / ошибка → `value.None`. Per contracts/eval-metric-condition.md. Это НЕ метод интерфейса `ProcessRuntime` → интерфейс не трогать.
- [ ] T004 Обновить ЕДИНСТВЕННЫЙ call-site `src/internal/daemon/metrics.go:39`: деструктуризация на 5 значений `cur, snapshot, threshold, computable, err := d.interp.EvalMetricCondition(spec)`. На этом шаге `threshold` ещё не используется в печати (печать в T009) — только связать переменную (или `_`-плейсхолдер временно, снять в T009). Убедиться, что других call-site нет (grep).

**Checkpoint**: `cd src && go build ./...` → BUILD_OK с новой сигнатурой; существующие тесты пока компилируются (golden ещё краснеют только после печати — печать добавляется в US1).

---

## Phase 3: User Story 1 — оператор видит «почему» при срабатывании (Priority: P1) 🎯 MVP

**Goal**: при срабатывании метрик-триггера всегда печатать человеко-explain «почему» — run в `i.out` (без ребра), serve в `d.logf` (с ребром `ложь→истина`), ДО тела триггера; точный формат §C-5.3.

**Independent Test**: прогнать триггер, который срабатывает, на каждом пути (run и serve) → строка-explain присутствует в нужном канале с exact-match §C-5.3; на тике serve уже-истина (нет ребра) — строки нет.

### Тесты US1 (пишем вместе с правкой — конституция VI; новые замки)

- [ ] T005 [P] [US1] Новый замок `TestRunTriggerExplain` в `src/internal/eval/explain_test.go` (новый файл): прогнать run-fire триггер → exact-match run-строки §C-5.3 в `out` (`триггер '<имя> <оп> <порог>' сработал: <имя> = <снимок> (снимок) <оп> <порог> (порог) → истина`), напечатанной ДО тела. Числа без подчёркиваний. Per contracts/explain-strings.md.
- [ ] T006 [P] [US1] Новые замки в `src/internal/daemon/explain_test.go` (новый файл): (a) `TestServeTriggerExplain` — fire по ребру `ложь→истина` → exact-match serve-строки §C-5.3 в logf-буфере (`... сработал (ребро ложь→истина): ...`); (b) silence-замок — тик уже-истина (`LastBool=true`, `cur=true`, ребра нет) → новой explain-строки НЕТ; (c) inversion-замок (документировать намерение мутпробы): если порог не протянут (nil/None при success) → serve-строка печатает неверный/пустой порог → `TestServeTriggerExplain` краснеет. Per contracts/explain-strings.md, §C-5.4.

### Реализация US1

- [ ] T007 [US1] Run-печать: в `src/internal/eval/trigger_run.go:78-92` при `fired` (fire-if-true) сформировать run-строку §C-5.3 из `spec.Metric.Name`, `metricVal` (снимок), `threshVal` (порог), `BinOp(spec.Op)` и напечатать в `i.out` ДО исполнения тела. Если `runMetricTrigger` не имеет доступа к `i.out` — протянуть `i.out` (писатель тела, НЕ параметр `w` из `RunTriggers`; см. `metric_window_golden_test.go:102-104`). Per contracts/explain-strings.md.
- [ ] T008 [US1] Хелпер форматирования explain-строки (общий для run/serve, во избежание расхождения): функция, собирающая строку по §C-5.3 из (имя, оп, снимок, порог, withEdge bool). Разместить в `eval` или общем месте без новых зависимостей/циклов; `value.String` для чисел (без подчёркиваний), `BinOp.String` для оператора. (Если хелпер удобнее держать раздельно per-path — допускается, но обе формы дословно §C-5.3.)
- [ ] T009 [US1] Serve-печать: в `src/internal/daemon/metrics.go` в ветке `if fired {` (:82, ДО `safeFire`/`fireBody`) напечатать serve-строку §C-5.3 (с маркером `(ребро ложь→истина)`) через `d.logf`, используя `threshold` из обновлённого возврата (T003/T004). Печатать ТОЛЬКО при `fired` (ребро) — не на тике уже-истина (FR-008). Per contracts/explain-strings.md.

**Checkpoint**: новые замки T005/T006 зелёные; build OK. Существующие golden пока краснеют (это ожидаемо до Phase 4 co-land).

---

## Phase 4: §C-5.5 golden-churn co-land (ОБЯЗАТЕЛЬНО) — [US1]

**Purpose**: always-on explain добавляет строку в вывод путей со срабатыванием → exact-match существующих golden'ов ломается. Обновить co-land РОВНО 8 тестов в 4 файлах. Без этого гейт `go test` недостижим. Per contracts/golden-churn.md, §C-5.5.

### MUST UPDATE — serve goldens (exact-match `out.String()`)

- [ ] T010 [P] [US1] Обновить `TestTickPhaseOrderAllThreeFire` в `src/internal/daemon/tick_test.go`: вставить serve-explain строку метрики §C-5.3 в ожидаемый вывод (текущее `"E\nM\nS\n"`), сохранив порядок фаз.
- [ ] T011 [P] [US1] Обновить `TestTickFourPhasesOrder` в `src/internal/daemon/tick_test.go`: то же для `"E\nM\nS\nD\n"`.

### MUST UPDATE — run goldens (exact-match stdout / `i.out`)

- [ ] T012 [P] [US1] Обновить `TestRunTriggerFiresGolden` в `src/cmd/ladix/trigger_golden_test.go`: добавить run-explain строку §C-5.3 в ожидаемый stdout (порядок: explain ДО тела).
- [ ] T013 [P] [US1] Обновить `TestRunTriggerDBRepeatEphemeral` в `src/cmd/ladix/trigger_golden_test.go`: добавить run-explain строку §C-5.3.
- [ ] T014 [P] [US1] Обновить `TestRunTriggerMultiMetricOrderGolden` в `src/cmd/ladix/trigger_golden_test.go`: ДВА fire → ДВЕ explain-строки, в правильном порядке обработки триггеров.
- [ ] T015 [P] [US1] Обновить `TestRunTriggerMixedKindsOrderGolden` в `src/cmd/ladix/trigger_golden_test.go`: добавить run-explain строку(и) §C-5.3 в корректной позиции относительно прочего вывода.
- [ ] T016 [P] [US1] Обновить `TestRunTriggerBodyReadShadowGolden` в `src/cmd/ladix/trigger_golden_test.go`: ДВА fire → ДВЕ explain-строки.

### MUST UPDATE — eval window golden

- [ ] T017 [US1] Обновить `TestWindowMetricTriggerFires` в `src/internal/eval/metric_window_golden_test.go`: explain ДОЛЖЕН идти в `out` (не в `w`); обновить ожидаемый `out` (было `"оконная метрика: 23\n"`) добавив run-explain строку §C-5.3; проверка `stubs.Len()==0` ОСТАЁТСЯ (explain не в `w`). Это подтверждает выбор писателя в T007.

**Checkpoint**: все 8 обновлённых golden зелёные; `cd src && go test ./...` зелёный целиком.

---

## Phase 5: Guards & Registry Invariants (Polish / Cross-Cutting)

**Purpose**: доказать, что НЕ затронутые golden целы и инварианты вехи держатся.

- [ ] T018 [US1] GUARD §C-5.5 «НЕ затронуты»: подтвердить, что НЕ изменены count()/contains()-тесты (`src/internal/daemon/metrics_test.go`, `schedule_test.go`, `daemon_test.go` MFIRE, `m2_endtoend` sink.count) и no-fire/error-тесты (`source_negatives` runtimeForceTrigger, `TestWindowMetricTriggerSilent`, events-FIFO `want=A\nB\nC`). Прогнать их → зелёные без правок; `git diff --stat` НЕ показывает этих файлов (0 правок). `inspect.go` не тронут. Per contracts/golden-churn.md.
- [ ] T019 GUARD реестров (§C-6/§C-7): `ProcessRuntime` = ровно 8 методов байт-цел (`grep` сигнатур `src/internal/eval/runtime.go`; `EvalMetricCondition` НЕ в интерфейсе — свободная функция); `Store` = 18 (ДВОЙНОЙ compile-замок `store.go` цел); прод-дифф `internal/store` и `internal/engine` ПУСТОЙ (`git diff --stat` — только `eval`/`daemon`/golden); схема БД без изменений.
- [ ] T020 GUARD «нет новых кодов/зависимостей»: 0 новых SE/eval-кодов (explain — не ошибка), 0 новых KW/builtins, 0 новых зависимостей (`go.mod` без изменений). inversion-замок T006(c) кусает (снять протяжку порога → `TestServeTriggerExplain` краснеет).
- [ ] T021 Финальный гейт (DoD, quickstart.md): `cd src && go build ./...` OK; `go vet ./...` без замечаний; `gofmt -l` чисто; `go test ./...` зелёный целиком (вкл. 8 обновлённых golden + новые замки T005/T006); детерминизм explain подтверждён (тот же снимок/порог → та же строка). Зафиксировать прохождение в комментарии/леджере вехи.

---

## Dependencies & Execution Order

- **Phase 1 (T001-T002)** → перед всем. T002 [P] с T001.
- **Phase 2 (T003-T004)** БЛОКИРУЕТ serve-печать (T009) и serve-замки (T006): порог должен возвращаться до его печати. T004 зависит от T003 (тот же контракт).
- **Phase 3 (US1)**:
  - Замки T005 (eval) и T006 (daemon) — [P] между собой (разные файлы).
  - T007 (run-печать) зависит от T002/T008; T009 (serve-печать) зависит от T003/T004/T008.
  - T008 (хелпер формата) — перед T007/T009 (общий формат).
- **Phase 4 (T010-T017)** ПОСЛЕ того как печать включена (T007/T009): golden обязаны отражать новую строку. T010-T016 [P] (разные тесты/файлы); T017 зависит от выбора писателя в T007.
- **Phase 5 (T018-T021)** — финальные guard'ы, после co-land.

### Параллельные возможности

- T010-T016 [P] — независимые обновления golden в `daemon/tick_test.go` и `cmd/ladix/trigger_golden_test.go`.
- T005 [P] T006 — разные пакеты (eval vs daemon).

## Implementation Strategy

- **MVP = US1 целиком** (единственная история). Поставка атомарна: печать + co-land golden + guard'ы в одной ветке (always-on explain нельзя влить без co-land — иначе гейт красный).
- Порядок: Foundational (порог) → печать + новые замки → golden co-land → guard'ы/гейт.

## Coverage Map (требования → задачи)

| Требование | Задачи |
|---|---|
| 1. EvalMetricCondition widening + call-site | T003, T004 |
| 2. run explain print (i.out) | T007 (+T008 формат) |
| 3. serve explain print (d.logf, ребро) | T009 (+T008 формат) |
| 4. новые замки (run/serve/silence/inversion) | T005, T006 |
| 5. §C-5.5 co-land 8 goldens | T010, T011, T012, T013, T014, T015, T016, T017 |
| 6. NOT-affected guard | T018 |
| 7. registry invariants / детерминизм / гейт | T019, T020, T021 |

**Итого: 21 задача (T001–T021), 5 фаз.** Все 8 §C-5.5 goldens перечислены явно (T010–T017); NOT-affected guard явный (T018); EvalMetricCondition widening (T003/T004); run/serve explain locks (T005/T006/T007/T009).
