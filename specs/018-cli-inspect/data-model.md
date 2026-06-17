# Data Model — B6 `ladix inspect` (018-cli-inspect)

Фича не вводит новых типов данных. Использует существующие `ProcessInstance`/`Task` и контракт `Store`,
добавляя РОВНО один read-only метод. Ниже — сущности (только чтение) и формат вывода.

## §1. Существующие сущности (источник снимка, не меняются)

### ProcessInstance (`store/types.go:29-37`)
| Поле | Тип | Роль в inspect |
|------|-----|----------------|
| `ID` | `string` (`p-NNNNNN`) | `инстанс <ID>` |
| `ProcessName` | `string` | `процесс <ProcessName>` |
| `Status` | `Status` (рус. строка: `ожидает`/`выполнен`/…) | `статус <Status>` (печатается как есть) |
| `CurrentStep` | `string` | `шаг '<CurrentStep>'` |
| `Variables` | `map[string]value.Value` | блок `переменные:` (`имя = value.String(v)`, порядок итерации) |

### Task (`store/types.go:48-58`, 9 полей; не меняется)
| Поле | Тип | Роль в inspect |
|------|-----|----------------|
| `ID` | `string` (`t-NNNNNN`) | `<ID>` (порядок ID ASC) |
| `InstanceID` | `string` | фильтр `ListTasksByInstance` |
| `StepName` | `string` | `шаг '<StepName>'` |
| `Assignee` | `string` | `→ <Assignee>` |
| `Deadline` | `*time.Time` | `, срок до <время>` (только при `!= nil`; layout `2006-01-02 15:04`) |
| `Status` | `TaskStatus` (`открыта`/`завершена`) | статус задачи |
| `Escalated` | `bool` (B4, D-AU-5) | суффикс `, эскалирована` тогда и только тогда, когда `true` |

`TaskStatus`: `TaskPending="открыта"`, `TaskCompleted="завершена"` (`types.go:42-45`).

## §2. Новый метод контракта `Store` (15 → 16, аддитивно §AU-2)

```go
// ListTasksByInstance — открытые + завершённые задачи инстанса, порядок ID ASC (read-only).
ListTasksByInstance(instanceID string) ([]*Task, error)
```

Добавляется в интерфейс `store.Store` (`store.go`) после `ListInstancesByStatus`. Существующие 15
сигнатур (`store.go:12-34`) НЕ трогаются. Compile-time замки `var _ Store = (*MemoryStore)(nil)` /
`(*SQLiteStore)(nil)` обязывают реализацию в обоих бэкендах.

### §2.1 Контракт поведения (оба бэкенда идентичны)
| Условие | Результат |
|---------|-----------|
| Инстанс с задачами (смешанные статусы) | ВСЕ задачи инстанса (открытые И завершённые), порядок ID ASC |
| Задачи нескольких инстансов | возвращены только задачи `instanceID` (фильтр по `InstanceID`) |
| Инстанс без задач / неизвестный id | пустой результат (`nil`/`[]`), `error == nil` |
| Задача с `Escalated==true` | поле `Escalated` сохранено в возвращённом `*Task` |
| read-only | метод НЕ мутирует Store |

### §2.2 MemoryStore (`memory.go`)
Зеркало `ListPendingTasks` (`memory.go:75`), но фильтр по инстансу вместо статуса/исполнителя:
```go
func (s *MemoryStore) ListTasksByInstance(instanceID string) ([]*Task, error) {
    s.mu.Lock(); defer s.mu.Unlock()
    var out []*Task
    for _, t := range s.tasks {
        if t.InstanceID != instanceID { continue }
        out = append(out, copyTask(t))   // copyTask копирует Escalated тривиально (bool)
    }
    sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
    return out, nil
}
```
`copyTask` (`memory.go:245`) уже копирует `Escalated` (`cp := *t`). Возврат `nil` при отсутствии задач —
допустим (`len==0`).

### §2.3 SQLiteStore (`sqlite.go`)
Зеркало `ListPendingTasks` (`sqlite.go:189`), фильтр по `instance_id`, БЕЗ фильтра статуса:
```go
func (s *SQLiteStore) ListTasksByInstance(instanceID string) ([]*Task, error) {
    rows, err := s.db.Query(
        `SELECT id, instance_id, step_name, assignee, deadline, status, created_at, completed_at, escalated
         FROM tasks WHERE instance_id = ? ORDER BY id ASC`, instanceID)
    if err != nil { return nil, err }
    defer rows.Close()
    var out []*Task
    for rows.Next() {
        // scan … → buildTask(…, escalated) — общий путь, escalated != 0 → Escalated
        out = append(out, t)
    }
    return out, rows.Err()
}
```
**КРИТИЧНО**: `escalated` в SELECT-списке + проход через `buildTask`/`scanTask` (`sqlite.go:296/310`),
иначе `Escalated` теряется (FR-013). Колонка `escalated` уже в DDL и читателях (B4); метод НЕ меняет
схему (read-only; D-AU-9 миграции не нужны).

## §3. Формат вывода `ladix inspect <id>` (§AU-10.D, exact-match)

### §3.1 Снимок (1-я строка)
```
инстанс <ID>: процесс <ProcessName>, статус <Status>, шаг '<CurrentStep>'
```

### §3.2 Переменные
```
переменные:
  <имя1> = <value.String(v1)>
  <имя2> = <value.String(v2)>
```
Отступ — 2 пробела. Порядок — итерация `inst.Variables`. Пустые переменные → только заголовок
`переменные:` без строк.

### §3.3 Задачи
```
задачи:
  <t-id> шаг '<StepName>' → <Assignee>[, срок до <время>], <открыта|завершена>[, эскалирована]
```
Отступ — 2 пробела. Порядок — `ListTasksByInstance` (ID ASC). Сегменты строки задачи (слева направо):
1. `<t-id> шаг '<StepName>' → <Assignee>` — всегда;
2. `, срок до <время>` — только при `Deadline != nil` (layout `2006-01-02 15:04`; golden маскирует);
3. `, <открыта|завершена>` — статус задачи (`Status`);
4. `, эскалирована` — только при `Escalated == true`.

Пустой список задач → только заголовок `задачи:` без строк.

### §3.4 CLI-ошибка (неизвестный инстанс, §AU-10.C)
stderr (exit 2):
```
ladix: инстанс '<id>' не найден
```

## §4. Инварианты модели
- INV-1: `ProcessRuntime`=8 НЕ трогается; eval без store/engine. inspect не использует движок.
- INV-2: Store 15→16 аддитивно; 15 старых сигнатур не тронуты; read-only метод; оба бэкенда.
- INV-3: каталог диагностик `L=11`/`SE=14`/`eval=28` не меняется (CLI-ошибки — stderr cmd).
- INV-4: единая `--db`/`openStore` без регресса остальных команд; durable(B4)/§EN-7/007b golden целы.
- INV-5: inspect read-only — Store не пишется.
