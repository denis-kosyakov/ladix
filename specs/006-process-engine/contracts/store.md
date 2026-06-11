# Контракт: интерфейс `Store` v006 (`internal/store`)

**Фаза**: 1 (design) | **Якорь**: `docs/engine-model.md §EN-2` | **Решения**: D-3, D-10, D-12, D-15,
D-5, D-6, D-21 | **FR**: FR-001…FR-007

> Канон сигнатур и DDL фиксируется §EN-2; этот контракт переносит их **дословно** + семантику каждого
> метода и реализаций. При расхождении побеждает §EN-2. Go-код — **контракт сигнатур**, не реализация.

## Назначение

Нарезанный контракт хранилища состояния (D-3): **ровно 8 методов** над `ProcessInstance`/`Task`. Две
реализации за одним интерфейсом — `MemoryStore` (эфемерно, без алиасинга) и `SQLiteStore` (персистентно,
`modernc.org/sqlite`). Типы данных, статусы, сентинелы живут в `internal/store` (рядом с контрактом —
методы интерфейса их принимают); `internal/engine` владеет только поведением (§EN-4). Сентинелы —
**английские** (наружу не печатаются, транслируются в русские тексты §EN-8.B). Триггерные методы НЕ
объявляются — 007 добавит аддитивно.

## Интерфейс (дословно §EN-2; ровно 8 методов, D-3)

```go
type Store interface {
    SaveInstance(inst *ProcessInstance) error            // upsert: создание и обновление
    LoadInstance(id string) (*ProcessInstance, error)    // не найден → ErrInstanceNotFound

    SaveTask(t *Task) error
    LoadTask(id string) (*Task, error)                   // не найдена → ErrTaskNotFound
    ListPendingTasks(assignee string) ([]*Task, error)   // assignee=="" → все открытые; порядок — по возрастанию ID (D-15)
    MarkTaskCompleted(id string, completedAt time.Time) error // атомарно открыта→завершена (D-12); повтор → ErrTaskAlreadyCompleted

    NextInstanceID() (string, error)                     // mint "p-NNNNNN" (D-10)
    NextTaskID() (string, error)                         // mint "t-NNNNNN"
}

var (
    ErrInstanceNotFound     = errors.New("process instance not found")
    ErrTaskNotFound         = errors.New("task not found")
    ErrTaskAlreadyCompleted = errors.New("task already completed")
)
```

## Семантика методов

| Метод | Семантика | Сентинел / трансляция §EN-8 | FR |
|---|---|---|---|
| `SaveInstance(inst)` | **upsert**: создание и обновление инстанса; коллизии исключены конструктивно (id из счётчика, D-10). В SQLite — кодек `Variables` в type-tagged JSON (§EN-2/D-21), времена в RFC3339 | сбой → обёртка `%w`, маршрутизация §EN-3 | FR-001 |
| `LoadInstance(id)` | загрузка инстанса по id; декод JSON `Variables` (SQLite). Не найден → `ErrInstanceNotFound` | `ErrInstanceNotFound` → `ladix: инстанс '<id>' не найден` (§EN-8.B) | FR-004 |
| `SaveTask(t)` | upsert задачи; времена RFC3339, `deadline`/`completed_at` могут быть NULL | сбой → обёртка `%w` | FR-001 |
| `LoadTask(id)` | загрузка задачи по id. Не найдена → `ErrTaskNotFound` | `ErrTaskNotFound` → `ladix: задача '<id>' не найдена` (§EN-8.B) | FR-004 |
| `ListPendingTasks(assignee)` | открытые задачи; `assignee==""` → **все** открытые (D-15); порядок — **по возрастанию ID** (детерминизм). В SQLite опирается на индекс `idx_tasks_pending(assignee, status)` | — (пустой список — не ошибка) | FR-001 |
| `MarkTaskCompleted(id, completedAt)` | **атомарно** `открыта → завершена` + фиксация `CompletedAt` (D-12). SQLite: условный `UPDATE … WHERE status='открыта'` + проверка rows affected; Memory: под mutex. Уже завершена → `ErrTaskAlreadyCompleted` | `ErrTaskAlreadyCompleted` → ветка догона D-4 (§EN-3) либо `ladix: задача '<id>' уже завершена` | FR-003 |
| `NextInstanceID()` | mint `p-NNNNNN` (D-10); персистентный монотонный счётчик `counters('instance')` (SQLite) / mutex (Memory); инкремент и выдача атомарны | — | FR-002 |
| `NextTaskID()` | mint `t-NNNNNN` (D-10); счётчик `counters('task')` | — | FR-002 |

**Чего в контракте нет (намеренно):**
- Методов **листинга инстансов** — D-4 (срок носителя якорь не назначает).
- **Триггерных** методов (`LoadTriggerState`/`SaveTriggerState`/`NextEventID`/`EnqueueEvent`/
  `ListUnprocessedEvents`/`MarkEventProcessed`) и `ErrTriggerStateNotFound` — 007 добавит **аддитивно**
  (путь отступления §EN-10).
- **Транзакционного комбо-метода** «завершить + продвинуть» — нет намеренно: корректность сбойного окна
  обеспечивает идемпотентный гард-догон D-4 (§EN-2).

## `MemoryStore` (FR-007)

`internal/store/memory.go`. Конструктор:

```go
func NewMemoryStore() *MemoryStore
```

- Карты `map[string]*ProcessInstance` / `map[string]*Task` + счётчики id — всё под одним `sync.Mutex`.
- **Без сериализации:** `value.Value` лежат как есть (JSON не трогается).
- **Без алиасинга указателей:** `Save`/`Load` **копируют** `ProcessInstance` и карту `Variables`
  (значения разделяются — ссылочность Список/Запись как в `Locals()`); `Task` — аналогично. `Load`
  возвращает **копию**: мутации снаружи не видны в Store до следующего `Save` (иначе гарды «инстанс не
  тронут» §EN-6 и тесты гранулярности персиста проверяли бы пустоту).
- `MarkTaskCompleted` — проверка статуса и перевод под тем же mutex.
- Назначение — `ladix run` без `--db`, `metric` и тесты lifecycle.

## `SQLiteStore` (FR-005)

`internal/store/sqlite.go`, `modernc.org/sqlite` (чистый Go, без CGO — **первая внешняя зависимость**;
go.mod + go.sum отдельным коммитом, FR-041). Сигнатуры:

```go
// NewSQLiteStore открывает БД и ЯВНО исполняет PRAGMA + DDL (включая сид counters;
// database/sql ленив — без явного Exec ошибка открытия не всплыла бы), первая ошибка
// возвращается наружу — это источник CLI-текста «не удалось открыть хранилище» (§EN-8.B).
func NewSQLiteStore(path string) (*SQLiteStore, error)

// Close — метод конкретного типа (НЕ интерфейса Store); CLI делает defer Close()
// после успешного открытия.
func (s *SQLiteStore) Close() error
```

- **PRAGMA при открытии** (EM-7): `journal_mode = WAL`, `busy_timeout = 5000`, `foreign_keys = ON`.
- **Mint (D-10):** `UPDATE` + чтение счётчика в **одной транзакции** (сид `counters` гарантирует наличие
  строки); формат `fmt.Sprintf("p-%06d", n)` / `"t-%06d"`.
- **Времена:** `time.Time` → строка через `.Format(time.RFC3339)`, парс — `time.Parse(time.RFC3339)`.
  Именно `time.RFC3339` (секундная точность), **не** `RFC3339Nano`.
- Таблицы `trigger_state`/`events` — **007**.

## DDL (дословно §EN-2; исполняется при открытии, `CREATE TABLE IF NOT EXISTS`)

```sql
CREATE TABLE IF NOT EXISTS instances (
    id           TEXT PRIMARY KEY,
    process_name TEXT NOT NULL,
    status       TEXT NOT NULL,
    current_step TEXT NOT NULL,
    variables    TEXT NOT NULL,   -- type-tagged JSON (кодек ниже), ключи верхнего уровня по возрастанию (D-21)
    created_at   TEXT NOT NULL,   -- RFC3339 (time.RFC3339)
    updated_at   TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS tasks (
    id           TEXT PRIMARY KEY,
    instance_id  TEXT NOT NULL REFERENCES instances(id),
    step_name    TEXT NOT NULL,
    assignee     TEXT NOT NULL,
    deadline     TEXT,            -- RFC3339 (time.RFC3339) или NULL
    status       TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    completed_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_tasks_pending ON tasks(assignee, status);
CREATE TABLE IF NOT EXISTS counters (
    name  TEXT PRIMARY KEY,       -- "instance" | "task"
    value INTEGER NOT NULL
);
INSERT OR IGNORE INTO counters(name, value) VALUES ('instance', 0), ('task', 0);
```

Сид `counters` (`INSERT OR IGNORE`) исполняется вместе с `CREATE` при каждом открытии — без него
`UPDATE` минта на свежей БД обновил бы 0 строк.

## Кодек значений (EM-4 + D-5/D-6/D-21) — внутреннее дело `SQLiteStore`

Колонка `variables` — type-tagged JSON. Интерфейс `Store` принимает/отдаёт `map[string]value.Value`;
`MemoryStore` JSON не трогает; движок про JSON/SQL не знает (путь к замене бэкенда открыт, EM-4).
Round-trip честный для всех 10 типов (FR-006).

| Тип Ladix | JSON-кодировка |
|---|---|
| Целое | `{"т":"Целое","зн":42}` |
| Дробное | `{"т":"Дробное","зн":3.14}`; **спецзначения — строки**: `"NaN"`, `"+Inf"`, `"-Inf"` (D-5) |
| Строка | `{"т":"Строка","зн":"привет"}` |
| Булево | `{"т":"Булево","зн":true}` |
| Пусто | `{"т":"Пусто","зн":null}` |
| Длительность | `{"т":"Длительность","зн":{"значение":5,"единица":"мин"}}` |
| Период | `{"т":"Период","зн":"ежемесячно"}` |
| Дата | `{"т":"Дата","зн":"2026-05-31"}` |
| Список | `{"т":"Список","зн":[ <рекурсивно>, … ]}` |
| Запись | `{"т":"Запись","зн":[["ключ", <значение>], …]}` — **массив пар**, порядок `Keys()` (D-6) |

- **D-21:** карта `Variables` (Go-map, порядок теряется) пишется верхним уровнем JSON-объектом с
  **ключами по возрастанию**; `Запись`-значения внутри переменных — массив пар D-6, их порядок `Keys()`
  сохраняется.

## Сбои Store — маршрутизация (§EN-3; FR-018)

Engine оборачивает не-сентинельные ошибки Store через `fmt.Errorf("<операция>: %w", err)`:
- **Пути, инициированные Ladix-узлом** (`запустить процесс`, `присвоить`, process-builtins — в т.ч. весь
  `advance` внутри `Start`) → §EN-8.A `сбой хранилища: <причина>`, позиция узла-инициатора, exit 1.
- **CLI-пути** `complete`/`tasks` (`LoadTask`/`LoadInstance`/`MarkTaskCompleted`/`ListPendingTasks`, ▼
  внутри `advance` из `Complete`, декод type-tagged JSON битой БД) → §EN-8.B
  `ladix: сбой хранилища: <причина>`, exit 2 (Ladix-позиции у неё нет — канон §13 неприменим).

## Тесты (по образцу пакетов 001–004)

- Round-trip кодека: все 10 типов значений `value.Value` через `SaveInstance`/`LoadInstance` →
  байт-эквивалентность (включая D-5 NaN/±Inf строками, D-6 Запись массивом пар, D-21 порядок ключей).
- `MemoryStore` без алиасинга: мутация загруженного инстанса/задачи не видна в Store до `Save`.
- `MarkTaskCompleted` атомарность: повтор → `ErrTaskAlreadyCompleted`; статус и `CompletedAt` корректны.
- `ListPendingTasks`: фильтр по `assignee`, `""` → все; порядок строго по возрастанию ID.
- Mint: свежий Store → `p-000001`/`t-000001`; SQLite — счётчик переживает переоткрытие (`Close`+открыть).
- Сентинелы: `errors.Is(err, ErrInstanceNotFound)` / `ErrTaskNotFound` / `ErrTaskAlreadyCompleted`.
