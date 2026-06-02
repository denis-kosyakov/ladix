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
