package metrics

// Задача 2.5 — Потолок сложности выражений (design.md Д-4, spec.md
// Requirement «Потолок сложности выражений»).
//
// ОПРЕДЕЛЕНИЯ, ЗАФИКСИРОВАННЫЕ ЭТИМ ФАЙЛОМ (реализатор ОБЯЗАН считать так же,
// см. astNodeCount/astDepth ниже — это буквальный код формулы, не пересказ):
//
//   - УЗЕЛ — каждый узел ast.Expression в разобранном дереве выражения
//     (обходом по конкретным типам: BinaryExpr, UnaryExpr, CallExpr — включая
//     Callee как отдельный узел, ListLit — включая каждый элемент, и листья
//     Ident/IntLit/FloatLit/StringLit/BoolLit/NoneLit, каждый лист = 1 узел).
//     Скобки группировки узла не образуют (в ast нет ParenExpr — печать
//     скобок чисто канонизатора). Корень выражения считается.
//   - ГЛУБИНА — длина самого длинного пути от корня выражения до листа;
//     корень (даже если он сам лист) = глубина 1. Для бинарной/унарной/
//     вызова/списка — 1 + максимум глубины детей.
//
// Оба предела (глубина > 100, число узлов > 10000) — до вычисления, на
// разобранном expr-AST (design.md Д-4). Тексты — дословно из spec.md.
//
// Тестовые выражения строятся программно и САМОПРОВЕРЯЮТСЯ через тот же
// walker (astNodeCount/astDepth) над РЕАЛЬНО РАЗОБРАННЫМ деревом
// (internal/lexer+internal/parser через синтетическую декларацию метрики,
// design.md Д-10) — числа в именах тестов гарантированно точны, не оценка на
// глаз.

import (
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
)

// astNodeCount — определение УЗЛА (см. комментарий файла). Паникует на
// незнакомом типе узла: список типов ниже ИСЧЕРПЫВАЮЩ для конструкций,
// которые строит этот файл (Binary/Unary/Call/List/литералы/Ident); если
// реализатору нужно больше — формулу придётся расширить симметрично.
func astNodeCount(e ast.Expression) int {
	switch n := e.(type) {
	case *ast.BinaryExpr:
		return 1 + astNodeCount(n.Left) + astNodeCount(n.Right)
	case *ast.UnaryExpr:
		return 1 + astNodeCount(n.Operand)
	case *ast.CallExpr:
		total := 1 + astNodeCount(n.Callee)
		for _, a := range n.Args {
			total += astNodeCount(a)
		}
		return total
	case *ast.ListLit:
		total := 1
		for _, el := range n.Elements {
			total += astNodeCount(el)
		}
		return total
	case *ast.Ident, *ast.IntLit, *ast.FloatLit, *ast.StringLit, *ast.BoolLit, *ast.NoneLit:
		return 1
	default:
		panic("astNodeCount: неизвестный узел, обнови формулу")
	}
}

// astDepth — определение ГЛУБИНЫ (см. комментарий файла); корень = глубина 1.
func astDepth(e ast.Expression) int {
	switch n := e.(type) {
	case *ast.BinaryExpr:
		l, r := astDepth(n.Left), astDepth(n.Right)
		if l > r {
			return 1 + l
		}
		return 1 + r
	case *ast.UnaryExpr:
		return 1 + astDepth(n.Operand)
	case *ast.CallExpr:
		max := astDepth(n.Callee)
		for _, a := range n.Args {
			if d := astDepth(a); d > max {
				max = d
			}
		}
		return 1 + max
	case *ast.ListLit:
		max := 0
		for _, el := range n.Elements {
			if d := astDepth(el); d > max {
				max = d
			}
		}
		return 1 + max
	default:
		return 1 // лист
	}
}

// parseAttrExpr разбирает exprText как атрибут attr ("где"/"агрегат")
// синтетической минимальной декларации метрики (design.md Д-10 — та же
// техника, которую использует фасад: parseMetricDecl достижим только через
// декларацию метрики целиком). Имя метрики/источника фиксированное и
// безопасное — вход потребителя попадает только в тело атрибута.
func parseAttrExpr(t *testing.T, attr, exprText string) ast.Expression {
	t.Helper()
	src := "метрика _m_:\n    " + attr + ": " + exprText + "\n"
	toks, errList := lexer.New(src).Tokenize()
	prog := parser.New(toks, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("разбор синтетической декларации не удался: %v", errList.Error())
	}
	md, ok := prog.Items[0].(*ast.MetricDecl)
	if !ok {
		t.Fatalf("ожидалась *ast.MetricDecl, получено %T", prog.Items[0])
	}
	switch attr {
	case "где":
		return md.Where
	case "агрегат":
		return md.Aggregate
	default:
		t.Fatalf("неизвестный атрибут %q в тесте", attr)
		return nil
	}
}

// buildDepthWhere строит "не не не … истина" (N = depth-1 повторов "не") —
// UnaryExpr-цепочка глубины ровно depth, узлов ровно depth (по формуле выше:
// (depth-1) UnaryExpr + 1 лист BoolLit = depth узлов), проверено самопроверкой
// через astDepth на РЕАЛЬНО разобранном дереве.
func buildDepthWhere(t *testing.T, depth int) string {
	t.Helper()
	if depth < 1 {
		t.Fatalf("depth должен быть >= 1, получено %d", depth)
	}
	expr := strings.Repeat("не ", depth-1) + "истина"
	got := astDepth(parseAttrExpr(t, "где", expr))
	if got != depth {
		t.Fatalf("самопроверка глубины: построили %q, реальная глубина = %d, хотели %d", expr, got, depth)
	}
	return expr
}

// buildNodeCountAggregate строит "количество([1,1,…,1]) > 0" (K единиц в
// списке), число узлов = K + 5: BinaryExpr(1) + CallExpr(1) + Callee-Ident(1)
// + ListLit(1) + K*IntLit + правый IntLit(1) = K+5. Глубина этой конструкции
// (4) заведомо далека от предела 100 — изолирует проверку числа узлов от
// проверки глубины. Проверено самопроверкой через astNodeCount.
func buildNodeCountAggregate(t *testing.T, targetNodes int) string {
	t.Helper()
	k := targetNodes - 5
	if k < 1 {
		t.Fatalf("targetNodes слишком мал для этой конструкции: %d", targetNodes)
	}
	elems := make([]string, k)
	for i := range elems {
		elems[i] = "1"
	}
	expr := "количество([" + strings.Join(elems, ",") + "]) > 0"
	got := astNodeCount(parseAttrExpr(t, "агрегат", expr))
	if got != targetNodes {
		t.Fatalf("самопроверка числа узлов: K=%d дал %d узлов, хотели %d — формула K+5 разошлась с реальным AST", k, got, targetNodes)
	}
	return expr
}

// TestComplexityDepthAt100Passes — глубина ровно 100 не должна давать
// диагностику потолка глубины (design.md Д-4: "проходит"). Полного успешного
// вычисления не требуется — важно лишь отсутствие ИМЕННО этой диагностики.
func TestComplexityDepthAt100Passes(t *testing.T) {
	where := buildDepthWhere(t, 100)
	m := baseMetric(where, "количество(ид)", "", "")
	_, diags, _ := Evaluate(m, salesRecords(), Options{Today: may31()})
	for _, d := range diags {
		if strings.Contains(d.Message, "выражение слишком сложное") {
			t.Errorf("глубина 100 отклонена потолком сложности: %q", d.Message)
		}
	}
}

// TestComplexityDepthAt101Fails — глубина 101 → дословный текст потолка
// глубины (spec.md).
func TestComplexityDepthAt101Fails(t *testing.T) {
	where := buildDepthWhere(t, 101)
	m := baseMetric(where, "количество(ид)", "", "")
	_, diags, err := Evaluate(m, salesRecords(), Options{Today: may31()})
	if err == nil {
		t.Fatalf("err == nil, ожидалась ошибка (глубина 101 > 100)")
	}
	wantMsg := "метрика 'м': где: выражение слишком сложное — глубина вложенности 101 превышает предел 100"
	found := false
	for _, d := range diags {
		if d.Message == wantMsg {
			found = true
			if d.Severity != "error" {
				t.Errorf("Severity = %q, хотим %q", d.Severity, "error")
			}
			if d.Stage != "runtime" {
				t.Errorf("Stage = %q, хотим %q", d.Stage, "runtime")
			}
		}
	}
	if !found {
		t.Errorf("не найдена диагностика с текстом %q среди %+v", wantMsg, diags)
	}
}

// TestComplexityNodesAt10000Passes — 10000 узлов не должны давать
// диагностику потолка числа узлов.
func TestComplexityNodesAt10000Passes(t *testing.T) {
	aggregate := buildNodeCountAggregate(t, 10000)
	m := baseMetric("", aggregate, "", "")
	_, diags, _ := Evaluate(m, salesRecords(), Options{Today: may31()})
	for _, d := range diags {
		if strings.Contains(d.Message, "выражение слишком сложное") {
			t.Errorf("10000 узлов отклонено потолком сложности: %q", d.Message)
		}
	}
}

// TestComplexityNodesAt10001Fails — 10001 узел → дословный текст потолка
// числа узлов (spec.md).
func TestComplexityNodesAt10001Fails(t *testing.T) {
	aggregate := buildNodeCountAggregate(t, 10001)
	m := baseMetric("", aggregate, "", "")
	_, diags, err := Evaluate(m, salesRecords(), Options{Today: may31()})
	if err == nil {
		t.Fatalf("err == nil, ожидалась ошибка (10001 узел > 10000)")
	}
	wantMsg := "метрика 'м': агрегат: выражение слишком сложное — число узлов 10001 превышает предел 10000"
	found := false
	for _, d := range diags {
		if d.Message == wantMsg {
			found = true
			if d.Severity != "error" {
				t.Errorf("Severity = %q, хотим %q", d.Severity, "error")
			}
			if d.Stage != "runtime" {
				t.Errorf("Stage = %q, хотим %q", d.Stage, "runtime")
			}
		}
	}
	if !found {
		t.Errorf("не найдена диагностика с текстом %q среди %+v", wantMsg, diags)
	}
}

// --- Исчерпываемость обхода потолка ---------------------------------------
//
// 🔁 ИНВЕРСИОННЫЙ ЗАМОК (аналог ast.TestCanonExprExhaustive): exprDepth/
// exprNodeCount имеют default-ветку, поэтому пропущенный тип узла НЕ падает, а
// молча считается ЛИСТОМ (глубина 1, узлов 1) — и потолок обходится сколь угодно
// глубоким деревом такого типа. Таблица содержит по одному экземпляру КАЖДОГО из
// 19 конкретных ast.Expression; у составных типов ребёнок нетривиален, поэтому
// удаление любой ветки составного типа из exprDepth/exprNodeCount немедленно даёт
// глубину/узлы = 1 и краснит соответствующий кейс.
func TestComplexityWalkExhaustive(t *testing.T) {
	p := ast.Position{Line: 1, Col: 1}
	child := func() ast.Expression { return ast.NewIdent(p, "x") }

	cases := []struct {
		name      string
		expr      ast.Expression
		wantDepth int
		wantNodes int
	}{
		// 11 листьев: глубина 1, узлов 1.
		{"IntLit", ast.NewIntLit(p, 42), 1, 1},
		{"FloatLit", ast.NewFloatLit(p, 1.5), 1, 1},
		{"StringLit", ast.NewStringLit(p, "привет"), 1, 1},
		{"BoolLit", ast.NewBoolLit(p, true), 1, 1},
		{"NoneLit", ast.NewNoneLit(p), 1, 1},
		{"DurationLit", ast.NewDurationLit(p, "3", "дн"), 1, 1},
		{"WindowPeriodLit", ast.NewWindowPeriodLit(p, "7", "дн"), 1, 1},
		{"LastCompletedPeriodLit", ast.NewLastCompletedPeriodLit(p, "месяц"), 1, 1},
		{"Ident", ast.NewIdent(p, "выручка"), 1, 1},
		{"ValueExpr", ast.NewValueExpr(p), 1, 1},
		{"EventExpr", ast.NewEventExpr(p), 1, 1},
		// 8 составных: ребёнок обязан быть УЧТЁН (глубина ≥ 2, узлов ≥ 2).
		{"BinaryExpr", ast.NewBinaryExpr(p, ast.OpAdd, child(), child()), 2, 3},
		{"UnaryExpr", ast.NewUnaryExpr(p, ast.OpNeg, child()), 2, 2},
		{"CallExpr", ast.NewCallExpr(ast.NewIdent(p, "f"), []ast.Expression{child()}), 2, 3},
		{"ListLit", ast.NewListLit(p, []ast.Expression{child()}), 2, 2},
		{"IndexExpr", ast.NewIndexExpr(ast.NewIdent(p, "xs"), child()), 2, 3},
		{"FieldExpr", ast.NewFieldExpr(child(), *ast.NewIdent(p, "поле")), 2, 2},
		{"RunProcessExpr", ast.NewRunProcessExpr(p, *ast.NewIdent(p, "проц"), []ast.Expression{child()}), 2, 2},
		{"CallExternalExpr", ast.NewCallExternalExpr(p, *ast.NewIdent(p, "crm"), []ast.Expression{child()}), 2, 2},
	}

	if len(cases) != 19 {
		t.Fatalf("таблица исчерпываемости: типов %d, хотим 19 (все конкретные ast.Expression)", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if d := exprDepth(tc.expr); d != tc.wantDepth {
				t.Errorf("exprDepth = %d, хотим %d (ветка типа выпала в default → узел считается листом, потолок обходится)", d, tc.wantDepth)
			}
			if n := exprNodeCount(tc.expr); n != tc.wantNodes {
				t.Errorf("exprNodeCount = %d, хотим %d (ветка типа выпала в default → потолок обходится)", n, tc.wantNodes)
			}
		})
	}
}
