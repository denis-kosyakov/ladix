package ast

import (
	"fmt"
	"strconv"
	"strings"
)

// CanonicalTriggerCondition строит каноническую строку условия триггера (§FR-002):
// текстонезависимое представление СМЫСЛА триггера, по которому минтится стабильный
// durable-ключ (buildTriggerKeys, EM-17.2.1). Эквивалентные по смыслу записи (разный
// формат числа, пробелы) дают одинаковую строку; различие имени/оператора/порога —
// разную. Канон НЕ зависит от позиции объявления и от исходного форматирования.
//
// Ключевые виды (метрика/расписание) возвращают непустой канон с префиксом-дискриминантом
// («metric|»/«every|»/«at|»). Не-ключевые виды (событие/дедлайн) durable-ключа не имеют —
// возвращают пустую строку как сигнал «не-ключевой» (buildTriggerKeys пропускает слот).
// Пустота однозначна: ключевые виды всегда непусты из-за префикса.
func CanonicalTriggerCondition(spec TriggerSpec) string {
	switch s := spec.(type) {
	case *MetricTrigger:
		return "metric|" + s.Metric.Name + "|" + s.Op.String() + "|" + canonExpr(s.Threshold)
	case *ScheduleTrigger:
		switch sub := s.Spec.(type) {
		case *EverySchedule:
			return "every|" + canonDuration(sub.Every)
		case *AtSchedule:
			return "at|" + sub.At.Value
		default:
			panic(fmt.Sprintf("CanonicalTriggerCondition: незнакомый ScheduleSpec %T", s.Spec))
		}
	case *EventTrigger, *DeadlineTrigger:
		return "" // не-ключевой триггер: durable-ключа нет
	default:
		panic(fmt.Sprintf("CanonicalTriggerCondition: незнакомый TriggerSpec %T", spec))
	}
}

// canonDuration канонизирует литерал длительности (величина|единица). Величина —
// уже нормализованная лексема (Amount), единица — нормализованная форма (Unit).
func canonDuration(d *DurationLit) string {
	return "длит(" + d.Amount + "|" + d.Unit + ")"
}

// canonExpr — тотальный рекурсивный канонизатор выражений (§FR-003). Покрывает ВСЕ
// 19 конкретных типов Expression; неизвестный тип — ПАНИКА (а не молчаливый default),
// чтобы новый узел AST не «провалился» в неопределённое поведение durable-ключа.
// Замок T1 (canon_test.go) проверяет исчерпываемость: удаление любой ветки уводит её
// тип в default-панику и краснит тест.
func canonExpr(e Expression) string {
	switch v := e.(type) {
	case *IntLit:
		return strconv.FormatInt(v.Value, 10)
	case *FloatLit:
		// КАНОН: кратчайшая round-trip-форма ('g', -1) — зафиксировано, стабильно.
		return strconv.FormatFloat(v.Value, 'g', -1, 64)
	case *StringLit:
		return strconv.Quote(v.Value)
	case *BoolLit:
		return strconv.FormatBool(v.Value)
	case *NoneLit:
		return "пусто"
	case *DurationLit:
		return canonDuration(v)
	case *WindowPeriodLit:
		return "окно(" + v.Amount + "|" + v.Unit + ")"
	case *LastCompletedPeriodLit:
		return "прошлый(" + v.Noun + ")"
	case *ListLit:
		return "[" + strings.Join(mapCanon(v.Elements), ",") + "]"
	case *Ident:
		return v.Name
	case *BinaryExpr:
		return "(" + canonExpr(v.Left) + " " + v.Op.String() + " " + canonExpr(v.Right) + ")"
	case *UnaryExpr:
		return "(" + v.Op.String() + canonExpr(v.Operand) + ")"
	case *CallExpr:
		return canonExpr(v.Callee) + "(" + strings.Join(mapCanon(v.Args), ",") + ")"
	case *IndexExpr:
		return canonExpr(v.Target) + "[" + canonExpr(v.Index) + "]"
	case *FieldExpr:
		return canonExpr(v.Target) + "." + v.Field.Name
	case *RunProcessExpr:
		return "запустить(" + v.Process.Name + "|" + strings.Join(mapCanon(v.Args), ",") + ")"
	case *CallExternalExpr:
		return "вызвать(" + v.Target.Name + "|" + strings.Join(mapCanon(v.Args), ",") + ")"
	case *ValueExpr:
		return "значение"
	case *EventExpr:
		return "событие"
	default:
		panic(fmt.Sprintf("canonExpr: незнакомый тип выражения %T", e))
	}
}

// mapCanon канонизирует срез выражений в срез строк (для списков/аргументов).
func mapCanon(es []Expression) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = canonExpr(e)
	}
	return out
}
