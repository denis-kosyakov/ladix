package ladix_test

import (
	"os/exec"
	"strings"
	"testing"
)

// Страж границы «публичная поверхность ↔ internal backend» (contracts/import-boundary.md).
//
// Инвариант: потребитель, импортирующий библиотеку, НЕ ОБЯЗАН тянуть в свой граф
// зависимостей SQLite, движок процессов, хранилище и демон. Проверяется машинно —
// через транзитивное замыкание `go list -deps`, а не «на честном слове».
//
// internal/eval из запретного списка ЯВНО ИСКЛЮЧЁН: фасад зовёт его Analyze ради
// семантической валидации, и сам eval sqlite-free (это и доказывает T1).

const (
	pkgFacade  = "github.com/denis-kosyakov/ladix"
	pkgIR      = "github.com/denis-kosyakov/ladix/ir"
	pkgMetrics = "github.com/denis-kosyakov/ladix/metrics"

	depSQLite = "modernc.org/sqlite"
	depStore  = "github.com/denis-kosyakov/ladix/internal/store"
	depEngine = "github.com/denis-kosyakov/ladix/internal/engine"
	depDaemon = "github.com/denis-kosyakov/ladix/internal/daemon"
	depEval   = "github.com/denis-kosyakov/ladix/internal/eval"
)

// deps возвращает транзитивное замыкание импортов пакета. Если тулчейн go
// недоступен (герметичная песочница), тест ПРОПУСКАЕТСЯ — молчаливый зелёный
// без проверки был бы хуже честного skip.
func deps(t *testing.T, pkg string) map[string]bool {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("страж границы требует тулчейн go в PATH")
	}
	out, err := exec.Command("go", "list", "-deps", pkg).Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}
	set := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			set[line] = true
		}
	}
	if len(set) == 0 {
		t.Fatalf("пустое замыкание для %s — страж не должен быть зелёным вхолостую", pkg)
	}
	return set
}

func assertAbsent(t *testing.T, pkg string, closure map[string]bool, forbidden ...string) {
	t.Helper()
	for _, f := range forbidden {
		if closure[f] {
			t.Errorf("граница пробита: %s тянет %s (потребитель библиотеки не должен его получать)", pkg, f)
		}
	}
}

// TestBoundaryT1FacadeIsSQLiteFree — T1: замыкание фасада НЕ содержит SQLite.
// Мягче T3 по internal/eval намеренно: eval фасаду легитимен, но обязан
// оставаться sqlite-free — если eval когда-нибудь потянет store, SQLite
// протечёт сюда транзитивно и тест покраснеет.
func TestBoundaryT1FacadeIsSQLiteFree(t *testing.T) {
	closure := deps(t, pkgFacade)
	if !closure[depEval] {
		t.Errorf("ожидалось, что фасад зависит от %s (семантический проход Analyze) — проверка стала холостой", depEval)
	}
	assertAbsent(t, pkgFacade, closure, depSQLite)
}

// TestBoundaryT2IRIsMinimalLeaf — T2: замыкание ir строго минимально —
// ни SQLite, ни backend, ни даже eval. ir обязан оставаться листом.
func TestBoundaryT2IRIsMinimalLeaf(t *testing.T) {
	closure := deps(t, pkgIR)
	assertAbsent(t, pkgIR, closure, depSQLite, depStore, depEngine, depDaemon, depEval)
}

// TestBoundaryT3PublicSurfaceHasNoBackend — T3: ни один пакет ПУБЛИЧНОЙ
// поверхности (по FR-003 их теперь три: ladix, ir, metrics — 030 добавляет
// metrics) не тянет sqlite/store/engine/daemon. При аддитивном расширении
// поверхности новый пакет добавляется в этот список. Проверяется и sqlite:
// по §MC-4 ни один публичный пакет не тянет modernc.org/sqlite.
//
// metrics, как и ladix, ВПРАВЕ зависеть от internal/eval (переиспользует
// EvalMetricPipeline/ApplySourceSchema) — это не нарушение T3, T3 проверяет
// sqlite и store/engine/daemon:
// eval обязан оставаться sqlite-free, и T3 это доказывает для каждого
// пакета поверхности напрямую.
func TestBoundaryT3PublicSurfaceHasNoBackend(t *testing.T) {
	for _, pkg := range []string{pkgFacade, pkgIR, pkgMetrics} {
		t.Run(pkg, func(t *testing.T) {
			assertAbsent(t, pkg, deps(t, pkg), depSQLite, depStore, depEngine, depDaemon)
		})
	}
}
