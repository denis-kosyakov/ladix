package lexer

import "fmt"

// Канонические тексты лексических ошибок — ДОСЛОВНО по contracts/lexical-errors.md
// (SPEC §13.4, FR-024, конституция VIII). Переформулировать ЗАПРЕЩЕНО. Это
// LexError.Msg (без заголовка «Ошибка в строке N, колонка M:»).
const (
	msgTabInIndent     = "табы в отступах запрещены, используйте пробелы"                       // L-1
	msgTabForbidden    = "табы запрещены, используйте пробелы"                                  // L-2
	msgIndentMultiple  = "отступ должен быть кратен 4 пробелам"                                 // L-3
	msgIndentNoLevel   = "отступ не соответствует ни одному внешнему уровню"                    // L-4
	msgUnterminatedStr = "незакрытый строковый литерал"                                         // L-5
	msgNewlineInStr    = "незакрытый строковый литерал (перевод строки внутри строки запрещён)" // L-6
)

// msgUnknownEscape — L-7. Параметр esc — реальная escape-последовательность
// целиком, например `\q`.
func msgUnknownEscape(esc string) string {
	return fmt.Sprintf("неизвестная escape-последовательность '%s'", esc)
}

// msgBadNumber — L-8 (битая ФОРМА числа). lex — реальная лексема, например `1__000`.
func msgBadNumber(lex string) string {
	return fmt.Sprintf("неверный числовой литерал '%s'", lex)
}

// msgUnknownDurationSuffix — L-9. lex — прочитанный run ЦЕЛИКОМ, например `5XYZ`.
func msgUnknownDurationSuffix(lex string) string {
	return fmt.Sprintf("'%s' — неизвестный суффикс длительности. Допустимые: сек, мин, час, дн, нед, мес.", lex)
}

// msgUnexpectedChar — L-10. Любой символ вне литералов/идентификаторов/операторов,
// включая одиночный `!`.
func msgUnexpectedChar(c rune) string {
	return fmt.Sprintf("неожиданный символ '%c'", c)
}

// msgReservedWord — L-11. word — реальное зарезервированное слово.
func msgReservedWord(word string) string {
	return fmt.Sprintf("'%s' — зарезервированное слово, появится в будущих версиях Ladix. Использование как имени не допускается.", word)
}
