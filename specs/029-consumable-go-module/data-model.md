# Data Model: Потребляемый Go-модуль LADIX (029)

Фича **не меняет** язык — она упаковывает фронтенд в публичную поверхность и вводит
**новый пакет `ir`**: стабильный версионируемый контракт промежуточного представления,
в который `ladix.Compile` понижает `ast.Program`. Этот документ описывает сущности
пакета `ir` (Go-типы с JSON-тегами `snake_case`), их поля, происхождение из AST и
инварианты.

`ir` — **листовой** пакет: он зависит только от стандартной библиотеки (`encoding/json`
неявно — теги). Он НЕ импортирует `ast`/`errors`/`value` в своей публичной форме
(дублирует `Position` ради листовости, см. ниже). Понижение `ast.Program → ir.Program`
живёт в фасадном пакете `ladix`, а не в `ir`.

## Назначение пакета `ir`

`ir` — это **граница между версиями LADIX и потребителем** (в частности платформой
«Уклад», вариант B: исполняет определения нативно, движок LADIX не переносит). Потребитель
опирается на `ir`, а не на `ast`, потому что:

- `ir` сериализуем (JSON, `snake_case`) и стабилен по номеру схемы `schema_version`;
- `ir` несёт только **декларативные определения** (метрики/процессы/триггеры), нужные
  для нативного исполнения, без внутреннего устройства парсера;
- выражения в `ir` v1 — **канонические строки** (детерминированная сериализация, образец
  `ast.CanonicalTriggerCondition`/`canonExpr`), а не структурные деревья → форма стабильна
  и не зависит от внутренней эволюции AST.

## Сущности

| Сущность | Назначение | JSON-корень |
|---|---|---|
| `SchemaVersion` (const) | версия формы IR | — (`schema_version` в `Program`) |
| `Program` | корень IR, контейнер определений | объект |
| `Metric` | нормализованная метрика | элемент `metrics` |
| `Process` | нормализованный процесс | элемент `processes` |
| `Step` | шаг процесса | элемент `steps` |
| `Trigger` | нормализованный триггер | элемент `triggers` |
| `Diagnostic` | одна проблема компиляции | (возврат фасада, не часть `Program`) |
| `Position` | позиция в исходнике | `pos` / вложенный объект |

### 1. `ir.SchemaVersion`

Константа версии формы IR.

```go
// SchemaVersion — версия формы (схемы) IR. Bump при breaking-изменении формата IR
// (см. «Политика версионирования IR»). v1 — канонические строки выражений.
const SchemaVersion = 1
```

- **Назначение**: единственный источник правды о форме IR, который потребитель пинит/проверяет.
- **Инвариант**: при компиляции текущей версией библиотеки
  `Program.SchemaVersion == ir.SchemaVersion`.

### 2. `ir.Program`

Корень IR.

```go
// Program — корень IR: декларативные определения, понижённые из ast.Program.
type Program struct {
	SchemaVersion int       `json:"schema_version"`
	Metrics       []Metric  `json:"metrics"`
	Processes     []Process `json:"processes"`
	Triggers      []Trigger `json:"triggers"`
}
```

- **Происхождение**: `ast.Program.Items` (union `TopLevelItem`) раскладывается по типам
  деклараций через type-switch: `*ast.MetricDecl → Metric`, `*ast.ProcessDecl → Process`,
  `*ast.TriggerDecl → Trigger`. Порядок внутри каждой коллекции сохраняется как в исходнике.
- **Осознанное ограничение v1**: `*ast.FunctionDecl`, `*ast.SourceDecl` и операторы
  верхнего уровня **НЕ попадают** в IR v1. Потребителю (Уклад) нужны декларативные
  определения метрик/процессов/триггеров для нативного исполнения; функции —
  внутренняя императивная деталь, источники резолвятся file-relative на стороне исполнителя.
  Расширение состава IR новыми коллекциями — аддитивно (не breaking), см. политику версий.
- **Инвариант**: `ladix.Compile` возвращает `program == nil` при наличии хотя бы одной
  диагностики `severity == "error"`; ненулевая `*Program` ⟺ ошибок уровня error нет.
- **Инвариант**: `SchemaVersion` всегда равен `ir.SchemaVersion` на момент компиляции.

### 3. `ir.Metric`

Понижение `ast.MetricDecl`.

```go
// Metric — нормализованное определение метрики. Выражения — канонические строки.
type Metric struct {
	Name      string   `json:"name"`
	Source    string   `json:"source"`
	Where     string   `json:"where,omitempty"`
	Aggregate string   `json:"aggregate"`
	Period    string   `json:"period,omitempty"`
	ByDate    string   `json:"by_date,omitempty"`
	Pos       Position `json:"pos"`
}
```

Маппинг из `ast.MetricDecl`:

| Поле IR | Источник AST | Тип AST | Преобразование |
|---|---|---|---|
| `Name` | `MetricDecl.Name` | `Ident` | `.Name` |
| `Source` | `MetricDecl.Source` | `Ident` | `.Name` |
| `Where` | `MetricDecl.Where` | `Expression` | канон-строка (`""` если nil) |
| `Aggregate` | `MetricDecl.Aggregate` | `Expression` | канон-строка (обязателен, не nil) |
| `Period` | `MetricDecl.Period` | `Expression` | канон-строка (`""` если nil) |
| `ByDate` | `MetricDecl.ByDate` | `Expression` | канон-строка (`""` если nil) |
| `Pos` | `MetricDecl.Pos()` | `ast.Position` | копия `{Line,Col}` |

- **Канонизация**: `Where`/`Aggregate`/`Period`/`ByDate` — КАНОНИЧЕСКИЕ СТРОКИ (по образцу
  `canonExpr` из `ast/canon.go`), а НЕ структурные деревья. `omitempty` на опциональных
  атрибутах: отсутствующий атрибут (nil-`Expression`) → поле опускается в JSON.
- **Инвариант**: `Aggregate != ""` (обязательный атрибут после успешного парса);
  `Name`/`Source` непусты.

### 4. `ir.Process`

Понижение `ast.ProcessDecl`.

```go
// Process — нормализованное определение процесса.
type Process struct {
	Name   string   `json:"name"`
	Params []string `json:"params"`
	Steps  []Step   `json:"steps"`
	Pos    Position `json:"pos"`
}
```

Маппинг из `ast.ProcessDecl`:

| Поле IR | Источник AST | Тип AST | Преобразование |
|---|---|---|---|
| `Name` | `ProcessDecl.Name` | `Ident` | `.Name` |
| `Params` | `ProcessDecl.Params` | `[]Ident` | срез `.Name` (порядок сохранён) |
| `Steps` | `ProcessDecl.Steps` | `[]*StepDecl` | поэлементное понижение в `Step` |
| `Pos` | `ProcessDecl.Pos()` | `ast.Position` | копия `{Line,Col}` |

- **Инвариант**: `len(Steps) >= 1` (грамматика `ProcessBlock ::= StepDecl+`); порядок
  шагов и параметров сохраняется как в исходнике.

### 5. `ir.Step`

Понижение `ast.StepDecl` (не top-level, вложен в `Process.Steps`).

```go
// Step — шаг процесса. Actions — канонические строки действий шага.
type Step struct {
	Name     string   `json:"name"`
	After    []string `json:"after"`
	Assignee string   `json:"assignee,omitempty"`
	Deadline string   `json:"deadline,omitempty"`
	Actions  []string `json:"actions"`
	Pos      Position `json:"pos"`
}
```

Маппинг из `ast.StepDecl`:

| Поле IR | Источник AST | Тип AST | Преобразование |
|---|---|---|---|
| `Name` | `StepDecl.Name` | `Ident` | `.Name` |
| `After` | `StepDecl.After` | `[]Ident` | срез `.Name` (предшественники/переходы; `nil`/пусто, если нет `после`) |
| `Assignee` | `StepDecl.Assignee` | `Expression` | канон-строка (`""` если nil) |
| `Deadline` | `StepDecl.Deadline` | `Expression` | канон-строка (`""` если nil) |
| `Actions` | `StepDecl.Body` | `[]Statement` | канон-строки действий |
| `Pos` | `StepDecl.Pos()` | `ast.Position` | копия `{Line,Col}` |

- **`Actions`**: каждый оператор тела (`*ast.AssignAction` / `*ast.CallAction` /
  `*ast.NotifyAction` из `ast/step.go`) канонизируется в строку. Канон действия строится по
  той же идее, что `canonExpr` (детерминированно, без зависимости от форматирования):
  - присвоить: `присвоить|<Name>|<canon(Value)>`
  - вызвать: `вызвать|<Name>|<canon(Args...)>`
  - уведомить: `уведомить|<Name>|<canon(Args...)>`
  (точные префиксы фиксируются на этапе реализации; важна детерминированность и стабильность —
  смена формата = breaking, bump `SchemaVersion`).
- **Инвариант**: `Assignee`/`Deadline` опускаются (`omitempty`), если атрибут отсутствует.

### 6. `ir.Trigger`

Понижение `ast.TriggerDecl` (union `TriggerSpec`). Тело триггера (`TriggerDecl.Body`) —
императивное ядро — в IR v1 НЕ попадает (как и тела функций); IR несёт только **условие**
срабатывания, нужное потребителю для маршрутизации.

```go
// Trigger — нормализованный триггер. Поля заполняются по Kind (остальные опускаются).
type Trigger struct {
	Kind      string   `json:"kind"`                // metric|event|schedule|deadline
	Metric    string   `json:"metric,omitempty"`    // Kind==metric
	Op        string   `json:"op,omitempty"`        // Kind==metric
	Threshold string   `json:"threshold,omitempty"` // Kind==metric (канон-строка)
	Event     string   `json:"event,omitempty"`     // Kind==event
	Schedule  string   `json:"schedule,omitempty"`  // Kind==schedule (канон-строка расписания)
	Process   string   `json:"process,omitempty"`   // Kind==deadline
	Step      string   `json:"step,omitempty"`      // Kind==deadline
	Pos       Position `json:"pos"`
}
```

Маппинг из `ast.TriggerDecl.Spec` (type-switch, дискриминант → `Kind`):

| `Kind` | Тип AST (`TriggerSpec`) | Заполняемые поля | Источник |
|---|---|---|---|
| `"metric"` | `*MetricTrigger` | `Metric`, `Op`, `Threshold` | `Metric.Name`, `Op.String()`, `canonExpr(Threshold)` |
| `"event"` | `*EventTrigger` | `Event` | `Event.Name` |
| `"schedule"` | `*ScheduleTrigger` | `Schedule` | канон `ScheduleSpec` (см. ниже) |
| `"deadline"` | `*DeadlineTrigger` | `Process`, `Step` | `Process.Name`, `Step.Name` |

- **Канон расписания** (`Schedule`): по образцу `CanonicalTriggerCondition`:
  - `*EverySchedule` → `every|<canonDuration(Every)>` (= `every|длит(<Amount>|<Unit>)`);
  - `*AtSchedule` → `at|<At.Value>`.
- **Канон условия метрики**: переиспользует идею `ast.CanonicalTriggerCondition` —
  `Op = Op.String()`, `Threshold = canonExpr(Threshold)`. (В IR компоненты разложены по
  отдельным полям, а не слиты в один префиксированный ключ, как durable-ключ триггера.)
- **Инвариант**: для каждого `Kind` заполнены только релевантные поля; прочие опущены
  (`omitempty`). `Kind ∈ {metric,event,schedule,deadline}`.

### 7. `ir.Diagnostic`

Одна проблема компиляции. Возвращается фасадом `ladix.Compile` отдельным срезом
(`[]Diagnostic`), не вложена в `Program`.

```go
// Diagnostic — одна проблема компиляции. Message — ДОСЛОВНЫЙ текст SPEC §13.
type Diagnostic struct {
	Severity string   `json:"severity"` // "error"
	Stage    string   `json:"stage"`    // lex|parse|semantic
	Message  string   `json:"message"`
	Pos      Position `json:"pos"`
}
```

- **Происхождение**:
  - `Stage == "lex"` / `"parse"` — из `errors.LexError`/`errors.ParseError` (через `ErrorList`,
    собираемый на этапах лексинга/парсинга);
  - `Stage == "semantic"` — из семантической ошибки `Analyze` (`СемантическаяОшибка`).
- **Severity**: в v1 единственное значение `"error"` (предупреждений фронтенд не выдаёт;
  расширение множества severity — аддитивно).
- **ИНВАРИАНТ (Принцип VIII — критичный)**: `Message` берётся **ДОСЛОВНО** из текста
  диагностики SPEC §13 — из существующего `.Error()` / сообщения соответствующего типа
  ошибки. Переформулировать, обрезать, локализовать ЗАПРЕЩЕНО: конвертация — это **только**
  перенос строки в DTO-поле, без изменения содержимого.
- **Pos**: позиция ошибки в исходнике (`ir.Position`), скопированная из `Position` типа ошибки.

### 8. `ir.Position`

Собственный локальный тип пакета `ir`.

```go
// Position — позиция в исходнике. 1-based; Col считается в рунах (Unicode code points).
type Position struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}
```

- **Дублирование намеренно**: `ir.Position` **не разделяется** с `ast.Position`/
  `errors.Position` — это собственный тип, чтобы `ir` оставался **листовым** (не импортировал
  `ast`/`errors`). Санкционировано конституцией 2.0.0, Принцип IV.
- **Семантика идентична** `errors.Position`: `Line >= 1`, `Col >= 1`; отсчёт 1-based;
  колонка в **рунах**, не в байтах (язык кириллический).
- **Понижение**: тривиальное копирование `ir.Position{Line: p.Line, Col: p.Col}` из
  `ast.Position`/`errors.Position` при компиляции.

## Маппинг AST → IR

Понижение выполняется в фасадном пакете `ladix` (не в листовом `ir`):

```
ast.Program.Items  (union TopLevelItem)
   ├─ *ast.MetricDecl   → ir.Metric    (в Program.Metrics)
   ├─ *ast.ProcessDecl  → ir.Process   (в Program.Processes)
   │     └─ []*ast.StepDecl → []ir.Step
   │            └─ []ast.Statement (действия) → []string (Actions, канон)
   ├─ *ast.TriggerDecl  → ir.Trigger   (в Program.Triggers; условие Spec)
   ├─ *ast.FunctionDecl → (опущен в v1)
   ├─ *ast.SourceDecl   → (опущен в v1)
   └─ операторы top-level → (опущены в v1)

errors.ErrorList (lex/parse) + СемантическаяОшибка (Analyze) → []ir.Diagnostic
ast.Position / errors.Position → ir.Position  (копия {Line,Col})
```

**Где происходит канонизация выражений.** Все `Expression`-поля (`Metric.Where/Aggregate/
Period/ByDate`, `Trigger.Threshold`, `Step.Assignee/Deadline`) и действия (`Step.Actions`) и
расписания (`Trigger.Schedule`) понижаются в **канонические строки**. Канонизатор — по образцу
`ast.canonExpr`/`ast.CanonicalTriggerCondition`/`ast.canonDuration`: тотальный рекурсивный
обход с детерминированной нормализацией чисел (`strconv.FormatInt`/`FormatFloat('g',-1,64)`),
кавычек строк (`strconv.Quote`), без зависимости от исходного форматирования и позиции.
Канон **стабилен**: эквивалентные по смыслу записи дают одинаковую строку.

**Что НЕ переносится в IR v1 (осознанные ограничения):**

- тела триггеров и функций (императивное ядро) — потребитель исполняет нативно;
- объявления функций (`FunctionDecl`) и источников (`SourceDecl`);
- операторы верхнего уровня;
- структурные деревья выражений (только канон-строки) — будущий bump `SchemaVersion`.

## Политика версионирования IR (`schema_version`)

`ir.SchemaVersion` — контракт формы IR через границу версий модуля.

**Breaking-изменение IR (ТРЕБУЕТ bump `SchemaVersion`):**

- удаление или переименование поля сущности IR;
- смена типа существующего поля;
- смена семантики существующего поля;
- **смена формата канонической строки** (выражений/действий/расписаний) — потребитель
  парсит/сравнивает эти строки, поэтому их формат — часть контракта;
- перевод выражений из канонических строк в структурное представление (планируемый будущий bump).

**НЕ breaking (аддитивно, `SchemaVersion` НЕ меняется):**

- добавление нового **опционального** поля (с `omitempty`), не меняющего чтение старых полей;
- добавление новой коллекции в `Program` (например, будущие `Functions`/`Sources`), если
  старые поля читаются без изменений;
- расширение допустимого множества значений (например, новый `Diagnostic.Severity`), если
  старые значения сохраняют смысл.

**Связь с semver библиотеки** (`vX.Y.Z`, первый тег `v0.1.0`):

- bump `ir.SchemaVersion` ⇒ **минимум MINOR** релиз библиотеки;
- если bump схемы ломает Go-типы публичной поверхности (удаление/переименование/смена типа
  экспортируемого поля) ⇒ **MAJOR** релиз;
- аддитивные изменения языка (новый синтаксис, не ломающий существующий) ⇒ MINOR, схему НЕ
  трогают.

## Инварианты

1. **Версия схемы.** При компиляции текущей версией библиотеки
   `Program.SchemaVersion == ir.SchemaVersion` (== `1` в v1).
2. **nil-программа при ошибках.** `ladix.Compile` возвращает `program == nil` ⟺ среди `diags`
   есть хотя бы одна `Diagnostic{Severity: "error"}`. Ненулевая `*Program` гарантирует
   отсутствие ошибок уровня error.
3. **Дословность диагностик (Принцип VIII).** `Diagnostic.Message` — дословный текст SPEC §13
   из существующего `.Error()`/сообщения типа ошибки; переформулирование запрещено, конвертация
   только в DTO-поле.
4. **Листовость `ir`.** Пакет `ir` зависит только от stdlib; имеет **собственный** тип
   `Position` (не импортирует `ast`/`errors`/`value`). Санкция — конституция 2.0.0, Принцип IV.
5. **Позиции.** Любая реальная `ir.Position`: `Line >= 1`, `Col >= 1`; 1-based, колонка в рунах.
6. **Канон детерминирован.** Канонические строки выражений/действий/расписаний не зависят от
   исходного форматирования и позиции; эквивалентные по смыслу записи → одинаковые строки.
7. **Состав v1.** В `Program` попадают только `Metric`/`Process`/`Trigger`; функции, источники,
   операторы и тела триггеров/функций опущены (осознанное ограничение v1).
