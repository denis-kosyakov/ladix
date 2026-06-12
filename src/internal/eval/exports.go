package eval

import (
	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// Экспортируемая исполнительная поверхность eval для движка процессов (006, §EN-4,
// D-1, вариант B). eval НЕ импортирует engine/store; связь — интерфейс
// ProcessRuntime (runtime.go) + сеттер. Движок строит кадр шага из этих примитивов.

// SetProcessRuntime инжектирует движок (вызывается до Run; результата Analyze не
// меняет — семпроход от runtime не зависит). nil-runtime: активированные конструкции
// дают ОшибкуВыполнения «внутренняя ошибка: движок процессов не подключён» (§EN-8.A).
func (i *Interpreter) SetProcessRuntime(rt ProcessRuntime) { i.runtime = rt }

// Process — доступ к реестру процессов (i.processes; заполняет Analyze Шаг 1). Для
// lookup определения и «следующего шага» в движке (§EN-4).
func (i *Interpreter) Process(name string) (*ast.ProcessDecl, bool) {
	pd, ok := i.processes[name]
	return pd, ok
}

// GlobalEnv — глобальная область (родитель processEnv в кадре шага, §EN-4).
func (i *Interpreter) GlobalEnv() *Environment { return i.global }

// EvalExpr — публичная обёртка evalExpr: вычисление атрибутов шага движком (D-9).
func (i *Interpreter) EvalExpr(env *Environment, e ast.Expression) (value.Value, error) {
	return i.evalExpr(env, e)
}

// ExecStepBody исполняет тело шага (StepDecl.Body []ast.Statement — НЕ *ast.Block,
// поэтому НЕ evalBlock) в области stepEnv; на время исполнения запоминает processEnv
// как приёмник «присвоить» (поле i.procEnv, save/restore реентерабельно — зеркало
// recordCtx: вложенный StartProcess из тела шага переключает и восстанавливает).
// Семантика цикла — как evalBlock (stmt.go): err → наверх; sig != SigNormal →
// прекратить и вернуть (§EN-4).
func (i *Interpreter) ExecStepBody(processEnv, stepEnv *Environment, body []ast.Statement) (Signal, error) {
	saved := i.procEnv
	i.procEnv = processEnv
	defer func() { i.procEnv = saved }()
	for _, st := range body {
		sig, err := i.evalStmt(stepEnv, st)
		if err != nil {
			return Signal{}, err
		}
		if sig.Kind != SigNormal {
			return sig, nil
		}
	}
	return Signal{Kind: SigNormal}, nil
}
