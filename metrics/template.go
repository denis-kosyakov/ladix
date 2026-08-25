package metrics

// Синтетическая декларация метрики (design.md Д-10): все четыре канонические
// строки (где:/агрегат:/период:/по_дате:) разбираются одной программой-текстом
// через публичные lexer.New(...).Tokenize() / parser.New(...).Parse(), потому что
// parseExpression НЕ экспортирован, а формы период: разбираются контекстным
// parsePeriodValue, достижимым только из parseMetricDecl (spike, см. design.md).
// Имена метрики/источника — ФИКСИРОВАННЫЕ безопасные идентификаторы (НЕ ключевые
// слова Ladix) — вход потребителя попадает в текст только как тело выражения
// атрибута. Позиции пересчитываются из координат синтетической обёртки в
// координаты канонической строки атрибута (design.md Д-10, «Пересчёт позиций»).

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	lerrors "github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/ir"
)

// synSourceName/synMetricName — фиксированные безопасные идентификаторы
// синтетической программы. НЕ ключевые слова (ловушка: "и" = KW_AND).
const (
	synSourceName = "_синт_источник_"
	synMetricName = "_синт_метрика_"
)

// position — общая пара (Line,Col) для позиций из lexer/parser/errors (все три
// структурно совпадают: Line/Col, 1-based, колонка в рунах).
type position struct{ Line, Col int }

// attrSlot — координаты одного атрибута (где/агрегат/период/по_дате) внутри
// синтетического текста: строка и колонка (в рунах), с которой начинается сам
// текст выражения (без ведущего "где:      " и т.п.).
type attrSlot struct {
	name    string // "где", "агрегат", "период", "по_дате" — для текста диагностики
	present bool
	line    int
	col     int
}

// template — синтетическая программа целиком: текст + координаты 4 атрибутов
// (в порядке где,агрегат,период,по_дате — design.md «Проверяй атрибуты в этом
// порядке»).
type template struct {
	text  string
	attrs [4]attrSlot // 0=где 1=агрегат 2=период 3=по_дате
}

const (
	idxWhere = iota
	idxAggregate
	idxPeriod
	idxByDate
)

// templateBuilder накапливает текст программы, отслеживая текущую (строка,
// колонка-в-рунах) — НИКАКОЕ смещение шаблона не хардкодится числом (design.md
// Д-10): координаты атрибутов получаются из фактически записанного текста.
type templateBuilder struct {
	sb   strings.Builder
	line int
	col  int
}

func newTemplateBuilder() *templateBuilder {
	return &templateBuilder{line: 1, col: 1}
}

// writeLine дописывает целую строку (без атрибута-выражения) + перевод строки.
func (b *templateBuilder) writeLine(s string) {
	b.sb.WriteString(s)
	b.sb.WriteByte('\n')
	b.line++
	b.col = 1
}

// writeAttrLine дописывает "prefix" + exprText + "\n", возвращая координаты
// НАЧАЛА exprText (после prefix) — именно они нужны для пересчёта позиций.
func (b *templateBuilder) writeAttrLine(prefix, exprText string) (line, col int) {
	b.sb.WriteString(prefix)
	b.col += utf8.RuneCountInString(prefix)
	line, col = b.line, b.col
	b.sb.WriteString(exprText)
	b.sb.WriteByte('\n')
	b.line++
	b.col = 1
	return line, col
}

// buildTemplate строит синтетическую программу для метрики m с опциональной
// схемой полей opts.Fields (Д-7). Пустые атрибуты ("") в текст НЕ включаются
// (design.md Д-10) — соответствующий attrSlot остаётся present=false.
func buildTemplate(m ir.Metric, opts Options) (*template, error) {
	fieldNames, ferr := validatedFieldNames(opts.Fields)
	if ferr != nil {
		return nil, ferr
	}

	b := newTemplateBuilder()
	b.writeLine(fmt.Sprintf("источник %s:", synSourceName))
	b.writeLine(`    файл: "_синт_.json"`)
	if len(fieldNames) > 0 {
		b.writeLine("    поля:")
		for _, name := range fieldNames {
			b.writeLine(fmt.Sprintf("        %s: %s", name, opts.Fields[name]))
		}
	}
	b.writeLine(fmt.Sprintf("метрика %s:", synMetricName))
	b.writeLine(fmt.Sprintf("    источник: %s", synSourceName))

	t := &template{}
	if m.Where != "" {
		l, c := b.writeAttrLine("    где:      ", m.Where)
		t.attrs[idxWhere] = attrSlot{name: "где", present: true, line: l, col: c}
	}
	if m.Aggregate != "" {
		l, c := b.writeAttrLine("    агрегат:  ", m.Aggregate)
		t.attrs[idxAggregate] = attrSlot{name: "агрегат", present: true, line: l, col: c}
	}
	if m.Period != "" {
		l, c := b.writeAttrLine("    период:   ", m.Period)
		t.attrs[idxPeriod] = attrSlot{name: "период", present: true, line: l, col: c}
	}
	if m.ByDate != "" {
		l, c := b.writeAttrLine("    по_дате:  ", m.ByDate)
		t.attrs[idxByDate] = attrSlot{name: "по_дате", present: true, line: l, col: c}
	}
	t.text = b.sb.String()
	return t, nil
}

// attrNames — имена атрибутов в каноническом порядке (индексы idxWhere…idxByDate).
var attrNames = [4]string{"где", "агрегат", "период", "по_дате"}

// attrTexts — канонические строки m в том же порядке.
func attrTexts(m ir.Metric) [4]string { return [4]string{m.Where, m.Aggregate, m.Period, m.ByDate} }

// multilineAttrDiag отклоняет канонические строки, содержащие перевод строки
// (или возврат каретки), ДО сборки синтетического текста.
//
// Это не косметика, а замок на СТРУКТУРНУЮ ИНЪЕКЦИЮ: атрибут вписывается в текст
// синтетической декларации отдельной строкой, поэтому «истина\n    период:
// ежемесячно» в Where добавил бы метрике атрибут, которого в ir.Metric нет, и
// Evaluate молча посчитал бы ДРУГУЮ метрику. Канонические строки (ast.canonExpr)
// однострочны по построению — строковые литералы печатаются strconv.Quote, где
// перевод строки экранируется, — так что многострочный вход НЕ канонический и по
// spec.md («Валидация канонических строк») обязан давать диагностику. Побочно это
// делает верным допущение пересчёта позиций (одна строка на атрибут).
func multilineAttrDiag(m ir.Metric) (ir.Diagnostic, bool) {
	texts := attrTexts(m)
	for i, text := range texts {
		if text == "" {
			continue
		}
		col := 1
		for _, r := range text {
			if r == '\n' || r == '\r' {
				return ir.Diagnostic{
					Severity: ir.SeverityError,
					Stage:    stageRuntime,
					Message:  fmt.Sprintf("метрика '%s': %s: недопустимое каноническое выражение", m.Name, attrNames[i]),
					Pos:      ir.Position{Line: 1, Col: col}, // колонка в РУНАХ
				}, true
			}
			col++
		}
	}
	return ir.Diagnostic{}, false
}

// canonPos пересчитывает позицию pos (в координатах синтетической обёртки) в
// координаты канонической строки атрибута slot (design.md Д-10): строка 1,
// колонка — от начала текста выражения. Защита от выхода за границы (Col < 1).
func canonPos(slot attrSlot, pos position) ir.Position {
	if !slot.present {
		return ir.Position{Line: 1, Col: 1}
	}
	canonLine := pos.Line - slot.line + 1
	canonCol := pos.Col
	if pos.Line == slot.line {
		canonCol = pos.Col - slot.col + 1
	}
	if canonLine < 1 {
		canonLine = 1
	}
	if canonCol < 1 {
		canonCol = 1
	}
	return ir.Position{Line: canonLine, Col: canonCol}
}

// locate находит атрибут, чья строка в синтетическом тексте совпадает с
// pos.Line, и возвращает его канонизированную позицию. Если точного совпадения
// нет (защитный путь — не должен достигаться на валидных входах), выбирает
// ближайший ПРИСУТСТВУЮЩИЙ атрибут по возрастанию расстояния строки, а если
// присутствующих нет вовсе — нулевую позицию {1,1}.
func (t *template) locate(pos position) (attrSlot, ir.Position) {
	for _, slot := range t.attrs {
		if slot.present && slot.line == pos.Line {
			return slot, canonPos(slot, pos)
		}
	}
	best := attrSlot{}
	bestDist := -1
	for _, slot := range t.attrs {
		if !slot.present {
			continue
		}
		d := pos.Line - slot.line
		if d < 0 {
			d = -d
		}
		if bestDist == -1 || d < bestDist {
			best, bestDist = slot, d
		}
	}
	if bestDist == -1 {
		return attrSlot{}, ir.Position{Line: 1, Col: 1}
	}
	return best, canonPos(best, pos)
}

// invalidExprDiag строит диагностику spec.md Requirement «Валидация канонических
// строк»: `метрика '<имя>': <атрибут> недопустимое каноническое выражение`.
func (t *template) invalidExprDiag(metricName string, pos position) ir.Diagnostic {
	slot, cpos := t.locate(pos)
	attr := slot.name
	if attr == "" {
		// Ни один атрибут не присутствует в тексте (например пустой ir.Metric{}
		// с пустым Aggregate) — по умолчанию относим к обязательному «агрегат:».
		attr = "агрегат"
	}
	return ir.Diagnostic{
		Severity: ir.SeverityError,
		Stage:    stageRuntime,
		Message:  fmt.Sprintf("метрика '%s': %s: недопустимое каноническое выражение", metricName, attr),
		Pos:      cpos,
	}
}

// missingAggregateDiag строит диагностику §3 для случая, когда «агрегат:»
// структурно отсутствует (ir.Metric.Aggregate == "") — обязательный атрибут,
// поэтому это трактуется как невалидное каноническое выражение атрибута
// «агрегат:» на нулевой позиции (текста атрибута нет вовсе).
func (t *template) missingAggregateDiag(metricName string) ir.Diagnostic {
	return ir.Diagnostic{
		Severity: ir.SeverityError,
		Stage:    stageRuntime,
		Message:  fmt.Sprintf("метрика '%s': агрегат: недопустимое каноническое выражение", metricName),
		Pos:      ir.Position{Line: 1, Col: 1},
	}
}

// runtimeDiag извлекает Msg/Pos из ошибки internal/eval (СемантическаяОшибка/
// ОшибкаТипа/ОшибкаВыполнения — все три реализуют lerrors.Расположенная) и
// строит диагностику БЕЗ двухстрочной обёртки «Ошибка в строке…» (design.md
// Д-5: та же идиома, что применяет корневой ladix.go к Compile-диагностикам —
// текст берётся из поля Msg, а не из Error()). Позиция пересчитывается в
// координаты канонической строки соответствующего атрибута.
func (t *template) runtimeDiag(err error) (ir.Diagnostic, bool) {
	msg, pos, ok := locatedMsgPos(err)
	if !ok {
		return ir.Diagnostic{}, false
	}
	_, cpos := t.locate(pos)
	return ir.Diagnostic{
		Severity: ir.SeverityError,
		Stage:    stageRuntime,
		Message:  msg,
		Pos:      cpos,
	}, true
}

// locatedMsgPos достаёт (Msg, Pos) из известных типов ошибок internal/eval по
// конкретному типу (errors.As), избегая двухстрочного канона Error().
func locatedMsgPos(err error) (string, position, bool) {
	var sem lerrors.СемантическаяОшибка
	if errors.As(err, &sem) {
		return sem.Msg, position{Line: sem.Pos.Line, Col: sem.Pos.Col}, true
	}
	var te lerrors.ОшибкаТипа
	if errors.As(err, &te) {
		return te.Msg, position{Line: te.Pos.Line, Col: te.Pos.Col}, true
	}
	var re lerrors.ОшибкаВыполнения
	if errors.As(err, &re) {
		return re.Msg, position{Line: re.Pos.Line, Col: re.Pos.Col}, true
	}
	return "", position{}, false
}
