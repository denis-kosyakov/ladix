package metrics

// Задача 2.1 — «Исполнение метрики над данными потребителя» (design.md Д-1,
// spec.md Requirement «Исполнение метрики над данными потребителя»/«Форма
// данных потребителя»). Табличные тесты: одна метрика (ir.Metric с
// каноническими строками, как в ir/json_golden_test.go), разные наборы
// записей/опций, проверка Result.{Type,Text,Value} и отсутствия диагностик.
//
// Записи и суммы зеркалят golden-фикстуру §SM-10 docs/source-metric-model.md
// (data/sales.json: 1 200 000 оплачен 05-03; 800 000 оплачен 05-17; 500 000
// отменён 05-20; FixedClock = 2026-05-31), чтобы числа были узнаваемы и
// сверяемы вручную.

import (
	"reflect"
	"testing"

	"github.com/denis-kosyakov/ladix/ir"
)

// salesRecords — 3 записи, зеркало §SM-10.
func salesRecords() []map[string]any {
	return []map[string]any{
		{"ид": int64(1), "статус": "оплачен", "сумма_заказа": int64(1200000), "дата_заказа": "2026-05-03"},
		{"ид": int64(2), "статус": "оплачен", "сумма_заказа": int64(800000), "дата_заказа": "2026-05-17"},
		{"ид": int64(3), "статус": "отменён", "сумма_заказа": int64(500000), "дата_заказа": "2026-05-20"},
	}
}

func metricPos() ir.Position { return ir.Position{Line: 3, Col: 1} }

// baseMetric строит ir.Metric с заданными атрибутами; пустая строка атрибута
// означает «атрибут не задан» (Where/Period/ByDate опциональны в IR).
func baseMetric(where, aggregate, period, byDate string) ir.Metric {
	return ir.Metric{
		Name:      "м",
		Source:    "продажи",
		Where:     where,
		Aggregate: aggregate,
		Period:    period,
		ByDate:    byDate,
		Pos:       metricPos(),
	}
}

func maySchema() map[string]string {
	return map[string]string{"дата_заказа": "Дата"}
}

type evalCase struct {
	name     string
	metric   ir.Metric
	records  []map[string]any
	opts     Options
	wantType string
	wantText string
	// wantValue — nil означает "Пусто" (Value == nil); иное — сравнивается через
	// reflect.DeepEqual с фактическим Result.Value.
	wantValue any
}

func may31() Date { return Date{Year: 2026, Month: 5, Day: 31} }
func jun15() Date { return Date{Year: 2026, Month: 6, Day: 15} }

func evalCases() []evalCase {
	return []evalCase{
		{
			name:      "количество без фильтра",
			metric:    baseMetric("", "количество(ид)", "", ""),
			records:   salesRecords(),
			opts:      Options{Today: may31()},
			wantType:  "Целое",
			wantText:  "3",
			wantValue: int64(3),
		},
		{
			name:      "сумма с фильтром где",
			metric:    baseMetric(`(статус == "оплачен")`, "сумма(сумма_заказа)", "", ""),
			records:   salesRecords(),
			opts:      Options{Today: may31()},
			wantType:  "Целое",
			wantText:  "2000000",
			wantValue: int64(2000000),
		},
		{
			name:      "среднее с фильтром где",
			metric:    baseMetric(`(статус == "оплачен")`, "среднее(сумма_заказа)", "", ""),
			records:   salesRecords(),
			opts:      Options{Today: may31()},
			wantType:  "Дробное",
			wantText:  "1000000.0",
			wantValue: float64(1000000),
		},
		{
			name:      "мин без фильтра",
			metric:    baseMetric("", "мин(сумма_заказа)", "", ""),
			records:   salesRecords(),
			opts:      Options{Today: may31()},
			wantType:  "Целое",
			wantText:  "500000",
			wantValue: int64(500000),
		},
		{
			name:      "макс без фильтра",
			metric:    baseMetric("", "макс(сумма_заказа)", "", ""),
			records:   salesRecords(),
			opts:      Options{Today: may31()},
			wantType:  "Целое",
			wantText:  "1200000",
			wantValue: int64(1200000),
		},
		{
			name:      "окно периода: ежемесячно включает все майские записи",
			metric:    baseMetric("", "сумма(сумма_заказа)", "ежемесячно", "дата_заказа"),
			records:   salesRecords(),
			opts:      Options{Today: may31(), Fields: maySchema()},
			wantType:  "Целое",
			wantText:  "2500000",
			wantValue: int64(2500000),
		},
		{
			name:      "окно периода + где: июньское сегодня исключает майские",
			metric:    baseMetric(`(статус == "оплачен")`, "сумма(сумма_заказа)", "ежемесячно", "дата_заказа"),
			records:   salesRecords(),
			opts:      Options{Today: jun15(), Fields: maySchema()},
			wantType:  "Целое",
			wantText:  "0",
			wantValue: int64(0),
		},
		// Пустой результат (SPEC §10.5, docs/source-metric-model.md §SM-8 шаг 5):
		// нет выживших записей → единичные сумма/количество дают Целое 0,
		// единичные среднее/мин/макс и составные выражения — Пусто.
		{
			name:      "пустой результат: сумма → Целое 0",
			metric:    baseMetric(`(статус == "неизвестный")`, "сумма(сумма_заказа)", "", ""),
			records:   salesRecords(),
			opts:      Options{Today: may31()},
			wantType:  "Целое",
			wantText:  "0",
			wantValue: int64(0),
		},
		{
			name:      "пустой результат: количество → Целое 0",
			metric:    baseMetric(`(статус == "неизвестный")`, "количество(ид)", "", ""),
			records:   salesRecords(),
			opts:      Options{Today: may31()},
			wantType:  "Целое",
			wantText:  "0",
			wantValue: int64(0),
		},
		{
			name:      "пустой результат: среднее → Пусто",
			metric:    baseMetric(`(статус == "неизвестный")`, "среднее(сумма_заказа)", "", ""),
			records:   salesRecords(),
			opts:      Options{Today: may31()},
			wantType:  "Пусто",
			wantText:  "пусто",
			wantValue: nil,
		},
		{
			name:      "пустой результат: мин → Пусто",
			metric:    baseMetric(`(статус == "неизвестный")`, "мин(сумма_заказа)", "", ""),
			records:   salesRecords(),
			opts:      Options{Today: may31()},
			wantType:  "Пусто",
			wantText:  "пусто",
			wantValue: nil,
		},
		{
			name:      "пустой результат: макс → Пусто",
			metric:    baseMetric(`(статус == "неизвестный")`, "макс(сумма_заказа)", "", ""),
			records:   salesRecords(),
			opts:      Options{Today: may31()},
			wantType:  "Пусто",
			wantText:  "пусто",
			wantValue: nil,
		},
		{
			name:      "пустой результат: составное выражение (средний чек) → Пусто",
			metric:    baseMetric(`(статус == "неизвестный")`, "(сумма(сумма_заказа) / количество(ид))", "", ""),
			records:   salesRecords(),
			opts:      Options{Today: may31()},
			wantType:  "Пусто",
			wantText:  "пусто",
			wantValue: nil,
		},
	}
}

func TestEvaluateTable(t *testing.T) {
	for _, tc := range evalCases() {
		t.Run(tc.name, func(t *testing.T) {
			got, diags, err := Evaluate(tc.metric, tc.records, tc.opts)
			if err != nil {
				t.Fatalf("Evaluate() вернул ошибку: %v", err)
			}
			if len(diags) != 0 {
				t.Fatalf("Evaluate() вернул диагностики %+v, ожидалось ни одной", diags)
			}
			if got.Type != tc.wantType {
				t.Errorf("Type = %q, хотим %q", got.Type, tc.wantType)
			}
			if got.Text != tc.wantText {
				t.Errorf("Text = %q, хотим %q", got.Text, tc.wantText)
			}
			if !reflect.DeepEqual(got.Value, tc.wantValue) {
				t.Errorf("Value = %#v (%T), хотим %#v (%T)", got.Value, got.Value, tc.wantValue, tc.wantValue)
			}
		})
	}
}
