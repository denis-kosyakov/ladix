package daemon

import (
	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// injection — инжектируемая в env тела триггера предопределённая привязка (§TR-5).
// name пусто ⇒ ничего не инжектировать (расписание: ни «значение», ни «событие»).
type injection struct {
	name string
	val  value.Value
}

// fireBody исполняет тело триггера штатным путём движка 006 с инжектом read-only
// предопределённой привязки (EM-17.5, §TR-5). Env тела — граница записи (env-барьер
// 007a, TR-BODY-RO): чтение глобалов/метрик поднимается вверх, запись в глобал
// забарьерена. «запустить процесс» доходит до Engine.Start (p-NNNNNN, fire-and-
// forget); несколько действий — последовательно (FR-018). Read-only глобалов и
// исполнитель блока переиспользуются из eval (NewTriggerBodyEnv/EvalBlockInTrigger),
// логика не дублируется. Возвращает error (вызывающий оборачивает в safeFire).
func (d *Daemon) fireBody(body *ast.Block, inj injection) error {
	env := d.interp.NewTriggerBodyEnv()
	if inj.name != "" {
		env.Define(inj.name, inj.val)
	}
	return d.interp.EvalBlockInTrigger(env, body)
}
