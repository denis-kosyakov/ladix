package ladix

import (
	"fmt"
	"io"
	"os"

	"github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/ir"
)

// Compile разбирает и валидирует исходник LADIX, понижая его в стабильный IR.
//
// Выполняются ТРИ стадии статической валидации: лексика, синтаксис и
// семантический проход (обход AST без исполнения программы). Исполнения не
// происходит: побочные эффекты невозможны, вывод программы глушится.
//
// Контракт возвратов:
//
//   - валидный исходник → program != nil (SchemaVersion == ir.SchemaVersion),
//     diags без записей уровня error, err == nil;
//   - исходник с пользовательскими ошибками → program == nil, err == nil,
//     diags содержит ≥1 запись Severity == ir.SeverityError с дословным русским
//     сообщением и позицией;
//   - внутренний сбой (перехваченная паника фронтенда) → err != nil.
//
// Инвариант: program != nil ⟺ среди diags нет записи уровня error.
// Пользовательская ошибка исходника НИКОГДА не маскируется под err.
//
// Compile не хранит состояния между вызовами: лексер, парсер и интерпретатор
// создаются заново на каждый вызов, поэтому вызывать её из нескольких горутин
// безопасно.
func Compile(source string) (program *ir.Program, diags []ir.Diagnostic, err error) {
	// Барьер: паника внутри фронтенда (инвариант «не должно случиться»,
	// например незнакомый узел AST в канонизаторе) становится err, а не крашем
	// процесса потребителя.
	defer func() {
		if r := recover(); r != nil {
			program, diags, err = nil, nil, fmt.Errorf("ladix: внутренний сбой компиляции: %v", r)
		}
	}()

	errs := errors.NewErrorList()

	toks, lexErrs := lexer.New(source).Tokenize()
	if lexErrs != nil && !lexErrs.Empty() {
		return nil, toDiagnostics(lexErrs), nil
	}

	prog := parser.New(toks, errs).Parse()
	if !errs.Empty() {
		return nil, toDiagnostics(errs), nil
	}

	// Семантический проход. Интерпретатор нужен ТОЛЬКО ради Analyze, поэтому
	// вывод уходит в io.Discard: программа не исполняется, печатать нечего.
	interp := eval.NewInterpreter(io.Discard, 0, eval.SystemClock{})
	if semErr := interp.Analyze(prog); semErr != nil {
		return nil, toDiagnostics(semErr), nil
	}

	return lowerProgram(prog), nil, nil
}

// CompileFile читает файл .ladix и компилирует его содержимое — результат
// эквивалентен Compile над прочитанным текстом.
//
// Ошибка чтения файла — ВНУТРЕННИЙ сбой, а не диагностика: возвращается
// err != nil при program == nil и пустых diags, компиляция не начинается.
func CompileFile(path string) (*ir.Program, []ir.Diagnostic, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("ladix: не прочитать %s: %w", path, err)
	}
	return Compile(string(b))
}
