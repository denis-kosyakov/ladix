package parser

import "testing"

// T048 (SC-001): сквозная проверка набора примеров.
//
// Набор parse-clean обязан давать НОЛЬ синтаксических ошибок: императивное ядро
// (подмножество B) + онбординг.ladix (процессы парсятся с 005, §PM-3) +
// выручка.ladix (триггер `когда` парсится с 007a, шов A; §TR-10.5 п.5).

func TestExamplesParseCleanSet(t *testing.T) {
	clean := []string{
		"hello.ladix", "арифметика.ladix",
		"условие.ladix", "цикл.ladix",
		"функция.ladix", "факториал.ladix",
		"ошибка.ladix",    // синтаксически валиден; дефект (деление на ноль) — рантайм
		"онбординг.ladix", // процессы парсятся с 005 (§PM-3); рантайм-граница — 'запустить процесс' (§PM-5)
		"выручка.ladix",   // триггер `когда метрика … < …:` парсится с 007a (шов A, §TR-1)
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
