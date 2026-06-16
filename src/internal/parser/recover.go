package parser

import "github.com/denis-kosyakov/ladix/internal/lexer"

// Panic-mode восстановление (D6/D-R8, guardrail 10; поверх лексерного panic-mode).
//
// Контроллер ошибок — два метода в parser.go:
//   - error()      регистрирует ошибку, ставит suppress и СИНХРОНИЗИРУЕТСЯ
//                  (SE-EXPECTED/SE-UNEXPECTED — парсер «потерян», нужно пропустить);
//   - errorLocal() регистрирует ошибку и ставит suppress, НО не синхронизируется
//                  (SE-CHAIN/SE-INT-RANGE/SE-ASSIGN-TARGET/SE-EMPTY-BLOCK/SE-NESTED-FN —
//                  парсер восстанавливается локально и продолжает штатно).
//
// suppress снимается на границе оператора (старт итерации top-level/блочного цикла),
// поэтому накапливается не более одной диагностики на логический оператор —
// фантомный каскад исключён. Каскад от ведущего sync-lead KW в позиции выражения на
// одной/смежной строке (sync-lead D-R8, напр. «пусть x = если») устранён в DX1
// (фича 012): parsePrimary потребляет такой токен ДО error() (parse_expr.go), а
// осиротевший индентный блок поглощается skipOrphanIndentedBody — ровно одна
// диагностика на сломанную конструкцию (contracts/parser-recovery.md). Независимые
// ошибки на РАЗНЫХ строках по-прежнему дают N диагностик (анти-over-suppress).
// Прогресс цикла гарантирован backstop-ом (если разбор оператора не сдвинул курсор,
// цикл потребляет один токен).

// isSyncLead сообщает, является ли вид токена ВЕДУЩИМ синхро-токеном. На таких
// токенах synchronize ОСТАНАВЛИВАЕТСЯ, НЕ потребляя их (разбор начнётся с них):
// ведущие statements, функция, step-action, верхнеуровневые декларации. После 004
// источник/метрика, а после 005 процесс — полноценные декларации (§SM-3, §PM-3),
// парсятся штатно; SE-UNEXPECTED дают лишь ещё отложенные когда/значение
// (и { }) — contracts/syntax-errors.md.
func isSyncLead(t lexer.TokenType) bool {
	switch t {
	case lexer.KW_LET, lexer.KW_IF, lexer.KW_WHILE, lexer.KW_FOR,
		lexer.KW_RETURN, lexer.KW_BREAK, lexer.KW_CONTINUE,
		lexer.KW_FUNC,
		lexer.KW_SET, lexer.KW_CALL, lexer.KW_NOTIFY,
		lexer.KW_SOURCE, lexer.KW_METRIC, lexer.KW_PROCESS, lexer.KW_WHEN:
		return true
	default:
		return false
	}
}

// skipOrphanIndentedBody потребляет осиротевший индентный блок: тело до парного
// DEDENT включительно (баланс по глубине вложенности). Вызывается, когда INDENT
// встретился в позиции выражения и НИ ОДНА конструкция его не открыла (parsePrimary
// default, DX1): весь блок трактуется как ОДНА сломанная конструкция — одна
// диагностика, без каскада «увеличение отступа»+«конец блока» (FR-001, фича 012).
// Ведущий INDENT уже потреблён вызывающим (глубина стартует с 1). На EOF
// останавливается (баланс INDENT/DEDENT гарантирует лексер, но EOF — страховка).
func (p *Parser) skipOrphanIndentedBody() {
	depth := 1
	for depth > 0 && !p.check(lexer.EOF) {
		switch p.advance().Type {
		case lexer.INDENT:
			depth++
		case lexer.DEDENT:
			depth--
		}
	}
}

// synchronize отбрасывает токены до ближайшего синхро-токена. Структурные
// NEWLINE/DEDENT ПОТРЕБЛЯЮТСЯ (разбор продолжается после них); ведущие ключевые
// слова и EOF НЕ потребляются (FR-026, D-R8).
func (p *Parser) synchronize() {
	for {
		switch t := p.peek().Type; {
		case t == lexer.EOF:
			return
		case t == lexer.NEWLINE || t == lexer.DEDENT:
			p.advance()
			return
		case isSyncLead(t):
			return
		default:
			p.advance()
		}
	}
}
