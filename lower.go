package ladix

import (
	"strings"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/ir"
)

// lowerProgram понижает разобранный и семантически проверенный AST в стабильный
// IR (contracts/ir-schema.md). Выражения представлены КАНОНИЧЕСКИМИ СТРОКАМИ
// (ast.CanonicalExpression / ast.CanonicalStatement) — текстонезависимой записью
// смысла: эквивалентные по смыслу записи дают одинаковую строку.
//
// Коллекции идут в порядке объявления в исходнике. Декларации, не имеющие
// представления в IR v1 (источники, функции, свободные операторы), пропускаются:
// потребитель (вариант B) исполняет их нативно и получает из IR только
// определения метрик, процессов и триггеров.
func lowerProgram(prog *ast.Program) *ir.Program {
	out := &ir.Program{
		SchemaVersion: ir.SchemaVersion,
		Metrics:       []ir.Metric{},
		Processes:     []ir.Process{},
		Triggers:      []ir.Trigger{},
	}
	for _, item := range prog.Items {
		switch d := item.(type) {
		case *ast.MetricDecl:
			out.Metrics = append(out.Metrics, lowerMetric(d))
		case *ast.ProcessDecl:
			out.Processes = append(out.Processes, lowerProcess(d))
		case *ast.TriggerDecl:
			out.Triggers = append(out.Triggers, lowerTrigger(d))
		}
	}
	return out
}

func lowerMetric(d *ast.MetricDecl) ir.Metric {
	return ir.Metric{
		Name:      d.Name.Name,
		Source:    d.Source.Name,
		Where:     ast.CanonicalExpression(d.Where),
		Aggregate: ast.CanonicalExpression(d.Aggregate),
		Period:    ast.CanonicalExpression(d.Period),
		ByDate:    ast.CanonicalExpression(d.ByDate),
		Pos:       lowerPos(d.Pos()),
	}
}

func lowerProcess(d *ast.ProcessDecl) ir.Process {
	params := make([]string, len(d.Params))
	for i, p := range d.Params {
		params[i] = p.Name
	}
	steps := make([]ir.Step, len(d.Steps))
	for i, s := range d.Steps {
		steps[i] = lowerStep(s)
	}
	return ir.Process{
		Name:   d.Name.Name,
		Params: params,
		Steps:  steps,
		Pos:    lowerPos(d.Pos()),
	}
}

func lowerStep(s *ast.StepDecl) ir.Step {
	after := make([]string, len(s.After))
	for i, a := range s.After {
		after[i] = a.Name
	}
	actions := make([]string, len(s.Body))
	for i, st := range s.Body {
		actions[i] = ast.CanonicalStatement(st)
	}
	return ir.Step{
		Name:     s.Name.Name,
		After:    strings.Join(after, ", "),
		Assignee: ast.CanonicalExpression(s.Assignee),
		Deadline: ast.CanonicalExpression(s.Deadline),
		Actions:  actions,
		Pos:      lowerPos(s.Pos()),
	}
}

// lowerTrigger понижает триггер, дискриминируя вид по конкретному типу формы.
// Поля, неприменимые к виду, остаются пустыми (контракт ir-schema.md).
// Незнакомая форма — паника: тотальность, как у канонизаторов ast (Конституция III);
// recover-барьер фасада превратит её в err, а не в краш потребителя.
func lowerTrigger(d *ast.TriggerDecl) ir.Trigger {
	t := ir.Trigger{Pos: lowerPos(d.Pos())}
	switch s := d.Spec.(type) {
	case *ast.MetricTrigger:
		t.Kind = ir.KindMetric
		t.Metric = s.Metric.Name
		t.Op = s.Op.String()
		t.Threshold = ast.CanonicalExpression(s.Threshold)
	case *ast.ScheduleTrigger:
		t.Kind = ir.KindSchedule
		t.Schedule = ast.CanonicalTriggerCondition(s)
	case *ast.EventTrigger:
		t.Kind = ir.KindEvent
		t.Event = s.Event.Name
	case *ast.DeadlineTrigger:
		t.Kind = ir.KindDeadline
		t.Process = s.Process.Name
		t.Step = s.Step.Name
	default:
		panic("lowerTrigger: незнакомая форма триггера")
	}
	return t
}

// lowerPos переносит позицию покомпонентно: ir.Position — СОБСТВЕННЫЙ тип пакета
// ir (дубль, не разделение), что сохраняет ir листовым (Конституция IV/VII).
func lowerPos(p ast.Position) ir.Position {
	return ir.Position{Line: p.Line, Col: p.Col}
}
