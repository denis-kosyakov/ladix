package engine_test

import (
	"bytes"
	stderrors "errors"
	"strings"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/store"
)

// runTopLevel компилирует исходник, собирает РЕАЛЬНЫЙ стек (interp+Engine поверх
// заданного Store) и исполняет top-level через interp.Run — то есть «запустить
// процесс» проходит через evalRunProcess (граница eval↔engine, F2). Возвращает
// ошибку Run (она же — ошибка узла запуска после условной обёртки §EN-8.A #8 / §EN-9).
func runTopLevel(t *testing.T, src string, now time.Time, st store.Store) error {
	t.Helper()
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные лекс/синт ошибки: %s", errList.Error())
	}
	var out bytes.Buffer
	interp := eval.NewInterpreter(&out, 0, eval.SystemClock{})
	eng := engine.NewEngine(st, interp, &out, engine.WithClock(fixedClock{now}))
	interp.SetProcessRuntime(eng)
	if err := interp.Analyze(prog); err != nil {
		t.Fatalf("неожиданная семантическая ошибка: %s", err.Error())
	}
	return interp.Run(prog)
}

// runProcStoreFailSrc / runProcDivSrc — узел «запустить процесс» на строке 5, колонка 12;
// тело шага — на строке 3 (для регресс-замка позиции тела).
const runProcStoreFailSrc = `процесс p(x):
    шаг s:
        присвоить y = x

пусть id = запустить процесс p(1)
`

const runProcDivSrc = `процесс p(x):
    шаг s:
        присвоить y = 1 / 0

пусть id = запустить процесс p(1)
`

// TestRunProcessStoreFailureNodePosition — F2 (интеграция, реальный *engine.StoreError):
// сбой Store на пути «запустить процесс» (NextInstanceID → ошибка) даёт ДВУХСТРОЧНУЮ
// диагностику §13 с позицией узла запуска (5:12), текст «сбой хранилища: <причина>»
// (§EN-8.A #8). Реальный StoreError движка НЕ реализует errors.Расположенная → обёртка
// позицией узла срабатывает (а не проходит мимо).
func TestRunProcessStoreFailureNodePosition(t *testing.T) {
	fs := failingStore{Store: store.NewMemoryStore(), err: stderrors.New("диск переполнен")}
	err := runTopLevel(t, runProcStoreFailSrc, goldenMoment(), fs)
	if err == nil {
		t.Fatalf("ожидали сбой хранилища с позицией узла, получили nil")
	}
	want := "Ошибка в строке 5, колонка 12:\nсбой хранилища: диск переполнен"
	if err.Error() != want {
		t.Errorf("диагностика =\n%q\nхотим\n%q (§EN-8.A #8, поз. узла «запустить процесс»)", err.Error(), want)
	}
}

// TestRunProcessBodyDivisionKeepsBodyPosition — F2 регресс-замок (§EN-9): тело шага с
// делением на ноль, достигнутое через «запустить процесс», по-прежнему несёт позицию
// ТЕЛА (строка 3), НЕ узла запуска (строка 5). Условная обёртка evalRunProcess не
// затирает уже позиционированную ОшибкуВыполнения. Инстанс → провален (D-14).
func TestRunProcessBodyDivisionKeepsBodyPosition(t *testing.T) {
	st := store.NewMemoryStore()
	err := runTopLevel(t, runProcDivSrc, goldenMoment(), st)
	if err == nil {
		t.Fatalf("ожидали ошибку тела, получили nil")
	}
	if !strings.HasPrefix(err.Error(), "Ошибка в строке 3, колонка ") {
		t.Errorf("позиция = %q, хотим строку 3 (тело), не узел запуска (стр.5) — §EN-9", err.Error())
	}
	if !strings.Contains(err.Error(), "деление на ноль") {
		t.Errorf("текст = %q, хотим содержащий 'деление на ноль'", err.Error())
	}
	if strings.Contains(err.Error(), "строке 5") {
		t.Errorf("позиция тела затёрта узлом запуска (стр.5): %q", err.Error())
	}
	inst, lerr := st.LoadInstance("p-000001")
	if lerr != nil {
		t.Fatalf("LoadInstance: %v", lerr)
	}
	if inst.Status != store.StatusFailed {
		t.Errorf("статус = %q, хотим %q (D-14)", inst.Status, store.StatusFailed)
	}
}
