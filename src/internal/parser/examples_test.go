package parser

import "testing"

// T048 (SC-001): сквозная проверка набора примеров.
//
// Набор parse-clean обязан давать НОЛЬ синтаксических ошибок: императивное ядро
// (подмножество B) + онбординг.ladix (процессы парсятся с 005, §PM-3).
// Декларативная выручка.ladix в набор НЕ входит: её триггер (когда) вне scope
// 005 → ожидаемый «неожиданный токен» (D-6, триггеры — 007).

func TestExamplesParseCleanSet(t *testing.T) {
	clean := []string{
		"hello.ladix", "арифметика.ladix",
		"условие.ladix", "цикл.ladix",
		"функция.ladix", "факториал.ladix",
		"ошибка.ladix",    // синтаксически валиден; дефект (деление на ноль) — рантайм
		"онбординг.ladix", // процессы парсятся с 005 (§PM-3); рантайм-граница — 'запустить процесс' (§PM-5)
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

// TestDeclarativeExamplesUnexpected — примеры с отложенными декларациями:
// процесс разбор_падения(...) в выручка.ladix теперь парсится (005, §PM-3),
// падение сдвинулось ПОЗЖЕ — на триггер 'когда' (D-6, триггеры — 007).
// онбординг.ladix снят из набора: парсится целиком (TestExamplesParseCleanSet).
func TestDeclarativeExamplesUnexpected(t *testing.T) {
	cases := []struct {
		name     string
		firstMsg string
	}{
		{"выручка.ladix", "неожиданный токен 'когда'"},
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
