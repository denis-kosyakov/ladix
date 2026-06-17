package daemon

import (
	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// checkDeadlines — ФАЗА 4 тика (016 B4b, §AU-6.2.2): эскалация просроченных задач
// человека-в-цикле. Под d.mu (как первые три фазы); аддитивна в хвост, не трогает их
// порядок/идемпотентность (INV-1 007b). Скан БЕЗ нового Store-метода (D-AU-5): фильтр
// эскалация-триггеров → ранний return (нет работы — без листинга) → ОДИН ListPendingTasks
// до циклов → durable-фильтр Escalated → просрочка через engine.Overdue → совпадение
// шаг/процесс → fireDeadlineBody (инжект всех InstanceVariables, D-AU-6) → пометка
// Escalated + SaveTask (durable-персист, UPSERT). Одна эскалация на задачу (break).
//
// Решения псевдокода (§AU-6.2.2, закрыты — не гадать): (1) ListPendingTasks — ОДИН раз
// до циклов, возвращает копии (повторный SaveTask внутри срез не инвалидирует).
// (2) Просрочка — единый engine.Overdue (нет off-by-one на now==Deadline). (3) Ошибка
// листинга — лог + выход из фазы (изоляция как у первых трёх). (4) Инжект — напрямую из
// уже загруженного inst.Variables. SaveTask после fire — at-least-once допустим до M3.
func (d *Daemon) checkDeadlines() {
	now := d.clock.Now()

	// Фильтр эскалация-триггеров по виду; нет ни одного → ранний return БЕЗ листинга.
	var deadlineTriggers []*ast.TriggerDecl
	for _, td := range d.interp.Triggers() {
		if _, ok := td.Spec.(*ast.DeadlineTrigger); ok {
			deadlineTriggers = append(deadlineTriggers, td)
		}
	}
	if len(deadlineTriggers) == 0 {
		return
	}

	// ОДИН листинг открытых задач до циклов (СУЩЕСТВУЮЩИЙ метод, без assignee-фильтра).
	tasks, err := d.st.ListPendingTasks("")
	if err != nil {
		d.logf("checkDeadlines: листинг задач: %s", err.Error())
		return
	}

	for _, t := range tasks {
		if t.Escalated {
			continue // durable-фильтр (D-AU-5): уже эскалирована — переживает рестарт
		}
		if !engine.Overdue(t, now) {
			continue // не просрочена (engine.Overdue: t.Deadline != nil && now.After(*t.Deadline))
		}
		inst, lerr := d.st.LoadInstance(t.InstanceID)
		if lerr != nil {
			continue // инстанс не загружается — изолируем (как safeFire), без падения фазы
		}
		for _, td := range deadlineTriggers {
			spec := td.Spec.(*ast.DeadlineTrigger)
			if t.StepName != spec.Step.Name || inst.ProcessName != spec.Process.Name {
				continue
			}
			body := td.Body
			vars := inst.Variables
			d.safeFire(func() error { return d.fireDeadlineBody(body, vars) })
			t.Escalated = true
			if serr := d.st.SaveTask(t); serr != nil {
				d.logf("checkDeadlines: персист Escalated задачи %s: %s", t.ID, serr.Error())
			}
			break // одна эскалация на задачу
		}
	}
}

// fireDeadlineBody исполняет тело эскалация-триггера, инжектируя ВСЕ переменные
// инстанса в read-only env тела (D-AU-6, §AU-6.2.3). В отличие от fireBody (один
// injection{name,val}, fire.go:22), эскалация инжектит все переменные прямым циклом
// Define поверх NewTriggerBodyEnv (env-барьер read-only глобалов, TR-BODY-RO),
// минуя struct injection (он НЕ расширяется). Тело читает инжект (напр. «факт»),
// зовёт уведомить/вызвать/запустить процесс, но не перепривязывает глобали.
func (d *Daemon) fireDeadlineBody(body *ast.Block, vars map[string]value.Value) error {
	env := d.interp.NewTriggerBodyEnv() // NewEnvironment(global) + markBoundary
	for k, v := range vars {
		env.Define(k, v) // все переменные инстанса как локали барьерного env
	}
	return d.interp.EvalBlockInTrigger(env, body)
}
