package parser

import "testing"

// T048 (SC-001): сквозная проверка набора примеров.
//
// Набор parse-clean (императивное ядро, подмножество B) обязан давать НОЛЬ
// синтаксических ошибок. Декларативные выручка/онбординг в набор НЕ входят: их
// ведущие декларации (источник/процесс) вне scope B → ожидаемый «неожиданный
// токен» (guardrail 12/13).

func TestExamplesParseCleanSet(t *testing.T) {
	clean := []string{
		"hello.ladix", "арифметика.ladix",
		"условие.ladix", "цикл.ladix",
		"функция.ladix", "факториал.ladix",
		"ошибка.ladix", // синтаксически валиден; дефект (деление на ноль) — рантайм
	}
	for _, name := range clean {
		t.Run(name, func(t *testing.T) {
			prog, el, lexErrs := parseExampleFile(t, name)
			if !lexErrs.Empty() {
				t.Fatalf("%s: лексические ошибки: %v", name, lexErrs.Error())
			}
			if !el.Empty() {
				t.Fatalf("%s: синтаксические ошибки: %v", name, el.Error())
			}
			if len(prog.Items) == 0 {
				t.Errorf("%s: пустой Program.Items", name)
			}
		})
	}
}

func TestDeclarativeExamplesUnexpected(t *testing.T) {
	cases := []struct {
		name     string
		firstMsg string
	}{
		{"выручка.ladix", "неожиданный токен 'процесс'"},
		{"онбординг.ladix", "неожиданный токен 'процесс'"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, el, _ := parseExampleFile(t, c.name)
			if el.Empty() {
				t.Fatalf("%s: ожидался «неожиданный токен», ошибок нет", c.name)
			}
			if got := firstParseError(t, el).Msg; got != c.firstMsg {
				t.Errorf("%s: первая ошибка %q, хотим %q", c.name, got, c.firstMsg)
			}
		})
	}
}
