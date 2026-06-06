package eval

import (
	"fmt"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// evalExpr вычисляет выражение (§3). Type switch по УКАЗАТЕЛЬНЫМ узлам AST;
// операнды/аргументы/элементы вычисляются слева направо; дерево приоритетов от
// парсера не переразбирается.
func (i *Interpreter) evalExpr(env *Environment, e ast.Expression) (value.Value, error) {
	switch ex := e.(type) {
	case *ast.IntLit:
		return value.Целое{V: ex.Value}, nil
	case *ast.FloatLit:
		return value.Дробное{V: ex.Value}, nil
	case *ast.StringLit:
		return value.Строка{V: ex.Value}, nil
	case *ast.BoolLit:
		return value.Булево{V: ex.Value}, nil
	case *ast.NoneLit:
		return value.None, nil
	case *ast.ListLit:
		elems := make([]value.Value, len(ex.Elements))
		for k, el := range ex.Elements {
			v, err := i.evalExpr(env, el)
			if err != nil {
				return nil, err
			}
			elems[k] = v
		}
		return value.NewList(elems), nil
	case *ast.Ident:
		return i.evalIdent(env, ex)
	case *ast.UnaryExpr:
		return i.evalUnary(env, ex)
	case *ast.BinaryExpr:
		return i.evalBinary(env, ex)
	case *ast.CallExpr:
		return i.evalCall(env, ex)
	case *ast.IndexExpr:
		return i.evalIndex(env, ex)
	case *ast.FieldExpr:
		return i.evalField(env, ex)
	case *ast.RunProcessExpr:
		return nil, i.deferredConstruct(ex)
	case *ast.DurationLit:
		return nil, i.deferredConstruct(ex)
	}
	return nil, runtimeErr(e.Pos(), "внутренняя ошибка: неизвестный узел выражения")
}

// evalIdent резолвит идентификатор в позиции ЗНАЧЕНИЯ (§2.3): сперва переменная,
// затем — имя функции (→ «функция-как-значение»), иначе → «не объявлено». Оба
// промаха flow-зависимы, поэтому это рантайм, не семпроход.
func (i *Interpreter) evalIdent(env *Environment, id *ast.Ident) (value.Value, error) {
	if v, ok := env.Lookup(id.Name); ok {
		return v, nil
	}
	if i.isFunctionName(id.Name) {
		return nil, runtimeErr(id.Pos(), fmt.Sprintf("'%s' — функция, её нельзя использовать как значение", id.Name))
	}
	return nil, runtimeErr(id.Pos(), fmt.Sprintf("'%s' не объявлено", id.Name))
}

// isFunctionName сообщает, известно ли имя в пространстве функций (пользовательские
// или встроенные, включая deferred-заглушки).
func (i *Interpreter) isFunctionName(name string) bool {
	if _, ok := i.funcs[name]; ok {
		return true
	}
	_, ok := i.builtins[name]
	return ok
}

// evalIndex — target[index] (§3.4). Индексация в рунах для Строки; границы вкл.
// отрицательные. Позиция = IndexExpr.Pos() (начало target).
func (i *Interpreter) evalIndex(env *Environment, ix *ast.IndexExpr) (value.Value, error) {
	target, err := i.evalExpr(env, ix.Target)
	if err != nil {
		return nil, err
	}
	idx, err := i.evalExpr(env, ix.Index)
	if err != nil {
		return nil, err
	}
	switch t := target.(type) {
	case value.Список:
		ci, ok := idx.(value.Целое)
		if !ok {
			return nil, typeErr(ix.Pos(), fmt.Sprintf("индекс должен быть Целое, получено %s", idx.TypeName()))
		}
		n := int64(len(*t.Elems))
		if ci.V < 0 || ci.V >= n {
			return nil, runtimeErr(ix.Pos(), fmt.Sprintf("индекс %d вне диапазона (длина %d)", ci.V, n))
		}
		return (*t.Elems)[ci.V], nil
	case value.Строка:
		ci, ok := idx.(value.Целое)
		if !ok {
			return nil, typeErr(ix.Pos(), fmt.Sprintf("индекс должен быть Целое, получено %s", idx.TypeName()))
		}
		runes := []rune(t.V)
		n := int64(len(runes))
		if ci.V < 0 || ci.V >= n {
			return nil, runtimeErr(ix.Pos(), fmt.Sprintf("индекс %d вне диапазона (длина %d)", ci.V, n))
		}
		return value.Строка{V: string(runes[ci.V])}, nil
	default:
		return nil, typeErr(ix.Pos(), fmt.Sprintf("значение типа %s не индексируется", target.TypeName()))
	}
}

// evalField — target.field (§3.4). В чистом 003 Запись не конструируется, поэтому
// FieldExpr всегда упирается в «не имеет полей» (механизм чтения готов к store).
func (i *Interpreter) evalField(env *Environment, fe *ast.FieldExpr) (value.Value, error) {
	target, err := i.evalExpr(env, fe.Target)
	if err != nil {
		return nil, err
	}
	if rec, ok := target.(value.Запись); ok {
		return rec.Get(fe.Field.Name), nil
	}
	return nil, typeErr(fe.Pos(), fmt.Sprintf("значение типа %s не имеет полей", target.TypeName()))
}
