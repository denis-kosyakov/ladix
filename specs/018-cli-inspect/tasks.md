---
description: "Task list — B6 ladix inspect (018-cli-inspect)"
---

# Tasks: B6 — `ladix inspect <id>` (снимок инстанса + лёгкая история задач)

**Input**: Design documents from `/specs/018-cli-inspect/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/store-list-tasks-by-instance.md,
contracts/inspect-cli.md, quickstart.md.

**Tests**: ВКЛЮЧЕНЫ и идут ПЕРВЫМИ (tests-first, Принцип VI + практика 002–017). Поведенческие тесты
пишутся красными до impl, краснеют по мутпробе.

**Organization**: по User Story. US3 (Store-метод) — фундамент US1 (inspect его читает).

## Format: `[ID] [P?] [Story] Description`
- **[P]**: параллелизуемо (разные файлы, без зависимостей).
- Точные пути — в описании. Все тексты — ДОСЛОВНО из §AU-10.D/§AU-10.C.

---

## Фаза 0 — Базлайн и регресс-якоря (до изменений)

- [x] **T001** Зафиксировать зелёный базлайн: `cd src && go build ./... && go test ./...` (все пакеты),
  `go test -race ./internal/daemon/... ./cmd/...` (демон/CLI). Записать в PR-черновик: каталог
  `L=11`/`SE=14`/`eval=28` зелён, durable(B4)/§EN-7/007b golden зелены. Это анти-регресс якорь INV-4.
- [x] **T002** [P] Снять текущее число методов `Store` (=15) перед изменением — для замка перехода 15→16
  (зафиксировать в комментарии теста T003).

---

## Фаза 1 — US3: Store-метод `ListTasksByInstance` (тесты ПЕРВЫМИ) — P1

> Контракт: contracts/store-list-tasks-by-instance.md. Фундамент для inspect (US1).

### Тесты (красные до impl)

- [x] **T003** [P] [US3] **Счётный замок Store=16**: тест (reflect)
  `reflect.TypeOf((*store.Store)(nil)).Elem().NumMethod() == 16` в `src/internal/store/contract_test.go`
  (или `store_test.go`). **Инверсия (e)**: до добавления метода — НЕ компилируется/красный (15≠16);
  удаление любого старого метода ломает компиляцию (15 старых не тронуты).
- [x] **T004** [US3] **Контракт-тест MemoryStore** `ListTasksByInstance` в
  `src/internal/store/memory_test.go` (table-driven по C1–C6):
  - **(a) ORDER**: сохранить `t-000003`,`t-000001`,`t-000002` инстанса `p-000001` вперемешку →
    результат РОВНО `[t-000001,t-000002,t-000003]` (ID ASC). Инверсия: порядок вставки/обратный → красный.
  - **FILTER**: + `t-000010` инстанса `p-000002` → не в результате `p-000001`.
  - **MIXED**: открытая + завершённая (`MarkTaskCompleted`) → ОБЕ в результате (контраст с
    `ListPendingTasks` — только открытые).
  - **EMPTY**: инстанс без задач → `len==0`, `err==nil` (без паники).
  - **ESCALATED**: задача `Escalated=true` → поле сохранено в возвращённом `*Task`.
- [x] **T005** [US3] **Контракт-тест SQLiteStore** `ListTasksByInstance` в
  `src/internal/store/sqlite_test.go` — зеркало T004 (a/FILTER/MIXED/EMPTY) + **ESCALATED через
  персист**: сохранить задачу `Escalated=true`, прочитать `ListTasksByInstance` → `Escalated==true`
  (замок FR-013: `escalated` в SELECT). Инверсия: убрать `escalated` из SELECT → `false` → красный.

### Реализация (после красных тестов)

- [x] **T006** [US3] Добавить сигнатуру в интерфейс `src/internal/store/store.go` после
  `ListInstancesByStatus`: `ListTasksByInstance(instanceID string) ([]*Task, error)` (read-only, ID ASC).
  15 старых сигнатур (`store.go:12-34`) НЕ трогать. Обновить doc-комментарий счёта (15→16, аддитивно §AU-2).
- [x] **T007** [P] [US3] Реализовать `MemoryStore.ListTasksByInstance` в `src/internal/store/memory.go`
  (зеркало `ListPendingTasks`): фильтр `t.InstanceID == instanceID`, `copyTask`, `sort.Slice` по ID ASC.
- [x] **T008** [P] [US3] Реализовать `SQLiteStore.ListTasksByInstance` в `src/internal/store/sqlite.go`:
  `SELECT id, instance_id, step_name, assignee, deadline, status, created_at, completed_at, escalated
  FROM tasks WHERE instance_id = ? ORDER BY id ASC` → `scanTask`/`buildTask`. **`escalated` в SELECT-списке**.
  Схему НЕ менять (read-only).
- [x] **T009** [US3] Прогнать T003–T005 зелёными; `var _ Store = …` компилируется в обоих бэкендах.

---

## Фаза 2 — US1: подкоманда `inspect` (снимок + история; тесты ПЕРВЫМИ) — P1

> Контракт: contracts/inspect-cli.md. Тексты дословно §AU-10.D.

### Golden-тесты (красные до impl)

- [x] **T010** [US1] **Golden снимок+история** в `src/cmd/ladix/inspect_golden_test.go`:
  построить фикстуру (через `ladix start` ИЛИ прямой `SaveInstance`/`SaveTask` в tmp SQLite) — инстанс
  `p-000001` процесса `эскалация_плана`, статус `ожидает`, шаг `'связаться_с_клиентом'`, переменная
  `факт = 2500000`, открытая НЕ эскалированная задача `t-000001` с дедлайном. Запустить `inspectMain`
  → stdout exact-match (golden маскирует `<время>`):
  ```
  инстанс p-000001: процесс эскалация_плана, статус ожидает, шаг 'связаться_с_клиентом'
  переменные:
    факт = 2500000
  задачи:
    t-000001 шаг 'связаться_с_клиентом' → менеджер, срок до <время>, открыта
  ```
  exit 0. **Инверсия (b)**: любой сдвиг разделителя/слова/отступа → красный (exact-match).
- [x] **T011** [US1] **Замок суффикса `, эскалирована`** (тот же файл): две под-проверки —
  - **(c1) эскалированная**: `t.Escalated=true` → строка задачи оканчивается `, открыта, эскалирована`.
  - **(c2) инверсия суффикса**: `t.Escalated=false` → строка оканчивается `, открыта` БЕЗ `, эскалирована`.
  Замок ловит и пропуск суффикса, и его ложное появление.
- [x] **T012** [P] [US1] **Завершённая задача**: задача `MarkTaskCompleted` → строка содержит
  `, завершена` (НЕ `, открыта`); порядок задач — ID ASC независимо от статуса (открытая + завершённая
  в одном выводе). Инверсия: статус `открыта` у завершённой → красный.
- [x] **T013** [P] [US1] **Edge — без дедлайна**: задача `Deadline==nil` → строка
  `  t-000001 шаг '…' → менеджер, открыта` (хвост `, срок до <время>` ОТСУТСТВУЕТ). Инверсия: лишний
  `срок до` → красный.
- [x] **T014** [P] [US1] **Edge — пустые переменные/задачи**: инстанс без переменных и без задач →
  блоки `переменные:` и `задачи:` печатаются БЕЗ строк под ними (форма по §AU-10.D). exit 0.

### Реализация (после красных тестов)

- [x] **T015** [US1] Создать `src/cmd/ladix/inspect.go`: `func inspectMain(rest []string, stdout, stderr
  io.Writer) int` — разбор `--db`/`--db=` (дефолт `defaultDBPath`) + один позиционный `<id>`; нет
  `<id>` → `usage` exit 2; неизв.флаг/флаг-без-значения → паритетные тексты exit 2.
- [x] **T016** [US1] В `inspectMain`: `openStore(dbPath)` (defer close, §AU-9). Ошибка открытия →
  `ladix: не удалось открыть хранилище '<path>': <err>` exit 2 (паритет start). Обернуть тело в
  `guard(stderr, …)`. **БЕЗ** `NewEngine`/`NewInterpreter` (INV-1; FR-005).
- [x] **T017** [US1] Чтение: `st.LoadInstance(id)` (`errors.Is ErrInstanceNotFound` → US2 ветка) +
  `st.ListTasksByInstance(id)`. read-only: НИКАКИХ `SaveInstance`/`SaveTask` (FR-014).
- [x] **T018** [US1] Принтер снимка (в `inspect.go`, СВОЙ формат — НЕ `FormatTaskLine`): 1-я строка
  `инстанс <id>: процесс <ProcessName>, статус <Status>, шаг '<CurrentStep>'`; блок `переменные:` +
  `  имя = value.String(v)` (порядок `inst.Variables`); блок `задачи:` + строка задачи
  `  <t-id> шаг '<StepName>' → <Assignee>[, срок до <время>], <открыта|завершена>[, эскалирована]`
  (дедлайн только при `!= nil`, layout `2006-01-02 15:04`; суффикс только при `Escalated`).
- [x] **T019** [US1] `cmd/ladix/main.go`: switch `case "inspect": return inspectMain(args[1:], stdout,
  stderr)`; РАСШИРИТЬ `usage`-строку записью `ladix inspect <id> [--db путь]`.
- [x] **T020** [US1] Прогнать T010–T014 зелёными.

---

## Фаза 3 — US2: неизвестный инстанс (тесты ПЕРВЫМИ) — P1

- [x] **T021** [US2] **Golden неизв.инстанс** в `inspect_golden_test.go`: `inspectMain ["p-999999",
  "--db", <db>]` над БД без `p-999999` → stderr РОВНО `ladix: инстанс 'p-999999' не найден\n`, exit 2,
  stdout пуст. **Инверсия (d)**: иной текст/код/непустой stdout → красный. (Импл — в T017: ветка
  `errors.Is(err, store.ErrInstanceNotFound)` → `fmt.Fprintf(stderr, "ladix: инстанс '%s' не найден\n", id)`.)
- [x] **T022** [US2] Подтвердить трансляцию сентинела: английский `ErrInstanceNotFound` НЕ печатается
  (только русский §AU-10.C). Прогнать T021 зелёным.

---

## Фаза 4 — Регресс, инварианты, полировка

- [x] **T023** **(f) Единая `--db` без регресса**: прогнать `go test ./...` целиком + существующие
  golden (`start_golden_test`, `trigger_golden_test`, `serve_golden_test`, durable B4, emit/tasks) —
  все зелёные, БЕЗ изменений их golden. INV-4. Если `inspect` использует `openStore`, проверить, что
  `start`/прочие команды не задеты.
- [x] **T024** [P] **Замки каталога/инвариантов**: подтвердить `L=11`/`SE=14`/`eval=28` не изменились
  (INV-3); `ProcessRuntime`=8 (eval/runtime.go) не тронут; пустой дифф `internal/eval`/`internal/engine`
  (INV-1). `git diff --stat` по этим путям = пусто (кроме, возможно, ничего).
- [x] **T025** [P] **Read-only замок** (INV-5): тест/проверка, что после `inspect` число записей/строк
  Store не изменилось (например, снимок `tasks`/`instances` до и после inspect идентичен) ИЛИ
  ревью-аргумент, что `inspectMain` не вызывает пишущих методов.
- [x] **T026** quickstart-смоук: воспроизвести демо-пару B5→B6 (`ladix start … --db demo.db` →
  `ladix inspect p-000001 --db demo.db`) — соответствие quickstart.md (опц. как smoke-тест).
- [x] **T027** Финал: `go build ./... && go vet ./... && go test ./...` зелёные; нет новых зависимостей
  (`go.mod`/`go.sum` без изменений); детерминизм golden (повторный прогон стабилен).

---

## Зависимости / порядок
- Фаза 0 → 1 → 2 → 3 → 4.
- T003–T005 (красные) ПЕРЕД T006–T009. T007/T008 — [P] (разные файлы).
- US3 (Фаза 1) — фундамент US1: T010 читает `ListTasksByInstance`, потому Фаза 1 завершена до T020.
- T010–T014 (красные) ПЕРЕД T015–T019. T021 (красный) ПЕРЕД/вместе с T017-веткой.
- Регресс-якоря (T001/T023/T024) обрамляют изменения.

## Инверсионные замки (сводка a–f)
| ID | Замок | Где |
|----|-------|-----|
| (a) | `ListTasksByInstance` ID ASC оба бэкенда (+ порядок-вперемешку) | T004/T005 |
| (b) | inspect канон exact-match (маска `<время>`) — красн. при сдвиге | T010 |
| (c) | `, эскалирована` только при `Escalated` (замок + инверсия суффикса) | T011 |
| (d) | неизв.инстанс дословно `ladix: инстанс '<id>' не найден` exit 2 | T021 |
| (e) | Store=16 (счёт) + 15 старых не тронуты | T003 |
| (f) | единая `--db` без регресса | T023 |

## Итого
**27 задач**, 5 фаз. Тесты-первыми (Фазы 1–3 начинаются с красных). Слой: CLI (`cmd/ladix`) + 1
read-only метод Store × 2 бэкенда. Пустой дифф eval/engine; 0 новых зависимостей; детерминизм golden.
