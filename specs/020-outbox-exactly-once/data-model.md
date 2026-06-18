# Data Model: C2b — Outbox-леджер

Источник: `docs/reliability-model.md` §C-2b.4/.6 + швы `digest-seams.md`.

## Сущность: OutboxRecord (`internal/store/types.go`, рядом с Event/TriggerState)

Одна durable-запись доставленного (или диспатчимого) эффекта тела шага. Value-ориентирована (INV-2).

```go
type OutboxRecord struct {
    DedupKey    string         // "<instanceID>|<stepName>|<effectIndex>" — PRIMARY KEY
    InstanceID  string         // инстанс процесса
    StepName    string         // имя шага (= inst.CurrentStep в момент тела)
    EffectIndex int            // порядковый № эффекта в теле шага, от 0
    Kind        string         // "вызвать" | "уведомить"
    Target      string         // цель эффекта (имя внешней системы)
    Args        []value.Value  // аргументы эффекта; сериализуются type-tagged внутри SQLiteStore
    Result      value.Value    // вызвать → результат; уведомить/statement → value.None
    Delivered   bool           // true после успешной доставки + персиста
    CreatedAt   time.Time      // штамп создания записи (часы движка)
    DeliveredAt *time.Time     // штамп доставки (nil до доставки)
}
```

| Поле | Тип | Правила / инварианты |
|---|---|---|
| DedupKey | string | Ключ дедупа; формат `fmt.Sprintf("%s|%s|%d", InstanceID, StepName, EffectIndex)`. PRIMARY KEY таблицы `outbox`. Уникален. |
| InstanceID | string | NOT NULL. |
| StepName | string | NOT NULL. Равен `inst.CurrentStep` в момент `ExecStepBody`. |
| EffectIndex | int | ≥ 0. Детерминированный: от 0 при каждом исполнении тела, +1 на каждый effect-вызов. |
| Kind | string | `"вызвать"` или `"уведомить"`. |
| Target | string | NOT NULL. |
| Args | []value.Value | Кодек: `encodeList(value.NewList(args))` → колонка `args_json` (NOT NULL). MemoryStore: глубокая копия среза. |
| Result | value.Value | Кодек: `encodeValue` → колонка `result_json`. None → tagged-`Пусто` blob (НЕ SQL NULL). |
| Delivered | bool | Колонка `delivered INTEGER DEFAULT 0`. Pre-check читает это поле. |
| CreatedAt | time.Time | RFC3339 (как существующие штампы); часы движка (FixedClock в тестах). |
| DeliveredAt | *time.Time | nil до доставки; `&now` при SaveOutbox(delivered=1). Колонка `delivered_at TEXT` (nullable). MemoryStore: копировать указатель. |

### Соответствие таблице `outbox` (создана миграцией C2a 1→2)

```sql
CREATE TABLE IF NOT EXISTS outbox (
    dedup_key    TEXT PRIMARY KEY,
    instance_id  TEXT NOT NULL,
    step_name    TEXT NOT NULL,
    effect_index INTEGER NOT NULL,
    kind         TEXT NOT NULL,
    target       TEXT NOT NULL,
    args_json    TEXT NOT NULL,
    result_json  TEXT,            -- tagged-blob (вкл. None); НЕ NULL для None
    delivered    INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT NOT NULL,
    delivered_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_outbox_instance ON outbox(instance_id, step_name);
```

C2b НЕ создаёт таблицу/индекс — только читает/пишет. Новых миграций нет.

### Жизненный цикл записи

```
[нет записи] --LoadOutbox→ ErrOutboxNotFound
     │
     │ доставка успешна
     ▼
SaveOutbox(Delivered=true, Result, CreatedAt=now, DeliveredAt=&now)  (upsert)
     │
     ▼
[delivered] --LoadOutbox→ rec.Delivered=true → pre-check СКИП доставки, вернуть rec.Result
```

Переход «не-delivered → delivered» односторонний в штатном пути. Запись с `Delivered=false` в durable-хранилище не появляется (deliver-then-record: запись пишется только после успешной доставки). `Delivered bool` поле существует для полноты модели и будущей семантики; pre-check проверяет его явно.

## Сущность: ErrOutboxNotFound (sentinel)

```go
var ErrOutboxNotFound = errors.New("outbox-запись не найдена")
```

- Место: рядом с `ErrTriggerStateNotFound` (живёт в `store/types.go:85-92` — см. research R-DRIFT-1; §C-2b.6 говорит `store/errors.go`).
- Зеркалит `ErrTriggerStateNotFound`. `errors.Is`-совместим (Принцип III).
- `LoadOutbox` возвращает его при `sql.ErrNoRows` (SQLite) / отсутствии ключа в map (Memory).

## Расширение: activeFrame (`internal/engine/engine.go:31-34`)

```go
type activeFrame struct {
    inst       *store.ProcessInstance
    processEnv *eval.Environment
    effectIndex int            // ◀ НОВОЕ M3-C2b: счётчик эффектов тела текущей итерации шага
}
```

| Поле | Правила |
|---|---|
| effectIndex | Сброс в 0 в начале каждой итерации шага в `advance` (перед `ExecStepBody`). Инкремент `fr.effectIndex++` в каждом effect-методе при `len(e.active)>0`. Инстанс-локально (не пакетное состояние, Принцип V). |

## Контракт Store: 16 → 18 (аддитивно, двойной замок)

```go
// --- outbox-леджер (M3-C2b, аддитивно 16→18) ---
LoadOutbox(dedupKey string) (*OutboxRecord, error) // не найдено → ErrOutboxNotFound
SaveOutbox(rec *OutboxRecord) error                // upsert по dedup_key
```

- Базовые 16 методов байт-целы (`SaveInstance … ListTasksByInstance`).
- Compile-замок `internal/store/store.go:44-45`: `_ Store = (*MemoryStore)(nil)`; `_ Store = (*SQLiteStore)(nil)` — расширяется по интерфейсу автоматически; обе impl обязаны иметь оба метода, иначе сборка падает.
- MemoryStore: `outbox map[string]*OutboxRecord` (новое поле структуры) + `NewMemoryStore` инициализирует map. Глубокая копия Args/времён в Load/Save (как `copyTask`).
- SQLiteStore: SELECT/INSERT ON CONFLICT внутри (сериализация через codec.go).
