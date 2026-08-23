package lexer

// scanIdent читает идентификатор жадно (Unicode-буква/`_`, далее буквы/цифры/`_`)
// и сверяет ПОЛНУЮ строку с таблицами (FR-006/FR-012):
//   - зарезервированное слово → L-11, токен НЕ эмитится (частный случай panic-mode);
//   - ключевое слово → его вид токена;
//   - истина/ложь → BOOL; пусто → NONE (FR-006);
//   - иначе → IDENT.
func (l *Lexer) scanIdent() {
	pos := l.mark()
	start := l.pos
	for !l.eof() && isIdentContinue(l.peek()) {
		l.advance()
	}
	word := string(l.src[start:l.pos])

	if reservedWords[word] {
		l.addError(pos, msgReservedWord(word)) // L-11: токен НЕ эмитится
		return
	}
	if tt, ok := keywords[word]; ok {
		l.emit(tt, word, pos, nil)
		return
	}
	switch word {
	case "истина":
		l.emit(BOOL, word, pos, true)
	case "ложь":
		l.emit(BOOL, word, pos, false)
	case "пусто":
		l.emit(NONE, word, pos, nil)
	default:
		l.emit(IDENT, word, pos, nil)
	}
}
