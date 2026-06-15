package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Контракт: specs/009-v1-finalization/contracts/docs-alignment.md (A1–A4).
// Замок US5 (T036): доки выровнены с реальностью, а поведение кода НЕ изменилось.
// ИНВЕРСИЯ: возврат любого снятого утверждения (Go 1.22 в командах сборки README,
// сводка «Найдено K ошибок», «deferred до 006» в онбординге, достижимость тип(x))
// → красный.

// readRepoDoc читает файл доки из корня репозитория (как repoRoot() в metric_test.go).
func readRepoDoc(t *testing.T, rel string) string {
	t.Helper()
	p := filepath.Join(repoRoot(), rel)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("не прочитать %s: %v", rel, err)
	}
	return string(b)
}

// A1: версия Go выровнена — в командах сборки README больше нет «1.22»;
// фактический минимум go.mod (go 1.25) указан как порог.
func TestDocsAlignmentA1GoVersion(t *testing.T) {
	readme := readRepoDoc(t, "README.md")

	// Команды сборки README не должны обещать Go 1.22 (порог занижен относительно go.mod).
	if strings.Contains(readme, "1.22") {
		t.Errorf("A1: README всё ещё упоминает «1.22» — версия Go рассинхронизирована с src/go.mod (go 1.25)")
	}
	// Фактический минимум присутствует.
	if !strings.Contains(readme, "1.25") {
		t.Errorf("A1: README не называет фактический минимум Go «1.25» из src/go.mod")
	}

	// Пользовательский quickstart тоже не должен занижать порог до 1.22.
	quickstart := readRepoDoc(t, "docs/quickstart.md")
	if strings.Contains(quickstart, "1.22") {
		t.Errorf("A1: docs/quickstart.md всё ещё упоминает «1.22» — версия Go рассинхронизирована с src/go.mod (go 1.25)")
	}

	// Источник истины — go.mod: порог не понижается ниже зафиксированного там.
	gomod := readRepoDoc(t, "src/go.mod")
	if !strings.Contains(gomod, "go 1.25") {
		t.Errorf("A1: src/go.mod не содержит «go 1.25» — обнови привязку версии в README/тесте")
	}
}

// A2: обещание итоговой строки-сводки снято — ни README, ни SPEC §13 не утверждают,
// что агрегатор печатает «Найдено K ошибок». (Сам код её не печатает: см.
// internal/errors/aggregate_test.go.)
func TestDocsAlignmentA2NoFoundSummary(t *testing.T) {
	for _, f := range []string{"README.md", "SPEC.md"} {
		doc := readRepoDoc(t, f)
		// Обещанная сводка вида «Найдено K ошибок» (с любым символом между «Найдено» и «ошибок»).
		if strings.Contains(doc, "Найдено K ошибок") {
			t.Errorf("A2: %s всё ещё обещает сводку «Найдено K ошибок» — код её не печатает", f)
		}
	}
}

// A3: стейл-коммент онбординга переписан — нет «deferred до 006» (006 смержена,
// процесс исполняется движком).
func TestDocsAlignmentA3OnboardingComment(t *testing.T) {
	onb := readRepoDoc(t, "examples/онбординг.ladix")
	if strings.Contains(onb, "deferred до 006") {
		t.Errorf("A3: examples/онбординг.ladix всё ещё содержит стейл-коммент «deferred до 006»")
	}
}

// A4: тип(x) недостижим из синтаксиса в v1 — имя «тип» зарезервировано лексером.
// печать(тип(5)) даёт reserved-word ошибку (код 1), а НЕ «Целое».
// Это эмпирический инвариант поведения (docs-alignment §«Инварианты поведения»).
func TestDocsAlignmentA4TipReserved(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "tip.ladix")
	if err := os.WriteFile(file, []byte("печать(тип(5))\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", file}, &out, &errBuf)

	if code != 1 {
		t.Fatalf("A4: печать(тип(5)) дал код %d, хотим 1 (reserved-word ошибка); stdout=%q stderr=%q",
			code, out.String(), errBuf.String())
	}
	// Должна быть именно reserved-word диагностика про «тип», а не успешный вывод «Целое».
	if !strings.Contains(errBuf.String(), "зарезервированное слово") {
		t.Errorf("A4: stderr не содержит «зарезервированное слово»: %q", errBuf.String())
	}
	if strings.Contains(out.String(), "Целое") {
		t.Errorf("A4: тип(x) стал достижим — stdout вернул «Целое»: %q (функция активирована вопреки v1-резерву)", out.String())
	}
}
