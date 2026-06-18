# Data Model — Phase 1: Каркас миграций схемы Store (forward-only)

**Feature**: 019-store-schema-migrations | **Date**: 2026-06-18

Этот документ описывает сущности данных в границах C2a. Внешний по отношению к пользователю «контракт»
— это контракт версионирования схемы (см. `contracts/store-schema-migration.md`). Go-структура
`OutboxRecord` и методы доступа — ВНЕ границ C2a (фиксируется в конце документа).

## Сущность 1: Отметка версии схемы (`user_version`)

- **Носитель**: встроенный слот заголовка файла SQLite, читается/пишется `PRAGMA user_version`.
- **Тип**: целое (int).
- **Значения и трактовка**:
  | Значение | Смысл | Действие migrate |
  |---|---|---|
  | `0` | свежая ИЛИ до-версионная база | трактуется как базовая (1); затем применяются ступени от 2 |
  | `1` (`baselineVersion`) | базовая схема «006/007/018» | применяется ступень 1→2 |
  | `2` (`currentSchemaVersion`) | актуальная схема после данной фичи | no-op (ступеней нет) |
  | `> 2` | версия из будущей сборки (откат продукта) | no-op (forward-only, понижений нет) |
- **Инварианты**:
  - INV-V1 (forward-only): отметка версии только не убывает между открытиями; миграции применяются,
    только когда целевая версия превышает текущую (FR-006).
  - INV-V2 (атомарность): отметка версии поднимается строго в той же транзакции, что и DDL-шаг,
    поднимающий до неё (FR-005). Расхождение схема↔версия невозможно по дизайну.
  - INV-V3 (нормализация нуля): `0` всегда нормализуется к `baselineVersion` (1) перед циклом ступеней
    (FR-003).

## Сущность 2: Реестр миграций (`schemaMigrations`)

- **Носитель**: пакетная неизменяемая переменная-срез строк DDL (`var schemaMigrations = []string{…}`),
  read-only данные (аналог `const ddl`/`const pragmas`).
- **Структура**: `[]string`; элемент с индексом `i` поднимает версию `baselineVersion+i` →
  `baselineVersion+i+1`.
- **Текущее содержимое**: ровно ОДИН элемент — ступень 1→2 (DDL `outbox`, см. Сущность 3).
- **Целевая версия**: `target = baselineVersion + len(schemaMigrations)` = `1 + 1` = `2` =
  `currentSchemaVersion`.
- **Инвариант**: INV-R1 — `currentSchemaVersion == baselineVersion + len(schemaMigrations)`.
  (Согласованность константы и реестра; при добавлении ступени обе сущности правятся вместе.)

## Сущность 3: Таблица `outbox` (создаётся ступенью 1→2)

DDL ступени 1→2 — ДОСЛОВНО из §C-2a.3 (FR-004). Создаётся через миграцию; Go-методы доступа к ней —
вне границ C2a (C2b).

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

### Колонки `outbox` (справочно; назначение полей реализует C2b)

| Колонка | Тип SQLite | NULL? | Назначение (контекст C2b) |
|---|---|---|---|
| `dedup_key` | TEXT | PK | ключ идемпотентности `instance_id|step_name|effect_index` |
| `instance_id` | TEXT | NOT NULL | экземпляр процесса |
| `step_name` | TEXT | NOT NULL | имя шага, в теле которого эффект |
| `effect_index` | INTEGER | NOT NULL | порядковый № эффекта в теле шага (от 0) |
| `kind` | TEXT | NOT NULL | вид эффекта («вызвать» / «уведомить») |
| `target` | TEXT | NOT NULL | цель эффекта |
| `args_json` | TEXT | NOT NULL | аргументы (type-tagged JSON) |
| `result_json` | TEXT | NULL-разрешён | результат (для `вызвать`-с-результатом) |
| `delivered` | INTEGER | NOT NULL DEFAULT 0 | флаг доставки (0/1) |
| `created_at` | TEXT | NOT NULL | момент создания записи |
| `delivered_at` | TEXT | NULL-разрешён | момент доставки |

- **Индекс**: `idx_outbox_instance` по `(instance_id, step_name)` — ускоряет выборки по экземпляру/шагу
  (используется C2b; в C2a — только создаётся).
- **Инвариант C2a**: INV-O1 — таблица и индекс создаются ИДЕМПОТЕНТНО (`IF NOT EXISTS`), повторная
  миграция не пересоздаёт и не дублирует (FR-007).

## Состояния и переходы (state machine версии схемы при открытии)

```text
[файл]
  │  NewSQLiteStore → db.Exec(ddl)  (базовые таблицы IF NOT EXISTS)
  ▼
read user_version = v
  ├── v == 0 ──► set user_version = baselineVersion(1); v = 1     (INV-V3)
  ▼
while v < target (=2):
    BEGIN TX
      exec schemaMigrations[v-1]          (DDL шага v→v+1)
      set user_version = v+1              (бамп в той же TX, INV-V2)
    COMMIT  (или ROLLBACK при ошибке → возврат error → db.Close())
    v = v+1
  ▼
v == target(2) ──► return *SQLiteStore   (схема актуальна)
```

- Свежая БД: v=0 → нормализуется к 1 → одна итерация (1→2) → v=2.
- Legacy v0: то же, что свежая (нормализация нуля), но базовые данные уже присутствуют и сохраняются.
- Уже актуальная (v=2): цикл не выполняет итераций → no-op → v=2 (идемпотентность).

## ВНЕ ГРАНИЦ C2a (фиксируется явно — это C2b)

- Go-тип `OutboxRecord` (структура записи) — НЕ создаётся в C2a.
- Методы `LoadOutbox(dedupKey)` / `SaveOutbox(rec)` — НЕ создаются; контракт `Store` остаётся 16 методов.
- Sentinel `ErrOutboxNotFound` — НЕ создаётся.
- Кодек значений `outbox` (`encodeList`/`encodeValue` для `args_json`/`result_json`) — НЕ создаётся.
- Двойной compile-time замок `Store` (`store.go:42-45`) — НЕ меняется (FR-011).
