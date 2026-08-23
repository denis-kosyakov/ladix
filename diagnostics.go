package ladix

import (
	goerrors "errors"

	lerrors "github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/ir"
)

// toDiagnostics разворачивает ошибку фронтенда в список диагностик IR.
// *errors.ErrorList распаковывается поэлементно; одиночная ошибка даёт одну
// диагностику. Порядок сохраняется (порядок обнаружения = порядок исходника).
func toDiagnostics(err error) []ir.Diagnostic {
	if err == nil {
		return nil
	}
	var list *lerrors.ErrorList
	if goerrors.As(err, &list) {
		out := make([]ir.Diagnostic, 0, list.Len())
		for _, e := range list.Errors() {
			out = append(out, toDiagnostic(e))
		}
		return out
	}
	return []ir.Diagnostic{toDiagnostic(err)}
}

// toDiagnostic конвертирует одну ошибку фронтенда в ir.Diagnostic.
//
// Message берётся из поля Msg — ОПИСАНИЕ БЕЗ двухстрочного заголовка §13
// («Ошибка в строке N, колонка M:»): позиция едет отдельным полем Pos, а текст
// остаётся ДОСЛОВНЫМ текстом SPEC §13. Переформулирование запрещено (FR-007,
// Конституция VIII).
//
// Stage выводится из типа ошибки: LexError → lexical, ParseError → syntax,
// СемантическаяОшибка → semantic. Ошибка неизвестного типа (в норме недостижимо:
// Compile использует только эти три слоя) размечается как semantic с текстом
// Error() — тише, чем паника, и без потери сообщения.
func toDiagnostic(err error) ir.Diagnostic {
	var lex lerrors.LexError
	if goerrors.As(err, &lex) {
		return ir.Diagnostic{
			Severity: ir.SeverityError,
			Stage:    ir.StageLexical,
			Message:  lex.Msg,
			Pos:      toPos(lex.Pos),
		}
	}
	var parse lerrors.ParseError
	if goerrors.As(err, &parse) {
		return ir.Diagnostic{
			Severity: ir.SeverityError,
			Stage:    ir.StageSyntax,
			Message:  parse.Msg,
			Pos:      toPos(parse.Pos),
		}
	}
	var sem lerrors.СемантическаяОшибка
	if goerrors.As(err, &sem) {
		return ir.Diagnostic{
			Severity: ir.SeverityError,
			Stage:    ir.StageSemantic,
			Message:  sem.Msg,
			Pos:      toPos(sem.Pos),
		}
	}
	// Ошибка, уже несущая позицию, но иного типа (Расположенная).
	var located lerrors.Расположенная
	if goerrors.As(err, &located) {
		return ir.Diagnostic{
			Severity: ir.SeverityError,
			Stage:    ir.StageSemantic,
			Message:  err.Error(),
			Pos:      toPos(located.Позиция()),
		}
	}
	return ir.Diagnostic{
		Severity: ir.SeverityError,
		Stage:    ir.StageSemantic,
		Message:  err.Error(),
	}
}

// toPos переносит позицию покомпонентно (ir.Position — собственный тип пакета ir).
func toPos(p lerrors.Position) ir.Position {
	return ir.Position{Line: p.Line, Col: p.Col}
}

// hasErrors сообщает, есть ли среди диагностик хоть одна уровня error.
// Инвариант фасада: program != nil ⟺ hasErrors(diags) == false. Формулировка
// устойчива к будущему появлению warning/info — они program не обнуляют.
func hasErrors(diags []ir.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == ir.SeverityError {
			return true
		}
	}
	return false
}
