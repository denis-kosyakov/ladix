# Contract — Версионирование схемы постоянного хранилища (forward-only)

**Feature**: 019-store-schema-migrations | **Date**: 2026-06-18

Это внутренний контракт пакета `src/internal/store`. У фичи НЕТ публичного API/CLI-поверхности (миграция
безмолвна, происходит при открытии хранилища). Контракт описывает наблюдаемое поведение открытия
`SQLiteStore` и внутренние инварианты функции миграции. Все идентификаторы/DDL — дословно из §C-2a.

## Контракт A: Поведение открытия постоянного хранилища

**Поверхность**: `NewSQLiteStore(path string) (*SQLiteStore, error)` (существующая сигнатура — НЕ
меняется). Внутри, между применением базовой схемы (`db.Exec(ddl)`) и возвратом хранилища, вызывается
`migrate(db)`.

| # | Предусловие (состояние файла) | Постусловие после успешного открытия |
|---|---|---|
| A1 | новый/несуществующий файл | `user_version == 2`; таблица `outbox` и индекс `idx_outbox_instance` существуют |
| A2 | базовые таблегицы есть, `user_version == 0`, есть данные | `user_version == 2`; `outbox` появилась; данные базовых таблиц целы |
| A3 | `user_version == 1` (базовая, без outbox) | `user_version == 2`; `outbox` появилась |
| A4 | `user_version == 2` (уже актуальна) | no-op: `user_version == 2`; outbox не пересоздаётся/не дублируется; без ошибок |
| A5 | ошибка применения шага миграции | `NewSQLiteStore` возвращает ошибку; соединение закрыто (`db.Close()`); `*SQLiteStore` == nil |

**Гарантии**:
- G-A1 (forward-only): открытие никогда не понижает `user_version` и не выполняет обратных шагов.
- G-A2 (атомарность): после возврата схема и `user_version` согласованы (нет полупримененного шага).
- G-A3 (сохранность): открытие не переинициализирует и не сбрасывает базовые таблицы (отзыв D-AU-9).
- G-A4 (идемпотентность): повторное открытие актуальной базы — чистый no-op миграций.

## Контракт B: Внутренняя функция `migrate(db *sql.DB) error`

**Сигнатура (целевая)**: `func migrate(db *sql.DB) error` — неэкспортируемая, в пакете `store`.

**Алгоритм (нормативный, по образцу `nextCounter`)**:

1. Прочитать `v := PRAGMA user_version` (через `db.QueryRow(...).Scan(&v)`). Ошибка чтения → вернуть.
2. Если `v == 0`: установить `v = baselineVersion` и выполнить `db.Exec("PRAGMA user_version = 1")`
   (нормализация нуля; вне tx — допустимо, т.к. базовая схема уже создана `db.Exec(ddl)`). Ошибка → вернуть.
3. `target := baselineVersion + len(schemaMigrations)` (= 2).
4. Пока `v < target`:
   a. `stmt := schemaMigrations[v-baselineVersion]` (шаг v→v+1).
   b. `tx, err := db.Begin()`; ошибка → вернуть.
   c. `tx.Exec(stmt)`; ошибка → `tx.Rollback()` + вернуть.
   d. `tx.Exec("PRAGMA user_version = " + (v+1))` (через `fmt.Sprintf`, НЕ bind `?`); ошибка →
      `tx.Rollback()` + вернуть.
   e. `tx.Commit()`; ошибка → вернуть.
   f. `v++`.
5. Вернуть `nil`.

**Инварианты функции**:
- B-I1: шаг DDL и бамп версии — в одной транзакции (шаги 4c–4e); откат целиком при любой ошибке внутри.
- B-I2: значение версии подставляется как доверенная int-константа через `fmt.Sprintf` (PRAGMA нельзя
  биндить `?`); внешнего ввода нет → инъекция исключена.
- B-I3: `migrate` использует переданный `*sql.DB` (то же одно соединение, что у `NewSQLiteStore`); не
  открывает второе соединение.
- B-I4: при `v >= target` цикл не выполняет итераций → детерминированный no-op (идемпотентность).

## Контракт C: Константы и реестр

| Идентификатор | Значение | Назначение |
|---|---|---|
| `baselineVersion` | `1` | базовая схема «006/007/018» |
| `currentSchemaVersion` | `2` | актуальная версия после фичи |
| `schemaMigrations` | `[]string{ <DDL 1→2> }` | упорядоченный реестр forward-only-шагов |

**Инвариант согласованности**: `currentSchemaVersion == baselineVersion + len(schemaMigrations)`.

**DDL ступени 1→2 (ВЕРБАТИМ — элемент `schemaMigrations[0]`)**:

```sql
CREATE TABLE IF NOT EXISTS outbox (
    dedup_key    TEXT PRIMARY KEY,
    instance_id  TEXT NOT NULL,
    step_name    TEXT NOT NULL,
    effect_index INTEGER NOT NULL,
    kind         TEXT NOT NULL,
    target       TEXT NOT NULL,
    args_json    TEXT NOT NULL,
    result_json  TEXT,
    delivered    INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT NOT NULL,
    delivered_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_outbox_instance ON outbox(instance_id, step_name);
```

## Контракт D: Границы (что НЕ меняется)

- D-1: контракт интерфейса `Store` остаётся 16 методов; новых методов нет.
- D-2: двойной compile-time замок (`var ( _ Store = (*MemoryStore)(nil); _ Store = (*SQLiteStore)(nil) )`)
  не меняется.
- D-3: `MemoryStore` миграций не имеет; его конструктор не меняется.
- D-4: дифф вне `src/internal/store/` ПУСТОЙ; 0 новых зависимостей.

## Покрытие тестами (отображение на §C-2a.4)

| Тест | Контракт | FR / SC |
|---|---|---|
| `TestMigrateFreshDB` | A1 | FR-001/FR-004 · SC-001 |
| `TestMigrateLegacyV0` | A2 (+G-A3) | FR-003/FR-008 · SC-002 |
| `TestMigrateIdempotent` | A4 (+B-I4) | FR-007 · SC-003 |
| инверсионная мутпроба (удалить шаг/бамп) | G-A1/B-I1 | SC-004 |
