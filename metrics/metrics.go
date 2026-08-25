// Package metrics — публичный исполнитель метрик Ladix (openspec/changes/
// 030-public-metrics-evaluator). Вычисляет ir.Metric над данными потребителя
// ([]map[string]any), переиспользуя семантику internal/eval (окно периода →
// фильтр → агрегат, §SM-8) — тот же движок, что использует reference-CLI
// «ladix metric». Публично экспортируются только Date/Options/Result/Evaluate
// и три программных сентинела; никакие типы AST/value наружу не текут (Д-1).
package metrics

import (
	"errors"
	"fmt"
	"io"

	"github.com/denis-kosyakov/ladix/internal/ast"
	lerrors "github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/value"
	"github.com/denis-kosyakov/ladix/ir"
)

// Date — календарная дата «сегодня» (Д-3): значение, не интерфейс. Инжектируется
// потребителем в Options.Today; wall-clock в пакете отсутствует конструктивно.
type Date struct{ Year, Month, Day int }

// Options — параметры исполнения (Д-11, заморожено для v0.2.0).
type Options struct {
	Today    Date              // обязательна: дата среза, окно периода строится от неё
	Fields   map[string]string // Д-7: схема полей источника (имя поля → имя типа Ladix), необязательна
	MaxDepth int               // 0 → DefaultMaxDepth интерпретатора
}

// Result — результат исполнения метрики (Д-11).
type Result struct {
	Type  string // имя типа Ladix: "Целое", "Дробное", "Строка", "Булево", "Пусто", "Дата", ...
	Text  string // каноническая печать — та же, что печатает `ladix metric`
	Value any    // Go-значение в JSON-семантике (Дата/Длительность/Период — строкой, равной Text)
}

var (
	// ErrInvalidOptions — Options некорректны (дата, неизвестное имя типа в Fields,
	// невалидное имя поля).
	ErrInvalidOptions = errors.New("metrics: некорректные Options")
	// ErrEvaluation — метрика не вычислена; подробности — в возвращённых диагностиках.
	ErrEvaluation = errors.New("metrics: метрика не вычислена")
	// ErrInternal — сработал recover-барьер (Принцип III): паника внутри фасада
	// не пересекла границу API как паника.
	ErrInternal = errors.New("metrics: внутренний сбой исполнения")
)

// stageRuntime — значение Stage диагностик этого пакета (Д-5). Строковый литерал,
// НЕ константа ir: граница диффа ir пуста (§MC-3, словарь Stage толерантен к
// неизвестным значениям).
const stageRuntime = "runtime"

// Evaluate вычисляет метрику m над records по семантике SPEC §10 (§SM-8), с той
// же арифметикой/типизацией, что у reference-CLI `ladix metric`. Опции задают
// дату среза (обязательна) и схему полей источника (Д-7).
//
// Контракт возвратов (Д-11): err == nil ⟺ результат получен; иначе диагностики
// несут пользовательский текст (Severity="error", Stage="runtime"), а err —
// один из ErrInvalidOptions/ErrEvaluation/ErrInternal (errors.Is).
func Evaluate(m ir.Metric, records []map[string]any, opts Options) (result Result, diags []ir.Diagnostic, err error) {
	// Recover-барьер (Принцип III, design.md Д-5): паника внутри фасада не
	// пересекает границу API. Без Go stack trace в тексте — короткое сообщение.
	defer func() {
		if r := recover(); r != nil {
			result, diags, err = Result{}, nil, fmt.Errorf("%w: %v", ErrInternal, r)
		}
	}()

	if verr := validateOptions(opts); verr != nil {
		return Result{}, nil, verr
	}

	// Замок на структурную инъекцию через многострочный атрибут (см.
	// multilineAttrDiag): проверяется ДО сборки синтетического текста.
	if d, bad := multilineAttrDiag(m); bad {
		return Result{}, []ir.Diagnostic{d}, fmt.Errorf("%w", ErrEvaluation)
	}

	tmpl, terr := buildTemplate(m, opts)
	if terr != nil {
		return Result{}, nil, terr
	}

	// Лексер/парсер синтетической декларации (Д-10).
	toks, lexErrs := lexer.New(tmpl.text).Tokenize()
	if lexErrs != nil && !lexErrs.Empty() {
		d := tmpl.invalidExprDiag(m.Name, firstLexPos(lexErrs))
		return Result{}, []ir.Diagnostic{d}, fmt.Errorf("%w", ErrEvaluation)
	}
	perrs := lerrors.NewErrorList()
	prog := parser.New(toks, perrs).Parse()
	if !perrs.Empty() {
		d := tmpl.invalidExprDiag(m.Name, firstParsePos(perrs))
		return Result{}, []ir.Diagnostic{d}, fmt.Errorf("%w", ErrEvaluation)
	}

	md := findMetricDecl(prog)
	sd := findSourceDecl(prog)
	if md == nil || sd == nil {
		return Result{}, nil, fmt.Errorf("%w: синтетическая программа не породила ожидаемые декларации", ErrInternal)
	}

	// Извлечение 4 выражений; агрегат обязателен структурно (реальный ir.Metric
	// всегда его несёт) — его отсутствие после разбора трактуется как невалидное
	// каноническое выражение атрибута «агрегат:» (design.md §3, «нужное выражение
	// не извлеклось»).
	if md.Aggregate == nil {
		d := tmpl.missingAggregateDiag(m.Name)
		return Result{}, []ir.Diagnostic{d}, fmt.Errorf("%w", ErrEvaluation)
	}

	// Потолок сложности (design.md Д-4) — ДО вычисления, порядок где/агрегат/
	// период/по_дате.
	if d, over := tmpl.checkComplexity(m.Name, md); over {
		return Result{}, []ir.Diagnostic{d}, fmt.Errorf("%w", ErrEvaluation)
	}

	// Семантический проход (предусловие EvalMetricPipeline, design.md Д-12).
	clock := eval.FixedClock{D: value.Дата{Year: opts.Today.Year, Month: opts.Today.Month, Day: opts.Today.Day}}
	interp := eval.NewInterpreter(io.Discard, opts.MaxDepth, clock)
	if aerr := interp.Analyze(prog); aerr != nil {
		if d, ok := tmpl.runtimeDiag(aerr); ok {
			return Result{}, []ir.Diagnostic{d}, fmt.Errorf("%w", ErrEvaluation)
		}
		return Result{}, nil, fmt.Errorf("%w: %v", ErrInternal, aerr)
	}

	// Конвертация записей потребителя (Д-2/Д-8/Д-9) + приведение по схеме (Д-7).
	valRecords, cerr := convertRecords(m.Source, records)
	if cerr != nil {
		return Result{}, []ir.Diagnostic{*cerr}, fmt.Errorf("%w", ErrEvaluation)
	}
	// Приведение по схеме (Д-7) — ТЕМ ЖЕ кодом, что у загрузчика источника CLI
	// (eval.ApplySourceSchema → applySchema/coerceField): тексты §SC-9.B и
	// результат дословно совпадают, дубликата в пакете нет.
	// Имя источника в текстах §SC-9.B — РЕАЛЬНОЕ (m.Source), а не синтетический
	// идентификатор шаблона: копия декларации с подменённым Name живёт только как
	// Go-структура и НЕ переразбирается, поэтому подстановки в текст программы
	// (и риска инъекции, Д-10) здесь нет.
	sdNamed := *sd
	sdNamed.Name = ast.Ident{Name: m.Source}
	coerced, serr := interp.ApplySourceSchema(&sdNamed, valRecords)
	if serr != nil {
		if d, ok := schemaDiag(serr); ok {
			return Result{}, []ir.Diagnostic{d}, fmt.Errorf("%w", ErrEvaluation)
		}
		return Result{}, nil, fmt.Errorf("%w: %v", ErrInternal, serr)
	}
	valRecords = coerced

	v, everr := interp.EvalMetricPipeline(valRecords, md.Where, md.Aggregate, md.Period, md.ByDate)
	if everr != nil {
		if d, ok := tmpl.runtimeDiag(everr); ok {
			return Result{}, []ir.Diagnostic{d}, fmt.Errorf("%w", ErrEvaluation)
		}
		return Result{}, nil, fmt.Errorf("%w: %v", ErrInternal, everr)
	}

	return toResult(v), nil, nil
}

// findMetricDecl/findSourceDecl ищут декларации в разобранной синтетической
// программе по типу (порядок Items не хардкодится — устойчиво к перестановке
// шаблона).
func findMetricDecl(prog *ast.Program) *ast.MetricDecl {
	for _, it := range prog.Items {
		if md, ok := it.(*ast.MetricDecl); ok {
			return md
		}
	}
	return nil
}

func findSourceDecl(prog *ast.Program) *ast.SourceDecl {
	for _, it := range prog.Items {
		if sd, ok := it.(*ast.SourceDecl); ok {
			return sd
		}
	}
	return nil
}

// firstLexPos/firstParsePos возвращают позицию первой накопленной ошибки.
func firstLexPos(l *lerrors.ErrorList) position {
	for _, e := range l.Errors() {
		var lex lerrors.LexError
		if errors.As(e, &lex) {
			return position{Line: lex.Pos.Line, Col: lex.Pos.Col}
		}
	}
	return position{}
}

func firstParsePos(l *lerrors.ErrorList) position {
	for _, e := range l.Errors() {
		var pe lerrors.ParseError
		if errors.As(e, &pe) {
			return position{Line: pe.Pos.Line, Col: pe.Pos.Col}
		}
	}
	return position{}
}

// toResult конвертирует value.Value в публичный Result (Д-11 §8): Type/Text —
// каноническое имя и печать; Value — Go-значение в JSON-семантике (Список/
// Запись — рекурсивно; Дата/Длительность/Период — строкой, равной Text).
func toResult(v value.Value) Result {
	text := value.String(v)
	res := Result{Type: v.TypeName(), Text: text}
	switch x := v.(type) {
	case value.Целое:
		res.Value = x.V
	case value.Дробное:
		res.Value = x.V
	case value.Строка:
		res.Value = x.V
	case value.Булево:
		res.Value = x.V
	case value.Пусто:
		res.Value = nil
	case value.Список:
		lst := make([]any, len(*x.Elems))
		for i, e := range *x.Elems {
			lst[i] = toResult(e).Value
		}
		res.Value = lst
	case value.Запись:
		m := make(map[string]any, len(x.Keys()))
		for _, k := range x.Keys() {
			m[k] = toResult(x.Get(k)).Value
		}
		res.Value = m
	default:
		// Дата/Длительность/Период (deferred-типы, §9.4): нет представления в
		// JSON — строка, равная канонической печати.
		res.Value = text
	}
	return res
}

// schemaDiag превращает ошибку приведения по схеме (ОшибкаВыполнения загрузчика,
// текст §SC-9.B) в ir.Diagnostic. Позиция — {1,1}: ошибка относится к ДАННЫМ
// записи, а не к тексту канонической строки выражения, поэтому позиция обёртки
// (decl.Pos()) в координаты атрибута не пересчитывается.
func schemaDiag(err error) (ir.Diagnostic, bool) {
	msg, _, ok := locatedMsgPos(err)
	if !ok {
		return ir.Diagnostic{}, false
	}
	return ir.Diagnostic{
		Severity: ir.SeverityError,
		Stage:    stageRuntime,
		Message:  msg,
		Pos:      ir.Position{Line: 1, Col: 1},
	}, true
}
