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

// SetSourceBase задаёт базовый каталог разрешения относительных путей источников
// (§SM-8.1, фича 026; вызывается CLI до Run). Зеркало SetProcessRuntime: инъекция
// рантайм-зависимости в значение-интерпретатор без глобального состояния (Принцип V).
// Пусто ("") эквивалентно резолву от cwd процесса. См. resolveSourcePath.
func (i *Interpreter) SetSourceBase(dir string) { i.sourceBase = dir }

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

// ApplySourceSchema — экспортированная обёртка applySchema (source_loader.go) для
// публичного исполнителя метрик (фича 030, Д-7). Приводит записи recs по объявленной
// схеме decl.Fields ТЕМ ЖЕ кодом, которым это делает загрузчик источника CLI: тексты
// ошибок приведения (§SC-9.B) и результат совпадают дословно, второй семантики нет.
// decl — обычно декларация из разобранной программы (для JSON-семантики decl.Type.Name
// пусто/"json"; "csv" включает строковый парс, §SC-D-COERCE). Пустой decl.Fields —
// no-op-копия. Метод НЕ читает файлы и НЕ трогает кеш записей.
func (i *Interpreter) ApplySourceSchema(decl *ast.SourceDecl, recs []value.Запись) ([]value.Запись, error) {
	if len(decl.Fields) == 0 {
		return recs, nil
	}
	return i.applySchema(decl, recs)
}

// MakeDate — экспортированная обёртка makeDate (builtins_date.go): построение
// value.Дата с ТОЙ ЖЕ григорианской проверкой календаря, что у встроенной «дата» и
// у коэрсии поля Дата. Нужна публичному исполнителю метрик (030, Д-3) для валидации
// инжектированной даты среза без второго календаря. ok == false ⟺ дата не существует.
func MakeDate(y, m, d int64) (value.Дата, bool) { return makeDate(y, m, d) }
