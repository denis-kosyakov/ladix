# Phase 1 — Data Model: Фронтенд процессов v1 — `процесс`/`шаг`/действия/`запустить процесс`

**Feature**: 005-process-frontend | **Date**: 2026-06-10 | **Plan**: [plan.md](./plan.md)

Описывает сущности фичи 005 на уровне **AST-контракта** (пакет `internal/ast`): два новых узла
`ProcessDecl`/`StepDecl` (оба — **плоские**, по образцу `MetricDecl` 004, D-1) и одну вспомогательную
структуру позиций `StepAttrPos` (по образцу `MetricAttrPos`). Узлы действий
(`AssignAction`/`CallAction`/`NotifyAction`, `ast/step.go`) и `RunProcessExpr` (`ast/expr.go`) **уже
построены** в 003/004 — 005 их **не вводит заново** и **не меняет форму** (D-2, D-10, FR-009),
меняется только их обработка в семпроходе (см. plan/research/contracts). Поля описаны **концептуально**
(контракт) плюс дословные Go-сигнатуры там, где их фиксирует якорь §PM-2; реализационные детали парсера
и семпрохода — в `contracts/` и `/speckit-tasks`. Публичные Go-имена английские, тексты сообщений
русские.

**Источник истины — `docs/process-model.md` §PM-0…§PM-8, D-1…D-13; при любом расхождении побеждает он
(§PM-2 для форм узлов).** Семантический фон — `SPEC.md §3/§7.3/§7.4/§11`, `docs/grammar.md §7/§9`,
`docs/eval-model.md §8.3/§9`. Эталон стиля/глубины — `specs/004-source-metric/data-model.md §1`
(`SourceDecl`/`MetricDecl`/`MetricAttrPos`).

> **Дрейф документации (зафиксировать в импл-чате).** Якорь §PM-2 ссылается на
> `ARCHITECTURE.md §4.4/§4.8` как на «уже синхронизированный» носитель форм узлов. Этот файл —
> `ARCHITECTURE.md` в **корне** репозитория (не `docs/ARCHITECTURE.md`): §4.4 (строки 178-191) и §4.8
> (строки 239-248) уже несут формы `ProcessDecl`/`StepDecl`/`StepAttrPos` согласованно с §PM-2
> (байт-в-байт: `ProcessDecl | Name Ident; Params []Ident (опц.); Steps []*StepDecl`;
> `StepDecl | Name Ident; After []Ident; Assignee Expression; Deadline Expression; Attrs StepAttrPos;
> Body []Statement`; `StepAttrPos | AssigneePos/DeadlinePos Position`) и явно фиксируют «**Узлов
> `StepLine`/`StepAttr` нет**» (строка 187, D-1) — синк §PM-0 п.6 **выполнен**. Архитектурный фон
> процессов/шагов также отражён в `docs/grammar.md §7` (EBNF; там `StepLine`/`StepAttr` —
> **грамматические продукции** парсера, а не AST-узлы) и `docs/STRUCTURE.md`, но они **дополняют**, а не
> заменяют `ARCHITECTURE.md`. Формы узлов канонизированы здесь по **фактическому коду 004**
> (`ast/decl.go`, плоский `MetricDecl`+`MetricAttrPos`) и тексту §PM-2; узлов `StepLine`/`StepAttr{Kind}`
> нет в плановом AST (D-1). При расхождении побеждает §PM-2.

---

## 1. AST-узлы (пакет `internal/ast`) — §PM-2

Конвенции 002/004 (см. `specs/002-parser-ast/data-model.md`, `internal/ast/decl.go`): встраиваемая база
даёт `Pos()` **и** (для top-level деклараций) маркеры `declNode()`/`topLevelItem()` через embedding —
**отдельные методы НЕ писать**. `Pos()` = ведущий ключевой токен. `Ident` хранится **по значению** (как
`FunctionDecl.Name Ident` / `MetricDecl.Source Ident`); опциональное подвыражение — поле типа
`Expression` со значением `nil` (как `MetricDecl.Where`); список идентификаторов — `[]Ident` (как
`FunctionDecl.Params`). Эталоны для копирования — `ast.FunctionDecl` (заголовок: `Name`/`Params`) и
`ast.MetricDecl`+`MetricAttrPos` (плоская форма + позиции атрибутов), оба в `internal/ast/decl.go`.

**Где разместить.** Новый файл `ast/process.go` либо дополнение `ast/decl.go`/`ast/step.go` — на
усмотрение импл-чата (§PM-2); формы фиксированы ниже. Стиль базы — как у `FunctionDecl`/`MetricDecl`:
`declBase{base{pos}}` для top-level, `base{pos}` для не-top-level.

### Узел top-level (реализует `Decl`)

| Узел | Поля | `Pos()` |
|---|---|---|
| `ProcessDecl` | `declBase`; `Name Ident`; `Params []Ident`; `Steps []*StepDecl` | токен `процесс` |

`ProcessDecl` встраивает `declBase` первым полем → автоматически реализует `declNode()`/`topLevelItem()`
(как `FunctionDecl`/`SourceDecl`/`MetricDecl`), пополняя union `Decl`:
**`FunctionDecl | SourceDecl | MetricDecl | ProcessDecl`** (FR-008; §PM-2). Заголовок зеркалит
`FunctionDecl` (`Name`+`Params`), тело — список **шагов** вместо `*Block`.

> **Синк-нота (зона якоря, §PM-0 п.6).** Doc-комментарий `ast/node.go:24` всё ещё гласит «В подмножестве
> B единственная — FunctionDecl» (устарел ещё на 004). Обновить на
> `union: FunctionDecl | SourceDecl | MetricDecl | ProcessDecl`. Текст `Decl`-интерфейса (`node.go:24-27`)
> и сами маркеры не меняются — только комментарий.

### Узел не-top-level (реализует **только** `Pos()`)

| Узел | Поля | `Pos()` |
|---|---|---|
| `StepDecl` | `base`; `Name Ident`; `After []Ident`; `Assignee Expression`; `Deadline Expression`; `Attrs StepAttrPos`; `Body []Statement` | токен `шаг` |

`StepDecl` — **НЕ top-level** (живёт только внутри `ProcessDecl.Steps`, FR-008; §PM-2): встраивает
`base` (не `declBase`/`stmtBase`), реализует **только** `Pos()`, **без** маркеров
`declNode()`/`topLevelItem()`/`stmtNode()`. Это значит: `StepDecl` **не входит** ни в `Decl`, ни в
`TopLevelItem`, ни в `Statement` — он достижим исключительно как элемент среза `ProcessDecl.Steps`.
Форма **плоская** (D-1): `Assignee`/`Deadline` — отдельные поля `Expression` (а не узлы-обёртки
`StepAttr{Kind, Value}`); `Body` — все императивные операторы тела. Узлы `StepLine`/`StepAttr{Kind}`
**не вводятся** (D-1; `StepLine`/`StepAttr` из grammar §7 — продукции парсера, парсер разводит их в
поля `Assignee`/`Deadline`/`Body` при разборе блока, см. §PM-3 и `contracts/parser.md`).

### Вспомогательная структура (не `Node`)

| Структура | Поля | Назначение |
|---|---|---|
| `StepAttrPos` | `AssigneePos Position`; `DeadlinePos Position` | позиция ключевого слова каждого **присутствующего** атрибута шага (нулевая `Position{}` для отсутствующего); для точной диагностики §PM-6.B `срок` без `исполнитель` (указывает на строку `срок:`) |

`StepAttrPos` — **вспомогательная структура, не `Node`** (без `Pos()`, без маркеров), точный аналог
`MetricAttrPos` (D-1/D-8). Несёт ровно две позиции — по числу атрибутов шага (`исполнитель`/`срок`).

---

## 2. Семантика полей

### `ProcessDecl`

- **`Name Ident`** — имя процесса, по значению (как `FunctionDecl.Name`). Регистрируется в **общем
  глобальном namespace** (делит с функциями/источниками/метриками, D-5/FR-010); резолв/коллизии —
  семпроход (Шаг 1, см. plan; §PM-4). `Name.Pos()` — позиция идентификатора (для диагностик повтора
  имени и резерва, §PM-6.B).
- **`Params []Ident`** — позиционные параметры, как `FunctionDecl.Params` (`[]Ident`). Скобки
  **опциональны** (grammar §7 `("(" ParamList? ")")?`): `процесс P:` без параметров → `Params == nil`
  или пустой срез. Дубль параметра (`процесс P(x, x):`) семпроходом **не проверяется** (D-13 —
  позиционное связывание, паритет с `FunctionDecl`, новую диагностику не вводим). Каждый `Ident` несёт
  свою `Pos()`. Параметры **засеваются** в область шага семпроходом для резолва чтения/вызова в рантайм
  (D-12, §PM-4) — но это поведение семпрохода, не свойство узла.
- **`Steps []*StepDecl`** — шаги в **порядке исходника** (D-4: порядок исполнения = порядок исходника,
  `после` — валидатор, не переупорядочиватель). Срез **указателей** `*StepDecl` (узел крупный, паритет с
  тем, как `[]*StepDecl` фигурирует в реестре/обходе). **Минимум один** шаг: грамматика
  `ProcessBlock ::= Newline Indent StepDecl+ Dedent` (grammar §7) требует ≥1; пустой `INDENT`-блок →
  ошибка парсера `пустой блок не допускается, добавьте хотя бы один оператор` (`msgEmptyBlock`,
  §PM-3/§PM-6.A). На уровне AST инвариант «`Steps` непуст» **обеспечивается парсером**, не самим
  конструктором (конструктор не валидирует — как `NewMetricDecl`).

### `StepDecl`

- **`Name Ident`** — имя шага, по значению. Уникально **в пределах процесса** (собственный namespace
  процесса, не глобальный, D-5/FR-011); дубль → `шаг '<имя>' уже объявлен в строке N` (семпроход,
  позиция имени повторного шага). `Name.Pos()` несёт позицию для диагностики.
- **`After []Ident`** — имена шагов-предшественников из `после Ident ("," Ident)*` (grammar §7
  `StepAfter`). `nil`/пусто, если `после` отсутствует. **Каждый `Ident` несёт свою `Pos()`** — критично
  для диагностик §PM-6.B (`шаг '<S>' после '<X>', но …` указывает на позицию имени `X`, а не на начало
  шага). Семантика (D-4, §11.2): валидатор ссылки-назад (`X` — объявленный ранее шаг этого процесса),
  не порядок исполнения; ацикличность обеспечена строго-назадной ссылкой по построению (§PM-4). `после`
  без скобок (отличие от `Params`); требует ≥1 имени (грамматически).
- **`Assignee Expression`** — выражение атрибута `исполнитель:` (grammar §7 `StepAttr`). **`nil`**, если
  атрибут отсутствует (как `MetricDecl.Where`). **Не** `Expression?`-обёртка — сам интерфейс со
  значением `nil` (единообразно с `MetricDecl`). Выражение семпроходом **не обходится** (D-11/FR-017,
  как `где`/`агрегат` у `metric`): резолв/типы/`DurationLit` — рантайму (006); поэтому `срок: 2дн`
  (`DurationLit`) проходит семантику (§PM-5).
- **`Deadline Expression`** — выражение атрибута `срок:`. **`nil`**, если отсутствует. Те же правила
  необхода, что и для `Assignee`. Семантическое правило `срок` без `исполнитель` (§11.4, FR-013)
  проверяется через `Attrs` (позиции), не через сами выражения (§PM-4).
- **`Attrs StepAttrPos`** — позиции ключевых слов присутствующих атрибутов (см. §3). Инвариант пэйринга:
  `Attrs.AssigneePos.Line != 0` ⟺ `Assignee != nil` (атрибут присутствует); аналогично
  `Attrs.DeadlinePos.Line != 0` ⟺ `Deadline != nil`. Парсер выставляет позицию ключевого слова
  (`исполнитель`/`срок`) при разборе соответствующей `StepAttr` (§PM-3).
- **`Body []Statement`** — императивные операторы тела шага в **порядке исходника** (как тело функции
  хранит `Block.Stmts`, но здесь — плоский срез, без обёртки `*Block`). Содержит:
  `AssignAction`/`CallAction`/`NotifyAction` (действия, через `parseStepAction`), `LetStmt`, `IfStmt`,
  `WhileStmt`, `ForStmt`, `ReturnStmt`, `ExpressionStmt` и т.п. Атрибуты (`исполнитель`/`срок`) **в
  `Body` НЕ попадают** — они оседают в `Assignee`/`Deadline`/`Attrs`. `Body` **обходится** семпроходом
  (контекст-гард действий, арность `запустить процесс`/вызовов в top-level операторах тела, контекст
  `вернуть`/`прервать`/`продолжить`; D-11/D-12, §PM-4).

**Инвариант «тело шага непусто».** Грамматика `StepBlock ::= Newline Indent StepLine+ Dedent` (grammar
§7) требует ≥1 строку, но строка может быть **атрибутом**. Поэтому «непусто» = «есть ≥1 атрибут (`Assignee`
или `Deadline` ≠ nil) **или** ≥1 оператор (`len(Body) > 0`)». То есть `Body` **может быть пустым**, если
у шага есть хотя бы один атрибут (кейс §PM-7:
`процесс P(x):`⏎`  шаг A:`⏎`    исполнитель: "и"`⏎`    срок: 2дн` → `Body == nil`,
`Assignee`/`Deadline` заполнены). Проверка непустоты — **парсер** (пустой `INDENT`-блок шага →
`msgEmptyBlock`), не конструктор.

### Действия и запуск (уже существуют — НЕ вводить заново)

`AssignAction`/`CallAction`/`NotifyAction` (`ast/step.go:8-42`) и `RunProcessExpr` (`ast/expr.go:69-78`)
**уже построены** в 003/004 (подтверждено сводом кода) — 005 их **не дублирует и не меняет форму**
(D-2/D-10/FR-009). Краткий контекст связей (формы — справочно, не предмет правки):

| Узел (существующий) | Форма | Роль в 005 |
|---|---|---|
| `AssignAction` | `stmtBase`; `Name Ident`; `Value Expression` | элемент `StepDecl.Body`; контекст-гард в семпроходе («только в шаге») вместо безусловного deferred (§PM-4); payload `Value` семпроходом не обходится; исполнение — рантайм-deferred (`stmt.go:64`, недостижимо в 005) |
| `CallAction` | `stmtBase`; `Name Ident`; `Args []Expression` | то же; `Args` не обходятся |
| `NotifyAction` | `stmtBase`; `Name Ident`; `Args []Expression` | то же; `Args` не обходятся |
| `RunProcessExpr` | `exprBase`; `Process Ident`; `Args []Expression` | мост из императивного кода; имя `Process.Name` резолвится семпроходом **только** против реестра процессов (D-10), арность против `pd.Params` (§PM-4/§PM-6.C); исполнение — рантайм-deferred (`expr.go:49`, наблюдаемая граница 005, §PM-5) |

> **Граница 007 (узлы НЕ вводятся в 005).** `TriggerDecl`/`MetricTrigger`/`EventTrigger`/
> `ScheduleTrigger`/`ScheduleSpec` (grammar §8) в 005 **отсутствуют** — их ведущие токены
> (`когда`/`значение`) остаются отвергаемыми парсером (`KW_WHEN`/`KW_VALUE` в `isUnexpectedTopLevel`,
> §PM-3). Их AST добавится тем же способом в 007. Конструктор `Длительность` не вводится; `DurationLit`
> остаётся deferred (§PM-0).

---

## 3. Вспомогательная структура позиций — `StepAttrPos`

```go
// StepAttrPos — позиции ключевых слов присутствующих атрибутов шага (D-1/D-8).
// Нулевая Position{} означает отсутствующий атрибут. Вспомогательная структура,
// НЕ Node (без Pos()/маркеров). Аналог MetricAttrPos.
type StepAttrPos struct {
	AssigneePos Position // позиция ключевого слова «исполнитель» (нулевая — атрибут отсутствует)
	DeadlinePos Position // позиция ключевого слова «срок» (нулевая — атрибут отсутствует)
}
```

**Зачем отдельная структура** (D-1/D-8, паритет `MetricAttrPos`): диагностика §PM-6.B
`шаг '<имя>': срок без исполнитель не имеет эффекта` должна указывать на строку `срок:`
(`Attrs.DeadlinePos`), а не на начало шага (`StepDecl.Pos()`). Узлы-обёртки `StepAttr{Kind, Value, Pos}`
избыточны — плоские поля проще для обхода в семпроходе (контекст-гард, проверка пэйринга) и в будущем
движке 006 (D-1). Семпроход читает `Attrs.DeadlinePos.Line != 0 && Attrs.AssigneePos.Line == 0` для
правила «срок без исполнителя» (§PM-4).

**Соответствие `MetricAttrPos` (004).** `MetricAttrPos` несёт 5 позиций (`SourcePos`/`WherePos`/
`AggregatePos`/`PeriodPos`/`ByDatePos`); `StepAttrPos` — ровно 2 (`AssigneePos`/`DeadlinePos`), по числу
атрибутов шага. Тот же контракт «нулевая `Position{}` ⟺ атрибут отсутствует».

---

## 4. Конструкторы (дословно по §PM-2)

```go
// NewProcessDecl строит объявление процесса; pos — позиция токена процесс.
func NewProcessDecl(pos Position, name Ident, params []Ident, steps []*StepDecl) *ProcessDecl

// NewStepDecl строит объявление шага; pos — позиция токена шаг.
func NewStepDecl(pos Position, name Ident, after []Ident,
	assignee Expression, deadline Expression,
	attrs StepAttrPos, body []Statement) *StepDecl
```

- Оба возвращают **указатель** (как `NewFunctionDecl`/`NewMetricDecl`/`NewSourceDecl`).
- `NewProcessDecl` инициализирует базу композитным литералом `declBase{base{pos}}` (top-level —
  как `NewMetricDecl`).
- `NewStepDecl` инициализирует базу `base{pos}` (НЕ `declBase`/`stmtBase` — `StepDecl` не top-level и
  не statement; реализует только `Pos()`).
- Конструкторы **не валидируют** (паритет `NewMetricDecl`): инвариант «`Steps` непуст», пэйринг
  `Attrs`↔`Assignee`/`Deadline`, уникальность шагов — забота **парсера** (структурное) и **семпрохода**
  (семантическое), не конструктора.
- Порядок параметров `NewStepDecl` фиксирован §PM-2: `pos, name, after, assignee, deadline, attrs, body`.

**Компайл-тайм маркеры (тесты, по образцу `decl_test.go` 004):**

```go
var _ Decl = (*ProcessDecl)(nil)         // ProcessDecl — top-level декларация
var _ TopLevelItem = (*ProcessDecl)(nil) // и элемент верхнего уровня
var _ Node = (*StepDecl)(nil)            // StepDecl — узел (Pos()), но НЕ Decl/TopLevelItem/Statement
```

Тест `TestProcessDeclPos`/`TestStepDeclPos` проверяет `Pos()` через `!=`, поля прямым доступом
(`pd.Name.Name`, `pd.Params[i].Name`, `pd.Steps[0].After[0].Name`, `sd.Attrs.AssigneePos`), а две
компайл-тайм строки для `ProcessDecl` фиксируют участие в union; для `StepDecl` — **отрицательный**
факт (его нельзя присвоить `Decl`/`Statement`) обеспечивается отсутствием маркеров (компилятор не
даст `var _ Decl = (*StepDecl)(nil)`).

---

## 5. Связи и инварианты (сводка)

| Связь / инвариант | Носитель | Правило | Где проверяется |
|---|---|---|---|
| `ProcessDecl` ∈ union `Decl` | `declBase` embedding | `FunctionDecl \| SourceDecl \| MetricDecl \| ProcessDecl` | компилятор (маркеры) |
| `StepDecl` ∉ `Decl`/`TopLevelItem`/`Statement` | `base` embedding (без маркеров) | достижим только через `ProcessDecl.Steps` | компилятор |
| `len(Steps) ≥ 1` | `ProcessDecl.Steps` | пустой блок процесса → `msgEmptyBlock` | парсер (§PM-3) |
| тело шага непусто (≥1 атрибут **или** ≥1 оператор) | `StepDecl` | пустой блок шага → `msgEmptyBlock` | парсер (§PM-3) |
| `AssigneePos.Line != 0` ⟺ `Assignee != nil` | `Attrs` ↔ `Assignee` | парсер выставляет позицию при разборе `исполнитель:` | парсер (§PM-3) |
| `DeadlinePos.Line != 0` ⟺ `Deadline != nil` | `Attrs` ↔ `Deadline` | парсер выставляет позицию при разборе `срок:` | парсер (§PM-3) |
| дубль атрибута (`исполнитель`/`срок` дважды) | один `Assignee`/`Deadline` | плоский AST негде хранить второе → `атрибут '<имя>' уже задан` (`p.error`+`break`) | парсер (D-8, §PM-3) |
| уникальность `Steps[i].Name` в процессе | `ProcessDecl.Steps` | дубль → `шаг '<имя>' уже объявлен в строке N` | семпроход (D-5, §PM-4) |
| `After[i]` — объявленный **ранее** шаг | `StepDecl.After` (каждый со своей `Pos()`) | не шаг → `шаг '<S>' после '<X>', но шаг '<X>' не объявлен`; позже/сам → `… '<X>' объявлен позже` | семпроход (D-4, §PM-4) |
| `Deadline ≠ nil && Assignee == nil` | `Attrs` | → `шаг '<имя>': срок без исполнитель не имеет эффекта` (поз. `DeadlinePos`) | семпроход (§11.4, §PM-4) |
| дубль параметра процесса | `Params` | **НЕ проверяется** (D-13, паритет `FunctionDecl`) | — (намеренно нет) |
| `Assignee`/`Deadline`/`Args`/`Value` не обходятся | подвыражения атрибутов и payload действий | резолв/типы/`DurationLit` — рантайму | — (D-11, §PM-4/§PM-5) |

---

## Сводка узлов и счёт

- **Пакет `internal/ast`** (+1 top-level узел, +1 не-top-level узел, +1 вспом. структура):
  - `ProcessDecl` (3 поля + `declBase`) — пополняет union `Decl` =
    `FunctionDecl | SourceDecl | MetricDecl | ProcessDecl` (FR-008). Конструктор `NewProcessDecl`
    (дословно §PM-2).
  - `StepDecl` (6 полей + `base`) — **не** top-level; реализует только `Pos()`. Конструктор
    `NewStepDecl` (дословно §PM-2). Плоская форма (D-1).
  - `StepAttrPos` (2 позиции, **не** `Node`) — аналог `MetricAttrPos`; нулевая `Position{}` ⟺ атрибут
    отсутствует.
- **Уже существуют (НЕ трогать форму):** `AssignAction`/`CallAction`/`NotifyAction` (`ast/step.go`,
  003/004), `RunProcessExpr` (`ast/expr.go`, 003) — D-2/D-10/FR-009. Меняется только их обработка в
  семпроходе (см. plan/contracts).
- **НЕ вводятся (D-1, §PM-2):** узлы `StepLine`, `StepAttr{Kind}` (плоский AST). **НЕ вводятся
  (007):** `TriggerDecl`/`MetricTrigger`/`EventTrigger`/`ScheduleTrigger`/`ScheduleSpec`. **НЕ
  вводится:** конструктор `Длительность` (`DurationLit` остаётся deferred, §PM-0).
- **Синк (зона якоря):** doc-комментарий `ast/node.go:24` (union `Decl`) — обновить под четыре
  декларации (§PM-0 п.6).
