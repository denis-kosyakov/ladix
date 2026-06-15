package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// quickstart_smoke_test.go — smoke-замок воспроизводимости пользовательского
// docs/quickstart.md (US4 фичи 009; FR-012/FR-013, SC-006).
//
// Замок ГЕРМЕТИЧЕН и комплементарен reproducibility_smoke_test.go: тот закрывает
// шаги 1–2 quickstart (сборка мостом README + run hello), здесь закрываются шаги 3
// (первая метрика) и 4 (первый процесс: run --db → tasks → complete × 2). Бинарник
// собирается ровно мостом README («cd src && go build -o ... ./cmd/ladix») в
// t.TempDir(); все команды запускаются из КОРНЯ репозитория, как обещает quickstart;
// БД процесса живёт в t.TempDir() (свежая → детерминированные id t-000001/p-000001).
// Сверяется exit 0 и БАЙТ-ТОЧНЫЙ stdout детерминированных шагов (метрики без периода,
// процесс без `срок`). Дата-зависимые шаги (онбординг со `срок`) сюда не входят
// намеренно. Без сети, без общего состояния.

// buildQuickstartBin собирает CLI-бинарник мостом README в t.TempDir() и возвращает
// путь к нему и корень репозитория. Тело замков ниже запускает его из root.
func buildQuickstartBin(t *testing.T) (binPath, root string) {
	t.Helper()
	root = repoRoot()
	srcDir := filepath.Join(root, "src")
	if _, err := os.Stat(filepath.Join(srcDir, "go.mod")); err != nil {
		t.Fatalf("корень репозитория не найден (нет src/go.mod) от %q: %v", root, err)
	}
	binName := "ladix"
	if runtime.GOOS == "windows" {
		binName = "ladix.exe"
	}
	binPath = filepath.Join(t.TempDir(), binName)
	build := exec.Command("go", "build", "-o", binPath, "./cmd/ladix")
	build.Dir = srcDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("сборка quickstart-бинарника провалилась (мост README сломан?): %v\n%s", err, out)
	}
	return binPath, root
}

// runFromRoot исполняет бинарник из корня репозитория, требует exit 0 и возвращает stdout.
func runFromRoot(t *testing.T, binPath, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("quickstart-команда %v провалилась (хотим exit 0): %v\nstderr=%s", args, err, stderr)
	}
	return string(out)
}

// TestQuickstartSmoke_MetricStep — шаг 3 quickstart: первая метрика. Метрики из
// examples/метрики.ladix объявлены без периода → значение детерминировано.
func TestQuickstartSmoke_MetricStep(t *testing.T) {
	if testing.Short() {
		t.Skip("smoke: пропуск в -short (компилирует бинарник)")
	}
	binPath, root := buildQuickstartBin(t)
	metricsFile := filepath.Join("examples", "метрики.ladix")

	cases := []struct {
		name string
		want string
	}{
		{"выручка_оплачено", "2000000\n"},
		{"число_заказов", "3\n"},
		{"расходы_оплачено", "1000000\n"},
	}
	for _, c := range cases {
		got := runFromRoot(t, binPath, root, "metric", metricsFile, c.name)
		if got != c.want {
			t.Errorf("metric %s: stdout байт-не-точен:\nполучено %q\nхотим   %q", c.name, got, c.want)
		}
	}
}

// TestQuickstartSmoke_ProcessLifecycle — шаг 4 quickstart: первый процесс целиком
// (run --db → tasks → complete → complete). examples/процесс.ladix не имеет `срок` →
// весь stdout детерминирован; свежая БД в TempDir даёт id t-000001/p-000001.
func TestQuickstartSmoke_ProcessLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("smoke: пропуск в -short (компилирует бинарник)")
	}
	binPath, root := buildQuickstartBin(t)
	procFile := filepath.Join("examples", "процесс.ladix")
	dbPath := filepath.Join(t.TempDir(), "demo.db")

	// 4.1 run --db: старт инстанса, первая задача собрать_заказ.
	got := runFromRoot(t, binPath, root, "run", procFile, "--db", dbPath)
	want := "[задача] t-000001 → комплектовщик, шаг 'собрать_заказ'\n" +
		"запущена выдача заказа, id: p-000001\n" +
		"открытых задач: 1\n" +
		"t-000001  p-000001  'собрать_заказ'  комплектовщик\n"
	if got != want {
		t.Fatalf("4.1 run --db stdout байт-не-точен:\nполучено %q\nхотим   %q", got, want)
	}

	// 4.2 tasks: открытая задача собрать_заказ.
	got = runFromRoot(t, binPath, root, "tasks", "--db", dbPath)
	want = "t-000001  p-000001  'собрать_заказ'  комплектовщик\n"
	if got != want {
		t.Fatalf("4.2 tasks stdout байт-не-точен:\nполучено %q\nхотим   %q", got, want)
	}

	// 4.3 complete первого шага: пробуждение, следующая задача отгрузить.
	got = runFromRoot(t, binPath, root, "complete", procFile, "t-000001", "--db", dbPath)
	want = "задача t-000001 завершена\n" +
		"[задача] t-000002 → логист, шаг 'отгрузить'\n" +
		"инстанс p-000001: ожидает, шаг 'отгрузить'\n"
	if got != want {
		t.Fatalf("4.3 complete t-000001 stdout байт-не-точен:\nполучено %q\nхотим   %q", got, want)
	}

	// 4.4 complete второго шага: процесс выполнен.
	got = runFromRoot(t, binPath, root, "complete", procFile, "t-000002", "--db", dbPath)
	want = "задача t-000002 завершена\n" +
		"инстанс p-000001: выполнен\n"
	if got != want {
		t.Fatalf("4.4 complete t-000002 stdout байт-не-точен:\nполучено %q\nхотим   %q", got, want)
	}

	// Хвост: открытых задач не осталось.
	got = runFromRoot(t, binPath, root, "tasks", "--db", dbPath)
	if want = "открытых задач нет\n"; !strings.Contains(got, want) {
		t.Fatalf("хвост tasks: ожидали %q, получили %q", want, got)
	}
}
