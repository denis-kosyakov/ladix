package eval

import (
	"fmt"
	"strconv"
	"strings"

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
		return i.evalRunProcess(env, ex)
	case *ast.DurationLit:
		return i.evalDurationLit(ex)
	}
	return nil, runtimeErr(e.Pos(), "внутренняя ошибка: неизвестный узел выражения")
}

// evalIdent резолвит идентификатор в позиции ЗНАЧЕНИЯ (§2.3, §SM-8.1). Порядок:
// (1) переменная (Lookup: локаль→…→глобаль, включая предопр. периоды); (2) имя
// функции (→ «функция-как-значение»); (3) имя источника (→ «нельзя как значение»,
// R6/FR-032); (4) имя метрики (→ пересчёт метрики, D-8); (5) поле текущей записи,
// если активен recordCtx (имя «запись» / поле схемы / иначе «неизвестное поле»);
// (6) «не объявлено». Глобаль/функция/встроенное/источник/метрика имеют приоритет
// над полем записи (env.Lookup и isFunctionName идут ПЕРЕД recordCtx). Все промахи
// flow-зависимы → рантайм, не семпроход.
func (i *Interpreter) evalIdent(env *Environment, id *ast.Ident) (value.Value, error) {
	if v, ok := env.Lookup(id.Name); ok {
		return v, nil
	}
	if i.isFunctionName(id.Name) {
		return nil, runtimeErr(id.Pos(), fmt.Sprintf("'%s' — функция, её нельзя использовать как значение", id.Name))
	}
	// (a) имя источника — не первоклассно (R6/FR-032, §SM-9.B).
	if _, ok := i.sources[id.Name]; ok {
		return nil, runtimeErr(id.Pos(), fmt.Sprintf("'%s' — источник, его нельзя использовать как значение", id.Name))
	}
	// (b) имя метрики — запускает пересчёт (D-8); цикл ловит evalMetricByName.
	if _, ok := i.metrics[id.Name]; ok {
		return i.evalMetricByName(id.Name, id.Pos())
	}
	// (c) контекст полей записи (метрика-контекст, §SM-8.1).
	if i.recordCtx != nil {
		if id.Name == "запись" {
			return i.recordCtx.rec, nil
		}
		if _, ok := i.recordCtx.schema[id.Name]; ok {
			return i.recordCtx.rec.Get(id.Name), nil // нет в этой записи → value.None
		}
		return nil, runtimeErr(id.Pos(),
			fmt.Sprintf("неизвестное поле '%s' (известные: %s)", id.Name, strings.Join(i.recordCtx.sortedFields, ", ")))
	}
	// (d) обычный промах вне контекста метрики.
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

// evalRunProcess активирует «запустить процесс P(args)» (006, §EN-5): вычисляет
// аргументы слева направо → runtime.StartProcess(P, args) → value.Строка{V: id}.
// Работает top-level, в функции, в теле шага (вложенный запуск — синхронно до
// первого ожидания/терминала вложенного). nil-runtime → §EN-8.A.
func (i *Interpreter) evalRunProcess(env *Environment, r *ast.RunProcessExpr) (value.Value, error) {
	if i.runtime == nil {
		return nil, runtimeErr(r.Pos(), "внутренняя ошибка: движок процессов не подключён")
	}
	args := make([]value.Value, len(r.Args))
	for k, a := range r.Args {
		v, err := i.evalExpr(env, a)
		if err != nil {
			return nil, err
		}
		args[k] = v
	}
	id, err := i.runtime.StartProcess(r.Process.Name, args)
	if err != nil {
		return nil, err
	}
	return value.Строка{V: id}, nil
}

// evalDurationLit активирует литерал длительности (006, §EN-5, D-7/D-16): парсит
// нормализованную лексему Amount в int64 → value.Длительность{Amount, Unit}. Вне
// диапазона int64 → ОшибкаВыполнения «литерал длительности вне диапазона типа Целое»
// (§EN-8.A, позиция литерала).
func (i *Interpreter) evalDurationLit(d *ast.DurationLit) (value.Value, error) {
	n, err := strconv.ParseInt(d.Amount, 10, 64)
	if err != nil {
		return nil, runtimeErr(d.Pos(), "литерал длительности вне диапазона типа Целое")
	}
	return value.Длительность{Amount: n, Unit: d.Unit}, nil
}
