package ast

import (
	"strings"
	"testing"
)

// csPos — позиция-заглушка: канон операторов НЕ зависит от позиции (как и канон выражений).
func csPos() Position { return Position{Line: 1, Col: 1} }

func csIdent(name string) Ident { return *NewIdent(csPos(), name) }

// csRef — идентификатор в позиции ВЫРАЖЕНИЯ (canonExpr знает *Ident, не Ident).
func csRef(name string) *Ident { return NewIdent(csPos(), name) }

func csInt(v int64) *IntLit { return NewIntLit(csPos(), v) }

func csStr(v string) *StringLit { return NewStringLit(csPos(), v) }

func csBlock(stmts ...Statement) *Block { return NewBlock(csPos(), stmts) }

// TestCanonicalStatementExhaustive — замок T-CANON-STMT (а): исчерпываемость. По одному
// кейсу на КАЖДЫЙ из 13 конкретных типов Statement; ни один не уходит в default-панику.
// Мутпроба: удаление любой ветки из type-switch уводит её тип в default и краснит тест.
func TestCanonicalStatementExhaustive(t *testing.T) {
	cases := []struct {
		name string
		stmt Statement
		want string
	}{
		{"LetStmt", NewLetStmt(csPos(), csIdent("x"), csInt(1)), "пусть x = 1"},
		{"AssignStmt", NewAssignStmt(csIdent("x"), csInt(2)), "x = 2"},
		{"ExpressionStmt", NewExpressionStmt(csInt(3)), "3"},
		{
			"IfStmt",
			NewIfStmt(csPos(), csRef("усл"), csBlock(NewBreakStmt(csPos())), nil),
			"если усл {прервать}",
		},
		{
			"WhileStmt",
			NewWhileStmt(csPos(), csRef("усл"), csBlock(NewContinueStmt(csPos()))),
			"пока усл {продолжить}",
		},
		{
			"TryStmt",
			NewTryStmt(csPos(), csBlock(NewBreakStmt(csPos())), csBlock(NewContinueStmt(csPos()))),
			"пытаться {прервать} словить {продолжить}",
		},
		{
			"ForStmt",
			NewForStmt(csPos(), csIdent("э"), csRef("список"), csBlock(NewBreakStmt(csPos()))),
			"для э в список {прервать}",
		},
		{"ReturnStmt", NewReturnStmt(csPos(), csInt(7)), "вернуть 7"},
		{"ReturnStmt/голый", NewReturnStmt(csPos(), nil), "вернуть"},
		{"BreakStmt", NewBreakStmt(csPos()), "прервать"},
		{"ContinueStmt", NewContinueStmt(csPos()), "продолжить"},
		{"AssignAction", NewAssignAction(csPos(), csIdent("статус"), csStr("готов")), `присвоить статус = "готов"`},
		{
			"CallAction",
			NewCallAction(csPos(), csIdent("crm"), []Expression{csRef("заказ"), csInt(5)}),
			"вызвать crm(заказ,5)",
		},
		{
			"NotifyAction",
			NewNotifyAction(csPos(), csIdent("менеджер"), []Expression{csStr("привет")}),
			`уведомить менеджер("привет")`,
		},
	}

	if len(cases) < 13 {
		t.Fatalf("замок исчерпываемости: покрыто %d кейсов, а конкретных типов Statement — 13", len(cases))
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CanonicalStatement(c.stmt)
			if got != c.want {
				t.Errorf("CanonicalStatement(%s) = %q, ожидалось %q", c.name, got, c.want)
			}
		})
	}
}

// TestCanonicalStatementNestedElseChain — канон вложенного ветвления «иначе если … иначе».
// Пинит рекурсию canonElse: промежуточные ветви и финальный иначе различимы.
func TestCanonicalStatementNestedElseChain(t *testing.T) {
	final := NewElseBlock(csPos(), csBlock(NewReturnStmt(csPos(), csInt(3))))
	middle := NewElseIf(csPos(), csRef("б"), csBlock(NewReturnStmt(csPos(), csInt(2))), final)
	stmt := NewIfStmt(csPos(), csRef("а"), csBlock(NewReturnStmt(csPos(), csInt(1))), middle)

	want := "если а {вернуть 1} иначе если б {вернуть 2} иначе {вернуть 3}"
	if got := CanonicalStatement(stmt); got != want {
		t.Errorf("канон цепочки иначе = %q, ожидалось %q", got, want)
	}
}

// TestCanonicalStatementMultiStatementBlock — блок из нескольких операторов
// разделяется «; » (канон читаем и однозначен).
func TestCanonicalStatementMultiStatementBlock(t *testing.T) {
	stmt := NewWhileStmt(csPos(), csRef("усл"), csBlock(
		NewLetStmt(csPos(), csIdent("i"), csInt(0)),
		NewAssignStmt(csIdent("i"), csInt(1)),
		NewBreakStmt(csPos()),
	))
	want := "пока усл {пусть i = 0; i = 1; прервать}"
	if got := CanonicalStatement(stmt); got != want {
		t.Errorf("канон блока = %q, ожидалось %q", got, want)
	}
}

// TestCanonicalStatementUnknownPanics — замок T-CANON-STMT (в): неизвестный тип
// оператора ОБЯЗАН громко паниковать, а не молча схлопываться в пустую строку.
// Это и есть мутпроба default-ветки.
func TestCanonicalStatementUnknownPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("CanonicalStatement на незнакомом типе обязана паниковать (инвариант, конституция III)")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "незнакомый тип оператора") {
			t.Errorf("паника без диагностирующего текста: %v", r)
		}
	}()
	CanonicalStatement(unknownStmt{})
}

// unknownStmt — синтетический Statement, отсутствующий в type-switch канонизатора.
type unknownStmt struct{ stmtBase }
