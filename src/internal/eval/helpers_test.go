package eval

import (
	"bytes"
	stderrors "errors"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// run исполняет исходник через конвейер лексер→парсер→Run и возвращает stdout и
// ошибку eval. Синтаксические/лексические ошибки — фатальны для теста (вход обязан
// быть корректным; интерпретатор проверяется отдельно).
func run(t *testing.T, src string) (string, error) {
	t.Helper()
	return runDepth(t, src, 0)
}

func runDepth(t *testing.T, src string, maxDepth int) (string, error) {
	t.Helper()
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные лексические/синтаксические ошибки: %v", errList.Error())
	}
	var buf bytes.Buffer
	interp := NewInterpreter(&buf, maxDepth)
	err := interp.Run(prog)
	return buf.String(), err
}

// evalExpr1 вычисляет одиночное выражение белым ящиком (через «пусть _r = E») и
// возвращает значение — для проверки и репрезентации, и КАНОНИЧЕСКОГО типа без
// встроенной тип(), которая недостижима из исходника (лексер резервирует «тип»).
func evalExpr1(t *testing.T, exprSrc string) value.Value {
	t.Helper()
	src := "пусть _r = " + exprSrc
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные ошибки разбора %q: %v", exprSrc, errList.Error())
	}
	interp := NewInterpreter(&bytes.Buffer{}, 0)
	if err := interp.Run(prog); err != nil {
		t.Fatalf("ошибка вычисления %q: %v", exprSrc, err)
	}
	v, ok := interp.global.Lookup("_r")
	if !ok {
		t.Fatalf("переменная _r не привязана")
	}
	return v
}

// analyzeSrc прогоняет ТОЛЬКО семпроход (стадия 3) на свежем интерпретаторе.
func analyzeSrc(t *testing.T, src string) error {
	t.Helper()
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные лексические/синтаксические ошибки: %v", errList.Error())
	}
	interp := NewInterpreter(&bytes.Buffer{}, 0)
	return interp.Analyze(prog)
}

// evalErr извлекает позицию (строка, колонка в рунах) и текст eval-ошибки.
func evalErr(t *testing.T, err error) (line, col int, msg string) {
	t.Helper()
	if err == nil {
		t.Fatalf("ожидалась ошибка eval, получено nil")
	}
	var se errors.СемантическаяОшибка
	var te errors.ОшибкаТипа
	var re errors.ОшибкаВыполнения
	switch {
	case stderrors.As(err, &se):
		return se.Pos.Line, se.Pos.Col, se.Msg
	case stderrors.As(err, &te):
		return te.Pos.Line, te.Pos.Col, te.Msg
	case stderrors.As(err, &re):
		return re.Pos.Line, re.Pos.Col, re.Msg
	}
	t.Fatalf("ошибка не из категорий eval: %T %v", err, err)
	return
}

// категории для проверки типа Go-ошибки (SC-003).
func isSem(err error) bool {
	var e errors.СемантическаяОшибка
	return stderrors.As(err, &e)
}
func isType(err error) bool {
	var e errors.ОшибкаТипа
	return stderrors.As(err, &e)
}
func isRuntime(err error) bool {
	var e errors.ОшибкаВыполнения
	return stderrors.As(err, &e)
}
