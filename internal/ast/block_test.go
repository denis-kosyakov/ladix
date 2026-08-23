package ast

import "testing"

// T028 (часть): Block (≥1 оператор; Pos() = INDENT/первый оператор) и корень Program.

func TestBlockPosAndStmts(t *testing.T) {
	indentPos := Position{Line: 2, Col: 5}
	s := NewExpressionStmt(NewIdent(Position{Line: 2, Col: 5}, "x"))
	b := NewBlock(indentPos, []Statement{s})
	if b.Pos() != indentPos {
		t.Errorf("Block.Pos() = %+v, хотим INDENT/первый оператор %+v", b.Pos(), indentPos)
	}
	if len(b.Stmts) != 1 {
		t.Errorf("Block.Stmts = %d, хотим 1", len(b.Stmts))
	}
}

func TestProgramRoot(t *testing.T) {
	prog := &Program{EOFPos: Position{Line: 9, Col: 1}}
	if len(prog.Items) != 0 {
		t.Errorf("пустой Program.Items = %d, хотим 0", len(prog.Items))
	}
	if prog.EOFPos.Line != 9 {
		t.Errorf("EOFPos = %+v", prog.EOFPos)
	}
}
