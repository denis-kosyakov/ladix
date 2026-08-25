package metrics

// Задача 4.4 — Корпус golden result-фикстур (spec.md Requirement
// «Семантическая стабильность результатов»: (программа + вход + фиксированные
// часы) → результат; MINOR/PATCH не меняют результаты СУЩЕСТВУЮЩИХ программ).
//
// Формат: testdata/<имя>.json — вход (ir.Metric-поля + records + Options),
// testdata/<имя>.golden.json — зафиксированный результат (Result.{Type,Text,
// Value} + диагностики + программный маркер ошибки). Сверка — БАЙТ-В-БАЙТ
// (bytes.Equal над сериализованным актуальным результатом и содержимым
// golden-файла), поэтому дрейф форматирования golden тоже красит тест —
// обновление обязано быть осознанным действием (флаг -update), не побочным
// эффектом правки продакшн-кода.
//
// Числа в records сериализуются через json.Decoder.UseNumber(), поэтому
// типизация идёт СТРОГО по форме токена (design.md Д-8: "1200000" → Целое,
// "1000000.0" дало бы Дробное) — то же правило, что у CLI-загрузчика
// источника.

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/denis-kosyakov/ladix/ir"
)

var updateGolden = flag.Bool("update", false, "перезаписать golden-файлы metrics/testdata под текущий вывод Evaluate")

// fixtureToday — {year,month,day} входной фикстуры; отдельный тип от Date,
// чтобы формат JSON-фикстуры был явным и не зависел от полей Date молча.
type fixtureToday struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

// fixtureInput — вход одной golden-фикстуры (testdata/<имя>.json).
type fixtureInput struct {
	Metric  ir.Metric         `json:"metric"`
	Records json.RawMessage   `json:"records"`
	Today   fixtureToday      `json:"today"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// goldenResult — зеркало Result для JSON (Result.Value — any, сериализуется
// как есть).
type goldenResult struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Value any    `json:"value"`
}

// fixtureGolden — зафиксированный вывод Evaluate (testdata/<имя>.golden.json).
// Err — программный маркер сентинела (НЕ текст error.Error(), тот не
// детерминирован по составу): "" (err == nil), "ErrInvalidOptions",
// "ErrEvaluation", "ErrInternal", либо "other" (защитный случай — не должен
// встречаться ни в одной фикстуре этого корпуса).
type fixtureGolden struct {
	Result      *goldenResult   `json:"result"`
	Diagnostics []ir.Diagnostic `json:"diagnostics"`
	Err         string          `json:"err"`
}

// errSentinelName сопоставляет err программному маркеру errors.Is (design.md
// Д-11 «Контракт возвратов»): маркер стабилен между запусками, в отличие от
// текста fmt.Errorf, который может нести обёрнутый recover()-текст (ErrInternal).
func errSentinelName(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrInvalidOptions):
		return "ErrInvalidOptions"
	case errors.Is(err, ErrEvaluation):
		return "ErrEvaluation"
	case errors.Is(err, ErrInternal):
		return "ErrInternal"
	default:
		return "other"
	}
}

// listFixtures возвращает базовые имена (без .json) всех входных фикстур в
// testdata/, отсортированные — детерминированный порядок сабтестов.
func listFixtures(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("os.ReadDir(\"testdata\"): %v", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if filepath.Ext(n) == ".json" && !hasGoldenSuffix(n) {
			names = append(names, n[:len(n)-len(".json")])
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatalf("testdata/ не содержит ни одной *.json фикстуры — корпус пуст")
	}
	return names
}

// hasGoldenSuffix отличает вход ("<имя>.json") от golden ("<имя>.golden.json")
// при обходе директории.
func hasGoldenSuffix(name string) bool {
	const suf = ".golden.json"
	return len(name) > len(suf) && name[len(name)-len(suf):] == suf
}

// TestGoldenCorpus прогоняет каждую фикстуру testdata/<имя>.json через
// Evaluate и сверяет байт-в-байт с testdata/<имя>.golden.json. С флагом
// -update перезаписывает golden под текущий вывод вместо сравнения (осознанное
// обновление корпуса, не значение по умолчанию).
func TestGoldenCorpus(t *testing.T) {
	for _, name := range listFixtures(t) {
		name := name
		t.Run(name, func(t *testing.T) {
			inPath := filepath.Join("testdata", name+".json")
			goldenPath := filepath.Join("testdata", name+".golden.json")

			raw, err := os.ReadFile(inPath)
			if err != nil {
				t.Fatalf("os.ReadFile(%q): %v", inPath, err)
			}
			var fx fixtureInput
			if err := json.Unmarshal(raw, &fx); err != nil {
				t.Fatalf("разбор фикстуры %q: %v", inPath, err)
			}

			var records []map[string]any
			if len(fx.Records) > 0 {
				dec := json.NewDecoder(bytes.NewReader(fx.Records))
				dec.UseNumber()
				if err := dec.Decode(&records); err != nil {
					t.Fatalf("разбор records фикстуры %q: %v", inPath, err)
				}
			}

			opts := Options{
				Today:  Date{Year: fx.Today.Year, Month: fx.Today.Month, Day: fx.Today.Day},
				Fields: fx.Fields,
			}

			res, diags, err := Evaluate(fx.Metric, records, opts)

			var out fixtureGolden
			if err == nil {
				out.Result = &goldenResult{Type: res.Type, Text: res.Text, Value: res.Value}
			}
			out.Diagnostics = diags
			if out.Diagnostics == nil {
				out.Diagnostics = []ir.Diagnostic{}
			}
			out.Err = errSentinelName(err)
			if out.Err == "other" {
				t.Fatalf("Evaluate вернул err вне трёх сентинелов ErrInvalidOptions/ErrEvaluation/ErrInternal: %v", err)
			}

			gotBytes, merr := json.MarshalIndent(out, "", "  ")
			if merr != nil {
				t.Fatalf("сериализация фактического результата: %v", merr)
			}
			gotBytes = append(gotBytes, '\n')

			if *updateGolden {
				if err := os.WriteFile(goldenPath, gotBytes, 0o644); err != nil {
					t.Fatalf("запись golden %q: %v", goldenPath, err)
				}
				return
			}

			wantBytes, rerr := os.ReadFile(goldenPath)
			if rerr != nil {
				t.Fatalf("os.ReadFile(%q): %v (запустите go test -run TestGoldenCorpus -update ./metrics/... для генерации)", goldenPath, rerr)
			}
			if !bytes.Equal(gotBytes, wantBytes) {
				t.Errorf("golden-результат разошёлся с фактическим для %q:\n--- golden (%s) ---\n%s\n--- фактический ---\n%s",
					name, goldenPath, wantBytes, gotBytes)
			}
		})
	}
}
