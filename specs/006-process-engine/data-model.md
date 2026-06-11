# Phase 1 — Data Model: Движок исполнения процессов + человек-в-цикле (006)

**Feature**: 006-process-engine | **Date**: 2026-06-11 | **Plan**: [plan.md](./plan.md)

Описывает сущности данных фичи 006 на уровне **контракта данных**: `ProcessInstance`/`Task`/статусы
и интерфейс `Store` (пакет `internal/store`), персистентный mint id (D-10), значение `Длительность`
(`internal/value`, активация D-7/D-17), часы движка `engine.Clock` (D-2), кодек значений type-tagged
JSON (внутреннее дело `SQLiteStore`). Поля и DDL — **дословно** из якоря §EN-2; машина статусов и
гранулярность персиста — §EN-3. Контракты интерфейсов (`Store`, `ProcessRuntime`, пакет `engine`),
CLI и реестры stdout/диагностик — в [contracts/](./contracts/). Публичные Go-имена — английские; статусы
и тексты сообщений — русские (наружу печатаются русские).

**Источник истины — `docs/engine-model.md` §EN-0…§EN-10, решения Q1–Q3/D-1…D-22; при любом расхождении
побеждает он (§EN-2 для полей/DDL/кодека, §EN-3 для машины состояний/гранулярности персиста).**
Семантический фон — `SPEC.md §4` (типы Длительность/Дата), `§11` (семантика процессов), `§12` (границы
версии); база механики lifecycle — `docs/execution-model.md` (EM-2/EM-3/EM-4/EM-9/EM-10). Эталон
стиля/глубины — `specs/005-process-frontend/data-model.md`.

> **Зависимости пакетов (§EN-4).** Типы данных и интерфейс `Store` живут в `internal/store` (рядом с
> контрактом — методы интерфейса их принимают); `internal/engine` владеет только **поведением** (импортирует
> `eval`, `store`, `ast`, `value`, `errors`). **eval НЕ импортирует ни `store`, ни `engine`** — цикл
> разорван интерфейсом `ProcessRuntime`, объявленным в `eval` (D-1). Значение `Длительность` и кодек
> значений — `internal/value` (тип) и `internal/store` (кодек); движок про JSON/SQL не знает (путь замены
> бэкенда открыт, EM-4). Сентинелы `Store` — **английские** (наружу не печатаются, транслируются в русские
> тексты §EN-8).

---

## 1. `ProcessInstance` — экземпляр процесса (§EN-2, EM-2)

Конкретный запуск процесса. Размещение типа — `internal/store` (EM-2/EM-3, без изменений по полям).

### Поля (дословно §EN-2)

```go
type ProcessInstance struct {
    ID          string                 // "p-NNNNNN" (D-10)
    ProcessName string                 // имя ProcessDecl
    Status      Status
    CurrentStep string                 // имя активного шага; при терминале — последний обработанный
    Variables   map[string]value.Value // переменные процесса; пусть-локали шага сюда НЕ попадают
    CreatedAt   time.Time              // engine-Clock (D-2)
    UpdatedAt   time.Time              // выставляет движок перед КАЖДЫМ SaveInstance
}
```

| Поле | Тип | Семантика | FR |
|---|---|---|---|
| `ID` | `string` | id инстанса `p-NNNNNN` (нуль-паддинг 6, D-10); выдаёт `NextInstanceID()` | FR-002 |
| `ProcessName` | `string` | имя `ProcessDecl`; lookup определения через `interp.Process(name)` (§EN-4) | FR-008 |
| `Status` | `Status` | статус машины (см. §1.2); строка-значение русская | FR-009 |
| `CurrentStep` | `string` | имя активного шага; при терминале — последний обработанный | FR-009/FR-013 |
| `Variables` | `map[string]value.Value` | переменные процесса: параметры при создании + `присвоить`; пусть-локали шага **не попадают** | FR-009/FR-020 |
| `CreatedAt` | `time.Time` | момент создания инстанса (engine-Clock, D-2) | FR-017 |
| `UpdatedAt` | `time.Time` | момент последней записи; движок выставляет `= clock.Now()` **перед каждым** `SaveInstance` | FR-009/FR-017 |

### 1.1. Тип `Status` (§EN-2, дословно)

```go
type Status string

const (
    StatusCreated   Status = "создан"      // персистирован, первый шаг ещё не активирован (транзиентно)
    StatusRunning   Status = "выполняется" // активный шаг исполняет тело
    StatusWaiting   Status = "ожидает"     // активный шаг создал Task, инстанс спит
    StatusDone      Status = "выполнен"    // все шаги готовы (терминал)
    StatusFailed    Status = "провален"    // runtime-ошибка шага/атрибута (терминал)
    StatusCancelled Status = "отменён"     // зарезервирован; в v1 недостижим (SPEC §12)
)
```

### 1.2. Машина статусов (SPEC §11.1 / §EN-3; FR-009)

```
                  ┌──────────── complete (есть следующий шаг) ───────────┐
                  ▼                                                       │
создан ──────► выполняется ──────► ожидает ─────────────────────────────►┘
(транзиентно)   │   ▲ (человеческий шаг создал Task → заснуть, EM-10)
               │   │
               │   └──── complete (нет следующего шага) ──┐
               │                                          ▼
               └────────── (нет следующего шага) ──────► выполнен  (терминал, тихо)

   любой шаг ── runtime-ошибка тела/атрибута ──► провален  (терминал, D-14)

   отменён  — зарезервирован, в v1 недостижим (SPEC §12)
```

- **создан → выполняется**: `Start` создаёт инстанс `создан` (транзиентно, ▼SaveInstance), затем
  `advance` ставит `выполняется` и исполняет тело (§EN-3).
- **выполняется → ожидает**: человеческий шаг (задан `исполнитель`) создал `Task` → инстанс засыпает
  (EM-10), персист `ожидает`.
- **ожидает ↔ выполняется**: `Complete` будит инстанс; при наличии следующего шага идёт через
  `выполняется` (продвижение), при отсутствии — переходит в `выполнен` **напрямую** (§EN-3:
  `complete`, ветка «next == ∅»).
- **→ выполнен**: шагов больше нет (терминал); печати нет (тихо, FR-013/FR-036).
- **→ провален**: необработанная runtime-ошибка тела или атрибута (D-9/D-14); терминал.
- **отменён**: зарезервирован, недостижим в v1 (SPEC §12).

### 1.3. Инварианты

| Инвариант | Правило | Где обеспечивается |
|---|---|---|
| Активен ровно один шаг | в любой момент `CurrentStep` указывает на единственный активный шаг; человеческий шаг = одна открытая `Task` | `advance`/`Complete` (§EN-3) |
| Порядок шагов = порядок исходника | `advance` берёт «следующий шаг по исходнику» (SPEC §11.2); `после` — валидатор 005, не переупорядочиватель | `advance` lookup (§EN-3) |
| Персист на каждую смену статуса/шага | ▼`SaveInstance` на создание, каждую смену `Status`/`CurrentStep`, каждое `присвоить` (хук §EN-4), терминал; перед каждым ▼ — `UpdatedAt = clock.Now()` | `advance`/хук `AssignProcessVar` (EM-9) |
| Пусть-локали шага НЕ в `Variables` | канал записи в `Variables` — только хук `присвоить` (`AssignProcessVar`); снапшот слоя при засыпании **не снимается** (иначе `x = E` мимо хука и `пусть`-локали утекли бы в БД, §EN-4/FR-020) | хук `AssignProcessVar` (§EN-4) |
| `присвоить` в цикле ⇒ много мелких записей | `присвоить` в `пока` даёт серию ▼`SaveInstance` — принято в v1 (WAL тянет; батчинг — v2) | граница §EN-10 |

### 1.4. Правила валидации

- Имя процесса гарантированно разрешено семантикой 005 (§PM-4) — `interp.Process(ProcessName)` при
  `Start` всегда находит определение; на путях `complete` оно проверяется **дрейф-гардами Q3** (процесс
  и текущий шаг найдены в поданном файле, §EN-3) — нарушение → CLI-ошибка exit 2 (§EN-8.B).
- `Variables` строится из **позиционной связки** `pd.Params` и аргументов запуска (`bind`, §EN-3).
- Без алиасинга в `MemoryStore`: `Save`/`Load` копируют инстанс и карту `Variables` (значения
  разделяются — ссылочность Список/Запись, FR-007).

---

## 2. `Task` — задача (точка ожидания человека) (§EN-2, EM-3)

Создаётся при входе в **человеческий** шаг (задан `исполнитель`). Размещение — `internal/store`.

### Поля (дословно §EN-2)

```go
type TaskStatus string

const (
    TaskPending   TaskStatus = "открыта"
    TaskCompleted TaskStatus = "завершена"
)

type Task struct {
    ID          string     // "t-NNNNNN" (D-10)
    InstanceID  string     // → ProcessInstance.ID
    StepName    string     // шаг, породивший задачу
    Assignee    string     // значение «исполнитель» (Строка, D-18)
    Deadline    *time.Time // CreatedAt + «срок» (D-19); nil, если «срок» не задан
    Status      TaskStatus
    CreatedAt   time.Time
    CompletedAt *time.Time // nil, пока открыта; выставляет MarkTaskCompleted (D-12)
}
```

| Поле | Тип | Семантика | FR |
|---|---|---|---|
| `ID` | `string` | id задачи `t-NNNNNN` (D-10); выдаёт `NextTaskID()` | FR-002 |
| `InstanceID` | `string` | ссылка на `ProcessInstance.ID` (FK в SQLite) | FR-013 |
| `StepName` | `string` | шаг, породивший задачу; сверяется гардом D-8 с `CurrentStep` | FR-015 |
| `Assignee` | `string` | значение атрибута `исполнитель` (обязан `Строка`, D-18); в v1 — совещательная метка, реестра ролей нет | FR-011 |
| `Deadline` | `*time.Time` | абсолютизированный момент `CreatedAt + срок` (D-19); `nil`, если `срок` не задан | FR-012 |
| `Status` | `TaskStatus` | `открыта`/`завершена` (русские значения) | FR-003 |
| `CreatedAt` | `time.Time` | момент создания задачи (engine-Clock) | FR-012/FR-017 |
| `CompletedAt` | `*time.Time` | `nil`, пока открыта; `MarkTaskCompleted` выставляет момент завершения (D-12) | FR-003 |

### 2.1. Признак «просрочена» — вычислимый, НЕ статус (D-22, EM-13; FR-017)

«Просрочена» — **производный** признак, в `Task` не хранится. Источник истины — функция движка:

```go
// Overdue: now.After(*t.Deadline); при nil-дедлайне — false (EM-13).
func Overdue(t *store.Task, now time.Time) bool
```

- `now` позже `Deadline` → просрочена; `Deadline == nil` → **не** просрочена.
- `now` берётся от часов движка (D-2): `tasks` и сводка `run` — через `engine.SystemClock{}.Now()`.
- Используется хвостом `ПРОСРОЧЕНА` строки задачи (§EN-7 строка 6, через `FormatTaskLine`) и полем
  Записи в `задачи_пользователя` (EM-13).

### 2.2. Абсолютизация дедлайна (D-19; FR-012)

`Deadline = Task.CreatedAt + срок` — **внутренняя Go-механика** (не Ladix-арифметика; clamp-семантика
SPEC §4 на неё **не** распространяется). Единицы `Длительность` → Go-время:

| Единица | Множитель / операция |
|---|---|
| `сек` | `n * time.Second` |
| `мин` | `n * time.Minute` |
| `час` | `n * time.Hour` |
| `дн` | `n * 24h` (фиксированный) |
| `нед` | `n * 168h` (фиксированный) |
| `мес` | `CreatedAt.AddDate(0, n, 0)` (календарный) |

Шаг без `срок` → задача без дедлайна (`Deadline == nil`).

### 2.3. Инварианты и валидация

| Инвариант | Правило | Где |
|---|---|---|
| Атомарность завершения | `открыта → завершена` атомарно (`MarkTaskCompleted`, D-12); повтор → `ErrTaskAlreadyCompleted` | `Store` (§3) |
| Порядок листинга | `ListPendingTasks` — открытые задачи **по возрастанию `ID`** (D-15) | `Store` (§3) |
| Без алиасинга (Memory) | `Save`/`Load` копируют `Task`; мутации снаружи не видны до следующего `Save` | `MemoryStore` (FR-007) |
| `CompletedAt` парность | `Status == завершена` ⟺ `CompletedAt != nil` | `MarkTaskCompleted` (D-12) |

---

## 3. `Store` — контракт хранилища (§EN-2; D-3)

Нарезанный контракт: **ровно 8 методов** (D-3); листингов инстансов и триггерных методов нет (007
добавит аддитивно). Две реализации за одним интерфейсом: `MemoryStore` (эфемерно, без алиасинга) и
`SQLiteStore` (персистентно). Полный контракт сигнатур и семантики — [contracts/store.md](./contracts/store.md).

### Интерфейс (дословно; ровно 8 методов, D-3)

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

- **Сентинелы** — английские (наружу не печатаются; транслируются в русские §EN-8.B: `ErrTaskNotFound`
  → `задача '<id>' не найдена`, `ErrInstanceNotFound` → `инстанс '<id>' не найден`,
  `ErrTaskAlreadyCompleted` → `задача '<id>' уже завершена` / ветка догона D-4).
- Транзакционного комбо-метода «завершить + продвинуть» **нет** намеренно — корректность сбойного окна
  обеспечивает идемпотентный гард-догон D-4 (§EN-2).
- FR-001 (контракт 8 операций), FR-003 (атомарное завершение), FR-004 (различимость отказов).

### 3.1. `MemoryStore` (FR-007)

`internal/store/memory.go`. Конструктор — **`func NewMemoryStore() *MemoryStore`**. Карты
`map[string]*ProcessInstance`/`map[string]*Task` + счётчики id под одним `sync.Mutex`. JSON не трогает
(значения лежат как есть). **Без алиасинга:** `Save`/`Load` копируют инстанс, карту `Variables` и задачу
(значения разделяются — ссылочность Список/Запись как в `Locals()`); `Load` возвращает копию.
`MarkTaskCompleted` — проверка статуса и перевод под тем же mutex. Назначение — `ladix run` без `--db`,
`metric` и тесты lifecycle.

### 3.2. `SQLiteStore` (FR-005)

`internal/store/sqlite.go`, `modernc.org/sqlite` (чистый Go, без CGO — **первая внешняя зависимость**,
go.mod + go.sum отдельным коммитом, FR-041). Сигнатуры:

```go
func NewSQLiteStore(path string) (*SQLiteStore, error) // открывает БД, явно исполняет PRAGMA + DDL (включая сид counters); первая ошибка → наружу (источник CLI-текста «не удалось открыть хранилище»)
func (s *SQLiteStore) Close() error                    // метод конкретного типа (НЕ интерфейса Store); CLI делает defer Close() после успешного открытия
```

PRAGMA при открытии (EM-7): `journal_mode = WAL`, `busy_timeout = 5000`, `foreign_keys = ON`.
Временные поля — `time.Time` ↔ строка через `.Format(time.RFC3339)` / `time.Parse(time.RFC3339)` —
именно `time.RFC3339` (секундная точность), **не** `RFC3339Nano`.

### 3.3. DDL SQLite (дословно §EN-2; исполняется при открытии, `CREATE TABLE IF NOT EXISTS`)

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
`UPDATE` минта на свежей БД обновил бы 0 строк. Таблицы `trigger_state`/`events` — **007**.

---

## 4. Счётчики id (§EN-2; D-10; FR-002)

Персистентный монотонный mint. Формат: `fmt.Sprintf("p-%06d", n)` / `"t-%06d"` (нуль-паддинг до 6).

| Реализация | Носитель | Атомарность |
|---|---|---|
| `MemoryStore` | счётчик под `sync.Mutex` | инкремент и выдача под mutex |
| `SQLiteStore` | таблица `counters(name TEXT PRIMARY KEY, value INTEGER NOT NULL)` | `UPDATE` + чтение счётчика в **одной транзакции** (сид гарантирует наличие строки) |

- Свежее хранилище всегда начинает с `p-000001` / `t-000001` (детерминизм golden, SC-001/SC-002).
- В SQLite счётчик **переживает перезапуски** — повторный `run --db` того же файла продолжает нумерацию
  (Q2/D-10: создаёт новые инстансы `p-000002`/`t-000003`, норма v1).
- Случайный суффикс EM-8 **отменён** (уникальность гарантирует счётчик; id маскируемы в golden).
  `SaveInstance` остаётся upsert — коллизии исключены конструктивно.

---

## 5. `Длительность` — значение (§EN-2/§EN-5; D-7/D-16/D-17)

Тип `internal/value` (`deferred.go:20-23`); активируется фичей 006 (был deferred в 003–005).

### Структура и конструирование

```go
type Длительность struct {
    Amount int64  // значение
    Unit   string // единица: "сек" | "мин" | "час" | "дн" | "нед" | "мес"
}
```

- **Конструирование — литералом** (`DurationLit`, `expr.go:50-51`): рантайм парсит
  `strconv.ParseInt(DurationLit.Amount, 10, 64)` → `value.Длительность{Amount, Unit}` (D-16).
  `DurationLit.Amount` — **нормализованная лексема-строка** (`literal.go:61-65`); вне диапазона `int64`
  → ОшибкаВыполнения `литерал длительности вне диапазона типа Целое` (поз. литерала, §EN-8.A, D-16).
- **Семантический deferred снят** (D-7): литерал валиден в **любой** позиции выражения
  (`analyze.go:429-430` — case `DurationLit` удаляется).
- Каноническое строковое представление — `value.String` (`repr.go`): `3дн` (число + единица), без
  пробела; используется в stdout (§EN-7) и `печать`.
- Дробные длительности литералом невыразимы (лексика) — граница SPEC §12.

### Операции (минимум D-17; FR-025)

| Операция | Активируется | Семантика |
|---|---|---|
| `==` / `!=` | да (`value.Equal`, новый case) | по паре **единица + значение** (`1час != 60мин`, нормализации нет; `2дн == 2дн` → истина) |
| `<` / `<=` / `>` / `>=` | да (`value.Compare`, новый case) | **только одна единица** → по значению; **разные единицы** → `ok == false` → существующий текст ОшибкиТипа `'<оп>' нельзя применить к Длительность и Длительность` |
| арифметика (`Длит ± Длит`, `Длит * X`, `Дата ± Длит`, `Дата - Дата`) | **нет** (граница §EN-10) | остаётся `'<оп>' нельзя применить к <тип> и <тип>` (SPEC §4 — целевая семантика v1+) |

---

## 6. Часы движка — `engine.Clock` (§EN-3; D-2; FR-017)

Отдельный источник времени lifecycle, **не путать** с `eval.Clock` (дневной, `value.Дата` для метрик 004).

```go
// Clock — время движка (D-2). НЕ путать с eval.Clock (дневной, value.Дата).
type Clock interface {
    Now() time.Time
}

// SystemClock — продовый Clock; ЕДИНСТВЕННОЕ легальное time.Now() движка.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }
```

- Используется для `CreatedAt`/`UpdatedAt`/`CompletedAt`, абсолютизации дедлайна (D-19), признака
  `просрочена` (через `Overdue`).
- Инъекция в тестах/golden — `WithClock(c Clock) Option` (фиксированный момент → байт-точный golden).
- `engine.SystemClock{}` **экспортируется** — `tasks` берёт `now` из него (D-22, инвариант D-2).
- `eval.Clock` (дневной, golden 2026-05-31) **не трогаем** (FR-043). Инвариант D-2/SC-006/SC-009:
  «прямой `time.Now()` запрещён всюду, кроме реализаций Clock-интерфейсов» (два легальных места — eval
  для метрик, engine для lifecycle).

---

## 7. Кодек значений — type-tagged JSON (§EN-2; D-5/D-6/D-21) — внутреннее дело `SQLiteStore`

Интерфейс `Store` принимает/отдаёт `map[string]value.Value`; `MemoryStore` JSON не трогает; движок про
JSON/SQL не знает (EM-4). Кодек — только в `SQLiteStore` (поле `variables`). Round-trip честный для
всех 10 типов (FR-006).

### Таблица 10 типов (дословно §EN-2)

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

### Правила порядка и формата (D-5/D-6/D-21; FR-006)

- **D-5 (NaN/±Inf):** `{"т":"Дробное","зн":"NaN"|"+Inf"|"-Inf"}` — три спецзначения Дробного кодируются
  **строками**, остальные — числом. Round-trip честный.
- **D-6 (Запись):** массив пар `[["ключ", <значение>], …]`; порядок `value.Запись.Keys()`
  (`record.go`: `keys []string`) сохраняется. Плоский JSON-объект из EM-4 **отменён**.
- **D-21 (верхний уровень):** карта `Variables` — Go-map (порядок теряется); кодек пишет верхний
  уровень JSON-объектом с **ключами по возрастанию** (детерминизм строки `variables`). `Запись`-значения
  внутри переменных — массив пар D-6, их собственный порядок `Keys()` сохраняется.
- **Времена:** временные поля (`created_at`/`updated_at`/`deadline`/`completed_at`) — RFC3339 секундной
  точности (`time.RFC3339`), вне кодека `variables` (отдельные TEXT-колонки).

---

## 8. Связи и инварианты (сводка)

| Связь / инвариант | Носитель | Правило | Где проверяется |
|---|---|---|---|
| `Task.InstanceID` → `ProcessInstance.ID` | FK SQLite `tasks.instance_id REFERENCES instances(id)` | задача всегда принадлежит инстансу | DDL (§3.3) |
| Активен ровно один шаг = одна открытая задача | `CurrentStep` + `Task.StepName` | гард D-8: `inst.Status==ожидает` И `inst.CurrentStep==task.StepName` | `engine.Complete` (§EN-3) |
| Дрейф-гарды до гарда повторности | `ProcessName`/`CurrentStep` против файла | сперва процесс/шаг найдены в файле, потом статус задачи | `engine.Complete` (§EN-3, FR-015) |
| Гард-догон D-4 | статус задачи `завершена` + инстанс `ожидает` на том же шаге | идемпотентное до-продвижение, exit 0, строка 8 §EN-7 | `engine.Complete` (FR-016) |
| Без алиасинга (Memory) | `MemoryStore` | копии инстанса/карты/задачи; значения разделяются | `MemoryStore` (FR-007) |
| Round-trip кодека честный | `SQLiteStore.variables` | что записано — то и читается, 10 типов | кодек (FR-006) |
| Детерминизм id | счётчики `counters` / mutex | свежее хранилище → `p-000001`/`t-000001` | mint (FR-002) |
| Два легальных `time.Now()` | `eval.Clock` / `engine.Clock` | прямой `time.Now()` запрещён вне реализаций Clock | grep (SC-009) |

---

## Сводка сущностей и счёт

- **Пакет `internal/store`** (новый): `ProcessInstance` (7 полей), `Status` (6 констант), `Task`
  (8 полей), `TaskStatus` (2 константы); интерфейс `Store` (8 методов) + 3 сентинела; реализации
  `MemoryStore` (`NewMemoryStore`) и `SQLiteStore` (`NewSQLiteStore`/`Close`) + DDL (3 таблицы + индекс,
  сид `counters`); кодек type-tagged JSON (10 типов, D-5/D-6/D-21) — внутри `SQLiteStore`.
- **Пакет `internal/value`** (правка): `Длительность{Amount int64, Unit string}` активируется
  (конструирование `DurationLit`, операции `==`/`!=`/`<`/`<=`/`>`/`>=` минимум D-17).
- **Пакет `internal/engine`** (новый, поведение): `Clock`/`SystemClock`/`WithClock` (D-2),
  `Overdue`/`FormatTaskLine` (D-22) — поведенческий контракт в [contracts/engine.md](./contracts/engine.md).
- **НЕ вводятся (007/v2):** триггерные методы `Store` + `ErrTriggerStateNotFound` + таблицы
  `trigger_state`/`events`; листинги инстансов; транзакционный комбо-метод «завершить + продвинуть»;
  арифметика Длительности/дат; категория `ОшибкаПроцесса` (§EN-10).
