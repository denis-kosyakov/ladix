package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// source_base_test.go — CLI-замки флага --source-base (фича 026, §SM-8.1, D-1/D-2):
// (1) флаг без значения → код 2 + дословная диагностика; (2) флаг переопределяет базу
// резолва относительных путей источников (обе формы «--source-base B» и «--source-base=B»).

// TestSourceBaseFlagMissingValue — --source-base в хвосте без значения → код 2 и
// stderr «ladix: флаг --source-base требует значение» (зеркало --interval/--max-depth).
// Проверяется на run и metric (общий парсинг-паттерн флага).
func TestSourceBaseFlagMissingValue(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"run", []string{"run", absExample(t, "выручка.ladix"), "--source-base"}},
		{"metric", []string{"metric", metricFixture(), "выручка_месяца", "--source-base"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := realMain(tc.args, &out, &errBuf)
			if code != 2 {
				t.Fatalf("код = %d, хотим 2; stderr=%q", code, errBuf.String())
			}
			if !strings.Contains(errBuf.String(), "ladix: флаг --source-base требует значение") {
				t.Errorf("stderr = %q не содержит «ladix: флаг --source-base требует значение»", errBuf.String())
			}
		})
	}
}

// TestSourceBaseFlagOverride — флаг ПЕРЕОПРЕДЕЛЯЕТ базу резолва. Готовим программу в
// каталоге A (БЕЗ подкаталога data/, ссылается на «data/sales.json») и каталог B с
// data/sales.json (копия из examples/data). Метрика-триггер форсит загрузку источника:
//   - БЕЗ флага: база = dir(программы) = A → файл не найден → exit 1;
//   - С --source-base B (и формой --source-base=B): база = B → загрузка ОК → exit 0.
//
// 🔁 ИНВЕРСИЯ: если флаг игнорируется (база всегда от каталога файла), форма с флагом
// даст exit 1 → красный.
func TestSourceBaseFlagOverride(t *testing.T) {
	// Каталог A: программа, ссылается на относительный «data/sales.json» (которого тут НЕТ).
	dirA := t.TempDir()
	prog := filepath.Join(dirA, "m.ladix")
	// Источник + метрика без периода (дата-независима) + метрика-триггер форсит загрузку
	// под `run`. Порог заведомо выше суммы → триггер срабатывает, тело пустое.
	src := "источник продажи:\n    файл: \"data/sales.json\"\n\n" +
		"метрика выручка:\n    источник: продажи\n    где:      статус == \"оплачен\"\n    агрегат:  сумма(сумма_заказа)\n\n" +
		"когда метрика выручка < 999_999_999:\n    печать(\"низко\")\n"
	if err := os.WriteFile(prog, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	// Каталог B: data/sales.json — копия из examples/data (данные те же, файл переехал).
	dirB := t.TempDir()
	if err := os.Mkdir(filepath.Join(dirB, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	srcData, err := os.ReadFile(filepath.Join(examplesDir(t), "data", "sales.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirB, "data", "sales.json"), srcData, 0o644); err != nil {
		t.Fatal(err)
	}

	// (а) БЕЗ флага: база = dirA (нет data/) → «файл не найден» → exit 1.
	t.Run("без_флага_файл_не_найден", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		code := realMain([]string{"run", prog}, &out, &errBuf)
		if code != 1 {
			t.Fatalf("код = %d, хотим 1 (источник не должен резолвиться из A); stderr=%q", code, errBuf.String())
		}
		if !strings.Contains(errBuf.String(), "не найден") {
			t.Errorf("stderr = %q не содержит «не найден»", errBuf.String())
		}
	})

	// (б) --source-base B (раздельная форма): база = dirB → загрузка ОК → exit 0.
	t.Run("флаг_раздельно", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		code := realMain([]string{"run", prog, "--source-base", dirB}, &out, &errBuf)
		if code != 0 {
			t.Fatalf("код = %d, хотим 0 (флаг должен переопределить базу на B); stderr=%q", code, errBuf.String())
		}
		if strings.Contains(errBuf.String(), "не найден") {
			t.Errorf("источник не резолвится из B при --source-base: %q", errBuf.String())
		}
	})

	// (в) --source-base=B (слитная форма): эквивалентна раздельной → exit 0.
	t.Run("флаг_слитно", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		code := realMain([]string{"run", prog, "--source-base=" + dirB}, &out, &errBuf)
		if code != 0 {
			t.Fatalf("код = %d, хотим 0 (форма --source-base=B); stderr=%q", code, errBuf.String())
		}
		if strings.Contains(errBuf.String(), "не найден") {
			t.Errorf("источник не резолвится из B при --source-base=B: %q", errBuf.String())
		}
	})
}
