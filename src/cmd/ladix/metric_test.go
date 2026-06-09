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
// Фикстура §SM-10 и data/sales.json адресуются относительно cwd процесса; в тестах
// metric chdir-имся в корень репо, чтобы относительный путь файла источника
// «data/sales.json» из фикстуры резолвился (§SM-8.1, §9.1).
func repoRoot() string { return filepath.Join("..", "..", "..") }

// metricFixture — путь к метрик-онли фикстуре §SM-10 (internal/eval/testdata),
// относительно корня репозитория.
func metricFixture() string {
	return filepath.Join("src", "internal", "eval", "testdata", "metric_only.ladix")
}

// fixedClock2026 — детерминированный Clock golden-приёмки §SM-10 (D=2026-05-31).
var fixedClock2026 = eval.FixedClock{D: value.Дата{Year: 2026, Month: 5, Day: 31}}

// withRepoRoot выполняет fn из корня репозитория (chdir + восстановление), чтобы
// относительный путь источника «data/sales.json» из фикстуры резолвился.
func withRepoRoot(t *testing.T, fn func()) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repoRoot()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatal(err)
		}
	}()
	fn()
}

// T029: успешный путь — ladix metric <фикстура> выручка_месяца → «2000000» / код 0
// (§SM-10, FixedClock 2026-05-31). Через runMetric с инжектированным FixedClock
// (прод-путь metricMain строит SystemClock — дата-зависим, CM-6).
func TestMetricSuccess(t *testing.T) {
	withRepoRoot(t, func() {
		var out, errBuf bytes.Buffer
		code := runMetric(metricFixture(), "выручка_месяца", eval.DefaultMaxDepth, fixedClock2026, &out, &errBuf)
		if code != 0 {
			t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
		}
		if out.String() != "2000000\n" {
			t.Errorf("stdout = %q, хотим %q", out.String(), "2000000\n")
		}
		if errBuf.Len() != 0 {
			t.Errorf("непустой stderr: %q", errBuf.String())
		}
	})
}

// T030: неизвестная метрика → stderr «неизвестная метрика '<имя>'» / код 1 (§SM-9.D).
func TestMetricUnknownName(t *testing.T) {
	withRepoRoot(t, func() {
		var out, errBuf bytes.Buffer
		code := runMetric(metricFixture(), "нет_такой", eval.DefaultMaxDepth, fixedClock2026, &out, &errBuf)
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
	})
}

// T030: имя источника (не метрика) → stderr «'<имя>' — не метрика» / код 1 (§SM-9.D).
func TestMetricNotAMetric(t *testing.T) {
	withRepoRoot(t, func() {
		var out, errBuf bytes.Buffer
		code := runMetric(metricFixture(), "продажи", eval.DefaultMaxDepth, fixedClock2026, &out, &errBuf)
		if code != 1 {
			t.Fatalf("код = %d, хотим 1", code)
		}
		if out.Len() != 0 {
			t.Errorf("непустой stdout: %q", out.String())
		}
		if !strings.Contains(errBuf.String(), "'продажи' — не метрика") {
			t.Errorf("stderr = %q не содержит §SM-9.D «не метрика»", errBuf.String())
		}
	})
}

// T030: предопределённый период (не метрика) → stderr «'<имя>' — не метрика» / код 1.
func TestMetricPeriodNotAMetric(t *testing.T) {
	withRepoRoot(t, func() {
		var out, errBuf bytes.Buffer
		code := runMetric(metricFixture(), "ежемесячно", eval.DefaultMaxDepth, fixedClock2026, &out, &errBuf)
		if code != 1 {
			t.Fatalf("код = %d, хотим 1", code)
		}
		if !strings.Contains(errBuf.String(), "'ежемесячно' — не метрика") {
			t.Errorf("stderr = %q не содержит §SM-9.D «не метрика»", errBuf.String())
		}
	})
}

// T029/T030: usage-коды подкоманды metric (код 2) — нет/мало/лишних позиционных,
// нечитаемый файл, неверный --max-depth, неизвестный флаг (через realMain-диспетчер).
func TestMetricUsageErrors(t *testing.T) {
	withRepoRoot(t, func() {
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
	})
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
	if !strings.Contains(errBuf.String(), "источник 's': файл «нет.json» не найден") {
		t.Errorf("stderr = %q не содержит §SM-9.B «файл не найден»", errBuf.String())
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
