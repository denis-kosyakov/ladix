package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// repoRoot — корень репозитория (на 3 уровня выше каталога пакета cmd/ladix).
// Используется smoke-тестами (docs_alignment/reproducibility/quickstart) для чтения
// файлов из корня репо; metric-тесты на него больше НЕ опираются (фича 026: пути
// источников резолвятся от каталога .ladix-файла, см. absExample/examplesDir).
func repoRoot() string { return filepath.Join("..", "..") }

// metricFixture — путь к метрик-онли фикстуре §SM-10 (internal/eval/testdata),
// относительно каталога пакета cmd/ladix (на 3 уровня выше — корень репо).
func metricFixture() string {
	return filepath.Join("..", "..", "internal", "eval", "testdata", "metric_only.ladix")
}

// fixedClock2026 — детерминированный Clock golden-приёмки §SM-10 (D=2026-05-31).
var fixedClock2026 = eval.FixedClock{D: value.Дата{Year: 2026, Month: 5, Day: 31}}

// absExample — абсолютный путь к примеру examples/<name> (фича 026: пути источников
// резолвятся от каталога .ladix-файла, не от cwd). Прогон через realMain/runMetric с
// абсолютным путём примера резолвит относительный «data/sales.json» источника от
// каталога примера (examples/), без os.Chdir в корень репо.
func absExample(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(examplePath(name))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// examplesDir — абсолютный путь к каталогу examples/ (где лежит data/sales.json после
// переезда, фича 026). Используется как явная база источников (--source-base) для
// фикстур §SM-10, чей файл лежит вне examples/, но ссылается на «data/sales.json».
func examplesDir(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "examples"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// T029: успешный путь — ladix metric <фикстура> выручка_месяца → «2000000» / код 0
// (§SM-10, FixedClock 2026-05-31). Через runMetric с инжектированным FixedClock
// (прод-путь metricMain строит SystemClock — дата-зависим, CM-6).
func TestMetricSuccess(t *testing.T) {
	var out, errBuf bytes.Buffer
	// --source-base = examples/ (фича 026): фикстура §SM-10 ссылается на «data/sales.json»,
	// который после переезда лежит в examples/data/. База резолвит относительный путь.
	code := runMetric(metricFixture(), "выручка_месяца", eval.DefaultMaxDepth, examplesDir(t), fixedClock2026, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if out.String() != "2000000\n" {
		t.Errorf("stdout = %q, хотим %q", out.String(), "2000000\n")
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
}

// T030: неизвестная метрика → stderr «неизвестная метрика '<имя>'» / код 1 (§SM-9.D).
func TestMetricUnknownName(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := runMetric(metricFixture(), "нет_такой", eval.DefaultMaxDepth, examplesDir(t), fixedClock2026, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1", code)
	}
	if out.Len() != 0 {
		t.Errorf("непустой stdout: %q", out.String())
	}
	if !strings.Contains(errBuf.String(), "неизвестная метрика 'нет_такой'") {
		t.Errorf("stderr = %q не содержит §SM-9.D «неизвестная метрика»", errBuf.String())
	}
	if strings.Contains(errBuf.String(), ".go:") || strings.Contains(errBuf.String(), "goroutine") {
		t.Errorf("в stderr просочился Go stack trace: %q", errBuf.String())
	}
}

// T030: имя источника (не метрика) → stderr «'<имя>' — не метрика» / код 1 (§SM-9.D).
func TestMetricNotAMetric(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := runMetric(metricFixture(), "продажи", eval.DefaultMaxDepth, examplesDir(t), fixedClock2026, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1", code)
	}
	if out.Len() != 0 {
		t.Errorf("непустой stdout: %q", out.String())
	}
	if !strings.Contains(errBuf.String(), "'продажи' — не метрика") {
		t.Errorf("stderr = %q не содержит §SM-9.D «не метрика»", errBuf.String())
	}
}

// T030: предопределённый период (не метрика) → stderr «'<имя>' — не метрика» / код 1.
func TestMetricPeriodNotAMetric(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := runMetric(metricFixture(), "ежемесячно", eval.DefaultMaxDepth, examplesDir(t), fixedClock2026, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1", code)
	}
	if !strings.Contains(errBuf.String(), "'ежемесячно' — не метрика") {
		t.Errorf("stderr = %q не содержит §SM-9.D «не метрика»", errBuf.String())
	}
}

// T029/T030: usage-коды подкоманды metric (код 2) — нет/мало/лишних позиционных,
// нечитаемый файл, неверный --max-depth, неизвестный флаг (через realMain-диспетчер).
func TestMetricUsageErrors(t *testing.T) {
	fx := metricFixture()
	cases := [][]string{
		{"metric"},     // нет позиционных
		{"metric", fx}, // один позиционный
		{"metric", fx, "выручка_месяца", "лишнее"},           // лишний позиционный
		{"metric", "нет-такого-файла.ladix", "м"},            // файл не читается
		{"metric", fx, "выручка_месяца", "--max-depth"},      // флаг без значения
		{"metric", fx, "выручка_месяца", "--max-depth", "0"}, // неверное значение
		{"metric", fx, "выручка_месяца", "--нечто"},          // неизвестный флаг
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := realMain(args, &out, &errBuf)
			if code != 2 {
				t.Errorf("args=%v: код = %d, хотим 2; stderr=%q", args, code, errBuf.String())
			}
		})
	}
}

// T029: успешный прогон через диспетчер realMain (--max-depth=N форма) → код 0.
// Проверяет, что ветвь metric корректно достижима через realMain и --max-depth
// парсится. Clock здесь — SystemClock (прод-путь), поэтому окно «без периода»:
// фикстуру без период:/по_дате не используем — берём метрику без периода через
// инлайн-файл во временном каталоге, чтобы результат не зависел от даты.
func TestMetricDispatchNoPeriod(t *testing.T) {
	dir := t.TempDir()
	dataFile := filepath.Join(dir, "data.json")
	if err := os.WriteFile(dataFile, []byte(`[{"x": 10}, {"x": 20}, {"x": 30}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	srcFile := filepath.Join(dir, "m.ladix")
	src := "источник s:\n    файл: \"" + dataFile + "\"\n\n" +
		"метрика сумма_x:\n    источник: s\n    агрегат: сумма(x)\n"
	if err := os.WriteFile(srcFile, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := realMain([]string{"metric", "--max-depth=5000", srcFile, "сумма_x"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if out.String() != "60\n" {
		t.Errorf("stdout = %q, хотим %q", out.String(), "60\n")
	}
}

// T030: ошибка загрузки источника (§SM-9.B) в ветви metric → stderr / код 1.
func TestMetricLoadError(t *testing.T) {
	dir := t.TempDir()
	srcFile := filepath.Join(dir, "m.ladix")
	src := "источник s:\n    файл: \"нет.json\"\n\n" +
		"метрика m:\n    источник: s\n    агрегат: сумма(x)\n"
	if err := os.WriteFile(srcFile, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := realMain([]string{"metric", srcFile, "m"}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1; stderr=%q", code, errBuf.String())
	}
	// Фича 026: путь источника резолвится от каталога .ladix-файла (sourceBaseDir),
	// т.е. от dir. Текст «не найден» несёт ИТОГОВЫЙ резолвленный путь (resolveSourcePath
	// → os.Open → текст ошибки), а не сырой «нет.json».
	wantPath := filepath.Join(dir, "нет.json")
	wantErr := "источник 's': файл «" + wantPath + "» не найден"
	if !strings.Contains(errBuf.String(), wantErr) {
		t.Errorf("stderr = %q не содержит §SM-9.B «файл не найден» (%q)", errBuf.String(), wantErr)
	}
}

// T030: семантическая ошибка декларации (§SM-9.A) в ветви metric → stderr / код 1.
func TestMetricAnalyzeError(t *testing.T) {
	dir := t.TempDir()
	srcFile := filepath.Join(dir, "m.ladix")
	// период без по_дате → §SM-9.A (Analyze).
	src := "источник s:\n    файл: \"data.json\"\n\n" +
		"метрика m:\n    источник: s\n    агрегат: сумма(x)\n    период: ежемесячно\n"
	if err := os.WriteFile(srcFile, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := realMain([]string{"metric", srcFile, "m"}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1; stderr=%q", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "метрика 'm': 'период' требует 'по_дате'") {
		t.Errorf("stderr = %q не содержит §SM-9.A", errBuf.String())
	}
}
