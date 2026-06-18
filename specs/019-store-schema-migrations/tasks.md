---
description: "Dependency-ordered tasks for 019-store-schema-migrations (C2a forward-only schema migrations)"
---

# Tasks: Каркас миграций схемы Store (forward-only)

**Input**: Design documents from `/specs/019-store-schema-migrations/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md,
contracts/store-schema-migration.md, quickstart.md

**Tests**: ВКЛЮЧЕНЫ — §C-2a.4 прямо требует замки (TestMigrateFreshDB / TestMigrateLegacyV0 /
TestMigrateIdempotent + инверсионная мутпроба) и SC-004. Тесты пишутся перед реализацией миграции
(красные → зелёные).

**Организация**: одна история P1 в одном пакете `src/internal/store/`. Дифф вне пакета store ПУСТОЙ.
Все пути — абсолютные/проектные относительно корня репо `/Users/denis/dev/ladix`.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: можно выполнять параллельно (разные файлы, нет зависимостей от незавершённых задач).
- **[US1]**: задача истории User Story 1 (единственная история фичи).
- Setup / Foundational / Polish — без метки истории.

---

## Phase 1: Setup (инициализация фичи)

- [ ] T001 Проверить предусловия среды: `cd src && go build ./...` зелёный на чистой ветке
  `019-store-schema-migrations` (baseline до изменений) в `src/`; зафиксировать, что
  `user_version`/`migrate`/`schemaMigrations` ОТСУТСТВУЮТ в `src/internal/store/` (grep → 0 совпадений).
- [ ] T002 Свериться с якорем и контрактами: перечитать `specs/019-store-schema-migrations/research.md`,
  `data-model.md`, `contracts/store-schema-migration.md` — зафиксировать точку встройки (после
  `db.Exec(ddl)`-блока, до `return` в `NewSQLiteStore`, `src/internal/store/sqlite.go`) и ДОСЛОВНЫЙ DDL
  ступени 1→2 (таблица `outbox` + индекс `idx_outbox_instance`).

**Checkpoint**: понятна точка встройки, DDL и tx-шаблон `nextCounter` (`src/internal/store/sqlite.go`).

---

## Phase 2: Foundational (блокирующие предпосылки)

> Для этой фичи нет отдельной инфраструктуры до истории: весь каркас миграций И есть содержание истории
> US1. Foundational сводится к подтверждению границ, чтобы не задеть смежные сущности.

- [ ] T003 Подтвердить инварианты границ ПЕРЕД правкой кода (только проверка, без изменений):
  контракт `Store` = 16 методов и двойной compile-замок в `src/internal/store/store.go` остаются
  нетронутыми; `src/internal/store/memory.go` (MemoryStore) НЕ меняется; `src/internal/store/types.go`
  (нет `OutboxRecord`) НЕ меняется. Это страховка FR-010/FR-011 — фиксируется как чек-лист задачи.

**Checkpoint**: границы зафиксированы — приступать к US1.

---

## Phase 3: User Story 1 — Постоянная БД автоматически получает актуальную версию схемы (Priority: P1) 🎯 MVP

**Goal**: forward-only миграция в `SQLiteStore` приводит схему к версии 2 (таблица `outbox` + индекс)
при каждом открытии — для свежей, legacy-v0 и уже-актуальной БД; данные сохраняются; идемпотентно.

**Independent Test**: открыть `NewSQLiteStore` на новом temp-файле → `user_version == 2` и `outbox`
существует; повторить на «старой» схеме с данными → версия→2, `outbox` появилась, данные целы; открыть
актуальную БД повторно → no-op без ошибок.

### Tests for User Story 1 (пишутся ПЕРВЫМИ, должны падать до реализации) ⚠️

- [ ] T004 [P] [US1] Создать `src/internal/store/migrate_test.go` со скелетом + `TestMigrateFreshDB`:
  открыть `NewSQLiteStore` на новом temp-файле (`t.TempDir()`); утверждать `PRAGMA user_version == 2`
  (через `db`-хэндл/повторное соединение к файлу) И существование таблицы `outbox`
  (`SELECT name FROM sqlite_master WHERE type='table' AND name='outbox'`). Контракт A1 · FR-001/FR-004 ·
  SC-001. (Падает до реализации — `outbox` нет, версия 0.)
- [ ] T005 [US1] В `src/internal/store/migrate_test.go` добавить `TestMigrateLegacyV0`: вручную создать
  файл БД, применить базовый `ddl` (или вставить через первый `NewSQLiteStore`), затем выставить
  `PRAGMA user_version = 0`, вставить экземпляр процесса и задачу (через store-методы `SaveInstance`/
  `SaveTask`); закрыть; открыть заново `NewSQLiteStore`; утверждать версия→2, `outbox` появилась,
  ранее вставленные инстанс+задача читаются без изменений (`LoadInstance`/`LoadTask`). Контракт A2 +
  G-A3 · FR-003/FR-008 · SC-002. (Зависит от T004 — тот же файл.)
- [ ] T006 [US1] В `src/internal/store/migrate_test.go` добавить `TestMigrateIdempotent`: открыть
  `NewSQLiteStore` дважды на одном temp-файле; второе открытие — без ошибок; утверждать
  `user_version == 2` и что `outbox` не дублируется (один объект в `sqlite_master`). Контракт A4 +
  B-I4 · FR-007 · SC-003. (Зависит от T004 — тот же файл.)

### Implementation for User Story 1

- [ ] T007 [US1] В `src/internal/store/sqlite.go` добавить константы рядом с `const ddl`/`const pragmas`:
  `const (baselineVersion = 1; currentSchemaVersion = 2)`. Инвариант согласованности
  `currentSchemaVersion == baselineVersion + len(schemaMigrations)` (data-model INV-R1). Контракт C.
- [ ] T008 [US1] В `src/internal/store/sqlite.go` добавить `var schemaMigrations = []string{ … }` с
  ОДНИМ элементом — ступень 1→2, DDL `outbox`+индекс ДОСЛОВНО из
  `contracts/store-schema-migration.md` / `data-model.md` (без отклонений). FR-004. Контракт C.
- [ ] T009 [US1] В `src/internal/store/sqlite.go` реализовать `func migrate(db *sql.DB) error` по
  алгоритму контракта B (по образцу `nextCounter`, sqlite.go:~308–328): прочитать `PRAGMA user_version`;
  нормализовать 0→`baselineVersion`; цикл `for v < target`: `db.Begin()` → `tx.Exec(stmt)` →
  `tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", v+1))` → `tx.Commit()` (Rollback при ошибке внутри).
  Значение версии через `fmt.Sprintf` (НЕ bind `?`). FR-005/FR-006 · B-I1/B-I2/B-I3.
- [ ] T010 [US1] В `src/internal/store/sqlite.go` вставить вызов `migrate(db)` в `NewSQLiteStore` ПОСЛЕ
  блока `db.Exec(ddl)` и ДО `return &SQLiteStore{db: db}, nil`:
  `if err := migrate(db); err != nil { db.Close(); return nil, err }`. FR-009 · Контракт A (A5).

**Checkpoint**: US1 функционально полна — T004/T005/T006 зелёные; миграция работает для fresh/legacy/
idempotent. MVP достигнут.

---

## Phase 4: Polish & Cross-Cutting (инверсия + гейт + границы)

- [ ] T011 [US1] Инверсионная мутпроба (SC-004): временно удалить элемент `schemaMigrations` (или бамп
  версии) и убедиться, что `TestMigrateFreshDB` КРАСНЕЕТ (нет `outbox` / версия ≠ 2); вернуть код.
  Зафиксировать в комментарии теста/мутпробы, что замок реально кусает. Контракт G-A1/B-I1.
- [ ] T012 Проверка границ диффа: `git diff --name-only` показывает изменения ТОЛЬКО в
  `src/internal/store/sqlite.go` и новый `src/internal/store/migrate_test.go`; дифф
  `src/internal/eval`, `src/internal/engine`, `src/cmd/ladix`, `src/internal/daemon` ПУСТОЙ; 0 новых
  зависимостей (`go.mod`/`go.sum` без изменений). FR-012/FR-013 · SC-005.
- [ ] T013 Подтвердить неизменность контракта: `src/internal/store/store.go` (16 методов + двойной
  compile-замок), `src/internal/store/memory.go`, `src/internal/store/types.go` — БЕЗ изменений
  (`git diff` пуст для них). FR-010/FR-011.
- [ ] T014 Финальный гейт: `cd src && go build ./... && go vet ./... && gofmt -l . && go test ./...` —
  всё зелёное, `gofmt -l` пустой вывод, все тесты пакета `store` проходят. SC-006.

---

## Dependencies & Execution Order

- **Phase 1 (Setup, T001–T002)** → **Phase 2 (Foundational, T003)** → **Phase 3 (US1, T004–T010)** →
  **Phase 4 (Polish, T011–T014)**.
- **Внутри US1**: тесты T004→T005→T006 (один файл `migrate_test.go`, последовательно — общий файл) идут
  ПЕРЕД/параллельно с реализацией, но красны до неё. Реализация: T007 (const) → T008 (реестр) → T009
  (func migrate, использует T007/T008) → T010 (вызов в NewSQLiteStore, использует T009).
- **T011** зависит от завершённой реализации (T009/T010) и теста T004.
- **T012/T013/T014** — финальные проверки, после всей реализации.

### User Story Dependencies

- US1 — единственная история, самодостаточна (MVP). Зависимостей от других историй нет.

### Within-story order

- T007 (const) → T008 (schemaMigrations) → T009 (migrate) → T010 (вызов). T004 можно начать первым
  (скелет+fresh-тест), T005/T006 — следом (тот же файл).

## Parallel Opportunities

- Параллелизм ограничен: все изменения кода — в одном файле `sqlite.go`, все тесты — в одном файле
  `migrate_test.go`. Поэтому `[P]` помечен только T004 (создание нового тестового файла независимо от
  правок `sqlite.go`). Прочие задачи по `sqlite.go` строго последовательны (общий файл).

## Parallel Example

```text
# Старт US1 можно вести двумя параллельными дорожками (разные файлы):
Дорожка A (тесты, migrate_test.go):   T004 [P] → T005 → T006
Дорожка B (реализация, sqlite.go):    T007 → T008 → T009 → T010
# Слияние: тесты T004–T006 становятся зелёными по мере готовности B; затем T011–T014.
```

## Implementation Strategy

- **MVP First**: US1 целиком и есть MVP (и вся фича). Завершение T004–T010 даёт рабочую миграцию.
- **Incremental**: сначала красные замки (T004–T006), затем минимальная реализация (T007–T010) до
  зелёного, затем инверсия+границы+гейт (T011–T014).
- **Границы — строго**: ни строки диффа вне `src/internal/store/`; DDL `outbox` — дословно (контракт с
  будущей C2b).

## Notes

- Тесты ВКЛЮЧЕНЫ по требованию §C-2a.4 (не опциональны для этой фичи).
- `OutboxRecord`, `LoadOutbox`, `SaveOutbox`, `ErrOutboxNotFound`, кодек значений outbox — ВНЕ границ
  (C2b). В C2a создаётся только сама таблица.
- Источник истины — `docs/reliability-model.md` §C-2a; сверка с кодом — `.m3-ledger/digest-seams.md`.
