package lexer

import "github.com/denis-kosyakov/ladix/internal/errors"

// processLineStart обрабатывает начало логической строки на верхнем уровне
// (вне скобок). Прозрачно пропускает пустые строки и строки только с
// комментарием (FR-022): для них INDENT/DEDENT/NEWLINE не эмитятся и отступ не
// пересчитывается. Для содержательной строки измеряет ведущие пробелы и
// применяет алгоритм INDENT/DEDENT (grammar §10.1). Табы в ведущих пробелах — L-1.
//
// Вызывается один раз перед главным циклом и затем после каждого поглощённого
// перевода строки вне скобок (handleNewline). Внутри скобок не вызывается:
// ведущие пробелы там — обычный незначимый разделитель.
func (l *Lexer) processLineStart() {
	for {
		spaces := 0
		sawTab := false
		for !l.eof() {
			switch l.peek() {
			case ' ':
				spaces++
				l.advance()
				continue
			case '\t':
				if !sawTab {
					l.addError(l.mark(), msgTabInIndent) // L-1: позиция первого таба
					sawTab = true
				}
				l.advance()
				continue
			}
			break
		}

		if l.eof() {
			return // закрытие уровней и EOF — на handleEOF
		}

		switch l.peek() {
		case '\n':
			l.advance() // пустая строка прозрачна
			l.suppressErrors = false
			continue
		case '#':
			l.skipComment()
			if l.eof() {
				return
			}
			l.advance() // поглотить \n строки-комментария
			l.suppressErrors = false
			continue
		}

		l.applyIndent(spaces)
		return
	}
}

// applyIndent применяет уровень отступа cur к стеку уровней (data-model §7):
// больше вершины → INDENT; меньше → серия DEDENT до совпадения; равно → ничего.
// Отступ, не кратный 4, — L-3; возврат на отсутствующий уровень — L-4.
// INDENT/DEDENT несут позицию (строка, колонка 1).
func (l *Lexer) applyIndent(cur int) {
	pos := errors.Position{Line: l.line, Col: 1}
	if cur%4 != 0 {
		l.addError(pos, msgIndentMultiple) // L-3
	}
	top := l.indentStack[len(l.indentStack)-1]
	switch {
	case cur > top:
		l.indentStack = append(l.indentStack, cur)
		l.emit(INDENT, "", pos, nil)
	case cur < top:
		for len(l.indentStack) > 1 && l.indentStack[len(l.indentStack)-1] > cur {
			l.indentStack = l.indentStack[:len(l.indentStack)-1]
			l.emit(DEDENT, "", pos, nil)
		}
		if l.indentStack[len(l.indentStack)-1] != cur {
			l.addError(pos, msgIndentNoLevel)          // L-4
			l.indentStack = append(l.indentStack, cur) // best-effort ресинхронизация
		}
	}
}
