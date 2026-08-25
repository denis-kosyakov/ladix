package metrics

// Задача 4.2 — Замок «без wall-clock» (design.md Д-3, spec.md Requirement
// «Детерминизм исполнения»).
//
// Обоснование (Д-3): детерминизм пакета metrics обязан быть свойством
// СИГНАТУРЫ (Options.Today инжектируется потребителем), а не дисциплины
// разработчика («просто не будем звать time.Now()»). Дисциплина не
// проверяется машинно и рассыпается при первой правке под давлением дедлайна;
// грамматический грep исходников — проверяется. Тест грепает НЕ-тестовые
// *.go пакета metrics на литеральный вызов time.Now и падает при первом
// найденном вхождении.
//
// Мутпроба (проведена вручную, мутация не коммитилась): временная вставка
// `_ = time.Now()` в metrics/options.go (+ `"time"` в импорт) → тест
// покраснел с сообщением о найденном time.Now в этом файле; мутация
// откачена, `git diff --stat metrics/` пуст.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoWallClockInNonTestSources — грепает *.go пакета metrics (кроме
// *_test.go) на "time.Now" построчно. Читает через os.ReadDir/os.ReadFile по
// относительному пути "." — тест исполняется в каталоге пакета metrics
// (go test устанавливает cwd = директория пакета).
func TestNoWallClockInNonTestSources(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("os.ReadDir(\".\"): %v", err)
	}

	checked := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		checked++

		data, rerr := os.ReadFile(filepath.Join(".", name))
		if rerr != nil {
			t.Fatalf("os.ReadFile(%q): %v", name, rerr)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "time.Now") {
				t.Errorf("%s:%d: найден wall-clock time.Now в не-тестовом файле пакета metrics — "+
					"детерминизм (Д-3) обязан идти через инжектируемую Options.Today, а не системное время: %q",
					name, i+1, strings.TrimSpace(line))
			}
		}
	}

	if checked == 0 {
		t.Fatalf("не найдено ни одного не-тестового *.go в metrics/ — замок вхолостую, проверьте рабочий каталог")
	}
}
