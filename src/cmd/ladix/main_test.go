package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func examplePath(name string) string {
	return filepath.Join("..", "..", "..", "examples", name)
}

// T044/T045: успешный прогон → stdout, код 0.
func TestRunSuccess(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", examplePath("hello.ladix")}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if out.String() != "Привет, Уклад!\n" {
		t.Errorf("stdout = %q", out.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
}

// T047/SC-002: ошибка.ladix → ровно двухстрочный stderr, код 1, без stack trace.
func TestRunErrorExample(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", examplePath("ошибка.ladix")}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1", code)
	}
	want := "Ошибка в строке 5, колонка 14:\nделение на ноль\n"
	if errBuf.String() != want {
		t.Errorf("stderr = %q, хотим %q", errBuf.String(), want)
	}
	if strings.Contains(errBuf.String(), ".go:") || strings.Contains(errBuf.String(), "goroutine") {
		t.Errorf("в stderr просочился Go stack trace: %q", errBuf.String())
	}
}

// Коды использования CLI (код 2): нет подкоманды/файла/неверный флаг/нет файла.
func TestUsageErrors(t *testing.T) {
	cases := [][]string{
		{},                     // нет подкоманды
		{"run"},                // нет файла
		{"run", "--max-depth"}, // флаг без значения
		{"run", "--max-depth", "0", examplePath("hello.ladix")}, // неверное значение
		{"run", "--нечто", examplePath("hello.ladix")},          // неизвестный флаг
		{"run", "нет-такого-файла.ladix"},                       // файл не читается
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := realMain(args, &out, &errBuf)
			if code != 2 {
				t.Errorf("args=%v: код = %d, хотим 2", args, code)
			}
		})
	}
}

// --max-depth пробрасывается в лимит и в текст ошибки RT-DEPTH.
func TestMaxDepthFlag(t *testing.T) {
	// файл с убегающей рекурсией готовим во временном каталоге
	dir := t.TempDir()
	file := filepath.Join(dir, "rec.ladix")
	src := "функция f():\n    вернуть f()\nпечать(f())\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", "--max-depth", "7", file}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1", code)
	}
	if !strings.Contains(errBuf.String(), "превышена максимальная глубина вызовов (7).") {
		t.Errorf("stderr не содержит лимит 7: %q", errBuf.String())
	}
}

// 005/SC-004 (CP-5.2 N15, CP-5.4): онбординг.ladix проходит парс+семантику чисто
// и падает в РАНТАЙМЕ на top-level 'запустить процесс' — единственная наблюдаемая
// рантайм-граница 005 (§PM-5, FR-025) → код 1, двухстрочная ошибка с payload §DP-4.
// stdout пуст: 'пусть id = запустить процесс …' — первый top-level оператор,
// до 'печать' исполнение не доходит.
func TestRunOnboardingProcessDeferred(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", examplePath("онбординг.ladix")}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1; stderr=%q", code, errBuf.String())
	}
	stderr := errBuf.String()
	if !strings.HasPrefix(stderr, "Ошибка в строке ") {
		t.Errorf("stderr не начинается с заголовка двухстрочной ошибки: %q", stderr)
	}
	if !strings.Contains(stderr, ":\nконструкция запустить процесс не поддерживается в этой версии\n") {
		t.Errorf("в stderr нет payload §DP-4 второй строкой: %q", stderr)
	}
	if out.Len() != 0 {
		t.Errorf("непустой stdout: %q", out.String())
	}
	if strings.Contains(stderr, ".go:") || strings.Contains(stderr, "goroutine") {
		t.Errorf("в stderr просочился Go stack trace: %q", stderr)
	}
}

// 005/SC-005 (CP-5.3 R1): выручка.ladix остаётся парс-ошибкой код 1, но падение
// сдвинулось — 'процесс' теперь парсится, отвергается top-level 'когда'
// (D-6, триггеры — 007).
func TestRunRevenueUnexpectedWhen(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", examplePath("выручка.ladix")}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("код = %d, хотим 1; stderr=%q", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "неожиданный токен 'когда'") {
		t.Errorf("в stderr нет парс-ошибки на 'когда': %q", errBuf.String())
	}
	if strings.Contains(errBuf.String(), "неожиданный токен 'процесс'") {
		t.Errorf("регресс: парсер снова отвергает 'процесс': %q", errBuf.String())
	}
	if out.Len() != 0 {
		t.Errorf("непустой stdout: %q", out.String())
	}
}

// 005/FR-023 (CP-3): программа, ТОЛЬКО объявляющая процесс (без top-level
// 'запустить процесс') → код 0: рантайм отрабатывает, ProcessDecl — Decl,
// не Statement — пропускается циклом Run(); тело шага в 005 не исполняется
// (печать(1) внутри шага не попадает в stdout, §PM-5).
func TestRunProcessDeclOnly(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "процесс.ladix")
	src := "процесс P:\n    шаг A:\n        печать(1)\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := realMain([]string{"run", file}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("код = %d, хотим 0; stderr=%q", code, errBuf.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout не пуст — тело шага не должно исполняться: %q", out.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("непустой stderr: %q", errBuf.String())
	}
}

// T047/SC-008: recover-барьер ловит непредвиденную Go-панику → дженерик, код 1.
func TestGuardRecoversPanic(t *testing.T) {
	var errBuf bytes.Buffer
	code := guard(&errBuf, func() int {
		panic("искусственная паника")
	})
	if code != 1 {
		t.Errorf("код = %d, хотим 1", code)
	}
	if errBuf.String() != "внутренняя ошибка интерпретатора\n" {
		t.Errorf("stderr = %q", errBuf.String())
	}
	if strings.Contains(errBuf.String(), "goroutine") {
		t.Errorf("в stderr просочился stack trace")
	}
}
