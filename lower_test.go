package ladix

import (
	"testing"

	"github.com/denis-kosyakov/ladix/ir"
)

// lowerSource — исходник, покрывающий ВСЕ сущности IR v1: метрику со всеми
// атрибутами, процесс с параметром и двумя шагами (второй — с после/исполнитель/
// срок и телом действий) и все ЧЕТЫРЕ вида триггера.
const lowerSource = `источник заказы:
    файл: "data/sales.json"

метрика выручка:
    источник: заказы
    где:      статус == "оплачен"
    агрегат:  сумма(сумма_заказа)
    период:   ежемесячно
    по_дате:  дата_заказа

процесс обработка(заказ):
    шаг принять:
        уведомить менеджер(заказ)

    шаг подтвердить после принять:
        исполнитель: "менеджер"
        срок: 2дн
        присвоить статус = "готов"

когда метрика выручка < 1000000:
    уведомить директор("падение")

когда расписание каждые 3дн:
    уведомить директор("отчёт")

когда событие оплата:
    уведомить директор("оплата")

когда задача просрочена в обработка.подтвердить:
    уведомить директор("просрочка")
`

func compileForLowering(t *testing.T) *ir.Program {
	t.Helper()
	program, diags, err := Compile(lowerSource)
	if err != nil {
		t.Fatalf("внутренний сбой: %v", err)
	}
	if program == nil {
		t.Fatalf("фикстура понижения обязана компилироваться; диагностики: %+v", diags)
	}
	return program
}

// TestLowerMetric — понижение метрики: имя/источник + канонические строки всех
// четырёх выражений + позиция объявления.
func TestLowerMetric(t *testing.T) {
	p := compileForLowering(t)
	if len(p.Metrics) != 1 {
		t.Fatalf("ожидалась 1 метрика, получено %d", len(p.Metrics))
	}
	m := p.Metrics[0]
	if m.Name != "выручка" || m.Source != "заказы" {
		t.Errorf("имя/источник: %+v", m)
	}
	if m.Where != `(статус == "оплачен")` {
		t.Errorf("где = %q", m.Where)
	}
	if m.Aggregate != "сумма(сумма_заказа)" {
		t.Errorf("агрегат = %q", m.Aggregate)
	}
	if m.Period != "ежемесячно" {
		t.Errorf("период = %q", m.Period)
	}
	if m.ByDate != "дата_заказа" {
		t.Errorf("по_дате = %q", m.ByDate)
	}
	if m.Pos.Line != 4 || m.Pos.Col != 1 {
		t.Errorf("позиция метрики = %+v, ожидалась {4 1}", m.Pos)
	}
}

// TestLowerMetricOptionalAttrsEmpty — отсутствующие необязательные атрибуты
// дают ПУСТЫЕ строки, а не панику (nil-гард CanonicalExpression).
func TestLowerMetricOptionalAttrsEmpty(t *testing.T) {
	program, diags, err := Compile(validSource)
	if err != nil || program == nil {
		t.Fatalf("минимальная метрика обязана компилироваться: err=%v diags=%+v", err, diags)
	}
	m := program.Metrics[0]
	if m.Period != "" || m.ByDate != "" {
		t.Errorf("отсутствующие атрибуты обязаны быть пустыми строками: %+v", m)
	}
}

// TestLowerProcess — понижение процесса: параметры, порядок шагов, атрибуты и
// канонические строки действий тела.
func TestLowerProcess(t *testing.T) {
	p := compileForLowering(t)
	if len(p.Processes) != 1 {
		t.Fatalf("ожидался 1 процесс, получено %d", len(p.Processes))
	}
	proc := p.Processes[0]
	if proc.Name != "обработка" {
		t.Errorf("имя процесса = %q", proc.Name)
	}
	if len(proc.Params) != 1 || proc.Params[0] != "заказ" {
		t.Errorf("параметры = %+v", proc.Params)
	}
	if len(proc.Steps) != 2 {
		t.Fatalf("ожидалось 2 шага, получено %d", len(proc.Steps))
	}

	first := proc.Steps[0]
	if first.Name != "принять" || first.After != "" {
		t.Errorf("стартовый шаг: %+v", first)
	}
	if len(first.Actions) != 1 || first.Actions[0] != "уведомить менеджер(заказ)" {
		t.Errorf("действия стартового шага = %+v", first.Actions)
	}

	second := proc.Steps[1]
	if second.Name != "подтвердить" || second.After != "принять" {
		t.Errorf("второй шаг: %+v", second)
	}
	if second.Assignee != `"менеджер"` {
		t.Errorf("исполнитель = %q", second.Assignee)
	}
	if second.Deadline != "длит(2|дн)" {
		t.Errorf("срок = %q", second.Deadline)
	}
	if len(second.Actions) != 1 || second.Actions[0] != `присвоить статус = "готов"` {
		t.Errorf("действия второго шага = %+v", second.Actions)
	}
	if second.Pos.Line < 1 || second.Pos.Col < 1 {
		t.Errorf("позиция шага не заполнена: %+v", second.Pos)
	}
}

// TestLowerTriggers — понижение всех ЧЕТЫРЁХ видов триггера: дискриминант Kind,
// заполненные для вида поля и ПУСТЫЕ неприменимые.
func TestLowerTriggers(t *testing.T) {
	p := compileForLowering(t)
	if len(p.Triggers) != 4 {
		t.Fatalf("ожидалось 4 триггера, получено %d", len(p.Triggers))
	}

	metric := p.Triggers[0]
	if metric.Kind != ir.KindMetric || metric.Metric != "выручка" ||
		metric.Op != "<" || metric.Threshold != "1000000" {
		t.Errorf("метрик-триггер: %+v", metric)
	}
	if metric.Event != "" || metric.Schedule != "" {
		t.Errorf("неприменимые поля метрик-триггера обязаны быть пустыми: %+v", metric)
	}

	schedule := p.Triggers[1]
	if schedule.Kind != ir.KindSchedule || schedule.Schedule != "every|длит(3|дн)" {
		t.Errorf("расписание-триггер: %+v", schedule)
	}
	if schedule.Metric != "" || schedule.Event != "" {
		t.Errorf("неприменимые поля расписание-триггера обязаны быть пустыми: %+v", schedule)
	}

	event := p.Triggers[2]
	if event.Kind != ir.KindEvent || event.Event != "оплата" {
		t.Errorf("событие-триггер: %+v", event)
	}

	deadline := p.Triggers[3]
	if deadline.Kind != ir.KindDeadline ||
		deadline.Process != "обработка" || deadline.Step != "подтвердить" {
		t.Errorf("дедлайн-триггер: %+v", deadline)
	}

	for i, tr := range p.Triggers {
		if tr.Pos.Line < 1 || tr.Pos.Col < 1 {
			t.Errorf("позиция триггера #%d не заполнена: %+v", i, tr.Pos)
		}
	}
}

// TestLowerSkipsNonIRDeclarations — источники в IR v1 не представлены (потребитель
// подключает данные сам), и это НЕ ошибка: программа компилируется, коллекции IR
// содержат только метрики/процессы/триггеры.
func TestLowerSkipsNonIRDeclarations(t *testing.T) {
	p := compileForLowering(t)
	if len(p.Metrics) == 0 || len(p.Processes) == 0 || len(p.Triggers) == 0 {
		t.Fatalf("коллекции IR не заполнены: %+v", p)
	}
	if p.SchemaVersion != ir.SchemaVersion {
		t.Errorf("SchemaVersion = %d", p.SchemaVersion)
	}
}
