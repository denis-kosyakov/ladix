package eval

import (
	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// Экспортная поверхность eval для демона 007b (internal/daemon, §EM-17). Аддитивна
// к RunTriggers 007a (trigger_run.go): демон НЕ зовёт RunTriggers (тот — эфемерный
// fire-if-true для `run`), а собирает edge-детект/durable trigger_state сам, но
// переиспользует те же приватные кирпичи (evalMetricByName + compareValues + env-
// барьер §TR-5). Никакой новой арифметики/логики — только публичный фасад.

// EvalMetricCondition вычисляет ТЕКУЩИЙ булев результат метрика-триггера (метрика
// vs порог) и возвращает снимок значения метрики для инжекции «значение» в тело при
// срабатывании (§TR-5). Переиспользует тот же конвейер, что runMetricTrigger 007a:
// (1) значение метрики через evalMetricByName (защита от цикла §SM-9.B); (2) порог в
// i.global; (3) сравнение оператором 003 через compareValues.
//
// Возвращает:
//   - cur  — булев результат сравнения (валиден только при ok==true);
//   - snapshot — снимок значения метрики (инжектируется как «значение», §TR-5);
//   - ok   — вычислимо ли условие. false ⇒ ЗАМОРОЗКА (FR-009): метрика пуста
//     (value.Пусто) ИЛИ сравнение не дало Булево (несравнимые типы / ОшибкаТипа).
//     Демон при ok==false ничего не персистит и тело не исполняет.
//   - err  — НЕштатная ошибка вычисления (цикл зависимостей метрик §SM-9.B, сбой
//     загрузки источника). Демон логирует её и трактует как заморозку (тик не падает).
//
// Метод над полями интерпретатора (Принцип V — без глобального состояния);
// синхронизация при разделении с горутиной демона — забота демона (d.mu вокруг тика).
func (i *Interpreter) EvalMetricCondition(spec *ast.MetricTrigger) (cur bool, snapshot value.Value, ok bool, err error) {
	metricVal, err := i.evalMetricByName(spec.Metric.Name, spec.Metric.Pos())
	if err != nil {
		return false, nil, false, err
	}
	// Метрика пуста (агрегат над пустым множеством) → сравнение бессмысленно:
	// заморозка (§TR-?, FR-009). Не ошибка — штатное «нет данных на этом тике».
	if _, empty := metricVal.(value.Пусто); empty {
		return false, metricVal, false, nil
	}
	threshVal, err := i.evalExpr(i.global, spec.Threshold)
	if err != nil {
		return false, nil, false, err
	}
	fired, cmpErr := i.compareValues(spec.Metric.Pos(), ast.BinOp(spec.Op), metricVal, threshVal)
	if cmpErr != nil {
		// Сравнение не дало Булево (несравнимые типы / ОшибкаТипа) → заморозка, не
		// падение тика (FR-009). Ошибку наверх не несём: это ожидаемая невычислимость.
		return false, metricVal, false, nil
	}
	return fired, metricVal, true, nil
}

// NewTriggerBodyEnv создаёт локальную область для исполнения тела триггера демоном
// с env-барьером read-only глобалов (§TR-5/TR-BODY-RO): запись в имя, которого нет
// среди локалей этой области, НЕ поднимается к global. Зеркало trigger_run.go:78-79
// (NewEnvironment(i.global) + markBoundary), но markBoundary приватен — демон в
// другом пакете не может его звать, поэтому фасад инкапсулирует оба шага. Демон
// затем кладёт сюда «значение»/«событие» через Environment.Define (экспортный).
func (i *Interpreter) NewTriggerBodyEnv() *Environment {
	env := NewEnvironment(i.global)
	env.markBoundary()
	return env
}

// EvalBlockInTrigger — публичная обёртка evalBlockInTrigger (trigger_run.go):
// исполняет тело-Block триггера в данном env штатным исполнителем блока (контекст
// inStep=false, как top-level; §TR-6.3). «запустить процесс» доходит до Engine.Start
// (p-NNNNNN, fire-and-forget); несколько действий — последовательно (FR-018). Сигнал
// блока гасится внутри; наружу — только error. Демон оборачивает вызов в recover
// (per-триггер изоляция, EM-17.6).
func (i *Interpreter) EvalBlockInTrigger(env *Environment, body *ast.Block) error {
	return i.evalBlockInTrigger(env, body)
}
