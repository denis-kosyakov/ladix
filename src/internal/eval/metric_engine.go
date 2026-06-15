package eval

import (
	"fmt"
	"math"
	"sort"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// aggregateNames — агрегатные встроенные (§10.3, §SM-8). Их аргумент — проекция
// по выжившим записям (R3); вне агрегата голое поле запрещено.
var aggregateNames = map[string]struct{}{
	"сумма":      {},
	"количество": {},
	"среднее":    {},
	"мин":        {},
	"макс":       {},
}

// evalMetric — конвейер вычисления одной метрики (§SM-8, §10.2; контракт ME-1/ME-3).
// Чистая функция по входам: те же данные источника + та же i.now() → тот же
// результат. Шаги: (2) загрузка источника + схема; (3) окно периода (глобально);
// (4) фильтр по_дате/где per-record; (5) агрегат-проекция; пустой результат §10.5.
func (i *Interpreter) evalMetric(m *ast.MetricDecl) (value.Value, error) {
	// recordCtx сброшен на входе: период:/глобальные подвыражения метрики
	// вычисляются в ГЛОБАЛЬНОЙ области (D-9, D9-1). На реентерабельном пути
	// (метрика-как-значение, D-8: внешняя метрика читает эту по имени во время
	// фильтра/проекции) внешний recordCtx сохраняется и восстанавливается через
	// defer — иначе голое имя в период:, совпавшее с полем внешней схемы, молча
	// резолвилось бы в поле вместо глобальной области.
	prevCtx := i.recordCtx
	i.recordCtx = nil
	defer func() { i.recordCtx = prevCtx }()

	// (2) Загрузка источника + схема. Источник провалидирован Analyze (Шаг 1b).
	decl := i.sources[m.Source.Name]
	records, err := i.loadSource(decl)
	if err != nil {
		return nil, err
	}
	schema, sortedFields := buildSchema(records)

	// (3) Окно периода — один раз, в ГЛОБАЛЬНОЙ области (recordCtx сброшен на входе,
	// см. выше), §SM-8 D-9.
	var winStart, winEnd value.Дата
	hasPeriod := m.Period != nil
	if hasPeriod {
		pv, err := i.evalExpr(i.global, m.Period)
		if err != nil {
			return nil, err
		}
		per, ok := pv.(value.Период)
		if !ok {
			return nil, runtimeErr(m.Period.Pos(),
				fmt.Sprintf("'период' должно давать Период, получено %s", pv.TypeName()))
		}
		winStart, winEnd, _ = periodWindow(per, i.now())
	}

	// (4) Фильтр по порядку записей: по_дате ∈ окно И где=истина.
	surviving := make([]value.Запись, 0, len(records))
	for _, r := range records {
		keep, err := i.recordSurvives(m, r, schema, sortedFields, hasPeriod, winStart, winEnd)
		if err != nil {
			return nil, err
		}
		if keep {
			surviving = append(surviving, r)
		}
	}

	// (5) Пустой набор выживших → решение по КОРНЮ m.Aggregate ДО вычисления
	// (§SM-8 шаг 5, §10.5, ревью №1 D4-1): корневой единичный сумма/количество →
	// Целое 0; единичный среднее/мин/макс ИЛИ любое составное (деривативное)
	// выражение → Пусто коротким замыканием, НЕ спускаясь в evalAggExpr/арифметику
	// (иначе сумма(x)/количество(y) дало бы 0/0, а сумма(x)+1 → Целое 1).
	if len(surviving) == 0 {
		return emptyWindowResult(m.Aggregate), nil
	}

	// (5) Агрегат-проекция (R3) поверх выживших.
	return i.evalAggExpr(m.Aggregate, surviving, schema, sortedFields)
}

// nounToAdverb отображает существительное «последнего завершённого периода»
// («прошлый <noun>», §MW-5) в базовый календарный адверб value.Период: день→
// ежедневно … год→ежегодно. Обратная к value.nounFromAdverb. Чистая функция;
// для незнакомого noun возвращает "" — periodWindow на нём даст ok=false (защитно;
// семантика §MW-SEM-3 отвергает неизвестный noun ДО eval, так что в норме сюда
// приходит только валидный noun).
func nounToAdverb(noun string) string {
	switch noun {
	case "день":
		return "ежедневно"
	case "неделя":
		return "еженедельно"
	case "месяц":
		return "ежемесячно"
	case "квартал":
		return "ежеквартально"
	case "год":
		return "ежегодно"
	}
	return ""
}

// emptyWindowResult выбирает результат метрики на ПУСТОМ наборе выживших по КОРНЮ
// выражения «агрегат:» (§SM-8 шаг 5, §10.5, ревью №1 D4-1) — ДО любого вычисления.
// Корневой единичный вызов сумма/количество → Целое 0; всё прочее (единичные
// среднее/мин/макс ИЛИ составное деривативное выражение, напр. средний чек
// сумма(x)/количество(запись)) → Пусто. Короткое замыкание на КОРНЕ (а не per-leaf)
// не даёт деривативу схлопнуться в 0/0 или Целое 1.
func emptyWindowResult(root ast.Expression) value.Value {
	if call, ok := root.(*ast.CallExpr); ok && len(call.Args) == 1 {
		if id, ok := call.Callee.(*ast.Ident); ok {
			switch id.Name {
			case "сумма", "количество":
				return value.Целое{V: 0}
			}
		}
	}
	return value.None
}

// recordSurvives применяет дату-фильтр и фильтр «где» к одной записи в scope полей
// (§SM-8 шаг 3). recordCtx сохраняется/восстанавливается реентерабельно (D-9,
// метрика-как-значение).
func (i *Interpreter) recordSurvives(m *ast.MetricDecl, r value.Запись,
	schema map[string]struct{}, sortedFields []string,
	hasPeriod bool, winStart, winEnd value.Дата) (bool, error) {
	prev := i.recordCtx
	i.recordCtx = &recordContext{rec: r, schema: schema, sortedFields: sortedFields}
	defer func() { i.recordCtx = prev }()

	// Дата-фильтр (§10.4): по_дате=Пусто → исключить без проверки где/без ошибки.
	if hasPeriod {
		dv, err := i.evalExpr(i.global, m.ByDate)
		if err != nil {
			return false, err
		}
		if _, isNone := dv.(value.Пусто); isNone {
			return false, nil
		}
		d, ok := dv.(value.Дата)
		if !ok {
			return false, runtimeErr(m.ByDate.Pos(),
				fmt.Sprintf("'по_дате' должно давать Дата или Пусто, получено %s", dv.TypeName()))
		}
		if c, _ := value.Compare(d, winStart); c < 0 {
			return false, nil
		}
		if c, _ := value.Compare(d, winEnd); c > 0 {
			return false, nil
		}
	}

	// Фильтр «где» (§SM-8 шаг 3.2): отсутствует → истина; не Булево → §SM-9.C.
	if m.Where != nil {
		wv, err := i.evalExpr(i.global, m.Where)
		if err != nil {
			return false, err
		}
		b, ok := wv.(value.Булево)
		if !ok {
			return false, typeErr(m.Where.Pos(),
				fmt.Sprintf("'где' должно давать Булево, получено %s", wv.TypeName()))
		}
		if !b.V {
			return false, nil
		}
	}
	return true, nil
}

// evalAggExpr вычисляет выражение «агрегат» поверх выживших записей (R3, §10.3).
// Случаи:
//   - корневой агрегатный вызов f(projExpr) (f ∈ aggregateNames): projExpr — проекция
//     по записям (пропуская Пусто, §10.3); пустая проекция → дефолт §10.5; иначе
//     переиспользуем существующий builtin Fn агрегата (НЕ дублируем);
//   - голое поле вне агрегата → §SM-9.C «использовано вне агрегатной функции»;
//   - иное (литерал/глобаль/BinaryExpr/др. метрика) — рекурсивно/в global.
func (i *Interpreter) evalAggExpr(expr ast.Expression, surviving []value.Запись,
	schema map[string]struct{}, sortedFields []string) (value.Value, error) {
	switch ex := expr.(type) {
	case *ast.CallExpr:
		if id, ok := ex.Callee.(*ast.Ident); ok {
			if _, isAgg := aggregateNames[id.Name]; isAgg && len(ex.Args) == 1 {
				return i.evalAggregateCall(id.Name, ex, surviving, schema, sortedFields)
			}
		}
		// Не-агрегатный вызов: подвыражения не в scope полей (вне агрегата) —
		// вычисляем в global (recordCtx сброшен на входе в evalMetric, см. выше).
		return i.evalExpr(i.global, expr)

	case *ast.Ident:
		// Голое имя на верхнем уровне «агрегат:». Если резолвится в поле/«запись» —
		// это поле вне агрегата (§10.3, §SM-9.C). Глобаль/функция/др. — допустимо.
		if ex.Name == "запись" {
			return nil, runtimeErr(ex.Pos(),
				fmt.Sprintf("поле '%s' использовано вне агрегатной функции", ex.Name))
		}
		if _, isVar := i.global.Lookup(ex.Name); isVar {
			return i.evalExpr(i.global, expr)
		}
		if i.isFunctionName(ex.Name) || i.sourceOrMetric(ex.Name) {
			return i.evalExpr(i.global, expr)
		}
		if _, ok := schema[ex.Name]; ok {
			return nil, runtimeErr(ex.Pos(),
				fmt.Sprintf("поле '%s' использовано вне агрегатной функции", ex.Name))
		}
		return i.evalExpr(i.global, expr)

	case *ast.BinaryExpr:
		// Деривативное выражение (например сумма(x)/количество(y)): операнды могут
		// содержать агрегаты/поля — вычисляем рекурсивно через evalAggExpr, затем
		// комбинируем существующими операторами (НЕ переразбираем арифметику).
		left, err := i.evalAggExpr(ex.Left, surviving, schema, sortedFields)
		if err != nil {
			return nil, err
		}
		right, err := i.evalAggExpr(ex.Right, surviving, schema, sortedFields)
		if err != nil {
			return nil, err
		}
		return i.combineBinary(ex, left, right)

	case *ast.UnaryExpr:
		operand, err := i.evalAggExpr(ex.Operand, surviving, schema, sortedFields)
		if err != nil {
			return nil, err
		}
		return i.combineUnary(ex, operand)

	default:
		// Литералы/прочее без полей — вычисляем напрямую (recordCtx сброшен).
		return i.evalExpr(i.global, expr)
	}
}

// combineBinary комбинирует уже вычисленные операнды деривативного выражения
// «агрегат» существующими операторами (переиспользует evalAdd/evalSubMul/… —
// не переразбирает арифметику). Логические и/или над агрегатами в v1 не нужны для
// golden, но поддержаны для полноты (без короткого замыкания — операнды уже есть).
func (i *Interpreter) combineBinary(b *ast.BinaryExpr, left, right value.Value) (value.Value, error) {
	switch b.Op {
	case ast.OpAdd:
		return i.evalAdd(b, left, right)
	case ast.OpSub, ast.OpMul:
		return i.evalSubMul(b, left, right)
	case ast.OpDiv:
		return i.evalDiv(b, left, right)
	case ast.OpFloorDiv:
		return i.evalFloorDiv(b, left, right)
	case ast.OpMod:
		return i.evalMod(b, left, right)
	case ast.OpEq:
		return value.Булево{V: value.Equal(left, right)}, nil
	case ast.OpNeq:
		return value.Булево{V: !value.Equal(left, right)}, nil
	case ast.OpLt, ast.OpLe, ast.OpGt, ast.OpGe:
		return i.evalOrder(b, left, right)
	case ast.OpAnd, ast.OpOr:
		lb, ok := left.(value.Булево)
		if !ok {
			return nil, typeErr(b.Pos(), fmt.Sprintf("'%s' требует Булево, получено %s", b.Op.String(), left.TypeName()))
		}
		rb, ok := right.(value.Булево)
		if !ok {
			return nil, typeErr(b.Pos(), fmt.Sprintf("'%s' требует Булево, получено %s", b.Op.String(), right.TypeName()))
		}
		if b.Op == ast.OpAnd {
			return value.Булево{V: lb.V && rb.V}, nil
		}
		return value.Булево{V: lb.V || rb.V}, nil
	}
	return nil, runtimeErr(b.Pos(), "внутренняя ошибка: неизвестный бинарный оператор")
}

// combineUnary комбинирует уже вычисленный операнд унарного деривативного
// выражения «агрегат».
func (i *Interpreter) combineUnary(u *ast.UnaryExpr, operand value.Value) (value.Value, error) {
	switch u.Op {
	case ast.OpNeg:
		switch v := operand.(type) {
		case value.Целое:
			if v.V == math.MinInt64 {
				return nil, runtimeErr(u.Pos(), "переполнение целого числа")
			}
			return value.Целое{V: -v.V}, nil
		case value.Дробное:
			return value.Дробное{V: -v.V}, nil
		default:
			return nil, typeErr(u.Pos(), fmt.Sprintf("унарный '-' нельзя применить к %s", operand.TypeName()))
		}
	case ast.OpNot:
		if v, ok := operand.(value.Булево); ok {
			return value.Булево{V: !v.V}, nil
		}
		return nil, typeErr(u.Pos(), fmt.Sprintf("'не' нельзя применить к %s", operand.TypeName()))
	}
	return nil, runtimeErr(u.Pos(), "внутренняя ошибка: неизвестный унарный оператор")
}

// evalAggregateCall строит проекцию для одного агрегатного вызова f(projExpr) и
// зовёт существующий builtin Fn агрегата (R3). Сначала запрещает вложенный агрегат
// в projExpr (§SM-9.C). Пустая проекция → дефолт §10.5 (короткое замыкание ДО Fn).
func (i *Interpreter) evalAggregateCall(name string, call *ast.CallExpr, surviving []value.Запись,
	schema map[string]struct{}, sortedFields []string) (value.Value, error) {
	projExpr := call.Args[0]
	if nested := findNestedAggregate(projExpr); nested != nil {
		return nil, runtimeErr(nested.Pos(), "вложенный агрегат недопустим")
	}

	projection := make([]value.Value, 0, len(surviving))
	for _, r := range surviving {
		v, err := i.projectRecord(projExpr, r, schema, sortedFields)
		if err != nil {
			return nil, err
		}
		if _, isNone := v.(value.Пусто); isNone {
			continue // §10.3: пропускаем записи, где проекция дала Пусто
		}
		projection = append(projection, v)
	}

	// Пустая проекция → дефолт §10.5 (НЕ зовём builtin: иначе «<имя>: список пуст»).
	if len(projection) == 0 {
		switch name {
		case "сумма", "количество":
			return value.Целое{V: 0}, nil
		default: // среднее/мин/макс
			return value.None, nil
		}
	}

	lst := value.NewList(projection)
	b := i.builtins[name]
	return b.Fn(i, []value.Value{lst}, call.Pos())
}

// projectRecord вычисляет projExpr на одной записи в scope полей (реентерабельно).
func (i *Interpreter) projectRecord(projExpr ast.Expression, r value.Запись,
	schema map[string]struct{}, sortedFields []string) (value.Value, error) {
	prev := i.recordCtx
	i.recordCtx = &recordContext{rec: r, schema: schema, sortedFields: sortedFields}
	defer func() { i.recordCtx = prev }()
	return i.evalExpr(i.global, projExpr)
}

// sourceOrMetric сообщает, известно ли имя как источник или метрика.
func (i *Interpreter) sourceOrMetric(name string) bool {
	if _, ok := i.sources[name]; ok {
		return true
	}
	_, ok := i.metrics[name]
	return ok
}

// buildSchema строит схему источника: объединение ключей всех записей (§9.5) и
// отсортированный по возрастанию список полей (детерминизм golden, §SM-8.1).
func buildSchema(records []value.Запись) (map[string]struct{}, []string) {
	schema := map[string]struct{}{}
	for _, r := range records {
		for _, k := range r.Keys() {
			schema[k] = struct{}{}
		}
	}
	sorted := make([]string, 0, len(schema))
	for k := range schema {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	return schema, sorted
}

// findNestedAggregate ищет в AST-выражении любой вложенный агрегатный вызов
// (CallExpr с callee-Ident ∈ aggregateNames) и возвращает его узел или nil
// (§10.3, §SM-9.C). Обход рекурсивный по всем подвыражениям.
func findNestedAggregate(e ast.Expression) ast.Expression {
	switch ex := e.(type) {
	case *ast.CallExpr:
		if id, ok := ex.Callee.(*ast.Ident); ok {
			if _, isAgg := aggregateNames[id.Name]; isAgg {
				return ex
			}
		}
		if n := findNestedAggregate(ex.Callee); n != nil {
			return n
		}
		for _, a := range ex.Args {
			if n := findNestedAggregate(a); n != nil {
				return n
			}
		}
	case *ast.BinaryExpr:
		if n := findNestedAggregate(ex.Left); n != nil {
			return n
		}
		return findNestedAggregate(ex.Right)
	case *ast.UnaryExpr:
		return findNestedAggregate(ex.Operand)
	case *ast.IndexExpr:
		if n := findNestedAggregate(ex.Target); n != nil {
			return n
		}
		return findNestedAggregate(ex.Index)
	case *ast.FieldExpr:
		return findNestedAggregate(ex.Target)
	case *ast.ListLit:
		for _, el := range ex.Elements {
			if n := findNestedAggregate(el); n != nil {
				return n
			}
		}
	}
	return nil
}
