package eval

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// srcDeclPos — фиксированная позиция объявления источника для проверки, что
// ошибки загрузки указывают на decl.Pos() (§SM-9.B).
var srcDeclPos = ast.Position{Line: 7, Col: 3}

// makeSourceDecl строит ast.SourceDecl с именем name и путём path, ведущий токен
// «источник» — в srcDeclPos.
func makeSourceDecl(name, path string) *ast.SourceDecl {
	filePos := ast.Position{Line: 8, Col: 11}
	return ast.NewSourceDecl(srcDeclPos,
		*ast.NewIdent(ast.Position{Line: 7, Col: 12}, name),
		*ast.NewStringLit(filePos, path), filePos)
}

// newTestInterp — интерпретатор с пустым выводом и FixedClock (дата нерелевантна загрузке).
func newTestInterp() *Interpreter { return NewInterpreter(&bytes.Buffer{}, 0, testClock) }

// writeJSON пишет временную .json-фикстуру в dir и возвращает путь.
func writeJSON(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("запись фикстуры %s: %v", p, err)
	}
	return p
}

// T019: загрузка data/sales.json → 3 записи; поля распознаны по §9.3.
func TestLoadSourceSalesJSON(t *testing.T) {
	// Путь относительно cwd процесса. Тест бежит из каталога пакета internal/eval;
	// data/sales.json лежит в корне репозитория — поднимаемся к нему.
	root := filepath.Join("..", "..", "..")
	path := filepath.Join(root, "data", "sales.json")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("data/sales.json недоступен из cwd теста (%v)", err)
	}
	i := newTestInterp()
	recs, err := i.loadSource(makeSourceDecl("продажи", path))
	if err != nil {
		t.Fatalf("loadSource вернул ошибку: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("записей = %d, хотим 3", len(recs))
	}
	// Первая запись: сумма_заказа 1200000 → Целое; статус → Строка.
	r0 := recs[0]
	if got, ok := r0.Get("сумма_заказа").(value.Целое); !ok || got.V != 1200000 {
		t.Errorf("сумма_заказа[0] = %v (%T), хотим Целое 1200000", r0.Get("сумма_заказа"), r0.Get("сумма_заказа"))
	}
	if got, ok := r0.Get("статус").(value.Строка); !ok || got.V != "оплачен" {
		t.Errorf("статус[0] = %v, хотим Строка «оплачен»", r0.Get("статус"))
	}
	// дата_заказа — Строка (даты НЕ распознаются, §9.4).
	if _, ok := r0.Get("дата_заказа").(value.Строка); !ok {
		t.Errorf("дата_заказа[0] должна быть Строка, получено %T", r0.Get("дата_заказа"))
	}
}

// T019: различение Целое / Дробное / экспонента по форме токена (§9.3).
func TestLoadSourceNumberForms(t *testing.T) {
	dir := t.TempDir()
	path := writeJSON(t, dir, "nums.json",
		`[{"i": 1200000, "f": 1200000.0, "e": 1.2e6, "neg": -5, "fl": null, "b": true, "s": "x"}]`)
	i := newTestInterp()
	recs, err := i.loadSource(makeSourceDecl("числа", path))
	if err != nil {
		t.Fatalf("loadSource: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("записей = %d, хотим 1", len(recs))
	}
	r := recs[0]
	if v, ok := r.Get("i").(value.Целое); !ok || v.V != 1200000 {
		t.Errorf("i = %v (%T), хотим Целое 1200000", r.Get("i"), r.Get("i"))
	}
	if v, ok := r.Get("f").(value.Дробное); !ok || v.V != 1200000.0 {
		t.Errorf("f = %v (%T), хотим Дробное 1200000.0", r.Get("f"), r.Get("f"))
	}
	if v, ok := r.Get("e").(value.Дробное); !ok || v.V != 1.2e6 {
		t.Errorf("e = %v (%T), хотим Дробное 1.2e6", r.Get("e"), r.Get("e"))
	}
	if v, ok := r.Get("neg").(value.Целое); !ok || v.V != -5 {
		t.Errorf("neg = %v (%T), хотим Целое -5", r.Get("neg"), r.Get("neg"))
	}
	if _, ok := r.Get("fl").(value.Пусто); !ok {
		t.Errorf("fl (null) = %T, хотим Пусто", r.Get("fl"))
	}
	if v, ok := r.Get("b").(value.Булево); !ok || v.V != true {
		t.Errorf("b = %v (%T), хотим Булево true", r.Get("b"), r.Get("b"))
	}
	if v, ok := r.Get("s").(value.Строка); !ok || v.V != "x" {
		t.Errorf("s = %v (%T), хотим Строка «x»", r.Get("s"), r.Get("s"))
	}
}

// T019: вложенные массив/объект (§9.3) → Список / Запись (рекурсивно).
func TestLoadSourceNested(t *testing.T) {
	dir := t.TempDir()
	path := writeJSON(t, dir, "nested.json",
		`[{"теги": [1, 2, "три"], "адрес": {"город": "Москва", "индекс": 101000}}]`)
	i := newTestInterp()
	recs, err := i.loadSource(makeSourceDecl("гнёзда", path))
	if err != nil {
		t.Fatalf("loadSource: %v", err)
	}
	r := recs[0]
	lst, ok := r.Get("теги").(value.Список)
	if !ok {
		t.Fatalf("теги = %T, хотим Список", r.Get("теги"))
	}
	if len(*lst.Elems) != 3 {
		t.Errorf("len(теги) = %d, хотим 3", len(*lst.Elems))
	}
	if v, ok := (*lst.Elems)[0].(value.Целое); !ok || v.V != 1 {
		t.Errorf("теги[0] = %v, хотим Целое 1", (*lst.Elems)[0])
	}
	if v, ok := (*lst.Elems)[2].(value.Строка); !ok || v.V != "три" {
		t.Errorf("теги[2] = %v, хотим Строка «три»", (*lst.Elems)[2])
	}
	rec, ok := r.Get("адрес").(value.Запись)
	if !ok {
		t.Fatalf("адрес = %T, хотим Запись", r.Get("адрес"))
	}
	if v, ok := rec.Get("город").(value.Строка); !ok || v.V != "Москва" {
		t.Errorf("адрес.город = %v, хотим Строка «Москва»", rec.Get("город"))
	}
	if v, ok := rec.Get("индекс").(value.Целое); !ok || v.V != 101000 {
		t.Errorf("адрес.индекс = %v, хотим Целое 101000", rec.Get("индекс"))
	}
}

// T019: лень + кеш на запуск — повторный вызов не перечитывает файл.
// Проверяем удалением файла между вызовами: если кеш работает, второй вызов
// возвращает те же записи без ошибки «файл не найден».
func TestLoadSourceCached(t *testing.T) {
	dir := t.TempDir()
	path := writeJSON(t, dir, "cache.json", `[{"a": 1}]`)
	i := newTestInterp()
	decl := makeSourceDecl("кеш", path)

	first, err := i.loadSource(decl)
	if err != nil {
		t.Fatalf("первый loadSource: %v", err)
	}
	// Удаляем файл — последующее чтение бы упало, если бы кеша не было.
	if err := os.Remove(path); err != nil {
		t.Fatalf("удаление файла: %v", err)
	}
	second, err := i.loadSource(decl)
	if err != nil {
		t.Fatalf("второй loadSource (ожидался кеш): %v", err)
	}
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("len first=%d second=%d, хотим 1/1", len(first), len(second))
	}
	// Идемпотентность: один и тот же срез из кеша.
	if &first[0] != &second[0] {
		t.Errorf("кеш вернул другой срез (не идемпотентно)")
	}
}

// --- T020: exact-match ошибки загрузки §SM-9.B (тексты дословно) ---

// assertLoadErr вычисляет loadSource, проверяет ОшибкаВыполнения, позицию
// decl.Pos() и точный текст сообщения.
func assertLoadErr(t *testing.T, decl *ast.SourceDecl, wantMsg string) {
	t.Helper()
	i := newTestInterp()
	_, err := i.loadSource(decl)
	if err == nil {
		t.Fatalf("ожидалась ошибка загрузки, получено nil")
	}
	if !isRuntime(err) {
		t.Fatalf("ошибка не ОшибкаВыполнения: %T %v", err, err)
	}
	line, col, msg := evalErr(t, err)
	if msg != wantMsg {
		t.Errorf("msg = %q\nхотим %q", msg, wantMsg)
	}
	if line != srcDeclPos.Line || col != srcDeclPos.Col {
		t.Errorf("позиция = (%d,%d), хотим decl.Pos() (%d,%d)", line, col, srcDeclPos.Line, srcDeclPos.Col)
	}
}

// T020: файл не найден.
func TestLoadSourceErrNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "нет.json")
	assertLoadErr(t, makeSourceDecl("продажи", path),
		"источник 'продажи': файл «"+path+"» не найден")
}

// T020: верхний уровень не массив.
func TestLoadSourceErrNotArray(t *testing.T) {
	dir := t.TempDir()
	path := writeJSON(t, dir, "obj.json", `{"a": 1}`)
	assertLoadErr(t, makeSourceDecl("продажи", path),
		"источник 'продажи': ожидался JSON-массив объектов в «"+path+"»")
}

// T020: элемент массива не объект (N с 1).
func TestLoadSourceErrElemNotObject(t *testing.T) {
	dir := t.TempDir()
	path := writeJSON(t, dir, "elem.json", `[{"a": 1}, 42]`)
	assertLoadErr(t, makeSourceDecl("продажи", path),
		"источник 'продажи': запись 2 не является объектом")
}

// T020: битый JSON.
func TestLoadSourceErrBadJSON(t *testing.T) {
	dir := t.TempDir()
	path := writeJSON(t, dir, "bad.json", `[{"a": 1`)
	i := newTestInterp()
	_, err := i.loadSource(makeSourceDecl("продажи", path))
	if err == nil {
		t.Fatalf("ожидалась ошибка битого JSON")
	}
	if !isRuntime(err) {
		t.Fatalf("ошибка не ОшибкаВыполнения: %T", err)
	}
	_, _, msg := evalErr(t, err)
	prefix := "источник 'продажи': некорректный JSON в «" + path + "» ("
	if len(msg) < len(prefix) || msg[:len(prefix)] != prefix || msg[len(msg)-1] != ')' {
		t.Errorf("msg = %q\nхотим префикс %q…)", msg, prefix)
	}
}

// T020: целое вне диапазона int64 (запись N, поле).
func TestLoadSourceErrIntOverflow(t *testing.T) {
	dir := t.TempDir()
	// 99999999999999999999 — заведомо вне int64.
	path := writeJSON(t, dir, "big.json", `[{"a": 1}, {"big": 99999999999999999999}]`)
	assertLoadErr(t, makeSourceDecl("продажи", path),
		"источник 'продажи': запись 2, поле 'big': целое число вне диапазона")
}

// === 010-A1: коннекторы источника (Phase E, §SC-6) ===

// fieldSpec — пара (имя поля, аннотация типа) для конструкции схемы в тестах.
type fieldSpec struct{ name, typ string }

// makeTypedSourceDecl строит ast.SourceDecl с типом typ и схемой fields.
// typ=="" → v1-форма (тип: опущен). fields nil → schemaless.
func makeTypedSourceDecl(name, path, typ string, fields []fieldSpec) *ast.SourceDecl {
	sd := makeSourceDecl(name, path)
	if typ != "" {
		sd.Type = *ast.NewIdent(ast.Position{Line: 9, Col: 11}, typ)
		sd.TypePos = ast.Position{Line: 9, Col: 5}
	}
	if fields != nil {
		sd.FieldsPos = ast.Position{Line: 10, Col: 5}
		for k, f := range fields {
			sd.Fields = append(sd.Fields, ast.FieldDef{
				Name:     *ast.NewIdent(ast.Position{Line: 11 + k, Col: 9}, f.name),
				TypeName: *ast.NewIdent(ast.Position{Line: 11 + k, Col: 9 + len([]rune(f.name)) + 2}, f.typ),
				Pos:      ast.Position{Line: 11 + k, Col: 9},
			})
		}
	}
	return sd
}

// writeFixture пишет произвольную текстовую фикстуру в dir.
func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("запись фикстуры %s: %v", p, err)
	}
	return p
}

// ordersSchema — общая схема для эквивалентных наборов orders.{json,csv,ndjson}.
func ordersSchema() []fieldSpec {
	return []fieldSpec{
		{"дата_заказа", "Дата"},
		{"сумма_заказа", "Дробное"},
		{"статус", "Строка"},
		{"количество", "Целое"},
		{"оплачен", "Логическое"},
	}
}

// testdataPath строит путь к фикстуре в internal/eval/testdata/.
func testdataPath(name string) string { return filepath.Join("testdata", name) }

// T014: CSV с заголовком грузится; типы коэрснуты; лишние столбцы игнорируются.
func TestLoadCSVHeaderAndCoerce(t *testing.T) {
	i := newTestInterp()
	recs, err := i.loadSource(makeTypedSourceDecl("заказы", testdataPath("orders.csv"), "csv", ordersSchema()))
	if err != nil {
		t.Fatalf("loadSource csv: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("записей = %d, хотим 3", len(recs))
	}
	r0 := recs[0]
	if v, ok := r0.Get("дата_заказа").(value.Дата); !ok || v != (value.Дата{Year: 2026, Month: 5, Day: 10}) {
		t.Errorf("дата_заказа[0] = %v (%T), хотим Дата 2026-05-10", r0.Get("дата_заказа"), r0.Get("дата_заказа"))
	}
	if v, ok := r0.Get("сумма_заказа").(value.Дробное); !ok || v.V != 1200000 {
		t.Errorf("сумма_заказа[0] = %v (%T), хотим Дробное 1200000", r0.Get("сумма_заказа"), r0.Get("сумма_заказа"))
	}
	if v, ok := r0.Get("статус").(value.Строка); !ok || v.V != "оплачен" {
		t.Errorf("статус[0] = %v, хотим Строка «оплачен»", r0.Get("статус"))
	}
	if v, ok := r0.Get("количество").(value.Целое); !ok || v.V != 3 {
		t.Errorf("количество[0] = %v, хотим Целое 3", r0.Get("количество"))
	}
	if v, ok := r0.Get("оплачен").(value.Булево); !ok || v.V != true {
		t.Errorf("оплачен[0] = %v, хотим Булево истина", r0.Get("оплачен"))
	}
}

// T014: CSV с лишним столбцом — игнорируется (A1-6), схема коэрсится штатно.
func TestLoadCSVExtraColumnIgnored(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "extra.csv", "a,лишнее,b\n1,мусор,2.5\n")
	i := newTestInterp()
	recs, err := i.loadSource(makeTypedSourceDecl("e", path, "csv", []fieldSpec{{"a", "Целое"}, {"b", "Дробное"}}))
	if err != nil {
		t.Fatalf("loadSource csv: %v", err)
	}
	r := recs[0]
	if v, ok := r.Get("a").(value.Целое); !ok || v.V != 1 {
		t.Errorf("a = %v, хотим Целое 1", r.Get("a"))
	}
	if v, ok := r.Get("b").(value.Дробное); !ok || v.V != 2.5 {
		t.Errorf("b = %v, хотим Дробное 2.5", r.Get("b"))
	}
	// Лишнее поле сохранено как Строка (A1-6).
	if v, ok := r.Get("лишнее").(value.Строка); !ok || v.V != "мусор" {
		t.Errorf("лишнее = %v, хотим Строка «мусор»", r.Get("лишнее"))
	}
}

// T014: CSV без объявленного столбца → load-ошибка §SC-9.B (поз. decl).
func TestLoadCSVMissingHeaderColumn(t *testing.T) {
	assertLoadErr(t, makeTypedSourceDecl("заказы", testdataPath("orders_missing_col.csv"), "csv", ordersSchema()),
		"источник 'заказы': в заголовке CSV «"+testdataPath("orders_missing_col.csv")+"» отсутствует поле 'сумма_заказа'")
}

// T014: некорректный CSV → load-ошибка §SC-9.B (префикс exact, деталь — в скобках).
func TestLoadCSVMalformed(t *testing.T) {
	dir := t.TempDir()
	// Несогласованное число полей в строке → ошибка encoding/csv.
	path := writeFixture(t, dir, "bad.csv", "a,b\n1,2,3\n")
	i := newTestInterp()
	_, err := i.loadSource(makeTypedSourceDecl("c", path, "csv", []fieldSpec{{"a", "Строка"}, {"b", "Строка"}}))
	if err == nil {
		t.Fatalf("ожидалась ошибка некорректного CSV")
	}
	if !isRuntime(err) {
		t.Fatalf("ошибка не ОшибкаВыполнения: %T", err)
	}
	_, _, msg := evalErr(t, err)
	prefix := "источник 'c': некорректный CSV в «" + path + "» ("
	if len(msg) < len(prefix) || msg[:len(prefix)] != prefix || msg[len(msg)-1] != ')' {
		t.Errorf("msg = %q\nхотим префикс %q…)", msg, prefix)
	}
}

// T014: NDJSON с пустыми строками — сквозная нумерация записей с 1, пустые skip.
func TestLoadNDJSONBlankLinesSkipped(t *testing.T) {
	i := newTestInterp()
	recs, err := i.loadSource(makeTypedSourceDecl("заказы", testdataPath("orders.ndjson"), "ndjson", ordersSchema()))
	if err != nil {
		t.Fatalf("loadSource ndjson: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("записей = %d, хотим 3 (пустые строки пропущены)", len(recs))
	}
	// Типизация: дата → Дата, сумма (Целое в JSON) → промоушен Дробное.
	r0 := recs[0]
	if v, ok := r0.Get("дата_заказа").(value.Дата); !ok || v != (value.Дата{Year: 2026, Month: 5, Day: 10}) {
		t.Errorf("дата_заказа[0] = %v (%T), хотим Дата 2026-05-10", r0.Get("дата_заказа"), r0.Get("дата_заказа"))
	}
	if v, ok := r0.Get("сумма_заказа").(value.Дробное); !ok || v.V != 1200000 {
		t.Errorf("сумма_заказа[0] = %v (%T), хотим Дробное 1200000 (промоушен)", r0.Get("сумма_заказа"), r0.Get("сумма_заказа"))
	}
}

// T014: NDJSON не-объект на строке → load-ошибка «запись N не является объектом».
func TestLoadNDJSONNonObject(t *testing.T) {
	assertLoadErr(t, makeTypedSourceDecl("заказы", testdataPath("orders_nonobject.ndjson"), "ndjson", []fieldSpec{{"a", "Целое"}}),
		"источник 'заказы': запись 2 не является объектом")
}

// T014: NDJSON битый JSON на строке → «запись N: некорректный JSON (деталь)».
func TestLoadNDJSONBadJSON(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "bad.ndjson", "{\"a\": 1}\n{\"b\": \n")
	i := newTestInterp()
	_, err := i.loadSource(makeTypedSourceDecl("c", path, "ndjson", []fieldSpec{{"a", "Целое"}}))
	if err == nil {
		t.Fatalf("ожидалась ошибка битого JSON в NDJSON")
	}
	if !isRuntime(err) {
		t.Fatalf("ошибка не ОшибкаВыполнения: %T", err)
	}
	_, _, msg := evalErr(t, err)
	prefix := "источник 'c': запись 2: некорректный JSON ("
	if len(msg) < len(prefix) || msg[:len(prefix)] != prefix || msg[len(msg)-1] != ')' {
		t.Errorf("msg = %q\nхотим префикс %q…)", msg, prefix)
	}
}

// T015: матрица коэрсии applySchema — CSV-строки в Целое/Дробное/Логическое/Дата.
func TestApplySchemaCSVCoercion(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "c.csv", "i,f,b,d,s\n42,3.14,истина,2026-05-31,привет\n")
	i := newTestInterp()
	recs, err := i.loadSource(makeTypedSourceDecl("c", path, "csv",
		[]fieldSpec{{"i", "Целое"}, {"f", "Дробное"}, {"b", "Логическое"}, {"d", "Дата"}, {"s", "Строка"}}))
	if err != nil {
		t.Fatalf("loadSource: %v", err)
	}
	r := recs[0]
	if v, ok := r.Get("i").(value.Целое); !ok || v.V != 42 {
		t.Errorf("i = %v, хотим Целое 42", r.Get("i"))
	}
	if v, ok := r.Get("f").(value.Дробное); !ok || v.V != 3.14 {
		t.Errorf("f = %v, хотим Дробное 3.14", r.Get("f"))
	}
	if v, ok := r.Get("b").(value.Булево); !ok || v.V != true {
		t.Errorf("b = %v, хотим Булево истина", r.Get("b"))
	}
	if v, ok := r.Get("d").(value.Дата); !ok || v != (value.Дата{Year: 2026, Month: 5, Day: 31}) {
		t.Errorf("d = %v, хотим Дата 2026-05-31", r.Get("d"))
	}
	if v, ok := r.Get("s").(value.Строка); !ok || v.V != "привет" {
		t.Errorf("s = %v, хотим Строка «привет»", r.Get("s"))
	}
}

// T015: CSV-Дробное принимает и целочисленную, и дробную форму (§SC-D-COERCE-FLOAT).
func TestApplySchemaCSVFloatForms(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "f.csv", "a,b\n1200000,1200000.5\n")
	i := newTestInterp()
	recs, err := i.loadSource(makeTypedSourceDecl("c", path, "csv", []fieldSpec{{"a", "Дробное"}, {"b", "Дробное"}}))
	if err != nil {
		t.Fatalf("loadSource: %v", err)
	}
	r := recs[0]
	if v, ok := r.Get("a").(value.Дробное); !ok || v.V != 1200000 {
		t.Errorf("a = %v, хотим Дробное 1200000", r.Get("a"))
	}
	if v, ok := r.Get("b").(value.Дробное); !ok || v.V != 1200000.5 {
		t.Errorf("b = %v, хотим Дробное 1200000.5", r.Get("b"))
	}
}

// T015: JSON-промоушен Целое→Дробное (единственный кросс-тип, §SC-D-COERCE-PROMO).
func TestApplySchemaJSONIntPromotesToFloat(t *testing.T) {
	dir := t.TempDir()
	path := writeJSON(t, dir, "p.json", `[{"x": 7}]`)
	i := newTestInterp()
	recs, err := i.loadSource(makeTypedSourceDecl("c", path, "json", []fieldSpec{{"x", "Дробное"}}))
	if err != nil {
		t.Fatalf("loadSource: %v", err)
	}
	if v, ok := recs[0].Get("x").(value.Дробное); !ok || v.V != 7 {
		t.Errorf("x = %v (%T), хотим Дробное 7 (промоушен)", recs[0].Get("x"), recs[0].Get("x"))
	}
}

// T015: JSON-Дробное в Целое-поле → ошибка (демоушена нет, §SC-D-COERCE).
func TestApplySchemaJSONFloatToIntErr(t *testing.T) {
	dir := t.TempDir()
	path := writeJSON(t, dir, "fi.json", `[{"x": 1.5}]`)
	assertLoadErr(t, makeTypedSourceDecl("c", path, "json", []fieldSpec{{"x", "Целое"}}),
		"источник 'c': запись 1, поле 'x': ожидался Целое, получено Дробное")
}

// T015: CSV-Целое с десятичной точкой → «не является целым» (§SC-D-COERCE-INT).
func TestApplySchemaCSVIntWithDotErr(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "i.csv", "x\n12.5\n")
	assertLoadErr(t, makeTypedSourceDecl("c", path, "csv", []fieldSpec{{"x", "Целое"}}),
		"источник 'c': запись 1, поле 'x': «12.5» не является целым")
}

// T015: CSV-Дробное непарсимое → «не является дробным».
func TestApplySchemaCSVFloatBadErr(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "f.csv", "x\nабв\n")
	assertLoadErr(t, makeTypedSourceDecl("c", path, "csv", []fieldSpec{{"x", "Дробное"}}),
		"источник 'c': запись 1, поле 'x': «абв» не является дробным")
}

// T015: CSV-Логическое вне истина/ложь → «не является логическим».
func TestApplySchemaCSVBoolBadErr(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "b.csv", "x\nyes\n")
	assertLoadErr(t, makeTypedSourceDecl("c", path, "csv", []fieldSpec{{"x", "Логическое"}}),
		"источник 'c': запись 1, поле 'x': «yes» не является логическим (ожидалось истина/ложь)")
}

// T015: CSV-Логическое «ложь» → Булево false.
func TestApplySchemaCSVBoolFalse(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "b.csv", "x\nложь\n")
	i := newTestInterp()
	recs, err := i.loadSource(makeTypedSourceDecl("c", path, "csv", []fieldSpec{{"x", "Логическое"}}))
	if err != nil {
		t.Fatalf("loadSource: %v", err)
	}
	if v, ok := recs[0].Get("x").(value.Булево); !ok || v.V != false {
		t.Errorf("x = %v, хотим Булево ложь", recs[0].Get("x"))
	}
}

// T015: отсутствие объявленного поля (членство в Keys, не Get!=None) → §SC-9.B.
func TestApplySchemaMissingField(t *testing.T) {
	dir := t.TempDir()
	path := writeJSON(t, dir, "m.json", `[{"a": 1}]`)
	assertLoadErr(t, makeTypedSourceDecl("c", path, "json", []fieldSpec{{"a", "Целое"}, {"b", "Строка"}}),
		"источник 'c': запись 1: отсутствует объявленное поле 'b'")
}

// T015: лишнее (необъявленное) поле JSON сохраняется без ошибки (A1-6).
func TestApplySchemaExtraJSONFieldKept(t *testing.T) {
	dir := t.TempDir()
	path := writeJSON(t, dir, "x.json", `[{"a": 1, "extra": "v"}]`)
	i := newTestInterp()
	recs, err := i.loadSource(makeTypedSourceDecl("c", path, "json", []fieldSpec{{"a", "Целое"}}))
	if err != nil {
		t.Fatalf("loadSource: %v", err)
	}
	r := recs[0]
	if v, ok := r.Get("extra").(value.Строка); !ok || v.V != "v" {
		t.Errorf("extra = %v, хотим Строка «v» (лишнее поле сохранено)", r.Get("extra"))
	}
	// Порядок ключей сохранён.
	if keys := r.Keys(); len(keys) != 2 || keys[0] != "a" || keys[1] != "extra" {
		t.Errorf("Keys() = %v, хотим [a extra]", r.Keys())
	}
}

// T015: JSON null в объявленном поле — НЕ «отсутствует» (ключ есть), а A1-10
// «ожидался <Тип>, получено Пусто» (edge §SC-D-RECORD).
func TestApplySchemaJSONNullIsTypeMismatch(t *testing.T) {
	dir := t.TempDir()
	path := writeJSON(t, dir, "n.json", `[{"a": null}]`)
	assertLoadErr(t, makeTypedSourceDecl("c", path, "json", []fieldSpec{{"a", "Целое"}}),
		"источник 'c': запись 1, поле 'a': ожидался Целое, получено Пусто")
}

// T015: пустая CSV-ячейка в Целое-поле → «не является целым» (edge §SC-D-RECORD A1-7).
func TestApplySchemaCSVEmptyCellInt(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "e.csv", "a,b\n,5\n")
	assertLoadErr(t, makeTypedSourceDecl("c", path, "csv", []fieldSpec{{"a", "Целое"}, {"b", "Целое"}}),
		"источник 'c': запись 1, поле 'a': «» не является целым")
}

// T015: JSON-Строка-поле, но значение не Строка → A1-10.
func TestApplySchemaJSONStringMismatch(t *testing.T) {
	dir := t.TempDir()
	path := writeJSON(t, dir, "s.json", `[{"a": 5}]`)
	assertLoadErr(t, makeTypedSourceDecl("c", path, "json", []fieldSpec{{"a", "Строка"}}),
		"источник 'c': запись 1, поле 'a': ожидался Строка, получено Целое")
}

// T016: распознавание дат — поле Дата из строки ISO → value.Дата (без дата(...)).
func TestApplySchemaDateRecognition(t *testing.T) {
	dir := t.TempDir()
	path := writeJSON(t, dir, "d.json", `[{"d": "2026-05-31"}]`)
	i := newTestInterp()
	recs, err := i.loadSource(makeTypedSourceDecl("c", path, "json", []fieldSpec{{"d", "Дата"}}))
	if err != nil {
		t.Fatalf("loadSource: %v", err)
	}
	if v, ok := recs[0].Get("d").(value.Дата); !ok || v != (value.Дата{Year: 2026, Month: 5, Day: 31}) {
		t.Errorf("d = %v (%T), хотим Дата 2026-05-31", recs[0].Get("d"), recs[0].Get("d"))
	}
}

// T016: невалидная дата (календарь/формат) → §SC-9.B «не является датой».
func TestApplySchemaDateInvalid(t *testing.T) {
	for _, tc := range []struct{ name, val string }{
		{"невалидный календарь", "2026-13-40"},
		{"неверный формат", "31.05.2026"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeJSON(t, dir, "d.json", `[{"d": "`+tc.val+`"}]`)
			assertLoadErr(t, makeTypedSourceDecl("c", path, "json", []fieldSpec{{"d", "Дата"}}),
				"источник 'c': запись 1, поле 'd': «"+tc.val+"» не является датой (ожидался формат ГГГГ-ММ-ДД)")
		})
	}
}

// T016: CSV-Дата из строки ISO → value.Дата; невалидная → §SC-9.B.
func TestApplySchemaCSVDate(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "d.csv", "d\n2026-02-29\n")
	// 2026 не високосный → 29 февраля невалидно.
	assertLoadErr(t, makeTypedSourceDecl("c", path, "csv", []fieldSpec{{"d", "Дата"}}),
		"источник 'c': запись 1, поле 'd': «2026-02-29» не является датой (ожидался формат ГГГГ-ММ-ДД)")
}

// T016: JSON-Дата-поле, но значение не Строка → A1-10 (иначе).
func TestApplySchemaJSONDateNonString(t *testing.T) {
	dir := t.TempDir()
	path := writeJSON(t, dir, "d.json", `[{"d": 5}]`)
	assertLoadErr(t, makeTypedSourceDecl("c", path, "json", []fieldSpec{{"d", "Дата"}}),
		"источник 'c': запись 1, поле 'd': ожидался Дата, получено Целое")
}

// === 010-A1 hardening (adversarial self-check): 4 MAJOR-замка загрузчика CSV ===

// bom — ведущий UTF-8 BOM (байты EF BB BF). Записываем фикстуры байтами, чтобы BOM
// был точным, а не приклеенным редактором.
const bom = "\ufeff"

// Defect 1+2: ведущий UTF-8 BOM на CSV приклеивается к имени первого столбца («ид» →
// «\ufeffид») → ложная «отсутствует поле 'ид'» ИЛИ тихая привязка к мохибейк-ключу.
// loadCSV ОБЯЗАН снять BOM с header[0]: поле 'ид' достижимо, ошибки нет.
func TestLoadCSVStripsBOMHeader(t *testing.T) {
	dir := t.TempDir()
	// EF BB BF + «ид,статус\n7,оплачен\n».
	path := writeFixture(t, dir, "bom.csv", bom+"ид,статус\n7,оплачен\n")
	i := newTestInterp()
	recs, err := i.loadSource(makeTypedSourceDecl("заказы", path, "csv",
		[]fieldSpec{{"ид", "Целое"}, {"статус", "Строка"}}))
	if err != nil {
		t.Fatalf("loadSource csv с BOM: %v (BOM не снят с header[0]?)", err)
	}
	if len(recs) != 1 {
		t.Fatalf("записей = %d, хотим 1", len(recs))
	}
	r := recs[0]
	// Поле 'ид' достижимо по настоящему имени (а не по «\ufeffид»).
	if v, ok := r.Get("ид").(value.Целое); !ok || v.V != 7 {
		t.Errorf("ид = %v (%T), хотим Целое 7 — BOM приклеился к ключу?", r.Get("ид"), r.Get("ид"))
	}
	if v, ok := r.Get("статус").(value.Строка); !ok || v.V != "оплачен" {
		t.Errorf("статус = %v, хотим Строка «оплачен»", r.Get("статус"))
	}
	// Мохибейк-ключ с BOM НЕ должен существовать.
	if _, ok := r.Get(bom + "ид").(value.Строка); ok {
		t.Errorf("мохибейк-ключ «\\ufeffид» присутствует — BOM не снят")
	}
}

// Defect 2 (NDJSON): ведущий BOM на первой непустой строке NDJSON → json.Valid ложно
// падает. loadNDJSON ОБЯЗАН снять BOM с первой непустой строки.
func TestLoadNDJSONStripsBOMFirstLine(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "bom.ndjson", bom+`{"ид": 7, "статус": "оплачен"}`+"\n")
	i := newTestInterp()
	recs, err := i.loadSource(makeTypedSourceDecl("заказы", path, "ndjson",
		[]fieldSpec{{"ид", "Целое"}, {"статус", "Строка"}}))
	if err != nil {
		t.Fatalf("loadSource ndjson с BOM: %v (BOM не снят с первой строки?)", err)
	}
	if len(recs) != 1 {
		t.Fatalf("записей = %d, хотим 1", len(recs))
	}
	if v, ok := recs[0].Get("ид").(value.Целое); !ok || v.V != 7 {
		t.Errorf("ид = %v (%T), хотим Целое 7", recs[0].Get("ид"), recs[0].Get("ид"))
	}
}

// Defect 3: дубликат имени столбца в заголовке CSV («ид,статус,статус») тихо
// схлопывается (последний побеждает) → потеря данных. loadCSV ОБЯЗАН выдать
// типизированную §SC-9.B-ошибку с позицией decl, байт-в-байт.
func TestLoadCSVDuplicateHeaderColumn(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, "dup.csv", "ид,статус,статус\n7,оплачен,новый\n")
	assertLoadErr(t, makeTypedSourceDecl("заказы", path, "csv",
		[]fieldSpec{{"ид", "Целое"}, {"статус", "Строка"}}),
		"источник 'заказы': в заголовке CSV «"+path+"» столбец 'статус' объявлен дважды")
}

// Defect 4: CSV-Дробное отвергает нечисловые ±Inf/NaN (ParseFloat принимает их без
// ошибки) — иначе отравляют агрегаты.
func TestApplySchemaCSVFloatRejectsNonFinite(t *testing.T) {
	for _, s := range []string{"Inf", "+Inf", "-Inf", "Infinity", "NaN", "inf", "nan"} {
		t.Run(s, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFixture(t, dir, "nf.csv", "x\n"+s+"\n")
			assertLoadErr(t, makeTypedSourceDecl("c", path, "csv", []fieldSpec{{"x", "Дробное"}}),
				"источник 'c': запись 1, поле 'x': «"+s+"» не является дробным")
		})
	}
}

// Defect 4: CSV-Дробное отвергает Go hex-float литералы («0x1p4» → 16.0) — это
// Go-синтаксис, не данные.
func TestApplySchemaCSVFloatRejectsHexFloat(t *testing.T) {
	for _, s := range []string{"0x1p4", "0X1P4", "0x1.8p3"} {
		t.Run(s, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFixture(t, dir, "hf.csv", "x\n"+s+"\n")
			assertLoadErr(t, makeTypedSourceDecl("c", path, "csv", []fieldSpec{{"x", "Дробное"}}),
				"источник 'c': запись 1, поле 'x': «"+s+"» не является дробным")
		})
	}
}

// Defect 4 (регресс): десятичная экспонента «1e3» и обычные формы остаются валидными
// после ужесточения (consistency с JSON number form).
func TestApplySchemaCSVFloatExponentStillValid(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want float64
	}{
		{"1e3", 1000}, {"1.5", 1.5}, {"1200000", 1200000}, {"1200000.5", 1200000.5}, {"-2.5e-1", -0.25},
	} {
		t.Run(tc.s, func(t *testing.T) {
			dir := t.TempDir()
			path := writeFixture(t, dir, "ok.csv", "x\n"+tc.s+"\n")
			i := newTestInterp()
			recs, err := i.loadSource(makeTypedSourceDecl("c", path, "csv", []fieldSpec{{"x", "Дробное"}}))
			if err != nil {
				t.Fatalf("loadSource (валидная форма %q): %v", tc.s, err)
			}
			if v, ok := recs[0].Get("x").(value.Дробное); !ok || v.V != tc.want {
				t.Errorf("x = %v (%T), хотим Дробное %v", recs[0].Get("x"), recs[0].Get("x"), tc.want)
			}
		})
	}
}
