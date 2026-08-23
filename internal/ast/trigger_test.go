package ast

import "testing"

// T005: триггер-узлы (007a §TR-2, contracts/trigger-ast.md). Компайл-тайм
// маркеры участия в union'ах: TriggerDecl — top-level декларация (Decl +
// TopLevelItem); три формы — TriggerSpec; две подформы — ScheduleSpec.

var (
	_ Decl         = (*TriggerDecl)(nil)
	_ TopLevelItem = (*TriggerDecl)(nil)

	_ TriggerSpec = (*MetricTrigger)(nil)
	_ TriggerSpec = (*EventTrigger)(nil)
	_ TriggerSpec = (*ScheduleTrigger)(nil)
	_ TriggerSpec = (*DeadlineTrigger)(nil)

	_ ScheduleSpec = (*EverySchedule)(nil)
	_ ScheduleSpec = (*AtSchedule)(nil)
)

func TestTriggerDeclPos(t *testing.T) {
	whenPos := Position{Line: 1, Col: 1}
	metricPos := Position{Line: 1, Col: 7}
	spec := NewMetricTrigger(metricPos,
		*NewIdent(Position{Line: 1, Col: 15}, "выручка"),
		CompGt, NewIntLit(Position{Line: 1, Col: 25}, 1000))
	body := NewBlock(Position{Line: 2, Col: 5}, []Statement{
		NewAssignAction(Position{Line: 2, Col: 5},
			*NewIdent(Position{Line: 2, Col: 15}, "x"),
			NewIntLit(Position{Line: 2, Col: 19}, 1)),
	})
	td := NewTriggerDecl(whenPos, spec, body)

	if td.Pos() != whenPos {
		t.Errorf("TriggerDecl.Pos() = %+v, хотим токен когда %+v", td.Pos(), whenPos)
	}
	if td.Spec != TriggerSpec(spec) {
		t.Errorf("TriggerDecl.Spec заполнено неверно: %+v", td.Spec)
	}
	if td.Body != body {
		t.Errorf("TriggerDecl.Body заполнено неверно: %+v", td.Body)
	}
	var _ Decl = td
	var _ TopLevelItem = td
}

func TestMetricTriggerPos(t *testing.T) {
	metricPos := Position{Line: 1, Col: 7}
	threshold := NewIntLit(Position{Line: 1, Col: 25}, 1000)
	mt := NewMetricTrigger(metricPos,
		*NewIdent(Position{Line: 1, Col: 15}, "выручка"),
		CompGt, threshold)

	if mt.Pos() != metricPos {
		t.Errorf("MetricTrigger.Pos() = %+v, хотим токен метрика %+v", mt.Pos(), metricPos)
	}
	if mt.Metric.Name != "выручка" {
		t.Errorf("MetricTrigger.Metric = %q, хотим \"выручка\"", mt.Metric.Name)
	}
	if mt.Op != CompGt {
		t.Errorf("MetricTrigger.Op = %v, хотим CompGt", mt.Op)
	}
	if mt.Threshold != Expression(threshold) {
		t.Errorf("MetricTrigger.Threshold заполнено неверно: %+v", mt.Threshold)
	}
	var _ TriggerSpec = mt
}

func TestEventTriggerPos(t *testing.T) {
	evtPos := Position{Line: 1, Col: 7}
	et := NewEventTrigger(evtPos, *NewIdent(Position{Line: 1, Col: 16}, "заказ_создан"))

	if et.Pos() != evtPos {
		t.Errorf("EventTrigger.Pos() = %+v, хотим токен событие %+v", et.Pos(), evtPos)
	}
	if et.Event.Name != "заказ_создан" {
		t.Errorf("EventTrigger.Event = %q, хотим \"заказ_создан\"", et.Event.Name)
	}
	var _ TriggerSpec = et
}

func TestScheduleTriggerPos(t *testing.T) {
	schedPos := Position{Line: 1, Col: 7}
	everyPos := Position{Line: 1, Col: 20}
	every := NewEverySchedule(everyPos, NewDurationLit(Position{Line: 1, Col: 28}, "3", "дн"))
	st := NewScheduleTrigger(schedPos, every)

	if st.Pos() != schedPos {
		t.Errorf("ScheduleTrigger.Pos() = %+v, хотим токен расписание %+v", st.Pos(), schedPos)
	}
	if st.Spec != ScheduleSpec(every) {
		t.Errorf("ScheduleTrigger.Spec заполнено неверно: %+v", st.Spec)
	}
	var _ TriggerSpec = st
}

// T003 (016 B4a, §AU-6.1.1): эскалация-триггер — узел DeadlineTrigger{Process, Step}.
// Pos() = ведущий токен формы (IDENT-лексема «задача»). Реализует TriggerSpec, как
// прочие три формы.
func TestDeadlineTriggerNode(t *testing.T) {
	taskPos := Position{Line: 1, Col: 7}
	dt := NewDeadlineTrigger(taskPos,
		*NewIdent(Position{Line: 1, Col: 27}, "согласование"),
		*NewIdent(Position{Line: 1, Col: 40}, "проверка"))

	if dt.Pos() != taskPos {
		t.Errorf("DeadlineTrigger.Pos() = %+v, хотим токен задача %+v", dt.Pos(), taskPos)
	}
	if dt.Process.Name != "согласование" {
		t.Errorf("DeadlineTrigger.Process = %q, хотим \"согласование\"", dt.Process.Name)
	}
	if dt.Step.Name != "проверка" {
		t.Errorf("DeadlineTrigger.Step = %q, хотим \"проверка\"", dt.Step.Name)
	}
	var _ TriggerSpec = dt
}

func TestEverySchedulePos(t *testing.T) {
	everyPos := Position{Line: 1, Col: 20}
	dur := NewDurationLit(Position{Line: 1, Col: 28}, "5", "мин")
	es := NewEverySchedule(everyPos, dur)

	if es.Pos() != everyPos {
		t.Errorf("EverySchedule.Pos() = %+v, хотим токен каждые %+v", es.Pos(), everyPos)
	}
	if es.Every != dur {
		t.Errorf("EverySchedule.Every заполнено неверно: %+v", es.Every)
	}
	if es.Every.Amount != "5" || es.Every.Unit != "мин" {
		t.Errorf("EverySchedule.Every = %+v, хотим {5 мин}", es.Every)
	}
	var _ ScheduleSpec = es
}

func TestAtSchedulePos(t *testing.T) {
	atPos := Position{Line: 1, Col: 20}
	as := NewAtSchedule(atPos, *NewStringLit(Position{Line: 1, Col: 23}, "09:00"))

	if as.Pos() != atPos {
		t.Errorf("AtSchedule.Pos() = %+v, хотим токен в %+v", as.Pos(), atPos)
	}
	if as.At.Value != "09:00" {
		t.Errorf("AtSchedule.At.Value = %q, хотим \"09:00\"", as.At.Value)
	}
	var _ ScheduleSpec = as
}
