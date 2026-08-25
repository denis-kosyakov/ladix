package metrics

// Конвертация записей потребителя ([]map[string]any → []value.Запись, Д-2/Д-8) и
// сортировка ключей записи (Д-9). Приведение по объявленной схеме Options.Fields
// (Д-7) здесь НЕ дублируется: его выполняет eval.(*Interpreter).ApplySourceSchema —
// экспортированная обёртка того же applySchema/coerceField, которым пользуется
// загрузчик источника CLI (см. metrics.go). Второй семантики и вторых текстов
// ошибок в пакете нет.

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/value"
	"github.com/denis-kosyakov/ladix/ir"
)

// fieldTypeNames — допустимые аннотации поля: (зеркало internal/eval/analyze.go
// fieldTypeNames, §SC-5): §7 checkSourceDecl проверяет ТО ЖЕ множество.
var fieldTypeNames = map[string]bool{
	"Целое": true, "Дробное": true, "Строка": true, "Логическое": true, "Дата": true,
}

// validIdentName сообщает, является ли name ровно одним валидным identifier-
// токеном Ladix (не ключевым словом/литералом истина|ложь|пусто): токенизирует
// name публичным лексером и требует РОВНО [IDENT, NEWLINE, EOF] — тот же
// механизм, которым фасад защищается от инъекции в синтетический текст
// (design.md «Инъекция»).
func validIdentName(name string) bool {
	if name == "" {
		return false
	}
	toks, errs := lexer.New(name).Tokenize()
	if errs != nil && !errs.Empty() {
		return false
	}
	if len(toks) != 3 {
		return false
	}
	return toks[0].Type == lexer.IDENT && toks[0].Lexeme == name
}

// validatedFieldNames валидирует Options.Fields (Д-7 «Инъекция»): имя поля —
// корректный НЕ-ключевой идентификатор Ladix, имя типа — из известного
// множества. Возвращает отсортированные имена полей (детерминированный порядок
// в собранном тексте) либо ErrInvalidOptions.
func validatedFieldNames(fields map[string]string) ([]string, error) {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	// Сортировка ДО валидации: при нескольких некорректных записях текст
	// ErrInvalidOptions не должен зависеть от порядка обхода map.
	sort.Strings(names)
	for _, name := range names {
		typ := fields[name]
		if !validIdentName(name) {
			return nil, fmt.Errorf("%w: имя поля %q не является допустимым идентификатором Ladix", ErrInvalidOptions, name)
		}
		if !fieldTypeNames[typ] {
			return nil, fmt.Errorf("%w: неизвестное имя типа %q для поля %q", ErrInvalidOptions, typ, name)
		}
	}
	return names, nil
}

// convErrKind — программный маркер причины отказа конвертации (НЕ пользовательский
// текст): сообщения собираются в recordFieldMessage дословно по реестрам
// §SM-9.B/§SC-9.B — у двух причин РАЗНЫЕ разделители, и переформулировать их
// нельзя (Принцип VIII).
type convErrKind int

const (
	convOK convErrKind = iota
	convUnsupportedType
	convIntOutOfRange
)

// recordFieldMessage собирает текст диагностики поля записи. «неподдерживаемый тип
// значения» — новый текст spec.md (образец §SM-9.B, разделитель «запись N: поле»);
// «целое число вне диапазона» — СУЩЕСТВУЮЩИЙ текст реестра §SM-9.B, который
// загрузчик источника печатает с разделителем «запись N, поле» (source_loader.go
// sourceNumberToValue) — байт-в-байт тот же.
func recordFieldMessage(kind convErrKind, sourceName string, idx int, field string) string {
	if kind == convIntOutOfRange {
		return fmt.Sprintf("источник '%s': запись %d, поле '%s': целое число вне диапазона", sourceName, idx, field)
	}
	return fmt.Sprintf("источник '%s': запись %d: поле '%s': неподдерживаемый тип значения", sourceName, idx, field)
}

// convertRecords конвертирует записи потребителя в []value.Запись (Д-2/Д-8),
// сортируя ключи каждой записи лексикографически ПЕРЕД value.NewRecord (Д-9) —
// порядок самих записей сохраняется (Д-2/детерминизм).
func convertRecords(sourceName string, records []map[string]any) ([]value.Запись, *ir.Diagnostic) {
	out := make([]value.Запись, 0, len(records))
	for idx, rec := range records {
		keys := make([]string, 0, len(rec))
		for k := range rec {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fields := make(map[string]value.Value, len(rec))
		for _, k := range keys {
			v, kind := convertJSONValue(rec[k], 1)
			if kind != convOK {
				d := ir.Diagnostic{
					Severity: ir.SeverityError,
					Stage:    stageRuntime,
					Message:  recordFieldMessage(kind, sourceName, idx+1, k),
					Pos:      ir.Position{Line: 1, Col: 1},
				}
				return nil, &d
			}
			fields[k] = v
		}
		out = append(out, value.NewRecord(keys, fields))
	}
	return out, nil
}

// recordValueDepthLimit — предел вложенности ЗНАЧЕНИЯ записи потребителя
// (Список/Запись рекурсивно). Гард обязателен, а не косметичен: `[]map[string]any`
// поступает извне и МОЖЕТ содержать ЦИКЛ (`m["сам"] = m`) либо произвольно
// глубокое дерево из декодированного JSON. Без предела convertJSONValue уходит в
// бесконечную рекурсию и роняет процесс ПОТРЕБИТЕЛЯ через `fatal error: stack
// overflow` — а переполнение стека в Go НЕ ловится recover-барьером Evaluate,
// то есть spec.md Scenario «Паника не пересекает границу API» был бы нарушен
// фатально. Предел 100 симметричен потолку глубины выражения (complexity.go) и
// на порядки выше любой реальной бизнес-записи. Превышение → тот же дословный
// текст `неподдерживаемый тип значения` (spec.md «Форма данных потребителя»):
// циклическое/бездонное значение не имеет представления в JSON-семантике §9.3,
// новых пользовательских текстов не вводится (Принцип VIII, «Новое = 0»).
const recordValueDepthLimit = 100

// convertJSONValue — Д-8: типизация чисел по Go-типу (json.Number — строго по
// форме токена, §9.3 дословно); string/bool/nil/[]any/map[string]any — по
// JSON-семантике §9.3; прочие Go-типы → структурная диагностика (errText
// непусто), НЕ паника. depth — текущая глубина вложенности значения (корень = 1),
// см. recordValueDepthLimit.
func convertJSONValue(v any, depth int) (value.Value, convErrKind) {
	if depth > recordValueDepthLimit {
		return nil, convUnsupportedType
	}
	switch x := v.(type) {
	case nil:
		return value.None, convOK
	case bool:
		return value.Булево{V: x}, convOK
	case string:
		return value.Строка{V: x}, convOK
	case int:
		return value.Целое{V: int64(x)}, convOK
	case int8:
		return value.Целое{V: int64(x)}, convOK
	case int16:
		return value.Целое{V: int64(x)}, convOK
	case int32:
		return value.Целое{V: int64(x)}, convOK
	case int64:
		return value.Целое{V: x}, convOK
	case uint:
		if uint64(x) > math.MaxInt64 {
			return nil, convIntOutOfRange
		}
		return value.Целое{V: int64(x)}, convOK
	case uint8:
		return value.Целое{V: int64(x)}, convOK
	case uint16:
		return value.Целое{V: int64(x)}, convOK
	case uint32:
		return value.Целое{V: int64(x)}, convOK
	case uint64:
		if x > math.MaxInt64 {
			return nil, convIntOutOfRange
		}
		return value.Целое{V: int64(x)}, convOK
	case float32:
		return value.Дробное{V: float64(x)}, convOK
	case float64:
		return value.Дробное{V: x}, convOK
	case json.Number:
		s := string(x)
		if hasFloatForm(s) {
			f, err := x.Float64()
			if err != nil {
				return nil, convUnsupportedType
			}
			return value.Дробное{V: f}, convOK
		}
		n, err := x.Int64()
		if err != nil {
			return nil, convIntOutOfRange
		}
		return value.Целое{V: n}, convOK
	case []any:
		elems := make([]value.Value, 0, len(x))
		for _, e := range x {
			ev, kind := convertJSONValue(e, depth+1)
			if kind != convOK {
				return nil, kind
			}
			elems = append(elems, ev)
		}
		return value.NewList(elems), convOK
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fields := make(map[string]value.Value, len(x))
		for _, k := range keys {
			fv, kind := convertJSONValue(x[k], depth+1)
			if kind != convOK {
				return nil, kind
			}
			fields[k] = fv
		}
		return value.NewRecord(keys, fields), convOK
	default:
		return nil, convUnsupportedType
	}
}

// hasFloatForm — §9.3: '.'/'e'/'E' в токене → Дробное.
func hasFloatForm(s string) bool {
	for _, r := range s {
		if r == '.' || r == 'e' || r == 'E' {
			return true
		}
	}
	return false
}
