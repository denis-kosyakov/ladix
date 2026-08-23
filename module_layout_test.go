package ladix_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Замки раскладки модуля (US2/US3). Тест живёт в КОРНЕВОМ пакете, поэтому его
// рабочий каталог — корень репозитория: go.mod лежит рядом.

// TestModuleLivesAtRepoRoot — A5/A6: go.mod находится в корне репозитория и
// объявляет module-path БЕЗ сегмента /src. Именно это делает возможным
// `go get github.com/denis-kosyakov/ladix`. Краснеет при откате переезда.
func TestModuleLivesAtRepoRoot(t *testing.T) {
	b, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("go.mod обязан лежать в корне репозитория: %v", err)
	}
	text := string(b)

	const want = "module github.com/denis-kosyakov/ladix"
	first := strings.SplitN(strings.TrimSpace(text), "\n", 2)[0]
	if strings.TrimSpace(first) != want {
		t.Errorf("директива module = %q, ожидалась %q", first, want)
	}
	if strings.Contains(text, "denis-kosyakov/ladix/src") {
		t.Error("module-path не имеет права содержать сегмент /src — он ломает go get с корня")
	}
	if _, err := os.Stat("src"); err == nil {
		t.Error("каталог src/ восстановлен — модуль обязан жить в корне (схлопывание фичи 029)")
	}
}

// TestGoDirectiveMatchesDocumentedFloor — A7 в редакции Complexity Tracking плана
// 029: FR-011 (понижение до 1.23) НЕ выполнен — modernc.org/sqlite v1.52.0
// требует go 1.25.0, а директива у модуля одна на всех. Замок пинит фактическую
// директиву и её согласованность с порогом, который называет README: рассинхрон
// документации и go.mod краснит тест.
func TestGoDirectiveMatchesDocumentedFloor(t *testing.T) {
	gomod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("не прочитать go.mod: %v", err)
	}
	if !strings.Contains(string(gomod), "go 1.25") {
		t.Errorf("go.mod не содержит директиву «go 1.25» — обнови README и этот замок вместе с ней")
	}
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("не прочитать README.md: %v", err)
	}
	if !strings.Contains(string(readme), "1.25") {
		t.Error("README не называет фактический порог Go «1.25» из go.mod — версии рассинхронизированы")
	}
}

// TestInternalPackagesStayInternal — A10/FR-003/FR-004: публичной поверхностью
// остаются РОВНО ladix (корень) + ir; всё прочее — под internal/, CLI — в
// cmd/ladix. Краснеет при случайном выносе пакета наружу.
func TestInternalPackagesStayInternal(t *testing.T) {
	internals := []string{
		"lexer", "parser", "ast", "value", "errors",
		"eval", "engine", "store", "daemon", "jsonval",
	}
	for _, pkg := range internals {
		if _, err := os.Stat(filepath.Join("internal", pkg)); err != nil {
			t.Errorf("пакет internal/%s не найден: %v", pkg, err)
		}
		if _, err := os.Stat(pkg); err == nil {
			t.Errorf("пакет %q вынесен из internal/ наружу — публичная поверхность обязана исчерпываться ladix+ir (FR-003)", pkg)
		}
	}
	if _, err := os.Stat(filepath.Join("cmd", "ladix")); err != nil {
		t.Errorf("reference-CLI cmd/ladix отсутствует: %v", err)
	}
	if _, err := os.Stat("ir"); err != nil {
		t.Errorf("публичный пакет ir отсутствует: %v", err)
	}
}
