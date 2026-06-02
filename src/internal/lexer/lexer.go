package lexer

import (
	"unicode"

	"github.com/denis-kosyakov/ladix/internal/errors"
)

// Lexer — токенизатор Ladix. Создаётся явно (New); пакет-уровневого изменяемого
// состояния нет (конституция V, FR-029). Исходник хранится как нормализованный
// срез РУН (BOM отброшен, \r\n и одиночный \r сведены к \n — FR-011), что даёт
// прямой подсчёт колонок в рунах (конституция IV).
type Lexer struct {
	src  []rune
	pos  int // индекс следующей необработанной руны
	line int // текущая строка, 1-based
	col  int // текущая колонка в рунах, 1-based (позиция следующей руны)

	tokens []Token
	errs   *errors.ErrorList

	indentStack  []int // уровни ведущих пробелов; инициализируется [0]
	bracketDepth int   // глубина незакрытых ()/[]/{}

	lineHasTokens  bool // в текущей логической строке уже эмитился реальный токен
	suppressErrors bool // panic-mode: подавлять регистрацию новых ошибок до синхронизации
}

// New создаёт лексер для исходного текста src.
func New(src string) *Lexer {
	return &Lexer{
		src:         normalize(src),
		pos:         0,
		line:        1,
		col:         1,
		errs:        errors.NewErrorList(),
		indentStack: []int{0},
	}
}

// Tokenize выполняет полный однопроходный скан и возвращает поток токенов
// (всегда завершается ровно одним EOF) и агрегат собранных лексических ошибок.
func (l *Lexer) Tokenize() ([]Token, *errors.ErrorList) {
	l.processLineStart()
	for !l.eof() {
		r := l.peek()
		switch {
		case r == '\n':
			l.handleNewline()
		case r == ' ':
			l.advance() // пробел между токенами незначим
		case r == '\t':
			l.addError(l.mark(), msgTabForbidden) // L-2: таб между токенами
			l.advance()
		case r == '#':
			l.skipComment() // комментарий до конца строки прозрачен (FR-014)
		case r == '"':
			l.scanString()
		case isDigit(r):
			l.scanNumber()
		case isIdentStart(r):
			l.scanIdent()
		default:
			l.scanOperator()
		}
	}
	l.handleEOF()
	return l.tokens, l.errs
}

// normalize отбрасывает ведущий BOM и нормализует \r\n и одиночный \r → \n
// (FR-011). После неё во входе нет \r.
func normalize(src string) []rune {
	in := []rune(src)
	if len(in) > 0 && in[0] == '\uFEFF' {
		in = in[1:]
	}
	out := make([]rune, 0, len(in))
	for i := 0; i < len(in); i++ {
		if in[i] == '\r' {
			out = append(out, '\n')
			if i+1 < len(in) && in[i+1] == '\n' {
				i++ // поглотить \n из пары \r\n
			}
			continue
		}
		out = append(out, in[i])
	}
	return out
}

func (l *Lexer) eof() bool { return l.pos >= len(l.src) }

// peek возвращает текущую руну или 0 на конце ввода.
func (l *Lexer) peek() rune {
	if l.eof() {
		return 0
	}
	return l.src[l.pos]
}

// peekAt возвращает руну со смещением o от текущей позиции или 0 за границами.
func (l *Lexer) peekAt(o int) rune {
	i := l.pos + o
	if i < 0 || i >= len(l.src) {
		return 0
	}
	return l.src[i]
}

// mark — текущая позиция (1-based, руны).
func (l *Lexer) mark() errors.Position {
	return errors.Position{Line: l.line, Col: l.col}
}

// advance поглощает одну руну, поддерживая позицию. После \n: line++, col=1.
func (l *Lexer) advance() rune {
	r := l.src[l.pos]
	l.pos++
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

// emit добавляет токен в поток. Реальные (не виртуальные) токены помечают
// логическую строку как непустую — это управляет эмиссией NEWLINE.
func (l *Lexer) emit(t TokenType, lexeme string, pos errors.Position, value any) {
	switch t {
	case NEWLINE, INDENT, DEDENT, EOF:
		// виртуальные токены не делают строку «непустой»
	default:
		l.lineHasTokens = true
	}
	l.tokens = append(l.tokens, Token{Type: t, Lexeme: lexeme, Pos: pos, Value: value})
}

// skipComment поглощает `#` и всё до конца строки, НЕ трогая сам \n.
func (l *Lexer) skipComment() {
	for !l.eof() && l.peek() != '\n' {
		l.advance()
	}
}

// handleNewline обрабатывает перевод строки. На верхнем уровне (вне скобок)
// эмитит NEWLINE для непустой логической строки и запускает обработку начала
// следующей строки. Внутри незакрытых скобок перевод прозрачен (C-1.2), но всё
// равно служит точкой синхронизации panic-mode (A4).
func (l *Lexer) handleNewline() {
	pos := l.mark()
	if l.bracketDepth == 0 {
		if l.lineHasTokens {
			l.emit(NEWLINE, "", pos, nil)
		}
		l.advance() // поглотить \n
		l.suppressErrors = false
		l.lineHasTokens = false
		l.processLineStart()
		return
	}
	l.advance()
	l.suppressErrors = false
}

// handleEOF завершает поток: при необходимости синтетический NEWLINE, затем
// закрытие всех уровней отступа серией DEDENT, затем ровно один EOF (FR-023).
// Все три вида несут одну позицию — сразу после последней руны (D-R11).
func (l *Lexer) handleEOF() {
	pos := l.mark()
	if l.bracketDepth == 0 && l.lineHasTokens {
		l.emit(NEWLINE, "", pos, nil)
	}
	for len(l.indentStack) > 1 {
		l.indentStack = l.indentStack[:len(l.indentStack)-1]
		l.emit(DEDENT, "", pos, nil)
	}
	l.emit(EOF, "", pos, nil)
}

// scanOperator разбирает операторы (жадным максимальным совпадением — FR-019),
// разделители и скобки. Любой нераспознанный символ — L-10.
func (l *Lexer) scanOperator() {
	pos := l.mark()
	switch l.peek() {
	case '+':
		l.advance()
		l.emit(PLUS, "+", pos, nil)
	case '-':
		l.advance()
		l.emit(MINUS, "-", pos, nil)
	case '*':
		l.advance()
		l.emit(STAR, "*", pos, nil)
	case '%':
		l.advance()
		l.emit(PERCENT, "%", pos, nil)
	case '/':
		l.advance()
		if l.peek() == '/' {
			l.advance()
			l.emit(SLASH_SLASH, "//", pos, nil)
		} else {
			l.emit(SLASH, "/", pos, nil)
		}
	case '=':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			l.emit(EQ, "==", pos, nil)
		} else {
			l.emit(ASSIGN, "=", pos, nil)
		}
	case '!':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			l.emit(NEQ, "!=", pos, nil)
		} else {
			l.addError(pos, msgUnexpectedChar('!')) // L-10: одиночный `!` недопустим
		}
	case '<':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			l.emit(LE, "<=", pos, nil)
		} else {
			l.emit(LT, "<", pos, nil)
		}
	case '>':
		l.advance()
		if l.peek() == '=' {
			l.advance()
			l.emit(GE, ">=", pos, nil)
		} else {
			l.emit(GT, ">", pos, nil)
		}
	case '.':
		l.advance()
		l.emit(DOT, ".", pos, nil)
	case ',':
		l.advance()
		l.emit(COMMA, ",", pos, nil)
	case ':':
		l.advance()
		l.emit(COLON, ":", pos, nil)
	case '(':
		l.advance()
		l.bracketDepth++
		l.emit(LPAREN, "(", pos, nil)
	case ')':
		l.advance()
		l.closeBracket()
		l.emit(RPAREN, ")", pos, nil)
	case '[':
		l.advance()
		l.bracketDepth++
		l.emit(LBRACKET, "[", pos, nil)
	case ']':
		l.advance()
		l.closeBracket()
		l.emit(RBRACKET, "]", pos, nil)
	case '{':
		l.advance()
		l.bracketDepth++
		l.emit(LBRACE, "{", pos, nil)
	case '}':
		l.advance()
		l.closeBracket()
		l.emit(RBRACE, "}", pos, nil)
	default:
		r := l.advance()
		l.addError(pos, msgUnexpectedChar(r)) // L-10
	}
}

// closeBracket уменьшает глубину скобок, не опускаясь ниже нуля. Несбалансированные
// скобки лексер НЕ диагностирует — это задача парсера (guardrail 8, C-6).
func (l *Lexer) closeBracket() {
	if l.bracketDepth > 0 {
		l.bracketDepth--
	}
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isIdentStart(r rune) bool { return r == '_' || unicode.IsLetter(r) }

func isIdentContinue(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
