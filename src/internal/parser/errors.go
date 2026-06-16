package parser

import (
	"fmt"

	"github.com/denis-kosyakov/ladix/internal/lexer"
)

// Тексты синтаксических ошибок (contracts/syntax-errors.md). Канон — docs/
// diagnostics-model.md §MDX (DX2, фича 012): бизнес-понятные формулировки scope A
// (де-жаргон «токен»/«литерал»), коды SE-* — внутренние, двухстрочный формат §13
// сохранён. Код берёт тексты ДОСЛОВНО из модель-дока (конституция VIII «дословно из
// docs»). Перенос канона в большой SPEC §13.4 — архитектором на шве (FR-013).
// Переформулировать в обход модель-дока запрещено.
const (
	// Канон §13.4.
	msgChain    = "сравнения нельзя сцеплять, используйте 'и': 1 < x и x < 10"
	msgNestedFn = "вложенные функции не поддерживаются в v1"
	// Эталоны.
	msgEmptyBlock   = "пустой блок не допускается, добавьте хотя бы один оператор"
	msgAssignTarget = "неверная цель присваивания: слева от '=' допустима только переменная"
	// Декларативный слой 004 (§SM-9.A, дословно).
	msgSourceName = "ожидается имя источника"
	// Триггеры 007a (§TR-7.F) — `expected`-строки для msgExpected. Тексты
	// канонизированы байт-в-байт (diagnostics.md §TR-7.F):
	//   SE-TRIGGER-KIND  → "ожидалось 'метрика, событие или расписание', получено '<лексема>'"
	//   SE-EXPECT-COMPOP → "ожидалось 'оператор сравнения', получено '<лексема>'"
	//   SE-SCHEDULE-SPEC → "ожидалось 'каждые или в', получено '<лексема>'"
	msgTriggerKind  = "метрика, событие или расписание"
	msgCompOp       = "оператор сравнения"
	msgScheduleSpec = "каждые или в"
)

// msgUnknownAttr — §SM-9.A: имя атрибута вне фикс-набора (поз. имени).
func msgUnknownAttr(name string) string {
	return fmt.Sprintf("неизвестный атрибут '%s'", name)
}

// msgDuplicateAttr — §SM-9.A: повтор атрибута (поз. повтора).
func msgDuplicateAttr(name string) string {
	return fmt.Sprintf("атрибут '%s' уже задан", name)
}

// msgDuplicateField — §SC-9.A (010-A1): повтор имени поля в блоке поля:
// источника (поз. повторного объявления). Доменный текст, отличный от
// msgDuplicateAttr (атрибут vs поле).
func msgDuplicateField(name string) string {
	return fmt.Sprintf("поле '%s' уже объявлено", name)
}

// msgIntRange — SE-INT-RANGE. Бизнес-формулировка (DX2): «целое число» вместо
// «целочисленный литерал», лексема числа целиком. Канон docs/diagnostics-model.md.
func msgIntRange(lexeme string) string {
	return fmt.Sprintf("целое число '%s' вне диапазона типа Целое", lexeme)
}

// msgExpected — эталон SE-EXPECTED. expected — ожидаемая лексема/символ без
// кавычек (например ":", ")", "в", "конец строки"); got — реальный токен.
func msgExpected(expected string, got lexer.Token) string {
	return fmt.Sprintf("ожидалось '%s', получено '%s'", expected, lexemeOf(got))
}

// msgUnexpected — эталон SE-UNEXPECTED: ведущий элемент не начинает допустимую
// конструкцию. Бизнес-формулировка (DX2, фича 012): «элемент» вместо внутреннего
// «токен» — канон docs/diagnostics-model.md §MDX.
func msgUnexpected(got lexer.Token) string {
	return fmt.Sprintf("неожиданный элемент '%s'", lexemeOf(got))
}

// pseudoLexeme — описательные псевдо-лексемы виртуальных токенов (у них пустая
// лексема). Используются в текстах ошибок для читаемости.
var pseudoLexeme = map[lexer.TokenType]string{
	lexer.NEWLINE: "конец строки",
	lexer.INDENT:  "увеличение отступа",
	lexer.DEDENT:  "конец блока",
	lexer.EOF:     "конец файла",
}

// lexemeOf возвращает отображаемую лексему токена: реальную для содержательных
// токенов либо псевдо-лексему для виртуальных (NEWLINE/INDENT/DEDENT/EOF).
func lexemeOf(tok lexer.Token) string {
	if s, ok := pseudoLexeme[tok.Type]; ok {
		return s
	}
	return tok.Lexeme
}
