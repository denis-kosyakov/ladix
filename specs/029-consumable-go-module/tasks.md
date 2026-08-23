# Tasks: Потребляемый Go-модуль LADIX

**Feature**: `029-consumable-go-module` | **Branch**: `029-consumable-go-module`
**Input**: [spec.md](./spec.md), [plan.md](./plan.md) (ревизия под путь B), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/](./contracts/), [quickstart.md](./quickstart.md)

**Путь B (Clarifications 2026-06-29)**: из `internal/` НЕ выносится НИЧЕГО; публичная поверхность —
РОВНО два НОВЫХ пакета `ladix` (фасад) + `ir` (контракт вывода). Массовой правки импорт-путей нет.

**Формат пути**: все пути ниже — **относительно корня репозитория ПОСЛЕ схлопывания `src/` → корень**
(T001). До T001 те же файлы живут под `src/`.

---

## Phase 1: Setup — схлопывание `src/` → корень

**Цель**: `go.mod` в корне, module-path стабилен, дерево компилируется, весь корпус зелёный.

- [x] T001 Схлопнуть `src/` в корень репозитория: `git mv src/go.mod src/go.sum src/cmd src/internal .` (и удалить `src/.gitkeep`), сохранив module-path `github.com/denis-kosyakov/ladix` **без** сегмента `/src`; import-пути `internal/*` и `cmd/*` НЕ править — они считаются от module-path (FR-001/FR-002/FR-003)
- [x] T002 Слить `src/README.md` и `src/AGENTS.md` в корневые аналоги (раздел «раскладка модуля») и удалить `src/README.md`, `src/AGENTS.md`; удалить артефакт сборки `src/ladix` (бинарник) — он не часть исходников
- [x] T003 ~~Понизить go-директиву до `1.23` (FR-011)~~ → **ОТМЕНЕНО** решением владельца 2026-08-23: `modernc.org/sqlite v1.52.0` объявляет `go 1.25.0`, директива в модуле одна на всех → `go 1.23` невозможен без отката sqlite на v1.38.0. Директива в `go.mod` остаётся `1.25.0`; запись внесена в Complexity Tracking плана
- [x] T004 Починить тест-хелперы путей после схлопывания (FR-014): `cmd/ladix/metric_test.go` — `repoRoot()` `../../..` → `../..` и `metricFixture()` `../../../src/internal/eval/testdata/...` → `../../internal/eval/testdata/...`; `cmd/ladix/main_test.go` — `examplePath()` `../../../examples` → `../../examples`; `internal/eval/metric_engine_test.go` — `salesPath()` `../../../examples/data/sales.json` → `../../examples/data/sales.json`
- [x] T005 Обновить комментарий-ориентир `cmd/ladix/golden_test.go:98` (`../../../examples/метрики.ladix` → `../../examples/метрики.ladix`) — текст комментария, поведение не меняется
- [x] T006 Верификационный гейт Setup: из **корня** прогнать `go build ./... && go vet ./... && go test ./...` — все три зелёные, ни один тест не удалён (FR-013, A8/A9)

**Checkpoint**: репозиторий — Go-модуль в корне; `go get github.com/denis-kosyakov/ladix` физически возможен; регрессии нет.

---

## Phase 2: Foundational — канонизаторы для понижения в IR

**Цель**: дать слою понижения `ast` → `ir` тотальные канонизаторы выражений и операторов.
**БЛОКИРУЕТ** Phase 3 (без них `ir.Metric.Where` / `ir.Step.Actions` нечем заполнить).

- [ ] T007 [P] Экспортировать канонизатор выражений: в `internal/ast/canon.go` добавить `func CanonicalExpression(e Expression) string` — тонкая публичная (внутри модуля) обёртка над существующим `canonExpr`, с `nil`-гардом (`e == nil` → `""`, отсутствующий необязательный атрибут метрики/шага); сам `canonExpr` НЕ трогать (FR-015, поведение `CanonicalTriggerCondition` байт-в-байт цело)
- [ ] T008 [P] Тест-замок исчерпываемости и nil-гарда `CanonicalExpression` в `internal/ast/canon_test.go`: все 19 видов `Expression` дают ту же строку, что и через `CanonicalTriggerCondition`; `CanonicalExpression(nil) == ""`; мутпроба — снятие nil-гарда роняет тест паникой
- [ ] T009 Новый тотальный канонизатор операторов `internal/ast/canon_stmt.go`: `func CanonicalStatement(s Statement) string` — рекурсивный type-switch по ВСЕМ 13 конкретным типам `Statement` (`LetStmt`, `AssignStmt`, `ExpressionStmt`, `IfStmt`, `WhileStmt`, `TryStmt`, `ForStmt`, `ReturnStmt`, `BreakStmt`, `ContinueStmt`, `AssignAction`, `CallAction`, `NotifyAction`) с **ГРОМКИМ default-panic** (дисциплина `canonExpr`, Конституция III), переиспользующий `canonExpr` для выражений; вложенные `*Block`/`*ElseClause` канонизируются рекурсивно
- [ ] T010 Тест-замок T-CANON-STMT в `internal/ast/canon_stmt_test.go`: (а) исчерпываемость — по одному кейсу на каждый из 13 типов, ни один не уходит в default; (б) стабильность канона — table-driven exact-match строк; (в) мутпроба — удаление любой ветки уводит тип в default-панику и краснит тест
- [ ] T011 Верификационный гейт Foundational: `go test ./internal/ast/...` зелёный; `go vet ./...` чист

**Checkpoint**: понижение в IR обеспечено тотальными канонизаторами; поведение существующего кода не изменено.

---

## Phase 3: User Story 1 — фасад `ladix.Compile` → `*ir.Program` (Priority: P1) 🎯 MVP

**Goal**: платформа «Уклад» импортирует `ladix` + `ir`, компилирует исходник и получает
версионируемый IR либо дословные русские диагностики — не затягивая SQLite и backend.

**Independent Test**: из теста корневого пакета вызвать `ladix.Compile` на валидном и невалидном
исходнике (проверить `SchemaVersion == 1`, инвариант `program != nil ⟺ нет error-диагностик`,
дословность `Message` и заполненность `Pos`), `ladix.CompileFile` на файле и на несуществующем пути;
отдельно — страж границы доказывает изоляцию `sqlite`/`internal/{store,engine,daemon}`.

### Тесты (tests-first, Конституция VI)

- [ ] T012 [P] [US1] Тест-замки формы пакета `ir` в `ir/ir_test.go`: `SchemaVersion == 1`; JSON-теги всех полей `snake_case` (рефлексия по `Program`/`Metric`/`Process`/`Step`/`Trigger`/`Diagnostic`/`Position`); незаданные строковые поля сериализуются как `""`, не опускаются (`omitempty` ЗАПРЕЩЁН) — контракт `contracts/ir-schema.md`, FR-006/A11
- [ ] T013 [P] [US1] Golden-замок JSON round-trip `ir` в `ir/json_golden_test.go` (FR-021): фикстура `Program` (одна метрика + один процесс с шагом + один метрик-триггер, как в `contracts/ir-schema.md`) → `json.Marshal` exact-match с golden-строкой → `json.Unmarshal` обратно → `reflect.DeepEqual` с исходником; отдельный кейс forward-compat — декодирование JSON с **неизвестным полем**, неизвестным `severity` и неизвестным `stage` НЕ падает (FR-020)
- [ ] T014 [P] [US1] Тест-замки фасада в `ladix_test.go`: C1 валидный исходник → `program != nil`, `SchemaVersion == 1`, нет `error`-диагностик, `err == nil`; C2 лексическая / синтаксическая / семантическая ошибка (три отдельных кейса) → `program == nil`, `err == nil`, `≥1` диагностика с `Severity == "error"`, `Stage` из `{"lexical","syntax","semantic"}`, `Message` **дословно** равный `Msg` соответствующей ошибки SPEC §13 и `Pos.Line`/`Pos.Col` ≥ 1; C3 `CompileFile` несуществующего пути → `err != nil`, `program == nil`; C3' `CompileFile` реального `examples/*.ladix` эквивалентен `Compile` над его содержимым (FR-005/FR-007, A1/A2/A3)
- [ ] T015 [P] [US1] Тест-замок понижения в `ladix_lower_test.go`: исходник с метрикой, процессом (шаг с `после`/`исполнитель`/`срок`/телом) и всеми четырьмя видами триггера (`metric`/`schedule`/`event`/`deadline`) → проверить заполнение полей `ir.Metric`/`ir.Process`/`ir.Step`/`ir.Trigger`, каноничность строк выражений и корректность `Pos` каждого узла (1-based, руны — Конституция IV)
- [ ] T016 [US1] Страж границы `boundary_test.go` в корневом пакете (FR-009/FR-010, контракт `contracts/import-boundary.md`): через `os/exec` → `go list -deps <pkg>` для `github.com/denis-kosyakov/ladix` и `.../ir`; **T1** — замыкание `ladix` НЕ содержит `modernc.org/sqlite`; **T2** — замыкание `ir` НЕ содержит `modernc.org/sqlite`, `internal/{store,engine,daemon}` И `internal/eval` (строгое минимальное замыкание листа); **T3** — ни `ladix`, ни `ir` не содержат `internal/{store,engine,daemon}`; `internal/eval`/`internal/jsonval` в замыкании `ladix` ЯВНО ДОПУСТИМЫ; при недоступности тулчейна `go` — `t.Skip`, не false-green

### Реализация

- [ ] T017 [US1] Пакет `ir/ir.go` (лист; НЕ импортирует `ast`/`errors`/`value` — Конституция VII): `const SchemaVersion = 1`; типы `Program`, `Metric`, `Process`, `Step`, `Trigger`, `Position`, `Diagnostic` с JSON-тегами `snake_case` строго по `contracts/ir-schema.md`; словарь `Severity` v1 = `{"error"}`, `Stage` = `{"lexical","syntax","semantic"}` — объявить как именованные строковые константы (`SeverityError`, `StageLexical`, `StageSyntax`, `StageSemantic`), НЕ как enum-тип (forward-compat FR-020: потребитель обязан толерировать неизвестные значения)
- [ ] T018 [US1] Документирующий docstring пакета `ir/doc.go`: назначение (версионируемый контракт вывода), политика `SchemaVersion` (что breaking, что аддитивно — FR-016), требование толерантности потребителя к неизвестным `severity`/`stage`/полям (FR-020)
- [ ] T019 [US1] Слой понижения `lower.go` (пакет `ladix`): `lowerProgram(*ast.Program) *ir.Program` — обход `prog.Items`, `*ast.MetricDecl` → `ir.Metric` (`Where`/`Aggregate`/`Period`/`ByDate` через `ast.CanonicalExpression`), `*ast.ProcessDecl` → `ir.Process` (+`Steps` → `ir.Step`: `After` = имена через `, `, `Assignee`/`Deadline` каноничны, `Actions` = `ast.CanonicalStatement` по каждому оператору `Body`), `*ast.TriggerDecl` → `ir.Trigger` с type-switch по `Spec` (`*ast.MetricTrigger` → `kind:"metric"`, `*ast.ScheduleTrigger` → `kind:"schedule"` + `Schedule` = канон подформы, `*ast.EventTrigger` → `kind:"event"`, `*ast.DeadlineTrigger` → `kind:"deadline"` + `Process`/`Step`); `ast.Position` → `ir.Position` покомпонентно (дубль типа, не разделение — Конституция IV); неприменимые к `kind` поля остаются `""`
- [ ] T020 [US1] Конвертер диагностик `diagnostics.go` (пакет `ladix`): `errors.LexError` → `Stage: "lexical"`, `errors.ParseError` → `"syntax"`, `errors.СемантическаяОшибка` → `"semantic"`; `Message` берётся из поля `Msg` (описание БЕЗ двухстрочного заголовка §13 — позиция едет отдельным полем `Pos`), переформулирование ЗАПРЕЩЕНО (FR-007, Конституция VIII); распаковка `*errors.ErrorList` через `Errors()`; `Severity` всегда `"error"` (v1)
- [ ] T021 [US1] Фасад `ladix.go` (корневой пакет `ladix`): `Compile(source string) (*ir.Program, []ir.Diagnostic, error)` — `lexer.New(source).Tokenize()` → `parser.New(toks, errs).Parse()` → при непустом `errs` вернуть `(nil, diags, nil)` **без** семантического прохода → иначе `eval.NewInterpreter(io.Discard, defaultDepth, systemClock).Analyze(prog)`; ошибка `Analyze` → `(nil, diags, nil)`; успех → `(lowerProgram(prog), nil, nil)`. Никаких пакетных изменяемых `var` (Конституция V): интерпретатор создаётся на каждый вызов. `defer recover()`-барьер превращает панику фронтенда в `err != nil` (C4), а не в краш потребителя
- [ ] T022 [US1] `CompileFile(path string) (*ir.Program, []ir.Diagnostic, error)` в `ladix.go`: `os.ReadFile(path)` → при IO-ошибке `(nil, nil, err)` **ДО** компиляции; иначе `Compile(string(b))` (C3)
- [ ] T023 [US1] Docstring корневого пакета в `doc.go`: назначение (узкая точка входа библиотеки), инвариант `program != nil ⟺ нет error-диагностик`, различение «пользовательская ошибка → `diags`» vs «внутренний сбой → `err`», указание что `lexer`/`parser`/`ast`/`value`/`errors`/`eval` — `internal` и частью semver-контракта НЕ являются

**Checkpoint US1**: `go test ./... ` зелёный; фасад и `ir` работают; страж границы красный при протечке `store`/`sqlite`.

---

## Phase 4: User Story 2 — `go get` с корня, module-path стабилен (Priority: P1)

**Goal**: внешний разработчик выполняет `go get github.com/denis-kosyakov/ladix@v0.1.0` и получает рабочий модуль.

**Independent Test**: `go.mod` в корне; `module` == `github.com/denis-kosyakov/ladix` без `/src`;
директива `go` == `1.23`; `go build ./...` из корня зелёный.

- [ ] T024 [US2] Тест-замок раскладки модуля в `module_layout_test.go` (корневой пакет): прочитать `go.mod` (файл рядом с тестом), проверить `module github.com/denis-kosyakov/ladix` **без** сегмента `/src` (A5/A6); директиву `go` сверить с порогом, который называет README (A7 в редакции Complexity Tracking: директива `1.25.0`, не `1.23`) — замок краснеет при откате переезда или при рассинхроне go-директивы с документацией
- [ ] T025 [US2] Проверить отсутствие остаточных ссылок на путь `src/` в конфигурации сборки и хелперах: греп `grep -rn "src/internal\|src/cmd\|src/go.mod" --include="*.go" --include="*.md" .` — в коде 0 попаданий (в `specs/001…028` исторические ссылки ДОПУСТИМЫ и НЕ правятся)

**Checkpoint US2**: модуль подключаем снаружи; версия Go согласована.

---

## Phase 5: User Story 3 — standalone reference-implementation зелён (Priority: P2)

**Goal**: CLI, движок процессов, SQLite-стор и демон целы; ~25000 LOC тестов проходят из корня.

**Independent Test**: `go build ./... && go vet ./... && go test ./...` из корня; ни один тест не удалён.

- [ ] T026 [US3] Регресс-гейт: из корня `go build ./... && go vet ./... && go test ./...` — зелёные (A8); зафиксировать в отчёте число прогнанных пакетов
- [ ] T027 [US3] Замок сохранности раскладки в `module_layout_test.go`: пакеты `lexer`, `parser`, `ast`, `value`, `errors`, `eval`, `engine`, `store`, `daemon`, `jsonval` физически присутствуют под `internal/`, а CLI — в `cmd/ladix` (A10, FR-003/FR-004); краснеет при случайном выносе пакета наружу
- [ ] T028 [US3] Замок сохранности тестового корпуса: сверить `git diff --stat master...HEAD -- '*_test.go'` — ни один существующий тест-файл не удалён и не усечён (FR-013); удаление кейса допускается только вместе с обоснованием в отчёте (ожидается: НЕТ удалений)
- [ ] T029 [US3] Smoke reference-CLI из корня: `go run ./cmd/ladix run examples/метрики.ladix` (или эквивалентный пример) отрабатывает с exit 0 — доказательство, что переезд не сломал резолв `examples/` (фича 026, file-relative)

**Checkpoint US3**: рефактор доказуемо ничего не сломал.

---

## Phase 6: User Story 4 — версионирование и первый релиз (Priority: P3)

**Goal**: политика semver задокументирована; репозиторий готов к тегу `v0.1.0`.

**Independent Test**: `ir.SchemaVersion == 1`; политика semver и аддитивности задокументирована.

- [ ] T030 [US4] Раздел «Версионирование и совместимость» в `README.md`: semver `vX.Y.Z`; первый тег `v0.1.0`; изменение сигнатур `Compile`/`CompileFile` = MAJOR; аддитивность языка не меняет `SchemaVersion`; breaking-изменение формата IR (удаление/переименование поля, смена типа/семантики) = bump `ir.SchemaVersion`; новое опциональное поле / новое значение `Severity`/`Stage` — аддитивно (FR-016/FR-020, A12/A13)
- [ ] T031 [US4] Зафиксировать в `README.md` (или `docs/`) требование forward-compat к потребителю IR: толерантность к неизвестным JSON-полям и неизвестным `severity`/`stage` — не падать, трактовать неизвестный `severity` как ≥информативный (FR-020)

**Checkpoint US4**: политика версий явная; тег `v0.1.0` = операция публикации (вне диффа кода).

---

## Phase 7: Polish — doc-sync и приёмка

- [ ] T032 [P] `ARCHITECTURE.md`: §2 таблица пакетов (10 `internal/*` + 2 публичных `ladix`/`ir`), §2.1 граф/листья (`value`/`errors`/`ast`/`ir`), §4.1 `Position` — добавить `ir.Position` как третий локальный дубль; зафиксировать инвариант изоляции `sqlite` от публичного замыкания
- [ ] T033 [P] `README.md`: раскладка модуля (`go.mod` в корне), все команды запускаются **из корня** (не из `src/`), go-версия `1.23`, раздел «Подключение как библиотеки» с примером `ladix.Compile` из `quickstart.md`
- [ ] T034 [P] `AGENTS.md` и корневой `CLAUDE.md`-совместимый раздел: пути пакетов и раскладка после схлопывания `src/` → корень
- [ ] T035 [P] `docs/*-model.md`: механическая правка ссылок на пути пакетов (`src/internal/...` → `internal/...`); **тексты диагностик и семантика НЕ трогаются** (FR-015)
- [ ] T036 [P] Новый `docs/module-contract.md` (или раздел в существующем): публичная поверхность = `ladix` + `ir`; контракт фасада; схема IR v1; политика `SchemaVersion`; инвариант границы и как краснеет страж — источник истины для будущих фич (Конституция IX)
- [ ] T037 Чеклист «контракта апстрима» (FR-018): зафиксировать в `specs/029-consumable-go-module/` (или `docs/`), что контракт с платформой «Уклад» принят со слов владельца (вариант B) и рекомендуется закрепить документом на стороне Уклада (`docs/ladix/integration_v4.md`) — вне диффа данной фичи
- [ ] T038 Прогнать `quickstart.md` целиком как приёмку: Раздел 1 (потребитель: `go get`, пример `Compile`, `go list -deps | grep -c modernc.org/sqlite` == 0), Раздел 2 (мейнтейнер: сборка/тесты из корня), Раздел 3 (чеклист SC-001…SC-009) — все пункты зелёные
- [ ] T039 Финальный гейт: `gofmt -l .` пуст, `go vet ./...` чист, `go test ./...` зелёный из корня; греп SC-проверок: 0 новых внешних зависимостей в `go.mod` (FR-019), 0 новых ключевых слов/встроенных/операторов/кодов eval (диффа в `internal/lexer`, `internal/parser` нет, кроме нуля)

---

## Dependencies

```text
Phase 1 (T001–T006)  ──БЛОКИРУЕТ ВСЁ──►  (без корневого модуля новый код некуда класть)
        │
        ▼
Phase 2 (T007–T011)  ──БЛОКИРУЕТ──►  Phase 3 (понижение нечем заполнять)
        │
        ▼
Phase 3 US1 (T012–T023)  ◄── MVP; T012–T016 (тесты) ПЕРЕД T017–T023 (реализация)
        │                      T017 (ir) ПЕРЕД T019/T020/T021 (используют ir.*)
        │                      T019+T020 ПЕРЕД T021 (фасад их зовёт); T021 ПЕРЕД T022
        ▼
Phase 4 US2 (T024–T025)  ── независима от US1, зависит только от Phase 1
Phase 5 US3 (T026–T029)  ── проверяет результат Phase 1 + Phase 3
Phase 6 US4 (T030–T031)  ── зависит от T017 (SchemaVersion существует)
        ▼
Phase 7 (T032–T039)  ── doc-sync и приёмка последними (документируют финальную раскладку)
```

**Порядок сдачи по приоритетам**: US1 (P1) + US2 (P1) — MVP; US3 (P2) — гарантия безопасности
рефактора; US4 (P3) — готовность к релизу.

## Parallel Execution

- **Phase 2**: T007 и T008 (`canon.go` + его тест) параллельны T009/T010 (`canon_stmt.go` + тест) — разные файлы.
- **Phase 3, тесты**: T012, T013, T014, T015 — разные файлы, пишутся параллельно (T016 отдельно — зависит от существования пакетов).
- **Phase 7**: T032–T036 — разные документы, параллельны.
- **Не параллелить**: T001–T006 (последовательное схлопывание с гейтом), T017→T019/T020→T021→T022.

## Implementation Strategy

**MVP = Phase 1 + Phase 2 + Phase 3 (US1) + Phase 4 (US2)** — этого достаточно, чтобы Уклад
подключил модуль и получал IR. Phase 5 — обязательный регресс-гейт перед мержем. Phase 6/7 —
готовность к публикации и синхронизация источников истины.

**Инкрементальная сдача**: каждый Checkpoint — точка, в которой `go build`/`go vet`/`go test`
из корня зелёные. Коммит на каждый Checkpoint, а не на каждую задачу.

## Format Validation

Все 39 задач соответствуют формату `- [ ] TID [P?] [Story?] описание с путём к файлу`:
чекбокс — есть; ID T001–T039 — сквозные в порядке исполнения; `[P]` — только на задачах в
разных файлах без зависимостей; `[US1]`–`[US4]` — только в фазах пользовательских историй
(Setup/Foundational/Polish — без метки); путь к файлу — в каждой задаче.
