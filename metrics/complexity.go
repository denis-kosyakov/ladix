package metrics

// Потолок сложности выражений (design.md Д-4, spec.md Requirement «Потолок
// сложности выражений»). Пределы: глубина > 100 ИЛИ число узлов > 10000 → отказ,
// ДО вычисления, на разобранном expr-AST. Определения СОВПАДАЮТ с
// metrics/complexity_test.go (astNodeCount/astDepth): УЗЕЛ — каждый узел
// ast.Expression (CallExpr считает Callee отдельным узлом; ListLit — каждый
// элемент; листья = 1 узел); ГЛУБИНА — длина самого длинного пути корень→лист,
// корень = 1.
//
// Обход ИСЧЕРПЫВАЮЩ по всем 19 конкретным типам ast.Expression (сверено с
// тотальным switch internal/ast/canon.go). Типы, недостижимые из где:/агрегат:/
// период:/по_дате: валидной грамматики (RunProcessExpr/CallExternalExpr/
// ValueExpr/EventExpr/IndexExpr/FieldExpr/DurationLit) обработаны защитно по той
// же формуле (1 + сумма детей), а не паникой — фасад не должен падать на
// синтаксически валидном, но семантически необычном входе.

import (
	"fmt"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/ir"
)

// complexityDepthLimit/complexityNodeLimit — пределы potolка (spec.md, дословно).
const (
	complexityDepthLimit = 100
	complexityNodeLimit  = 10000
)

// exprNodeCount — определение УЗЛА, см. комментарий файла.
func exprNodeCount(e ast.Expression) int {
	switch n := e.(type) {
	case *ast.BinaryExpr:
		return 1 + exprNodeCount(n.Left) + exprNodeCount(n.Right)
	case *ast.UnaryExpr:
		return 1 + exprNodeCount(n.Operand)
	case *ast.CallExpr:
		total := 1 + exprNodeCount(n.Callee)
		for _, a := range n.Args {
			total += exprNodeCount(a)
		}
		return total
	case *ast.ListLit:
		total := 1
		for _, el := range n.Elements {
			total += exprNodeCount(el)
		}
		return total
	case *ast.IndexExpr:
		return 1 + exprNodeCount(n.Target) + exprNodeCount(n.Index)
	case *ast.FieldExpr:
		return 1 + exprNodeCount(n.Target)
	case *ast.RunProcessExpr:
		total := 1
		for _, a := range n.Args {
			total += exprNodeCount(a)
		}
		return total
	case *ast.CallExternalExpr:
		total := 1
		for _, a := range n.Args {
			total += exprNodeCount(a)
		}
		return total
	case *ast.Ident, *ast.IntLit, *ast.FloatLit, *ast.StringLit, *ast.BoolLit, *ast.NoneLit,
		*ast.DurationLit, *ast.WindowPeriodLit, *ast.LastCompletedPeriodLit,
		*ast.ValueExpr, *ast.EventExpr:
		return 1
	default:
		return 1
	}
}

// exprDepth — определение ГЛУБИНЫ, см. комментарий файла; корень = глубина 1.
func exprDepth(e ast.Expression) int {
	switch n := e.(type) {
	case *ast.BinaryExpr:
		return 1 + maxInt(exprDepth(n.Left), exprDepth(n.Right))
	case *ast.UnaryExpr:
		return 1 + exprDepth(n.Operand)
	case *ast.CallExpr:
		max := exprDepth(n.Callee)
		for _, a := range n.Args {
			if d := exprDepth(a); d > max {
				max = d
			}
		}
		return 1 + max
	case *ast.ListLit:
		max := 0
		for _, el := range n.Elements {
			if d := exprDepth(el); d > max {
				max = d
			}
		}
		return 1 + max
	case *ast.IndexExpr:
		return 1 + maxInt(exprDepth(n.Target), exprDepth(n.Index))
	case *ast.FieldExpr:
		return 1 + exprDepth(n.Target)
	case *ast.RunProcessExpr:
		max := 0
		for _, a := range n.Args {
			if d := exprDepth(a); d > max {
				max = d
			}
		}
		return 1 + max
	case *ast.CallExternalExpr:
		max := 0
		for _, a := range n.Args {
			if d := exprDepth(a); d > max {
				max = d
			}
		}
		return 1 + max
	default:
		return 1 // лист: Ident/IntLit/FloatLit/StringLit/BoolLit/NoneLit/DurationLit/
		// WindowPeriodLit/LastCompletedPeriodLit/ValueExpr/EventExpr
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// checkComplexity обходит все 4 присутствующих атрибута метрики md в порядке
// где,агрегат,период,по_дате (design.md) и возвращает диагностику потолка на
// ПЕРВОМ найденном нарушении (глубина, затем число узлов на том же атрибуте).
// over == false → пределы не превышены ни одним атрибутом.
func (t *template) checkComplexity(metricName string, md *ast.MetricDecl) (ir.Diagnostic, bool) {
	exprs := [4]ast.Expression{md.Where, md.Aggregate, md.Period, md.ByDate}
	for i, expr := range exprs {
		slot := t.attrs[i]
		if !slot.present || expr == nil {
			continue
		}
		if d := exprDepth(expr); d > complexityDepthLimit {
			return ir.Diagnostic{
				Severity: ir.SeverityError,
				Stage:    stageRuntime,
				Message: fmt.Sprintf("метрика '%s': %s: выражение слишком сложное — глубина вложенности %d превышает предел %d",
					metricName, slot.name, d, complexityDepthLimit),
				Pos: canonPos(slot, position{Line: expr.Pos().Line, Col: expr.Pos().Col}),
			}, true
		}
		if n := exprNodeCount(expr); n > complexityNodeLimit {
			return ir.Diagnostic{
				Severity: ir.SeverityError,
				Stage:    stageRuntime,
				Message: fmt.Sprintf("метрика '%s': %s: выражение слишком сложное — число узлов %d превышает предел %d",
					metricName, slot.name, n, complexityNodeLimit),
				Pos: canonPos(slot, position{Line: expr.Pos().Line, Col: expr.Pos().Col}),
			}, true
		}
	}
	return ir.Diagnostic{}, false
}
