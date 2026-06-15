package eval

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// windowClock — детерминированный Clock для оконных метрик A2. ДАТА-НЕ-КОНЕЦ-МЕСЯЦА
// (2026-06-15), как требует §MW-D-WINDOW-EDGE: «последние 30дн» от конца месяца
// (fixedClock2026={2026,5,31}) дало бы неинтуитивную нормализацию AddDate; 2026-06-15
// держит окно (2026-05-16, 2026-06-15] чистым.
var windowClock = FixedClock{D: value.Дата{Year: 2026, Month: 6, Day: 15}}

// buildWindowBoundsInterp парсит метрику над ВЫДЕЛЕННОЙ фикстурой границ окна
// (testdata/window_bounds.json — записи ровно на 2026-05-16/05-17/06-15), переписывает
// путь источника на эту фикстуру и возвращает интерпретатор с windowClock {2026,6,15}
// + буфер его stdout (печать из тела триггера пишет в i.out, не в writer RunTriggers).
// Фикстура отдельная (НЕ data/orders.csv — общий с A1): три записи позволяют пиннить
// мембершип у обеих границ полуинтервала (d−N исключена, d−N+1 и d включены).
func buildWindowBoundsInterp(t *testing.T, src string) (*Interpreter, *bytes.Buffer) {
	t.Helper()
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("лексические/синтаксические ошибки: %v", errList.Error())
	}
	for _, item := range prog.Items {
		if sd, ok := item.(*ast.SourceDecl); ok {
			sd.File.Value = filepath.Join("testdata", "window_bounds.json")
		}
	}
	out := &bytes.Buffer{}
	i := NewInterpreter(out, 0, windowClock)
	if err := i.Analyze(prog); err != nil {
		t.Fatalf("Analyze вернул ошибку: %v", err)
	}
	return i, out
}

// windowBoundsSrc собирает декларатив с типизированным источником (поле дата_заказа:
// Дата — распознавание дат A1) и метрикой m с заданным period (период:). по_дате —
// голое Дата-поле дата_заказа (без обёртки дата(), §SC-D-DATE).
func windowBoundsSrc(period string) string {
	return "" +
		"источник заказы:\n" +
		"    файл: \"X\"\n" +
		"    тип: json\n" +
		"    поля:\n" +
		"        дата_заказа: Дата\n" +
		"        сумма_заказа: Целое\n" +
		"        статус: Строка\n\n" +
		"метрика m:\n" +
		"    источник: заказы\n" +
		"    агрегат:  сумма(сумма_заказа)\n" +
		"    период:   " + period + "\n" +
		"    по_дате:  дата_заказа\n"
}

// T019 (Phase F, US1, SC-003/§MW-9 #1) — ГРАНИЦА СКОЛЬЗЯЩЕГО ОКНА на FixedClock
// {2026,6,15}. Метрика «последние 30дн» над выделенной фикстурой границ: записи ровно
// на 2026-05-16 (= d−30, НИЖНЯЯ граница), 2026-05-17 (= d−30+1) и 2026-06-15 (= d,
// ВЕРХНЯЯ граница). Полуинтервал (d−30, d] виден в выводе: суммы умышленно различны
// (16-е=100, 17-е=20, 15-е=3), поэтому скаляр однозначно кодирует мембершип.
//
// Окно (2026-05-16, 2026-06-15]: 05-16 ИСКЛЮЧЕНА (нижняя граница строгая), 05-17 ВКЛ,
// 06-15 ВКЛ → сумма = 20 + 3 = 23 (запись на 100 НЕ учтена → полуинтервал доказан).
//
// 🔁 ИНВЕРСИЯ: если нижнюю границу сделать включающей ([d−N, d]) → 100 попадёт →
// 123 ≠ 23 → красный; если N сдвинуть (29/31) → 05-16/05-17 пересекут границу →
// красный.
func TestWindowSlidingBoundary(t *testing.T) {
	i, _ := buildWindowBoundsInterp(t, windowBoundsSrc("последние 30дн"))
	v, err := i.evalMetricByName("m", ast.Position{Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("evalMetric вернул ошибку: %v", err)
	}
	if got := value.String(v); got != "23" {
		t.Errorf("оконный скаляр = %q, хотим %q (окно (2026-05-16, 2026-06-15]: 05-16 искл, 05-17+06-15 вкл = 20+3)", got, "23")
	}
}

// T021 (Phase F, US1, FR-011/SC-009/§MW-9 #7) — ТРИГГЕР ПО ОКОННОЙ МЕТРИКЕ A2-7 на
// FixedClock{2026,6,15}. `когда метрика <последние 30дн> < N` срабатывает на скаляр
// окна (23 < 100 → истина), тело печатает «значение» (= оконный скаляр 23). Прогон —
// interp.Run (глобалы/декларации) + RunTriggers (fire-if-true) на инжектированном
// windowClock; trigger_run.go НЕ изменён (тест лишь ВЫЗЫВАЕТ RunTriggers).
//
// 🔁 ИНВЕРСИЯ: если окно eval разойдётся с движком или порядок eval↔триггер сломается
// — вывод/exit разойдутся → красный.
func TestWindowMetricTriggerFires(t *testing.T) {
	src := windowBoundsSrc("последние 30дн") +
		"\nкогда метрика m < 100:\n    печать(\"оконная метрика:\", значение)\n"
	i, out := buildWindowBoundsInterp(t, src)

	// Тело триггера (печать) пишет в i.out (= out); RunTriggers(w) пишет в w лишь
	// заглушки событие/расписание — для метрика-триггера весь вывод в out.
	var stubs bytes.Buffer
	if err := i.RunTriggers(&stubs); err != nil {
		t.Fatalf("RunTriggers вернул ошибку: %v", err)
	}
	want := "оконная метрика: 23\n"
	if got := out.String(); got != want {
		t.Errorf("вывод триггера = %q, хотим %q", got, want)
	}
	if stubs.Len() != 0 {
		t.Errorf("непустые заглушки RunTriggers для метрика-триггера: %q", stubs.String())
	}
}

// T021 контр-демо — ЛОЖНОЕ условие (23 < 10) → триггер молчит, exit-эквивалент ok.
// Замок чувствительности: оконный скаляр действительно сравнивается (не «всегда
// срабатывает»).
func TestWindowMetricTriggerSilent(t *testing.T) {
	src := windowBoundsSrc("последние 30дн") +
		"\nкогда метрика m < 10:\n    печать(\"не должно печататься\")\n"
	i, out := buildWindowBoundsInterp(t, src)

	var stubs bytes.Buffer
	if err := i.RunTriggers(&stubs); err != nil {
		t.Fatalf("RunTriggers вернул ошибку: %v", err)
	}
	if got := out.String(); got != "" {
		t.Errorf("ложное условие должно молчать, получено %q", got)
	}
}

// T021 sanity — оконный скаляр НЕ зависит от пиннинга «сегодня()»: тот же windowClock
// инжектируется через harness, проверка что окно/мембершип не «течёт» в строку с
// .go:/goroutine при ошибке (защитный, на случай регресса диагностики окна).
func TestWindowMetricNoStackLeak(t *testing.T) {
	i, _ := buildWindowBoundsInterp(t, windowBoundsSrc("последние 30дн"))
	v, err := i.evalMetricByName("m", ast.Position{Line: 1, Col: 1})
	if err != nil {
		if strings.Contains(err.Error(), ".go:") || strings.Contains(err.Error(), "goroutine") {
			t.Errorf("Go stack trace в ошибке окна: %v", err)
		}
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if value.String(v) != "23" {
		t.Errorf("скаляр = %q, хотим 23", value.String(v))
	}
}
