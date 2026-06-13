package ast

// TriggerDecl — объявление триггера верхнего уровня: «когда <Spec>: <Body>».
// Pos() = токен «когда». Spec — одна из трёх форм (MetricTrigger/EventTrigger/
// ScheduleTrigger). Body — индентный блок (тело триггера, императивное ядро).
// Зеркало ProcessDecl/FunctionDecl: встраивает declBase (→ реализует Decl и
// TopLevelItem). Pos() = токен «когда».
type TriggerDecl struct {
	declBase
	Spec TriggerSpec // форма триггера (метрика/событие/расписание)
	Body *Block      // тело: NEWLINE INDENT Statement+ DEDENT
}

// NewTriggerDecl строит объявление триггера; pos — позиция токена «когда».
func NewTriggerDecl(pos Position, spec TriggerSpec, body *Block) *TriggerDecl {
	return &TriggerDecl{declBase: declBase{base{pos}}, Spec: spec, Body: body}
}

// TriggerSpec — форма триггера. Реализуется MetricTrigger, EventTrigger,
// ScheduleTrigger. Маркер triggerSpec() — пустой метод (как stmtNode/exprNode).
// Дискриминация форм — Go type-switch по конкретному типу, без tag-полей.
type TriggerSpec interface {
	Node
	triggerSpec()
}

// specBase — база для форм триггера: Pos() (= ведущий токен формы) + маркер
// triggerSpec(). Вспомогательные узлы (встраивают base, как ElseClause).
type specBase struct{ base }

func (specBase) triggerSpec() {}

// MetricTrigger — «метрика Metric Op Threshold». Pos() = токен «метрика».
// Условие — ОДНО сравнение (ограничение v1). Метрика резолвится семпроходом
// против реестра i.metrics.
type MetricTrigger struct {
	specBase
	Metric    Ident      // имя метрики
	Op        CompOp     // оператор сравнения (==,!=,<,<=,>,>=)
	Threshold Expression // правая часть сравнения
}

// NewMetricTrigger строит форму метрика-триггера; pos — позиция токена «метрика».
func NewMetricTrigger(pos Position, metric Ident, op CompOp, threshold Expression) *MetricTrigger {
	return &MetricTrigger{specBase: specBase{base{pos}}, Metric: metric, Op: op, Threshold: threshold}
}

// EventTrigger — «событие Event». Pos() = токен «событие».
type EventTrigger struct {
	specBase
	Event Ident // имя события
}

// NewEventTrigger строит форму событие-триггера; pos — позиция токена «событие».
func NewEventTrigger(pos Position, event Ident) *EventTrigger {
	return &EventTrigger{specBase: specBase{base{pos}}, Event: event}
}

// ScheduleTrigger — «расписание ScheduleSpec». Pos() = токен «расписание».
type ScheduleTrigger struct {
	specBase
	Spec ScheduleSpec // «каждые DurationLit» ИЛИ «в StringLit»
}

// NewScheduleTrigger строит форму расписание-триггера; pos — позиция токена «расписание».
func NewScheduleTrigger(pos Position, spec ScheduleSpec) *ScheduleTrigger {
	return &ScheduleTrigger{specBase: specBase{base{pos}}, Spec: spec}
}

// ScheduleSpec — спецификация расписания: интервал ИЛИ время суток.
// Реализуется EverySchedule («каждые») и AtSchedule («в»). Маркер scheduleSpec()
// — пустой метод; дискриминация подформ через Go type-switch.
type ScheduleSpec interface {
	Node
	scheduleSpec()
}

// schedBase — база для подформ расписания: Pos() (= ведущий токен подформы) +
// маркер scheduleSpec().
type schedBase struct{ base }

func (schedBase) scheduleSpec() {}

// EverySchedule — «каждые DurationLit». Pos() = токен «каждые». Every — литерал
// длительности; все 6 единиц принимаются в 007a (нед/мес без ошибки).
type EverySchedule struct {
	schedBase
	Every *DurationLit // 3дн/5мин/2нед/1мес/30сек/1час
}

// NewEverySchedule строит подформу «каждые»; pos — позиция токена «каждые».
func NewEverySchedule(pos Position, every *DurationLit) *EverySchedule {
	return &EverySchedule{schedBase: schedBase{base{pos}}, Every: every}
}

// AtSchedule — «в StringLit». Pos() = токен «в». At — строковый литерал «ЧЧ:ММ»
// (по значению, зеркало SourceDecl.File StringLit); формат содержимого в 007a НЕ
// проверяется (валидация → 007b).
type AtSchedule struct {
	schedBase
	At StringLit // "ЧЧ:ММ" (любая строка)
}

// NewAtSchedule строит подформу «в»; pos — позиция токена «в».
func NewAtSchedule(pos Position, at StringLit) *AtSchedule {
	return &AtSchedule{schedBase: schedBase{base{pos}}, At: at}
}
