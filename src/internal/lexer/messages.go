package lexer

import "fmt"

// Канонические тексты лексических ошибок. Канон — docs/diagnostics-model.md §MDX
// (DX2, фича 012): бизнес-понятные формулировки scope A (де-жаргон «литерал»),
// двухстрочный формат §13 сохранён. Код берёт тексты ДОСЛОВНО из модель-дока
// (конституция VIII «дословно из docs»); перенос в SPEC §13.4 — архитектором на
// шве (FR-013). Это LexError.Msg (без заголовка «Ошибка в строке N, колонка M:»).
const (
	msgTabInIndent     = "табы в отступах запрещены, используйте пробелы"                       // L-1
	msgTabForbidden    = "табы запрещены, используйте пробелы"                                  // L-2
	msgIndentMultiple  = "отступ должен быть кратен 4 пробелам"                                 // L-3
	msgIndentNoLevel   = "отступ не соответствует ни одному внешнему уровню"                    // L-4
	msgUnterminatedStr = "незакрытая строка в кавычках"                                         // L-5
	msgNewlineInStr    = "незакрытая строка в кавычках (перевод строки внутри строки запрещён)" // L-6
)

// msgUnknownEscape — L-7. Параметр esc — реальная escape-последовательность
// целиком, например `\q`.
func msgUnknownEscape(esc string) string {
	return fmt.Sprintf("неизвестная escape-последовательность '%s'", esc)
}

// msgBadNumber — L-8 (битая ФОРМА числа). lex — реальная лексема, например `1__000`.
// Бизнес-формулировка (DX2): «неверная запись числа» вместо «числовой литерал».
func msgBadNumber(lex string) string {
	return fmt.Sprintf("неверная запись числа '%s'", lex)
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
