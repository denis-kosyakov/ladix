package metrics

// Валидация Options (Д-3/Д-7): дата «сегодня» обязательна и должна быть
// календарно валидной; схема полей валидируется отдельно в buildTemplate
// (validatedFieldNames) — там же, где её текст попадает в синтетическую
// программу («Инъекция»).

import (
	"fmt"

	"github.com/denis-kosyakov/ladix/internal/eval"
)

// validateOptions проверяет Options.Today (Д-3): нулевая/невалидная календарная
// дата → ErrInvalidOptions. Fields проверяются позже в buildTemplate — там же
// нужна и валидация имён/типов, и построение текста.
func validateOptions(opts Options) error {
	if !validDate(opts.Today) {
		return fmt.Errorf("%w: Options.Today — не календарно валидная дата: %+v", ErrInvalidOptions, opts.Today)
	}
	return nil
}

// validDate — календарная проверка Options.Today ТЕМ ЖЕ кодом, что и встроенная
// «дата»/коэрсия поля Дата (eval.MakeDate → makeDate, builtins_date.go): второго
// григорианского календаря в пакете нет.
func validDate(d Date) bool {
	_, ok := eval.MakeDate(int64(d.Year), int64(d.Month), int64(d.Day))
	return ok
}
