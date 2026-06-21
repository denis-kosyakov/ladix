---
description: "Декомпозиция задач — фича 026-source-path-resolution"
---

# Tasks: Разрешение путей источников относительно каталога программы

**Input**: Design documents from `/specs/026-source-path-resolution/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/cli-source-base.md, quickstart.md

**Tests**: включены (Принцип VI — tests-first; каждый замок обязан КРАСНЕТЬ при инверсии фикса).

**Все go-команды — из `src/`.** Дифф СТРОГО в `src/internal/eval` + `src/cmd/ladix` + `examples/` + `docs`.
ПУСТОЙ дифф `internal/{store,engine,daemon}`; Store 18 цел; 0 новых зависимостей; golden-байты stdout неизменны.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: можно параллельно (разные файлы, нет зависимостей)
- **[Story]**: US1/US2/US3 — к какой пользовательской истории относится задача

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: точка отсчёта регресса и переезд витрины данных.

- [ ] T001 Зафиксировать зелёный старт: из `src/` выполнить `go build ./... && go vet ./... && go test ./...` — база регресса (golden-байты до изменений).
- [ ] T002 `git mv data/ examples/data/` (5 файлов: `sales.json`, `orders.csv`, `orders.json`, `orders.ndjson`, `costs.json`); пути `"data/..."` в `examples/*.ladix` НЕ трогать. Примечание: дерево станет транзиентно-красным до Phase 2–3 — коммитить общим логическим блоком с резолвером и обновлением eval-тестов.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: механизм резолва — общий фундамент всех историй. Обрабатывает file-relative (US1), override-инвариант (US2) и абсолютный путь + диагностику (US3).

**⚠️ CRITICAL**: ни одна история не может быть реализована до завершения этой фазы.

- [ ] T003 [P] eval: добавить поле `sourceBase string` в struct `Interpreter` + сеттер `SetSourceBase(dir string)` (зеркало `SetProcessRuntime`) в `src/internal/eval/interpreter.go`. `NewInterpreter` НЕ менять.
- [ ] T004 eval (тест-замок, TDD): добавить table-driven `TestResolveSourcePath` в `src/internal/eval/source_loader_test.go` — кейсы: абсолютный путь как есть (база игнорируется); относительный от заданной базы; относительный при пустой базе (≡ cwd). Замок не компилируется/падает до T005. Мутпроба (вернуть `decl.File.Value` без `Join`/`IsAbs`) → RED (research.md).
- [ ] T005 eval: в `src/internal/eval/source_loader.go` добавить `import "path/filepath"`; добавить метод `resolveSourcePath(p string) string` (`if filepath.IsAbs(p) { return p }; return filepath.Join(i.sourceBase, p)`); заменить `path := decl.File.Value` на `path := i.resolveSourcePath(decl.File.Value)` в loadJSON (~:68), loadCSV (~:241), loadNDJSON (~:321). Текст/код ошибки не менять (резолвленный путь подставляется автоматически, FR-008). (зависит: T003, T004)

**Checkpoint**: резолвер готов; `TestResolveSourcePath` зелёный; существующие тесты с дефолтной базой `""` не изменились.

---

## Phase 3: User Story 1 — Запуск программы из любого каталога (Priority: P1) 🎯 MVP

**Goal**: относительный путь источника резолвится от каталога `.ladix`-файла; примеры запускаются по пути к программе из любого cwd.

**Independent Test**: `ladix run <abs>/examples/выручка.ladix` из `t.TempDir()` → exit 0; golden-байты примеров неизменны.

### Tests for User Story 1 ⚠️ (написать ПЕРЕД impl, убедиться что краснеют)

- [ ] T006 [P] [US1] eval: переписать `TestLoadSourceSalesJSON` в `src/internal/eval/source_loader_test.go` под `examples/data/sales.json` + явный `SetSourceBase` (база — каталог `examples` относительно пакета), без зависимости от cwd запуска `go test`. Мутпроба (резолвер игнорирует базу) → RED.
- [ ] T007 [P] [US1] cmd/ladix: переработать `withRepoRoot` (`metric_test.go` ~:29) и все 14 call-sites — убрать `chdir`, передавать в CLI **абсолютный** путь примера (`filepath.Abs(examplePath(...))`). Файлы: `metric_test.go`, `control_plan_golden_test.go`, `metric_window_golden_test.go`, `golden_test.go`, `trigger_golden_test.go`, `main_test.go`. golden-байты stdout НЕ меняются. (До правки прод-кода эти тесты должны краснеть: данные переехали в `examples/data`, chdir убран.)
- [ ] T008 [US1] cmd/ladix: новый интеграционный замок `TestRunRevenueAbsolutePathFromTempDir` в `src/cmd/ladix/main_test.go` — `chdir` в `t.TempDir()`, `realMain(["run", <abs>/examples/выручка.ladix])` → exit 0 (источник найден). Инверсия (cwd-резолв) → RED.

### Implementation for User Story 1

- [ ] T009 [US1] cmd/ladix `main.go`: `import "path/filepath"`; в `runFile` (~:254), `runMetric` (~:313), `completeTask` (~:457) вычислить `base := filepath.Dir(programPath)` и вызвать `interp.SetSourceBase(base)` сразу после `NewInterpreter`. (зависит: T005)
- [ ] T010 [P] [US1] cmd/ladix `start.go`: `import "path/filepath"`; в `startMain` (~:136) `base := filepath.Dir(programPath)`; `interp.SetSourceBase(base)` после `NewInterpreter`. (зависит: T005)
- [ ] T011 [P] [US1] cmd/ladix `serve.go`: `import "path/filepath"`; пробросить `base := filepath.Dir(programPath)` из `serveFile`/`serveMain` в `buildServeDaemon` (~:300); `interp.SetSourceBase(base)` после `NewInterpreter`. (зависит: T005)

**Checkpoint**: примеры с источниками работают по абсолютному пути из любого cwd; `go test ./...` зелёный; golden неизменны. **MVP готов.**

---

## Phase 4: User Story 2 — Переопределение базы флагом `--source-base` (Priority: P2)

**Goal**: `--source-base <dir>` (обе формы) во всех 5 подкомандах переопределяет каталог программы; пропуск значения → exit 2.

**Independent Test**: подкоманда с `--source-base B` читает источник из B; `--source-base` без значения → exit 2 + RU-stderr.

### Tests for User Story 2 ⚠️

- [ ] T012 [P] [US2] cmd/ladix: `TestSourceBaseFlagOverride` в `src/cmd/ladix/` — `--source-base=B` и `--source-base B` дают базу B поверх каталога программы (источник читается из B). Инверсия (флаг игнорируется) → RED.
- [ ] T013 [P] [US2] cmd/ladix: `TestSourceBaseFlagMissingValue` в `src/cmd/ladix/` — `--source-base` без значения → exit 2 + stderr `ladix: флаг --source-base требует значение` (репрезентативно для run + ещё одной подкоманды). Зеркало существующих флаг-тестов.

### Implementation for User Story 2

- [ ] T014 [US2] cmd/ladix `main.go`: разбор `--source-base` (зеркало `--db`, ~:137-145) в run/metric/complete — `case a=="--source-base"` (k+1 проверка → stderr+return 2; `k++`) + `case strings.HasPrefix(a,"--source-base=")` (`TrimPrefix`); база `:= sourceBaseFlag; if base=="" { base = filepath.Dir(programPath) }`. (зависит: T009)
- [ ] T015 [P] [US2] cmd/ladix `start.go`: тот же разбор `--source-base` + приоритет флага над `filepath.Dir`. (зависит: T010)
- [ ] T016 [P] [US2] cmd/ladix `serve.go`: тот же разбор `--source-base` + приоритет флага над `filepath.Dir`. (зависит: T011)

**Checkpoint**: US1 + US2 работают; все 5 подкоманд принимают `--source-base` в обеих формах; пропуск значения → exit 2.

---

## Phase 5: User Story 3 — Абсолютные пути и диагностика (Priority: P3)

**Goal**: абсолютный путь источника читается как есть независимо от базы; ошибка «файл не найден» показывает резолвленный путь.

**Independent Test**: источник с абсолютным путём читается при любой базе; несуществующий файл → ошибка с резолвленным путём.

### Tests for User Story 3 ⚠️ (impl уже в foundational — резолвер обрабатывает абсолют + текст ошибки)

- [ ] T017 [P] [US3] eval: подтвердить кейс абсолютного пути в `TestResolveSourcePath` (абсолют игнорирует непустую базу) — если не покрыт T004, добавить строку таблицы в `src/internal/eval/source_loader_test.go`.
- [ ] T018 [P] [US3] cmd/ladix или eval: `TestSourceFileNotFoundShowsResolvedPath` — относительный путь + несуществующий файл при заданной базе → ошибка содержит **резолвленный** путь (`<base>/data/...`), категория/код целы. Инверсия (показывать сырой `decl.File.Value`) → RED.

**Checkpoint**: все истории независимо функциональны.

---

## Phase 6: Polish & Cross-Cutting (doc-sync + регресс-гейты)

**Purpose**: синхронизация канонов под новую семантику и финальные барьеры.

- [ ] T019 [P] doc: `docs/source-metric-model.md` §SM-8.1 (~:324) + ~:462 — «относительно cwd» → «относительно каталога `.ladix`-файла; `--source-base` переопределяет; абсолютный — как есть». (источник истины канона)
- [ ] T020 [P] doc: `SPEC.md` §9.1 «Разрешение пути» (~:406) + ~:462 + ~:601 — то же; убрать «базовый каталог-конфиг — v2».
- [ ] T021 [P] doc: `README.md` — строки ~:37, ~:131, ~:244 и раздел «Примечание о путях к источникам» (~:366-374): переписать под file-relative; УДАЛИТЬ «отложен в v2»; убрать «запускать из корня репо»; отразить `examples/data/`.
- [ ] T022 [P] doc: `examples/MANIFEST.md` — ~:26, ~:67, ~:94, ~:144-145, ~:184, ~:241, ~:267: убрать «из корня репо», отразить самодостаточность `examples/` + путь `examples/data/`; обновить упоминание `withRepoRoot`.
- [ ] T023 [P] doc: `specs/004-source-metric/quickstart.md` — ~:63, ~:67, ~:75, ~:99: обновить/пометить под новую семантику.
- [ ] T024 grep-гейт (SC-004): подтвердить, что в `docs/`, `SPEC.md`, `README.md`, `examples/MANIFEST.md`, `specs/004-source-metric/` НЕТ «cwd»/«из корня репо»/«отложен в v2» применительно к путям источников.
- [ ] T025 Финал: из `src/` `go build ./... && go vet ./... && go test ./...` зелёные; пройти `quickstart.md` (запуск примеров из постороннего cwd); обновить агент-контекст `CLAUDE.md` (ссылка на план → 026). Подтвердить пустой дифф `internal/{store,engine,daemon}` и неизменность golden-байтов.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: старт. T002 (git mv) логически коммитится вместе с Phase 2–3.
- **Foundational (Phase 2)**: зависит от Setup; БЛОКИРУЕТ все истории (T003→T005, T004 перед T005).
- **US1 (Phase 3)**: после Foundational. T006/T007/T008 (тесты) перед/вместе с T009-T011 (impl).
- **US2 (Phase 4)**: после US1 (CLI-точки уже модифицированы): T014⇐T009, T015⇐T010, T016⇐T011.
- **US3 (Phase 5)**: после Foundational (impl абсолюта/диагностики уже там); только тест-замки.
- **Polish (Phase 6)**: после реализации; doc-sync [P] параллелен, T024/T025 — финальные гейты.

### User Story Dependencies

- **US1 (P1)**: фундамент → MVP. Не зависит от US2/US3.
- **US2 (P2)**: расширяет CLI-точки US1 (тот же switch/строка базы) — последовательно после US1 в одних файлах.
- **US3 (P3)**: independent; реализация в Foundational, фаза добавляет только замки.

### Within Each User Story

- Тесты пишутся и краснеют ДО impl.
- eval-механизм (Foundational) до CLI-проброса (US1) до флага (US2).
- Один файл — одна задача за раз (CLI-файлы `main.go`/`serve.go`/`start.go` правятся US1→US2 последовательно).

### Parallel Opportunities

- T003 ‖ (T004 пишется параллельно как замок).
- US1-тесты: T006 (eval) ‖ T007 (cmd) ‖ T008 (cmd, разные функции).
- US1-impl: T010 (start.go) ‖ T011 (serve.go) после T009-паттерна (разные файлы).
- US2-impl: T015 (start.go) ‖ T016 (serve.go).
- Phase 6 doc-sync: T019‖T020‖T021‖T022‖T023 (разные файлы).

---

## Parallel Example: doc-sync (Phase 6)

```bash
Task: "docs/source-metric-model.md §SM-8.1"
Task: "SPEC.md §9.1"
Task: "README.md «Примечание о путях»"
Task: "examples/MANIFEST.md"
Task: "specs/004-source-metric/quickstart.md"
```

---

## Implementation Strategy

### MVP First (US1)

1. Phase 1 Setup (T001-T002) → 2. Phase 2 Foundational (T003-T005) → 3. Phase 3 US1 (T006-T011)
4. **STOP & VALIDATE**: примеры запускаются по абсолютному пути из любого cwd, golden неизменны.

### Incremental Delivery

1. Foundational → резолвер готов (база `""` = старое поведение, ничего не сломано).
2. US1 → file-relative дефолт + git mv + smoke (MVP).
3. US2 → `--source-base` override.
4. US3 → замки абсолюта/диагностики.
5. Polish → doc-sync + грепы-гейты.

---

## Notes

- [P] = разные файлы, нет зависимостей.
- Каждый замок RED при инверсии фикса (мутпробы — research.md).
- Коммитить логическими группами; per-commit дерево держать собираемым (git mv + резолвер + eval-тесты — один блок).
- Избегать: тронуть `internal/{store,engine,daemon}`; менять сигнатуру `NewInterpreter`; менять golden-байты; вводить env-переменную/stdin/новый синтаксис.
