package lexer

import "strings"

// scanString разбирает строковый литерал в двойных кавычках с раскрытием escape
// `\n \t \\ \"` (FR-018). `#` внутри строки — обычный символ (FR-014).
//
// Диагностики: L-5 (незакрытый до EOF), L-6 (перевод строки внутри строки),
// L-7 (неизвестная escape), L-2 (таб внутри строки — FR-020). При любой ошибке
// внутри строки проблемная лексема целиком пропускается (токен не эмитится —
// panic-mode, вариант a); скан остатка строки продолжается обычным циклом.
func (l *Lexer) scanString() {
	openPos := l.mark()
	start := l.pos
	l.advance() // открывающая `"`

	var sb strings.Builder
	hadError := false

	for {
		if l.eof() {
			l.addError(openPos, msgUnterminatedStr) // L-5
			return
		}
		switch r := l.peek(); r {
		case '"':
			l.advance() // закрывающая `"`
			if !hadError {
				l.emit(STRING, string(l.src[start:l.pos]), openPos, sb.String())
			}
			return
		case '\n':
			l.addError(openPos, msgNewlineInStr) // L-6: `\n` НЕ поглощаем
			return
		case '\t':
			l.addError(l.mark(), msgTabForbidden) // L-2: таб внутри строки
			hadError = true
			l.advance()
		case '\\':
			escPos := l.mark()
			l.advance() // `\`
			if l.eof() {
				l.addError(openPos, msgUnterminatedStr) // L-5: `\` у самого EOF
				return
			}
			switch e := l.peek(); e {
			case 'n':
				sb.WriteRune('\n')
				l.advance()
			case 't':
				sb.WriteRune('\t')
				l.advance()
			case '\\':
				sb.WriteRune('\\')
				l.advance()
			case '"':
				sb.WriteRune('"')
				l.advance()
			default:
				l.addError(escPos, msgUnknownEscape("\\"+string(e))) // L-7
				hadError = true
				l.advance()
			}
		default:
			sb.WriteRune(r)
			l.advance()
		}
	}
}
