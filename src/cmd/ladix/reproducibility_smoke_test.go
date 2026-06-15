package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// reproducibility_smoke_test.go — smoke-замок контракта воспроизводимости README
// (specs/009-v1-finalization/contracts/reproducibility.md, R1–R5; FR-005/SC-001).
//
// Замок ГЕРМЕТИЧЕН: собирает CLI-бинарник из Go-модуля `src/` в t.TempDir() ровно
// тем мостом, что обещает README («cd src && go build -o ../ladix ./cmd/ladix»), и
// прогоняет examples/hello.ladix из КОРНЯ репозитория («./ladix run examples/hello.ladix»),
// сверяя exit 0 и БАЙТ-ТОЧНЫЙ stdout. Если команда README ломается (неверный пакет/путь/
// рабочий каталог) — замок КРАСНЕЕТ. Без сети, без общего состояния, детерминирован.
//
// R3 (`cd src && go test ./...`) и R4 (`cd src && go vet ./...`) НЕ исполняются
// внутри этого замка: их рекурсивный self-host невозможен (go test над собой), а
// детерминированность гарантирует сам гейт фичи — этот же `go test ./...`/`go vet ./...`,
// в котором замок и живёт. Замок закрывает R1 (build) и R2 (run hello).
//
// repoRoot() (общий хелпер из metric_test.go) даёт корень репозитория относительно
// каталога пакета cmd/ladix (cwd процесса `go test`); src-каталог модуля — repoRoot()/src.

// helloGoldenStdout — байт-точный stdout examples/hello.ladix (R2). Пин совпадает с
// TestCLIGoldenStdout/hello.ladix в golden_test.go; дублируется здесь намеренно, чтобы
// smoke-канон был самодостаточен.
const helloGoldenStdout = "Привет, Уклад!\n"

// TestReproducibilitySmoke_R1Build_R2RunHello — герметичный прогон R1+R2 контракта.
func TestReproducibilitySmoke_R1Build_R2RunHello(t *testing.T) {
	if testing.Short() {
		t.Skip("smoke: пропуск в -short (компилирует бинарник)")
	}
	root := repoRoot()
	srcDir := filepath.Join(root, "src")
	if _, err := os.Stat(filepath.Join(srcDir, "go.mod")); err != nil {
		t.Fatalf("корень репозитория не найден (нет src/go.mod) от %q: %v", root, err)
	}

	binName := "ladix"
	if runtime.GOOS == "windows" {
		binName = "ladix.exe"
	}
	binPath := filepath.Join(t.TempDir(), binName)

	// R1: cd src && go build -o <tmp>/ladix ./cmd/ladix
	build := exec.Command("go", "build", "-o", binPath, "./cmd/ladix")
	build.Dir = srcDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("R1 сборка провалилась (мост README сломан?): %v\n%s", err, out)
	}
	if _, err := os.Stat(binPath); err != nil {
		t.Fatalf("R1: бинарник не появился по %q: %v", binPath, err)
	}

	// R2: из КОРНЯ репозитория — ./ladix run examples/hello.ladix
	run := exec.Command(binPath, "run", filepath.Join("examples", "hello.ladix"))
	run.Dir = root
	out, err := run.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("R2 запуск hello провалился: %v\nstderr=%s", err, stderr)
	}
	if got := string(out); got != helloGoldenStdout {
		t.Errorf("R2 stdout байт-не-точен:\nполучено %q\nхотим   %q", got, helloGoldenStdout)
	}
}
