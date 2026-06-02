package lexer

import "testing"

// T039: вычитка границ (guardrail 8) как исполняемые проверки — лексер НЕ
// диагностирует незакрытые скобки и НЕ резолвит семантику.

func TestUnclosedBracketsNotDiagnosed(t *testing.T) {
	// Незакрытые скобки/блоки — забота парсера (C-6): лексер ошибок не даёт,
	// отдаёт структурные токены и EOF.
	for _, src := range []string{"(", "[1, 2", "{", "функция f("} {
		_, errs := lexAll(src)
		if !errs.Empty() {
			t.Errorf("%q: лексер не должен диагностировать незакрытые скобки, получено %v", src, errs.Errors())
		}
	}
}

func TestSemanticsNotResolvedToIdent(t *testing.T) {
	// Периоды, встроенные функции и поля записи — обычные IDENT (FR-010, C-6).
	idents := []string{"ежедневно", "ежемесячно", "ежегодно", "печать", "длина", "подстрока"}
	for _, w := range idents {
		t.Run(w, func(t *testing.T) {
			toks, errs := lexAll(w)
			requireNoErrors(t, errs)
			requireTypes(t, toks, IDENT, NEWLINE, EOF)
		})
	}
}

func TestFieldAccessIsIdentDotIdent(t *testing.T) {
	// r.поле — IDENT·DOT·IDENT; семантику поля лексер не резолвит.
	toks, errs := lexAll("r.поле")
	requireNoErrors(t, errs)
	requireTypes(t, toks, IDENT, DOT, IDENT, NEWLINE, EOF)
}
