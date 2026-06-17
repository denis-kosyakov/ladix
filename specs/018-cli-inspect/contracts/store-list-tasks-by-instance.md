# Contract — `Store.ListTasksByInstance` (B6, новый read-only метод)

Источник: §AU-2 (контракт Store 15→16), §AU-8 (B6), D-AU-8. Аддитивный инвариант.

## Сигнатура

```go
// ListTasksByInstance — открытые + завершённые задачи инстанса, порядок ID ASC (read-only).
ListTasksByInstance(instanceID string) ([]*Task, error)
```

- Добавляется в интерфейс `store.Store` (`internal/store/store.go`) после `ListInstancesByStatus`.
- Число методов: **15 → 16**. Существующие 15 сигнатур (`store.go:12-34`) НЕ меняются (§AU-2).
- Возврат — `[]*Task` (slice указателей), зеркало `ListPendingTasks`/`ListInstancesByStatus`.
  (Бриф упоминал `[]Task` — расходится с якорем §AU-2; контракт следует якорю.)

## Семантика (оба бэкенда идентичны)

| # | Условие | Ожидание |
|---|---------|----------|
| C1 | задачи инстанса со смешанными статусами | возвращены ВСЕ (открытые И завершённые) |
| C2 | задачи нескольких инстансов | возвращены только задачи `instanceID` (фильтр `InstanceID`) |
| C3 | порядок | по `ID` ASC (лексикографически `t-NNNNNN`) |
| C4 | инстанс без задач / неизвестный id | `nil`/`[]`, `error == nil` (НЕ ошибка) |
| C5 | задача `Escalated==true` | поле `Escalated` сохранено в возвращённом `*Task` |
| C6 | read-only | Store НЕ мутируется (никаких записей) |

## Реализация

### MemoryStore (`memory.go`)
Зеркало `ListPendingTasks`: итерация `s.tasks`, фильтр `t.InstanceID == instanceID` (БЕЗ фильтра
статуса), `copyTask` (копирует `Escalated`), `sort.Slice(out, out[i].ID < out[j].ID)`.

### SQLiteStore (`sqlite.go`)
```sql
SELECT id, instance_id, step_name, assignee, deadline, status, created_at, completed_at, escalated
FROM tasks WHERE instance_id = ? ORDER BY id ASC
```
Построчно `scanTask`/`buildTask` (общий путь, `sqlite.go:296/310`). **`escalated` ОБЯЗАН быть в
SELECT-списке** и пройти через `buildTask` (`Escalated = escalated != 0`), иначе поле теряется (FR-013).
Схема НЕ меняется (read-only; колонка `escalated` уже есть с B4).

## Compile-time замки
`var _ Store = (*MemoryStore)(nil)` / `var _ Store = (*SQLiteStore)(nil)` (`store.go:37-40`) — нереализация
в любом бэкенде → ошибка компиляции.

## Тест-замки (tests-first, оба бэкенда — table-driven)
| ID | Замок | Инверсия (красное при) |
|----|-------|------------------------|
| T-LTBI-ORDER | сохранить `t-000003,t-000001,t-000002` вперемешку → результат `[t-000001,t-000002,t-000003]` | возврат в порядке вставки/обратном |
| T-LTBI-FILTER | + `t-000010` инстанса `p-000002` → не возвращается для `p-000001` | утечка чужих задач |
| T-LTBI-MIXED | открытая + завершённая → обе в результате | фильтрация только открытых (как ListPendingTasks) |
| T-LTBI-EMPTY | инстанс без задач → `len==0`, `err==nil` | ошибка/паника на пустом |
| T-LTBI-ESCALATED | задача `Escalated=true` сохранена и читается из SQLite | `escalated` выпал из SELECT → `false` |
| T-STORE-COUNT-16 | `reflect.TypeOf((*Store)(nil)).Elem().NumMethod() == 16` | сдвиг при удалении/недобавлении метода |

## Инварианты
- INV-2: аддитивно; 15 старых сигнатур не тронуты; read-only; оба бэкенда.
- 0 новых зависимостей; детерминизм (ID-сорт стабилен).
