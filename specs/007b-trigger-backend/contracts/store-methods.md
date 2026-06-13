# Contract — Store: 7 аддитивных методов + 2 таблицы (007b)

**Anchor**: EM-17.2/EM-17.3 (структуры/DDL), engine-model D-3/D-4 (контракт), store.go:9–13
(предобъявленные имена 6 методов). FR-021/022/023, SC-009.

> Аддитивно к `internal/store`. **8 методов и 3 таблицы (instances/tasks/counters) 006 НЕ меняются.**
> Паритет: единый контрактный table-driven тест прогоняется на `*MemoryStore` И `*SQLiteStore`.

## Интерфейс (`store.go`)

Добавить в `type Store interface` (после 8 существующих). Комментарий-шапку интерфейса обновить:
«+6 триггерных методов **+ `ListInstancesByStatus`** (рестарт-скан, FR-022)».

```go
// --- триггерные методы (007b, аддитивно; обещание engine-model «+6») ---
LoadTriggerState(triggerID string) (*TriggerState, error) // не найдено → ErrTriggerStateNotFound
SaveTriggerState(ts *TriggerState) error                  // upsert
NextEventID() (string, error)                             // mint "e-NNNNNN"
EnqueueEvent(e *Event) error                              // запись в очередь (Processed=false)
ListUnprocessedEvents() ([]*Event, error)                 // FIFO по CreatedAt; пусто → []
MarkEventProcessed(id string) error                       // идемпотентно

// --- рестарт-скан (007b, ОСОЗНАННОЕ ОТСТУПЛЕНИЕ «+6→+7», FR-022) ---
ListInstancesByStatus(status string) ([]*ProcessInstance, error) // по возрастанию ID; пусто → []
```

Compile-time проверки `_ Store = (*MemoryStore)(nil)` / `(*SQLiteStore)(nil)` — остаются (store.go:28).

## Сентинел

```go
var ErrTriggerStateNotFound = stderrors.New("состояние триггера не найдено") // EM-17.2; errors.Is
```

## Семантика каждого метода (контракт паритета)

| Метод | Вход | Выход | Memory | SQLite |
|---|---|---|---|---|
| `LoadTriggerState` | triggerID | `*TriggerState` / `ErrTriggerStateNotFound` | карта под mu; miss→сентинел | `SELECT … WHERE trigger_id=?`; `sql.ErrNoRows`→сентинел |
| `SaveTriggerState` | `*TriggerState` | `error` | upsert копии под mu | `INSERT … ON CONFLICT(trigger_id) DO UPDATE` |
| `NextEventID` | — | `"e-NNNNNN"` | `eventSeq++` под mu, `fmt.Sprintf("e-%06d", n)` | атомарный mint счётчика `event` (зеркало NextInstanceID) |
| `EnqueueEvent` | `*Event` | `error` | append копии под mu | `INSERT INTO events(…)` (WAL сериализует с serve) |
| `ListUnprocessedEvents` | — | `[]*Event` (FIFO) | фильтр `!Processed`, копии в порядке среза | `SELECT … WHERE processed=0 ORDER BY created_at, id` |
| `MarkEventProcessed` | id | `error` (идемпотентно) | `Processed=true` под mu | `UPDATE events SET processed=1 WHERE id=?` |
| `ListInstancesByStatus` | status | `[]*ProcessInstance` (по ID) | фильтр по `Status`, копии, сорт по ID | `SELECT … WHERE status=? ORDER BY id` |

**Кодирование (SQLite, codec.go-стиль 006):** `LastBool *bool` → `last_bool INTEGER` (0/1/NULL);
`LastFire *time.Time` → RFC3339 / NULL; `LastFiredDate *string` → как есть / NULL; `Event.CreatedAt` →
RFC3339. Без алиасинга указателей (Save/Load копируют — зеркало copyInstance/copyTask 006).

## DDL (добавить к `ddl`, sqlite.go:23 — НЕ менять instances/tasks/counters)

```sql
CREATE TABLE IF NOT EXISTS trigger_state (
    trigger_id      TEXT PRIMARY KEY,
    kind            TEXT NOT NULL,
    last_bool       INTEGER,
    last_fire       TEXT,
    last_fired_date TEXT
);
CREATE TABLE IF NOT EXISTS events (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    processed    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_events_pending ON events(processed, created_at);
INSERT OR IGNORE INTO counters(name, value) VALUES ('event', 0);
```

## Контрактные тесты (паритет Memory+SQLite, table-driven)

Единый набор кейсов, прогоняемый на обеих реализациях (как 006):
1. `LoadTriggerState` на пустом Store → `ErrTriggerStateNotFound` (`errors.Is`).
2. `SaveTriggerState` → `LoadTriggerState` round-trip: все поля (вкл. nil-указатели) совпадают.
3. `SaveTriggerState` upsert: повторный Save с новым `LastBool` обновляет, не дублирует.
4. `NextEventID` монотонен: `e-000001`, `e-000002`, … (SQLite: переживает переоткрытие БД).
5. `EnqueueEvent` ×3 → `ListUnprocessedEvents` отдаёт 3 в FIFO-порядке создания.
6. `MarkEventProcessed(id)` → событие исчезает из `ListUnprocessedEvents`; повтор — no-op без ошибки.
7. `ListUnprocessedEvents` на пустой очереди → пустой срез (не nil-ошибка).
8. `ListInstancesByStatus("выполняется")` → только инстансы этого статуса, по возрастанию ID;
   `ожидает`-инстансы не попадают в выборку по «выполняется».
9. **Регресс:** 8 методов 006 и таблицы instances/tasks/counters не задеты (существующие тесты Store
   зелёные).
