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
