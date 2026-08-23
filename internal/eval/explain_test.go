package eval

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// buildExplainInterp парсит/анализирует/прогоняет src над temp-JSON-фикстурой и
// возвращает интерпретатор + буфер i.out (тело триггера и explain пишут сюда).
// Путь источника переписывается на временный файл — без зависимости от cwd.
func buildExplainInterp(t *testing.T, src, dataJSON string) (*Interpreter, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	dataPath := filepath.Join(dir, "данные.json")
	if err := os.WriteFile(dataPath, []byte(dataJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("лексические/синтаксические ошибки: %v", errList.Error())
	}
	for _, item := range prog.Items {
		if sd, ok := item.(*ast.SourceDecl); ok {
			sd.File.Value = dataPath
		}
	}
	out := &bytes.Buffer{}
	i := NewInterpreter(out, 0, testClock)
	if err := i.Analyze(prog); err != nil {
		t.Fatalf("Analyze вернул ошибку: %v", err)
	}
	if err := i.Run(prog); err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}
	return i, out
}

// TestRunTriggerExplain (§C-5.4, T005) — ALWAYS-ON explain на пути run (fire-if-true):
// при срабатывании метрика-триггера в i.out печатается ровно одна строка-explain
// §C-5.3 (БЕЗ маркера ребра — run, не serve), ДО вывода тела. Числа без подчёркиваний
// (value.String). Exact-match всего out: explain-строка, затем строка тела.
//
// 🔁 ИНВЕРСИЯ (мутпроба §C-5.4): снять emit в trigger_run.go → out=только тело →
// exact-match краснеет (explain отсутствует).
func TestRunTriggerExplain(t *testing.T) {
	src := "источник s:\n    файл: \"X\"\n" +
		"метрика выручка_30д:\n    источник: s\n    агрегат: сумма(сумма_заказа)\n" +
		"когда метрика выручка_30д < 3000000:\n    печать(\"тело:\", значение)\n"
	i, out := buildExplainInterp(t, src, `[{"сумма_заказа":2500000}]`)

	var w bytes.Buffer
	if err := i.RunTriggers(&w); err != nil {
		t.Fatalf("RunTriggers вернул ошибку: %v", err)
	}

	// explain ДО тела; run-форма без «(ребро …)». снимок=2500000, порог=3000000.
	want := "триггер 'выручка_30д < 3000000' сработал: выручка_30д = 2500000 (снимок) < 3000000 (порог) → истина\n" +
		"тело: 2500000\n"
	if got := out.String(); got != want {
		t.Errorf("explain run-формы байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}
	if w.Len() != 0 {
		t.Errorf("explain не должен идти в writer RunTriggers (w), w=%q", w.String())
	}
}

// TestRunTriggerNoExplainWhenSilent (§C-5.4) — при ЛОЖНОМ условии тело не исполняется
// И explain НЕ печатается (explain привязан к fire, не к проходу). Exact-match: out
// пуст. 🔁 ИНВЕРСИЯ: если explain печатать вне fire-ветки → out непуст → красный.
func TestRunTriggerNoExplainWhenSilent(t *testing.T) {
	src := "источник s:\n    файл: \"X\"\n" +
		"метрика выручка_30д:\n    источник: s\n    агрегат: сумма(сумма_заказа)\n" +
		"когда метрика выручка_30д < 1000000:\n    печать(\"тело:\", значение)\n"
	i, out := buildExplainInterp(t, src, `[{"сумма_заказа":2500000}]`)

	var w bytes.Buffer
	if err := i.RunTriggers(&w); err != nil {
		t.Fatalf("RunTriggers вернул ошибку: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("ложное условие: ни тела, ни explain — out должен быть пуст, got=%q", out.String())
	}
}

// TestExplainFireFormat — модульный замок единого форматтера (§C-5.3): обе формы
// (run withEdge=false, serve withEdge=true) дословно. Защищает текст от дрейфа в
// одном месте; пути run/serve лишь зовут эту функцию.
func TestExplainFireFormat(t *testing.T) {
	snap := value.Целое{V: 2500000}
	thr := value.Целое{V: 3000000}
	if got, want := ExplainFire("выручка_30д", ast.CompLt, snap, thr, false),
		"триггер 'выручка_30д < 3000000' сработал: выручка_30д = 2500000 (снимок) < 3000000 (порог) → истина"; got != want {
		t.Errorf("run-форма = %q, хотим %q", got, want)
	}
	if got, want := ExplainFire("выручка_30д", ast.CompLt, snap, thr, true),
		"триггер 'выручка_30д < 3000000' сработал (ребро ложь→истина): выручка_30д = 2500000 (снимок) < 3000000 (порог) → истина"; got != want {
		t.Errorf("serve-форма = %q, хотим %q", got, want)
	}
}
