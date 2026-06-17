package eval

import (
	stderrors "errors"
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/errors"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// C-SEAM-1 (B1, 013): компиляц.-замок — fakeRuntime реализует ВСЕ методы
// ProcessRuntime, включая CallExternalResult. Если удалить CallExternalResult из
// интерфейса, эта проверка останется зелёной, но кейс eval (T017/18/19) не
// скомпилируется — счёт «ровно 8» закрепляется связкой замков.
var _ ProcessRuntime = (*fakeRuntime)(nil)

// runCallExtSrc компилирует top-level «пусть r = вызвать …», инъектирует rt и
// исполняет; возвращает интерпретатор (для чтения r) и ошибку Run. Узел вызвать —
// на строке 1, колонка 11 (после «пусть r = »).
func runCallExtSrc(t *testing.T, src string, rt ProcessRuntime) (*Interpreter, error) {
	t.Helper()
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные лекс/синт ошибки: %s", errList.Error())
	}
	interp := NewInterpreter(&stringsBuilderWriter{}, 0, testClock)
	interp.SetProcessRuntime(rt)
	if err := interp.Analyze(prog); err != nil {
		t.Fatalf("неожиданная семантическая ошибка: %s", err.Error())
	}
	return interp, interp.Run(prog)
}

// TestEvalCallExternalStubReturnsNone — C-SEAM-3.3 (B1): под фейк-стабом
// (callResult==nil → value.None) результат вызвать = value.None, err=nil; r
// привязан к Пусто. Инверсия: стаб вернуть value.Строка{…} → r ≠ None.
func TestEvalCallExternalStubReturnsNone(t *testing.T) {
	rt := &fakeRuntime{}
	interp, err := runCallExtSrc(t, `пусть r = вызвать сервис(1)`+"\n", rt)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	v, ok := interp.GlobalEnv().Lookup("r")
	if !ok {
		t.Fatalf("r не привязан")
	}
	if v != value.None {
		t.Errorf("r = %v (%s), хотим value.None (Пусто)", v, v.TypeName())
	}
	if len(rt.calls) != 1 || rt.calls[0].target != "сервис" {
		t.Errorf("вызовы CallExternalResult = %+v, хотим один для «сервис»", rt.calls)
	}
	if len(rt.calls[0].args) != 1 || value.String(rt.calls[0].args[0]) != "1" {
		t.Errorf("аргументы = %+v, хотим [1]", rt.calls[0].args)
	}
}

// TestEvalCallExternalArgsLeftToRight — C-SEAM-3.1 (B1): аргументы вычисляются
// строго слева направо. Наблюдаемо через вложенные вызвать: внутренние a(), b()
// записываются в rt.calls в порядке вычисления, затем внешний внешн.
// Инверсия: вычислять c.Args в обратном порядке → порядок ≠ [a, b, внешн].
func TestEvalCallExternalArgsLeftToRight(t *testing.T) {
	rt := &fakeRuntime{}
	src := `пусть r = вызвать внешн(вызвать a(), вызвать b())` + "\n"
	if _, err := runCallExtSrc(t, src, rt); err != nil {
		t.Fatalf("Run: %v", err)
	}
	var order []string
	for _, c := range rt.calls {
		order = append(order, c.target)
	}
	want := []string{"a", "b", "внешн"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("порядок вызовов = %v, хотим %v (аргументы слева направо)", order, want)
	}
}

// TestEvalCallExternalWrapsError — C-SEAM-3.2 (B1): ошибка шва → errors.
// ОшибкаВыполнения через runtimeErrWrap с Pos == токен вызвать (1:11) и
// Cause == исходная ошибка. Инверсия: вернуть raw err без runtimeErrWrap → тип ≠
// ОшибкаВыполнения / нет Pos / нет Cause.
func TestEvalCallExternalWrapsError(t *testing.T) {
	boom := stderrors.New("boom")
	rt := &fakeRuntime{callResultErr: boom}
	_, err := runCallExtSrc(t, `пусть r = вызвать сервис(1)`+"\n", rt)
	if err == nil {
		t.Fatalf("ожидали ОшибкуВыполнения, получили nil")
	}
	var re errors.ОшибкаВыполнения
	if !stderrors.As(err, &re) {
		t.Fatalf("категория = %T, хотим errors.ОшибкаВыполнения", err)
	}
	if re.Pos.Line != 1 || re.Pos.Col != 11 {
		t.Errorf("Pos = %+v, хотим токен вызвать {1,11}", re.Pos)
	}
	if !stderrors.Is(err, boom) {
		t.Errorf("Cause не сохранён (Unwrap не видит исходную boom)")
	}
}
