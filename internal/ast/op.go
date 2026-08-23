package ast

import "fmt"

// BinOp — единый enum всех 14 бинарных операторов Ladix (D3, guardrail 3).
// Один источник истины: операторы сравнения отбираются из этого набора через
// CompOp, без дублирования констант.
type BinOp int

const (
	_          BinOp = iota // нулевое значение не используется
	OpOr                    // или
	OpAnd                   // и
	OpAdd                   // +
	OpSub                   // -
	OpMul                   // *
	OpDiv                   // /
	OpFloorDiv              // //
	OpMod                   // %
	OpEq                    // ==
	OpNeq                   // !=
	OpLt                    // <
	OpLe                    // <=
	OpGt                    // >
	OpGe                    // >=
)

var binOpText = map[BinOp]string{
	OpOr: "или", OpAnd: "и",
	OpAdd: "+", OpSub: "-", OpMul: "*", OpDiv: "/", OpFloorDiv: "//", OpMod: "%",
	OpEq: "==", OpNeq: "!=", OpLt: "<", OpLe: "<=", OpGt: ">", OpGe: ">=",
}

// String даёт лексему оператора для диагностики и табличных тестов.
func (op BinOp) String() string {
	if s, ok := binOpText[op]; ok {
		return s
	}
	return fmt.Sprintf("BinOp(%d)", int(op))
}

// IsComparison сообщает, входит ли оператор в подмножество сравнений CompOp
// (== != < <= > >=). Это предикат принадлежности, а не отдельный enum.
func (op BinOp) IsComparison() bool {
	switch op {
	case OpEq, OpNeq, OpLt, OpLe, OpGt, OpGe:
		return true
	default:
		return false
	}
}

// CompOp — именованное подмножество BinOp из 6 операторов сравнения (D3). Это
// defined type над BinOp (НЕ type-alias `= BinOp`, который принял бы любой BinOp).
// На CompOp сошлётся будущий MetricTrigger.Op (ARCHITECTURE §4.5).
type CompOp BinOp

const (
	CompEq  = CompOp(OpEq)
	CompNeq = CompOp(OpNeq)
	CompLt  = CompOp(OpLt)
	CompLe  = CompOp(OpLe)
	CompGt  = CompOp(OpGt)
	CompGe  = CompOp(OpGe)
)

// String переиспользует лексему базового BinOp.
func (op CompOp) String() string { return BinOp(op).String() }

// UnOp — enum унарных операторов (2): логическое отрицание и арифметический минус.
type UnOp int

const (
	_     UnOp = iota // нулевое значение не используется
	OpNot             // не
	OpNeg             // унарный -
)

var unOpText = map[UnOp]string{
	OpNot: "не",
	OpNeg: "-",
}

// String даёт лексему унарного оператора.
func (op UnOp) String() string {
	if s, ok := unOpText[op]; ok {
		return s
	}
	return fmt.Sprintf("UnOp(%d)", int(op))
}
