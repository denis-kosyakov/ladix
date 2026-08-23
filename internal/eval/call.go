package eval

import (
	"fmt"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// evalCall резолвит и исполняет вызов (§5.2). Порядок имени f: переменная (за ( —
// «не функция») → пользовательская → активная встроенная (deferred → «не
// поддерживается») → «не объявлена». Позиция всех ошибок = CallExpr.Pos().
func (i *Interpreter) evalCall(env *Environment, c *ast.CallExpr) (value.Value, error) {
	id, ok := c.Callee.(*ast.Ident)
	if !ok {
		// не-Ident callee в v1 не порождает функций
		v, err := i.evalExpr(env, c.Callee)
		if err != nil {
			return nil, err
		}
		return nil, runtimeErr(c.Pos(), fmt.Sprintf("значение — не функция (%s), вызов невозможен", v.TypeName()))
	}
	name := id.Name

	// 1. затенение переменной: значение в v1 никогда не функция
	if v, ok := env.Lookup(name); ok {
		return nil, runtimeErr(c.Pos(), fmt.Sprintf("'%s' — не функция (%s), вызов невозможен", name, v.TypeName()))
	}
	// 2. пользовательская функция
	if decl, ok := i.funcs[name]; ok {
		return i.callUser(env, c, decl)
	}
	// 3. встроенная
	if b, ok := i.builtins[name]; ok {
		// backstop: при пустом deferredNames недостижим; поле Deferred и механизм
		// НЕ удаляются (008/§DB-6, D-DB-якорь).
		if b.Deferred {
			return nil, semErr(c.Pos(), fmt.Sprintf("функция '%s' не поддерживается в этой версии", name))
		}
		return i.callBuiltin(env, c, b)
	}
	// 4. не объявлена
	return nil, semErr(c.Pos(), fmt.Sprintf("функция '%s' не объявлена", name))
}

// callUser вызывает пользовательскую функцию (§5.3): аргументы слева направо;
// кадр parent = global (лексическая видимость); счётчик глубины; SigReturn →
// значение, SigNormal → None.
func (i *Interpreter) callUser(env *Environment, c *ast.CallExpr, decl *ast.FunctionDecl) (value.Value, error) {
	if len(c.Args) != len(decl.Params) { // защитно: фикс. арность проверена Analyze
		return nil, semErr(c.Pos(), fmt.Sprintf("'%s' принимает %d аргументов, передано %d", decl.Name.Name, len(decl.Params), len(c.Args)))
	}
	args := make([]value.Value, len(c.Args))
	for k, a := range c.Args {
		v, err := i.evalExpr(env, a)
		if err != nil {
			return nil, err
		}
		args[k] = v
	}
	i.depth++
	defer func() { i.depth-- }()
	if i.depth > i.maxDepth {
		return nil, runtimeErr(c.Pos(), fmt.Sprintf("превышена максимальная глубина вызовов (%d). Возможна бесконечная рекурсия.", i.maxDepth))
	}
	frame := NewEnvironment(i.global)
	for k, p := range decl.Params {
		frame.Define(p.Name, args[k]) // ссылочные Список/Запись — мутация видна вызывающему
	}
	sig, err := i.evalBlock(frame, decl.Body)
	if err != nil {
		return nil, err
	}
	if sig.Kind == SigReturn {
		return sig.Value, nil
	}
	return value.None, nil
}

// callBuiltin вызывает встроенную функцию: аргументы слева направо; для фикс.
// арности — защитная проверка (основная — на семпроходе), вариативные/
// перегруженные проверяют число аргументов внутри реализации.
func (i *Interpreter) callBuiltin(env *Environment, c *ast.CallExpr, b Builtin) (value.Value, error) {
	args := make([]value.Value, len(c.Args))
	for k, a := range c.Args {
		v, err := i.evalExpr(env, a)
		if err != nil {
			return nil, err
		}
		args[k] = v
	}
	if b.Arity == ArityFixed && len(args) != b.N {
		return nil, semErr(c.Pos(), fmt.Sprintf("'%s' принимает %d аргументов, передано %d", b.Name, b.N, len(args)))
	}
	return b.Fn(i, args, c.Pos())
}
