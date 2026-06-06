package eval

import (
	"fmt"
	"io"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// DefaultMaxDepth — лимит глубины пользовательских вызовов по умолчанию (D9).
const DefaultMaxDepth = 10000

// Interpreter — состояние исполнения (§5.1). Без глобалов: out инжектируется,
// лимит глубины — поле (конституция V).
type Interpreter struct {
	global    *Environment
	funcs     map[string]*ast.FunctionDecl // пользовательские функции (из Analyze)
	builtins  map[string]Builtin           // 23 активных + 12 deferred-заглушек
	maxDepth  int                          // из флага --max-depth
	depth     int                          // текущая глубина пользовательских вызовов
	out       io.Writer                    // канал печать()
	iterating map[*[]value.Value]int       // списки под активной итерацией для (§4.3)
	analyzed  bool                         // Analyze уже отработал (защита от двойного резолва)
}

// NewInterpreter создаёт интерпретатор с инжектированным каналом вывода и лимитом
// глубины (maxDepth ≤ 0 → DefaultMaxDepth).
func NewInterpreter(out io.Writer, maxDepth int) *Interpreter {
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDepth
	}
	return &Interpreter{
		global:    NewEnvironment(nil),
		funcs:     make(map[string]*ast.FunctionDecl),
		builtins:  registerBuiltins(),
		maxDepth:  maxDepth,
		out:       out,
		iterating: make(map[*[]value.Value]int),
	}
}

// Run исполняет программу: семпроход Analyze (если ещё не выполнен), затем
// top-level statements сверху вниз в глобальной области (§4.4). Fail-fast: первая
// ошибка всплывает наверх.
func (i *Interpreter) Run(prog *ast.Program) error {
	if !i.analyzed {
		if err := i.Analyze(prog); err != nil {
			return err
		}
	}
	for _, item := range prog.Items {
		if _, ok := item.(*ast.FunctionDecl); ok {
			continue // зарегистрирована Analyze, как statement пропускается
		}
		st, ok := item.(ast.Statement)
		if !ok {
			continue
		}
		if _, err := i.evalStmt(i.global, st); err != nil {
			return err
		}
	}
	return nil
}

// --- метки итерации (защита от мутации списка во время «для», §4.3) ---

func (i *Interpreter) markIterating(p *[]value.Value) { i.iterating[p]++ }
func (i *Interpreter) unmarkIterating(p *[]value.Value) {
	if i.iterating[p] <= 1 {
		delete(i.iterating, p)
		return
	}
	i.iterating[p]--
}
func (i *Interpreter) isIterating(p *[]value.Value) bool { return i.iterating[p] > 0 }

// --- конструкторы диагностик (привязка ast.Position → errors.Position) ---

func toErrPos(p ast.Position) errors.Position {
	return errors.Position{Line: p.Line, Col: p.Col}
}

// semErr — СемантическаяОшибка (стадия 3).
func semErr(p ast.Position, msg string) error {
	return errors.СемантическаяОшибка{Pos: toErrPos(p), Msg: msg}
}

// typeErr — ОшибкаТипа (стадия 4).
func typeErr(p ast.Position, msg string) error {
	return errors.ОшибкаТипа{Pos: toErrPos(p), Msg: msg}
}

// runtimeErr — ОшибкаВыполнения (стадия 4).
func runtimeErr(p ast.Position, msg string) error {
	return errors.ОшибкаВыполнения{Pos: toErrPos(p), Msg: msg}
}

// deferredConstruct — SEM-DEFERRED-CONSTRUCT для reserved-узлов/декларатива/
// литерала длительности (§9, FR-036). <X> — человекочитаемое имя конструкции.
func (i *Interpreter) deferredConstruct(node ast.Node) error {
	return semErr(node.Pos(), fmt.Sprintf("конструкция %s не поддерживается в этой версии", constructName(node)))
}

func constructName(node ast.Node) string {
	switch node.(type) {
	case *ast.RunProcessExpr:
		return "запустить процесс"
	case *ast.DurationLit:
		return "литерал длительности"
	case *ast.AssignAction:
		return "присвоить"
	case *ast.CallAction:
		return "вызвать"
	case *ast.NotifyAction:
		return "уведомить"
	}
	return "неизвестная конструкция"
}
