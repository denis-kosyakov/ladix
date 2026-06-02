package ast

// Зарезервированные действия шага процесса (ARCHITECTURE §4.6, grammar §7). Все
// реализуют Statement и СТРОЯТСЯ парсером, но семантически НЕ валидируются: гард
// «допустимо только в шаге процесса» — eval-later (guardrail 6). Pos() — ведущий
// ключевой токен.

// AssignAction — присвоить Name = Value (мутирует переменные процесса).
type AssignAction struct {
	stmtBase
	Name  Ident
	Value Expression
}

// NewAssignAction строит присвоить; pos — позиция токена присвоить.
func NewAssignAction(pos Position, name Ident, value Expression) *AssignAction {
	return &AssignAction{stmtBase: stmtBase{base{pos}}, Name: name, Value: value}
}

// CallAction — вызвать Name(Args) (внешняя система, fire-and-forget).
type CallAction struct {
	stmtBase
	Name Ident
	Args []Expression
}

// NewCallAction строит вызвать; pos — позиция токена вызвать.
func NewCallAction(pos Position, name Ident, args []Expression) *CallAction {
	return &CallAction{stmtBase: stmtBase{base{pos}}, Name: name, Args: args}
}

// NotifyAction — уведомить Name(Args) (стаб-лог сообщения).
type NotifyAction struct {
	stmtBase
	Name Ident
	Args []Expression
}

// NewNotifyAction строит уведомить; pos — позиция токена уведомить.
func NewNotifyAction(pos Position, name Ident, args []Expression) *NotifyAction {
	return &NotifyAction{stmtBase: stmtBase{base{pos}}, Name: name, Args: args}
}
