package ladix

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/ir"
)

// validSource — минимальный валидный исходник: источник + метрика над ним.
// Путь к файлу источника при компиляции НЕ читается (исполнения нет).
const validSource = `источник заказы:
    файл: "data/sales.json"

метрика выручка:
    источник: заказы
    где:      статус == "оплачен"
    агрегат:  сумма(сумма_заказа)
`

// TestCompileValid — C1: валидный исходник → program со стабильной схемой,
// без диагностик уровня error и без err.
func TestCompileValid(t *testing.T) {
	program, diags, err := Compile(validSource)
	if err != nil {
		t.Fatalf("валидный исходник не должен давать err: %v", err)
	}
	if program == nil {
		t.Fatalf("валидный исходник обязан дать program; диагностики: %+v", diags)
	}
	if program.SchemaVersion != ir.SchemaVersion {
		t.Errorf("SchemaVersion = %d, ожидалась %d", program.SchemaVersion, ir.SchemaVersion)
	}
	if hasErrors(diags) {
		t.Errorf("инвариант нарушен: program != nil при наличии error-диагностик: %+v", diags)
	}
	if len(program.Metrics) != 1 || program.Metrics[0].Name != "выручка" {
		t.Errorf("метрика не понижена в IR: %+v", program.Metrics)
	}
}

// TestCompileInvalid — C2: пользовательская ошибка исходника едет в diags при
// err == nil и program == nil, на каждой из трёх стадий валидации.
func TestCompileInvalid(t *testing.T) {
	cases := []struct {
		name      string
		source    string
		wantStage string
	}{
		{
			name:      "лексика",
			source:    "пусть x = \"незакрытая строка\n",
			wantStage: ir.StageLexical,
		},
		{
			name:      "синтаксис",
			source:    "пусть = 5\n",
			wantStage: ir.StageSyntax,
		},
		{
			name:      "семантика",
			source:    "метрика м:\n    источник: нет_такого\n    агрегат:  сумма(поле)\n",
			wantStage: ir.StageSemantic,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			program, diags, err := Compile(c.source)
			if err != nil {
				t.Fatalf("ошибка исходника НЕ должна маскироваться под err: %v", err)
			}
			if program != nil {
				t.Fatalf("при ошибке исходника program обязана быть nil, получено %+v", program)
			}
			if len(diags) == 0 {
				t.Fatal("ожидалась хотя бы одна диагностика")
			}
			if !hasErrors(diags) {
				t.Errorf("ожидалась диагностика уровня error, получено: %+v", diags)
			}
			d := diags[0]
			if d.Stage != c.wantStage {
				t.Errorf("Stage = %q, ожидалась %q (диагностика: %+v)", d.Stage, c.wantStage, d)
			}
			if d.Severity != ir.SeverityError {
				t.Errorf("Severity = %q, ожидалась %q", d.Severity, ir.SeverityError)
			}
			if d.Pos.Line < 1 || d.Pos.Col < 1 {
				t.Errorf("позиция обязана быть 1-based и заполненной, получено %+v", d.Pos)
			}
			if strings.TrimSpace(d.Message) == "" {
				t.Error("Message обязано нести дословный текст диагностики, получена пустая строка")
			}
			// Message — ОПИСАНИЕ без двухстрочного заголовка §13: позиция едет
			// отдельным полем, дублировать её в тексте запрещено.
			if strings.Contains(d.Message, "Ошибка в строке") {
				t.Errorf("Message не должно содержать заголовок §13 (позиция — в Pos): %q", d.Message)
			}
		})
	}
}

// TestCompileFileEquivalence — C3': CompileFile эквивалентна Compile над
// содержимым файла (проверяется на реальном примере репозитория).
func TestCompileFileEquivalence(t *testing.T) {
	path := filepath.Join("examples", "метрики.ladix")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("не прочитать пример: %v", err)
	}

	fromFile, fileDiags, fileErr := CompileFile(path)
	fromText, textDiags, textErr := Compile(string(b))

	if fileErr != nil || textErr != nil {
		t.Fatalf("пример репозитория обязан компилироваться: file=%v text=%v", fileErr, textErr)
	}
	if fromFile == nil || fromText == nil {
		t.Fatalf("пример репозитория обязан дать program; диагностики: %+v / %+v", fileDiags, textDiags)
	}
	if len(fromFile.Metrics) != len(fromText.Metrics) ||
		len(fromFile.Processes) != len(fromText.Processes) ||
		len(fromFile.Triggers) != len(fromText.Triggers) {
		t.Errorf("CompileFile разошлась с Compile: %+v vs %+v", fromFile, fromText)
	}
}

// TestCompileFileMissing — C3: сбой чтения файла — ВНУТРЕННИЙ сбой (err),
// а не диагностика.
func TestCompileFileMissing(t *testing.T) {
	program, diags, err := CompileFile(filepath.Join(t.TempDir(), "нет-такого.ladix"))
	if err == nil {
		t.Fatal("ошибка чтения файла обязана возвращаться как err")
	}
	if program != nil {
		t.Errorf("при сбое чтения program обязана быть nil, получено %+v", program)
	}
	if len(diags) != 0 {
		t.Errorf("сбой чтения — не диагностика исходника, получено %+v", diags)
	}
}

// TestCompileAllRepositoryExamples — широкий регресс: каждый пример репозитория
// либо компилируется в program, либо даёт корректно оформленные диагностики.
// Ни один пример не имеет права уронить Compile внутренним сбоем (err != nil) —
// это и есть проверка recover-барьера и тотальности канонизаторов на реальном
// корпусе (в examples/ есть намеренно ошибочные программы).
func TestCompileAllRepositoryExamples(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("examples", "*.ladix"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("примеры не найдены: %v", err)
	}
	for _, p := range paths {
		t.Run(filepath.Base(p), func(t *testing.T) {
			program, diags, err := CompileFile(p)
			if err != nil {
				t.Fatalf("внутренний сбой на примере: %v", err)
			}
			if program == nil && !hasErrors(diags) {
				t.Errorf("program == nil обязана сопровождаться error-диагностикой, получено %+v", diags)
			}
			if program != nil && hasErrors(diags) {
				t.Errorf("инвариант нарушен: program != nil при error-диагностиках %+v", diags)
			}
		})
	}
}
