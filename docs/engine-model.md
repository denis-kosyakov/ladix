# Модель движка процессов Ladix — исполнение (связывающий якорь фичи 006)

> **Назначение.** Это **единственный связывающий** документ фичи **006 — движок исполнения
> процессов + человек-в-цикле**: контракт `Store` v006, пакет `engine`, экспортируемая
> поверхность `eval`, интерфейс `ProcessRuntime`, активация deferred-веток, CLI
> (`run --db`/`complete`/`tasks`), **байт-точные** реестры stdout (§EN-7) и диагностик (§EN-8),
> приёмочная таблица (§EN-9). По образцу `docs/process-model.md` (якорь 005) и
> `docs/eval-model.md §8.3` (exact-match реестр 003).
>
> **Соотношение с другими документами.** `docs/execution-model.md` (EM-1..EM-16) — **база
> механики** (lifecycle, Store, персист, восстановление); этот якорь её **не переписывает**,
> а фиксирует **ревизии решений kickoff 006** (D-1..D-15 в §EN-1) и **реализационные дыры**,
> которые EM оставляет открытыми: точные Go-сигнатуры экспорта eval и `ProcessRuntime`,
> кадр шага, гарды `complete`, тексты stdout/stderr. **EM-17 (планировщик триггеров) — фича
> 007, в 006 не входит.** `SPEC §11` — семантический фон (что делает шаг); `SPEC §13` — канон
> диагностики; `docs/process-model.md` — фронтенд процессов (формы AST §PM-2 — читаются движком
> как есть); `docs/eval-model.md` — модель интерпретатора, чьи рантайм-ветки активируются;
> `docs/stdlib.md` — сигнатуры process-builtins; `README, раздел CLI` — контракт команд;
> `ARCHITECTURE.md §2/§6/§7` — раскладка пакетов и эскизы (синхронизируются под этот якорь).
>
> **Приоритет при расхождении:** для всего слоя 006 (Store/engine/экспорт eval/CLI-тексты/stdout)
> побеждает **этот файл**; для текстов 003 — `eval-model.md §8.3`; 004 — `source-metric-model.md
> §SM-9`; 005 — `process-model.md §PM-6`. `EM-1..EM-16` и `ARCHITECTURE §6/§7` в местах,
> где противоречат §EN-1..§EN-8, считаются **устаревшими до синка** (ревизия EM — отдельный шаг).
> Формы AST не меняются (канон — §PM-2); Go-код в примерах ниже — **контракт сигнатур**, не
> реализация.
>
> **Нумерация локальная:** §EN-0..§EN-10 принадлежат этому файлу; «EM-N» — execution-model.md,
> «§PM-N» — process-model.md, «§SM-N» — source-metric-model.md, «SPEC §N» — спецификация.

---

## §EN-0. Граница фичи 006

**Входит в 006 (движок + человек-в-цикле; решение владельца Q1=A):**

1. **`internal/store`** — типы данных (`ProcessInstance`/`Task`/статусы), интерфейс `Store`
   v006 (нарезанный контракт D-3), `MemoryStore`, `SQLiteStore` (`modernc.org/sqlite` —
   **первая внешняя зависимость**, go.mod+go.sum — отдельный коммит), кодек type-tagged JSON
   (EM-4 + D-5/D-6), персистентный mint id (D-10). Полный контракт — §EN-2.
2. **`internal/engine`** — lifecycle `Start`/`advance`/`Complete` (EM-9 с правками D-4/D-8/D-9),
   засыпание/пробуждение EM-10, engine-Clock (D-2), реализация `eval.ProcessRuntime`. §EN-3.
3. **Экспорт исполнительной поверхности `eval`** + интерфейс **`ProcessRuntime`,
   объявленный в eval** (D-1, разрыв цикла eval↔engine). §EN-4.
4. **Активация deferred-веток eval:** `RunProcessExpr` (`expr.go:48-49`),
   `AssignAction`/`CallAction`/`NotifyAction` (`stmt.go:63-64`), `DurationLit`
   (`expr.go:50-51` + снятие семантического deferred `analyze.go:429-430`) — D-7. §EN-5.
5. **Активация 3 process-builtins** `статус_процесса`/`состояние_процесса`/`задачи_пользователя`
   (D-15): реестр **25 активных + 10 deferred → 28 + 7** (факт кода: `builtins.go:49-53`
   `deferredNames`; стейл-комментарии `interpreter.go:20` «23 активных + 12» и `builtins.go:55`
   «РОВНО 25 активных + 10» — перелочить на «28 + 7»). §EN-5.
6. **CLI:** `run` + новый опциональный флаг `--db` (мост в SQLite, Q2=а), `complete` с **новой
   сигнатурой** `ladix complete <file.ladix> <task-id> [--db путь]` (Q3=а), новая команда
   `tasks [исполнитель] [--db путь]` (файл НЕ принимает — всё из БД). §EN-6.
7. **`.gitignore`:** `ladix.db` (GAP-9).

**Инвентарь замков (deferred-тесты и стейл-контракты, которые 006 переворачивает):**

| Тест | Сейчас залочено | Станет |
|---|---|---|
| `cmd/ladix/main_test.go::TestRunOnboardingProcessDeferred` (:86-110; стейл-докстринг :86-90 «код 1, рантайм-граница 005» — тоже перелочить) | `онбординг.ladix` → код 1, рантайм-deferred на `запустить процесс` | код 0, golden сценария А (§EN-9; время дедлайна маскируется на CLI-уровне) |
| `eval/analyze_decl_test.go::TestAnalyzeDeferredBoundaryUnchanged` (:747-801; докстринг :747-752 «поведение 003/004 БЕЗ изменений» — тоже перелочить) | `пусть x = 2дн` → семантический deferred (2 кейса); `статус_процесса(1)` → SEM-DEFERRED-BUILTIN | `2дн` валиден (семантика чиста, рантайм даёт значение); `статус_процесса(1)` семантика чиста → рантайм `статус_процесса: ожидается Строка, получено Целое` (§EN-8.A) |
| `eval/builtins_test.go::TestBuiltinDeferredAll` (:146; стейл-комментарий :145 «Все 10 deferred → SEM-DEFERRED-BUILTIN») | итерирует `deferredNames` (10 имён) | итерирует 7 оставшихся (сам адаптируется); комментарий :145 («Все 10» → 7) и счёт-комментарии реестра перелочить |
| `value/deferred.go` — шапка файла (:3-7) | стейл-комментарий «в 003 НЕ конструируются (ворота закрыты, §1.2) … ни один путь кода 003 их не заполняет» | после активации D-7 `Длительность` конструируется (`DurationLit` → `value.Длительность`, `deferred.go:20-23`) — шапку перелочить |
| `examples/MANIFEST.md` §Бизнес-демонстрации (онбординг: «код 1», «строгой golden нет», «фикстура должна маскировать ID») | стейл-контракт 005 | код 0, golden сценария А §EN-9, id детерминированы (D-10), маска только `<время>`; правит импл-чат (examples/ — его зона) |

**НЕ входит (→ фича 007, кроме помеченного):** `serve`, `emit`, флаг `--interval`, весь EM-17
(планировщик: тик, `TriggerState`, очередь `events`), fire-if-true-проход метрика-триггеров в
`run`, триггерные 6 методов `Store` + `ErrTriggerStateNotFound` + таблицы `trigger_state`/`events`,
фронтенд `когда` (парсер/AST/семантика, предопределённые `значение`/`событие`). `repl` — вне 006
(бонус, фича не назначена). Рестарт-скан залипших инстансов — 007 (D-4). Учебный чеклист от 006
ничего не требует — фича целиком продуктовая.

---

## §EN-1. Решения kickoff и заземления (обязательны к исполнению)

**Q1..Q3** — владелец; **D-1..D-15** — архитектор (kickoff); **D-16..D-22** — гэпы, всплывшие
при байт-точном заземлении по коду (закрыты здесь).

| # | Вопрос | Решение |
|---|---|---|
| **Q1** | Разрез 006/007. | **006 = движок + человек-в-цикле** (списки «входит/не входит» — §EN-0); триггеры/демон/события — 007. |
| **Q2** | Мост в SQLite без `serve`. | **`run --db`**: `ladix run file.ladix [--db путь]`; без флага — `MemoryStore` (как сейчас), с флагом — `SQLiteStore`. **Повторный `run --db` того же файла создаёт НОВЫЕ инстансы** (id-счётчик персистентен, D-10) — норма v1, документируется. Канон «у `run` нет `--db`» (README:195, EM-1, ARCH §8.1) правится синком. |
| **Q3** | Сигнатура `complete`. | **`ladix complete <file.ladix> <task-id> [--db путь]`**: истина — в исходнике, БД хранит только состояние. `complete` строит Engine из файла (лексер→парсер→семантика обязаны пройти чисто; ошибки компиляции → обычный барьер CLI, exit 1). **Дрейф исходника против БД** → CLI-ошибка exit 2, инстанс не трогается (тексты §EN-8.B); граница v1 — исходник между запуском и `complete` не меняют. `tasks` файл НЕ принимает. |
| **D-1** | Граница eval↔engine. | **Вариант B: минимальный экспорт + callback-интерфейс.** eval экспортирует исполнение тела шага, реестр процессов, глобальное окружение, снапшот слоя `Environment`; цикл разрывается интерфейсом **`ProcessRuntime`, объявленным в eval** и инжектируемым сеттером; engine его реализует. Кадр шага: `global → processEnv → stepEnv` (= SPEC §6.4). Точные сигнатуры — §EN-4. |
| **D-2** | Время движка. | **Отдельный engine-Clock**: `type Clock interface { Now() time.Time }` в пакете engine; дефолт `SystemClock{}` — единственное легальное `time.Now()` движка; инъекция `WithClock`. Используется для `CreatedAt`/`UpdatedAt`/`CompletedAt`, абсолютизации дедлайна, `просрочена`. `eval.Clock` (дневной, `clock.go:12-14`, golden 2026-05-31) **не трогаем**. Инвариант D-7/SC-006 source-metric-model переформулируется: «прямой `time.Now()` запрещён всюду, кроме реализаций Clock-интерфейсов (eval: `value.Дата` для метрик; engine: `time.Time` для lifecycle)». |
| **D-3** | Контракт Store v006. | **Нарезанный**: только 8 методов (§EN-2), сентинелы `ErrInstanceNotFound`/`ErrTaskNotFound`/`ErrTaskAlreadyCompleted` (новый). Триггерные методы НЕ объявляются (007 добавит аддитивно). **Методов листинга инстансов НЕТ** (D-4). |
| **D-4** | Восстановление без листингов. | Рестарт-скана в 006 нет (носитель — демон 007). Сбойное окно «`MarkTaskCompleted` прошёл, advance не успел» закрывает **гард-догон в `complete`**: повторный `complete` уже-завершённой задачи проверяет инстанс; если `ожидает` И `CurrentStep == task.StepName` → хвост сбоя, **идемпотентное до-продвижение, exit 0** с пометкой в выводе (§EN-7); иначе → `задача '<id>' уже завершена`, exit 2. ARCH-канон «повторный complete всегда exit 2» правится. Инстанс, упавший в `выполняется` (kill посреди шага), в 006 остаётся залипшим до 007 — граница §EN-10. |
| **D-5** | Кодек NaN/±Inf. | `{"т":"Дробное","зн":"NaN"\|"+Inf"\|"-Inf"}` — **строки** для трёх спецзначений, число для остальных. Round-trip честный. |
| **D-6** | Кодек Записи. | **Массив пар**: `{"т":"Запись","зн":[["ключ", <значение>], …]}` — порядок `value.Запись.Keys()` сохраняется (`record.go`: `keys []string`). Плоский JSON-объект из EM-4 **отменяется**. |
| **D-7** | `DurationLit`. | **Полная активация**: `expr.go:50-51` → `value.Длительность{Amount, Unit}` (`deferred.go:20-23`); семантический deferred (`analyze.go:429-430`) **снимается** — литерал валиден в любой позиции выражения. Операции над Длительностью НЕ расширяются сверх минимума D-17. Дробные длительности литералом не выразимы (лексика) — уже граница SPEC §12. |
| **D-8** | Гарды `complete`. | Перед продвижением: задача существует; инстанс существует; `inst.Status == ожидает` И `inst.CurrentStep == task.StepName`. Нарушение → CLI-ошибка exit 2 (тексты §EN-8.B). Плюс гард-догон D-4. |
| **D-9** | Фаза атрибутов шага. | **До тела** (правка EM-9 под SPEC §11.3): (1) вычислить `исполнитель`/`срок` в кадре шага, (2) исполнить тело, (3) развилка Task/продвижение. Ошибка вычисления атрибута = runtime-ошибка шага → `провален` (как ошибка тела). |
| **D-10** | Mint id. | **Персистентный счётчик**: SQLite — таблица `counters(name TEXT PRIMARY KEY, value INTEGER NOT NULL)`, инкремент и выдача в одной транзакции; Memory — счётчик под mutex. Формат: `p-<NNNNNN>`/`t-<NNNNNN>` (нуль-паддинг 6). Случайный суффикс EM-8 **отменяется** (уникальность гарантирует счётчик; id маскируемы в golden). `SaveInstance` остаётся upsert — коллизии исключены конструктивно. |
| **D-11** | At-least-once и «присвоить». | Переактивация/повтор шага может **повторить мутации переменных** (`присвоить x = x + 1`). Шаги пишутся идемпотентно по переменным; граница v1 (§EN-10, ревизия EM-12). |
| **D-12** | Атомарное завершение задачи. | Метод Store **`MarkTaskCompleted(id string, completedAt time.Time) error`**: атомарно `открыта`→`завершена` (SQLite: условный `UPDATE … WHERE status='открыта'` + проверка rows affected; Memory: под mutex). Уже завершена → `ErrTaskAlreadyCompleted`. Остаточная гонка «двойной advance» двух одновременных `complete` (проигравший попадает в окно догона D-4) — документированная граница v1 (§EN-10). |
| **D-13** | stdout-контракты. | **Байт-точные**, залочены в §EN-7. Запуск процесса — **тихий** (системного сообщения нет; «запущен онбординг, id:» в демо — печать ПРОГРАММЫ). |
| **D-14** | Провал инстанса. | Runtime-ошибка тела шага (или атрибута, D-9): инстанс персистируется `провален`, далее ошибка всплывает как обычная ошибка программы — канон SPEC §13 в stderr, fail-fast, **exit 1** (`run` и `complete` одинаково). Двойная природа: в Store — статус, в CLI — диагностика; дополнительной CLI-строки о провале НЕТ. `вызвать` в v1 всегда успешен (стаб) — путь «сбой вызвать → провален» описан, но **недостижим**. Категория `ОшибкаПроцесса` в 006 **не вводится** (ей нечего нести; SPEC §13.1 строка «Процесса» остаётся зарезервированной прозой). |
| **D-15** | Process-builtins. | Реализуются в eval через тот же `ProcessRuntime` (методы «статус по id / переменные по id / открытые задачи по исполнителю»), engine транслирует к Store. `процесс '<id>' не найден` — ОшибкаВыполнения с позицией вызова (EM-13/EM-16). `задачи_пользователя("")` → все открытые (= `ListPendingTasks("")`); порядок — **по возрастанию id** (детерминизм). Реестр: 28 активных + 7 deferred; перелочить `TestBuiltinDeferredAll` и стейл-комментарии (§EN-0). |
| **D-16** | `DurationLit.Amount` — строка (факт кода). | AST несёт **нормализованную лексему-строку** (`literal.go:61-65`, диапазон не проверен — D-R3); `value.Длительность.Amount` — `int64`. Активация парсит `strconv.ParseInt(Amount, 10, 64)`; ошибка диапазона → **ОшибкаВыполнения** `литерал длительности вне диапазона типа Целое` (поз. литерала, §EN-8.A). |
| **D-17** | Операции над Длительностью в 006. | Активируется **минимум, без которого значение врёт**: `==`/`!=` (новый case в `value.Equal` — единица+значение, `1час != 60мин`) и `<`/`<=`/`>`/`>=` **одной единицы** (новый case в `value.Compare`; разные единицы → `ok == false` → существующий текст ОшибкиТипа). **Арифметика** (`Длит ± Длит`, `Длит * X`, `Дата ± Длит`, `Дата - Дата` — SPEC §4) в 006 **НЕ активируется** — остаётся `'<оп>' нельзя применить к <тип> и <тип>`; граница §EN-10 (SPEC §4 описывает целевую семантику v1+). |
| **D-18** | Типы атрибутов шага. | Проверяет **движок на фазе атрибутов** (семпроход их не обходит, §PM-1/D-11): `исполнитель` → `Строка`, `срок` → `Длительность`; иначе **ОшибкаТипа** (тексты §EN-8.A, позиция выражения атрибута) → шаг провален (D-9/D-14). |
| **D-19** | Абсолютизация `срок`. | `Deadline = Task.CreatedAt + срок`; единицы → Go-время: `сек`/`мин`/`час`/`дн`/`нед` — фиксированные множители `time.Duration` (дн=24h, нед=168h); `мес` — календарный `AddDate(0, n, 0)`. Это **внутренняя Go-механика дедлайна**, не Ladix-арифметика (clamp-семантика SPEC §4 на неё не распространяется). |
| **D-20** | Формат CLI-ошибок 006. | stderr одной строкой **`ladix: <текст>`** (префикс — стиль существующего `main.go`), exit 2. Тексты — §EN-8.B. |
| **D-21** | Детерминизм порядков `Variables`. | `ProcessInstance.Variables` — Go-map (порядок теряется): кодек SQLite пишет верхний уровень JSON-объектом с **ключами по возрастанию**; `состояние_процесса(id)` строит `Запись` с ключами **по возрастанию имени**. (`Запись`-значения внутри переменных — массив пар D-6, их собственный порядок `Keys()` сохраняется.) |
| **D-22** | Один источник формата строки задачи. | `engine.FormatTaskLine(t *store.Task, now time.Time) string` — единственное место формата (§EN-7, строка 6); используется и `ladix tasks`, и сводкой `run`. `engine.SystemClock{}` экспортируется — `tasks` берёт `now` из него (инвариант D-2). |

---

## §EN-2. Контракт `Store` v006 (пакет `internal/store`)

**Размещение типов:** `ProcessInstance`/`Task`/статусы/сентинелы живут в `internal/store`
(рядом с контрактом — методы интерфейса их принимают); `internal/engine` владеет только
поведением. Сентинелы — английские (наружу не печатаются, транслируются в русские тексты
§EN-8; SPEC §13/ARCH-конвенция).

### Типы данных (EM-2/EM-3, без изменений по полям)

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

type ProcessInstance struct {
    ID          string                 // "p-NNNNNN" (D-10)
    ProcessName string                 // имя ProcessDecl
    Status      Status
    CurrentStep string                 // имя активного шага; при терминале — последний обработанный
    Variables   map[string]value.Value // переменные процесса; пусть-локали шага сюда НЕ попадают
    CreatedAt   time.Time              // engine-Clock (D-2)
    UpdatedAt   time.Time              // выставляет движок перед КАЖДЫМ SaveInstance
}

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

Триггерные методы (`LoadTriggerState`/`SaveTriggerState`/`NextEventID`/`EnqueueEvent`/
`ListUnprocessedEvents`/`MarkEventProcessed`) и `ErrTriggerStateNotFound` в 006 **не объявляются**
— 007 добавит аддитивно (EM-5 в этой части — фон 007). Транзакционного комбо-метода
«завершить + продвинуть» нет намеренно: корректность — идемпотентный догон D-4.

### `MemoryStore`

`internal/store/memory.go`. Конструктор — **`func NewMemoryStore() *MemoryStore`**. Карты
`map[string]*ProcessInstance`/`map[string]*Task` + счётчики id — всё под одним `sync.Mutex`.
Никакой сериализации: `Value` лежат как есть. **Без алиасинга указателей:** `Save`/`Load`
копируют `ProcessInstance` и карту `Variables` (значения разделяются — ссылочность
Список/Запись как в `Locals()`); `Task` — аналогично. `Load` возвращает копию: мутации
снаружи не видны в Store до следующего `Save` (иначе гарды «инстанс не тронут» §EN-6 и
тесты гранулярности персиста проверяли бы пустоту). `MarkTaskCompleted` — проверка статуса
и перевод под тем же mutex. Назначение — `ladix run` без `--db` и тесты lifecycle.

### `SQLiteStore`

`internal/store/sqlite.go`, `modernc.org/sqlite` (чистый Go, без CGO). Канонические сигнатуры:
**`func NewSQLiteStore(path string) (*SQLiteStore, error)`** — конструктор открывает БД и
**явно исполняет** PRAGMA + DDL (включая сид `counters`; `database/sql` ленив — без явного
`Exec` ошибка открытия не всплыла бы), первая ошибка возвращается наружу — это и есть источник
CLI-текста `не удалось открыть хранилище` (§EN-6/§EN-8.B); **`func (s *SQLiteStore) Close()
error`** — метод конкретного типа (НЕ интерфейса `Store`), CLI делает `defer Close()` после
успешного открытия. DDL (исполняется при открытии, `CREATE TABLE IF NOT EXISTS`):

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

Сид `counters` (строка `INSERT OR IGNORE` выше) исполняется вместе с `CREATE` при каждом
открытии — без него `UPDATE` минта на свежей БД обновил бы 0 строк. PRAGMA при открытии:
`journal_mode = WAL`, `busy_timeout = 5000`, `foreign_keys = ON` (EM-7). FK — **внутренняя
деталь `SQLiteStore`**: контракт `Store` (§EN-2) ссылочной целостности не требует, движок на
неё не опирается, и orphan-read даёт одинаковый `ErrInstanceNotFound`→B3 в Memory и SQLite.
Mint (D-10): `UPDATE`+чтение счётчика в одной транзакции (сид гарантирует наличие строки);
формат `fmt.Sprintf("p-%06d", n)` / `"t-%06d"`. Таблицы `trigger_state`/`events` — **007**.

Временные поля (`created_at`/`updated_at`/`deadline`/`completed_at`): `time.Time` → строка
через `.Format(time.RFC3339)`, парс — `time.Parse(time.RFC3339)`. Именно `time.RFC3339`
(секундная точность), НЕ `RFC3339Nano`: секунд достаточно, детерминизм строк проще.

### Кодек значений (EM-4 + D-5/D-6) — внутреннее дело `SQLiteStore`

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

Интерфейс `Store` принимает/отдаёт `map[string]value.Value`; `MemoryStore` JSON не трогает;
движок про JSON/SQL не знает (путь к замене бэкенда на Camunda/Kestra открыт, EM-4).

---

## §EN-3. Пакет `internal/engine`

### Конструктор, Clock, опции

```go
// Clock — время движка (D-2). НЕ путать с eval.Clock (дневной, value.Дата).
type Clock interface {
    Now() time.Time
}

// SystemClock — продовый Clock; ЕДИНСТВЕННОЕ легальное time.Now() движка.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

type Option func(*Engine)

// WithClock подменяет часы (тесты/golden: фиксированный момент).
func WithClock(c Clock) Option

// NewEngine строит движок над Store и интерпретатором. out — канал системных
// строк stdout (§EN-7); в CLI совпадает с out интерпретатора (печать программы
// и движка перемешиваются в порядке исполнения — всё синхронно).
func NewEngine(st store.Store, interp *eval.Interpreter, out io.Writer, opts ...Option) *Engine
```

Поля `Engine` (минимум): `st store.Store`, `interp *eval.Interpreter`, `out io.Writer`,
`clock Clock` (дефолт `SystemClock{}`), `active []*activeFrame` — **стек активных инстансов**
(пара инстанс + его `processEnv`) для атрибуции хука `присвоить` (§EN-4): push при входе в
тело шага, pop при выходе; вложенный `запустить процесс` из тела шага кладёт новый кадр поверх.
Всё синхронно в одной горутине — mutex не нужен (конкурентность между процессами ОС —
WAL+busy_timeout, EM-11).

Определения процессов engine берёт из интерпретатора (`interp.Process(name)`, §EN-4) — карта
`procs` из эскиза ARCH §7.1 **не передаётся** (сигнатура `NewEngine` выше — канон, ARCH синкается).

### Машина состояний (ревизованный EM-9; ▼ = `SaveInstance`, перед каждым ▼ движок выставляет `UpdatedAt = clock.Now()`)

```
Start(P, args):                                  # реализация eval.ProcessRuntime.StartProcess; запуск ТИХИЙ (D-13)
    pd ← interp.Process(P)                       # гарантированно есть: семпроход резолвит имя (§PM-4)
    id ← st.NextInstanceID()
    inst ← {id, P, статус=создан, Variables=bind(pd.Params, args), CurrentStep=pd.Steps[0].Name, CreatedAt=clock.Now()}
    ▼ SaveInstance(inst)                         # «создан» (транзиентно)
    advance(inst)                                # синхронный прогон; ошибка → наверх (инстанс уже провален)
    return id

advance(inst):                                   # крутит шаги до ожидания/терминала
    processEnv ← кадр процесса (§EN-4)           # ОДИН на весь advance-прогон; stepEnv — свой на каждый шаг
    loop:
        шаг ← lookup(inst.ProcessName, inst.CurrentStep)   # по interp.Process; порядок = порядок исходника (§11.2)
        inst.статус = выполняется
        ▼ SaveInstance(inst)
        stepEnv ← NewEnvironment(processEnv)
        # (1) фаза атрибутов (D-9):
        assignee ← interp.EvalExpr(stepEnv, шаг.Assignee), обязан Строка (D-18)     # если атрибут есть
        срок     ← interp.EvalExpr(stepEnv, шаг.Deadline), обязан Длительность (D-18)
        # (2) тело:
        sig, err ← interp.ExecStepBody(processEnv, stepEnv, шаг.Body)   # каждое «присвоить» внутри ▼ персистит (хук §EN-4)
        если ошибка атрибута или err:
            inst.статус = провален
            ▼ SaveInstance(inst)
            return err                           # D-14: всплывает канон §13, exit 1
        # (3) развилка:
        если assignee задан:                     # человеческий шаг → заснуть
            t ← {st.NextTaskID(), inst.ID, шаг.Name, assignee, Deadline=CreatedAt+срок (D-19), CreatedAt=clock.Now()}
            ▼ SaveTask(t); печать строки создания Task (§EN-7, строки 3/4)
            inst.статус = ожидает
            ▼ SaveInstance(inst); return         # засыпание (EM-10)
        next ← следующий шаг по исходнику
        если next == ∅:
            inst.статус = выполнен
            ▼ SaveInstance(inst); return         # терминал; печати нет (тихо)
        inst.CurrentStep = next

Complete(taskID):                                # для CLI complete; синхронно
    t ← st.LoadTask(taskID)                      # ErrTaskNotFound → CLI exit 2
    inst ← st.LoadInstance(t.InstanceID)         # ErrInstanceNotFound → CLI exit 2
    # дрейф-гарды Q3 (до любых мутаций):
    pd ← interp.Process(inst.ProcessName)        # нет → «процесс … не найден в определении», exit 2
    CurrentStep ∈ pd.Steps?                      # нет → «шаг … не найден в определении …», exit 2
    если t.Status == завершена:                  # гард-догон D-4
        если inst.статус == ожидает И inst.CurrentStep == t.StepName:
            → печать строки 8 (§EN-7) ВМЕСТО строки 7, далее до-продвижение как ниже
              (без MarkTaskCompleted), CaughtUp=true, exit 0
        иначе → «задача '<id>' уже завершена», exit 2
    # гарды D-8 (открытая задача):
    если inst.статус != ожидает → «инстанс … не ожидает (статус …)», exit 2
    если inst.CurrentStep != t.StepName → «задача … не соответствует текущему шагу …», exit 2
    err ← st.MarkTaskCompleted(taskID, clock.Now())
    если errors.Is(err, ErrTaskAlreadyCompleted):           # проигравший гонку D-12
        перечитать inst; → ветка догона D-4 (выше)   # повторный LoadInstance; ErrInstanceNotFound здесь → §EN-8.B B9 (общий «сбой хранилища»), НЕ B3; недостижимо v1: инстансы не удаляются
    печать «задача <id> завершена» (§EN-7, строка 7)        # ДО advance: задача уже завершена фактом
    next ← следующий шаг по исходнику
    если next == ∅: inst.статус = выполнен; ▼ SaveInstance(inst)
    иначе: inst.CurrentStep = next; err ← advance(inst)      # может снова заснуть или провалиться
    если err: return err                                     # exit 1 (D-14); итоговой строки НЕТ
    печать итоговой строки инстанса (§EN-7, строки 9/10)
```

Сигнатуры экспорта engine:

```go
func (e *Engine) Start(name string, args []value.Value) (string, error)

type CompleteResult struct {
    Instance *store.ProcessInstance // состояние после продвижения
    CaughtUp bool                   // true = гард-догон D-4 (до-продвижение уже-завершённой задачи)
}

func (e *Engine) Complete(taskID string) (CompleteResult, error)

// FormatTaskLine — единственный источник формата строки задачи (D-22, §EN-7 строка 6).
func FormatTaskLine(t *store.Task, now time.Time) string

// Overdue: now.After(*t.Deadline); при nil-дедлайне — false (EM-13).
func Overdue(t *store.Task, now time.Time) bool
```

Ошибки `Complete` уровня гардов — типизированы под CLI (различимы `errors.Is`/`errors.As`;
точная Go-форма — на усмотрение имплементации, но **тексты** для пользователя формирует CLI
по §EN-8.B). Runtime-ошибки тела/атрибута возвращаются как есть (уже типизированные ошибки
Ladix с позицией) — CLI печатает канон §13, exit 1 (D-14).

**Владелец печати строк 7-10 (§EN-7)** — сам `engine.Complete`: пишет их в `e.out`; CLI по
`CompleteResult` **ничего не печатает** (структура — для тестов и маппинга exit-кодов).

**Сбои Store вне хука «присвоить»/запуска.** Engine оборачивает ошибки Store **развёртываемой
обёрткой `%w`** — ошибкой, из которой исходный тип восстанавливается через `errors.As`/`Unwrap`.
Конкретная Go-форма — на усмотрение имплементации, но буквальный `fmt.Errorf("…: %w", err)`
**недостаточен**: он стёр бы конкретный тип Store-ошибки, а CLI на путях `complete`/`tasks`
классифицирует «сбой хранилища» именно по типу (§EN-8.B B9, ниже). Каноничная реализация —
типизированный `*StoreError` на границе Store (свой `Error()` с **единственным** префиксом
«сбой хранилища: » + `Unwrap()`) и обёртка-с-причиной на границе eval (`ОшибкаВыполнения` с
полем-причиной и `Unwrap()` — value-receiver, т.к. эти ошибки возвращаются по значению), так
что `errors.As(err, &se)` для `*StoreError` доходит до типа сквозь всю цепочку. Пути,
инициированные Ladix-узлом (`запустить процесс`,
`присвоить`, process-builtins — в т.ч. весь `advance` внутри `Start`), идут по §EN-8.A
(`сбой хранилища: <причина>`, позиция узла-инициатора). Любая не-сентинельная ошибка Store
на CLI-путях `complete`/`tasks` (включая ▼ внутри `advance`, вызванного из `Complete`,
`LoadTask`/`LoadInstance`/`MarkTaskCompleted`/`ListPendingTasks` и ошибки декода type-tagged
JSON при чтении битой БД) — CLI-ошибка §EN-8.B `ladix: сбой хранилища: <причина>`, exit 2
(Ladix-позиции у неё нет — канон §13 неприменим).

**Гранулярность персиста** (EM-9): `SaveInstance` — на создание, каждую смену статуса/шага,
каждое `присвоить` (через хук §EN-4), терминал. `присвоить` в цикле `пока` ⇒ много мелких
записей — принято в v1 (WAL тянет), батчинг — v2.

---

## §EN-4. Интеграция eval↔engine (экспортируемая поверхность)

Зависимости: **eval НЕ импортирует ни store, ни engine** (ребро `eval → store` из ARCH §2.1 —
устаревший эскиз, синкается); `engine → eval, store, ast, value, errors`. Цикл разорван
интерфейсом в eval (D-1).

### Интерфейс `ProcessRuntime` (объявлен в `internal/eval`, дословно)

```go
// ProcessRuntime — мост исполнения процессов (D-1). Реализуется пакетом engine,
// инжектируется сеттером. Все вызовы синхронны, в одной горутине.
type ProcessRuntime interface {
    // StartProcess запускает процесс по имени: создаёт инстанс, синхронно доводит
    // до первого ожидания/терминала, возвращает id ("p-NNNNNN"). Ошибка — уже
    // типизированная ошибка Ladix (всплывает как есть) либо сбой Store.
    StartProcess(name string, args []value.Value) (string, error)

    // AssignProcessVar — хук персиста «присвоить»: значение уже записано в
    // processEnv интерпретатором; движок обновляет Variables активного инстанса
    // (вершина стека active) и персистит (▼SaveInstance).
    AssignProcessVar(name string, v value.Value) error

    // CallExternal — стаб «вызвать» (D-13): одна строка в stdout; в v1 всегда nil.
    // Контракт на будущее: не-nil ошибка → шаг провален (D-14, недостижимо в v1).
    CallExternal(target string, args []value.Value) error

    // Notify — стаб «уведомить» (D-13): одна строка в stdout; всегда nil (best-effort).
    Notify(target string, args []value.Value) error

    // InstanceStatus — статус инстанса по id; ok=false → вызывающий builtin даёт
    // «процесс '<id>' не найден» (D-15). err — только сбой Store.
    InstanceStatus(id string) (status string, ok bool, err error)

    // InstanceVariables — переменные инстанса как Запись, ключи по возрастанию (D-21).
    InstanceVariables(id string) (vars value.Запись, ok bool, err error)

    // UserTasks — открытые задачи исполнителя (""=все), по возрастанию id (D-15);
    // поля Записи — таблица «Task → Запись» (EM-13/ARCH §7.7, включая «просрочена»).
    UserTasks(assignee string) ([]value.Запись, error)
}
```

### Экспорт `eval` (новые методы; имена/сигнатуры — канон)

```go
// SetProcessRuntime инжектирует движок (вызывается до Run; результата Analyze
// не меняет — семпроход от runtime не зависит). nil-runtime: активированные
// конструкции дают ОшибкуВыполнения «внутренняя ошибка: движок процессов не
// подключён» (§EN-8.A) — во всех командах 006, исполняющих Ladix-код, движок
// присутствует: run/complete/metric собирают единый стек §EN-6 (metric —
// MemoryStore, как run без --db).
func (i *Interpreter) SetProcessRuntime(rt ProcessRuntime)

// Process — доступ к реестру процессов (i.processes, interpreter.go:32; заполняет
// Analyze Шаг 1). Для lookup определения и «следующего шага» в движке.
func (i *Interpreter) Process(name string) (*ast.ProcessDecl, bool)

// GlobalEnv — глобальная область (родитель processEnv в кадре шага).
func (i *Interpreter) GlobalEnv() *Environment

// EvalExpr — публичная обёртка evalExpr: вычисление атрибутов шага движком (D-9).
func (i *Interpreter) EvalExpr(env *Environment, e ast.Expression) (value.Value, error)

// ExecStepBody исполняет тело шага (StepDecl.Body []ast.Statement — НЕ *ast.Block,
// поэтому НЕ evalBlock) в области stepEnv; на время исполнения запоминает processEnv
// как приёмник «присвоить» (поле i.procEnv, save/restore реентерабельно — зеркало
// recordCtx: вложенный StartProcess из тела шага переключает и восстанавливает).
// Семантика цикла — как evalBlock (stmt.go:72-83): err → наверх; sig != SigNormal →
// прекратить и вернуть (вернуть/прервать/продолжить вне циклов шага исключены
// семпроходом — практически всегда SigNormal).
func (i *Interpreter) ExecStepBody(processEnv, stepEnv *Environment, body []ast.Statement) (Signal, error)
```

```go
// Locals — снапшот ЛОКАЛЬНОГО слоя области (копия карты; значения разделяются —
// ссылочность Список/Запись сохраняется). ТОЛЬКО для тестов/сверки Variables:
// в алгоритмах §EN-3 движок его НЕ зовёт — при засыпании снапшот processEnv не
// снимается, канал персиста ТОЛЬКО хук AssignProcessVar (снапшот при засыпании
// молча персистировал бы и мутации `x = E` мимо хука — граница §EN-10, D-11).
func (e *Environment) Locals() map[string]value.Value
```

`Signal`/`SigNormal`/… уже экспортированы (`signal.go`); `NewEnvironment`/`Define`/`Assign`/
`Lookup` уже экспортированы (`environment.go`) — движку хватает их для сборки кадров.

### Кадр шага и цепочка резолва (SPEC §6.4)

```
global (interp.GlobalEnv: предопределённые периоды + глобали программы — последние только в run, §EN-6)
  └─ processEnv (переменные процесса: параметры при создании + «присвоить»)
       └─ stepEnv (пусть-локали шага, переменные «для»)
```

- `processEnv` движок строит при **каждом входе в `advance`**: `NewEnvironment(interp.GlobalEnv())`
  + `Define` каждой пары из `inst.Variables` — **один processEnv на весь `advance`-прогон**
  (при пробуждении `complete` строится заново из персистированных `Variables`).
  `stepEnv = NewEnvironment(processEnv)` — свой на каждый шаг.
- **Чтение** имени в теле шага: `stepEnv → processEnv → global` (штатный `Lookup`), затем
  функции/источники/метрики (existing `evalIdent`, `expr.go:64-92`) — без изменений.
- **`присвоить x = E`** (активация `stmt.go:63-64`, ветка `AssignAction`): вычислить `E` в
  текущем env → `i.procEnv.Define(x, v)` (создаёт ИЛИ обновляет переменную процесса; пишет в
  слой процесса, **мимо** тени `пусть`-локали шага — §6.4) → `i.runtime.AssignProcessVar(x, v)`
  (персист). Ошибка хука → ОшибкаВыполнения `сбой хранилища: <причина>` (поз. `присвоить`).
- **`x = E`** (обычный `AssignStmt`) в теле шага работает штатно (`Assign` вверх по цепочке):
  достигнув слоя processEnv, обновит переменную процесса **без** персист-хука — поэтому
  канонический способ мутации переменной процесса — `присвоить` (SPEC §11.4); правило
  «`'x' — переменная процесса, используйте 'присвоить'`» в v1 **не** вводится (рантайм не
  различает слой; граница §EN-10).
- **`вызвать Имя(args)`/`уведомить Имя(args)`**: вычислить аргументы слева направо →
  `CallExternal`/`Notify` (имя — `Ident` цели как строка, НЕ резолвится как переменная).
- **`запустить процесс P(args)`** (активация `expr.go:48-49`): вычислить аргументы →
  `StartProcess(P, args)` → `value.Строка{V: id}`. Работает на top-level, в функции, в теле
  шага (вложенный запуск — синхронно до первого ожидания/терминала вложенного, EM-17.5-семантика
  без триггеров).

---

## §EN-5. Активация deferred (точечная карта кода)

| Конструкция/имя | Место в коде | Было (005) | Станет (006) |
|---|---|---|---|
| `RunProcessExpr` | `eval/expr.go:48-49` | рантайм-deferred (`deferredConstruct`) | аргументы → `runtime.StartProcess` → `Строка` id (§EN-4) |
| `AssignAction` | `eval/stmt.go:63-64` | рантайм-deferred (недостижимо) | запись в `procEnv` + хук `AssignProcessVar` |
| `CallAction` | `eval/stmt.go:63-64` | рантайм-deferred (недостижимо) | стаб `CallExternal` (строка §EN-7.2, всегда успех) |
| `NotifyAction` | `eval/stmt.go:63-64` | рантайм-deferred (недостижимо) | стаб `Notify` (строка §EN-7.1) |
| `DurationLit` (рантайм) | `eval/expr.go:50-51` | рантайм-deferred | `ParseInt(Amount)` → `value.Длительность{Amount, Unit}`; вне диапазона → §EN-8.A (D-16) |
| `DurationLit` (семантика) | `eval/analyze.go:429-430` | семантический deferred (`checkExpr`) | **case удаляется** — литерал валиден в любой позиции (D-7) |
| `статус_процесса`/`состояние_процесса`/`задачи_пользователя` | `eval/builtins.go:49-53` (`deferredNames`) | deferred-builtins (SEM-DEFERRED-BUILTIN) | `fixed(имя, 1, …)` активные; реализация через `runtime` (D-15) |
| `==`/`!=` Длительности | `value/equal.go` (case отсутствует — иначе `2дн == 2дн` → ложь) | разные/одинаковые → `false` | case `Длительность`: единица+значение (D-17) |
| `<`/`<=`/`>`/`>=` Длительности | `value/equal.go::Compare` | `ok == false` → ОшибкаТипа | case одной единицы → по значению; разные единицы → `ok == false` (D-17) |

**Что НЕ активируется** (остаётся как есть): арифметика Длительности и `Дата±Длит`/`Дата-Дата`
(D-17, граница §EN-10); deferred-builtins дата/времени — `вчера`, `завтра`, `длительность`,
`в_секундах`, `в_минутах`, `в_часах`, `в_днях` (**7 имён** остаются в `deferredNames`);
`constructName` (`interpreter.go:152-166`) остаётся — живой вызов у контекст-гарда действий
(`analyze.go:371`); `deferredConstruct` (`interpreter.go:148-150`) после активации остаётся
**без вызовов** (все четыре — `expr.go:49`, `expr.go:51`, `stmt.go:64`, `analyze.go:430` —
активируются/удаляются 006) — **удалить вместе с активацией** (иначе мёртвый приватный метод,
staticcheck U1000; узлы будущих фич при необходимости вернут аналог).

**Семпроход (`analyze.go`) больше не меняется нигде**, кроме удаления case `DurationLit`:
контекст-гард действий (:365-373), `checkRunProcess` (:472-495), `checkCall` deferred-builtins
(:453-456) уже корректны для 006 (deferred-флаг трёх процессных имён снимается данными реестра,
не кодом).

**Реестр builtins после 006: 28 активных + 7 deferred = 35** (инвариант суммы ×1 сохраняется).
Новые активные — `ArityFixed, N=1`; аргумент обязан `Строка`, иначе ОшибкаТипа §EN-8.A
(позиция вызова, как у `дата`). Перелочить: `interpreter.go:20` (стейл «23 активных + 12»),
`builtins.go:55-56` («РОВНО 25 активных + 10 deferred = 35»), `eval/clock.go:16-18` (стейл
«Это ЕДИНСТВЕННЫЙ легальный вызов time.Now() во всей цепочке eval/движка» — с D-2 появляется
второе легальное место; переформулировать: единственный легальный `time.Now()` в **eval**;
у движка свой `engine.SystemClock` — вторая легальная реализация Clock-интерфейса, D-2/§SM-7),
тесты из инвентаря §EN-0.

---

## §EN-6. CLI 006

Новая usage-строка (`main.go:31`):

```
использование: ladix run [--max-depth N] [--db путь] <файл> | ladix metric [--max-depth N] <файл> <имя> | ladix complete [--db путь] [--max-depth N] <файл> <task-id> | ladix tasks [--db путь] [исполнитель]
```

Разбор флагов — ручной, стиль `runMain` (`--флаг значение` и `--флаг=значение`; неизвестный
флаг → `ladix: неизвестный флаг <а>`, exit 2). Сборка движка (общая для `run`/`complete`/
`metric`): `interp := eval.NewInterpreter(stdout, maxDepth, eval.SystemClock{})` → `st`
(Memory либо SQLite) → `eng := engine.NewEngine(st, interp, stdout)` →
`interp.SetProcessRuntime(eng)`. У `metric` Store — всегда `MemoryStore` (как `run` без
`--db`, флага `--db` у неё нет): формула метрики может через функцию дёрнуть
`запустить процесс`/process-builtin (семпроход это разрешает) — без движка она упёрлась бы
в nil-runtime (§EN-8.A).

> **Латентный гэп часов (в v1 безвреден).** Движок этого пути по умолчанию берёт
> `engine.SystemClock{}` (D-2, wall-clock), а не дневные `eval.Clock` (golden 2026-05-31).
> Сейчас незаметно — ни одна golden-метрика не стартует процесс с шагом-«срок», а `run`/
> `complete` маскируют время как `<DT>` (§EN-7). Но такая метрика в будущем дала бы
> недетерминированные `CreatedAt`/абсолютизацию дедлайна. Путь отступления:
> `engine.WithClock(...)`, синхронный с дневным `eval.Clock`; закрыть при первой
> golden-метрике, стартующей процесс.

### `ladix run <file.ladix> [--db путь] [--max-depth N]`

1. Чтение файла, лексер→парсер (накопленные ошибки → stderr, exit 1) — как сейчас
   (`runFile`, `main.go:140-161`).
2. Store: **без `--db` — `MemoryStore`** (эфемерно, как сейчас); **с `--db` — `SQLiteStore`**
   (Q2). Открытие/инициализация схемы не удались → `ladix: не удалось открыть хранилище
   '<путь>': <причина>`, exit 2.
3. Под `guard`: `interp.Run(prog)` — top-level сверху вниз; `запустить процесс` синхронно
   доводит инстанс до ожидания/терминала (§EN-3). Ошибка → канон §13 stderr, exit 1
   (включая провал инстанса, D-14).
4. **Сводка висящих задач**: `st.ListPendingTasks("")`; если непусто — заголовок + строки
   задач (§EN-7, строки 5-6) в stdout. Exit 0 (висящие задачи — не ошибка).
5. **Повторный `run --db` того же файла создаёт новые инстансы** (Q2): id-счётчик
   персистентен, старые «ожидающие» инстансы остаются в БД и видны в `tasks`. Норма v1.

### `ladix complete <file.ladix> <task-id> [--db путь] [--max-depth N]`

Ровно два позиционных аргумента (файл, task-id); меньше/больше → usage, exit 2.

1. Компиляция файла: лексер→парсер→`Analyze` обязаны пройти чисто; ошибки → штатный барьер
   (stderr, exit 1). Семантика 005 гарантирует валидность определений процессов.
   **`interp.Run` НЕ вызывается** (повторное исполнение top-level создавало бы новые инстансы
   при каждом `complete`): `GlobalEnv` при `complete` содержит только предопределённые периоды
   (top-level `пусть` определяет только `Run`, `interpreter.go:80-99`) — шаг, читающий
   top-level `пусть`-глобаль, даёт `'<имя>' не объявлено` → инстанс `провален`, exit 1
   (граница §EN-10); источники/метрики работают (ленивая загрузка, `recordCache`).
2. Store: `SQLiteStore`, путь из `--db`, **дефолт `ladix.db`** (README:195). Не открылось →
   exit 2. (`MemoryStore` для `complete` бессмыслен — общего состояния нет.)
3. `eng.Complete(taskID)` — гарды и порядок строго §EN-3: задача → инстанс → **дрейф-гарды
   Q3** → догон D-4 → гарды D-8 → `MarkTaskCompleted` → печать → продвижение. Нарушение
   гардов → §EN-8.B, exit 2, **инстанс не тронут**. Runtime-ошибка продвижения → инстанс
   `провален` + канон §13, exit 1 (D-14).
4. Успех (включая догон D-4) → stdout §EN-7 (строки 7-10), exit 0.

### `ladix tasks [исполнитель] [--db путь]`

Файл **не принимает** (Q3): всё из БД. Store: `SQLiteStore`, дефолт `ladix.db`. Один
опциональный позиционный — фильтр-исполнитель (без него — все открытые).
`st.ListPendingTasks(фильтр)` → по строке `engine.FormatTaskLine(t, engine.SystemClock{}.Now())`
на задачу (§EN-7, строка 6); пусто → `открытых задач нет` (строка 11). Exit 0 в обоих случаях.
Движок/интерпретатор не строятся (ни Engine, ни файла не нужно); «просрочена» считает
`engine.Overdue` (D-22).

**Граница `--db`:** несуществующий путь создаёт пустую БД со схемой (как у любой команды,
открывающей `Store`, — `CREATE TABLE IF NOT EXISTS`); для `complete` это означает
`ladix: задача '<id>' не найдена` (для `tasks` — `открытых задач нет`) — отдельной
диагностики «базы нет» в v1 нет.

### Коды возврата (без изменений канона README)

**0** — успех (включая `run` с висящими задачами и догон D-4); **1** — ошибка программы Ladix
(компиляция, рантайм, провал инстанса) и внутренняя паника (`guard`); **2** — ошибка
использования/окружения (аргументы, файл, хранилище, гарды `complete`, дрейф Q3).

---

## §EN-7. Реестр stdout (D-13, exact-match)

Все строки — в **stdout**, каждая завершается `\n`. `<арг>` = `value.String(аргумент)`
(`repr.go`; Длительность — `3дн`). `<время>` = `Deadline.Format("2006-01-02 15:04")`
(локальная зона). Печатает: 1-2 — стабы действий (через engine); 3-4 — `advance` (создание
Task); 5-6 — сводка `run` и `tasks`; 7-10 — `complete`; 11 — `tasks` при пустом списке.
Это exact-match канон: тесты сверяют байт-точно.

```
[уведомление] <получатель>: <арг1 арг2 …>                      // 1. уведомить с ≥1 аргументом; разделитель аргументов — один пробел (как печать)
[уведомление] <получатель>                                      // 1а. уведомить без аргументов (без двоеточия и хвостовых пробелов)
[вызов] <имя>(<арг1, арг2, …>)                                  // 2. вызвать; разделитель — ", "; без аргументов — "[вызов] <имя>()"
[задача] <t-id> → <исполнитель>, шаг '<шаг>', срок до <время>    // 3. создание Task со сроком (advance, фаза 3)
[задача] <t-id> → <исполнитель>, шаг '<шаг>'                     // 4. создание Task без срока
открытых задач: <N>                                              // 5. заголовок сводки run (печатается ТОЛЬКО при N ≥ 1, после последнего top-level)
<t-id>  <p-id>  '<шаг>'  <исполнитель>  срок до <время>  ПРОСРОЧЕНА   // 6. строка задачи (FormatTaskLine, D-22): разделитель полей — ДВА пробела; хвост "срок до <время>" только при дедлайне; хвост "ПРОСРОЧЕНА" только при Overdue
задача <t-id> завершена                                          // 7. complete, сразу после MarkTaskCompleted (до продвижения)
задача <t-id> уже была завершена, инстанс до-продвинут            // 8. complete, ветка гард-догона D-4 (вместо строки 7)
инстанс <p-id>: ожидает, шаг '<имя>'                              // 9. итог complete: снова заснул (после строки 3/4 нового Task)
инстанс <p-id>: выполнен                                          // 10. итог complete: терминальный успех
открытых задач нет                                                // 11. tasks при пустом списке (канон README, раздел `ladix tasks`)
```

**Чего в stdout НЕТ (осознанно):** сообщения о запуске процесса (запуск тихий, D-13 — печать
`запущен онбординг, id: p-…` в демо делает ПРОГРАММА); сообщения о терминале внутри `run`;
строки о провале (провал — диагностика stderr, D-14); заголовка у `tasks` при непустом списке.

---

## §EN-8. Реестр диагностик (exact-match)

Соглашения `eval-model §8.3`: payload без завершающей точки; `'…'` — идентификаторы/имена;
позиция в заголовке канона §13. Существующие тексты §8.3/§SM-9/§PM-6 продолжают действовать
без изменений (runtime-ошибки тела шага — это они и есть). Ниже — **только новый слой 006**.

### §EN-8.A. Ошибки программы (канон §13, двухстрочные, stderr, exit 1)

```
процесс '<id>' не найден                                  // ОшибкаВыполнения; статус_процесса/состояние_процесса с неизвестным id (D-15, EM-16); поз. вызова
статус_процесса: ожидается Строка, получено <тип>          // ОшибкаТипа; поз. вызова (стиль «дата: ожидается …», §8.3)
состояние_процесса: ожидается Строка, получено <тип>       // ОшибкаТипа; поз. вызова
задачи_пользователя: ожидается Строка, получено <тип>      // ОшибкаТипа; поз. вызова
шаг '<имя>': исполнитель должен быть Строка, получено <тип> // ОшибкаТипа; движок, фаза атрибутов (D-18); поз. выражения атрибута; инстанс → провален
шаг '<имя>': срок должен быть Длительность, получено <тип>  // ОшибкаТипа; то же
литерал длительности вне диапазона типа Целое               // ОшибкаВыполнения; ParseInt(DurationLit.Amount) за int64 (D-16); поз. литерала
сбой хранилища: <причина>                                  // ОшибкаВыполнения; ошибка Store из хука присвоить/запуска (обёртка %w, EM-16); поз. узла-инициатора
внутренняя ошибка: движок процессов не подключён            // ОшибкаВыполнения; runtime==nil (защитная, в CLI 006 недостижима — все исполняющие команды, включая metric, собирают стек §EN-6); поз. узла
```

Провал инстанса (D-14) **не добавляет** своего текста: stderr несёт исходную ошибку тела/
атрибута (любой текст §8.3/§EN-8.A), статус `провален` виден через `статус_процесса`/БД.
Категория «Процесса» (SPEC §13.1) в 006 не порождается — `вызвать` всегда успешен.

### §EN-8.B. CLI-ошибки (одна строка `ladix: <текст>`, stderr, exit 2 — D-20)

```
ladix: задача '<id>' не найдена                                       // LoadTask → ErrTaskNotFound
ladix: задача '<id>' уже завершена                                    // завершена И догон D-4 неприменим
ladix: инстанс '<id>' не найден                                       // LoadInstance → ErrInstanceNotFound (битая/чужая БД)
ladix: инстанс '<p-id>' не ожидает (статус '<статус>')                 // гард D-8: Status != ожидает
ladix: задача '<t-id>' не соответствует текущему шагу инстанса '<p-id>' // гард D-8: CurrentStep != task.StepName
ladix: процесс '<имя>' не найден в определении                         // дрейф Q3: ProcessName инстанса отсутствует в файле
ladix: шаг '<имя>' не найден в определении процесса '<имя>'            // дрейф Q3: CurrentStep отсутствует в ProcessDecl
ladix: не удалось открыть хранилище '<путь>': <причина>                // открытие/инициализация SQLite (README:195 + префикс D-20)
ladix: сбой хранилища: <причина>                                       // не-сентинельная ошибка Store на CLI-путях complete/tasks (включая ▼ advance из Complete и декод кодека битой БД; §EN-3)
ladix: флаг --db требует значение                                      // разбор флага (стиль --max-depth)
```

Существующие CLI-тексты (`не удалось прочитать файл`, `неверное значение --max-depth`,
`лишний аргумент`, usage) — без изменений.

---

## §EN-9. Приёмочная таблица

Канонический вход — `examples/онбординг.ladix` **как есть** (005-редакция: процесс `онбординг`
с шагами `завести_доступы` (авто: `присвоить` + `уведомить`) → `провести_встречу`
(`"руководитель"`, `3дн`) → `закрыть_адаптацию` (`"HR"`, `5дн`); top-level `пусть id =
запустить процесс онбординг("Петров")` + `печать`). Engine-тесты используют
`WithClock(фикс. 2026-05-31 00:00:00 Local)` → **байт-точный** golden; CLI-тесты
(`main_test.go`) маскируют только `<время>` дедлайнов (id детерминированы: свежий Store
всегда выдаёт `p-000001`/`t-000001`, …).

### Сценарий А — `ladix run examples/онбординг.ladix` (MemoryStore), exit 0

stdout дословно (с фиксированными часами; 5 строк):

```
[уведомление] ИТ: создать учётку для Петров
[задача] t-000001 → руководитель, шаг 'провести_встречу', срок до 2026-06-03 00:00
запущен онбординг, id: p-000001
открытых задач: 1
t-000001  p-000001  'провести_встречу'  руководитель  срок до 2026-06-03 00:00
```

Состояние Store на выходе: инстанс `p-000001` `ожидает`/`провести_встречу`,
`Variables == {имя: "Петров", сотрудник: "Петров"}`; задача `t-000001` `открыта`.

### Сценарий Б — мост SQLite (golden-цепочка, свежая БД `--db test.db`)

| Шаг | Команда | stdout (дословно; `<DT>` — маска времени на CLI-уровне) | exit |
|---|---|---|---|
| 1 | `run онбординг.ladix --db test.db` | как сценарий А (5 строк) | 0 |
| 2 | `tasks --db test.db` | `t-000001  p-000001  'провести_встречу'  руководитель  срок до <DT>` | 0 |
| 3 | `tasks Петров --db test.db` | `открытых задач нет` | 0 |
| 4 | `complete онбординг.ladix t-000001 --db test.db` | `задача t-000001 завершена` ⏎ `[задача] t-000002 → HR, шаг 'закрыть_адаптацию', срок до <DT>` ⏎ `инстанс p-000001: ожидает, шаг 'закрыть_адаптацию'` | 0 |
| 5 | `complete онбординг.ladix t-000002 --db test.db` | `задача t-000002 завершена` ⏎ `инстанс p-000001: выполнен` | 0 |
| 6 | `run онбординг.ladix --db test.db` (повтор, Q2) | как шаг 1, но id `p-000002`/`t-000003` (счётчик персистентен, D-10) | 0 |

### Негативы (после шага 5, та же БД)

| Команда | stderr (дословно) | exit |
|---|---|---|
| `complete онбординг.ladix t-999999 --db test.db` | `ladix: задача 't-999999' не найдена` | 2 |
| `complete онбординг.ladix t-000001 --db test.db` (повтор; инстанс `выполнен`, догон неприменим) | `ladix: задача 't-000001' уже завершена` | 2 |
| `complete examples/hello.ladix t-000001 --db test.db` (существующий файл, компилируется чисто, процесса `онбординг` нет) | `ladix: процесс 'онбординг' не найден в определении` | 2 |
| `complete онбординг-дрейф.ladix t-000001 --db test.db` (testdata-фикстура импл-чата: процесс `онбординг` есть, шаг `закрыть_адаптацию` переименован; после шага 5 `inst.CurrentStep == 'закрыть_адаптацию'`, дрейф-гарды идут ДО гарда «уже завершена» — §EN-3) | `ladix: шаг 'закрыть_адаптацию' не найден в определении процесса 'онбординг'` | 2 |
| `complete онбординг.ladix t-000001 --db нет/такого/каталога.db` | `ladix: не удалось открыть хранилище 'нет/такого/каталога.db': <причина>` | 2 |
| `complete сломанный.ladix t-000001 --db test.db` (testdata-фикстура импл-чата с ПАРС-ошибкой; НЕ `examples/ошибка.ladix` — та падает в рантайме, а `complete` файл не исполняет, компиляцию она проходит чисто) | канон §13 (двухстрочный/накопленный) | 1 |

### Точечные приёмки слоя (для табличных тестов)

| Вход | Уровень | Ожидание |
|---|---|---|
| `пусть d = 2дн` ⏎ `печать(d)` | run | `2дн`, exit 0 (D-7/D-16) |
| `печать(2дн == 2дн)` / `печать(1час == 60мин)` | run | `истина` / `ложь` (D-17) |
| `печать(2дн < 5дн)` | run | `истина` (D-17: одна единица) |
| `печать(2дн < 5мин)` | run | `'<' нельзя применить к Длительность и Длительность` (разные единицы → `Compare ok==false` → существующий текст §8.3), exit 1 |
| `пусть x = 9999999999999999999дн` | run | `литерал длительности вне диапазона типа Целое`, exit 1 (D-16) |
| `печать(статус_процесса("p-999999"))` | run | `процесс 'p-999999' не найден`, exit 1 (D-15) |
| `печать(статус_процесса(1))` | run | `статус_процесса: ожидается Строка, получено Целое`, exit 1 |
| `печать(статус_процесса(id))` после запуска | run | `ожидает` (id из `запустить процесс`) |
| `печать(длина(задачи_пользователя("")))` после запуска онбординга | run | `1` (D-15: ""=все) |
| шаг с `исполнитель: 42` | run | `шаг '<имя>': исполнитель должен быть Строка, получено Целое`, exit 1; инстанс `провален` (D-18/D-14) |
| шаг с телом `присвоить x = 1/0` | run | `деление на ноль` (канон §13), exit 1; инстанс `провален` (D-14) |
| `вчера()` | run | `функция 'вчера' не поддерживается в этой версии` — deferred-граница 7 имён НЕ сдвинулась |
| фабрикованное Store-состояние: открытая задача при инстансе со статусом `выполняется` → `Complete(t-id)` | engine (unit, Store готовится тестом) | ошибка гарда D-8, CLI-текст `ladix: инстанс '<p-id>' не ожидает (статус 'выполняется')` (§EN-8.B), инстанс не тронут; через CLI-цепочку кейс практически недостижим (один активный шаг = одна открытая задача) — поэтому уровень engine |
| фабрикованное Store-состояние: открытая задача с `StepName` ≠ `CurrentStep` инстанса в `ожидает` → `Complete(t-id)` | engine (unit, Store готовится тестом) | ошибка гарда D-8, CLI-текст `ladix: задача '<t-id>' не соответствует текущему шагу инстанса '<p-id>'` (§EN-8.B), инстанс не тронут |

---

## §EN-10. Что 006 осознанно НЕ делает (границы и путь отступления)

- **Триггеры и демон** (EM-17 целиком: `serve`, `emit`, `--interval`, тик, `TriggerState`,
  очередь `events`, fire-if-true в `run`, триггерные методы Store, фронтенд `когда`) → **007**.
- **Рестарт-скан залипших инстансов** (упавших в `выполняется`/`создан` посреди шага) → **007**
  (носитель — демон); в 006 такой инстанс остаётся залипшим навсегда (D-4); закрыто только
  окно «задача завершена, advance не успел» — гард-догоном D-4.
- **At-least-once «присвоить»** (D-11): повтор шага (догон, будущий рестарт-скан) может
  повторить мутации; идемпотентность по переменным — ответственность автора программы. v1.
- **Остаточная гонка двойного `complete`** (D-12): два одновременных `complete` одной задачи —
  выигравший продвигает, проигравший уходит в догон D-4; вероятность ничтожна, последствия
  ограничены гардами D-8.
- **Версионирование определений**: исходник между `run --db` и `complete` не меняют (дрейф
  ловится гардами Q3 только по именам процесса/шага; смена ТЕЛА шага не детектится). v2 —
  хеш определения в инстансе.
- **Арифметика Длительности и дат** (`Длит ± Длит`, `Длит * X`, `Дата ± Длит`, `Дата - Дата`
  — SPEC §4): не активируется (D-17), остаётся ОшибкаТипа; конструктор `длительность(…)` и
  аксессоры — deferred-builtins (7 имён). Бэклог 007+/v2.
- **Правило «`'x' — переменная процесса, используйте 'присвоить'`»** (§6.4): рантайм 006 не
  различает запись `x = E` в слой процесса (без персист-хука) — канонический канал мутации
  только `присвоить`; такие мутации живут до конца `advance`-прогона (processEnv — per-прогон,
  §EN-4) и теряются при следующем пробуждении. Диагностика — v2.
- **Top-level `пусть`-глобали в `complete`**: `interp.Run` при `complete` не вызывается
  (§EN-6) — шаг, читающий top-level глобаль, при пробуждении даёт `'<имя>' не объявлено` →
  инстанс `провален`, exit 1 (в `run` тот же шаг работает). Граница v1; канал данных между
  шагами — переменные процесса, источники/метрики доступны в обоих режимах.
- **Exactly-once эффектов, батчинг персиста, транзакционный outbox, отзыв осиротевших задач
  при `провален`, payload задачи (`complete` не передаёт данные), реестр ролей/прав** → v2
  (EM-12/EM-15, SPEC §12).
- **Команды `serve`/`emit`/`repl`**, машинно-читаемый вывод, миграции схемы БД → 007/v2.

**Путь отступления.** Контракт `Store` (§EN-2) аддитивен: 007 добавляет 6 триггерных методов
и 2 таблицы, не меняя существующие 8 методов и DDL. `ProcessRuntime` (§EN-4) аддитивен:
триггерам новые методы не нужны (тело триггера = `запустить процесс` + чтение метрик).
Синтаксис и AST не меняются вовсе (канон §PM-2/§PM-8): движок читает `ProcessDecl.Steps` как
есть, замена встроенного движка на Camunda/Kestra — замена реализаций `ProcessRuntime`+`Store`
при том же фронтенде.
