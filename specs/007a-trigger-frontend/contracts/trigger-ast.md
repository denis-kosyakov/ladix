# Контракт: AST-узлы триггеров

**Фаза**: 1 (design) | **Якорь**: `docs/trigger-model.md §TR-2` | **Решения**: D-TR-1, D-25 | **FR**: FR-001…006

> Все новые узлы — **аддитивные**, листовые (`ast` не импортирует `errors`); формы триггера и расписания моделируются маркер-интерфейсами с type-switch, без tag-полей.

## Назначение

Go-сигнатуры новых узлов AST фронтенда триггеров: верхнеуровневая декларация `TriggerDecl`; маркер-интерфейс `TriggerSpec` с тремя формами (`MetricTrigger`/`EventTrigger`/`ScheduleTrigger`); маркер-интерфейс `ScheduleSpec` с двумя подформами (`EverySchedule`/`AtSchedule`); два первичных выражения (`ValueExpr`/`EventExpr`). Тип `CompOp` уже существует в `ast/op.go:53-68` — переиспользуется, не вводится. Доступ `событие.поле` собирается существующим `FieldExpr`, нового узла не требует.

**Конвенции** (по образцам существующего AST): `declBase` для top-level деклараций (реализует `topLevelItem()`+`declNode()`), `exprBase` для выражений (реализует `exprNode()`), `base` для вспомогательных узлов (только `Pos()`); конструкторы `New…(pos Position, …)` встраивают `base{pos}`; `Position` — 1-based, колонка в рунах.

**Размещение**: `src/internal/ast/trigger.go` — НОВЫЙ (`TriggerDecl`, формы, `ScheduleSpec`, подформы); `src/internal/ast/expr.go` — ИЗМЕНЯЕТСЯ (+`ValueExpr`, +`EventExpr`).

## TriggerDecl — top-level декларация (§TR-2)

Зеркало `ProcessDecl`/`FunctionDecl`. Позиция = токен `когда`. Тело — `*Block` (как `FunctionDecl.Body`): парсится штатным `parseBlock()`, который возвращает `*ast.Block`.

```go
// TriggerDecl — объявление триггера верхнего уровня: «когда <Spec>: <Body>».
// Pos() = токен «когда». Spec — одна из трёх форм (MetricTrigger/EventTrigger/
// ScheduleTrigger). Body — индентный блок (тело триггера, императивное ядро).
type TriggerDecl struct {
	declBase
	Spec TriggerSpec // форма триггера (метрика/событие/расписание)
	Body *Block      // тело: NEWLINE INDENT Statement+ DEDENT
}

func NewTriggerDecl(pos Position, spec TriggerSpec, body *Block) *TriggerDecl {
	return &TriggerDecl{declBase: declBase{base{pos}}, Spec: spec, Body: body}
}
```

`TriggerDecl` реализует **`Decl`** (через `declBase` → `declNode()`+`topLevelItem()`), встаёт в `parseTopLevelItem` рядом с `ProcessDecl` (см. parser-seams.md, шов A).

## TriggerSpec — маркер-интерфейс трёх форм (§TR-2)

Спецификации — вспомогательные узлы (встраивают `base`, как `ElseClause`), несут только данные формы и позицию. Дискриминация — Go type-switch по конкретному типу.

```go
// TriggerSpec — форма триггера. Реализуется MetricTrigger, EventTrigger,
// ScheduleTrigger. Маркер triggerSpec() — пустой метод (как stmtNode/exprNode).
type TriggerSpec interface {
	Node
	triggerSpec()
}

type specBase struct{ base }

func (specBase) triggerSpec() {}

// MetricTrigger — «метрика Metric Op Threshold». Pos() = токен «метрика».
// Условие — ОДНО сравнение (см. analyze-trigger.md проверка 6, ограничение v1).
type MetricTrigger struct {
	specBase
	Metric    Ident      // имя метрики (резолвится семпроходом против i.metrics)
	Op        CompOp     // оператор сравнения (==,!=,<,<=,>,>=)
	Threshold Expression // правая часть сравнения
}

func NewMetricTrigger(pos Position, metric Ident, op CompOp, threshold Expression) *MetricTrigger {
	return &MetricTrigger{specBase: specBase{base{pos}}, Metric: metric, Op: op, Threshold: threshold}
}

// EventTrigger — «событие Event». Pos() = токен «событие».
type EventTrigger struct {
	specBase
	Event Ident // имя события
}

func NewEventTrigger(pos Position, event Ident) *EventTrigger {
	return &EventTrigger{specBase: specBase{base{pos}}, Event: event}
}

// ScheduleTrigger — «расписание ScheduleSpec». Pos() = токен «расписание».
type ScheduleTrigger struct {
	specBase
	Spec ScheduleSpec // «каждые DurationLiteral» ИЛИ «в StringLiteral»
}

func NewScheduleTrigger(pos Position, spec ScheduleSpec) *ScheduleTrigger {
	return &ScheduleTrigger{specBase: specBase{base{pos}}, Spec: spec}
}
```

### CompOp — наземная правда (`ast/op.go:53-68`)

`MetricTrigger.Op` имеет тип `ast.CompOp` — **defined type** над `BinOp` (`type CompOp BinOp`, НЕ type-alias `= BinOp`), уже объявлен. Шесть констант: `CompEq/CompNeq/CompLt/CompLe/CompGt/CompGe` (значения `OpEq/OpNeq/OpLt/OpLe/OpGt/OpGe`). Метод `String()` переиспользует `BinOp.String()`. Комментарий `op.go:55` прямо фиксирует: «На CompOp сошлётся будущий `MetricTrigger.Op`». Это канон, не выбор — узел `MetricTrigger` лишь несёт его в поле `Op`.

## ScheduleSpec — вариант «каждые | в» (§TR-2)

Два взаимоисключающих варианта моделируются **двумя конкретными типами под общим маркером** (а не одним типом с тегом+двумя nil-полями) — единообразно с `TriggerSpec` и проще для type-switch.

```go
// ScheduleSpec — спецификация расписания: интервал ИЛИ время суток.
// Реализуется EverySchedule («каждые») и AtSchedule («в»).
type ScheduleSpec interface {
	Node
	scheduleSpec()
}

type schedBase struct{ base }

func (schedBase) scheduleSpec() {}

// EverySchedule — «каждые DurationLiteral». Pos() = токен «каждые».
// Every — литерал длительности; все 6 единиц принимаются в 007a (§TR-1).
type EverySchedule struct {
	schedBase
	Every *DurationLit // 3дн/5мин/2нед/1мес/…
}

func NewEverySchedule(pos Position, every *DurationLit) *EverySchedule {
	return &EverySchedule{schedBase: schedBase{base{pos}}, Every: every}
}

// AtSchedule — «в StringLiteral». Pos() = токен «в». At — строковый литерал
// "ЧЧ:ММ"; формат содержимого в 007a НЕ проверяется (валидация → 007b serve).
type AtSchedule struct {
	schedBase
	At StringLit // "ЧЧ:ММ"
}

func NewAtSchedule(pos Position, at StringLit) *AtSchedule {
	return &AtSchedule{schedBase: schedBase{base{pos}}, At: at}
}
```

> **TODO-FACT (StringLit передача).** Поле `AtSchedule.At` взято **по значению** (`StringLit`), по образцу `SourceDecl.File StringLit` (по значению, не указатель). Точную передачу (значение vs указатель) подтвердить при импл-проходе по факту существующих узлов.
>
> **Примечание о наименовании.** Псевдокод парсера (parser-seams.md, шов A) ссылается на конструкторы `ScheduleEvery`/`ScheduleAt`; это синонимы `EverySchedule`/`AtSchedule` — финальные имена за импл-чатом, важна форма «два типа под маркером».

## ValueExpr / EventExpr — новые Primary-выражения (§TR-2)

Зеркало `RunProcessExpr`/`NoneLit`: встраивают `exprBase` (→ `exprNode()`), внутренних параметров нет (первичные выражения), `Pos()` = токен `значение`/`событие`. Принимаются `parsePrimary` синтаксически везде; контекст проверяет семпроход (см. analyze-trigger.md проверка 3).

```go
// ValueExpr — выражение «значение» (предопределённое имя метрика-триггера).
// Pos() = токен «значение». Допустимо только в теле метрика-триггера (гард семпрохода).
type ValueExpr struct {
	exprBase
}

func NewValueExpr(pos Position) *ValueExpr {
	return &ValueExpr{exprBase: exprBase{base{pos}}}
}

// EventExpr — выражение «событие» (предопределённое имя событие-триггера).
// Pos() = токен «событие». Допустимо только в теле событие-триггера (гард семпрохода).
type EventExpr struct {
	exprBase
}

func NewEventExpr(pos Position) *EventExpr {
	return &EventExpr{exprBase: exprBase{base{pos}}}
}
```

### Доступ `событие.поле` = переиспользование `FieldExpr`

Постфиксный доступ `событие.клиент` **НЕ хранится внутри `EventExpr`**. `parsePrimary` вернёт `EventExpr`, постфиксный слой выражений навесит существующий `FieldExpr{Target, Field}`:

```
событие.клиент  →  FieldExpr{Target: *EventExpr, Field: Ident("клиент")}
```

Это убирает дублирование логики доступа к полям и держит `EventExpr` беспараметрическим (как `RunProcessExpr` не тащит постфикс-вызов внутрь). Нового узла для доступа не вводится.

`ValueExpr`/`EventExpr` реализуют **`Expression`** (через `exprBase` → `exprNode()`).

## Таблица узлов

| Узел | Поля | Реализует | FR |
|---|---|---|---|
| `TriggerDecl` | `Spec TriggerSpec`, `Body *Block` | `Decl` (`declBase`: `declNode()`+`topLevelItem()`) | FR-001 |
| `TriggerSpec` (интерфейс) | — | `Node` + маркер `triggerSpec()` | FR-002…003 |
| `MetricTrigger` | `Metric Ident`, `Op CompOp`, `Threshold Expression` | `TriggerSpec` (`specBase`) | FR-002 |
| `EventTrigger` | `Event Ident` | `TriggerSpec` (`specBase`) | FR-003 |
| `ScheduleTrigger` | `Spec ScheduleSpec` | `TriggerSpec` (`specBase`) | FR-003 |
| `ScheduleSpec` (интерфейс) | — | `Node` + маркер `scheduleSpec()` | FR-003 |
| `EverySchedule` | `Every *DurationLit` | `ScheduleSpec` (`schedBase`) | FR-003, FR-004 |
| `AtSchedule` | `At StringLit` | `ScheduleSpec` (`schedBase`) | FR-003, FR-005 |
| `ValueExpr` | — (беспараметрическое) | `Expression` (`exprBase`) | FR-006 |
| `EventExpr` | — (беспараметрическое) | `Expression` (`exprBase`) | FR-006 |
| `CompOp` (`op.go`, **существует**) | `type CompOp BinOp` + 6 констант | — (несётся `MetricTrigger.Op`) | FR-002 |
| `FieldExpr` (**существует**) | `Target`, `Field` | `Expression` — даёт `событие.поле` | FR-006 |

## Чего нет

- **Durable-состояние триггера** в AST (`trigger_state`, `last_value`/`last_fire`) — узлы только описывают синтаксис; edge-детект и персист — 007b (§TR-11 п.2).
- **Tag-поля** дискриминации форм (`kind int`, `IsMetric bool`) — дискриминация через Go type-switch по конкретному типу, не через тег.
- **Узел доступа к полю события** — переиспользуется существующий `FieldExpr` над `EventExpr`.
- **Узел/тип `CompOp`** — не вводится, существует в `op.go:56`.
- **Поле имени у расписания** — у `ScheduleTrigger` собственного имени нет (только подформа); форма `<имя>` в строке-заглушке §TR-6.4 — импл-факт, закрывается кодом.
- **Валидация формата `"ЧЧ:ММ"`** в `AtSchedule` — узел принимает строку как есть, проверка отложена в 007b (§TR-11 п.3).
