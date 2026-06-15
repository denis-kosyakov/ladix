package eval

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// T020 (Phase F, US3, §SC-10 #2) — ЭКВИВАЛЕНТНОСТЬ ФОРМАТОВ. Один и тот же набор
// записей в трёх формах (testdata/orders.{json,csv,ndjson}) под ОДНОЙ схемой и ОДНОЙ
// дата-зависимой метрикой даёт БАЙТ-ТОЧНО равный скаляр. Детерминизм — testClock
// (FixedClock 2026-05-31), что ставит окно «ежемесячно» на май 2026.
//
// Метрика умышленно использует `по_дате: дата_заказа` — ГОЛОЕ Дата-поле схемы (без
// явного дата(...)). Это и есть распознавание дат A1 (§SC-D-DATE): поле, объявленное
// `Дата`, парсится из строки ISO в value.Дата и течёт в ось `по_дате` одинаково во
// всех трёх форматах. `сумма_заказа: Дробное` упражняет коэрсию-промоушен Целое→Дробное
// (JSON 1200000 — Целое; CSV «1200000» — строка; NDJSON 1200000 — Целое) → во всех
// формах Дробное, суммы совпадают до бита.
//
// Ожидание: майские ОПЛАЧЕННЫЕ заказы 1200000 + 350000.5 = 1550000.5 (новый-заказ
// 99000 отфильтрован `где статус == "оплачен"`). Все три формы → «1550000.5».
//
// 🔁 ИНВЕРСИЯ: если коэрсия/дата-парс/диспетчер разойдутся между форматами — скаляры
// перестанут совпадать и/или разойдутся с пином → красный.
func TestSourceFormatEquivalenceGolden(t *testing.T) {
	const want = "1550000.5"
	formats := []string{"json", "csv", "ndjson"}
	got := make(map[string]string, len(formats))
	for _, f := range formats {
		i := buildFormatInterp(t, f)
		v, err := i.evalMetricByName("выручка_месяца", ast.Position{Line: 1, Col: 1})
		if err != nil {
			t.Fatalf("формат %s: evalMetric вернул ошибку: %v", f, err)
		}
		got[f] = value.String(v)
		if got[f] != want {
			t.Errorf("формат %s: скаляр = %q, хотим %q", f, got[f], want)
		}
	}
	// Все три формы — байт-в-байт идентичны (не только равны пину, но и друг другу).
	if got["json"] != got["csv"] || got["csv"] != got["ndjson"] {
		t.Errorf("форматы разошлись: json=%q csv=%q ndjson=%q", got["json"], got["csv"], got["ndjson"])
	}
}

// T021 (Phase F, US1, §SC-10 #3) — ДИСПЕТЧЕР «`тип:` опущен → json schemaless».
// Источник БЕЗ `тип:`/`поля:` грузится ровно как v1 JSON (loadJSON, БЕЗ applySchema):
// нативные JSON-типы сохраняются (Целое остаётся Целым, НЕ коэрсится в Дробное), а
// метрика-скаляр совпадает с v1-расчётом. Это замок на регресс диспетчера loadSource
// (§SC-6: «» → json).
//
// 🔁 ИНВЕРСИЯ: если опущенный `тип:` перестанет означать v1-JSON (напр. начнёт
// коэрсить/менять типы или разойдётся в скаляре) — красный.
func TestSourceDispatchOmittedTypeIsJSON(t *testing.T) {
	// v1-форма: ни `тип:`, ни `поля:` — schemaless JSON (как до 010).
	src := "" +
		"источник заказы:\n" +
		"    файл: \"X\"\n\n" +
		"метрика всего:\n" +
		"    источник: заказы\n" +
		"    агрегат:  сумма(сумма_заказа)\n"
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("лексические/синтаксические ошибки: %v", errList.Error())
	}
	for _, item := range prog.Items {
		if sd, ok := item.(*ast.SourceDecl); ok {
			sd.File.Value = filepath.Join("testdata", "orders.json")
		}
	}
	i := NewInterpreter(&bytes.Buffer{}, 0, testClock)
	if err := i.Analyze(prog); err != nil {
		t.Fatalf("Analyze вернул ошибку: %v", err)
	}

	// (1) Скаляр v1: сумма всех (без где) сумма_заказа = 1200000 + 350000.5 + 99000.
	v, err := i.evalMetricByName("всего", ast.Position{Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("evalMetric: %v", err)
	}
	if got := value.String(v); got != "1649000.5" {
		t.Errorf("schemaless скаляр = %q, хотим %q", got, "1649000.5")
	}

	// (2) Нативные JSON-типы СОХРАНЕНЫ (нет applySchema): сумма_заказа первой записи —
	// Целое (1200000), а НЕ коэрснутое Дробное. Это и отличает диспетчер «→ json» от
	// схематизированного пути.
	recs, err := i.loadSource(prog.Items[0].(*ast.SourceDecl))
	if err != nil {
		t.Fatalf("loadSource: %v", err)
	}
	if got, ok := recs[0].Get("сумма_заказа").(value.Целое); !ok || got.V != 1200000 {
		t.Errorf("сумма_заказа[0] = %v (%T), хотим нативное Целое 1200000 (schemaless, без коэрсии)",
			recs[0].Get("сумма_заказа"), recs[0].Get("сумма_заказа"))
	}
}

// buildFormatInterp парсит дата-зависимую метрику над типизированным источником
// формата fmtName, переписывает путь файла на testdata/orders.<fmt> и возвращает
// интерпретатор с testClock (FixedClock 2026-05-31). Схема — ordersSchema() (общая
// для эквивалентных наборов из source_loader_test.go).
func buildFormatInterp(t *testing.T, fmtName string) *Interpreter {
	t.Helper()
	src := "" +
		"источник заказы:\n" +
		"    файл: \"X\"\n" +
		"    тип: " + fmtName + "\n" +
		"    поля:\n" +
		"        дата_заказа: Дата\n" +
		"        сумма_заказа: Дробное\n" +
		"        статус: Строка\n" +
		"        количество: Целое\n" +
		"        оплачен: Логическое\n\n" +
		"метрика выручка_месяца:\n" +
		"    источник: заказы\n" +
		"    где:      статус == \"оплачен\"\n" +
		"    агрегат:  сумма(сумма_заказа)\n" +
		"    период:   ежемесячно\n" +
		"    по_дате:  дата_заказа\n"
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("формат %s: лексические/синтаксические ошибки: %v", fmtName, errList.Error())
	}
	for _, item := range prog.Items {
		if sd, ok := item.(*ast.SourceDecl); ok {
			sd.File.Value = filepath.Join("testdata", "orders."+fmtName)
		}
	}
	i := NewInterpreter(&bytes.Buffer{}, 0, testClock)
	if err := i.Analyze(prog); err != nil {
		t.Fatalf("формат %s: Analyze вернул ошибку: %v", fmtName, err)
	}
	return i
}
