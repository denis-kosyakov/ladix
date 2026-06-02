package ast

import "testing"

// T004: String() операторов и предикат принадлежности CompOp подмножеству BinOp.

func TestBinOpString(t *testing.T) {
	tests := []struct {
		op   BinOp
		want string
	}{
		{OpOr, "или"}, {OpAnd, "и"},
		{OpAdd, "+"}, {OpSub, "-"}, {OpMul, "*"}, {OpDiv, "/"},
		{OpFloorDiv, "//"}, {OpMod, "%"},
		{OpEq, "=="}, {OpNeq, "!="}, {OpLt, "<"}, {OpLe, "<="}, {OpGt, ">"}, {OpGe, ">="},
	}
	if len(tests) != 14 {
		t.Fatalf("ожидалось 14 бинарных операторов, в таблице %d", len(tests))
	}
	for _, tt := range tests {
		if got := tt.op.String(); got != tt.want {
			t.Errorf("BinOp(%d).String() = %q, хотим %q", int(tt.op), got, tt.want)
		}
	}
	// неизвестный оператор — отладочный фолбэк
	if got := BinOp(99).String(); got != "BinOp(99)" {
		t.Errorf("BinOp(99).String() = %q, хотим \"BinOp(99)\"", got)
	}
}

func TestCompOpSubsetOfBinOp(t *testing.T) {
	comps := []BinOp{OpEq, OpNeq, OpLt, OpLe, OpGt, OpGe}
	if len(comps) != 6 {
		t.Fatalf("ожидалось 6 сравнений, %d", len(comps))
	}
	for _, op := range comps {
		if !op.IsComparison() {
			t.Errorf("%s должен принадлежать подмножеству CompOp", op)
		}
		// CompOp над тем же значением даёт ту же лексему — константы не дублируются
		if CompOp(op).String() != op.String() {
			t.Errorf("CompOp(%s).String() = %q != BinOp %q", op, CompOp(op).String(), op.String())
		}
	}
	// именованные константы CompOp совпадают со своими BinOp
	if CompEq != CompOp(OpEq) || CompGe != CompOp(OpGe) {
		t.Errorf("именованные CompOp не совпадают с BinOp")
	}
	for _, op := range []BinOp{OpOr, OpAnd, OpAdd, OpSub, OpMul, OpDiv, OpFloorDiv, OpMod} {
		if op.IsComparison() {
			t.Errorf("%s НЕ должен быть сравнением", op)
		}
	}
}

func TestUnOpString(t *testing.T) {
	if got := OpNot.String(); got != "не" {
		t.Errorf("OpNot.String() = %q, хотим \"не\"", got)
	}
	if got := OpNeg.String(); got != "-" {
		t.Errorf("OpNeg.String() = %q, хотим \"-\"", got)
	}
	if got := UnOp(99).String(); got != "UnOp(99)" {
		t.Errorf("UnOp(99).String() = %q, хотим \"UnOp(99)\"", got)
	}
}
