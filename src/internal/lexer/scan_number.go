package lexer

import (
	"strconv"
	"unicode"
)

// scanNumber разбирает INT / FLOAT / DURATION (data-model §5.1, C-4).
//
// Числовая лексема несётся КАК СТРОКА без конверсии в int64 и без проверки
// диапазона (guardrail 1): диапазон проверит парсер. `_` допустим только между
// цифрами; дробная часть образуется только при цифре сразу после `.`; суффикс
// длительности читается жадным буквенным run'ом ТОЛЬКО за ЦЕЛЫМ числом.
func (l *Lexer) scanNumber() {
	pos := l.mark()
	start := l.pos

	// Целая часть: прогон [цифра|`_`].
	l.consumeDigitRun()
	intSrc := string(l.src[start:l.pos])

	// Дробная часть: `.` и сразу цифра → FLOAT (суффикс за дробным НЕ читается).
	if l.peek() == '.' && isDigit(l.peekAt(1)) {
		l.advance() // `.`
		fracStart := l.pos
		l.consumeDigitRun()
		fracSrc := string(l.src[fracStart:l.pos])
		lexeme := string(l.src[start:l.pos])
		normInt, okInt := normalizeDigits(intSrc)
		normFrac, okFrac := normalizeDigits(fracSrc)
		if !okInt || !okFrac {
			l.addError(pos, msgBadNumber(lexeme)) // L-8
			return
		}
		f, err := strconv.ParseFloat(normInt+"."+normFrac, 64)
		if err != nil {
			l.addError(pos, msgBadNumber(lexeme)) // L-8 (страховка)
			return
		}
		l.emit(FLOAT, lexeme, pos, f)
		return
	}

	// Целое: проверка формы.
	normInt, okInt := normalizeDigits(intSrc)
	if !okInt {
		l.addError(pos, msgBadNumber(intSrc)) // L-8
		return
	}

	// Суффикс длительности: слитный буквенный run сразу за ЦЕЛЫМ числом.
	if unicode.IsLetter(l.peek()) {
		runStart := l.pos
		for !l.eof() && isIdentContinue(l.peek()) {
			l.advance()
		}
		suffix := string(l.src[runStart:l.pos])
		lexeme := string(l.src[start:l.pos])
		if durationUnits[suffix] {
			l.emit(DURATION, lexeme, pos, DurationValue{Amount: normInt, Unit: suffix})
			return
		}
		l.addError(pos, msgUnknownDurationSuffix(lexeme)) // L-9: run целиком
		return
	}

	l.emit(INT, normInt, pos, nil)
}

// consumeDigitRun поглощает максимальный прогон из цифр и `_`.
func (l *Lexer) consumeDigitRun() {
	for !l.eof() {
		r := l.peek()
		if isDigit(r) || r == '_' {
			l.advance()
		} else {
			break
		}
	}
}

// normalizeDigits убирает `_` и проверяет форму: `_` только между цифрами
// (нет ведущего/хвостового `_` и нет `__`). Возвращает нормализованную строку
// цифр и признак корректности формы. Диапазон значения НЕ проверяется (guardrail 1).
func normalizeDigits(run string) (string, bool) {
	if run == "" {
		return "", false
	}
	rs := []rune(run)
	out := make([]rune, 0, len(rs))
	for i, r := range rs {
		if r == '_' {
			if i == 0 || i == len(rs)-1 {
				return "", false
			}
			if !isDigit(rs[i-1]) || !isDigit(rs[i+1]) {
				return "", false
			}
			continue
		}
		if !isDigit(r) {
			return "", false
		}
		out = append(out, r)
	}
	return string(out), true
}
