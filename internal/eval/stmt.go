package eval

import (
	"fmt"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// evalStmt исполняет один statement (§4.2). Два канала: error (рантайм, всплывает
// немедленно) и Signal (штатный поток).
func (i *Interpreter) evalStmt(env *Environment, s ast.Statement) (Signal, error) {
	switch st := s.(type) {
	case *ast.LetStmt:
		v, err := i.evalExpr(env, st.Value)
		if err != nil {
			return Signal{}, err
		}
		env.Define(st.Name.Name, v)
		return Signal{Kind: SigNormal}, nil

	case *ast.AssignStmt:
		v, err := i.evalExpr(env, st.Value)
		if err != nil {
			return Signal{}, err
		}
		if !env.Assign(st.Name.Name, v) {
			// Boundary-env тела триггера (§TR-5): имя резолвится Lookup-ом вверх
			// (глобал виден), но запись забарьерена → read-only глобала (TR-BODY-RO,
			// §TR-7.G). Не резолвится нигде → прежняя «'x' не объявлено». Вне триггера
			// boundary=false: Assign падает лишь когда Lookup тоже не находит → нижняя ветвь.
			if _, ok := env.Lookup(st.Name.Name); ok {
				return Signal{}, runtimeErr(st.Name.Pos(), fmt.Sprintf("глобальная переменная '%s' доступна в теле триггера только для чтения", st.Name.Name))
			}
			return Signal{}, runtimeErr(st.Name.Pos(), fmt.Sprintf("'%s' не объявлено", st.Name.Name))
		}
		return Signal{Kind: SigNormal}, nil

	case *ast.ExpressionStmt:
		if _, err := i.evalExpr(env, st.Expr); err != nil {
			return Signal{}, err
		}
		return Signal{Kind: SigNormal}, nil

	case *ast.IfStmt:
		return i.evalIf(env, st)

	case *ast.TryStmt:
		return i.evalTry(env, st)

	case *ast.WhileStmt:
		return i.evalWhile(env, st)

	case *ast.ForStmt:
		return i.evalFor(env, st)

	case *ast.ReturnStmt:
		if st.Value == nil {
			return Signal{Kind: SigReturn, Value: value.None}, nil
		}
		v, err := i.evalExpr(env, st.Value)
		if err != nil {
			return Signal{}, err
		}
		return Signal{Kind: SigReturn, Value: v}, nil

	case *ast.BreakStmt:
		return Signal{Kind: SigBreak}, nil

	case *ast.ContinueStmt:
		return Signal{Kind: SigContinue}, nil

	case *ast.AssignAction:
		return i.evalAssignAction(env, st)

	case *ast.CallAction:
		return i.evalCallAction(env, st)

	case *ast.NotifyAction:
		return i.evalNotifyAction(env, st)
	}
	return Signal{}, runtimeErr(s.Pos(), "внутренняя ошибка: неизвестный statement")
}

// evalAssignAction активирует «присвоить x = E» (006, §EN-5): вычислить E в текущем
// env → i.procEnv.Define(x, v) (создаёт ИЛИ обновляет переменную процесса, мимо тени
// пусть-локали шага, §6.4) → хук AssignProcessVar (персист). nil-runtime → §EN-8.A;
// ошибка хука → ОшибкаВыполнения с позицией присвоить; текст «сбой хранилища: <причина>»
// несёт сам StoreError движка — НЕ дублируем префикс (ср. builtins_process.go), §EN-8.A.
func (i *Interpreter) evalAssignAction(env *Environment, a *ast.AssignAction) (Signal, error) {
	if i.runtime == nil || i.procEnv == nil {
		return Signal{}, runtimeErr(a.Pos(), "внутренняя ошибка: движок процессов не подключён")
	}
	v, err := i.evalExpr(env, a.Value)
	if err != nil {
		return Signal{}, err
	}
	i.procEnv.Define(a.Name.Name, v)
	if err := i.runtime.AssignProcessVar(a.Name.Name, v); err != nil {
		return Signal{}, runtimeErrWrap(a.Pos(), err)
	}
	return Signal{Kind: SigNormal}, nil
}

// evalCallAction активирует «вызвать Имя(args)» (006, §EN-5): аргументы слева
// направо → стаб CallExternal (печать строки §EN-7.2; в v1 всегда успех). Имя цели —
// Ident как строка, НЕ резолвится как переменная.
func (i *Interpreter) evalCallAction(env *Environment, c *ast.CallAction) (Signal, error) {
	if i.runtime == nil {
		return Signal{}, runtimeErr(c.Pos(), "внутренняя ошибка: движок процессов не подключён")
	}
	args, err := i.evalArgs(env, c.Args)
	if err != nil {
		return Signal{}, err
	}
	// Сбой реального драйвера (B2, §AU-4.4): оборачиваем в ОшибкаВыполнения с
	// сохранением цепочки Cause/Unwrap (как evalAssignAction stmt.go:96 и evalRunProcess
	// expr.go:205). Под дефолт-стабом ошибки нет (printCaller → nil), поведение v1 цело.
	if err := i.runtime.CallExternal(c.Name.Name, args); err != nil {
		return Signal{}, runtimeErrWrap(c.Pos(), err)
	}
	return Signal{Kind: SigNormal}, nil
}

// evalNotifyAction активирует «уведомить Имя(args)» (006, §EN-5): аргументы слева
// направо → стаб Notify (печать строки §EN-7.1/1а; всегда best-effort nil).
func (i *Interpreter) evalNotifyAction(env *Environment, n *ast.NotifyAction) (Signal, error) {
	if i.runtime == nil {
		return Signal{}, runtimeErr(n.Pos(), "внутренняя ошибка: движок процессов не подключён")
	}
	args, err := i.evalArgs(env, n.Args)
	if err != nil {
		return Signal{}, err
	}
	// Сбой реального драйвера (B2, §AU-4.4): оборачиваем в ОшибкаВыполнения с
	// сохранением цепочки Cause/Unwrap (как evalAssignAction stmt.go:96 и evalRunProcess
	// expr.go:205). Под дефолт-стабом ошибки нет (printCaller → nil), поведение v1 цело.
	if err := i.runtime.Notify(n.Name.Name, args); err != nil {
		return Signal{}, runtimeErrWrap(n.Pos(), err)
	}
	return Signal{Kind: SigNormal}, nil
}

// evalArgs вычисляет список аргументов слева направо.
func (i *Interpreter) evalArgs(env *Environment, exprs []ast.Expression) ([]value.Value, error) {
	args := make([]value.Value, len(exprs))
	for k, a := range exprs {
		v, err := i.evalExpr(env, a)
		if err != nil {
			return nil, err
		}
		args[k] = v
	}
	return args, nil
}

// evalBlock исполняет последовательность statements в ТЕКУЩЕЙ области (блок свою
// область не создаёт). err≠nil → наверх; sig≠SigNormal → прекратить блок и
// вернуть сигнал (§4.1).
func (i *Interpreter) evalBlock(env *Environment, b *ast.Block) (Signal, error) {
	for _, st := range b.Stmts {
		sig, err := i.evalStmt(env, st)
		if err != nil {
			return Signal{}, err
		}
		if sig.Kind != SigNormal {
			return sig, nil
		}
	}
	return Signal{Kind: SigNormal}, nil
}

// evalTry — обработка ошибок выполнения (029): пытаться/словить, семантика
// REDIRECT. Если любой оператор тела Try вернул runtime-ошибку (весь канал error:
// сбой вызвать, деление на ноль, ошибка типа, индекс…) — остаток Try бросается и
// исполняется Catch. Ошибки нет: сигнал тела Try (вернуть/прервать/продолжить/
// норм) проходит насквозь — Catch НЕ исполняется. Скоуп арма зеркалит evalIf
// (то же env, без дочернего окружения). Без panic/recover (конституция III):
// проверки err != nil достаточно, Signal — отдельный канал, в err не попадает.
func (i *Interpreter) evalTry(env *Environment, t *ast.TryStmt) (Signal, error) {
	sig, err := i.evalBlock(env, t.Try)
	if err != nil {
		return i.evalBlock(env, t.Catch)
	}
	return sig, nil
}

// evalIf — если/иначе если/иначе (§4.2): strict-Булево, первая истинная ветвь,
// цепочка ElseClause по IsFinal().
func (i *Interpreter) evalIf(env *Environment, st *ast.IfStmt) (Signal, error) {
	cond, err := i.condBool(env, st.Cond)
	if err != nil {
		return Signal{}, err
	}
	if cond {
		return i.evalBlock(env, st.Then)
	}
	return i.evalElse(env, st.Else)
}

func (i *Interpreter) evalElse(env *Environment, e *ast.ElseClause) (Signal, error) {
	if e == nil {
		return Signal{Kind: SigNormal}, nil
	}
	if e.IsFinal() {
		return i.evalBlock(env, e.Body)
	}
	cond, err := i.condBool(env, e.Cond)
	if err != nil {
		return Signal{}, err
	}
	if cond {
		return i.evalBlock(env, e.Then)
	}
	return i.evalElse(env, e.Else)
}

// condBool вычисляет условие со strict-Булево (truthiness не действует, §1.3.3).
// Позиция ошибки = Cond.Pos().
func (i *Interpreter) condBool(env *Environment, cond ast.Expression) (bool, error) {
	v, err := i.evalExpr(env, cond)
	if err != nil {
		return false, err
	}
	bv, ok := v.(value.Булево)
	if !ok {
		return false, typeErr(cond.Pos(), fmt.Sprintf("условие должно быть Булево, получено %s", v.TypeName()))
	}
	return bv.V, nil
}

// evalWhile — пока Cond: Body (§4.3). SigBreak поглощается (выход), SigContinue
// поглощается (следующая итерация), SigReturn пробрасывается наверх.
func (i *Interpreter) evalWhile(env *Environment, st *ast.WhileStmt) (Signal, error) {
	for {
		cond, err := i.condBool(env, st.Cond)
		if err != nil {
			return Signal{}, err
		}
		if !cond {
			break
		}
		sig, err := i.evalBlock(env, st.Body)
		if err != nil {
			return Signal{}, err
		}
		switch sig.Kind {
		case SigBreak:
			return Signal{Kind: SigNormal}, nil
		case SigContinue:
			continue
		case SigReturn:
			return sig, nil
		}
	}
	return Signal{Kind: SigNormal}, nil
}

// evalFor — для Var в Iterable: Body (§4.3). Iterable обязан Список; переменная
// привязывается в ОХВАТЫВАЮЩЕЙ области (Define-если-нет, затем Assign); на пустом
// списке не создаётся; список помечается «итерируется» (защита от мутации).
func (i *Interpreter) evalFor(env *Environment, st *ast.ForStmt) (Signal, error) {
	it, err := i.evalExpr(env, st.Iterable)
	if err != nil {
		return Signal{}, err
	}
	lst, ok := it.(value.Список)
	if !ok {
		return Signal{}, typeErr(st.Iterable.Pos(), fmt.Sprintf("'для' требует Список, получено %s", it.TypeName()))
	}
	name := st.Var.Name
	i.markIterating(lst.Elems)
	defer i.unmarkIterating(lst.Elems)
	n := len(*lst.Elems)
	for idx := 0; idx < n; idx++ {
		elem := (*lst.Elems)[idx]
		if env.hasLocal(name) {
			env.Assign(name, elem)
		} else {
			env.Define(name, elem)
		}
		sig, err := i.evalBlock(env, st.Body)
		if err != nil {
			return Signal{}, err
		}
		switch sig.Kind {
		case SigBreak:
			return Signal{Kind: SigNormal}, nil
		case SigContinue:
			continue
		case SigReturn:
			return sig, nil
		}
	}
	return Signal{Kind: SigNormal}, nil
}
