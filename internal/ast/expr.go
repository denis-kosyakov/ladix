package ast

// BinaryExpr — бинарная операция. Лево-ассоциативна (цикл в каскаде разбора);
// и/или короткозамкнуты (семантика — eval). Pos() = токен ОПЕРАТОРА (D4).
type BinaryExpr struct {
	exprBase
	Op    BinOp
	Left  Expression
	Right Expression
}

// NewBinaryExpr строит бинарную операцию; opPos — позиция токена оператора.
func NewBinaryExpr(opPos Position, op BinOp, left, right Expression) *BinaryExpr {
	return &BinaryExpr{exprBase: exprBase{base{opPos}}, Op: op, Left: left, Right: right}
}

// UnaryExpr — унарная операция (не / унарный -). Pos() = токен ОПЕРАТОРА (D4);
// знак НЕ сворачивается в литерал (-X = UnaryExpr(-, IntLit(X))).
type UnaryExpr struct {
	exprBase
	Op      UnOp
	Operand Expression
}

// NewUnaryExpr строит унарную операцию; opPos — позиция токена оператора.
func NewUnaryExpr(opPos Position, op UnOp, operand Expression) *UnaryExpr {
	return &UnaryExpr{exprBase: exprBase{base{opPos}}, Op: op, Operand: operand}
}

// CallExpr — вызов f(args...). Pos() = позиция Callee (D4).
type CallExpr struct {
	exprBase
	Callee Expression
	Args   []Expression
}

// NewCallExpr строит вызов; позиция берётся у Callee.
func NewCallExpr(callee Expression, args []Expression) *CallExpr {
	return &CallExpr{exprBase: exprBase{base{callee.Pos()}}, Callee: callee, Args: args}
}

// IndexExpr — индексация target[index] (срезов нет в v1). Pos() = Target (D4).
type IndexExpr struct {
	exprBase
	Target Expression
	Index  Expression
}

// NewIndexExpr строит индексацию; позиция берётся у Target.
func NewIndexExpr(target, index Expression) *IndexExpr {
	return &IndexExpr{exprBase: exprBase{base{target.Pos()}}, Target: target, Index: index}
}

// FieldExpr — доступ к полю target.field. Pos() = Target (D4).
type FieldExpr struct {
	exprBase
	Target Expression
	Field  Ident
}

// NewFieldExpr строит доступ к полю; позиция берётся у Target.
func NewFieldExpr(target Expression, field Ident) *FieldExpr {
	return &FieldExpr{exprBase: exprBase{base{target.Pos()}}, Target: target, Field: field}
}

// RunProcessExpr — запуск процесса как выражение: запустить процесс Process(Args).
// Зарезервирован (семантика — eval). Скобки — часть узла, не постфикс-вызов по
// результату (grammar §9). Pos() = токен запустить.
type RunProcessExpr struct {
	exprBase
	Process Ident
	Args    []Expression
}

// NewRunProcessExpr строит запуск процесса; pos — позиция токена запустить.
func NewRunProcessExpr(pos Position, process Ident, args []Expression) *RunProcessExpr {
	return &RunProcessExpr{exprBase: exprBase{base{pos}}, Process: process, Args: args}
}

// CallExternalExpr — захват результата внешнего вызова: вызвать Target(Args) как
// ВЫРАЖЕНИЕ (B1, §AU-3). Имя CallExpr занято постфикс-вызовом f(args) (:31) →
// узел B1 называется CallExternalExpr. Цель — логическое имя (строка), не символ
// программы (не резолвится, арность не проверяется). Скобки — часть узла (как у
// RunProcessExpr), постфикс на результат — отдельной цепочкой parsePostfix.
// Pos() = токен вызвать.
type CallExternalExpr struct {
	exprBase
	Target Ident        // логическое имя цели (crm, ИТ, …)
	Args   []Expression // позиционные аргументы (могут быть пусты)
}

// NewCallExternalExpr строит узел захвата результата; pos — позиция токена вызвать.
func NewCallExternalExpr(pos Position, target Ident, args []Expression) *CallExternalExpr {
	return &CallExternalExpr{exprBase: exprBase{base{pos}}, Target: target, Args: args}
}

// ValueExpr — выражение «значение» (предопределённое имя метрика-триггера).
// Беспараметрическое первичное выражение (зеркало NoneLit). Pos() = токен
// «значение». Допустимо только в теле метрика-триггера (гард семпрохода).
type ValueExpr struct {
	exprBase
}

// NewValueExpr строит выражение «значение»; pos — позиция токена «значение».
func NewValueExpr(pos Position) *ValueExpr {
	return &ValueExpr{exprBase: exprBase{base{pos}}}
}

// EventExpr — выражение «событие» (предопределённое имя событие-триггера).
// Беспараметрическое первичное выражение (зеркало NoneLit). Доступ событие.поле
// навешивает существующий FieldExpr, нового узла не вводится. Pos() = токен
// «событие». Допустимо только в теле событие-триггера (гард семпрохода).
type EventExpr struct {
	exprBase
}

// NewEventExpr строит выражение «событие»; pos — позиция токена «событие».
func NewEventExpr(pos Position) *EventExpr {
	return &EventExpr{exprBase: exprBase{base{pos}}}
}
