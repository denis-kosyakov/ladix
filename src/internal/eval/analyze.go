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

	// Шаг 1 — объединённый упорядоченный проход по top-level декларациям
	// (FunctionDecl+SourceDecl+MetricDecl) в общем глобальном пространстве имён
	// (§SM-4). declLine — строка первого объявления имени; declIsFunc — было ли
	// первое объявление функцией (для различения текста повтора, R5).
	declLine := map[string]int{}
	declIsFunc := map[string]bool{}
	for _, item := range prog.Items {
		var name string
		var pos ast.Position
		isFunc := false
		switch d := item.(type) {
		case *ast.FunctionDecl:
			name, pos, isFunc = d.Name.Name, d.Name.Pos(), true
		case *ast.SourceDecl:
			name, pos = d.Name.Name, d.Name.Pos()
		case *ast.MetricDecl:
			name, pos = d.Name.Name, d.Name.Pos()
		default:
			continue
		}
		if line, exists := declLine[name]; exists {
			// функция↔функция → старый текст (регресс 003, R5); любая коллизия
			// с участием источника/метрики → общий §SM-9.A текст.
			if isFunc && declIsFunc[name] {
				return semErr(pos, fmt.Sprintf("функция '%s' уже объявлена в строке %d", name, line))
			}
			return semErr(pos, fmt.Sprintf("'%s' уже объявлено в строке %d", name, line))
		}
		declLine[name] = pos.Line
		declIsFunc[name] = isFunc
		switch d := item.(type) {
		case *ast.FunctionDecl:
			i.funcs[name] = d
		case *ast.SourceDecl:
			i.sources[name] = d
		case *ast.MetricDecl:
			i.metrics[name] = d
		}
	}

	// Шаг 1b — статическая валидация метрик: обязательность источник/агрегат,
	// связка период↔по_дате, резолв источника (§SM-4, §SM-9.A). НЕ резолвит поля
	// записи/типы/голое поле/цикл — это eval-time (D-5/D-6/D-8).
	for _, item := range prog.Items {
		md, ok := item.(*ast.MetricDecl)
		if !ok {
			continue
		}
		if err := i.checkMetricDecl(md); err != nil {
			return err
		}
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

// checkMetricDecl — статическая валидация одной метрики (Шаг 1b, §SM-4). Порядок:
// (1) обязательность источник/агрегат; (2) пара период↔по_дате; (3) резолв
// источника. Тексты §SM-9.A дословно; fail-fast. Поля/типы/голое поле/цикл не
// резолвятся — eval-time. Присутствие атрибута определяется по ненулевой строке
// его позиции в Attrs (парсер выставляет Line!=0 ровно для присутствующих).
func (i *Interpreter) checkMetricDecl(m *ast.MetricDecl) error {
	name := m.Name.Name
	// (1) обязательные атрибуты источник/агрегат (поз. токена метрика).
	if m.Attrs.SourcePos.Line == 0 {
		return semErr(m.Pos(), fmt.Sprintf("метрика '%s': отсутствует обязательный атрибут 'источник'", name))
	}
	if m.Attrs.AggregatePos.Line == 0 {
		return semErr(m.Pos(), fmt.Sprintf("метрика '%s': отсутствует обязательный атрибут 'агрегат'", name))
	}
	// (2) связка период↔по_дате.
	hasPeriod := m.Attrs.PeriodPos.Line != 0
	hasByDate := m.Attrs.ByDatePos.Line != 0
	if hasPeriod && !hasByDate {
		return semErr(m.Attrs.PeriodPos, fmt.Sprintf("метрика '%s': 'период' требует 'по_дате'", name))
	}
	if hasByDate && !hasPeriod {
		return semErr(m.Attrs.ByDatePos, fmt.Sprintf("метрика '%s': 'по_дате' без 'период' не имеет смысла", name))
	}
	// (3) резолв источника: имя должно быть зарегистрированным источником.
	src := m.Source.Name
	if _, ok := i.sources[src]; !ok {
		if _, fn := i.funcs[src]; fn {
			return semErr(m.Attrs.SourcePos, fmt.Sprintf("метрика '%s': '%s' — не источник", name, src))
		}
		if _, mt := i.metrics[src]; mt {
			return semErr(m.Attrs.SourcePos, fmt.Sprintf("метрика '%s': '%s' — не источник", name, src))
		}
		return semErr(m.Attrs.SourcePos, fmt.Sprintf("метрика '%s': источник '%s' не объявлен", name, src))
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
