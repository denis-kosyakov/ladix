# Контракт: миграция схемы Store 2→3 (`internal/store/sqlite.go`)

## Изменения

| Что | Файл:строка | Было | Станет |
|-----|-------------|------|--------|
| Целевая версия | `sqlite.go:82-84` | `currentSchemaVersion = 2` | `currentSchemaVersion = 3` |
| Ступени миграции | `sqlite.go:106-122` | `schemaMigrations` с 1 элементом (1→2 outbox) | + 2-й элемент-строка `"DELETE FROM trigger_state;"` (2→3) |

**Новый элемент** (с комментарием):
```go
// 2 → 3: ре-кей триггеров на контентные ключи (§FR-009), сброс позиционного состояния.
"DELETE FROM trigger_state;",
```

**НЕ трогать**: DDL `trigger_state` (`sqlite.go:52-58`), тип `store.TriggerState`
(`types.go:64-70`), `baselineVersion=1` (`sqlite.go:81`), контракт `Store`.

## Инвариант INV-R1 (двойной compile/runtime-замок)

`init()` (`sqlite.go:91-97`) паникует, если
`currentSchemaVersion != baselineVersion + len(schemaMigrations)`.

После правки: `3 == 1 + 2`. **`1 + 2 = 3`** ✅.

> **Мутпроба**: забыть бамп версии (оставить `currentSchemaVersion = 2`) при добавленном 2-м
> элементе → `2 != 1 + 2` → `init()` паникует на загрузке пакета → все store-тесты краснеют.

## Применение миграции (`migrate(db)` `:374-412`) — без изменений механики

1. `PRAGMA user_version` → текущая `v`.
2. Если `v == 0` → применить baseline (полный DDL), `v := baselineVersion (1)`.
3. Пока `v < currentSchemaVersion (3)`: в одной `db.Begin()`-транзакции выполнить
   `schemaMigrations[v - baselineVersion]` + `PRAGMA user_version = v+1` (атомарно), `v++`.

Для существующей БД версии 2: применяется `schemaMigrations[2-1] == schemaMigrations[1] ==
"DELETE FROM trigger_state;"`, `user_version` → 3. Forward-only. Вызов из `NewSQLiteStore:150`.

## Семантика перехода (FR-009)

- Миграция видит **только базу**, не AST программы → старые позиционные `trg-<N>` **нельзя**
  переиздать в контентные.
- Переход = **сброс `trigger_state` + ленивый ре-прайминг** на последующих тиках.
- Поведенческая нейтральность первого тика (FR-010): метрика и `каждые` уже праймят-без-срабатывания;
  schedule_at приведён под прайм (см. `contracts/trigger-keys.md`). Сброс не вызывает ложных запусков.

## Контракт результата

| Свойство | Гарантия (SC-005) |
|----------|-------------------|
| Версия после открытия v2-БД | `PRAGMA user_version == 3` |
| Состояние `trigger_state` | очищена (0 строк) |
| Повторное открытие | новые (контентные) ключи сохраняются; версия остаётся 3 |
| DDL/тип/контракт Store | без изменений (FR-007) |

## Memory parity

`Memory`-impl (`memory.go`) **не имеет** версии схемы — миграция SQLite-специфична (`user_version`).
Контракт durable-данных (`LoadTriggerState`/`SaveTriggerState`) у Memory не меняется; ключ-минт
детерминирован одинаково в обеих impl (T9 — паритет durable-замков через `eachStore` там, где
применимо; миграционный T7 — только SQLite).

## Замок

- **T7** (`store/sqlite_test.go` или `migrate_test.go`, паттерн существующего `migrate_test.go`):
  собрать v2-БД со строками `trigger_state` (`trg-0`, `trg-1`, …) и `user_version=2` →
  `NewSQLiteStore(path)` → `trigger_state` ПУСТА, `PRAGMA user_version == 3`; Close+reopen
  сохраняет новые ключи и версию 3.
