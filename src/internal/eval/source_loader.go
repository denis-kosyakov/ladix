package eval

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// resolveSourcePath разрешает путь к файлу-источнику относительно базового каталога
// источников (§SM-8.1, фича 026, D-1/D-2). Абсолютный путь — как есть (база
// игнорируется); относительный — filepath.Join(i.sourceBase, p). При пустой базе
// ("") сводится к p (≡ резолв от cwd процесса). Чистая функция без I/O. Результат
// идёт и в os.Open, и в текст ошибки «файл не найден» (диагностируется итоговый путь).
func (i *Interpreter) resolveSourcePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(i.sourceBase, p)
}

// loadSource лениво читает файл источника decl, диспетчеризует по decl.Type.Name
// (§SC-6, A1-2: пусто/json → JSON, csv → CSV, ndjson → NDJSON) и порождает срез
// value.Запись. При наличии схемы (len(decl.Fields) > 0) — applySchema коэрсит/
// валидирует записи (§SC-D-COERCE). Результат кешируется в i.recordCache[name] на
// запуск (§SC-D-CACHE, §9.6): повторный вызов любой метрики не перечитывает файл.
// Все ошибки — жёсткие ОшибкаВыполнения с позицией decl.Pos() и текстами §SM-9.B/
// §SC-9.B.
func (i *Interpreter) loadSource(decl *ast.SourceDecl) ([]value.Запись, error) {
	name := decl.Name.Name
	if recs, ok := i.recordCache[name]; ok {
		return recs, nil
	}

	var recs []value.Запись
	var err error
	switch decl.Type.Name { // "" ≡ json (A1-2, §SC-6)
	case "", "json":
		recs, err = i.loadJSON(decl)
	case "csv":
		recs, err = i.loadCSV(decl)
	case "ndjson":
		recs, err = i.loadNDJSON(decl)
	default:
		// Недостижимо: множество тип: валидируется семантикой (§SC-4-sem).
		recs, err = i.loadJSON(decl)
	}
	if err != nil {
		return nil, err
	}

	if len(decl.Fields) > 0 {
		recs, err = i.applySchema(decl, recs)
		if err != nil {
			return nil, err
		}
	}

	i.recordCache[name] = recs
	return recs, nil
}

// loadJSON читает JSON-файл источника decl (исходный путь v1, §SM-8.1, §9.3),
// БЕЗ изменения поведения. Маппинг JSON→value: null→Пусто, bool→Булево,
// строка→Строка (даты НЕ распознаются здесь), json.Number без '.'/'e'/'E' → Целое
// (вне int64 → ошибка), json.Number с '.'/экспонентой → Дробное, массив→Список
// (рекурсивно), объект→Запись (рекурсивно, порядок ключей — по тексту).
func (i *Interpreter) loadJSON(decl *ast.SourceDecl) ([]value.Запись, error) {
	name := decl.Name.Name
	path := i.resolveSourcePath(decl.File.Value)

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

	// Кеш пишет диспетчер loadSource ПОСЛЕ applySchema (§SC-D-CACHE).
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
		return i.sourceNumberToValue(decl, idx, field, t)
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

// sourceNumberToValue различает Целое/Дробное по форме токена JSON (§9.3): наличие
// '.'/'e'/'E' → Дробное; иначе Целое (вне int64 → §SM-9.B). Строгий контракт.
// Толерантный двойник: jsonval.payloadNumberToValue (вне диапазона → ±Inf, никогда не None).
func (i *Interpreter) sourceNumberToValue(decl *ast.SourceDecl, idx int, field string, n json.Number) (value.Value, error) {
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

// loadCSV читает CSV-источник (010-A1, §SC-D-CSV) через encoding/csv (stdlib, 0
// новых зависимостей): 1-я строка — заголовок (имена столбцов), разделитель `,`,
// UTF-8. Каждая ячейка — value.Строка; коэрсия в объявленный тип — в applySchema
// (поля: обязательно для csv, §SC-4-sem). Заголовок ОБЯЗАН содержать все
// объявленные поля (иначе load-ошибка §SC-9.B); лишние столбцы сохраняются как
// Строка (A1-6). Записи нумеруются с 1 (строки данных после заголовка). Ошибка
// разбора CSV → §SC-9.B «некорректный CSV». Путь резолвится resolveSourcePath от
// базового каталога источников (каталог .ladix-файла или --source-base;
// абсолютный — как есть), §SM-8.1.
func (i *Interpreter) loadCSV(decl *ast.SourceDecl) ([]value.Запись, error) {
	name := decl.Name.Name
	path := i.resolveSourcePath(decl.File.Value)

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

	r := csv.NewReader(f)
	r.FieldsPerRecord = 0 // требовать одинаковое число полей во всех строках
	rows, err := r.ReadAll()
	if err != nil {
		return nil, runtimeErr(decl.Pos(),
			fmt.Sprintf("источник '%s': некорректный CSV в «%s» (%s)", name, path, err.Error()))
	}
	if len(rows) == 0 {
		return []value.Запись{}, nil // нет даже заголовка → нуль записей (§SC-D-EMPTY)
	}

	header := rows[0]
	// Снимаем ведущий UTF-8 BOM с первого столбца заголовка (экспорт Excel/Windows
	// добавляет EF BB BF к первой ячейке) — иначе имя первого столбца приклеивается к
	// BOM и сопоставление с полями ложно проваливается/уходит в мохибейк-ключ.
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\ufeff")
	}
	// Дубликат имени столбца в заголовке (любого, не только объявленного) → потеря
	// данных при тихом схлопывании (последний побеждает). Жёсткая load-ошибка §SC-9.B.
	seenCol := map[string]bool{}
	for _, h := range header {
		if seenCol[h] {
			return nil, runtimeErr(decl.Pos(),
				fmt.Sprintf("источник '%s': в заголовке CSV «%s» столбец '%s' объявлен дважды", name, path, h))
		}
		seenCol[h] = true
	}
	// Заголовок ОБЯЗАН содержать все объявленные поля (§SC-D-CSV).
	headerSet := map[string]bool{}
	for _, h := range header {
		headerSet[h] = true
	}
	for _, fd := range decl.Fields {
		if !headerSet[fd.Name.Name] {
			return nil, runtimeErr(decl.Pos(),
				fmt.Sprintf("источник '%s': в заголовке CSV «%s» отсутствует поле '%s'", name, path, fd.Name.Name))
		}
	}

	recs := make([]value.Запись, 0, len(rows)-1)
	for _, row := range rows[1:] {
		keys := make([]string, 0, len(header))
		fields := make(map[string]value.Value, len(header))
		for col, h := range header {
			if _, exists := fields[h]; !exists {
				keys = append(keys, h)
			}
			cell := ""
			if col < len(row) {
				cell = row[col]
			}
			fields[h] = value.Строка{V: cell} // каждая ячейка — Строка (коэрсия в applySchema)
		}
		recs = append(recs, value.NewRecord(keys, fields))
	}
	return recs, nil
}

// loadNDJSON читает NDJSON-источник (010-A1, §SC-D-NDJSON): построчно, пустые
// строки пропускаются (A1-9), каждая непустая строка — независимый JSON-объект,
// декодируемый тем же путём, что и JSON-источник (decodeObject → одинаковый
// маппинг типов/порядок ключей). Не-объект на строке → load-ошибка «запись N не
// является объектом»; битый JSON → «запись N: некорректный JSON». Нумерация
// записей N — с 1, сквозная по НЕпустым строкам. Путь резолвится
// resolveSourcePath от базового каталога источников (каталог .ladix-файла или
// --source-base; абсолютный — как есть), §SM-8.1.
func (i *Interpreter) loadNDJSON(decl *ast.SourceDecl) ([]value.Запись, error) {
	name := decl.Name.Name
	path := i.resolveSourcePath(decl.File.Value)

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

	recs := []value.Запись{}
	idx := 0
	firstLine := true
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // допускаем длинные строки
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue // пустые строки пропускаются (A1-9)
		}
		if firstLine {
			// Снимаем ведущий UTF-8 BOM с первой непустой строки (экспорт
			// Excel/Windows) — иначе json.Valid/декод первого объекта ложно падает.
			line = strings.TrimPrefix(line, "\ufeff")
			firstLine = false
		}
		idx++
		// Сначала проверяем синтаксическую валидность строки целиком, чтобы
		// per-line ошибка несла §SC-9.B «запись N: некорректный JSON» (а не
		// файловую §SM-9.B decodeObject) — единый канон NDJSON.
		if !json.Valid([]byte(line)) {
			return nil, i.ndjsonErr(decl, idx, fmt.Errorf("некорректный синтаксис"))
		}
		dec := json.NewDecoder(bytes.NewReader([]byte(line)))
		dec.UseNumber()
		tok, err := dec.Token()
		if err != nil {
			return nil, i.ndjsonErr(decl, idx, err)
		}
		d, ok := tok.(json.Delim)
		if !ok || d != '{' {
			return nil, runtimeErr(decl.Pos(),
				fmt.Sprintf("источник '%s': запись %d не является объектом", name, idx))
		}
		v, err := i.decodeObject(decl, idx, dec)
		if err != nil {
			return nil, err
		}
		recs = append(recs, v.(value.Запись))
	}
	if err := sc.Err(); err != nil {
		return nil, runtimeErr(decl.Pos(),
			fmt.Sprintf("источник '%s': не удалось прочитать файл «%s»", name, path))
	}
	return recs, nil
}

// ndjsonErr оборачивает низкоуровневую JSON-ошибку строки NDJSON в §SC-9.B
// «запись N: некорректный JSON».
func (i *Interpreter) ndjsonErr(decl *ast.SourceDecl, idx int, err error) error {
	return runtimeErr(decl.Pos(),
		fmt.Sprintf("источник '%s': запись %d: некорректный JSON (%s)", decl.Name.Name, idx, jsonDetail(err)))
}

// applySchema коэрсит/валидирует записи recs по объявленной схеме decl.Fields
// (§SC-D-COERCE/RECORD). НЕ мутирует value.Запись (приватные поля, Set-метода нет,
// конституция VII), а ПЕРЕСОБИРАЕТ через NewRecord/Keys/Get. Для каждой записи
// (N с 1): копирует все ключи (лишние поля как есть, A1-6); для каждого
// FieldDef{name,T} — присутствие = членство name в Keys() (НЕ Get!=None: отличает
// «отсутствует» A1-5 от «есть, но Пусто/null» A1-10); коэрсит значение по матрице
// §SC-D-COERCE (CSV-источник — всё Строка; JSON/NDJSON — типизировано). Единственный
// кросс-тип промоушен — Целое→Дробное. Дата строится тем же parseISODate
// (builtins_date.go), НЕ дублируется. Тексты §SC-9.B byte-identical; позиция decl.
func (i *Interpreter) applySchema(decl *ast.SourceDecl, recs []value.Запись) ([]value.Запись, error) {
	name := decl.Name.Name
	isCSV := decl.Type.Name == "csv"
	out := make([]value.Запись, 0, len(recs))
	for k, r := range recs {
		idx := k + 1
		keys := r.Keys()
		present := make(map[string]bool, len(keys))
		newFields := make(map[string]value.Value, len(keys))
		for _, key := range keys {
			present[key] = true
			newFields[key] = r.Get(key) // лишние поля сохраняются как есть (A1-6)
		}
		for _, fd := range decl.Fields {
			fname := fd.Name.Name
			if !present[fname] {
				return nil, runtimeErr(decl.Pos(),
					fmt.Sprintf("источник '%s': запись %d: отсутствует объявленное поле '%s'", name, idx, fname))
			}
			coerced, err := i.coerceField(decl, idx, fname, fd.TypeName.Name, r.Get(fname), isCSV)
			if err != nil {
				return nil, err
			}
			newFields[fname] = coerced
		}
		out = append(out, value.NewRecord(keys, newFields))
	}
	return out, nil
}

// coerceField коэрсит одно значение v в объявленный тип T (§SC-D-COERCE). isCSV
// различает источник: CSV — все значения value.Строка (strconv-парс); JSON/NDJSON —
// типизированные значения (проверка типа + единственный промоушен Целое→Дробное).
// Тексты §SC-9.B byte-identical, позиция = decl.Pos(); <Тип> в «ожидался» — это
// аннотация T (Логическое, не Булево); <тип> в «получено» — v.TypeName().
func (i *Interpreter) coerceField(decl *ast.SourceDecl, idx int, fname, T string, v value.Value, isCSV bool) (value.Value, error) {
	mismatch := func() error {
		return runtimeErr(decl.Pos(),
			fmt.Sprintf("источник '%s': запись %d, поле '%s': ожидался %s, получено %s", decl.Name.Name, idx, fname, T, v.TypeName()))
	}
	badParse := func(s, kind string) error {
		return runtimeErr(decl.Pos(),
			fmt.Sprintf("источник '%s': запись %d, поле '%s': «%s» %s", decl.Name.Name, idx, fname, s, kind))
	}

	switch T {
	case "Строка":
		if isCSV {
			return v, nil // CSV-ячейка уже Строка
		}
		if _, ok := v.(value.Строка); !ok {
			return nil, mismatch()
		}
		return v, nil

	case "Целое":
		if isCSV {
			s := v.(value.Строка).V
			// §SC-D-COERCE-INT: без '.'/'e'/'E' — иначе «не является целым».
			if strings.ContainsAny(s, ".eE") {
				return nil, badParse(s, "не является целым")
			}
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return nil, badParse(s, "не является целым")
			}
			return value.Целое{V: n}, nil
		}
		if _, ok := v.(value.Целое); ok {
			return v, nil
		}
		return nil, mismatch() // Дробное→Целое: демоушена нет (§SC-D-COERCE)

	case "Дробное":
		if isCSV {
			s := v.(value.Строка).V
			// §SC-D-COERCE-FLOAT: только конечные десятичные формы. Отвергаем
			// hex-float (маркеры x/X/p/P, напр. «0x1p4») — это Go-синтаксис, не
			// данные; и нечисловые ±Inf/NaN (ParseFloat принимает их без ошибки) —
			// они отравляют агрегаты. Десятичная экспонента «1e3» остаётся валидной.
			if strings.ContainsAny(s, "xXpP") {
				return nil, badParse(s, "не является дробным")
			}
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return nil, badParse(s, "не является дробным")
			}
			if math.IsInf(f, 0) || math.IsNaN(f) {
				return nil, badParse(s, "не является дробным")
			}
			return value.Дробное{V: f}, nil
		}
		if d, ok := v.(value.Дробное); ok {
			return d, nil
		}
		if n, ok := v.(value.Целое); ok {
			return value.Дробное{V: float64(n.V)}, nil // §SC-D-COERCE-PROMO
		}
		return nil, mismatch()

	case "Логическое":
		if isCSV {
			s := v.(value.Строка).V
			switch s {
			case "истина":
				return value.Булево{V: true}, nil
			case "ложь":
				return value.Булево{V: false}, nil
			default:
				return nil, badParse(s, "не является логическим (ожидалось истина/ложь)")
			}
		}
		if _, ok := v.(value.Булево); !ok {
			return nil, mismatch()
		}
		return v, nil

	case "Дата":
		if isCSV {
			s := v.(value.Строка).V
			d, ok := parseISODate(s)
			if !ok {
				return nil, badParse(s, "не является датой (ожидался формат ГГГГ-ММ-ДД)")
			}
			return d, nil
		}
		s, ok := v.(value.Строка)
		if !ok {
			return nil, mismatch() // не-Строка в Дата-поле → A1-10
		}
		d, ok := parseISODate(s.V)
		if !ok {
			return nil, badParse(s.V, "не является датой (ожидался формат ГГГГ-ММ-ДД)")
		}
		return d, nil
	}
	// Недостижимо: множество типов полей валидируется семантикой (§SC-4-sem п.3).
	return v, nil
}
