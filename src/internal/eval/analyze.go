package eval

import (
	"fmt"

	"github.com/denis-kosyakov/ladix/internal/ast"
)

// Analyze — семантический проход стадии 3 (§9, FR-034/035). Лёгкий fail-fast
// обход AST ДО исполнения: регистрирует top-level функции (forward-вызовы
// разрешены), проверяет повтор объявлений, контекст вернуть/прервать/продолжить,
// резолв вызовов + ФИКС. арность и deferred-границу. НЕ проверяет: declaredness
// переменной в позиции значения (flow-зависимо), типы, арность вариативных/
// перегруженных встроенных.
func (i *Interpreter) Analyze(prog *ast.Program) error {
	i.analyzed = true

	// Шаг 1 — регистрация top-level FunctionDecl.
	declLine := map[string]int{}
	for _, item := range prog.Items {
		fd, ok := item.(*ast.FunctionDecl)
		if !ok {
			continue
		}
		name := fd.Name.Name
		if line, exists := declLine[name]; exists {
			return semErr(fd.Name.Pos(), fmt.Sprintf("функция '%s' уже объявлена в строке %d", name, line))
		}
		declLine[name] = fd.Pos().Line
		i.funcs[name] = fd
	}

	// Шаг 2 — обход глобальной области и тел функций (блоки область не открывают).
	var globalStmts []ast.Statement
	for _, item := range prog.Items {
		if st, ok := item.(ast.Statement); ok {
			globalStmts = append(globalStmts, st)
		}
	}
	if err := i.analyzeArea(globalStmts, nil, false, 0); err != nil {
		return err
	}
	for _, item := range prog.Items {
		if fd, ok := item.(*ast.FunctionDecl); ok {
			if err := i.analyzeArea(fd.Body.Stmts, fd.Params, true, 0); err != nil {
				return err
			}
		}
	}
	return nil
}

// analyzeArea анализирует одну область (кадр функции / global). Два подпрохода:
// (1) собрать множество имён-переменных области и поймать повтор пусть/для;
// (2) проверить контексты сигналов, резолв вызовов и deferred-узлы.
func (i *Interpreter) analyzeArea(stmts []ast.Statement, params []ast.Ident, inFunction bool, loopDepth int) error {
	letLine := map[string]int{} // имя пусть/параметра → строка первого объявления
	vars := map[string]bool{}   // ВСЕ имена-переменные области (пусть+параметры+для)
	for _, p := range params {
		letLine[p.Name] = p.Pos().Line
		vars[p.Name] = true
	}
	if err := collectVars(stmts, letLine, vars); err != nil {
		return err
	}
	return i.checkStmts(stmts, vars, inFunction, loopDepth)
}

// collectVars рекурсивно собирает переменные области (нисходя в блоки, но не
// открывая новых областей) и ловит повтор объявления (SEM-REDECL-VAR). Переменная
// для НЕ добавляется в letLine — два «для x» подряд легальны (D-R13), но «пусть x»
// при объявленном x (и наоборот) — ошибка.
func collectVars(stmts []ast.Statement, letLine map[string]int, vars map[string]bool) error {
	for _, s := range stmts {
		switch st := s.(type) {
		case *ast.LetStmt:
			name := st.Name.Name
			if line, ok := letLine[name]; ok {
				return semErr(st.Name.Pos(), fmt.Sprintf("'%s' уже объявлено в строке %d", name, line))
			}
			letLine[name] = st.Name.Pos().Line
			vars[name] = true
		case *ast.ForStmt:
			name := st.Var.Name
			if line, ok := letLine[name]; ok {
				return semErr(st.Var.Pos(), fmt.Sprintf("'%s' уже объявлено в строке %d", name, line))
			}
			vars[name] = true
			if err := collectVars(st.Body.Stmts, letLine, vars); err != nil {
				return err
			}
		case *ast.IfStmt:
			if err := collectVars(st.Then.Stmts, letLine, vars); err != nil {
				return err
			}
			if err := collectVarsElse(st.Else, letLine, vars); err != nil {
				return err
			}
		case *ast.WhileStmt:
			if err := collectVars(st.Body.Stmts, letLine, vars); err != nil {
				return err
			}
		}
	}
	return nil
}

func collectVarsElse(e *ast.ElseClause, letLine map[string]int, vars map[string]bool) error {
	if e == nil {
		return nil
	}
	if e.IsFinal() {
		return collectVars(e.Body.Stmts, letLine, vars)
	}
	if err := collectVars(e.Then.Stmts, letLine, vars); err != nil {
		return err
	}
	return collectVarsElse(e.Else, letLine, vars)
}

// checkStmts — второй подпроход: контексты сигналов, вызовы, deferred-узлы.
func (i *Interpreter) checkStmts(stmts []ast.Statement, vars map[string]bool, inFunction bool, loopDepth int) error {
	for _, s := range stmts {
		if err := i.checkStmt(s, vars, inFunction, loopDepth); err != nil {
			return err
		}
	}
	return nil
}

func (i *Interpreter) checkStmt(s ast.Statement, vars map[string]bool, inFunction bool, loopDepth int) error {
	switch st := s.(type) {
	case *ast.LetStmt:
		return i.checkExpr(st.Value, vars)
	case *ast.AssignStmt:
		return i.checkExpr(st.Value, vars)
	case *ast.ExpressionStmt:
		return i.checkExpr(st.Expr, vars)
	case *ast.ReturnStmt:
		if !inFunction {
			return semErr(st.Pos(), "'вернуть' допустимо только внутри функции")
		}
		if st.Value != nil {
			return i.checkExpr(st.Value, vars)
		}
		return nil
	case *ast.BreakStmt:
		if loopDepth == 0 {
			return semErr(st.Pos(), "'прервать' допустимо только внутри 'пока' или 'для'")
		}
		return nil
	case *ast.ContinueStmt:
		if loopDepth == 0 {
			return semErr(st.Pos(), "'продолжить' допустимо только внутри 'пока' или 'для'")
		}
		return nil
	case *ast.IfStmt:
		if err := i.checkExpr(st.Cond, vars); err != nil {
			return err
		}
		if err := i.checkStmts(st.Then.Stmts, vars, inFunction, loopDepth); err != nil {
			return err
		}
		return i.checkElse(st.Else, vars, inFunction, loopDepth)
	case *ast.WhileStmt:
		if err := i.checkExpr(st.Cond, vars); err != nil {
			return err
		}
		return i.checkStmts(st.Body.Stmts, vars, inFunction, loopDepth+1)
	case *ast.ForStmt:
		if err := i.checkExpr(st.Iterable, vars); err != nil {
			return err
		}
		return i.checkStmts(st.Body.Stmts, vars, inFunction, loopDepth+1)
	case *ast.AssignAction, *ast.CallAction, *ast.NotifyAction:
		return i.deferredConstruct(st)
	}
	return nil
}

func (i *Interpreter) checkElse(e *ast.ElseClause, vars map[string]bool, inFunction bool, loopDepth int) error {
	if e == nil {
		return nil
	}
	if e.IsFinal() {
		return i.checkStmts(e.Body.Stmts, vars, inFunction, loopDepth)
	}
	if err := i.checkExpr(e.Cond, vars); err != nil {
		return err
	}
	if err := i.checkStmts(e.Then.Stmts, vars, inFunction, loopDepth); err != nil {
		return err
	}
	return i.checkElse(e.Else, vars, inFunction, loopDepth)
}

// checkExpr рекурсивно ищет deferred-узлы (RunProcessExpr/DurationLit) и проверяет
// резолв CallExpr + фикс. арность. Плоский Ident НЕ проверяется (declaredness —
// рантайму).
func (i *Interpreter) checkExpr(e ast.Expression, vars map[string]bool) error {
	switch ex := e.(type) {
	case *ast.BinaryExpr:
		if err := i.checkExpr(ex.Left, vars); err != nil {
			return err
		}
		return i.checkExpr(ex.Right, vars)
	case *ast.UnaryExpr:
		return i.checkExpr(ex.Operand, vars)
	case *ast.CallExpr:
		for _, a := range ex.Args {
			if err := i.checkExpr(a, vars); err != nil {
				return err
			}
		}
		return i.checkCall(ex, vars)
	case *ast.IndexExpr:
		if err := i.checkExpr(ex.Target, vars); err != nil {
			return err
		}
		return i.checkExpr(ex.Index, vars)
	case *ast.FieldExpr:
		return i.checkExpr(ex.Target, vars)
	case *ast.ListLit:
		for _, el := range ex.Elements {
			if err := i.checkExpr(el, vars); err != nil {
				return err
			}
		}
		return nil
	case *ast.RunProcessExpr:
		return i.deferredConstruct(ex)
	case *ast.DurationLit:
		return i.deferredConstruct(ex)
	}
	return nil
}

// checkCall резолвит вызов на семпроходе (§9): имя-переменная области → рантайм;
// пользовательская/активная встроенная фикс. арности → проверка числа аргументов;
// deferred-встроенная → SEM-DEFERRED-BUILTIN; иначе → SEM-FN-UNDECLARED.
func (i *Interpreter) checkCall(c *ast.CallExpr, vars map[string]bool) error {
	id, ok := c.Callee.(*ast.Ident)
	if !ok {
		return i.checkExpr(c.Callee, vars)
	}
	name := id.Name
	if vars[name] {
		return nil // затенение переменной → решает рантайм (§5.2)
	}
	if decl, ok := i.funcs[name]; ok {
		if len(c.Args) != len(decl.Params) {
			return semErr(c.Pos(), fmt.Sprintf("'%s' принимает %d аргументов, передано %d", name, len(decl.Params), len(c.Args)))
		}
		return nil
	}
	if b, ok := i.builtins[name]; ok {
		if b.Deferred {
			return semErr(c.Pos(), fmt.Sprintf("функция '%s' не поддерживается в этой версии", name))
		}
		if b.Arity == ArityFixed && len(c.Args) != b.N {
			return semErr(c.Pos(), fmt.Sprintf("'%s' принимает %d аргументов, передано %d", name, b.N, len(c.Args)))
		}
		return nil // вариативная/перегруженная — арность в рантайме
	}
	return semErr(c.Pos(), fmt.Sprintf("функция '%s' не объявлена", name))
}
