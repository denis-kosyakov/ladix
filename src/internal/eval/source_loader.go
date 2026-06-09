package eval

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// loadSource лениво читает JSON-файл источника decl и порождает срез value.Запись
// (§SM-8.1). Результат кешируется в i.recordCache[decl.Name.Name] на запуск:
// повторный вызов любой метрики не перечитывает файл (§9.6). Все ошибки — жёсткие
// ОшибкаВыполнения с позицией объявления источника (decl.Pos()) и текстами §SM-9.B.
//
// Маппинг JSON→value (§9.3): null→Пусто, bool→Булево, строка→Строка (даты НЕ
// распознаются), json.Number без '.'/'e'/'E' → Целое (вне int64 → ошибка),
// json.Number с '.'/экспонентой → Дробное, массив→Список (рекурсивно),
// объект→Запись (рекурсивно, порядок ключей — по тексту).
func (i *Interpreter) loadSource(decl *ast.SourceDecl) ([]value.Запись, error) {
	name := decl.Name.Name
	if recs, ok := i.recordCache[name]; ok {
		return recs, nil
	}
	path := decl.File.Value

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, runtimeErr(decl.Pos(),
				fmt.Sprintf("источник '%s': файл «%s» не найден", name, path))
		}
		return nil, runtimeErr(decl.Pos(),
			fmt.Sprintf("источник '%s': не удалось прочитать файл «%s»", name, path))
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.UseNumber()

	// Верхний уровень — JSON-массив объектов (§9.2): первый токен — «[».
	tok, err := dec.Token()
	if err != nil {
		return nil, i.jsonErr(decl, err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return nil, runtimeErr(decl.Pos(),
			fmt.Sprintf("источник '%s': ожидался JSON-массив объектов в «%s»", name, path))
	}

	recs := []value.Запись{}
	idx := 0
	for dec.More() {
		idx++
		// Следующий токен решает: «{» → объект-запись; иначе → не объект.
		v, err := i.decodeValue(decl, idx, "", dec)
		if err != nil {
			return nil, err
		}
		rec, ok := v.(value.Запись)
		if !ok {
			return nil, runtimeErr(decl.Pos(),
				fmt.Sprintf("источник '%s': запись %d не является объектом", name, idx))
		}
		recs = append(recs, rec)
	}
	// Закрывающую «]» читаем для полноты, но висящий мусор после массива в v1 не валидируем строго.
	if _, err := dec.Token(); err != nil && err != io.EOF {
		return nil, i.jsonErr(decl, err)
	}

	i.recordCache[name] = recs
	return recs, nil
}

// decodeValue рекурсивно читает одно JSON-значение из потокового декодера dec и
// конвертирует его в value.Value (§9.3), сохраняя текстовый порядок ключей
// объектов. idx — номер записи (с 1), field — имя верхнеуровневого поля для текста
// ошибки «целое вне диапазона».
func (i *Interpreter) decodeValue(decl *ast.SourceDecl, idx int, field string, dec *json.Decoder) (value.Value, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, i.jsonErr(decl, err)
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			return i.decodeObject(decl, idx, dec)
		case '[':
			return i.decodeArray(decl, idx, field, dec)
		default:
			return nil, i.jsonErr(decl, fmt.Errorf("неожиданный токен '%c'", rune(t)))
		}
	case nil:
		return value.None, nil // null
	case bool:
		return value.Булево{V: t}, nil
	case string:
		return value.Строка{V: t}, nil // даты НЕ распознаются (§9.4)
	case json.Number:
		return i.numberToValue(decl, idx, field, t)
	default:
		return nil, i.jsonErr(decl, fmt.Errorf("неподдерживаемое значение"))
	}
}

// decodeObject читает тело объекта (открывающая «{» уже прочитана) → value.Запись,
// сохраняя порядок ключей. Дубликат ключа — побеждает последний (§9.2).
func (i *Interpreter) decodeObject(decl *ast.SourceDecl, idx int, dec *json.Decoder) (value.Value, error) {
	keys := []string{}
	fields := map[string]value.Value{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, i.jsonErr(decl, err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, i.jsonErr(decl, fmt.Errorf("ожидался ключ объекта"))
		}
		v, err := i.decodeValue(decl, idx, key, dec)
		if err != nil {
			return nil, err
		}
		if _, exists := fields[key]; !exists {
			keys = append(keys, key)
		}
		fields[key] = v
	}
	if _, err := dec.Token(); err != nil { // закрывающая «}»
		return nil, i.jsonErr(decl, err)
	}
	return value.NewRecord(keys, fields), nil
}

// decodeArray читает тело массива (открывающая «[» уже прочитана) → value.Список.
func (i *Interpreter) decodeArray(decl *ast.SourceDecl, idx int, field string, dec *json.Decoder) (value.Value, error) {
	elems := []value.Value{}
	for dec.More() {
		ev, err := i.decodeValue(decl, idx, field, dec)
		if err != nil {
			return nil, err
		}
		elems = append(elems, ev)
	}
	if _, err := dec.Token(); err != nil { // закрывающая «]»
		return nil, i.jsonErr(decl, err)
	}
	return value.NewList(elems), nil
}

// numberToValue различает Целое/Дробное по форме токена JSON (§9.3): наличие
// '.'/'e'/'E' → Дробное; иначе Целое (вне int64 → §SM-9.B).
func (i *Interpreter) numberToValue(decl *ast.SourceDecl, idx int, field string, n json.Number) (value.Value, error) {
	s := string(n)
	if strings.ContainsAny(s, ".eE") {
		f, err := n.Float64()
		if err != nil {
			return nil, i.jsonErr(decl, err)
		}
		return value.Дробное{V: f}, nil
	}
	v, err := n.Int64()
	if err != nil {
		// Целое вне диапазона int64 (§9.6 / §SM-9.B).
		return nil, runtimeErr(decl.Pos(),
			fmt.Sprintf("источник '%s': запись %d, поле '%s': целое число вне диапазона", decl.Name.Name, idx, field))
	}
	return value.Целое{V: v}, nil
}

// jsonErr оборачивает низкоуровневую JSON-ошибку в §SM-9.B «некорректный JSON».
func (i *Interpreter) jsonErr(decl *ast.SourceDecl, err error) error {
	return runtimeErr(decl.Pos(),
		fmt.Sprintf("источник '%s': некорректный JSON в «%s» (%s)", decl.Name.Name, decl.File.Value, jsonDetail(err)))
}

// jsonDetail извлекает компактную деталь JSON-ошибки для §SM-9.B.
func jsonDetail(err error) string {
	if err == io.EOF || stderrors.Is(err, io.ErrUnexpectedEOF) {
		return "неожиданный конец файла"
	}
	return err.Error()
}
