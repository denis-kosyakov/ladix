package ladix

// Задача 4.3 — Замок «Compile не ограничивает» (openspec/changes/
// 030-public-metrics-evaluator: design.md/spec.md, потолок сложности
// выражений 100/10000 — исключительно свойство ИСПОЛНИТЕЛЯ metrics.Evaluate,
// НЕ фронтенда ladix.Compile). Живёт в корневом пакете (не в metrics/),
// потому что проверяет поведение ladix.Compile — функции ЭТОГО пакета;
// класть замок в metrics/ означало бы тестировать чужой публичный API
// косвенно и разрывать локальность теста с проверяемым кодом.
//
// Мутпроба (проведена вручную, мутация НЕ коммитилась, дерево возвращено в
// исходное состояние): временно перенесли проверку потолка сложности из
// metrics в путь Compile — в ladix.go после semErr-проверки добавили ровно
// такую же формулу глубины (astDepth-эквивалент, порог >100) над Where каждой
// ir.Metric из lowerProgram(prog).Metrics и возврат диагностики при
// превышении. Замок покраснел: TestCompileNotBoundedByComplexityCeiling
// свалился с "program == nil" при 300 уровнях "не" — Compile начал
// отклонять то, что обязан пропускать. Мутация отменена (git diff --stat
// ladix.go пуст).
//
// (Эквивалентная мутация выбрана, а не буквальный "вызов checkComplexity из
// metrics в ladix.go": checkComplexity — метод неэкспортированного типа
// template пакета metrics, недостижим из ladix.go без экспорта наружу;
// экспорт нарушил бы Д-1 «наружу не текут типы AST/value». Инлайн той же
// формулы поверх lowerProgram(prog) — технически эквивалентная мутация,
// проверяющая ровно то же: попадание потолка в путь Compile.)

import (
	"strings"
	"testing"
)

// astDepthOfNots — глубина цепочки N унарных "не" над листом (корень = N+1,
// N=0 → лист, глубина 1). Используется только для построения ожидаемой
// канонической строки в этом тесте — не переиспользует и не дублирует
// продакшн-формулу сложности (та живёт исключительно в metrics/complexity.go).
func canonicalNotChain(n int, leaf string) string {
	// Каноническая печать UnaryExpr — "(не" + canon(operand) + ")" (internal/
	// ast/canon.go): без пробела между "не" и операндом.
	return strings.Repeat("(не", n) + leaf + strings.Repeat(")", n)
}

// TestCompileNotBoundedByComplexityCeiling — spec.md: потолок сложности
// (глубина > 100 ИЛИ узлов > 10000) применяется ТОЛЬКО в исполнителе
// (metrics.Evaluate); Compile обязан компилировать программу с выражением
// глубиной СИЛЬНО больше 100 без каких-либо диагностик потолка (собственно
// без диагностик вообще — источник синтаксически и семантически валиден).
//
// "не не не … истина", 300 повторов "не": глубина выражения — 301
// (300 UnaryExpr + лист BoolLit), втрое больше предела 100 из spec.md.
func TestCompileNotBoundedByComplexityCeiling(t *testing.T) {
	const depth = 300
	where := strings.Repeat("не ", depth) + "истина"

	source := "источник заказы:\n" +
		"    файл: \"data/sales.json\"\n\n" +
		"метрика выручка:\n" +
		"    источник: заказы\n" +
		"    где:      " + where + "\n" +
		"    агрегат:  сумма(сумма_заказа)\n"

	program, diags, err := Compile(source)
	if err != nil {
		t.Fatalf("Compile вернул err для синтаксически/семантически валидного исходника (глубина where=301): %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("Compile вернул диагностики для валидного исходника с глубоким where (потолок сложности не должен применяться в Compile): %+v", diags)
	}
	if program == nil {
		t.Fatalf("program == nil — Compile отклонил валидный исходник с глубоким where; потолок сложности исполнителя протёк во фронтенд")
	}
	if len(program.Metrics) != 1 {
		t.Fatalf("ожидалась ровно 1 метрика в IR, получено %d: %+v", len(program.Metrics), program.Metrics)
	}

	// Каноническая печать BoolLit — strconv.FormatBool (internal/ast/canon.go):
	// "true", не исходная лексема "истина".
	wantWhere := canonicalNotChain(depth, "true")
	gotWhere := program.Metrics[0].Where
	if gotWhere != wantWhere {
		t.Errorf("каноническая строка Where не совпала:\nхотим  %q\nполучили %q", wantWhere, gotWhere)
	}
}
