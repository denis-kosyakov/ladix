package eval

import (
	"fmt"
	"io"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// valueName — предопределённое имя метрика-триггера, инжектируемое в env тела на
// момент срабатывания (§TR-5). Жёсткое KW (KW_VALUE), не Ident: пользователь не
// может его переобъявить/присвоить, поэтому привязка эффективно read-only —
// отдельного DefineReadOnly в Environment нет (зеркало предопределённых Период,
// которые тоже заводятся обычным Define).
const valueName = "значение"

// eventName — предопределённое имя событие-триггера, инжектируемое в env тела демоном
// (007b §TR-5, drainEvents): «событие» = Запись из payload. Жёсткое KW (KW_EVENT), не
// Ident — пользователь не может его переобъявить, привязка эффективно read-only
// (зеркало valueName). Резолвится evalExpr на узле *ast.EventExpr.
const eventName = "событие"

// RunTriggers исполняет одноразовый проход fire-if-true по зарегистрированным
// триггерам (реестр i.triggers, заполнен Analyze Шаг 1d) в порядке объявления
// (§TR-6.1). Вызывается из cmd/ladix ПОСЛЕ interp.Run (глобалы связаны) и ДО сводки
// задач (§TR-8.1).
//
// Для метрика-триггера: вычислить метрику → вычислить порог в i.global → сравнить
// существующим оператором сравнения 003; база ЛОЖЬ эфемерно (§TR-6.2: не читает/не
// пишет trigger_state даже под --db) ⇒ «переход ложь→истина» ≡ «истинно сейчас».
// При истине: инжектировать read-only «значение» = снимок метрики и исполнить тело
// штатным исполнителем блока (запуск процесса доходит до Engine.StartProcess, id
// p-NNNNNN; под --db персистится штатно). При лжи — молчание.
//
// Событие/расписание-триггеры в run не исполняются (§TR-6.4): для каждого печатается
// одна строка-заглушка «требует serve (фича 007b)» в w, в порядке объявления.
//
// Ошибка вычисления метрики/порога/сравнения или тела всплывает как error
// (обрабатывается зеркалом interp.Run: двухстрочный Error() → stderr, exit 1).
func (i *Interpreter) RunTriggers(w io.Writer) error {
	for _, td := range i.triggers {
		switch spec := td.Spec.(type) {
		case *ast.MetricTrigger:
			if err := i.runMetricTrigger(td, spec); err != nil {
				return err
			}
		case *ast.EventTrigger:
			fmt.Fprintf(w, "событие триггер '%s' требует serve (фича 007b)\n", spec.Event.Name)
		case *ast.ScheduleTrigger:
			fmt.Fprintf(w, "расписание триггер '%s' требует serve (фича 007b)\n", scheduleName(spec.Spec))
		}
	}
	return nil
}

// runMetricTrigger исполняет один метрика-триггер по правилу fire-if-true (§TR-6.3):
// (1) значение метрики; (2) порог в i.global; (3) сравнение CompOp; (4) при истине —
// инжекция «значение» в локальный env и исполнение тела.
func (i *Interpreter) runMetricTrigger(td *ast.TriggerDecl, spec *ast.MetricTrigger) error {
	// (1) значение метрики (числовое value.Value). Реестр i.metrics уже провалидирован
	// семпроходом (§TR-4 проверка 1), внутренний evalMetricByName переиспользует
	// конвейер метрик + защиту от цикла; позиция = токен имени метрики.
	metricVal, err := i.evalMetricByName(spec.Metric.Name, spec.Metric.Pos())
	if err != nil {
		return err
	}
	// (2) порог — обычное выражение в среде верхнего уровня (глобалы доступны).
	threshVal, err := i.evalExpr(i.global, spec.Threshold)
	if err != nil {
		return err
	}
	// (3) сравнение существующим оператором сравнения 003 → Булево.
	fired, err := i.compareValues(spec.Metric.Pos(), ast.BinOp(spec.Op), metricVal, threshVal)
	if err != nil {
		return err
	}
	if !fired {
		return nil // ложь → молчание (§TR-6.3): тело не исполнено, значение не инжектировано
	}
	// (4) истинно: read-only «значение» = снимок метрики (§TR-5) в локальном env тела.
	// Env тела — граница записи: чтение глобалов/метрик поднимается вверх, но запись в
	// глобал забарьерена (TR-BODY-RO, §TR-5/§TR-7.G); локальные «пусть» тела эфемерны.
	env := NewEnvironment(i.global)
	env.markBoundary()
	env.Define(valueName, metricVal)
	return i.evalBlockInTrigger(env, td.Body)
}

// compareValues применяет оператор сравнения CompOp к УЖЕ вычисленным значениям,
// переиспользуя готовый путь сравнения 003 (value.Equal для == / !=, evalOrder для
// < <= > >=) — никакой новой арифметики. Синтетический *ast.BinaryExpr несёт только
// Op и позицию (для текста ОшибкиТипа при несравнимых операндах); операнды НЕ
// перевычисляются. pos = позиция имени метрики (привязка диагностики).
func (i *Interpreter) compareValues(pos ast.Position, op ast.BinOp, left, right value.Value) (bool, error) {
	b := ast.NewBinaryExpr(pos, op, nil, nil)
	switch op {
	case ast.OpEq:
		return value.Equal(left, right), nil
	case ast.OpNeq:
		return !value.Equal(left, right), nil
	case ast.OpLt, ast.OpLe, ast.OpGt, ast.OpGe:
		v, err := i.evalOrder(b, left, right)
		if err != nil {
			return false, err
		}
		bv, ok := v.(value.Булево)
		if !ok {
			return false, runtimeErr(pos, "внутренняя ошибка: сравнение триггера вернуло не Булево")
		}
		return bv.V, nil
	}
	// CompOp гарантированно один из шести (тип ast.CompOp), default защитно.
	return false, runtimeErr(pos, "внутренняя ошибка: неизвестный оператор сравнения триггера")
}

// evalBlockInTrigger исполняет тело-Block триггера в данном env штатным исполнителем
// блока (§TR-6.3: контекст inStep=false, как top-level). Сигнал блока (вернуть/
// прервать/продолжить) гасится здесь: тело триггера — не функция и не цикл, поэтому
// go-уровневые сигналы наружу не несут смысла (их контекст-гарды отвергают такие
// конструкции на семпроходе). Возвращаем только error.
func (i *Interpreter) evalBlockInTrigger(env *Environment, body *ast.Block) error {
	_, err := i.evalBlock(env, body)
	return err
}

// scheduleName — имя расписания для строки-заглушки (§TR-6.4). У расписания нет
// собственного идентификатора: для «каждые» это лексема длительности (5мин/1дн/…),
// для «в» — строка времени ("ЧЧ:ММ"). Спека (§TR-6.4 TODO-FACT) оставляет выбор
// импл-проходу — берём содержательную форму подформы.
func scheduleName(spec ast.ScheduleSpec) string {
	switch s := spec.(type) {
	case *ast.EverySchedule:
		return s.Every.Amount + s.Every.Unit
	case *ast.AtSchedule:
		return s.At.Value
	}
	return "?"
}
