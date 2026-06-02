package ast

// LetStmt — объявление переменной: пусть Name = Value. Pos() = токен пусть (D4).
type LetStmt struct {
	stmtBase
	Name  Ident
	Value Expression
}

// NewLetStmt строит объявление; pos — позиция токена пусть.
func NewLetStmt(pos Position, name Ident, value Expression) *LetStmt {
	return &LetStmt{stmtBase: stmtBase{base{pos}}, Name: name, Value: value}
}

// AssignStmt — присваивание Name = Value. lvalue только Ident (поля/индексы
// запрещены v1 → SE-ASSIGN-TARGET). Pos() = позиция lvalue (без ведущего ключевого
// слова, D4).
type AssignStmt struct {
	stmtBase
	Name  Ident
	Value Expression
}

// NewAssignStmt строит присваивание; позиция берётся у lvalue (Name).
func NewAssignStmt(name Ident, value Expression) *AssignStmt {
	return &AssignStmt{stmtBase: stmtBase{base{name.Pos()}}, Name: name, Value: value}
}

// ExpressionStmt — выражение в роли оператора (например вызов печать(...)).
// Pos() = Expr.Pos() (без ведущего ключевого слова, D4).
type ExpressionStmt struct {
	stmtBase
	Expr Expression
}

// NewExpressionStmt строит оператор-выражение; позиция берётся у выражения.
func NewExpressionStmt(expr Expression) *ExpressionStmt {
	return &ExpressionStmt{stmtBase: stmtBase{base{expr.Pos()}}, Expr: expr}
}
