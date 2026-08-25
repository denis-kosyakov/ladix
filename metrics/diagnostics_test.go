package metrics

// Задача 2.3 — негативные сценарии (design.md Д-5, spec.md Requirements
// «Диагностики в идиоме ir.Diagnostic», «Валидация канонических строк»,
// docs/eval-model.md §8.3, docs/source-metric-model.md §SM-9). Тексты —
// ДОСЛОВНО из реестров, НЕ переформулированы. Позиции — строка 1 (канонические
// строки однострочные), колонка в рунах ВНУТРИ канонической строки атрибута
// (design.md Д-5/Д-10: смещение синтетической обёртки скомпенсировано внутри
// фасада — снаружи виден только текст самого атрибута).

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/ir"
)

// checkOneDiag проверяет: err != nil и errors.Is(err, ErrEvaluation); ровно
// одна диагностика с Severity="error", Stage="runtime", дословным текстом и
// позицией.
func checkOneDiag(t *testing.T, diags []ir.Diagnostic, err error, wantMsg string, wantPos ir.Position) {
	t.Helper()
	if err == nil {
		t.Fatalf("err == nil, ожидалась ошибка")
	}
	if !errors.Is(err, ErrEvaluation) {
		t.Errorf("errors.Is(err, ErrEvaluation) == false; err = %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("диагностик = %d, хотим ровно 1: %+v", len(diags), diags)
	}
	d := diags[0]
	if d.Severity != "error" {
		t.Errorf("Severity = %q, хотим %q", d.Severity, "error")
	}
	if d.Stage != "runtime" {
		t.Errorf("Stage = %q, хотим %q", d.Stage, "runtime")
	}
	if d.Message != wantMsg {
		t.Errorf("Message = %q, хотим %q (дословно из реестра)", d.Message, wantMsg)
	}
	if d.Pos != wantPos {
		t.Errorf("Pos = %+v, хотим %+v", d.Pos, wantPos)
	}
}

// TestDiagnosticDivisionByZero — docs/eval-model.md §8.3 «деление на ноль»,
// позиция — токен оператора '/' (§8.2). Каноническая строка "(1 / 0)":
// '(' col1 '1' col2 ' ' col3 '/' col4 ' ' col5 '0' col6 ')' col7.
func TestDiagnosticDivisionByZero(t *testing.T) {
	m := baseMetric("(1 / 0)", "количество(ид)", "", "")
	_, diags, err := Evaluate(m, salesRecords(), Options{Today: may31()})
	checkOneDiag(t, diags, err, "деление на ноль", ir.Position{Line: 1, Col: 4})
}

// TestDiagnosticWhereNotBoolean — docs/source-metric-model.md §SM-9.C
// «'где' должно давать Булево, получено <тип>». Where="1" — весь текст
// является узлом IntLit в колонке 1.
func TestDiagnosticWhereNotBoolean(t *testing.T) {
	m := baseMetric("1", "количество(ид)", "", "")
	_, diags, err := Evaluate(m, salesRecords(), Options{Today: may31()})
	checkOneDiag(t, diags, err,
		"'где' должно давать Булево, получено Целое", ir.Position{Line: 1, Col: 1})
}

// TestDiagnosticByDateWrongType — §SM-9.B/C «'по_дате' должно давать Дата или
// Пусто, получено <тип>». ByDate="1" (весь текст — IntLit, колонка 1); период
// задан, чтобы пара период/по_дате была структурно валидна (§SM-9.A pairing —
// не в фокусе этого теста).
func TestDiagnosticByDateWrongType(t *testing.T) {
	m := baseMetric("", "количество(ид)", "ежемесячно", "1")
	_, diags, err := Evaluate(m, salesRecords(), Options{Today: may31()})
	checkOneDiag(t, diags, err,
		"'по_дате' должно давать Дата или Пусто, получено Целое", ir.Position{Line: 1, Col: 1})
}

// TestDiagnosticUnknownField — §SM-9.C «неизвестное поле '<имя>' (известные:
// <поле1>, <поле2>, …)», список полей отсортирован лексикографически (Д-9).
// Where="(несуществующее == 1)": '(' col1, идент "несуществующее" (14 рун)
// начинается с col2.
func TestDiagnosticUnknownField(t *testing.T) {
	m := baseMetric("(несуществующее == 1)", "количество(ид)", "", "")
	records := []map[string]any{{"a": int64(1), "b": int64(2)}}
	_, diags, err := Evaluate(m, records, Options{Today: may31()})
	checkOneDiag(t, diags, err,
		"неизвестное поле 'несуществующее' (известные: a, b)", ir.Position{Line: 1, Col: 2})
}

// TestDiagnosticInvalidCanonicalExpression — spec.md Requirement «Валидация
// канонических строк»: текст `метрика '<имя>': <атрибут> недопустимое
// каноническое выражение`. Where="@" — мусор с первого символа, канонический
// текст атрибута начинается с колонки 1 → позиция недопустимого места col1.
func TestDiagnosticInvalidCanonicalExpression(t *testing.T) {
	m := baseMetric("@", "количество(ид)", "", "")
	_, diags, err := Evaluate(m, salesRecords(), Options{Today: may31()})
	checkOneDiag(t, diags, err,
		"метрика 'м': где: недопустимое каноническое выражение", ir.Position{Line: 1, Col: 1})
}

// TestDiagnosticFieldOutsideAggregate — §10.3/§SM-9.C «поле '<имя>'
// использовано вне агрегатной функции». Aggregate="сумма_заказа" — голое
// имя поля целиком (колонка 1).
func TestDiagnosticFieldOutsideAggregate(t *testing.T) {
	m := baseMetric("", "сумма_заказа", "", "")
	_, diags, err := Evaluate(m, salesRecords(), Options{Today: may31()})
	checkOneDiag(t, diags, err,
		"поле 'сумма_заказа' использовано вне агрегатной функции", ir.Position{Line: 1, Col: 1})
}

// TestDiagnosticNestedAggregate — §10.3/§SM-9.C «вложенный агрегат
// недопустим». Aggregate="сумма(среднее(сумма_заказа))". Позиция —
// ДОПУЩЕНИЕ (открытый вопрос к реализатору, реестр не фиксирует узел для
// этого конкретного случая): по аналогии с конвенцией §8.2
// (CallExpr.Pos()=Callee.Pos()) берётся позиция Callee вложенного вызова
// 'среднее' — "сумма(среднее(...": 'с'(1)'у'(2)'м'(3)'м'(4)'а'(5)'('(6),
// 'среднее' начинается с колонки 7.
func TestDiagnosticNestedAggregate(t *testing.T) {
	m := baseMetric("", "сумма(среднее(сумма_заказа))", "", "")
	_, diags, err := Evaluate(m, salesRecords(), Options{Today: may31()})
	checkOneDiag(t, diags, err,
		"вложенный агрегат недопустим", ir.Position{Line: 1, Col: 7})
}

// TestDiagnosticUnsupportedGoType — spec.md Requirement «Форма данных
// потребителя», Scenario «Неподдерживаемый Go-тип»: time.Time / struct вне
// JSON-семантики (§9.3) → структурная диагностика, НЕ паника.
//
// Текст ДОСЛОВЕН и зафиксирован спекой: `источник '<ист>': запись <N>: поле
// '<поле>': неподдерживаемый тип значения` (реестр — docs/diagnostics-model.md
// §MDX-2, docs/source-metric-model.md §SM-9.B). Пунктуация — точка расхождения
// с соседним текстом загрузчика «запись N, поле '<поле>': целое число вне
// диапазона» (ЗАПЯТАЯ): здесь после «запись <N>» ДВОЕТОЧИЕ. Проверять только
// структурные свойства (Severity/Stage) недостаточно — это оставляло бы
// Принцип VIII без замка, и переформулировка текста прошла бы незамеченной.
// Нумерация записей — с 1: вторая запись даёт «запись 2».
func TestDiagnosticUnsupportedGoType(t *testing.T) {
	type чужаяСтруктура struct{ A int }

	cases := []struct {
		name    string
		records []map[string]any
		wantMsg string
	}{
		{
			name:    "time.Time в первой записи",
			records: []map[string]any{{"ид": int64(1), "когда": time.Now()}},
			wantMsg: "источник 'продажи': запись 1: поле 'когда': неподдерживаемый тип значения",
		},
		{
			name: "struct во ВТОРОЙ записи (нумерация с 1)",
			records: []map[string]any{
				{"ид": int64(1)},
				{"ид": int64(2), "чужое": чужаяСтруктура{A: 1}},
			},
			wantMsg: "источник 'продажи': запись 2: поле 'чужое': неподдерживаемый тип значения",
		},
		{
			name:    "вложенный неподдерживаемый тип внутри списка",
			records: []map[string]any{{"ид": int64(1), "сп": []any{int64(1), time.Now()}}},
			wantMsg: "источник 'продажи': запись 1: поле 'сп': неподдерживаемый тип значения",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := baseMetric("", "количество(ид)", "", "")
			_, diags, err := Evaluate(m, tc.records, Options{Today: may31()})
			checkOneDiag(t, diags, err, tc.wantMsg, ir.Position{Line: 1, Col: 1})
		})
	}
}

// TestDiagnosticIntOutOfRangeText — соседний текст реестра §SM-9.B с ДРУГИМ
// разделителем («запись N,» — ЗАПЯТАЯ, не двоеточие): замок на то, что две
// пунктуации не перепутаны местами (обе ветки recordFieldMessage).
func TestDiagnosticIntOutOfRangeText(t *testing.T) {
	m := baseMetric("", "количество(ид)", "", "")
	records := []map[string]any{{"ид": int64(1), "огромное": uint64(1) << 63}}
	_, diags, err := Evaluate(m, records, Options{Today: may31()})
	checkOneDiag(t, diags, err,
		"источник 'продажи': запись 1, поле 'огромное': целое число вне диапазона",
		ir.Position{Line: 1, Col: 1})
}

// TestEvaluateNeverPanics — spec.md Requirement «Диагностики в идиоме
// ir.Diagnostic», Scenario «Паника не пересекает границу API» (design.md Д-5:
// recover-барьер на границе фасада). Прогоняет заведомо злые входы и требует
// либо структурную диагностику, либо ошибку Go — но никогда панику.
func TestEvaluateNeverPanics(t *testing.T) {
	cases := []struct {
		name    string
		metric  ir.Metric
		records []map[string]any
		opts    Options
		// wantErr — по умолчанию true (злой вход ожидаемо даёт ошибку/диагностику).
		// "nil records" — ИСКЛЮЧЕНИЕ (spec.md Requirement «Исполнение метрики над
		// данными потребителя», Scenario «Пустой результат»: набор выживших
		// записей ПУСТ и без единой входной записи — единичный количество(...)
		// корректно даёт Целое 0 по правилам SPEC §10.5, это НЕ ошибка). Здесь
		// проверяется только отсутствие паники.
		wantErr bool
	}{
		{"nil records", baseMetric("", "количество(ид)", "", ""), nil, Options{Today: may31()}, false},
		{"пустой ir.Metric", ir.Metric{}, salesRecords(), Options{Today: may31()}, true},
		{
			"мусор во всех четырёх строках",
			ir.Metric{Name: "м", Source: "продажи", Where: ")))", Aggregate: "((( )", Period: "!!!", ByDate: "???", Pos: metricPos()},
			salesRecords(),
			Options{Today: may31()},
			true,
		},
		{"нулевые Options", baseMetric("", "количество(ид)", "", ""), salesRecords(), Options{}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Evaluate() паникнул: %v", r)
				}
			}()
			_, _, err := Evaluate(tc.metric, tc.records, tc.opts)
			if tc.wantErr && err == nil {
				t.Errorf("err == nil для заведомо злого входа %q — ожидалась ошибка/диагностика", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("err = %v для входа %q — по спеке (§10.5) ожидался валидный результат без ошибки", err, tc.name)
			}
		})
	}
}

// TestDiagnosticColumnsAreRunesNotBytes — Принцип IV: колонка считается в РУНАХ.
// В "(\"🙂🙂\" == \"🙂🙂\") и (1 / 0)" оператор '/' — 21-я РУНА, но 34-й БАЙТ;
// байтовый подсчёт покраснил бы этот тест. Эмодзи выбраны намеренно: 4 байта на
// руну (в кириллице — 2), поэтому расхождение видно даже при «полукорректном»
// подсчёте по UTF-16/парам байт. Левый операнд 'и' истинен, поэтому короткое
// замыкание НЕ прячет деление на ноль.
func TestDiagnosticColumnsAreRunesNotBytes(t *testing.T) {
	where := `("🙂🙂" == "🙂🙂") и (1 / 0)`
	if idx := strings.Index(where, "/"); idx+1 == 21 {
		t.Fatalf("байтовый и рунный индексы совпали — тест перестал различать подсчёты")
	}
	_, diags, err := Evaluate(baseMetric(where, "количество(ид)", "", ""),
		salesRecords(), Options{Today: may31()})
	checkOneDiag(t, diags, err, "деление на ноль", ir.Position{Line: 1, Col: 21})
}

// TestDiagnosticMultilineAttributeRejected — замок на СТРУКТУРНУЮ ИНЪЕКЦИЮ через
// многострочную «каноническую» строку: атрибут попадает в текст синтетической
// декларации отдельной строкой, поэтому перевод строки в Where дописал бы метрике
// период:/по_дате:, которых в ir.Metric нет, и Evaluate посчитал бы ДРУГУЮ метрику
// молча и успешно. Канонические строки однострочны (ast.canonExpr печатает
// строковые литералы через strconv.Quote), значит такой вход НЕ канонический и
// обязан давать диагностику spec.md «Валидация канонических строк».
func TestDiagnosticMultilineAttributeRejected(t *testing.T) {
	cases := []struct {
		name    string
		metric  ir.Metric
		wantMsg string
		wantCol int
	}{
		{
			name: "инъекция период:/по_дате: через где:",
			metric: ir.Metric{Name: "м", Source: "продажи", Aggregate: "количество(ид)",
				Pos:   metricPos(),
				Where: "истина\n    период:   ежемесячно\n    по_дате:  дата_заказа"},
			wantMsg: "метрика 'м': где: недопустимое каноническое выражение",
			wantCol: 7, // руна сразу за "истина"
		},
		{
			name: "перевод строки в агрегат:",
			metric: ir.Metric{Name: "м", Source: "продажи",
				Pos: metricPos(), Aggregate: "количество(ид)\n    где: ложь"},
			wantMsg: "метрика 'м': агрегат: недопустимое каноническое выражение",
			wantCol: 15,
		},
		{
			name: "возврат каретки в по_дате:",
			metric: ir.Metric{Name: "м", Source: "продажи", Aggregate: "количество(ид)",
				Pos: metricPos(), Period: "ежемесячно", ByDate: "дата_заказа\r"},
			wantMsg: "метрика 'м': по_дате: недопустимое каноническое выражение",
			wantCol: 12,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, diags, err := Evaluate(tc.metric, salesRecords(),
				Options{Today: may31(), Fields: maySchema()})
			checkOneDiag(t, diags, err, tc.wantMsg, ir.Position{Line: 1, Col: tc.wantCol})
		})
	}
}
