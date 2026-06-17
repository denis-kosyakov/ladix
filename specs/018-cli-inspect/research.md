# Research — B6 `ladix inspect` (018-cli-inspect)

Цель: подтвердить прецеденты и эмпирические строки, на которых строится B6, чтобы план/контракты не
выдумывали API. Все факты сверены по коду на `master` HEAD=`5cfa9d3` (B1–B5 влиты).

## R1. Чтение Store напрямую без движка — прецедент `tasksMain`/`listTasks`

`cmd/ladix/main.go:503 tasksMain` разбирает `--db` (дефолт `defaultDBPath="ladix.db"`, `main.go:64`),
открывает Store, читает `ListPendingTasks` и печатает — БЕЗ `engine`/`eval`. `listTasks`
(`main.go:540`) обёрнута `guard(stderr, …)` (recover-барьер, конституция III) и при `len==0` печатает
`открытых задач нет`. **Вывод**: `inspectMain` строится по тому же скелету: разбор `--db` + позиционный
`<id>`, `openStore`, прямое чтение, `guard`-барьер. Никакого `NewEngine`/`NewInterpreter`.

> Нюанс: `listTasks` конструирует `NewSQLiteStore` напрямую (старый код), но B5 ввёл `openStore`
> (см. R3). `inspect` ДОЛЖЕН использовать `openStore` (§AU-9), а не дублировать конструкцию.

## R2. `LoadInstance` и сентинел `ErrInstanceNotFound`

- `store.go:14`: `LoadInstance(id string) (*ProcessInstance, error)` — «не найден → ErrInstanceNotFound».
- `memory.go:48-56`: `MemoryStore.LoadInstance` возвращает `ErrInstanceNotFound` при отсутствии (copyInstance иначе).
- Сентинел: `types.go:86 ErrInstanceNotFound = errors.New("process instance not found")` (английский,
  наружу НЕ печатается; CLI транслирует в русский §AU-10.C).

**Вывод**: `inspectMain` после `LoadInstance(id)` проверяет `errors.Is(err, store.ErrInstanceNotFound)`
→ печатает `ladix: инстанс '<id>' не найден` (stderr) exit 2 (§AU-10.C). Прочие ошибки — `ladix: сбой
хранилища: …` или паритетный текст (как `listTasks` `ladix: сбой хранилища`).

## R3. Хелпер `openStore` (B5, §AU-9)

`start.go:282`:
```go
func openStore(dbPath string) (st store.Store, closeFn func() error, err error) {
    if dbPath == "" { return store.NewMemoryStore(), func() error { return nil }, nil }
    sq, oerr := store.NewSQLiteStore(dbPath)
    if oerr != nil { return nil, nil, oerr }
    return sq, sq.Close, nil
}
```
Возвращает `store.Store` (интерфейс) + `closeFn` (defer). `inspectMain` дефолтит `dbPath=defaultDBPath`
(`"ladix.db"`, непуст → SQLite), как `start`. **Вывод**: переиспользуем `openStore` напрямую (он уже
в пакете `cmd/ladix`); не вводим новый метод Store (INV-1).

## R4. `ListPendingTasks` (memory + sqlite) — шаблон нового метода

- `memory.go:75 ListPendingTasks`: итерация `s.tasks`, фильтр статуса/исполнителя, `copyTask`,
  `sort.Slice(out, func(i,j) bool { return out[i].ID < out[j].ID })`. **`ListTasksByInstance`** = тот же
  скелет, но фильтр `t.InstanceID == instanceID` (БЕЗ фильтра статуса — открытые И завершённые).
- `sqlite.go:189 ListPendingTasks`: `SELECT id, instance_id, step_name, assignee, deadline, status,
  created_at, completed_at, escalated FROM tasks WHERE status = ? ORDER BY id ASC` → построчно
  `scanTask`/`buildTask`. **`ListTasksByInstance`** = `… WHERE instance_id = ? ORDER BY id ASC` (без
  фильтра статуса), та же колонка-список (включая `escalated`), тот же `buildTask`.

**Критично (FR-013)**: `escalated` уже в SELECT-списке `ListPendingTasks` и в `buildTask`
(`sqlite.go: t.Escalated = escalated != 0`). Новый метод обязан включить `escalated` в SELECT и пройти
через `buildTask`, иначе поле `Escalated` потеряется молча → суффикс `, эскалирована` не отрисуется.

## R5. `value.String` — формат значений переменных

`value/repr.go:20 String(v Value) string`: Целое→десятичная (`2500000`), Дробное→formatFloat,
Строка→текст без кавычек, Булево→`истина`/`ложь`, Пусто→`пусто`, Список/Запись рекурсивно. **Вывод**:
строка переменной `  факт = 2500000` — `имя + " = " + value.String(v)`. Порядок переменных — порядок
итерации `inst.Variables` (`map[string]value.Value`; в коде — детерминизм обеспечивает фикстура/golden;
канон §AU-10.D говорит «в порядке `inst.Variables`»).

## R6. Формат строки задачи и `<время>` дедлайна — `FormatTaskLine`

`engine/format.go`: `deadlineLayout = "2006-01-02 15:04"` (локальная зона); `срок до <время>` —
ТОЛЬКО при `Deadline != nil`. Golden команд (`run`/`tasks`) маскируют `<время>`→`<DT>`.

**Важно — НЕ переиспользуем `FormatTaskLine`**: его форма (`<t-id>  <p-id>  '<шаг>'  <исполнитель>
срок до <время>  ПРОСРОЧЕНА`, два пробела, с `InstanceID` и хвостом ПРОСРОЧЕНА) — это формат `§EN-7`
для `tasks`/`run`, ОТЛИЧАЕТСЯ от канона inspect §AU-10.D
(`  <t-id> шаг '<шаг>' → <исполнитель>, срок до <время>, <статус>[, эскалирована]`). inspect печатает
СВОЙ формат (engine не зависит, inspect — cmd-слой). Но формат `<время>` — тот же `deadlineLayout`,
так что маска golden совместима.

## R7. Контракт Store: число методов 15→16

`store.go:12-34` — интерфейс перечисляет 15 сигнатур (8 core 006 + 6 trigger + 1 рестарт-скан
`ListInstancesByStatus`; комментарий «8 методов» в шапке — историческое описание core-набора, НЕ счёт
интерфейса). §AU-2 явно: «существующие 15 … не трогаются», `ListTasksByInstance` — 16-й. Compile-time
замки `var _ Store = (*MemoryStore)(nil)` / `(*SQLiteStore)(nil)` (`store.go:37-40`) поймают
нереализацию в любом бэкенде. **Вывод**: добавить сигнатуру в интерфейс + impl в обоих; счётный замок
теста (reflect по `Store`) = 16.

## R8. CLI-ошибка дословно (§AU-10.C)

`ladix: инстанс '<id>' не найден` (stderr, exit 2). Паритет существующих CLI-ошибок семьи (B3/B5):
`fmt.Fprintf(stderr, "ladix: инстанс '%s' не найден\n", id)`.

## R9. stdout канон inspect дословно (§AU-10.D)

```
инстанс p-000001: процесс эскалация_плана, статус ожидает, шаг 'связаться_с_клиентом'
переменные:
  факт = 2500000
задачи:
  t-000001 шаг 'связаться_с_клиентом' → менеджер, срок до <время>, открыта, эскалирована
```
- 1-я строка: `инстанс <id>: процесс <ProcessName>, статус <Status>, шаг '<CurrentStep>'`.
- `переменные:` + по `  имя = value.String(v)` в порядке `inst.Variables`.
- `задачи:` + по задаче из `ListTasksByInstance` (ID ASC):
  `  <t-id> шаг '<StepName>' → <Assignee>[, срок до <время>], <открыта|завершена>[, эскалирована]`.
- `, эскалирована` — только при `t.Escalated`; статус — `открыта`/`завершена`.

## R10. Детерминизм / golden

`<время>` дедлайна маскируется в golden (как у `start`/`tasks`). id (`p-000001`/`t-000001`)
детерминированы счётчиками Store. Фикстуру можно построить через `ladix start` (R1 demo) или прямой
`SaveInstance`/`SaveTask` в тесте. Регресс-проверка: существующие golden (`start_golden_test`,
`trigger_golden_test`, `serve_golden_test`, durable B4) не должны измениться от добавления метода.

## Резюме открытых вопросов

| Вопрос | Решение |
|--------|---------|
| Сигнатура `[]Task` vs `[]*Task`? | `[]*Task` — дословно §AU-2 (зеркало `ListPendingTasks`); бриф `[]Task` неточен. |
| Переиспользовать `FormatTaskLine`? | НЕТ — другой формат (§EN-7 ≠ §AU-10.D). inspect — свой принтер. |
| Форма блоков при пустых списках задач/переменных? | Заголовок печатается; строк нет. Закреплено в contracts/inspect-cli.md по §AU-10.D. |
| Где счётный замок Store=16? | `internal/store` тест (reflect). |
