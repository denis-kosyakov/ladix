package parser

import (
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/lexer"
)

// T042: panic-mode восстановление — несколько независимых ошибок без фантомного
// каскада, бюджет, отсутствие Go stack trace, best-effort Program (SC-005, FR-025).

func TestMultipleIndependentErrors(t *testing.T) {
	// две независимые ошибки на разных строках → ровно две диагностики
	prog, el := parseProgramSrc(t, "значение\n{\n")
	if prog == nil {
		t.Fatalf("Program == nil; ожидается best-effort дерево")
	}
	if el.Len() != 2 {
		t.Fatalf("ошибок %d, хотим 2 (без фантомного каскада):\n%s", el.Len(), el.Error())
	}
	e0 := el.Errors()[0].(errors.ParseError)
	e1 := el.Errors()[1].(errors.ParseError)
	if e0.Pos.Line != 1 || e1.Pos.Line != 2 {
		t.Errorf("строки ошибок = %d,%d, хотим 1,2", e0.Pos.Line, e1.Pos.Line)
	}
	if e0.Msg != "неожиданный элемент 'значение'" || e1.Msg != "неожиданный элемент '{'" {
		t.Errorf("сообщения: %q / %q", e0.Msg, e1.Msg)
	}
}

func TestErrorOnOneLineDoesNotCascadeNextValidLine(t *testing.T) {
	// ошибка на строке 1 не мешает корректно разобрать строку 2
	prog, el := parseProgramSrc(t, "1 < y < 10\nпечать(42)\n")
	if el.Len() != 1 {
		t.Fatalf("ошибок %d, хотим 1 (без каскада на валидную строку):\n%s", el.Len(), el.Error())
	}
	if len(prog.Items) != 2 {
		t.Errorf("Items = %d, хотим 2 (ошибочное выражение + печать)", len(prog.Items))
	}
}

func TestErrorBudgetShared(t *testing.T) {
	// много ошибочных строк → накопитель не превышает мягкий бюджет (общий с лексером)
	var b strings.Builder
	for i := 0; i < errors.DefaultErrorBudget+10; i++ {
		b.WriteString("значение\n")
	}
	_, el := parseProgramSrc(t, b.String())
	if el.Len() != errors.DefaultErrorBudget {
		t.Errorf("накоплено %d, хотим мягкий предел %d", el.Len(), errors.DefaultErrorBudget)
	}
}

func TestNoGoStackTrace(t *testing.T) {
	_, el := parseProgramSrc(t, "значение\nx.поле = 5\n1 < y < 10\n")
	out := el.Error()
	for _, marker := range []string{"goroutine", ".go:", "panic:", "runtime."} {
		if strings.Contains(out, marker) {
			t.Errorf("в выводе обнаружен Go stack trace (%q):\n%s", marker, out)
		}
	}
}

// T043: прямой тест synchronize() — потребляет NEWLINE/DEDENT, останавливается на
// ведущем ключевом слове, не потребляя его.

func TestSynchronizeConsumesNewline(t *testing.T) {
	// junk... NEWLINE пусть : отбросить до NEWLINE включительно, остановиться на пусть
	toks := []lexer.Token{
		{Type: lexer.RPAREN, Lexeme: ")", Pos: errors.Position{Line: 1, Col: 1}},
		{Type: lexer.COMMA, Lexeme: ",", Pos: errors.Position{Line: 1, Col: 2}},
		{Type: lexer.NEWLINE, Pos: errors.Position{Line: 1, Col: 3}},
		{Type: lexer.KW_LET, Lexeme: "пусть", Pos: errors.Position{Line: 2, Col: 1}},
		{Type: lexer.EOF, Pos: errors.Position{Line: 2, Col: 6}},
	}
	p := New(toks, nil)
	p.synchronize()
	if !p.check(lexer.KW_LET) {
		t.Errorf("после synchronize peek = %s, хотим KW_LET (NEWLINE потреблён, ключевое слово — нет)", p.peek().Type)
	}
}

// T045: examples/ошибка.ladix синтаксически валиден — дефект (деление на ноль) —
// рантайм, парсер ошибок не даёт.
func TestErrorExampleParsesClean(t *testing.T) {
	prog, el, lexErrs := parseExampleFile(t, "ошибка.ladix")
	if !lexErrs.Empty() {
		t.Fatalf("ошибка.ladix: лексические ошибки: %v", lexErrs.Error())
	}
	if !el.Empty() {
		t.Fatalf("ошибка.ladix: синтаксические ошибки (дефект — рантайм): %v", el.Error())
	}
	if len(prog.Items) == 0 {
		t.Errorf("ошибка.ladix: пустой Program.Items")
	}
}

func TestSynchronizeStopsAtLeadKeyword(t *testing.T) {
	// текущий токен уже ведущее ключевое слово → синхронизация не двигается
	toks := []lexer.Token{
		{Type: lexer.KW_IF, Lexeme: "если", Pos: errors.Position{Line: 1, Col: 1}},
		{Type: lexer.EOF, Pos: errors.Position{Line: 1, Col: 5}},
	}
	p := New(toks, nil)
	p.synchronize()
	if !p.check(lexer.KW_IF) {
		t.Errorf("synchronize не должен потреблять ведущее ключевое слово, peek = %s", p.peek().Type)
	}
}

// === DX1 (фича 012-mdx-diagnostics): подавление фантомного каскада ===
//
// Ведущее sync-lead ключевое слово (если/пока/вернуть/…) в позиции ВЫРАЖЕНИЯ на
// одной/смежной строке давало 2–4 диагностики на одну сломанную конструкцию
// (ре-диспетч не-потреблённого токена после сброса suppress на границе оператора).
// Фикс: parsePrimary default-ветка потребляет ошибочный токен ДО error()
// (зеркально parse_stmt.go:29). Контракт — contracts/parser-recovery.md.

// assertDiagnostics — упорядоченный count-exact ассерт (обобщает one-only
// assertGolden). Сначала КОЛИЧЕСТВО (с дампом при несовпадении), затем каждая
// диагностика по индексу байт-в-байт (двухстрочный канон §13). want — ожидаемые
// Error()-строки в порядке регистрации.
func assertDiagnostics(t *testing.T, el *errors.ErrorList, want ...string) {
	t.Helper()
	if el.Len() != len(want) {
		t.Fatalf("диагностик %d, хотим %d:\n%s", el.Len(), len(want), el.Error())
	}
	for i, w := range want {
		if got := el.Errors()[i].Error(); got != w {
			t.Errorf("диагностика #%d:\n got  = %q\nхотим = %q", i, got, w)
		}
	}
}

// C-REC-1: sync-lead KW в позиции выражения на ОДНОЙ строке → ровно 1
// диагностика (было 2). FR-001, SC-001.
func TestCascadeSameLineSingleDiagnostic(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"пусть-если", "пусть x = если\n",
			"Ошибка в строке 1, колонка 11:\nнеожиданный элемент 'если'"},
		{"пусть-пока", "пусть y = пока\n",
			"Ошибка в строке 1, колонка 11:\nнеожиданный элемент 'пока'"},
		{"печать-для", "печать(для)\n",
			"Ошибка в строке 1, колонка 8:\nнеожиданный элемент 'для'"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, el := parseProgramSrc(t, c.src)
			assertDiagnostics(t, el, c.want)
		})
	}
}

// C-REC-2: блок-владеющий заголовок (если/пока) со сломанным условием → ровно 1;
// тело INDENT…DEDENT поглощается структурно (было 4 — multiline bleed). FR-002, SC-002.
func TestCascadeBlockBleedSingleDiagnostic(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"если-вернуть", "если вернуть:\n    печать(1)\n",
			"Ошибка в строке 1, колонка 6:\nнеожиданный элемент 'вернуть'"},
		{"пока-вернуть", "пока вернуть:\n    печать(1)\n",
			"Ошибка в строке 1, колонка 6:\nнеожиданный элемент 'вернуть'"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, el := parseProgramSrc(t, c.src)
			assertDiagnostics(t, el, c.want)
		})
	}
}

// C-REC-3: N независимых ошибок на N РАЗНЫХ строках НЕ схлопываются (анти-over-
// suppress). Контроль: значение⏎{ → ровно 2. FR-003, SC-003. (Дублирует проверку
// TestMultipleIndependentErrors через assertDiagnostics — фиксирует, что фикс DX1
// не превратил его в 1.)
func TestIndependentErrorsNotCollapsed(t *testing.T) {
	_, el := parseProgramSrc(t, "значение\n{\n")
	assertDiagnostics(t, el,
		"Ошибка в строке 1, колонка 1:\nнеожиданный элемент 'значение'",
		"Ошибка в строке 2, колонка 1:\nнеожиданный элемент '{'")
}

// Орфан-отступ после НЕ-блок-владеющей сломки (пусть не открывает блок): сиротский
// индентный блок поглощается как ОДНА сломанная конструкция (FR-001), без мини-
// каскада «увеличение отступа»+«конец блока». Итог: ровно 2 диагностики —
// (1) сломка 'если', (2) осиротевший блок. Не схлопывается в 1 (это ДВЕ разные
// проблемы: ключевое слово в выражении И неуместный отступ), но и не каскадит до 3.
func TestOrphanIndentAfterNonBlockBreak(t *testing.T) {
	_, el := parseProgramSrc(t, "пусть x = если\n    печать(1)\n")
	assertDiagnostics(t, el,
		"Ошибка в строке 1, колонка 11:\nнеожиданный элемент 'если'",
		"Ошибка в строке 2, колонка 1:\nнеожиданный элемент 'увеличение отступа'")
}

// C-REC-6: sync-lead ведущее ключевое слово вместо шага в блоке процесса.
// ОХРАНЯЕМЫЙ САЙТ — parse_decl.go (process-body non-step, ветка `else { bad :=
// p.advance(); p.error(...) }`): consume-before-error, как в parsePrimary, но это
// ОТДЕЛЬНАЯ правка — мутация ТОЛЬКО parse_expr.go этот замок НЕ ломает (нужна и
// правка parse_decl.go). Даёт ровно 1 (было 2). Не-sync-lead не-шаг и так давал 1.
// NB: field-block (поля:) и metric-window (последние) каскадят ОБЩИМ образом для
// ЛЮБОГО плохого токена (мутпроба: если≡123≡+≡)) — это пре-существующая decl-line
// recovery, НЕ sync-lead-инвариант DX1; вне scope (см. docs/diagnostics-model.md).
func TestProcessBlockNonStepSyncLeadSingleDiagnostic(t *testing.T) {
	src := "процесс п:\n    шаг а:\n        исполнитель: \"x\"\n    если\n"
	_, el := parseProgramSrc(t, src)
	assertDiagnostics(t, el,
		"Ошибка в строке 4, колонка 5:\nнеожиданный элемент 'если'")
}
