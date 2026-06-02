package lexer

import "github.com/denis-kosyakov/ladix/internal/errors"

// addError — единая точка регистрации лексических ошибок (контроллер panic-mode,
// вариант a — FR-025, guardrail 6).
//
// До точки синхронизации (конец логической строки: следующий нормализованный
// `\n`; внутри незакрытых скобок — физический перевод) регистрация НОВЫХ ошибок
// подавляется, чтобы не плодить фантомный каскад. Сама токенизация остатка строки
// при этом продолжается штатно (best-effort): подавляются только ОШИБКИ, токены
// валидного остатка эмитятся (C-1.6).
//
// Сброс suppressErrors происходит при поглощении `\n` (handleNewline / пустые и
// комментарные строки в processLineStart) — это и есть точка синхронизации (A4).
//
// Накопитель ограничен мягким бюджетом errors.DefaultErrorBudget (A3): по его
// достижении Add молча отбрасывает новые ошибки, но поток токенов от этого не
// зависит и всё равно завершается EOF.
func (l *Lexer) addError(pos errors.Position, msg string) {
	if l.suppressErrors {
		return
	}
	l.errs.Add(errors.LexError{Pos: pos, Msg: msg})
	l.suppressErrors = true
}
