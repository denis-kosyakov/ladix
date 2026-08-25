package metrics

// Задача 2.2 — «Совпадение с reference-CLI» (design.md Д-1/Д-6, spec.md
// Requirement «Исполнение метрики над данными потребителя», Scenario
// «Совпадение с reference-CLI»).
//
// ВАЖНО: путь (б) ниже — не имитация, а РЕАЛЬНЫЙ код `ladix metric <файл>
// <имя>`: eval.NewInterpreter → SetSourceBase → Analyze(prog) →
// EvalMetricByName, тот же internal/eval, который использует CLI (см.
// internal/eval/metric.go: EvalMetricByName — «публичная точка входа CLI
// «ladix metric»»). Тест сравнивает результат этого пути с результатом
// публичного metrics.Evaluate над идентичным набором записей.

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	lerrors "github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/value"
	"github.com/denis-kosyakov/ladix/ir"
)

type parityCase struct {
	name       string
	ladix      string // .ladix-текст; источник ссылается на "records.json" в том же каталоге
	metricName string
	records    []map[string]any // вход пути (а) — тот же набор идёт в records.json для пути (б)
	irMetric   ir.Metric
	fields     map[string]string
	today      Date
	clock      eval.FixedClock
}

func parityCases() []parityCase {
	records := salesRecords()
	return []parityCase{
		{
			name: "с фильтром где",
			ladix: "источник продажи:\n" +
				"    файл: \"records.json\"\n\n" +
				"метрика выручка:\n" +
				"    источник: продажи\n" +
				"    где:      (статус == \"оплачен\")\n" +
				"    агрегат:  сумма(сумма_заказа)\n",
			metricName: "выручка",
			records:    records,
			irMetric: ir.Metric{
				Name: "выручка", Source: "продажи",
				Where: `(статус == "оплачен")`, Aggregate: "сумма(сумма_заказа)",
				Pos: metricPos(),
			},
			today: may31(),
			clock: eval.FixedClock{D: value.Дата{Year: 2026, Month: 5, Day: 31}},
		},
		{
			name: "с окном периода",
			ladix: "источник продажи:\n" +
				"    файл: \"records.json\"\n" +
				"    поля:\n" +
				"        дата_заказа: Дата\n\n" +
				"метрика выручка_месяца:\n" +
				"    источник: продажи\n" +
				"    агрегат:  сумма(сумма_заказа)\n" +
				"    период:   ежемесячно\n" +
				"    по_дате:  дата_заказа\n",
			metricName: "выручка_месяца",
			records:    records,
			irMetric: ir.Metric{
				Name: "выручка_месяца", Source: "продажи",
				Aggregate: "сумма(сумма_заказа)", Period: "ежемесячно", ByDate: "дата_заказа",
				Pos: metricPos(),
			},
			fields: maySchema(),
			today:  may31(),
			clock:  eval.FixedClock{D: value.Дата{Year: 2026, Month: 5, Day: 31}},
		},
		{
			name: "пустой результат (фильтр исключает всё)",
			ladix: "источник продажи:\n" +
				"    файл: \"records.json\"\n\n" +
				"метрика средний_чек:\n" +
				"    источник: продажи\n" +
				"    где:      (статус == \"неизвестный\")\n" +
				"    агрегат:  среднее(сумма_заказа)\n",
			metricName: "средний_чек",
			records:    records,
			irMetric: ir.Metric{
				Name: "средний_чек", Source: "продажи",
				Where: `(статус == "неизвестный")`, Aggregate: "среднее(сумма_заказа)",
				Pos: metricPos(),
			},
			today: may31(),
			clock: eval.FixedClock{D: value.Дата{Year: 2026, Month: 5, Day: 31}},
		},
	}
}

func TestParityWithReferenceCLI(t *testing.T) {
	for _, tc := range parityCases() {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			recJSON, err := json.Marshal(tc.records)
			if err != nil {
				t.Fatalf("marshal records: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "records.json"), recJSON, 0o644); err != nil {
				t.Fatalf("write records.json: %v", err)
			}

			// Путь (б): РЕАЛЬНЫЙ код CLI `ladix metric`.
			toks, errList := lexer.New(tc.ladix).Tokenize()
			prog := parser.New(toks, errList).Parse()
			if !errList.Empty() {
				t.Fatalf("лексер/парсер вернули ошибки: %v", errList.Error())
			}
			interp := eval.NewInterpreter(io.Discard, 0, tc.clock)
			interp.SetSourceBase(dir)
			if err := interp.Analyze(prog); err != nil {
				t.Fatalf("Analyze: %v", err)
			}
			cliValue, err := interp.EvalMetricByName(tc.metricName)
			if err != nil {
				t.Fatalf("CLI-путь EvalMetricByName(%q): %v", tc.metricName, err)
			}
			wantText := value.String(cliValue)
			wantType := cliValue.TypeName()

			// Путь (а): публичный фасад.
			got, diags, err := Evaluate(tc.irMetric, tc.records, Options{Today: tc.today, Fields: tc.fields})
			if err != nil {
				t.Fatalf("публичный путь Evaluate: %v", err)
			}
			if len(diags) != 0 {
				t.Fatalf("публичный путь вернул диагностики %+v, ожидалось ни одной", diags)
			}

			if got.Type != wantType {
				t.Errorf("Type = %q, CLI-путь дал %q", got.Type, wantType)
			}
			if got.Text != wantText {
				t.Errorf("Text = %q, CLI-путь дал %q", got.Text, wantText)
			}
		})
	}
}

// --- Парити ПРИВЕДЕНИЯ ПО СХЕМЕ (Д-7) ------------------------------------
//
// Замок на переиспользование ОДНОГО пути коэрсии: metrics.Evaluate обязан
// приводить поле по Options.Fields тем же eval.applySchema/coerceField, что и
// загрузчик источника CLI. Тест краснеет, если в metrics снова заведут копию
// логики или текстов: сравниваются и УСПЕШНЫЙ результат, и ДОСЛОВНЫЙ текст
// ошибки приведения на невалидной дате.

// runCLIMetric прогоняет РЕАЛЬНЫЙ путь `ladix metric <файл> <имя>` над записями
// recs, записанными в records.json каталога dir.
func runCLIMetric(t *testing.T, src, metricName string, recs []map[string]any, clock eval.FixedClock) (value.Value, error) {
	t.Helper()
	dir := t.TempDir()
	recJSON, err := json.Marshal(recs)
	if err != nil {
		t.Fatalf("marshal records: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "records.json"), recJSON, 0o644); err != nil {
		t.Fatalf("write records.json: %v", err)
	}
	toks, errList := lexer.New(src).Tokenize()
	prog := parser.New(toks, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("лексер/парсер вернули ошибки: %v", errList.Error())
	}
	interp := eval.NewInterpreter(io.Discard, 0, clock)
	interp.SetSourceBase(dir)
	if err := interp.Analyze(prog); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	return interp.EvalMetricByName(metricName)
}

// runtimeMsg достаёт поле Msg ОшибкиВыполнения — тот же текст, который CLI
// печатает второй строкой канона диагностики.
func runtimeMsg(t *testing.T, err error) string {
	t.Helper()
	var re lerrors.ОшибкаВыполнения
	if !errors.As(err, &re) {
		t.Fatalf("ожидалась ОшибкаВыполнения, получено %T: %v", err, err)
	}
	return re.Msg
}

const schemaParityLadix = "источник продажи:\n" +
	"    файл: \"records.json\"\n" +
	"    поля:\n" +
	"        дата_заказа: Дата\n\n" +
	"метрика последняя_дата:\n" +
	"    источник: продажи\n" +
	"    агрегат:  макс(дата_заказа)\n"

func schemaParityMetric() ir.Metric {
	return ir.Metric{
		Name: "последняя_дата", Source: "продажи",
		Aggregate: "макс(дата_заказа)", Pos: metricPos(),
	}
}

// TestParitySchemaCoercionSuccess: поле-дата, приведённое по объявленной схеме,
// даёт ОДИН И ТОТ ЖЕ типизированный результат на публичном пути и на пути CLI.
func TestParitySchemaCoercionSuccess(t *testing.T) {
	recs := salesRecords()
	clock := eval.FixedClock{D: value.Дата{Year: 2026, Month: 5, Day: 31}}

	cliValue, err := runCLIMetric(t, schemaParityLadix, "последняя_дата", recs, clock)
	if err != nil {
		t.Fatalf("CLI-путь: %v", err)
	}
	if cliValue.TypeName() != "Дата" {
		t.Fatalf("CLI-путь дал %s, ожидалась Дата — схема не применилась, тест бессмыслен", cliValue.TypeName())
	}

	got, diags, err := Evaluate(schemaParityMetric(), recs, Options{Today: may31(), Fields: maySchema()})
	if err != nil {
		t.Fatalf("публичный путь: %v (диагностики %+v)", err, diags)
	}
	if len(diags) != 0 {
		t.Fatalf("публичный путь вернул диагностики %+v", diags)
	}
	if got.Type != cliValue.TypeName() {
		t.Errorf("Type = %q, CLI-путь дал %q", got.Type, cliValue.TypeName())
	}
	if got.Text != value.String(cliValue) {
		t.Errorf("Text = %q, CLI-путь дал %q", got.Text, value.String(cliValue))
	}
}

// TestParitySchemaCoercionErrorText: НЕВАЛИДНАЯ дата даёт ДОСЛОВНО одинаковый
// текст ошибки приведения на обоих путях (§SC-9.B). Замок на Д-7/Принцип VIII.
func TestParitySchemaCoercionErrorText(t *testing.T) {
	badRecords := []struct {
		name string
		recs []map[string]any
	}{
		{
			name: "невалидная дата в строке",
			recs: []map[string]any{
				{"ид": int64(1), "статус": "оплачен", "сумма_заказа": int64(1), "дата_заказа": "2026-02-30"},
			},
		},
		{
			name: "дата не в формате ГГГГ-ММ-ДД",
			recs: []map[string]any{
				{"ид": int64(1), "статус": "оплачен", "сумма_заказа": int64(1), "дата_заказа": "31.05.2026"},
			},
		},
		{
			name: "поле-дата не Строка",
			recs: []map[string]any{
				{"ид": int64(1), "статус": "оплачен", "сумма_заказа": int64(1), "дата_заказа": int64(20260531)},
			},
		},
		{
			name: "объявленное поле отсутствует",
			recs: []map[string]any{
				{"ид": int64(1), "статус": "оплачен", "сумма_заказа": int64(1)},
			},
		},
		{
			name: "невалидная дата во ВТОРОЙ записи (нумерация с 1)",
			recs: []map[string]any{
				{"ид": int64(1), "статус": "оплачен", "сумма_заказа": int64(1), "дата_заказа": "2026-05-03"},
				{"ид": int64(2), "статус": "оплачен", "сумма_заказа": int64(2), "дата_заказа": "2026-13-01"},
			},
		},
	}
	clock := eval.FixedClock{D: value.Дата{Year: 2026, Month: 5, Day: 31}}

	for _, tc := range badRecords {
		t.Run(tc.name, func(t *testing.T) {
			_, cliErr := runCLIMetric(t, schemaParityLadix, "последняя_дата", tc.recs, clock)
			if cliErr == nil {
				t.Fatalf("CLI-путь не дал ошибки — тест бессмыслен")
			}
			want := runtimeMsg(t, cliErr)

			_, diags, err := Evaluate(schemaParityMetric(), tc.recs, Options{Today: may31(), Fields: maySchema()})
			if err == nil {
				t.Fatalf("публичный путь не дал ошибки, CLI дал %q", want)
			}
			if !errors.Is(err, ErrEvaluation) {
				t.Fatalf("err = %v, ожидалась ErrEvaluation", err)
			}
			if len(diags) != 1 {
				t.Fatalf("диагностик %d, ожидалась 1: %+v", len(diags), diags)
			}
			if diags[0].Message != want {
				t.Errorf("текст публичного пути:\n  %q\nтекст CLI-пути:\n  %q", diags[0].Message, want)
			}
			if diags[0].Severity != ir.SeverityError || diags[0].Stage != stageRuntime {
				t.Errorf("severity/stage = %q/%q", diags[0].Severity, diags[0].Stage)
			}
			if diags[0].Pos.Line < 1 || diags[0].Pos.Col < 1 {
				t.Errorf("позиция %+v нарушает Принцип IV (координаты с 1)", diags[0].Pos)
			}
		})
	}
}

// TestParityIntOutOfRangeText: существующий текст реестра §SM-9.B «целое число
// вне диапазона» печатается публичным путём ДОСЛОВНО так же, как загрузчиком
// источника CLI (разделитель «запись N, поле», а не «запись N: поле»).
func TestParityIntOutOfRangeText(t *testing.T) {
	const big = "99999999999999999999" // вне int64
	dir := t.TempDir()
	raw := `[{"ид": 1, "сумма_заказа": ` + big + `}]`
	if err := os.WriteFile(filepath.Join(dir, "records.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write records.json: %v", err)
	}
	src := "источник продажи:\n" +
		"    файл: \"records.json\"\n\n" +
		"метрика итог:\n" +
		"    источник: продажи\n" +
		"    агрегат:  сумма(сумма_заказа)\n"
	toks, errList := lexer.New(src).Tokenize()
	prog := parser.New(toks, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("лексер/парсер: %v", errList.Error())
	}
	interp := eval.NewInterpreter(io.Discard, 0, eval.FixedClock{D: value.Дата{Year: 2026, Month: 5, Day: 31}})
	interp.SetSourceBase(dir)
	if err := interp.Analyze(prog); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	_, cliErr := interp.EvalMetricByName("итог")
	if cliErr == nil {
		t.Fatalf("CLI-путь не дал ошибки — тест бессмыслен")
	}
	want := runtimeMsg(t, cliErr)

	m := ir.Metric{Name: "итог", Source: "продажи", Aggregate: "сумма(сумма_заказа)", Pos: metricPos()}
	recs := []map[string]any{{"ид": int64(1), "сумма_заказа": json.Number(big)}}
	_, diags, err := Evaluate(m, recs, Options{Today: may31()})
	if err == nil {
		t.Fatalf("публичный путь не дал ошибки, CLI дал %q", want)
	}
	if len(diags) != 1 {
		t.Fatalf("диагностик %d, ожидалась 1: %+v", len(diags), diags)
	}
	if diags[0].Message != want {
		t.Errorf("текст публичного пути:\n  %q\nтекст CLI-пути:\n  %q", diags[0].Message, want)
	}
}
