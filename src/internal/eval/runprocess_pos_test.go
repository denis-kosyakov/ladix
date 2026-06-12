package eval

import (
	"bytes"
	stderrors "errors"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
)

// runProcSrc — top-level «запустить процесс» (узел запустить на строке 5, колонка 12).
// Тело шага НЕ исполняется fakeRuntime: StartProcess фабрикуется, поэтому позиция в
// диагностике — целиком заслуга условной обёртки evalRunProcess (§EN-8.A #8 / §EN-9).
const runProcSrc = `процесс p(x):
    шаг s:
        присвоить y = x

пусть id = запустить процесс p(1)
`

// runTopLevelFake компилирует runProcSrc, инъектирует fakeRuntime и исполняет top-level;
// возвращает ошибку interp.Run (ошибка узла «запустить процесс»).
func runTopLevelFake(t *testing.T, rt ProcessRuntime) error {
	t.Helper()
	tokens, errList := lexer.New(runProcSrc).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные лекс/синт ошибки: %s", errList.Error())
	}
	interp := NewInterpreter(&bytes.Buffer{}, 0, SystemClock{})
	interp.SetProcessRuntime(rt)
	if err := interp.Analyze(prog); err != nil {
		t.Fatalf("неожиданная семантическая ошибка: %s", err.Error())
	}
	return interp.Run(prog)
}

// TestRunProcessStoreErrorGetsNodePosition — F2: НЕ-позиционированная ошибка от
// StartProcess (признак StoreError движка: текст «сбой хранилища: …», нет позиции)
// оборачивается позицией узла «запустить процесс» — двухстрочный канон §13 (§EN-8.A #8).
// Текст НЕ дублирует префикс «сбой хранилища:» (runtimeErr берёт err.Error() как есть).
func TestRunProcessStoreErrorGetsNodePosition(t *testing.T) {
	// Имитация StoreError движка: текст с префиксом, но БЕЗ метода Позиция() (не
	// errors.Расположенная). eval не импортирует engine — отсюда plain error.
	rt := &fakeRuntime{startErr: stderrors.New("сбой хранилища: диск переполнен")}
	err := runTopLevelFake(t, rt)
	if err == nil {
		t.Fatalf("ожидали ОшибкуВыполнения с позицией узла, получили nil")
	}
	want := "Ошибка в строке 5, колонка 12:\nсбой хранилища: диск переполнен"
	if err.Error() != want {
		t.Errorf("диагностика =\n%q\nхотим\n%q (§EN-8.A #8, поз. узла «запустить процесс»)", err.Error(), want)
	}
}

// TestRunProcessBodyErrorKeepsBodyPosition — F2 регресс-замок: УЖЕ позиционированная
// eval-ошибка (ОшибкаВыполнения тела шага, §EN-9) проходит сквозь evalRunProcess как
// есть — позиция ТЕЛА НЕ затирается позицией узла запуска. Имитируем телесную ошибку,
// возвращая её прямо из StartProcess (в проде её несёт advance → Start → сюда).
func TestRunProcessBodyErrorKeepsBodyPosition(t *testing.T) {
	bodyErr := errors.ОшибкаВыполнения{
		Pos: errors.Position{Line: 42, Col: 7},
		Msg: "деление на ноль",
	}
	rt := &fakeRuntime{startErr: bodyErr}
	err := runTopLevelFake(t, rt)
	if err == nil {
		t.Fatalf("ожидали проброс позиционированной ошибки тела, получили nil")
	}
	// Позиция ТЕЛА (42:7), НЕ узла «запустить процесс» (5:12).
	want := "Ошибка в строке 42, колонка 7:\nделение на ноль"
	if err.Error() != want {
		t.Errorf("диагностика =\n%q\nхотим\n%q (§EN-9: позиция тела, не узла запуска)", err.Error(), want)
	}
}
