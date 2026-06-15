package lexer

import (
	"fmt"

	"github.com/denis-kosyakov/ladix/internal/errors"
)

// TokenType — вид токена. Всего 68 ЭМИТИРУЕМЫХ видов (data-model §5; +1 за
// 010-A1 KW_TYPE):
// 6 литералов + IDENT + 35 ключевых слов + 14 операторов + 8 разделителей/скобок
// + 4 виртуальных. INVALID — внутренний нулевой сентинел: не эмитится, делает
// нулевое значение Token безопасно отличимым.
type TokenType int

const (
	INVALID TokenType = iota

	// --- Литералы (6) ---
	INT      // 42, 5_000 — несёт нормализованную цифровую строку (Lexeme), без int64
	FLOAT    // 3.14 — несёт предразобранное float64 (Value)
	STRING   // "привет" — несёт развёрнутую строку (Value)
	BOOL     // истина / ложь — несёт bool (Value)
	NONE     // пусто — без значения
	DURATION // 3дн, 1_000сек — несёт DurationValue (Value)

	// --- Идентификатор (1) ---
	IDENT

	// --- Ключевые слова (35) ---
	KW_LET       // пусть
	KW_FUNC      // функция
	KW_RETURN    // вернуть
	KW_IF        // если
	KW_ELSE      // иначе
	KW_WHILE     // пока
	KW_FOR       // для
	KW_BREAK     // прервать
	KW_CONTINUE  // продолжить
	KW_AND       // и
	KW_OR        // или
	KW_NOT       // не
	KW_SOURCE    // источник
	KW_FILE      // файл
	KW_METRIC    // метрика
	KW_WHERE     // где
	KW_AGGREGATE // агрегат
	KW_PERIOD    // период
	KW_BY_DATE   // по_дате
	KW_PROCESS   // процесс
	KW_STEP      // шаг
	KW_ASSIGNEE  // исполнитель
	KW_DEADLINE  // срок
	KW_AFTER     // после
	KW_SET       // присвоить
	KW_CALL      // вызвать
	KW_NOTIFY    // уведомить
	KW_WHEN      // когда
	KW_EVENT     // событие
	KW_VALUE     // значение
	KW_SCHEDULE  // расписание
	KW_EVERY     // каждые
	KW_IN        // в
	KW_RUN       // запустить
	KW_TYPE      // тип — 010-A1 §SC-D-RESERVE: атрибут источника; в выражении НЕ начинает primary

	// --- Операторы (14) ---
	PLUS        // +
	MINUS       // -
	STAR        // *
	SLASH       // /
	SLASH_SLASH // //
	PERCENT     // %
	ASSIGN      // =
	EQ          // ==
	NEQ         // !=
	LT          // <
	LE          // <=
	GT          // >
	GE          // >=
	DOT         // .

	// --- Разделители и скобки (8) ---
	COMMA    // ,
	COLON    // :
	LPAREN   // (
	RPAREN   // )
	LBRACKET // [
	RBRACKET // ]
	LBRACE   // {
	RBRACE   // }

	// --- Виртуальные (4) ---
	NEWLINE // конец непустой логической строки
	INDENT  // вход на новый уровень отступа
	DEDENT  // снятие одного уровня отступа
	EOF     // конец ввода; ровно один, всегда последний
)

var tokenNames = map[TokenType]string{
	INVALID: "INVALID",

	INT: "INT", FLOAT: "FLOAT", STRING: "STRING",
	BOOL: "BOOL", NONE: "NONE", DURATION: "DURATION",

	IDENT: "IDENT",

	KW_LET: "KW_LET", KW_FUNC: "KW_FUNC", KW_RETURN: "KW_RETURN",
	KW_IF: "KW_IF", KW_ELSE: "KW_ELSE", KW_WHILE: "KW_WHILE", KW_FOR: "KW_FOR",
	KW_BREAK: "KW_BREAK", KW_CONTINUE: "KW_CONTINUE",
	KW_AND: "KW_AND", KW_OR: "KW_OR", KW_NOT: "KW_NOT",
	KW_SOURCE: "KW_SOURCE", KW_FILE: "KW_FILE", KW_METRIC: "KW_METRIC", KW_WHERE: "KW_WHERE",
	KW_AGGREGATE: "KW_AGGREGATE", KW_PERIOD: "KW_PERIOD", KW_BY_DATE: "KW_BY_DATE",
	KW_PROCESS: "KW_PROCESS", KW_STEP: "KW_STEP", KW_ASSIGNEE: "KW_ASSIGNEE", KW_DEADLINE: "KW_DEADLINE",
	KW_AFTER: "KW_AFTER", KW_SET: "KW_SET", KW_CALL: "KW_CALL", KW_NOTIFY: "KW_NOTIFY",
	KW_WHEN: "KW_WHEN", KW_EVENT: "KW_EVENT", KW_VALUE: "KW_VALUE", KW_SCHEDULE: "KW_SCHEDULE",
	KW_EVERY: "KW_EVERY", KW_IN: "KW_IN", KW_RUN: "KW_RUN", KW_TYPE: "KW_TYPE",

	PLUS: "PLUS", MINUS: "MINUS", STAR: "STAR", SLASH: "SLASH",
	SLASH_SLASH: "SLASH_SLASH", PERCENT: "PERCENT", ASSIGN: "ASSIGN",
	EQ: "EQ", NEQ: "NEQ", LT: "LT", LE: "LE", GT: "GT", GE: "GE", DOT: "DOT",

	COMMA: "COMMA", COLON: "COLON",
	LPAREN: "LPAREN", RPAREN: "RPAREN",
	LBRACKET: "LBRACKET", RBRACKET: "RBRACKET",
	LBRACE: "LBRACE", RBRACE: "RBRACE",

	NEWLINE: "NEWLINE", INDENT: "INDENT", DEDENT: "DEDENT", EOF: "EOF",
}

// String даёт имя вида токена для диагностики и табличных тестов.
func (t TokenType) String() string {
	if name, ok := tokenNames[t]; ok {
		return name
	}
	return fmt.Sprintf("TokenType(%d)", int(t))
}

// DurationValue — предразобранное значение литерала длительности: единица плюс
// нормализованная цифровая строка (диапазон проверит парсер — guardrail 1).
type DurationValue struct {
	Amount string // нормализованная цифровая строка (без '_'), например "1000"
	Unit   string // одна из: сек, мин, час, дн, нед, мес
}

// Token — единица потока.
//
// Контракт поля Value (D-R4 / contracts/token-stream.md C-3):
//   - FLOAT    → float64;
//   - STRING   → string (escape раскрыты);
//   - BOOL     → bool;
//   - DURATION → DurationValue;
//   - INT      → nil (нормализованная цифровая строка лежит в Lexeme);
//   - NONE и прочие токены → nil.
//
// Lexeme для INT нормализован (цифры без '_'); для остальных — исходный текст
// лексемы. Виртуальные токены (NEWLINE/INDENT/DEDENT/EOF) имеют пустую лексему.
type Token struct {
	Type   TokenType
	Lexeme string
	Pos    errors.Position
	Value  any
}
