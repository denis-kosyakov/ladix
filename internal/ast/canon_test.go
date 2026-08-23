package ast

import "testing"

// pos — общая позиция-болванка для конструкторов узлов (канон позиции не зависит).
var pos = Position{Line: 1, Col: 1}

// stubExpr — локальный фиктивный тип выражения для теста исчерпываемости canonExpr.
// Встраивает unexported exprBase → удовлетворяет Expression, НЕ будучи ни одним из 19
// конкретных типов в switch. Таким образом он гарантированно проваливается в
// default-ветку (панику) — ловит молчаливый default, если бы тот вернул строку.
type stubExpr struct {
	exprBase
}

// TestCanonExprExhaustive — исчерпываемость тотального канонизатора canonExpr (§FR-003).
// Таблица содержит по ОДНОМУ собранному экземпляру КАЖДОГО из 19 конкретных типов
// Expression; для каждого проверяется точная каноническая строка (без паники). Составные
// типы (ListLit/BinaryExpr/CallExpr/IndexExpr/FieldExpr/RunProcessExpr/CallExternalExpr/
// UnaryExpr) покрыты простыми детьми. Плюс stub-тип, не входящий в switch, обязан
// уводить canonExpr в default-панику.
//
// 🔁 ИНВЕРСИОННЫЙ ЗАМОК: удалить любую ветку switch в canonExpr → её тип уходит в
// default-panic → соответствующий кейс таблицы краснеет (panic вместо строки); stub-тип
// ловит молчаливый default (если бы default вернул строку вместо паники — подкейс
// «незнакомый тип паникует» покраснел бы).
func TestCanonExprExhaustive(t *testing.T) {
	cases := []struct {
		name string
		expr Expression
		want string
	}{
		{"IntLit", NewIntLit(pos, 42), "42"},
		{"FloatLit", NewFloatLit(pos, 1.5), "1.5"},
		{"StringLit", NewStringLit(pos, "привет"), `"привет"`},
		{"BoolLit", NewBoolLit(pos, true), "true"},
		{"NoneLit", NewNoneLit(pos), "пусто"},
		{"DurationLit", NewDurationLit(pos, "3", "дн"), "длит(3|дн)"},
		{"WindowPeriodLit", NewWindowPeriodLit(pos, "7", "дн"), "окно(7|дн)"},
		{"LastCompletedPeriodLit", NewLastCompletedPeriodLit(pos, "месяц"), "прошлый(месяц)"},
		{"ListLit", NewListLit(pos, []Expression{NewIntLit(pos, 1), NewIntLit(pos, 2)}), "[1,2]"},
		{"Ident", NewIdent(pos, "выручка"), "выручка"},
		{"BinaryExpr", NewBinaryExpr(pos, OpAdd, NewIntLit(pos, 1), NewIntLit(pos, 2)), "(1 + 2)"},
		{"UnaryExpr", NewUnaryExpr(pos, OpNeg, NewIntLit(pos, 5)), "(-5)"},
		{"CallExpr", NewCallExpr(NewIdent(pos, "f"), []Expression{NewIntLit(pos, 1)}), "f(1)"},
		{"IndexExpr", NewIndexExpr(NewIdent(pos, "xs"), NewIntLit(pos, 0)), "xs[0]"},
		{"FieldExpr", NewFieldExpr(NewIdent(pos, "rec"), *NewIdent(pos, "поле")), "rec.поле"},
		{"RunProcessExpr", NewRunProcessExpr(pos, *NewIdent(pos, "проц"), []Expression{NewIntLit(pos, 1)}), "запустить(проц|1)"},
		{"CallExternalExpr", NewCallExternalExpr(pos, *NewIdent(pos, "crm"), []Expression{NewIntLit(pos, 1)}), "вызвать(crm|1)"},
		{"ValueExpr", NewValueExpr(pos), "значение"},
		{"EventExpr", NewEventExpr(pos), "событие"},
	}

	if len(cases) != 19 {
		t.Fatalf("таблица исчерпываемости: типов %d, хотим 19 (все конкретные Expression)", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := canonExpr(tc.expr); got != tc.want {
				t.Fatalf("canonExpr(%s) = %q, хотим %q", tc.name, got, tc.want)
			}
		})
	}

	// Незнакомый тип (stub) обязан вызвать default-панику, а не молчаливо вернуть строку.
	t.Run("незнакомый тип паникует", func(t *testing.T) {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("canonExpr(stubExpr) не запаниковал: default-ветка должна быть panic, не молчаливый возврат")
				}
			}()
			_ = canonExpr(&stubExpr{})
		}()
	})
}

// TestCanonicalExpressionMirrorsCanonExpr — замок 029/T008: публичная точка входа
// CanonicalExpression даёт для НЕ-nil выражения ту же строку, что видит
// CanonicalTriggerCondition (общий canonExpr — канон ОДИН, не два расходящихся).
// Сверка ведётся через метрик-триггер: его канон оканчивается каноном порога.
func TestCanonicalExpressionMirrorsCanonExpr(t *testing.T) {
	exprs := []Expression{
		NewIntLit(csPos(), 42),
		NewFloatLit(csPos(), 1.5),
		NewStringLit(csPos(), "оплачено"),
		NewBoolLit(csPos(), true),
		NewNoneLit(csPos()),
		NewIdent(csPos(), "выручка"),
		NewBinaryExpr(csPos(), OpAdd, NewIntLit(csPos(), 1), NewIntLit(csPos(), 2)),
		NewUnaryExpr(csPos(), OpNeg, NewIntLit(csPos(), 7)),
		NewCallExpr(NewIdent(csPos(), "сумма"), []Expression{NewIdent(csPos(), "поле")}),
		NewListLit(csPos(), []Expression{NewIntLit(csPos(), 1), NewIntLit(csPos(), 2)}),
	}
	for _, e := range exprs {
		spec := NewMetricTrigger(csPos(), csIdent("м"), CompLt, e)
		want := CanonicalTriggerCondition(spec)
		got := "metric|м|" + CompLt.String() + "|" + CanonicalExpression(e)
		if got != want {
			t.Errorf("CanonicalExpression разошлась с canonExpr: %q vs %q", got, want)
		}
	}
}

// TestCanonicalExpressionNilGuard — nil-гард: отсутствующий необязательный атрибут
// метрики/шага (где/период/по_дате/исполнитель/срок) приходит как nil Expression и
// ШТАТНО даёт пустую строку, а не панику. Мутпроба: снятие гарда роняет тест паникой.
func TestCanonicalExpressionNilGuard(t *testing.T) {
	if got := CanonicalExpression(nil); got != "" {
		t.Errorf("CanonicalExpression(nil) = %q, ожидалась пустая строка", got)
	}
}
