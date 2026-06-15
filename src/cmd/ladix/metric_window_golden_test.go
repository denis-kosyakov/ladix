package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// fixedClock20260615 — детерминированный Clock для оконных метрик A2 (§MW-9). ДАТА-НЕ-
// КОНЕЦ-МЕСЯЦА (2026-06-15), отдельный от fixedClock2026 ({2026,5,31}): «последние 30дн»
// нужен не-конец-месяца, чтобы окно (2026-05-16, 2026-06-15] было интуитивным
// (§MW-D-WINDOW-EDGE). Пиннинг «сегодня()» ЗАПРЕЩЁН — детерминизм через инъекцию Clock.
var fixedClock20260615 = eval.FixedClock{D: value.Дата{Year: 2026, Month: 6, Day: 15}}

// T026 (Phase F, US1, SC-004/FR-019/§MW-9 #9) — GOLDEN DoD-СРЕЗА M1. Прогон САМОГО
// examples/выручка_30д.ladix через runMetric (инжектированный FixedClock{2026,6,15})
// для резолва data/orders.csv (через withRepoRoot, cwd=корень репо). Метрика
// «последние 30дн» + по_дате: дата_заказа на CSV-источнике A1.
//
// Окно (2026-05-16, 2026-06-15] исключает 2026-05-04 (1.2M) и 2026-05-12 (0.8M),
// оставляет 2026-05-27 (оплачен, 300000); 2026-05-19 (отменён) отфильтрован `где` →
// DoD-скаляр 300000.0 (демонстрирует фильтрацию ОКНОМ vs полная сумма 2300000.5,
// которую пинит A1-golden TestCLIGoldenSourceCSV). data/orders.csv НЕ модифицируется.
//
// 🔁 ИНВЕРСИЯ: CSV→окно→метрика разошлась / граница окна сдвинулась / Clock сменился —
// stdout разойдётся с пином → красный.
func TestCLIMetricWindowDoDGolden(t *testing.T) {
	withRepoRoot(t, func() {
		example := filepath.Join("examples", "выручка_30д.ladix")
		var out, errBuf bytes.Buffer
		code := runMetric(example, "выручка_30д", eval.DefaultMaxDepth, fixedClock20260615, &out, &errBuf)
		if code != 0 {
			t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
		}
		if errBuf.Len() != 0 {
			t.Errorf("непустой stderr: %q", errBuf.String())
		}
		if out.String() != "300000.0\n" {
			t.Errorf("DoD-скаляр байт-не-точен (FixedClock 2026-06-15): получено %q, хотим %q", out.String(), "300000.0\n")
		}
	})
}

// T020 (Phase F, US3, FR-013/SC-005/§MW-9 #8) — РЕГРЕСС ОБР. СОВМЕСТИМОСТИ. Прогон
// САМОГО examples/выручка.ladix (период: ежемесячно + по_дате) через runMetric на
// fixedClock2026 ({2026,5,31}) → скаляр БЕЗ изменения относительно v1 (2000000),
// 5 календарных адвербов не тронуты добавлением скользящих/завершённых форм. Этот
// замок дублирует инвариант trigger_golden TestRevenueExampleFixedClockGolden НА
// CMD-уровне как явный backward-compat-лок A2: если оконная фича сдвинула путь
// «ежемесячно» — здесь красный.
//
// 🔁 ИНВЕРСИЯ: если ветка period «ежемесячно» (Name-only, Amount/Unit/Offset=0)
// изменилась — скаляр разойдётся → красный.
func TestCLIMetricCalendarPeriodBackwardCompat(t *testing.T) {
	withRepoRoot(t, func() {
		example := filepath.Join("examples", "выручка.ladix")
		var out, errBuf bytes.Buffer
		code := runMetric(example, "выручка_месяца", eval.DefaultMaxDepth, fixedClock2026, &out, &errBuf)
		if code != 0 {
			t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
		}
		if out.String() != "2000000\n" {
			t.Errorf("v1-регресс (ежемесячно, FixedClock 2026-05-31): получено %q, хотим %q", out.String(), "2000000\n")
		}
	})
}

// T028 (Phase F, US3, SC-006/§MW-9 #5/§MW-8) — NEGATIVE-ЗАМКИ ВИТРИНЫ A2 через
// assertNegativeExample: каждый exit 1 + stderr БАЙТ-В-БАЙТ §13-канон §MW-8 + пустой
// stdout + НЕТ Go stack trace. Тексты пиннены из ФАКТИЧЕСКОГО прогона бинаря (T025).
//
// 🔁 ИНВЕРСИЯ: если пример перестал падать ИМЕННО этой §MW-8-ошибкой (unit-guard снят /
// N≥1-guard снят / noun-set расширён / сместилась строка/колонка) — красный.

// §MW-SEM-1 — недопустимая единица окна: «последние 5час» (час ∉ {дн,нед,мес}).
func TestCLINegativeWindowUnit(t *testing.T) {
	assertNegativeExample(t, "окно_единица.ladix",
		"Ошибка в строке 16, колонка 5:\n"+
			"метрика 'выручка_окно': единица 'час' недопустима для окна (допустимо: дн, нед, мес)\n")
}

// §MW-SEM-2 — неположительный размер окна: «последние 0дн» (N должен быть ≥ 1).
func TestCLINegativeWindowSize(t *testing.T) {
	assertNegativeExample(t, "окно_размер.ladix",
		"Ошибка в строке 16, колонка 5:\n"+
			"метрика 'выручка_окно': размер окна должен быть положительным\n")
}

// §MW-SEM-3 — неизвестный noun завершённого периода: «прошлый век».
func TestCLINegativeWindowNoun(t *testing.T) {
	assertNegativeExample(t, "окно_noun.ladix",
		"Ошибка в строке 17, колонка 5:\n"+
			"метрика 'выручка_окно': неизвестный период 'век' (допустимо: день, неделя, месяц, квартал, год)\n")
}

// §MW-SEM-2 (Defect 4) — переполнение размера окна: «последние
// 99999999999999999999999дн». N положителен, но вне int64 → ОТДЕЛЬНОЕ «размер окна
// слишком велик» (НЕ «должен быть положительным», т.к. значение положительно, лишь
// вне диапазона). 🔁 если ParseInt-err и N<1 снова слить в одну ветку — красный.
func TestCLINegativeWindowOverflow(t *testing.T) {
	assertNegativeExample(t, "окно_переполнение.ladix",
		"Ошибка в строке 17, колонка 5:\n"+
			"метрика 'выручка_окно': размер окна слишком велик\n")
}
