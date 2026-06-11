# Контракт: AST-узлы процесса (`internal/ast`)

**Фаза**: 1 (design) | **Якорь**: `docs/process-model.md §PM-2` | **Решения**: D-1, D-2, D-10

> Канон форм фиксируется §PM-2; этот контракт переписывает его в файлово-конкретные сигнатуры с
> проверенными координатами образца (004: `ast/decl.go`/`ast/decl_test.go`). При расхождении
> побеждает §PM-2.

## Назначение

Два новых **плоских** узла + одна вспомогательная структура. Никаких `StepLine`/`StepAttr{Kind}`
(D-1). Узлы действий (`AssignAction`/`CallAction`/`NotifyAction`, `ast/step.go:8-42`) и
`RunProcessExpr` (`ast/expr.go:69-78`) **уже построены** (003/004) и НЕ вводятся заново (D-2/D-10).

Файл — новый `ast/process.go` (или дополнение `ast/decl.go`/`ast/step.go` — формы фиксированы).
`ast` остаётся листовым: только `ast`-локальная `Position` и существующие `ast`-типы.

## `ProcessDecl` — объявление процесса (top-level декларация)

```go
// ProcessDecl — объявление процесса: процесс Name(Params): Steps. Только верхний
// уровень (§PM-2). Параметры позиционные, опциональны. Steps — минимум один
// (грамматика ProcessBlock ::= StepDecl+; пустой блок → ошибка парсера). Само не
// исполняется в рантайме 005 (исполнение — движок 006). Pos() = токен процесс.
type ProcessDecl struct {
	declBase            // → Decl/TopLevelItem (как FunctionDecl/SourceDecl/MetricDecl)
	Name   Ident
	Params []Ident      // позиционные; nil/пусто при процесс P:
	Steps  []*StepDecl  // ≥1
}

func NewProcessDecl(pos Position, name Ident, params []Ident, steps []*StepDecl) *ProcessDecl {
	return &ProcessDecl{declBase: declBase{base{pos}}, Name: name, Params: params, Steps: steps}
}
```

- Встраивает `declBase` первым полем → реализует `declNode()`/`topLevelItem()` автоматически
  (`ast/node.go:49-53`), пополняет union `Decl` (`FunctionDecl | SourceDecl | MetricDecl |
  ProcessDecl`). Инвариант: `var _ Decl = (*ProcessDecl)(nil)`.

## `StepDecl` — объявление шага (НЕ top-level)

```go
// StepDecl — шаг внутри ProcessDecl.Steps (НЕ top-level, НЕ оператор). Реализует
// только Pos() (встраивает base, без declBase/stmtBase). Assignee/Deadline = nil
// при отсутствии атрибута. After = nil/пусто при отсутствии после. Body — операторы
// тела в порядке исходника. Pos() = токен шаг.
type StepDecl struct {
	base                  // → только Pos()
	Name     Ident
	After    []Ident      // имена шагов-предшественников; каждый несёт свою Pos()
	Assignee Expression   // nil при отсутствии исполнитель:
	Deadline Expression   // nil при отсутствии срок:
	Attrs    StepAttrPos
	Body     []Statement  // действия + императивные операторы
}

func NewStepDecl(pos Position, name Ident, after []Ident,
	assignee Expression, deadline Expression,
	attrs StepAttrPos, body []Statement) *StepDecl {
	return &StepDecl{base: base{pos}, Name: name, After: after,
		Assignee: assignee, Deadline: deadline, Attrs: attrs, Body: body}
}
```

- Встраивает `base` (НЕ `declBase`/`stmtBase`): шаг живёт только в `ProcessDecl.Steps`, не
  верхнеуровневый и не оператор. Инвариант: `var _ Node = (*StepDecl)(nil)` (но НЕ `Statement`/`Decl`).
- «Тело шага непусто» = «есть ≥1 атрибут **или** ≥1 оператор»: если шаг — только `исполнитель:`/`срок:`,
  то `Body` пуст, а `Assignee`/`Deadline` заполнены (проверка пустого `INDENT`-блока — парсер,
  §PM-3).

## `StepAttrPos` — позиции ключевых слов атрибутов

```go
// StepAttrPos — позиции ключевых слов присутствующих атрибутов шага (D-1).
// Нулевая Position{} ⟺ атрибут отсутствует. Вспомогательная структура, НЕ Node.
type StepAttrPos struct {
	AssigneePos Position // позиция токена исполнитель (Line!=0 ⟺ присутствует)
	DeadlinePos Position // позиция токена срок
}
```

- Аналог `MetricAttrPos` (`ast/decl.go:70-76`). НЕ реализует `Pos()`/маркеры. Нужна для диагностики
  `шаг '<имя>': срок без исполнитель не имеет эффекта` с позицией строки `срок:` (§PM-6.B, IV).

## Тесты (образец `ast/decl_test.go:5-73`, `ast/step_test.go`)

`ast/process_test.go`:
- `TestProcessDeclPos`: `Pos()`=токен `процесс`; поля `Name`/`Params`/`Steps`; `var _ Decl = pd`;
  `var _ TopLevelItem = pd`.
- `TestStepDeclPos`: `Pos()`=токен `шаг`; поля `Name`/`After`/`Assignee`/`Deadline`/`Attrs`/`Body`;
  `var _ Node = sd`; проверить, что `Assignee==nil`/`Deadline==nil` при отсутствии; `Attrs.AssigneePos`/
  `DeadlinePos` ненулевые при наличии.
- Опц. параметры: `процесс P:` → `Params==nil`/пусто.

## Синк union

`ast/node.go:24` — doc-комментарий `Decl`: «В подмножестве B единственная — FunctionDecl» →
«union: FunctionDecl | SourceDecl | MetricDecl | ProcessDecl» (§PM-2, FR-028).
