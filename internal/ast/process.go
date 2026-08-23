package ast

// ProcessDecl — объявление процесса: процесс Name(Params): Steps. Только верхний
// уровень (§PM-2). Параметры позиционные, опциональны. Steps — минимум один
// (грамматика ProcessBlock ::= StepDecl+; пустой блок → ошибка парсера). Само не
// исполняется в рантайме 005 (исполнение — движок 006). Pos() = токен процесс.
type ProcessDecl struct {
	declBase
	Name   Ident
	Params []Ident
	Steps  []*StepDecl
}

// NewProcessDecl строит объявление процесса; pos — позиция токена процесс.
func NewProcessDecl(pos Position, name Ident, params []Ident, steps []*StepDecl) *ProcessDecl {
	return &ProcessDecl{declBase: declBase{base{pos}}, Name: name, Params: params, Steps: steps}
}

// StepDecl — шаг внутри ProcessDecl.Steps (НЕ top-level, НЕ оператор). Реализует
// только Pos() (встраивает base, без declBase/stmtBase). Assignee/Deadline = nil
// при отсутствии атрибута. After = nil/пусто при отсутствии после. Body — операторы
// тела в порядке исходника. Pos() = токен шаг.
type StepDecl struct {
	base
	Name     Ident
	After    []Ident
	Assignee Expression
	Deadline Expression
	Attrs    StepAttrPos
	Body     []Statement
}

// NewStepDecl строит объявление шага; pos — позиция токена шаг.
func NewStepDecl(pos Position, name Ident, after []Ident,
	assignee Expression, deadline Expression,
	attrs StepAttrPos, body []Statement) *StepDecl {
	return &StepDecl{base: base{pos}, Name: name, After: after,
		Assignee: assignee, Deadline: deadline, Attrs: attrs, Body: body}
}

// StepAttrPos — позиции ключевых слов присутствующих атрибутов шага (D-1).
// Нулевая Position{} означает отсутствующий атрибут. Вспомогательная структура,
// НЕ Node (без Pos()/маркеров). Аналог MetricAttrPos.
type StepAttrPos struct {
	AssigneePos Position
	DeadlinePos Position
}
