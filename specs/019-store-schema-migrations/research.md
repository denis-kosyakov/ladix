# Research — Phase 0: Каркас миграций схемы Store (forward-only)

**Feature**: 019-store-schema-migrations | **Date**: 2026-06-18

Источник истины: `docs/reliability-model.md` §C-2a (+ §C-0/§C-6/§C-7). Сверка с живым кодом:
on-disk дайджесты `.m3-ledger/digest-anchor.md` (секция C2a) и `.m3-ledger/digest-seams.md`.
Все «NEEDS CLARIFICATION» из Technical Context разрешены ниже — ни одного открытого вопроса.

## R-1: Механизм версионирования схемы SQLite

- **Decision**: использовать встроенный `PRAGMA user_version` как единственную отметку версии схемы;
  отдельной таблицы истории миграций НЕ заводить.
- **Rationale**: `user_version` — нативный целочисленный слот в заголовке файла SQLite, хранится в
  самой базе, читается/пишется одним PRAGMA, транзакционен (см. R-4). Минимальный механизм под задачу
  (Принцип простоты, §C-0). Якорь §C-2a.2 предписывает именно его.
- **Alternatives considered**: (a) отдельная таблица `schema_migrations(version, applied_at)` —
  избыточна для forward-only одной-двух ступеней, добавляет объект схемы и запись/чтение; отвергнута.
  (b) внешняя миграционная библиотека (`golang-migrate` и т.п.) — нарушает «0 новых зависимостей» и
  Принцип I (единственная dep `modernc.org/sqlite`); отвергнута.

## R-2: Константы версий и трактовка user_version = 0

- **Decision**: `baselineVersion = 1`, `currentSchemaVersion = 2`. `user_version = 0` (свежая ИЛИ
  до-версионная БД) трактуется как базовая версия (1) — без различения «новая/старая».
- **Rationale**: базовая схема «006/007/018» (`const ddl`: instances/tasks/counters/trigger_state/
  events + индексы + сид счётчиков) уже создаётся существующей `db.Exec(ddl)` при каждом открытии
  через `CREATE … IF NOT EXISTS` ДО вызова `migrate`. Поэтому к моменту миграции базовые таблицы
  гарантированно существуют независимо от того, новый это файл или до-версионный → различать не нужно
  (Assumption A-3). Якорь §C-2a.2 фиксирует эти значения дословно.
- **Alternatives considered**: проверять наличие таблиц для различения new/legacy — лишняя сложность,
  не влияет на результат (обе ветки → версия 1 → миграции от 2); отвергнута.

## R-3: Точка встройки вызова migrate(db)

- **Decision**: вызвать `migrate(db)` в `NewSQLiteStore` ПОСЛЕ успешного `db.Exec(ddl)` и ДО
  `return &SQLiteStore{db: db}, nil`. При ошибке миграции — `db.Close()` + возврат ошибки.
- **Rationale**: к этой точке базовая схема применена (`db.Exec(ddl)`), настройки соединения
  установлены (`db.Exec(pragmas)`, `SetMaxOpenConns(1)`), но хранилище ещё не отдано наружу — идеальное
  место привести схему к актуальной версии единожды на открытие. Сверено с живым кодом
  (`digest-seams.md` §C2a): `NewSQLiteStore` = sqlite.go:79; блок `db.Exec(ddl)` :94–97; `return` :98.
  Зеркалит существующий паттерн ошибок `db.Exec` (`db.Close(); return nil, err`).
- **DRIFT-нота**: якорь §C-10 пишет «db.Exec(ddl) :94, return :98»; реальный `const ddl` закрывается на
  :66 (якорь :67, off-by-1), `const pragmas` на :73 (якорь :74), закрывающая скобка `NewSQLiteStore`
  на :99 (`return` на :98 — совпадает). Это сдвиги нумерации, НЕ ошибки содержания; точка встройки
  (после `db.Exec(ddl)`-блока, перед `return`) однозначна структурно.

## R-4: Атомарность шага миграции и бампа версии (транзакция)

- **Decision**: каждый DDL-шаг ступени И поднятие `user_version` выполнять в ОДНОЙ транзакции
  (`db.Begin()` → `tx.Exec(stmt)` → `tx.Exec("PRAGMA user_version = N")` → `tx.Commit()`; при ошибке
  `tx.Rollback()`), по образцу существующего `nextCounter` (sqlite.go:308–328).
- **Rationale**: `PRAGMA user_version = N` транзакционен в SQLite — бамп внутри tx атомарен с DDL шага.
  При крахе до `Commit` транзакция откатывается целиком → схема и отметка версии не расходятся
  (нет полупримененного шага). Якорь §C-2a.3 «Идиомы/ловушки» подтверждает транзакционность PRAGMA.
- **Alternatives considered**: безтранзакционные `db.Exec` подряд — допускают расхождение схема↔версия
  при крахе между ними; отвергнуто (FR-005).

## R-5: Подстановка значения версии (нельзя `?`)

- **Decision**: значение версии в `PRAGMA user_version = N` подставлять через `fmt.Sprintf` с
  int-константой (`baselineVersion` / `v+1`), НЕ через bind-параметр `?`.
- **Rationale**: SQLite не позволяет связывать значение PRAGMA параметром-плейсхолдером. Значение —
  доверенная внутренняя целочисленная константа (не внешний ввод) → инъекция исключена (Assumption A-6,
  якорь §C-2a.3). `fmt` уже импортирован в `sqlite.go`.
- **Alternatives considered**: prepared statement с `?` — не поддерживается SQLite для PRAGMA;
  отвергнуто (технически невозможно).

## R-6: Соединение и режим внешних ключей

- **Decision**: `migrate` использует ТОТ ЖЕ `*sql.DB`, что и `NewSQLiteStore` (одно соединение,
  `SetMaxOpenConns(1)`). Управлять `PRAGMA foreign_keys` внутри tx миграции НЕ нужно.
- **Rationale**: одно соединение сохраняет «липкость» pragmas и атомарность mint счётчика (D-10,
  комментарий в sqlite.go). Ступень 1→2 = отдельностоящая таблица `outbox` БЕЗ внешних ключей →
  `PRAGMA foreign_keys` нерелевантен (Assumption A-4, якорь §C-2a.3). Замечание на будущее: менять
  `foreign_keys` внутри tx — no-op (для M3 неактуально), но это не касается данной фичи.
- **Alternatives considered**: открыть второе соединение для миграции — нарушит модель «одно
  соединение», сломает «липкость» pragmas; отвергнуто.

## R-7: MemoryStore без миграций

- **Decision**: `MemoryStore` НЕ получает механизма миграций; `migrate` живёт только в `SQLiteStore`.
- **Rationale**: у `MemoryStore` нет персистентной версионируемой схемы (карты в памяти, эфемерны).
  Контракт `Store` ради миграций НЕ расширяется (INV-2; FR-010/FR-011). Якорь §C-2a.2/§C-2a «Идиомы»
  фиксируют это явно.
- **Alternatives considered**: добавить no-op `migrate` в MemoryStore ради симметрии — лишний код без
  ценности (нет схемы для версионирования); отвергнуто.

## R-8: Реестр schemaMigrations и DDL ступени 1→2 (ВЕРБАТИМ)

- **Decision**: `var schemaMigrations = []string{ <DDL 1→2> }`; индекс элемента = пара версий
  (`schemaMigrations[v-baselineVersion]` поднимает v→v+1). DDL ступени 1→2 копируется ДОСЛОВНО из
  §C-2a.3 (см. data-model.md / contracts/) — создаёт таблицу `outbox` + индекс `idx_outbox_instance`.
- **Rationale**: реестр-срез даёт расширяемый forward-only-каркас (ступени 2→3+ добавляются как
  элементы), индекс структурно кодирует forward-only-инвариант. DDL дословен, т.к. форма таблицы
  `outbox` — контракт между C2a (создаёт) и C2b (читает/пишет через будущие методы); любое отклонение
  сломало бы C2b. `CREATE … IF NOT EXISTS` обеспечивает идемпотентность DDL даже при гонке.
- **Alternatives considered**: inline один `CREATE TABLE outbox` без реестра — проще сейчас, но без
  каркаса для будущих миграций; отвергнуто (якорь требует реестр + раннер).

## R-9: Тест-стратегия (замки §C-2a.4)

- **Decision**: новый файл `src/internal/store/migrate_test.go` с табличными/целевыми тестами:
  `TestMigrateFreshDB` (новый файл → версия 2 + `outbox` существует), `TestMigrateLegacyV0` (вручную
  базовые таблицы + `user_version=0` + вставленные инстанс/задача → версия 2, `outbox` появилась,
  данные целы), `TestMigrateIdempotent` (повторное открытие версии 2 → no-op, версия 2, без ошибок).
  Инверсия (мутпроба): удалить строку миграции 1→2 → `TestMigrateFreshDB` краснит (нет `outbox`/версия 1).
- **Rationale**: каждый замок отображается на FR/SC (FreshDB↔SC-001/FR-001; LegacyV0↔SC-002/FR-008;
  Idempotent↔SC-003/FR-007; инверсия↔SC-004). Проверка существования таблицы — через
  `sqlite_master`/`PRAGMA table_info`; версия — `PRAGMA user_version`. Полностью детерминированы.
- **Alternatives considered**: только fresh-тест — не покрывает legacy/идемпотентность/инверсию;
  отвергнуто (§C-2a.4 требует все четыре).

## R-10: Сверка отсутствия дрейфа (digest-seams)

- `user_version` / `baselineVersion` / `currentSchemaVersion` / `migrate` / `schemaMigrations` —
  НЕ существуют в репо сегодня (grep zero matches в `internal/store/`). Ожидаемо: C2a их вводит.
- `OutboxRecord` / `LoadOutbox` / `SaveOutbox` / `ErrOutboxNotFound` — также не существуют; они вне
  границ C2a (это C2b). Контракт `Store` сегодня = 16 методов (`store.go:13-40`), двойной compile-замок
  `store.go:42-45`. C2a их НЕ трогает.
- Все пути файлов из §C-10 подтверждены корректными в `digest-seams.md`; расхождения только в
  нумерации строк (off-by-1, см. R-3 DRIFT-нота) — не влияют на дизайн.
