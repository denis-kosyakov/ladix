# Модель автоматизации Ladix — веха M2 (связывающий якорь B1–B6) — §AU

> **Статус:** M2 (исходная редакция якоря). Источник истины для автономного поезда фич B1–B6.
> **Назначение:** закрыть 5 аддитивных разрывов автоматизации, чтобы золотой сценарий
> «Контроль плана продаж с эскалацией» (`docs/v2-charter.md §2`) замкнулся под `serve` и
> **пережил рестарт демона**. Якорь предрешает все вопросы скиллов, чтобы impl-чат прогнал
> `specify → plan → tasks → analyze → implement` без возвратов (паттерн 009).
> **Предшественники:** v1 (specs 001–009), M1 аналитика (010 коннекторы + 011 окна),
> M-DX диагностика (012). Бэкенд-контекст — `docs/engine-model.md §EN`, `docs/trigger-model.md §TR`,
> `docs/execution-model.md`. Якорь НЕ повторяет закрытое; ссылается.

---

## §AU-0. Граница вехи M2 (что входит / что отложено)

### Входит в M2 (6 подфич, 7 автономных прогонов)

| Код | Подфича | Слой | Гейтит |
|---|---|---|---|
| **B1** | Захват результата `вызвать` как выражения (`присвоить r = вызвать crm(x)`) | фронтенд + eval + 1 метод интерфейса | **B2** |
| **B2** | Реальные `вызвать`/`уведомить` — HTTP-вебхук через Option движка; дефолт = печать-стаб | engine | — |
| **B3** | Payload задачи: `ladix complete <файл> <id> --data '{…}'` | CLI + engine + eval-инжект | — |
| **B4a** | Триггер эскалации — фронтенд: `когда задача просрочена в P.шаг:` + run-заглушка | фронтенд (parser/AST/семпроход) | **B4b** |
| **B4b** | Эскалация — бэкенд: 4-я фаза `tick()` (скан просроченных), durable `Task.Escalated`, инжект `InstanceVariables` | daemon + store-поле | — |
| **B5** | `ladix start <процесс> [аргументы]` — внешний запуск с типизир. литералами argv | CLI | — |
| **B6** | `ladix inspect <id>` — снимок инстанса + лёгкая история задач | CLI + 1 метод Store | — |

**Топо-рёбра (фиксированы):** `B1 → B2` (захват результата до интеграций, иначе их переписывать);
`B4a → B4b` (фронтенд до бэкенда); `B4b` мягко после `B2` (тело эскалации делает реальную доставку);
`B3 / B5 / B6` независимы. Демо-пара: `B5 → B6` (стартовали → инспектируем). Рекомендованный
порядок прогонов: **B1 → B2 → B3 ∥ B4a → B4b ∥ B5 → B6**.

### Отложено за пределы M2 (путь отступления — §AU-13)

- **Параллельные шаги (`параллельно`)** — единственный XL-каскад (хартия §3); вне v2.
- **Серверная модель** (демон-владелец-БД + `--json`) — развилка входа M3-C3.
  M2 идёт на варианте (а): CLI-над-общим-SQLite + аддитивные расширения. *HTTP-приём событий выделен
  из этой связки и реализован отдельно после v2 (трек B, opt-in `serve --listen`, `docs/inbound-events-model.md`).*
- **Двойные часы** (`eval.Clock` дневной vs `engine.Clock` wall-clock) — закрывается в M3-C4 (`WithClock`).
  M2 терпит: эскалация и golden используют один инжектируемый `engine.Clock` (§AU-6.4).
- **Версионирование определений / стабильные ключи триггеров / exactly-once / миграции схемы** — M3-C1/C2.
  До M3 честный at-least-once допустим (эффекты-стабы и эскалация идемпотентны по `Task.Escalated`).
- **Реестр ролей / тип `Роль`** — НЕ берём: эскалация «руководителю» = строка-цель `Notify(target string)`,
  разграничение ролей сценарию §2 не требуется (хартия §4, развилка §8 закрыта на backlog).
- **Реальная сетевая доставка под `вызвать`/`уведомить`** ограничена HTTP-POST на конфигурируемый
  базовый URL; брокеры/очереди/ретраи — backlog M3.

---

## §AU-1. Решения kickoff (locked владельцем 2026-06-16, обязательны к исполнению)

| # | Решение | Обоснование |
|---|---|---|
| **D-AU-1** | **B1 = `вызвать` как ВЫРАЖЕНИЕ** (вариант «б» развилки §8). Дуально: statement-форма `CallAction` (существует, fire-and-forget, `CallExternal`) СОХРАНЯЕТСЯ; добавляется **выражение** `CallExternalExpr` (захват результата, новый метод `CallExternalResult`; имя `CallExpr` уже занято постфикс-вызовом). Интерфейс `ProcessRuntime` 7→8 (+`CallExternalResult`). `уведомить` остаётся ТОЛЬКО statement. | Прецедент `RunProcessExpr`. Аддитивно к §5-инварианту 1 (новый метод, не смена сигнатуры). Старые `вызвать ИТ(...)` как действие парсятся прежним путём. |
| **D-AU-2** | **B2 канал = HTTP-вебхук** (не файл, против рекомендации kickoff). Драйвер инжектится Option'ом движка. **Дефолт-драйвер = печать-стаб** → держит §EN-7 golden (≥6 пинов). Реальный = `POST` JSON `{"цель": target, "данные": [args]}`. Базовый URL — CLI-флаг `--webhook` / env `LADIX_WEBHOOK`; `.ladix` остаётся чистым (цель = логическое имя, не URL). Детерминизм тестов — `httptest`. | Нужен детерминированный приёмник без сети. Логическое имя цели — путь отступления к реестру интеграций. |
| **D-AU-3** | **B3 = payload `данные`**: read-only эфемерная `value.Запись`, декодируется единым JSON-кодеком, поднятым из `daemon/events.go` (`payloadToRecord`) в общий пакет. Без `--data` → пустая `Запись`. | «Эфемерный» = живёт только сквозь догон одного `complete`, не персистится в инстанс (путь отступления — персист в M3). |
| **D-AU-4** | **B4 KW = КОНТЕКСТНЫЙ разбор**: `задача`/`просрочена` остаются IDENT (НЕ глобальные KW). Распознавание — в ПАРСЕРЕ по лексеме после `когда`. | Глобальный KW ломает v1-IDENT: `пусть задача = 10` доказанно `exit 0` сегодня. Лексер остаётся контекстно-независимым (SPEC §2:76) — контекст применяет парсер. Осознанное D-исключение (§AU-6.1). |
| **D-AU-5** | **B4 durable = поле `Task.Escalated`** (+колонка SQLite), персист через СУЩЕСТВУЮЩИЙ `SaveTask` (без нового Store-метода для скана; скан фильтрует `ListPendingTasks` в демоне). Эскалация — **одноразовая по задаче** (не `TriggerState`, не re-arm). | Restart-safe: после рестарта скан видит `Escalated=true` → нет повтора. Per-task ключ обходит проблему стабильных ключей триггеров (отложена в M3-C1). |
| **D-AU-6** | **B4 инжект = все `InstanceVariables`** инстанса в read-only env тела триггера эскалации. | Тело `уведомить руководитель(факт)` читает переменную процесса `факт`. Анализ тела — lenient-scope (§AU-6.1.3). |
| **D-AU-7** | **B5 = типизир. литералы argv + сверка арности на CLI**. | `ladix start эскалация_плана 2500000`. Парсер литералов CLI-уровня (§AU-7.2). Арность → CLI-ошибка exit 2. |
| **D-AU-8** | **B6 = снимок + лёгкая история** (открытые + завершённые задачи инстанса) → новый read-only метод `ListTasksByInstance`. Store 15→16. | Наблюдаемость оператора (хартия §3); полная трассировка/`explain`/`--json` отложены в M3-C5. |
| **D-AU-9** | **ОТОЗВАНО в M3** (было: сброс схемы допустим, БД тестовые). M2-эра пересоздавала БД как stopgap; M3-C2a заменяет сброс на **forward-only ALTER** (`PRAGMA user_version`, база=1 = «схема 006/007/018»), сохраняя данные. Колонка `Task.Escalated` — часть базовой схемы v1 (ALTER не нужен); новые таблицы v2+ (outbox) = миграция 1→2. | Полные миграции доставлены M3-C2a; см. `reliability-model.md` §C-2a (D-C-2). |
| **D-AU-10** | **Единая `--db`** обязательна для `start/complete/inspect/serve/run/tasks/emit`; `metric` остаётся MemoryStore. | Предусловие демо: инстанс, созданный `start`, виден `serve`/`complete`/`inspect`. Без `--db` MemoryStore эфемерен (§AU-9). |

---

## §AU-2. Дельта несущих швов (аддитивна; цели drift-watch на M2-гейте)

### Интерфейс `ProcessRuntime` (объявлен в `internal/eval`): 7 → 8 методов

Добавляется РОВНО один метод (D-AU-1). Существующие 7 — дословно как `eval/runtime.go:9-37`, не трогаются.

```go
type ProcessRuntime interface {
    StartProcess(name string, args []value.Value) (string, error)
    AssignProcessVar(name string, v value.Value) error
    CallExternal(target string, args []value.Value) error                    // СОХРАНЯЕТСЯ (вызвать-statement)
    CallExternalResult(target string, args []value.Value) (value.Value, error) // НОВЫЙ (B1, вызвать-выражение)
    Notify(target string, args []value.Value) error
    InstanceStatus(id string) (status string, ok bool, err error)
    InstanceVariables(id string) (vars value.Запись, ok bool, err error)
    UserTasks(assignee string) ([]value.Запись, error)
}
```

**Однонаправленность ребра `engine → eval` не меняется.** `engine.Engine` реализует обновлённый
интерфейс (`var _ eval.ProcessRuntime = (*Engine)(nil)`, `engine/runtime.go:16`). Чтобы не дублировать
эффект, `CallExternal` делегирует: `func (e *Engine) CallExternal(t string, a []value.Value) error { _, err := e.CallExternalResult(t, a); return err }`.

### Контракт `Store`: 15 → 16 методов

Добавляется РОВНО один read-only метод (D-AU-8); существующие 15 (`store.go:12-34`) не трогаются.

```go
ListTasksByInstance(instanceID string) ([]*Task, error)  // открытые + завершённые задачи инстанса, порядок ID ASC
```

Реализация в `MemoryStore` и `SQLiteStore` (изолировано). SQLite: `SELECT … FROM tasks WHERE instance_id = ? ORDER BY id ASC`.

### Поле `Task.Escalated` + колонка (D-AU-5)

```go
type Task struct {
    ID          string
    InstanceID  string
    StepName    string
    Assignee    string
    Deadline    *time.Time
    Status      TaskStatus
    CreatedAt   time.Time
    CompletedAt *time.Time
    Escalated   bool          // НОВОЕ: задача уже эскалирована (durable, одноразово)
}
```

**SQLite-кодек `Task` — ВСЕ точки правки (иначе `Escalated` молча теряется на рестарте):**
1. DDL `tasks` (`sqlite.go:33-42`) +колонка `escalated INTEGER NOT NULL DEFAULT 0`;
2. `SaveTask` INSERT-список колонок + `ON CONFLICT(id) DO UPDATE SET … escalated = …` (`sqlite.go:161-177`);
3. **ВСЕ читатели `tasks`** (SELECT + общий `buildTask`/`scanTask`, `sqlite.go:296/310`): `LoadTask`
   (`sqlite.go:179-184`), `ListPendingTasks` (`sqlite.go:186-224` — ГЛАВНЫЙ читатель скана §AU-6.2.2) и
   новый `ListTasksByInstance` (§AU-8) — добавить `escalated` в SELECT и в сигнатуру `buildTask`/`scanTask`.
   `UserTasks` (`engine/runtime.go`) делегирует `ListPendingTasks` → наследует `Escalated` автоматически.

`MemoryStore.SaveTask` (`memory.go:58-63`) перезаписывает запись глубокой копией; `copyTask`
(`memory.go:245`) копирует `Escalated` тривиально (`cp := *t`, bool-значение). **Миграция схемы
(D-AU-9 ОТОЗВАНО в M3):** колонка `escalated` — часть базовой схемы v1 (`const ddl`); на M2-эре
добавлялась пересозданием БД, M3-C2a заменяет сброс на forward-only ALTER (`PRAGMA user_version`, см.
`reliability-model.md` §C-2a) — новые таблицы/колонки v2+ мигрируют без потери данных. Подтверждено
вживую: `SaveTask` в обоих Store —
настоящий UPSERT (`sqlite.go:161` `ON CONFLICT … DO UPDATE`; `memory.go:58` перезапись карты), durable-флаг
переживает рестарт через перечитку `ListPendingTasks` из той же `--db`.

> **Инвариант 2 (хартия §5):** транзакционного комбо «завершить+эскалировать» нет; durable-флаг
> ставится отдельным `SaveTask`. Корректность — через идемпотентность по `Escalated` (D-4-аналог).

---

## §AU-3. B1 — захват результата `вызвать` (фронтенд + eval)

### §AU-3.1 Грамматика и AST

`вызвать` получает ВТОРОЙ контекст разбора — выражение (statement-форма `CallAction` остаётся).

- **Statement-позиция** (ведущий токен в `parseStatement`): `вызвать Ident "(" ArgList? ")"` →
  `ast.CallAction` (без изменений, `step.go:21-25`). Эффект через `CallExternal`, результат не нужен.
- **Выражение-позиция** (`parsePrimary`): добавить ветку `case lexer.KW_CALL` → `parseCallExternalExpr()`:

```go
// ВНИМАНИЕ: имя CallExpr/NewCallExpr УЖЕ ЗАНЯТО постфиксным вызовом f(args) (ast/expr.go:31-40,
// Callee Expression) — новый узел B1 ОБЯЗАН называться иначе, иначе redeclared в пакете ast.
type CallExternalExpr struct {    // ast/expr.go (новый узел B1, по образцу RunProcessExpr:69)
    exprBase
    Target Ident
    Args   []Expression
}
func NewCallExternalExpr(pos Position, target Ident, args []Expression) *CallExternalExpr
```

`parseCallExternalExpr` зеркалит `parseRunProcess` (`parse_expr.go:240-252`): потребляет `KW_CALL`,
`expect(IDENT, "имя цели")`, опциональный `"(" ArgList? ")"`. Добавить `lexer.KW_CALL` в
`startsExpression` (`parse_expr.go:18`, case-список рядом с `KW_RUN`/`KW_VALUE`/`KW_EVENT`).

> **Развязка контекста (без неоднозначности):** `parseStatement` ловит ведущий `KW_CALL` ДО входа в
> `parseExpression` (`parse_stmt.go:77` → `parseStepAction` → `CallAction`), поэтому `вызвать crm(x)`
> отдельной строкой → `CallAction`. `присвоить` = `KW_SET` → `parseStepAction` строит **`AssignAction`**
> (`ast/step.go`, НЕ `AssignStmt` — `AssignStmt` рождается лишь из голого `r = …`), а его правая часть
> через `parseExpression` → `parsePrimary` → `CallExternalExpr`. Постфиксы (`.поле`, `(…)`, `[…]`)
> навешиваются на `CallExternalExpr` штатной цепочкой `parsePostfix` (как на любой primary, в т.ч. RunProcessExpr).

### §AU-3.2 Семантика

`вызвать`-выражение разрешено везде, где разрешено выражение (RHS `присвоить`, аргумент, элемент
списка). Цель — свободное логическое имя (строка), НЕ символ программы → резолв символа НЕ нужен,
арность не проверяется (внешняя система). Новых статических диагностик B1 НЕ вводит. В теле триггера
read-only барьер (007a) сохраняется: `вызвать`-выражение можно ВЫЧИСЛЯТЬ (эффект), но `присвоить`
из него в глобал по-прежнему запрещён барьером.

> **Намеренная асимметрия контекст-гарда (impl, не сломай тесты):** statement-форма (`*ast.CallAction`/
> `*ast.NotifyAction`/`*ast.AssignAction`) подпадает под step-only контекст-гард действий
> (`analyze.go:584-592`, замки `analyze_decl_test.go:422`, `analyze_trigger_test.go:92`) — `вызвать f()`
> отдельной строкой вне шага запрещён. `вызвать`-ВЫРАЖЕНИЕ (`CallExternalExpr`) проверяется через
> `checkExpr` и под этот гард НЕ подпадает → разрешено в теле функции/триггера/метрики. Это ПО ДИЗАЙНУ.
> Существующие гард-замки не трогаются, т.к. ведущий `вызвать` по-прежнему → `CallAction`.

### §AU-3.3 Исполнение (`internal/eval/expr.go`)

`evalExpr` получает кейс `*ast.CallExternalExpr`: вычислить `args` через `evalArgs` (слева направо,
`stmt.go:144-154`) → `i.runtime.CallExternalResult(c.Target.Name, args)` → вернуть `value.Value`.
Ошибка заворачивается `runtimeErrWrap(c.Pos(), err)` (`interpreter.go:189-191`) — фактический Go-тип
**`errors.ОшибкаВыполнения` с цепочкой `Cause`** (закрывает TODO(D-14), §AU-4.4). Это ЕДИНАЯ категория
сбоя внешнего вызова во всём якоре (§AU-4.3 — то же; «категория Процесса» из kickoff D-AU-2 F2 —
концептуальная рамка «сбой на стороне процесса», конкретный тип — `ОшибкаВыполнения`, т.к. путь идёт
через `runtimeErrWrap`). Под дефолт-стабом (§AU-4) возвращается `value.None` (Пусто); под HTTP-драйвером —
декодированный ответ (§AU-4.3).

---

## §AU-4. B2 — реальные `вызвать` / `уведомить` (engine, драйвер через Option)

### §AU-4.1 Драйвер как Option движка

Движок получает поле-драйвер внешних эффектов, инжектируемое Option'ом (паттерн `WithClock`,
`clock.go:22-24`). По умолчанию — печать-стаб (держит §EN-7 golden). CLI включает HTTP-драйвер
флагом/env (§AU-4.4).

```go
type ExternalCaller interface {
    Call(target string, args []value.Value) (value.Value, error)  // вызвать → результат
    Notify(target string, args []value.Value) error               // уведомить → эффект
}
func WithExternalCaller(c ExternalCaller) Option   // engine/clock.go или engine/options.go
// e.caller по умолчанию = printCaller{out: e.out}
```

`engine.CallExternalResult` / `CallExternal` / `Notify` делегируют `e.caller`. `CallExternal` =
`CallExternalResult` с отбросом значения (§AU-2).

### §AU-4.2 Дефолт-драйвер `printCaller` (держит golden §EN-7 байт-точно)

```go
// Call: печатает строку формата engine/runtime.go:41-48, возвращает (value.None, nil)
fmt.Fprintf(out, "[вызов] %s(%s)\n", target, strings.Join(parts, ", "))   // parts[k] = value.String(args[k])
// Notify: формат engine/runtime.go:54-65 (без аргументов и с аргументами через " ")
fmt.Fprintf(out, "[уведомление] %s\n", target)                            // len(args)==0
fmt.Fprintf(out, "[уведомление] %s: %s\n", target, strings.Join(parts, " "))
```

§EN-7-пины (`engine_test 108/167/176/185/194`, `main_test 117/200/235`) НЕ ломаются: дефолт-драйвер =
текущая логика, перенесённая в `printCaller`. `вызвать`-выражение под стабом печатает ту же
`[вызов]`-строку и возвращает Пусто.

### §AU-4.3 HTTP-драйвер `webhookCaller`

`POST <baseURL>` с телом `{"цель": <target>, "данные": [<args как JSON>]}` (логическое имя цели — в
payload). `Content-Type: application/json`. **Кодирование `args` (value → plain-JSON) — НОВЫЙ энкодер**
(см. §AU-5.2: поднятый кодек — ДЕКОДЕР-only; `store/codec.encodeValue` даёт ТЕГИРОВАННЫЙ `{"т","зн"}` —
не годится; `value.String` — дисплей-репр, не валидный JSON). Парный энкодер пишется с нуля:
`Целое/Дробное`→число, `Строка`→quoted, `Булево`, `Пусто`→`null`, `Список`→array, `Запись`→object;
`Дата/Длительность/Период` → строковая форма (решение impl, задокументировать). Ответ `Call` декодируется
через `decodeValue` (одно значение любого типа, `events.go:148` — НЕ `payloadToRecord`, который жёстко
требует объект, `events.go:106`): JSON-объект → `Запись`, скаляр → `Value`. **Пустое тело → Пусто
проверяется в `webhookCaller` ДО `decodeValue`** (сам `decodeValue` на пустом потоке вернёт `io.EOF`,
`events.go:149`, а не Пусто). `Notify` ответ игнорирует (best-effort). Сетевой/HTTP-сбой → `error` → eval оборачивает `errors.ОшибкаВыполнения`
(§AU-3.3, ЕДИНАЯ категория — без «Процесса»). Тайм-аут клиента — конечный (напр. 30с), путь отступления — конфиг в M3.

### §AU-4.4 Точка активации `runtimeErrWrap` (закрытие TODO D-14)

TODO(D-14) активируется. Исполняемые строки (НЕ комментарий): `evalCallAction` — заменить
`runtimeErr(c.Pos(), err.Error())` на `runtimeErrWrap(c.Pos(), err)` на **`eval/stmt.go:118`**;
`evalNotifyAction` — то же на **`:138`**; сопутствующие TODO-комментарии (`:113-115` / `:133-136`)
удалить. Теперь сбой реального драйвера несёт цепочку `Cause` (для `errors.As/Is`). `evalExpr(*CallExternalExpr)`
сразу использует `runtimeErrWrap` (§AU-3.3). Все три точки → `errors.ОшибкаВыполнения` (единая категория).

### §AU-4.5 CLI-проводка драйвера

`run`, `serve`, `complete`, `start` принимают `--webhook <URL>` (или env `LADIX_WEBHOOK`). Задан →
`NewEngine(..., WithExternalCaller(webhookCaller{baseURL, httpClient}))`. Не задан → дефолт-стаб.
**КРИТИЧНО для DoD:** под `serve` `--webhook` проводится в ТОТ ЖЕ экземпляр движка, чьи `Notify`/`Call`
зовёт `checkDeadlines → fireDeadlineBody` (§AU-6.2.3) и тело метрика/событие-триггеров. Иначе эскалация
под `serve` напечатает стаб вместо реальной доставки — тихий разрыв §AU-12.C. `complete` тоже принимает
(эффекты могут сработать на догоне). Формат CLI-ошибки невалидного URL — `ladix: неверный URL вебхука '<URL>'` exit 2 (§EN-8.B).

---

## §AU-5. B3 — payload задачи через `complete` (CLI + engine + eval-инжект)

### §AU-5.1 CLI

`completeMain` (`main.go:281`) получает опциональный флаг `--data '<json-объект>'`. Грамматика
вызова: `ladix complete <файл> <id> [--db путь] [--data '{…}'] [--webhook URL] [--max-depth N]`.
Без `--data` → пустой payload. Невалидный JSON → `ladix: неверный JSON в --data: …` exit 2.

### §AU-5.2 JSON-кодек: декодер (лифт) + парный энкодер (новый)

**Целевой пакет — `internal/jsonval`** (НОВЫЙ, нейтральный к слоям). Импортёры: `daemon` (события),
`engine` (декод ответа вебхука B2 + энкодер тела), `cmd/ladix` (декод `--data` B3 на CLI, §AU-5.3 —
корень композиции, импорт допустим). **НЕ `eval`** — иначе нарушится инвариант 1/3 хартии §5 (`eval`
импортирует только `ast`+`value`; `данные` приходит в eval уже как `value.Запись` через `stepEnv.Define`).
Проверить импорт-граф после лифта. НЕ класть в `internal/value` (его импортирует фронтенд → утечка JSON в фронтенд).

**Декодер (лифт как есть):** `payloadToRecord`/`decodeObject`/`decodeValue`/`decodeArray`/`numberToValue`
(`events.go:95-206`) — пакетные функции без зависимости на `*Daemon` (импортируют только
`bytes/encoding/json/fmt/strings` + `value`) → лифтятся в `internal/jsonval` чисто. Потребители: B2
(ответ вебхука — через `decodeValue`, §AU-4.3), B3 (payload — через `payloadToRecord`), события 007b.
JSON-объект → `value.Запись` (ключи в порядке появления); число без `.eE` → `Целое`, иначе `Дробное`
(int64-overflow → `Дробное`); `null` → Пусто; вложенные → `Запись`/`Список`.

> **НЕ «один декодер на всё»:** в `eval/source_loader.go:119-216` живёт ВТОРОЙ JSON→value декодер
> (источники M1) с ДРУГОЙ семантикой (int-overflow → ОШИБКА §SM-9.B, даты не распознаются §9.4). Он
> остаётся отдельным — НЕ сливать (толерантная деградация payload несовместима со строгостью источников).
> «Единый» относится строго к B2/B3/события.

**Энкодер (новый, value → plain-JSON):** для тела вебхука (§AU-4.3) — пишется с нуля (поднятый кодек
только декодирует; `store/codec.encodeValue` тегированный — не годится; `value.String` не JSON).

**Перенос теста:** `TestPayloadToRecordValueTypes` (`daemon/events_test.go:174`) зовёт неэкспортированный
`payloadToRecord` напрямую → после лифта ОБЯЗАН переехать в `internal/jsonval` (или обращаться к
экспортированному имени). «Байт-в-байт» относится к golden ВЫХОДА (serve/emit), unit-тест кодека
физически меняет импорт/имя — это co-land, не «без изменений».

### §AU-5.3 Инжект `данные` (read-only, эфемерный)

**Где декодируется (закрыто):** `--data '{…}'` → `value.Запись` декодирует **CLI** (`completeTask`,
`main.go`) через `jsonval.payloadToRecord` ПЕРЕД `eng.Complete` (тут же валидация: невалидный JSON →
`ladix: неверный JSON в --data: …` exit 2, §AU-5.1). Движок получает уже готовую `value.Запись`
(не строку) — поэтому `cmd/ladix` импортирует `jsonval` (§AU-5.2). `данные` делается доступной под этим
именем во время догона/продвижения.

**Прокидывание payload — ЧЕТЫРЕ функции (cite-точно, иначе compile-gap):** прямой путь
`Complete(taskID)` (`engine.go:103`) → `advanceAfterComplete(inst, caughtUp)` (`engine.go:184`) →
`advance(inst)` (`engine.go:237`); путь ДОГОНА (`caughtUp=true`) идёт через `catchUp(inst, t)`
(`engine.go:172`, зовётся из `Complete` на `engine.go:135/159`, сам зовёт `advanceAfterComplete` на
`engine.go:178`). ВСЕ ЧЕТЫРЕ расширяются параметром `data value.Запись`:
`Complete(taskID, data)` → [`catchUp(inst, data, t)` →] `advanceAfterComplete(inst, data, caughtUp)` →
`advance(inst, data)`. На догоне человеческий ввод тоже актуален → `data` прокидывается и через `catchUp`.

**Точка `Define` и область инжекта:** `advance` крутит ЦИКЛ шагов (`engine.go:247-313`), строит per-step
`stepEnv := eval.NewEnvironment(processEnv)` (`engine.go:257`); `данные` инжектится `stepEnv.Define("данные", data)`
(НЕ `processEnv` — иначе переживёт догон через персист, нарушив эфемерность) перед `ExecStepBody`
(`engine.go:270`). **Только ПЕРВЫЙ шаг догона** видит `данные`; механизм (закрыт, не гадать): локальная
`cur := data` перед циклом, инжект `cur` в `stepEnv`, после первой итерации `cur = value.NewRecord(nil, nil)`
(пустая `Запись`) — последующие шаги того же догона видят пустую. Read-only барьер (как тело триггера):
шаг ЧИТАЕТ `данные.итог`, но `присвоить данные = …` запрещён; чтобы сохранить — `присвоить факт = данные.итог`
(своя переменная, через `AssignProcessVar` персистится в инстанс). `данные` отсутствует вне догона →
следующий `complete` без `--data` видит пустую `Запись` (`данные.итог` → Пусто, открытая-запись семантика).
Все четыре расширяемые сигнатуры — внутренний API движка (НЕ шов ProcessRuntime, §5-инвариант 1 не нарушен);
существующие вызовы передают пустую `Запись`.

---

## §AU-6. B4 — эскалация дедлайна (УСИЛЕННЫЙ раздел: фронтенд + бэкенд + durable × рестарт)

> Самый широкий пункт M2 (трогает все слои). Разрезан на B4a (фронтенд) и B4b (бэкенд) — единственный
> пункт с разрезом. Усиление: durable-модель и обязательный golden `serve → эскалация → рестарт →
> нет повтора` прописаны ДО импла.

### §AU-6.1 B4a — фронтенд: новый вид триггера

#### §AU-6.1.1 Синтаксис и контекстный разбор (D-AU-4)

```
когда задача просрочена в <Процесс>.<Шаг>:
    <тело>
```

Лексемы: `когда` = `KW_WHEN`; `в` = `KW_IN`; **`задача`/`просрочена` = IDENT** (НЕ ключевые слова).
Распознавание контекстное в `parseTriggerDecl` (`parse_decl.go:406-422`): после `KW_WHEN` диспетчер
сейчас переключается по типу токена (`KW_METRIC`/`KW_EVENT`/`KW_SCHEDULE`). Добавить ветку: если
`p.peek().Type == lexer.IDENT && p.peek().Lexeme == "задача"` → `parseDeadlineTrigger()`. Иначе —
прежний `default` → `SE-TRIGGER-KIND`.

`parseDeadlineTrigger`:
1. `advance()` потребляет IDENT `задача`;
2. `expectLexeme("просрочена")` — **НОВЫЙ хелпер** (в `parser` его нет: есть `expect(TokenType, string)`
   `parser.go:79` и `expectCompOp` `parse_decl.go:441`). Сверяет `tok.Type == lexer.IDENT && tok.Lexeme == want`,
   иначе `p.error(pos, msgExpected(want, tok))` → код `SE-EXPECTED` (счёт 14 НЕ меняется, §AU-10.A);
3. `expect(lexer.KW_IN, "в")`;
4. `expect(lexer.IDENT, "имя процесса")` → `Process Ident`;
5. `expect(lexer.DOT, ".")` (токен `DOT`, `token.go:81`);
6. `expect(lexer.IDENT, "имя шага")` → `Step Ident`.

> **D-исключение к SPEC §2:76 (контекстная независимость лексера):** лексер по-прежнему эмитит
> `задача`/`просрочена` как IDENT без контекста; КОНТЕКСТ применяет ПАРСЕР (распознаёт лексему IDENT
> в позиции после `когда`). Это сохраняет v1: `пусть задача = 10`, `задача()` и т.п. вне позиции
> триггера остаются обычными идентификаторами. Записать как осознанный отход в SPEC §2 / grammar.

#### §AU-6.1.2 AST-узел

```go
type DeadlineTrigger struct {   // ast/trigger.go, рядом с MetricTrigger/EventTrigger/ScheduleTrigger
    specBase
    Process Ident
    Step    Ident
}
func NewDeadlineTrigger(pos Position, process, step Ident) *DeadlineTrigger
func (*DeadlineTrigger) triggerSpec() {}   // маркер TriggerSpec
```

`Pos()` = токен `задача` (specBase). `TriggerDecl.Spec` = четвёртый конкретный тип маркера `TriggerSpec`.

#### §AU-6.1.3 Семантический проход (`analyze.go` checkTrigger, `:319`)

Кейс `*ast.DeadlineTrigger`:
- **Процесс объявлен:** `Process.Name` ∈ объявленных процессов, иначе `СемантическаяОшибка`
  (переиспользовать формулировку §PM-6.C/§TR-4 «процесс '…' не объявлен», если есть; иначе новый
  exact-match код в семействе триггеров).
- **Шаг существует в процессе:** `Step.Name` ∈ шагов процесса, иначе `СемантическаяОшибка`
  («шаг '…' не найден в процессе '…'»).
- **Тело: scope-leniency наследуется бесплатно, но allow-список действий потребовал НОВОГО флага
  `inTriggerBody` (⚠ ПОПРАВКА гейта M2, ратифицировано владельцем 2026-06-17).** Исходная версия этого
  раздела ошибочно утверждала «lenient-scope БЕСПЛАТНО, никакого нового флага не нужно, тело ТОЧНО как
  расписание-триггер». Это смешало две независимые грани; верно следующее:
  - *Scope (бесплатно):* семпроход и так НЕ проверяет объявленность плоских Ident («плоский Ident
    НЕ резолвится — declaredness рантайму», `analyze.go`) → свободный `факт` в теле эскалации статической
    «не объявлено» НЕ даёт, резолвится в рантайме против инжектированных переменных инстанса (§AU-6.2.3);
    неразрешённый → рантайм-`ОшибкаВыполнения`. Этой грани новый флаг не нужен.
  - *Allow-список действий (НЕ бесплатно):* до M2 `присвоить`/`вызвать`/`уведомить` (форма-statement)
    были запрещены ВО ВСЕХ телах триггеров одним гардом `if !inStep` (`95f61e7:analyze.go:584-590`,
    «действие '…' допустимо только в шаге процесса»). Значит расписание-триггер их тоже запрещал — а тело
    эскалации `уведомить руководитель` было бы НЕВАЛИДНЫМ, durable golden §AU-12.B недостижим. Импл (коммит
    `e838afa`) протянул НОВЫЙ параметр `inTriggerBody bool` сквозь `checkStmts`/`checkStmt`/`checkElse`
    (`analyze.go:548`/`:557`/`:636`) и расщепил кейс: `case *ast.CallAction, *ast.NotifyAction: … if !inStep
    && !inTriggerBody { return semErr(…) } return nil` (`analyze.go:608-622`) — `вызвать`/`уведомить`
    РАЗРЕШЕНЫ при `inTriggerBody=true`. `присвоить` (`*ast.AssignAction`) остался `if !inStep`
    (`analyze.go:624-629`) — страж над-ослабления.
  - **Скоуп ослабления — ВСЕ виды триггера (метрика/событие/расписание/задача), ратифицировано как
    намеренная фича.** `checkTriggerBody` (`analyze.go:432`, сигнатура 2 флага не менялась) для ЛЮБОГО вида
    триггера зовёт `checkStmts(…, inTriggerBody=true)` (`analyze.go:438`, захардкожено) → `вызвать`/`уведомить`
    валидны в любом теле триггера (напр. `когда метрика выручка < N: уведомить директор …`; `когда событие
    X: вызвать crm(…)`). Залочено двусторонне: позитивы `analyze_trigger_test.go:230-243` (метрика/событие/
    расписание — комментарий «РЕГРЕСС §AU-6.1.3 … Раньше был замок-запрет»); негативы `:92` (присвоить в теле
    метрики отвергнут) и `:287` (присвоить в теле эскалации отвергнут).
- Действия-шага (`исполнитель:`/`срок:`) в теле триггера запрещены (наследуется §TR-7.C). `значение`/`событие`
  в теле эскалации запрещены наследуемым контекст-гардом §TR-7.A (TR-VAL-CTX/TR-EVT-CTX). Read-only барьер
  тела — рантайм (TR-BODY-RO, `NewTriggerBodyEnv`+`markBoundary`, §AU-6.2.3). Разрешено императивное ядро +
  `уведомить`/`вызвать`/`запустить процесс` (реальные эффекты в B2).

#### §AU-6.1.4 Поведение под `run` (заглушка)

`run` — однопроходный, без живых задач/дедлайнов. Эскалация-триггер под `run` НЕ исполняется
(нет таймера/задач), печатает строку-заглушку и тело НЕ выполняет — зеркально событие/расписание-
триггерам под `run` (`trigger_run.go:49,51`). **Точная строка (канон, ветка в `runTriggers`):**
```go
fmt.Fprintf(w, "задача триггер '%s.%s' требует serve (фича 007b)\n", spec.Process.Name, spec.Step.Name)
```
(тот же стем `требует serve (фича 007b)`, что у существующих заглушек). Компиляция всех 4 форм триггера
на общих стадиях (lex/parse/analyze) одинакова (§TR-8.4).

### §AU-6.2 B4b — бэкенд: 4-я фаза `tick`, durable, инжект

#### §AU-6.2.1 Четвёртая фаза `tick()` (крит-инвариант 007b)

`tick()` (`tick.go:10`) сегодня: `d.mu.Lock(); defer d.mu.Unlock(); ResetRunState(); drainEvents();
evalMetrics(); checkSchedules()`. Добавить ЧЕТВЁРТУЮ фазу В КОНЕЦ, под тем же `d.mu`:

```go
func (d *Daemon) tick() {
    d.mu.Lock()
    defer d.mu.Unlock()
    d.interp.ResetRunState()
    d.drainEvents()
    d.evalMetrics()
    d.checkSchedules()
    d.checkDeadlines()   // НОВАЯ 4-я фаза (B4b)
}
```

**Инвариант сохраняется:** фаза аддитивна в хвосте, использует ту же машинерию `safeFire`/`fireBody`,
держит `d.mu`, не трогает порядок и идемпотентность первых трёх. `ResetRunState` уже сбросил кеши.
Верифицировать живым daemon-тестом (риск критиков #3, §AU-12).

#### §AU-6.2.2 `checkDeadlines` — скан без нового Store-метода (D-AU-5)

```
now := d.clock.Now()
deadlineTriggers := фильтр d.interp.Triggers() по td.Spec.(*ast.DeadlineTrigger)
если deadlineTriggers пуст { return }                          // нет работы
tasks, err := d.st.ListPendingTasks("")                        // ОДИН листинг до циклов (СУЩЕСТВУЮЩИЙ метод)
если err != nil { d.logf("checkDeadlines: листинг задач: %s", err); return }   // как safeFire-изоляция
for каждой t из tasks:                                          // внешний цикл по задачам
    если t.Escalated { continue }                              // durable-фильтр (D-AU-5)
    если !engine.Overdue(t, now) { continue }                  // СУЩЕСТВУЮЩИЙ хелпер просрочки, format.go:35
    inst, lerr := d.st.LoadInstance(t.InstanceID); если lerr != nil { continue }
    for каждого td из deadlineTriggers:
        spec := td.Spec.(*ast.DeadlineTrigger)
        если t.StepName != spec.Step.Name || inst.ProcessName != spec.Process.Name { continue }
        d.safeFire(func() error { return d.fireDeadlineBody(td.Body, inst.Variables) })  // инжект §AU-6.2.3
        t.Escalated = true; d.st.SaveTask(t)                   // durable-персист, СУЩЕСТВУЮЩИЙ SaveTask (upsert)
        break                                                  // одна эскалация на задачу
```

**Решения псевдокода (закрыты, не гадать):** (1) `ListPendingTasks` — ОДИН раз до циклов (не на триггер);
возвращает копии (`memory.go:86` copyTask / SQLite свежие структуры), повторный `SaveTask` внутри срез
не инвалидирует. (2) Просрочка — через существующий `engine.Overdue(t, now)` (`format.go:35`,
`t.Deadline != nil && now.After(*t.Deadline)`), единый источник с колонкой «просрочена» в `UserTasks`/`inspect`
(нет off-by-one на `now==Deadline`). (3) Ошибка листинга — лог + выход из фазы (изоляция как у первых трёх).
(4) Инжект — напрямую из уже загруженного `inst.Variables` (`map[string]value.Value`), без round-trip
`InstanceVariables` (§AU-6.2.3). `SaveTask` после успешного fire — at-least-once допустим до M3 (повтор тела
до пометки крайне маловероятен и идемпотентен по цели).

#### §AU-6.2.3 Инжект `InstanceVariables` в тело (D-AU-6)

В отличие от метрика/событие/расписание (один инжект `injection{name, val}`, `fire.go:10/22`), эскалация
инжектит ВСЕ переменные инстанса. `fireDeadlineBody` — **НОВАЯ функция рядом с `fireBody` (`fire.go:22`)**;
struct `injection` (`fire.go:10`) НЕ трогается и НЕ расширяется — мульти-инжект идёт прямым циклом `Define`
поверх `NewTriggerBodyEnv`, минуя `injection`-конверт:

```go
func (d *Daemon) fireDeadlineBody(body *ast.Block, vars map[string]value.Value) error {
    env := d.interp.NewTriggerBodyEnv()          // NewEnvironment(global) + markBoundary, trigger_daemon.go:60
    for k, v := range vars {
        env.Define(k, v)                         // факт, текущая_выручка, … как локали барьерного env
    }
    return d.interp.EvalBlockInTrigger(env, body)
}
```

`vars` = `inst.Variables` (уже загружено в `checkDeadlines`, `map[string]value.Value`) — без round-trip
`InstanceVariables`. `env.Define` — `environment.go:33`. Read-only барьер: тело
может читать/использовать инжектированные имена, вызывать `уведомить`/`вызвать`/`запустить процесс`,
но не перепривязывать глобали (TR-BODY-RO).

### §AU-6.3 Рестарт × durable (обязательный инвариант DoD)

Демон рестартует → `RunRestartScan` (`restart.go:28`) реактивирует инстансы по
`ListInstancesByStatus([Running, Created])`. Эскалация-состояние НЕ в `TriggerState` (D-AU-5) — оно в
`Task.Escalated`, персистнутом в SQLite. После рестарта `checkDeadlines` читает `ListPendingTasks` из той
же `--db`, видит `t.Escalated == true` → `continue` → **повтор эскалации невозможен**. Это и есть
durable-гарантия «переживает рестарт без дублирования» (хартия §2, риск критиков #1).

Граничные случаи:
- Задача завершена `complete` ДО просрочки → не в `ListPendingTasks` → эскалации нет (корректно).
- Задача эскалирована, затем завершена → `Escalated` уже стоит; завершение штатно.
- `--db` отсутствует (MemoryStore) → durable-гарантия НЕ держится (эфемерно). Демо ОБЯЗАНО идти с `--db` (§AU-9).
- Реактивация залипшего `выполняется`-инстанса повторно входит в `advance`, который снова доходит до
  человеческого шага. **Дубль открытой задачи здесь исключён** идемпотентным created-задачи гардом:
  `advance` перед минтом проверяет уже-открытую задачу на `(inst.ID, шаг.Name)` через `ListTasksByInstance`
  и переиспользует её (фикс A фичи 024; механика и обоснование — engine-model §EN-3, согласовано с M3
  exactly-once reliability-model §C-2b.3).

### §AU-6.4 Часы эскалации (детерминизм golden)

Скан использует `d.clock.Now()` (`engine.Clock`, инжектируемый в демон, `daemon.go:25`). Тот же
`engine.Clock` инжектится в движок и интерпретатор (один источник, как `serve.go:191`, M1-гейт
подтвердил отсутствие «двойных часов» в serve). Golden управляет просрочкой, продвигая поддельный
`Clock` за `Deadline` (§AU-12 golden Б). Реальные часы (`SystemClock`) — в проде; монотонность → M3-C4.

---

## §AU-7. B5 — `ladix start <процесс> [аргументы]` (CLI)

### §AU-7.1 Подкоманда

`main.go:43-60` switch получает `case "start": return startMain(args[1:], stdout, stderr)`.
`startMain`: парсит `<процесс>` (первый позиционный), `[аргументы…]` (остальные позиционные),
`--db путь` (D-AU-10), `--webhook URL`, `--max-depth N`. Конструирует Store (SQLite при `--db`),
интерпретатор, движок; вызывает `engine.Start(процесс, args)`; печатает результат старта (id инстанса
+ строки движка как при `run`). Точная строка стартового вывода — реестр §AU-10.

### §AU-7.2 Парсер типизированных литералов argv (D-AU-7)

CLI-парсер argv-строки → `value.Value` (новый, т.к. в репо его нет — gap разведки). Грамматика
одного аргумента (без выражений, только литералы):
- `^-?\d+$` → `value.Целое{V}` (диапазон Int64; вне → CLI-ошибка `SE-INT-RANGE`-аналог exit 2);
- `^-?\d+\.\d+$` (или с `eE`) → `value.Дробное{V}`;
- `истина` / `ложь` → `value.Булево` (BOOL-литералы лексера; маппинг — `scan_ident.go:26-31`);
- `пусто` → `value.None`;
- `^\d{4}-\d{2}-\d{2}$` (ISO) → `value.Дата{Year,Month,Day}` (парс как M1 Дата-поле);
- иначе → `value.Строка{V}` (без кавычек — весь argv-токен как строка).

Числа: `_`-разделители argv НЕ обязательны (CLI). Запись/Список/Длительность/Период в argv НЕ
поддерживаются (путь отступления — JSON-arg в M3). Кавычки argv снимает шелл; `"2 500 000"` придёт
одним токеном → Строка (пользователь даёт `2500000` для числа).

### §AU-7.3 Сверка арности на CLI (D-AU-7)

До `engine.Start`: `pd, ok := eval.Process(name)` (`exports.go:18`, возвращает ПАРУ `(*ast.ProcessDecl, bool)`).
`!ok` → процесс не объявлен → `ladix: процесс '<имя>' не объявлен` exit 2 (CLI формирует САМ; НЕ полагаться
на `engine.Start` — он даёт ДРУГОЙ текст `процесс '%s' не найден в определении`, `engine.go:59`, защитно).
`ok` → сравнить `len(pd.Params)` (`ast/process.go:10`) с `len(args)`; несовпадение →
`ladix: процесс '<имя>' ожидает <N> аргументов, получено <M>` exit 2. Эти ошибки — CLI-уровня
(§EN-8.B, `ladix: …`, exit 2), НЕ программные диагностики.

---

## §AU-8. B6 — `ladix inspect <id>` (CLI + 1 метод Store)

### §AU-8.1 Подкоманда

`main.go` switch: `case "inspect": return inspectMain(args[1:], stdout, stderr)`. `inspectMain`:
`<id>` (позиционный) + `--db путь` (обязателен по сути — без него инстанса нет). Читает Store НАПРЯМУЮ
(как `tasksMain`), без движка/eval:
- `st.LoadInstance(id)` → `СтатусИнстанса`, `ТекущийШаг`, `Variables`. Нет → `ladix: инстанс '<id>' не найден` exit 2.
- `st.ListTasksByInstance(id)` (новый, §AU-2) → история: открытые + завершённые задачи (ID, шаг,
  исполнитель, срок, статус, эскалирована-ли).

### §AU-8.2 Снимок + история (D-AU-8)

Печать человекочитаемого снимка: имя процесса, статус, текущий шаг, переменные (имя=значение через
`value.String`), затем список задач инстанса в порядке ID. Точный формат строк — реестр §AU-10
(exact-match golden). `--json` и `explain` (почему сработал триггер / trace метрики) — ОТЛОЖЕНЫ в M3-C5
(D-AU-8); B6 даёт минимальную наблюдаемость снимком.

---

## §AU-9. Серверная модель и единая `--db` (риск критиков #2)

M2 — на варианте (а) развилки §8: CLI-над-общим-SQLite, без сетевого демона. Все команды, участвующие
в сквозном демо, работают над ОДНОЙ `--db`-файлом:

**ВАЖНО — два разных дефолта Store в текущем коде (не выдумывать единый):**

| Команда | дефолт Store без явного `--db` | др. флаги M2 | Примечание |
|---|---|---|---|
| `run` | `dbPath:=""` → **MemoryStore** (`main.go:191`) | `--webhook`, `--max-depth` | §EN-6 |
| `serve` | `dbPath:=""` → **MemoryStore** (`serve.go:152`) | `--webhook`, `--interval`, `--max-depth`, `--listen`, `--token` (трек B) | демон, 4 фазы (+ опц. HTTP-приём) |
| `complete` | `defaultDBPath="ladix.db"` → **SQLite** (`main.go:283,348`) | `--data` (B3), `--webhook` | НЕ Memory |
| `tasks` | `"ladix.db"` → **SQLite** (`main.go:414,451`) | — | НЕ Memory |
| `emit` | `"ladix.db"` → **SQLite** (`emit.go:19,59`) | — | НЕ Memory |
| **`start`** (B5) | **= как complete-семья: дефолт `ladix.db` → SQLite** | `--webhook`, `--max-depth` | новый; см. контракт ↓ |
| **`inspect`** (B6) | **дефолт `ladix.db` → SQLite** (без БД нечего смотреть) | — | новый |
| `metric` | всегда **MemoryStore** (`main.go:261`) | — | без изменений |

**Дефолт-контракт новых команд (D-AU-10, закрыт):** `start`/`inspect` следуют семье `complete`/`tasks`/`emit`
(дефолт `defaultDBPath="ladix.db"`, `main.go:105`), а НЕ семье `run`/`serve` (пусто→Memory). Если бы `start`
дефолтил в Memory (как `run`), пользователь без `--db` получил бы рассинхрон с `complete`/`inspect`.

**⚠ Уточнение демо (поправка гейта M2):** `serve` — единственная команда цепочки, которая по таблице (и по
коду, `serve.go:51`) дефолтит в **Memory**, как `run` (демон — транзиентный по умолчанию). Поэтому демо НЕ
работает «без явного `--db` для всех команд»: персистентные `start`/`complete`/`inspect` укажут на `ladix.db`,
а `serve` без `--db` уйдёт в эфемерный Memory и не увидит инстанс. **Демо обязано запускать `serve --db ladix.db`
явно** (остальные команды могут опираться на дефолт `ladix.db`). Альтернатива «дефолтить `serve` в `ladix.db`» —
изменение поведения демона, вне скоупа M2 (кандидат на M3-серверную-модель); код M2 следует таблице.

**Предусловие демо:** `start` создаёт инстанс в БД; `serve --db ladix.db` (та же БД, явный флаг) эскалирует;
`complete` (та же БД) закрывает; `inspect` (та же БД) показывает. Конструкция Store сейчас инлайн-ДУБЛИРУЕТСЯ в 5 местах
(`main.go:182-193`/`serve.go:156-168`/`main.go:348-353`/`main.go:451-456`/`emit.go:59-64`) — выделить
**НОВЫЙ хелпер** `openStore(dbPath)` (`dbPath != "" → SQLiteStore` defer Close, иначе MemoryStore — узкий
снимок логики `runFile` `main.go:182-193`, не вся функция) и переиспользовать в `start`/`inspect`.

---

## §AU-10. Реестр диагностик и stdout M2 (exact-match)

> Принцип VIII (M-DX §MDX-6): тексты — дословно из этого якоря; архитектор синкает SPEC §13.4 ссылкой.
> Все новые сообщения — exact-match golden. Счётные замки (`errors_golden_test.go:205` len(seen)=28;
> `parser/inventory_test.go:38` wantCodes=14; `lexer/lexerrors_test.go:29` множество L-1..L-11=11)
> трогаются ТОЛЬКО где явно сказано ниже.

### §AU-10.A Синтаксические (parser, SE-*)

- **SE-TRIGGER-KIND — текст РАСШИРЯЕТСЯ**: `msgTriggerKind` (`parser/errors.go:29`) меняется с
  `"метрика, событие или расписание"` на `"метрика, событие, расписание или задача"` (задача — теперь
  валидное продолжение). Тот же код SE-TRIGGER-KIND (счёт 14 НЕ меняется), но это РАСШИРЕНИЕ golden-строки
  ломает несколько замков — **co-обновить ВСЕ зеркала старого текста (иначе гейт `go test` НЕ зелёный):**
  - `parser/errors.go:29` (`const msgTriggerKind`) + строка-комментарий `:26` если цитирует текст;
  - `parser/inventory_test.go:34` (fragment-match SE-TRIGGER-KIND);
  - `parser/parse_decl_test.go:1549`, `:1622`, `:1666` — **ТРИ exact-match golden**:
    `TestTriggerSyntaxDiagnostics` (:1549), `TestTriggerNegativesExactPos` (:1622), полная двухстрочная
    `TestGoldenTriggerSyntaxDiagnostics` (:1666);
  - `docs/trigger-model.md:432`, `:433`, `:1070`, `:1087` (доковые зеркала — моя зона на синке);
  - любые зеркала в SPEC §13 / `diagnostics-model.md`.
  Это **единственный сдвиг golden-строки фронтенда в M2**, но он многоточечный — занижать объём нельзя.
- **Малформенный эскалация-триггер** (`когда задача X…`, `когда задача просрочена Y…`, пропуск `.`/шага)
  → реиспользуют `SE-EXPECTED` через `msgExpected("просрочена"/"в"/"имя процесса"/"."/"имя шага", got)`
  → `ожидалось '…', получено '<лексема>'`. **Новых SE-кодов НЕТ**, счёт 14 не меняется.

### §AU-10.B Семантические (analyze, СемантическаяОшибка — НЕ 28-счёт eval)

Триггерные семантические диагностики живут в семействе `analyze_trigger_test.go` (как §TR-7), НЕ в
`errors_golden_test.go` (len=28 для RT/TY/core-SEM). Новые exact-match для `DeadlineTrigger`:
- процесс не объявлен → переиспользовать существующую формулировку запуска процесса (§PM-6.C/§TR-4.5)
  если совпадает; иначе exact-match `процесс '<имя>' не объявлен`.
- шаг не найден → `шаг '<шаг>' не найден в процессе '<процесс>'`.
- `значение`/`событие` в теле эскалации → наследуемый контекст-гард TR-VAL-CTX / TR-EVT-CTX (§TR-7.A,
  `inMetricTrigger=false`, `inEventTrigger=false`).

> Impl-чат подтверждает на specify: добавление этих кодов идёт в `analyze_trigger_test.go`-семейство и
> НЕ инкрементит `errors_golden_test.go` len(seen)=28. Если 007a-проверки оказались внутри 28-счёта —
> зафиксировать в spec и обновить число явно (флаг точности, §AU-12 верификация).

### §AU-10.C CLI-ошибки (`ladix: <текст>`, stderr, exit 2 — §EN-8.B)

- `ladix: неверный JSON в --data: <деталь>` (B3).
- `ladix: процесс '<имя>' ожидает <N> аргументов, получено <M>` (B5 арность).
- `ladix: процесс '<имя>' не объявлен` (B5).
- `ladix: не удалось разобрать аргумент '<argv>': целое вне диапазона типа Целое` (B5 литерал).
- `ladix: инстанс '<id>' не найден` (B6).
- `ladix: неверный URL вебхука '<URL>'` (B2 `--webhook`).

### §AU-10.D stdout-форматы (exact-match golden — КАНОН, impl лочит дословно)

- `вызвать`/`уведомить` дефолт-стаб — §AU-4.2 (байт-точно §EN-7).
- **Эскалация-триггер под `run` (заглушка):** `задача триггер '<процесс>.<шаг>' требует serve (фича 007b)`
  (§AU-6.1.4, дословно по `trigger_run.go:49,51`).
- **`ladix start` (человеко-первый шаг):** движок печатает ТОЛЬКО `[задача]`-строку через
  `printTaskCreated` (`engine.go:436-443`; `Start`→`advance`, НЕ `advanceAfterComplete` → статус-строки
  `engine.go:206` НЕТ); id инстанса печатает САМ `startMain`. Канон (golden маскирует `<время>` дедлайна):
  ```
  [задача] t-000001 → менеджер, шаг 'связаться_с_клиентом', срок до <время>
  запущен инстанс p-000001
  ```
  Если процесс терминальный без задач — движок тих, `startMain` печатает только `запущен инстанс <id>`.
- **`ladix inspect <id>`** — снимок + история (golden маскирует `<время>`). Канон:
  ```
  инстанс p-000001: процесс эскалация_плана, статус ожидает, шаг 'связаться_с_клиентом'
  переменные:
    факт = 2500000
  задачи:
    t-000001 шаг 'связаться_с_клиентом' → менеджер, срок до <время>, открыта, эскалирована
  ```
  Переменные — `имя = value.String(v)` в порядке `inst.Variables`; задачи — в порядке `ListTasksByInstance`
  (ID ASC); пометка `, эскалирована` — только если `t.Escalated`; пометка статуса — `открыта`/`завершена`.

---

## §AU-11. Витрина, README, quickstart (риск критиков #5)

### §AU-11.1 Примеры M2

Добавить демонстрационные `.ladix` (синтаксис v2, исполнимы):
- **`examples/контроль_плана.ladix`** — срез золотого сценария §2: источник CSV + окно-метрика (M1) +
  `когда метрика … < план: запустить процесс эскалация_плана(значение)` + процесс с человеческим шагом
  и `срок:` + `когда задача просрочена в эскалация_плана.связаться_с_клиентом: уведомить руководитель(факт)`.
- Опц. **`examples/вызов_результата.ladix`** — B1: `присвоить ответ = вызвать crm(номер)` + использование.
- **Негативный** `examples/ошибка_эскалация.ladix` (или допись `examples/ошибочная.ladix`) — малформенный
  эскалация-триггер → демонстрирует SE-EXPECTED/SE-TRIGGER-KIND.

**Замок `internal/parser/examples_test.go` `TestExamplesParseCleanSet`** (clean[] = текущий набор, на момент
M2 — 22 файла, `:12-35`): новые ЧИСТО-парсящиеся примеры ОБЯЗАНЫ быть добавлены в `clean[]` (иначе тест
падает на необъявленном файле — НЕ молчит). Негативные примеры идут в golden-замки `cmd/ladix/golden_test.go`,
не в clean[]. `examples/MANIFEST.md` — зарегистрировать новые примеры с описанием и привязкой golden.

### §AU-11.2 README

Убрать устаревшие ограничения (co-land с соответствующей подфичей):
- **`README:302-306`** «`ladix start` … в v1 отсутствует» — переписать под B5 (команда есть).
- **`README:84-89`** «Что отложено (v2): `ladix start`, передача payload через `complete`» — снять оба
  пункта (реализованы B5/B3). Раздел «Что работает» дополнить эскалацией дедлайна и реальными
  интеграциями (с пометкой «по умолчанию печать-стаб; HTTP — `--webhook`»).

### §AU-11.3 quickstart

`docs/quickstart.md` обрывается на шаге 4.4 (`complete` второго шага). Дополнить:
- 4.5 `ladix start <процесс> <арг>` (ручной запуск, B5);
- 4.6 `ladix inspect <id>` (снимок + история, B6);
- 4.7 эскалация под `serve` (задача просрочена → уведомление руководителю) с упоминанием durable-рестарта;
- 4.8 `complete … --data '{…}'` (передача результата, B3).

---

## §AU-12. Приёмочная таблица и golden (срез золотого сценария + усиление B4)

### §AU-12.A Серверная-edge demo метрики (риск критиков #4 — НЕ молчащее демо)

Метрика §2 «всегда ниже порога» под `serve` НЕ даёт edge-пересечения
(`fired := ts.LastBool != nil && !*ts.LastBool && cur`, **`metrics.go:72`** — с nil-гардом, не воспроизводить
упрощённую форму без `ts.LastBool != nil`) → молчит. Демо/golden ОБЯЗАНО внести пересечение порога во время демона:
- вариант (i): источник стартует ВЫШЕ плана, затем данные меняются (перезапись `orders.csv` или
  отдельная фикстура) так, что окно-метрика падает НИЖЕ порога → на следующем тике
  `LastBool: false→true` → edge fire → `запустить процесс`;
- вариант (ii) для golden эскалации: обойти метрику и создать инстанс прямым `ladix start` (B5) — чище
  и детерминированнее (см. §AU-12.B).

Golden фиксирует: до пересечения — тишина; после — ровно одно срабатывание (re-arm не повторяет).

### §AU-12.B Durable-эскалация × рестарт (ОБЯЗАТЕЛЬНЫЙ golden, ядро B4)

Детерминированный сценарий на инжектируемом `engine.Clock` и `--db`:

```
1. ladix start эскалация_плана 2500000 --db demo.db
   → инстанс p-000001, человеческий шаг связаться_с_клиентом, задача t-000001 со сроком (created + 2дн)
2. serve --db demo.db (Clock = created):  тики идут, checkDeadlines: t-000001 НЕ просрочена → тишина
3. Clock += 3дн (за срок):  следующий тик checkDeadlines:
   → t-000001 просрочена, !Escalated, шаг совпал, процесс совпал
   → fireDeadlineBody: "[уведомление] руководитель: 2500000" (или через факт) — РОВНО ОДИН раз
   → t-000001.Escalated = true, SaveTask
4. стоп демона (ctx cancel)
5. ПЕРЕЗАПУСК: serve --db demo.db (та же БД):
   → RunRestartScan реактивирует p-000001
   → checkDeadlines: t-000001.Escalated == true → continue → НЕТ повтора
6. assert: уведомление руководителю напечатано РОВНО ОДИН раз за оба прогона
```

Замки: (а) единичность эскалации; (б) `Escalated` персистнут в SQLite между прогонами; (в) 4-я фаза не
ломает первые три (живой daemon-тест, риск #3); (г) мутпроба — снять durable-фильтр `if t.Escalated`
→ golden ДОЛЖЕН покраснеть (двойная эскалация после рестарта).

### §AU-12.C Срез золотого сценария end-to-end (DoD M2)

`контроль_плана.ladix` под `serve --db` + реальный CSV + edge метрики (§AU-12.A) → процесс стартует →
человеческая задача со сроком → просрочка → эскалация → `complete … --data` → реальный эффект
(`--webhook` через `httptest`, B2) → `inspect` показывает историю. Переживает рестарт (§AU-12.B). Это
терминальный гейт-критерий M2 (хартия §2/§7.1).

### §AU-12.D Точечные приёмки слоёв

| Слой | Проверка |
|---|---|
| B1 парсер | `присвоить r = вызвать crm(x)` → `AssignAction{Value: *CallExternalExpr}` (НЕ AssignStmt); `вызвать crm(x)` строкой → `CallAction` |
| B1 eval | под стабом `r == Пусто` + печать `[вызов] crm(x)`; под httptest `r` = декодир. ответ |
| B2 | дефолт = §EN-7 golden байт-точно; `--webhook` → POST `{"цель","данные"}` принят httptest |
| B3 | `--data '{"итог":"перезвонит"}'` → след. шаг читает `данные.итог == "перезвонит"`; без флага → Пусто |
| B4a | `когда задача просрочена в P.S:` → `DeadlineTrigger{P,S}`; неизв. процесс/шаг → СемантическаяОшибка |
| B4b | §AU-12.B golden |
| B5 | арность mismatch → exit 2; `2500000` → Целое, `истина` → Булево, `2026-01-01` → Дата |
| B6 | `inspect p-000001` → снимок + ≥1 задача в истории; неизв. id → exit 2 |
| Регресс | весь v1/M1/M-DX golden зелёный; §EN-7 цел; событие-кодек после переноса (§AU-5.2) байт-точен |

---

## §AU-13. Что M2 осознанно НЕ делает (границы и путь отступления)

| Не делаем | Путь отступления |
|---|---|
| Версионирование определений, стабильные ключи триггеров | M3-C1; durable-ключ эскалации — per-task `Escalated`, миграция на хеш не нужна |
| Exactly-once доставки эффектов / outbox | M3-C2; до M3 at-least-once (эскалация идемпотентна по `Escalated`, эффекты-стабы повторяемы) |
| Миграции схемы Store (ALTER) | M2-эра: сброс БД (D-AU-9, **ОТОЗВАНО в M3**); M3-C2a: forward-only ALTER (`PRAGMA user_version`, `reliability-model.md` §C-2a) |
| Двойные часы (`eval.Clock` vs `engine.Clock`) | M3-C4 `WithClock`; M2 — один инжект. Clock (§AU-6.4) |
| Сетевой демон / `--json` | M3-C3 (развилка); M2 — CLI-над-SQLite |
| HTTP-приём событий (`serve --listen`) | Реализовано после v2 (трек B, `docs/inbound-events-model.md`); M2 — только `ladix emit` |
| Персист payload `данные` в переменные инстанса | M3; M2 — эфемерно сквозь догон (D-AU-3), шаг сам `присвоить`-ит при нужде |
| Запись/Список/Длительность/Период в argv `start` | M3 JSON-arg; M2 — скалярные литералы (§AU-7.2) |
| `explain`/trace метрики/аудит-журнал в `inspect` | M3-C5; M2 — снимок + история задач |
| Реестр ролей / тип `Роль` | backlog; `Notify(target string)` строкой достаточно сценарию |
| Параллельные шаги | вне v2 (хартия §3) |
| Брокеры/очереди/ретраи внешних вызовов | backlog; M2 — синхронный HTTP-POST best-effort |

---

> **Каскад синков на M2-гейте (моя зона — архитектор):** SPEC §11/§12/§13 (строки kickoff
> 558/560/564/589/620/642/291), `grammar.md` (DeadlineTrigger, CallExternalExpr, контекст-разбор задача/просрочена),
> `README.md` (start/payload сняты, эскалация добавлена), `engine-model.md §EN-4/§EN-7/§EN-8`
> (ProcessRuntime 7→8, драйвер-Option), `execution-model.md` (4-я фаза tick, Task.Escalated),
> `examples/MANIFEST.md`. Дрейф-аудит против инвариантов §5 (хартия) + срез золотого сценария (§AU-12.C).
