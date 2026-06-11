# Контракт: пакет `internal/engine` (lifecycle, машина состояний)

**Фаза**: 1 (design) | **Якорь**: `docs/engine-model.md §EN-3` | **Решения**: D-2, D-4, D-8, D-9,
D-12, D-13, D-14, D-19, D-22 | **FR**: FR-008…FR-018

> Канон сигнатур и алгоритмов фиксируется §EN-3; этот контракт переносит их **дословно** + порядок
> гардов. При расхождении побеждает §EN-3. Псевдокод `advance`/`Complete` — **контракт алгоритма**, не
> построчная реализация; Go-сигнатуры — контракт сигнатур.

## Назначение

Lifecycle инстанса: `Start`/`advance`/`Complete` (EM-9 с правками D-4/D-8/D-9), засыпание/пробуждение
EM-10, engine-Clock (D-2), реализация `eval.ProcessRuntime` (мост §EN-4). Engine **импортирует** `eval`,
`store`, `ast`, `value`, `errors`. Всё синхронно в одной горутине — mutex не нужен (конкурентность между
процессами ОС — WAL + busy_timeout, EM-11).

## Конструктор, Clock, опции (дословно §EN-3)

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

**Поля `Engine`** (минимум): `st store.Store`, `interp *eval.Interpreter`, `out io.Writer`,
`clock Clock` (дефолт `SystemClock{}`), `active []*activeFrame` — **стек активных инстансов** (пара
инстанс + его `processEnv`) для атрибуции хука `присвоить` (§EN-4): push при входе в тело шага, pop при
выходе; вложенный `запустить процесс` из тела шага кладёт новый кадр поверх.

Определения процессов engine берёт из интерпретатора (`interp.Process(name)`, §EN-4) — карта `procs`
из эскиза ARCH §7.1 **не передаётся** (сигнатура `NewEngine` выше — канон, ARCH синкается).

## Экспорт engine (дословно §EN-3)

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

- `Start` — реализация `eval.ProcessRuntime.StartProcess`; **тихий** запуск (D-13, FR-008).
- `Complete` — для CLI `complete`; синхронно (FR-015).
- `FormatTaskLine` — **единственное** место формата строки задачи (D-22); используется и `ladix tasks`,
  и сводкой `run`. `engine.SystemClock{}` экспортируется — `tasks` берёт `now` из него (инвариант D-2).
- Ошибки `Complete` уровня гардов — типизированы под CLI (различимы `errors.Is`/`errors.As`; точная
  Go-форма — на усмотрение имплементации, но **тексты** для пользователя формирует CLI по §EN-8.B).
  Runtime-ошибки тела/атрибута возвращаются как есть (уже типизированные ошибки Ladix с позицией) —
  CLI печатает канон §13, exit 1 (D-14).
- **Владелец печати строк 7–10 (§EN-7)** — сам `engine.Complete`: пишет их в `e.out`; CLI по
  `CompleteResult` **ничего не печатает** (структура — для тестов и маппинга exit-кодов).

## Машина состояний (ревизованный EM-9; ▼ = `SaveInstance`, перед каждым ▼ движок выставляет `UpdatedAt = clock.Now()`)

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
        перечитать inst; → ветка догона D-4 (выше)
    печать «задача <id> завершена» (§EN-7, строка 7)        # ДО advance: задача уже завершена фактом
    next ← следующий шаг по исходнику
    если next == ∅: inst.статус = выполнен; ▼ SaveInstance(inst)
    иначе: inst.CurrentStep = next; err ← advance(inst)      # может снова заснуть или провалиться
    если err: return err                                     # exit 1 (D-14); итоговой строки НЕТ
    печать итоговой строки инстанса (§EN-7, строки 9/10)
```

## Порядок гардов `complete` (строго; §EN-3, FR-015)

1. `LoadTask(taskID)` — `ErrTaskNotFound` → `ladix: задача '<id>' не найдена`, exit 2.
2. `LoadInstance(t.InstanceID)` — `ErrInstanceNotFound` → `ladix: инстанс '<id>' не найден`, exit 2.
3. **Дрейф-гарды Q3** (до любых мутаций):
   - процесс инстанса в файле? нет → `ladix: процесс '<имя>' не найден в определении`, exit 2.
   - `CurrentStep` ∈ `pd.Steps`? нет → `ladix: шаг '<имя>' не найден в определении процесса '<имя>'`, exit 2.
4. **Гард-догон D-4**: `t.Status == завершена`?
   - И инстанс `ожидает` И `CurrentStep == t.StepName` → строка 8 §EN-7 + до-продвижение (без
     `MarkTaskCompleted`), `CaughtUp=true`, exit 0.
   - иначе → `ladix: задача '<id>' уже завершена`, exit 2.
5. **Гарды D-8** (открытая задача):
   - `inst.Status != ожидает` → `ladix: инстанс '<p-id>' не ожидает (статус '<статус>')`, exit 2.
   - `inst.CurrentStep != t.StepName` → `ladix: задача '<t-id>' не соответствует текущему шагу инстанса '<p-id>'`, exit 2.
6. `MarkTaskCompleted` (D-12); проигравший гонку (`ErrTaskAlreadyCompleted`) → перечитать inst → ветка
   догона D-4.
7. Печать строки 7 → продвижение (`next == ∅` → `выполнен`; иначе `advance`) → печать строки 9/10.

**Любое нарушение гарда (шаги 1–5) → CLI-ошибка §EN-8.B, exit 2, инстанс НЕ тронут** (FR-015).
Дрейф-гарды Q3 идут **до** гарда «уже завершена» (негатив 4 §EN-9).

## Фаза атрибутов (D-9/D-18; FR-010/FR-011)

До тела (правка EM-9 под SPEC §11.3): (1) вычислить `исполнитель`/`срок` в кадре шага через
`interp.EvalExpr`, проверить тип (`исполнитель` → `Строка`, `срок` → `Длительность`; иначе ОшибкаТипа
§EN-8.A, позиция выражения атрибута → шаг **провален**); (2) исполнить тело `ExecStepBody`; (3) развилка
Task/продвижение. Ошибка вычисления атрибута = runtime-ошибка шага → `провален` (как ошибка тела, D-14).

## Провал инстанса (D-14; FR-014)

Runtime-ошибка тела шага (или атрибута): инстанс персистируется `провален`, далее ошибка всплывает как
обычная ошибка программы — канон SPEC §13 в stderr, fail-fast, **exit 1** (`run` и `complete` одинаково).
Двойная природа: в Store — статус, в CLI — диагностика; **дополнительной CLI/stdout-строки о провале
НЕТ**. Категория `ОшибкаПроцесса` в 006 **не вводится** (ей нечего нести; SPEC §13.1 строка «Процесса»
остаётся зарезервированной прозой).

## Гранулярность персиста (EM-9; FR-009)

`SaveInstance` — на создание, каждую смену статуса/шага, каждое `присвоить` (через хук §EN-4), терминал.
Перед каждым ▼ — `UpdatedAt = clock.Now()`. `присвоить` в цикле `пока` ⇒ много мелких записей — принято
в v1 (WAL тянет), батчинг — v2.

## Сбои Store вне хука «присвоить»/запуска (§EN-3; FR-018)

Engine оборачивает ошибки Store через `fmt.Errorf("<операция>: %w", err)`. Пути, инициированные
Ladix-узлом (`запустить процесс`, `присвоить`, process-builtins — в т.ч. весь `advance` внутри `Start`),
→ §EN-8.A (`сбой хранилища: <причина>`, позиция узла-инициатора, exit 1). Любая не-сентинельная ошибка
Store на CLI-путях `complete`/`tasks` (включая ▼ внутри `advance` из `Complete`,
`LoadTask`/`LoadInstance`/`MarkTaskCompleted`/`ListPendingTasks`, декод type-tagged JSON битой БД) →
§EN-8.B `ladix: сбой хранилища: <причина>`, exit 2 (Ladix-позиции нет — канон §13 неприменим).

## Тесты (engine unit, по образцу пакетов 003/004)

- Сценарий А (§EN-9): `Start` с `WithClock(фикс. 2026-05-31 00:00:00 Local)` → байт-точный stdout (5
  строк) + состояние Store (инстанс `p-000001` `ожидает`/`провести_встречу`, `Variables`, задача
  `t-000001` `открыта`).
- `Complete` цепочка: пробуждение → следующая задача → терминал (`выполнен`).
- Гарды: `LoadTask` нет, `LoadInstance` нет, дрейф процесса/шага, догон D-4 (строка 8 + exit 0), уже
  завершена (exit 2), D-8 фабрикованные (инстанс `выполняется`; `StepName ≠ CurrentStep`) — оба «инстанс
  не тронут».
- `FormatTaskLine`: с дедлайном/без, `ПРОСРОЧЕНА`/нет; `Overdue` при nil-дедлайне → false.
- Фаза атрибутов: `исполнитель: 42` → ОшибкаТипа + `провален`; тело `1/0` → `деление на ноль` + `провален`.
- Абсолютизация дедлайна (D-19): множители единиц; `мес` календарно.
