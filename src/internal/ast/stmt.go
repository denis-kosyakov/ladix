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

// IfStmt — условие: если Cond: Then [ElseClause]. Else == nil → без иначе.
// Pos() = токен если.
type IfStmt struct {
	stmtBase
	Cond Expression
	Then *Block
	Else *ElseClause
}

// NewIfStmt строит условие; pos — позиция токена если.
func NewIfStmt(pos Position, cond Expression, then *Block, els *ElseClause) *IfStmt {
	return &IfStmt{stmtBase: stmtBase{base{pos}}, Cond: cond, Then: then, Else: els}
}

// ElseClause — ветвь иначе (union, ARCHITECTURE §4.3):
//   - финальный иначе: Body != nil (Cond/Then/Else пусты);
//   - иначе если: Cond/Then заданы, Else — следующая ветвь (nil → конец цепочки).
//
// Pos() = токен иначе. Это не Statement — отдельный узел, на который ссылается IfStmt.
type ElseClause struct {
	base
	Body *Block      // != nil → финальный иначе
	Cond Expression  // иначе если: условие
	Then *Block      // иначе если: тело
	Else *ElseClause // иначе если: следующая ветвь
}

// NewElseBlock строит финальную ветвь иначе с телом body.
func NewElseBlock(pos Position, body *Block) *ElseClause {
	return &ElseClause{base: base{pos}, Body: body}
}

// NewElseIf строит ветвь иначе если с условием/телом и (опц.) следующей ветвью.
func NewElseIf(pos Position, cond Expression, then *Block, els *ElseClause) *ElseClause {
	return &ElseClause{base: base{pos}, Cond: cond, Then: then, Else: els}
}

// IsFinal сообщает, является ли ветвь финальным иначе (а не иначе если).
func (e *ElseClause) IsFinal() bool { return e.Body != nil }

// WhileStmt — пока Cond: Body. Pos() = токен пока.
type WhileStmt struct {
	stmtBase
	Cond Expression
	Body *Block
}

// NewWhileStmt строит цикл пока; pos — позиция токена пока.
func NewWhileStmt(pos Position, cond Expression, body *Block) *WhileStmt {
	return &WhileStmt{stmtBase: stmtBase{base{pos}}, Cond: cond, Body: body}
}

// ForStmt — для Var в Iterable: Body. Pos() = токен для.
type ForStmt struct {
	stmtBase
	Var      Ident
	Iterable Expression
	Body     *Block
}

// NewForStmt строит цикл для; pos — позиция токена для.
func NewForStmt(pos Position, variable Ident, iterable Expression, body *Block) *ForStmt {
	return &ForStmt{stmtBase: stmtBase{base{pos}}, Var: variable, Iterable: iterable, Body: body}
}

// ReturnStmt — вернуть [Value]. Value == nil → голый возврат. Pos() = токен вернуть.
type ReturnStmt struct {
	stmtBase
	Value Expression
}

// NewReturnStmt строит возврат; value может быть nil (голый вернуть).
func NewReturnStmt(pos Position, value Expression) *ReturnStmt {
	return &ReturnStmt{stmtBase: stmtBase{base{pos}}, Value: value}
}

// BreakStmt — прервать. Pos() = токен прервать.
type BreakStmt struct {
	stmtBase
}

// NewBreakStmt строит прервать.
func NewBreakStmt(pos Position) *BreakStmt {
	return &BreakStmt{stmtBase: stmtBase{base{pos}}}
}

// ContinueStmt — продолжить. Pos() = токен продолжить.
type ContinueStmt struct {
	stmtBase
}

// NewContinueStmt строит продолжить.
func NewContinueStmt(pos Position) *ContinueStmt {
	return &ContinueStmt{stmtBase: stmtBase{base{pos}}}
}
