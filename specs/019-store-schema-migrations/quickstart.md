# Quickstart — Каркас миграций схемы Store (forward-only)

**Feature**: 019-store-schema-migrations | **Date**: 2026-06-18

Краткий гид для разработчика, реализующего/проверяющего C2a. Источник истины —
`docs/reliability-model.md` §C-2a; сверка с кодом — `.m3-ledger/digest-seams.md`.

## Что делает фича

Постоянное хранилище (`SQLiteStore`) при каждом открытии приводит схему к версии 2 через forward-only
миграцию на `PRAGMA user_version`. Единственная ступень 1→2 создаёт таблицу `outbox` + индекс. Данные
сохраняются (отзыв паллиатива D-AU-9 «сбрасывать БД»). `MemoryStore` не затрагивается.

## Где менять код (один пакет)

```text
src/internal/store/sqlite.go      # вставить вызов migrate(db) + const/реестр/func migrate
src/internal/store/migrate_test.go # НОВЫЙ файл с замками
```

Дифф вне `src/internal/store/` ДОЛЖЕН быть пустым.

## Шаги реализации (по контракту)

1. Добавить константы рядом с существующими `const ddl`/`const pragmas` в `sqlite.go`:
   `baselineVersion = 1`, `currentSchemaVersion = 2`.
2. Добавить `var schemaMigrations = []string{ <DDL 1→2 ВЕРБАТИМ из data-model.md> }`.
3. Добавить `func migrate(db *sql.DB) error` по алгоритму контракта B (tx-шаблон по образцу
   `nextCounter`, sqlite.go:308–328).
4. В `NewSQLiteStore` вставить вызов ПОСЛЕ блока `db.Exec(ddl)` и ДО `return`:
   ```go
   if _, err := db.Exec(ddl); err != nil { db.Close(); return nil, err }
   if err := migrate(db); err != nil { db.Close(); return nil, err }   // ◀ НОВОЕ
   return &SQLiteStore{db: db}, nil
   ```

## Локальная проверка (гейт)

```bash
cd /Users/denis/dev/ladix/src
go build ./...
go vet ./...
gofmt -l .            # должен быть пустой вывод
go test ./...         # все зелёные, включая internal/store
```

## Тест-замки (создать в migrate_test.go)

| Тест | Сценарий | Ожидание |
|---|---|---|
| `TestMigrateFreshDB` | открыть `NewSQLiteStore` на новом temp-файле | `PRAGMA user_version == 2`; таблица `outbox` существует |
| `TestMigrateLegacyV0` | вручную создать базовые таблицы, `user_version=0`, вставить инстанс+задачу; открыть | версия → 2; `outbox` появилась; инстанс+задача читаются без изменений |
| `TestMigrateIdempotent` | открыть актуальную БД (версия 2) повторно | без ошибок; версия == 2; `outbox` не дублируется |
| инверсия (мутпроба) | вручную убрать шаг 1→2 / бамп → прогнать | `TestMigrateFreshDB` краснит (нет `outbox` / версия 1) |

Проверка наличия таблицы — запрос к `sqlite_master` (`SELECT name FROM sqlite_master WHERE
type='table' AND name='outbox'`) или `PRAGMA table_info(outbox)`. Версия — `PRAGMA user_version`.

## Чего НЕ делать (границы C2a)

- НЕ добавлять `OutboxRecord`, `LoadOutbox`, `SaveOutbox`, `ErrOutboxNotFound` — это C2b.
- НЕ менять контракт `Store` (остаётся 16 методов) и двойной compile-замок.
- НЕ трогать `MemoryStore`, `eval`, `engine`, `cmd`, `daemon`.
- НЕ добавлять зависимостей; `database/sql`+`fmt` уже импортированы.
- DDL `outbox` копировать ДОСЛОВНО (форма — контракт с C2b).
