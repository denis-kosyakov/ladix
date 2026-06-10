package ast

// Node — базовый интерфейс всех узлов: каждый узел знает свою позицию (1-based,
// руны) по конвенции Pos() (D4).
type Node interface {
	Pos() Position
}

// TopLevelItem — элемент верхнего уровня Program: Statement или Decl. Маркер
// topLevelItem() реализуют все Statement и FunctionDecl, что и образует union.
type TopLevelItem interface {
	Node
	topLevelItem()
}

// Statement — маркер-интерфейс операторов (sum-type через пустой метод
// stmtNode()). Любой Statement пригоден и как TopLevelItem.
type Statement interface {
	TopLevelItem
	stmtNode()
}

// Decl — маркер-интерфейс деклараций. union: FunctionDecl | SourceDecl | MetricDecl | ProcessDecl.
type Decl interface {
	TopLevelItem
	declNode()
}

// Expression — маркер-интерфейс выражений (sum-type через пустой метод exprNode()).
type Expression interface {
	Node
	exprNode()
}

// base — встраиваемая база: даёт Pos() через embedding, без копипасты в узлах.
type base struct {
	position Position
}

// Pos возвращает позицию узла.
func (b base) Pos() Position { return b.position }

// stmtBase — база для statements: Pos() + маркеры stmtNode/topLevelItem.
type stmtBase struct{ base }

func (stmtBase) stmtNode()     {}
func (stmtBase) topLevelItem() {}

// declBase — база для деклараций: Pos() + маркеры declNode/topLevelItem.
type declBase struct{ base }

func (declBase) declNode()     {}
func (declBase) topLevelItem() {}

// exprBase — база для выражений: Pos() + маркер exprNode.
type exprBase struct{ base }

func (exprBase) exprNode() {}
