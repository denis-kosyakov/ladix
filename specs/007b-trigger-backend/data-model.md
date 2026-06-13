# Data Model — Бэкенд триггеров (007b)

**Feature**: 007b-trigger-backend | **Date**: 2026-06-13 | **Plan**: [plan.md](./plan.md)

> Источник: `docs/execution-model.md` EM-17.2/EM-17.3 (структуры `TriggerState`/`Event`, DDL),
> EM-17.2.1 (ключ `trg-<N>`), `docs/engine-model.md` D-3/D-4 (контракт Store). Сущности ложатся
> **аддитивно** в `internal/store` (types.go/sqlite.go/memory.go); 3 таблицы и 8 методов 006 не
> меняются (FR-021/022, SC-009). Все новые поля/таблицы дублируют форму существующих
> (`ProcessInstance`/`Task` + DDL instances/tasks/counters).

---

## Сущность 1 — `TriggerState` (состояние триггера)

Durable-строка состояния триггера. Ключ — `TriggerID` (`trg-<N>`, 0-based порядок объявления `когда`,
EM-17.2.1). Читается/пишется **только** демоном (FR-025). Несуществующая строка при первой оценке =
прайминг (`ErrTriggerStateNotFound`). Заводится **только** для метрика- и расписание-триггеров;
событие-триггер строки не имеет (FR-023).

### Go-структура (`internal/store/types.go`, аддитивно)

```go
// TriggerState — durable-состояние триггера между тиками и рестартами (EM-17.2).
// Указатели = трёхзначность: nil ⇒ «нет значения для этого вида / не праймлен».
type TriggerState struct {
    TriggerID     string     // ключ: "trg-<N>", N 0-based порядок объявления (EM-17.2.1, FR-023)
    Kind          string     // "metric" | "schedule_every" | "schedule_at"
    LastBool      *bool      // metric: базовая линия edge-детекта; nil = ещё не праймлен (FR-006/007)
    LastFire      *time.Time // schedule_every: момент последнего срабатывания (RFC3339); nil = не зарегистрирован (FR-011)
    LastFiredDate *string    // schedule_at: "YYYY-MM-DD" последнего срабатывания; nil = ещё не срабатывал (FR-013)
}
```

### Поля и инварианты

| Поле | Тип | Семантика | Инвариант |
|---|---|---|---|
| `TriggerID` | `string` | первичный ключ; `trg-<N>` | стабилен, пока исходник не переупорядочен (граница v1, FR-023) |
| `Kind` | `string` | дискриминатор вида | один из трёх литералов; задаётся при первой записи по типу `TriggerSpec` |
| `LastBool` | `*bool` | edge-база метрики | релевантно только `Kind="metric"`; `nil`=не праймлен; **персист ДО тела** (at-most-once, FR-008); не перезаписывается при невычислимой метрике (заморозка, FR-009) |
| `LastFire` | `*time.Time` | якорь `каждые` | только `Kind="schedule_every"`; при первой регистрации = момент старта без срабатывания (FR-011); далее = факт срабатывания (дрейф не копится) |
| `LastFiredDate` | `*string` | дата `в "ЧЧ:ММ"` | только `Kind="schedule_at"`; сравнение с `today` гарантирует «раз в сутки» (FR-013) |

### Состояния и переходы (метрика, edge-детект)

```
[нет строки] --первая оценка (прайминг, FR-007)--> LastBool=снимок, тело НЕ исполнено
LastBool=ложь --оценка=истина--> persist LastBool=истина (ДО тела) --> тело исполнено (СРАБАТЫВАНИЕ)
LastBool=истина --оценка=истина--> нет изменения, тело НЕ исполнено (нет перехода, FR-006)
LastBool=истина --оценка=ложь--> persist LastBool=ложь, тело НЕ исполнено
LastBool=* --метрика пусто / сравнение не-Булево--> заморозка: persist пропущен, тело НЕ исполнено (FR-009)
```

### Состояния (расписание `каждые`)

```
[нет строки] --первая регистрация (FR-011)--> LastFire=now (старт), без срабатывания
LastFire=T --тик: now >= shiftEvery(T, amount, unit)--> СРАБАТЫВАНИЕ, LastFire=now (факт)
LastFire=T --тик: now <  shiftEvery(...)--> нет срабатывания
```
`shiftEvery`: сек/мин/час/дн — фикс-множитель; нед/мес — календарный сдвиг с зажимом конца месяца
(R-6, FR-012). 31 янв + 1мес → 28/29 фев.

### Состояния (расписание `в "ЧЧ:ММ"`)

```
тик: today=now.date, target=now.date в ЧЧ:ММ
LastFiredDate != today И now >= target --> СРАБАТЫВАНИЕ, LastFiredDate=today (FR-013)
LastFiredDate == today --> нет срабатывания (раз в сутки)
старт после ЧЧ:ММ, дата не отмечена --> срабатывание в день старта (FR-013)
```

---

## Сущность 2 — `Event` (событие в очереди)

Запись внешнего события в очереди доставки `events`. Создаётся `ladix emit` (другой процесс ОС),
разбирается демоном на тике FIFO, помечается processed **ПОСЛЕ** исполнения тела (at-least-once,
FR-017). Событие без обработчиков отбрасывается с логом.

### Go-структура (`internal/store/types.go`, аддитивно)

```go
// Event — запись внешнего события в очереди доставки (EM-17.3).
type Event struct {
    ID          string    // opaque, "e-NNNNNN" (mint через NextEventID, зеркало p-/t-)
    Name        string    // имя события — матч с EventTrigger.Event.Name (FR-016)
    PayloadJSON string    // сырой JSON payload; маппится в value.Запись при обработке (FR-016)
    CreatedAt   time.Time // FIFO-порядок разбора (FR-016, SC-006)
    Processed   bool      // false=в очереди; true=обработано/отброшено (FR-017)
}
```

### Поля и инварианты

| Поле | Тип | Семантика | Инвариант |
|---|---|---|---|
| `ID` | `string` | первичный ключ; `e-NNNNNN` | mint через `NextEventID` (персистентный счётчик `event`), зеркало `p-`/`t-` (D-10) |
| `Name` | `string` | имя для матча | сравнивается с `EventTrigger.Event.Name` всех событие-триггеров |
| `PayloadJSON` | `string` | сырой JSON | парсится в `value.Запись` маппингом источников (SPEC §9); невалидный JSON — импл-факт (лог/skip, фиксируется кодом) |
| `CreatedAt` | `time.Time` | время создания | FIFO-порядок (`ListUnprocessedEvents` сортирует по `created_at`/rowid) |
| `Processed` | `bool` | признак обработки | `MarkEventProcessed` ставит true ПОСЛЕ тела; событие без триггеров → true + лог «без триггеров» |

### Жизненный цикл

```
emit <E> [json]: NextEventID → e-NNNNNN; EnqueueEvent(Processed=false); exit 0
тик drainEvents: ListUnprocessedEvents() FIFO
  для каждого E:
    найти EventTrigger с Name==E.Name
    есть совпавшие: PayloadJSON→Запись, инжект «событие», исполнить тело каждого → MarkEventProcessed (ПОСЛЕ тела, at-least-once)
    нет совпавших: MarkEventProcessed + лог «событие <имя> без триггеров»
краш между телом и MarkEventProcessed → переобработка на следующем старте (FR-017)
```

---

## Контракт Store — 7 новых методов (точные сигнатуры)

Все 7 ложатся **аддитивно** в интерфейс `store.Store` (store.go:14). 8 методов 006 не меняются.
Паритет: идентичная семантика в `*MemoryStore` (под `mu`) и `*SQLiteStore`. Сентинел
`ErrTriggerStateNotFound` объявляется явно (Принцип III, `errors.Is`).

```go
// --- 6 триггерных методов (обещание engine-model «+6») ---

// LoadTriggerState — состояние триггера по ключу. Не найдено → ErrTriggerStateNotFound
// (прайминг/первая регистрация, FR-007/011). Memory: чтение карты под mu; SQLite: SELECT по PK.
LoadTriggerState(triggerID string) (*TriggerState, error)

// SaveTriggerState — upsert состояния триггера (persist ДО тела для метрики, FR-008/010).
// Memory: запись копии в карту под mu; SQLite: INSERT … ON CONFLICT(trigger_id) DO UPDATE.
SaveTriggerState(ts *TriggerState) error

// NextEventID — mint "e-NNNNNN" (персистентный счётчик 'event', зеркало NextInstanceID, D-10).
NextEventID() (string, error)

// EnqueueEvent — записать событие в очередь (Processed=false). Вызывается командой emit
// (другой процесс ОС). Memory: append/карта под mu; SQLite: INSERT (WAL сериализует с serve).
EnqueueEvent(e *Event) error

// ListUnprocessedEvents — необработанные события в FIFO-порядке (по CreatedAt, затем rowid/ID
// для стабильности при равных штампах, FR-016/SC-006). Пустая очередь → пустой срез, не ошибка.
ListUnprocessedEvents() ([]*Event, error)

// MarkEventProcessed — пометить событие обработанным (ПОСЛЕ тела, at-least-once, FR-017).
// Идемпотентно: повторная пометка уже-обработанного — no-op без ошибки. Не найдено →
// (импл-факт: no-op/ошибка — фиксируется кодом; недостижимо в норме — id из ListUnprocessedEvents).
MarkEventProcessed(id string) error

// --- 7-й метод: рестарт-скан (ОСОЗНАННОЕ ОТСТУПЛЕНИЕ от «+6», FR-022, deviation) ---

// ListInstancesByStatus — инстансы в данном статусе (для рестарт-скана: "выполняется"/"создан",
// FR-019). Служебный, не языковая поверхность (Out of Scope как первоклассный запрос).
// Порядок — по возрастанию ID (детерминизм, зеркало ListPendingTasks). Пусто → пустой срез.
ListInstancesByStatus(status string) ([]*ProcessInstance, error)
```

### Сентинел (объявить явно, Принцип III)

```go
// ErrTriggerStateNotFound — LoadTriggerState не нашёл строку (прайминг/первая регистрация,
// EM-17.2). Зеркало ErrInstanceNotFound/ErrTaskNotFound; развёртка через errors.Is.
var ErrTriggerStateNotFound = stderrors.New("состояние триггера не найдено")
```

---

## DDL — две новые таблицы (`internal/store/sqlite.go`, аддитивно к `ddl`)

Зеркало EM-17.3. Добавляются к константе `ddl` (sqlite.go:23) **без** изменения instances/tasks/
counters. Времена — RFC3339 (секундная точность, как 006). Сид счётчика `event` — в `INSERT OR IGNORE`.

```sql
CREATE TABLE IF NOT EXISTS trigger_state (
    trigger_id      TEXT PRIMARY KEY,
    kind            TEXT NOT NULL,
    last_bool       INTEGER,        -- 0/1/NULL (NULL = не праймлен)
    last_fire       TEXT,           -- RFC3339 или NULL
    last_fired_date TEXT            -- "YYYY-MM-DD" или NULL
);
CREATE TABLE IF NOT EXISTS events (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    processed    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_events_pending ON events(processed, created_at);
-- сид счётчика событий (к существующему INSERT OR IGNORE counters):
INSERT OR IGNORE INTO counters(name, value) VALUES ('event', 0);
```

### MemoryStore (паритет, `internal/store/memory.go`, аддитивно)

```go
type MemoryStore struct {
    mu           sync.Mutex
    instances    map[string]*ProcessInstance
    tasks        map[string]*Task
    triggerState map[string]*TriggerState   // НОВОЕ: ключ TriggerID
    events       []*Event                   // НОВОЕ: FIFO-срез (порядок = порядок Enqueue ≈ CreatedAt)
    instSeq      int64
    taskSeq      int64
    eventSeq     int64                       // НОВОЕ: счётчик e-NNNNNN
}
```
Все 7 методов — под `mu`; копирование при Save/Load (без алиасинга, зеркало copyInstance/copyTask).
`ListUnprocessedEvents` фильтрует `!Processed`, отдаёт копии в порядке среза (FIFO). Паритет с SQLite
подтверждается **единым** контрактным набором тестов, прогоняемым на обоих бэкендах.

---

## Связь с AST 007a (неизменно)

`TriggerDecl`/`MetricTrigger`/`EventTrigger`/`ScheduleTrigger`/`EverySchedule`/`AtSchedule` (007a) —
**не меняются** (§TR-11, FR-026). 007b читает их:
- `Kind` выводится из типа `td.Spec` (`*ast.MetricTrigger`→"metric"; `*ast.EverySchedule`→
  "schedule_every"; `*ast.AtSchedule`→"schedule_at"; `*ast.EventTrigger`→нет строки в trigger_state).
- `EverySchedule.Every.Amount`/`.Unit` → аргументы `shiftEvery`.
- `AtSchedule.At.Value` → валидируется новой семош (R-8), парсится в `target` времени на тике.
- `EventTrigger.Event.Name` → матч с `Event.Name`.
- Тело `td.Body` (`*ast.Block`) → исполняется штатным путём движка 006 (env-scope §TR-5).
