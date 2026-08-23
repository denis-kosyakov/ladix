package engine_test

import (
	stderrors "errors"
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/store"
)

// Локи §EN-8.A #8 / §EN-8.B B9: сбой Store, ВСПЛЫВШИЙ через тело шага на пути
// advance из Complete, обязан нести *engine.StoreError в Unwrap-цепочке
// (ОшибкаВыполнения.Cause → *StoreError). Раньше уплощение строкой (runtimeErr с
// err.Error()) теряло тип → completeError давал §13/exit 1 вместо B9/exit 2. Оба
// лока НЕ-тавтологичны: ассерт — про errors.As(*engine.StoreError), а НЕ про текст;
// при откате runtimeErrWrap→runtimeErr(...err.Error()) Unwrap-цепочка рвётся и
// errors.As перестаёт находить тип → лок A падает.

// nthCallFailStore — обёртка Store со счётчиком вызовов NextInstanceID: возвращает
// ошибку РОВНО на failOn-м вызове, прочие делегирует вложенному MemoryStore. Это
// дискриминатор «внешний инстанс минтится успешно, ВЛОЖЕННЫЙ запуск падает»: внешний
// Start потребляет вызов №1 (p-000001), вложенный StartProcess из тела авто-шага —
// вызов №2 (сбой). Точечный сбой именно на старте вложенного процесса.
type nthCallFailStore struct {
	store.Store
	failOn   int
	callsLog int
	err      error
}

func (s *nthCallFailStore) NextInstanceID() (string, error) {
	s.callsLog++
	if s.callsLog == s.failOn {
		return "", s.err
	}
	return s.Store.NextInstanceID()
}

// lockAOuterSrc — ЛОК A: внешний процесс «вн» = ЧЕЛОВЕЧЕСКИЙ шаг «первый», затем
// АВТО-шаг «запуск», тело которого делает вложенный «запустить процесс вло(...)».
// Вложенный процесс «вло» — одиночный человеческий шаг (валидное определение, чтобы
// interp.Process резолвил имя). Старт «вн» → инстанс p-000001 + задача t-000001,
// ожидание на «первый». Complete t-000001 → advance входит в «запуск» → ExecStepBody
// исполняет вложенный StartProcess(вло) → NextInstanceID (вызов №2) ловит сбой Store.
const lockAOuterSrc = `процесс вло(к):
    шаг единственный:
        исполнитель: "Сидоров"

процесс вн(x):
    шаг первый:
        исполнитель: "Иванов"
    шаг запуск после первый:
        присвоить y = запустить процесс вло(x)

пусть id = запустить процесс вн(1)
`

// TestCompleteNestedStartProcessStoreFailureB9 — ЛОК A (основной, реальный движок):
// сбой Store на старте ВЛОЖЕННОГО процесса в теле авто-шага, достигнутого через
// advance из Complete. Падающий Store успешен для внешнего инстанса/задачи, падает
// на NextInstanceID вложенного (счётчик failOn=2). АССЕРТ: errors.As(completeErr,
// &*engine.StoreError) == true И se.Error() начинается с «сбой хранилища: ».
func TestCompleteNestedStartProcessStoreFailureB9(t *testing.T) {
	fs := &nthCallFailStore{
		Store:  store.NewMemoryStore(),
		failOn: 2, // №1 — внешний «вн» (успех), №2 — вложенный «вло» (сбой)
		err:    stderrors.New("диск переполнен"),
	}
	eng := buildStackStore(t, lockAOuterSrc, goldenMoment(), fs)

	// Старт внешнего процесса: инстанс+задача созданы, ожидание (человеческий шаг).
	if _, err := eng.Start("вн", argsInt(1)); err != nil {
		t.Fatalf("Start внешнего процесса не должен падать (вызов №1 успешен): %v", err)
	}

	// Complete задачи внешнего процесса → advance в авто-шаг → тело → вложенный
	// StartProcess(вло) → NextInstanceID вызов №2 ловит сбой Store.
	_, completeErr := eng.Complete("t-000001", emptyRec())
	if completeErr == nil {
		t.Fatalf("ожидали сбой Store на старте вложенного процесса, получили nil")
	}

	var se *engine.StoreError
	if !stderrors.As(completeErr, &se) {
		t.Fatalf("completeErr = %v (%T) — НЕ оборачивает *engine.StoreError; "+
			"Unwrap-цепочка ОшибкаВыполнения.Cause→*StoreError разорвана (B9 теряется → §13/exit 1)", completeErr, completeErr)
	}
	if !strings.HasPrefix(se.Error(), "сбой хранилища: ") {
		t.Errorf("se.Error() = %q, хотим начинающийся с «сбой хранилища: » (§EN-8.A #8 / B9)", se.Error())
	}
}

// saveInstFailOnStep — обёртка Store, падающая на SaveInstance ТОЛЬКО когда инстанс
// уже на целевом авто-шаге (Status «выполняется» И CurrentStep == targetStep) —
// именно тот ▼SaveInstance, что инициирует хук AssignProcessVar из тела шага. Прочие
// SaveInstance (создание/ожидание/смена шага) проходят. Дискриминатор по состоянию,
// а не по счётчику: внешний lifecycle до целевого шага не задет.
type saveInstFailOnStep struct {
	store.Store
	targetStep string
	err        error
}

func (s *saveInstFailOnStep) SaveInstance(inst *store.ProcessInstance) error {
	if inst.Status == store.StatusRunning && inst.CurrentStep == s.targetStep {
		// Пропускаем ПЕРВЫЙ ▼ входа в шаг (advance ставит «выполняется» до тела);
		// хук присвоить даёт ВТОРОЙ ▼ — на нём и падаем. Различаем по наличию
		// присвоенной переменной в Variables (хук уже положил x перед save).
		if _, assigned := inst.Variables["x"]; assigned {
			return s.err
		}
	}
	return s.Store.SaveInstance(inst)
}

// lockBSrc — ЛОК B: процесс «б» = ЧЕЛОВЕЧЕСКИЙ шаг «первый», затем АВТО-шаг «работа»,
// тело которого делает «присвоить x = 42». На пути Complete advance входит в «работа»,
// исполняет тело, хук AssignProcessVar персистит инстанс (▼SaveInstance) — там и
// ловится сбой Store.
const lockBSrc = `процесс б(n):
    шаг первый:
        исполнитель: "Иванов"
    шаг работа после первый:
        присвоить x = 42

пусть id = запустить процесс б(1)
`

// TestCompleteAssignProcessVarStoreFailureB9 — ЛОК B (вторичный): AssignProcessVar
// ловит сбой Store (▼SaveInstance) на пути complete→advance→ExecStepBody. Тот же
// ассерт: errors.As(*engine.StoreError) + префикс «сбой хранилища: ». Сбой нацелен на
// SaveInstance шага «работа» ПОСЛЕ записи переменной x хуком (внешний lifecycle до
// этого момента не задет).
func TestCompleteAssignProcessVarStoreFailureB9(t *testing.T) {
	fs := &saveInstFailOnStep{
		Store:      store.NewMemoryStore(),
		targetStep: "работа",
		err:        stderrors.New("бд повреждена"),
	}
	eng := buildStackStore(t, lockBSrc, goldenMoment(), fs)

	if _, err := eng.Start("б", argsInt(1)); err != nil {
		t.Fatalf("Start не должен падать (целевой шаг ещё не достигнут): %v", err)
	}

	_, completeErr := eng.Complete("t-000001", emptyRec())
	if completeErr == nil {
		t.Fatalf("ожидали сбой Store в AssignProcessVar на пути complete, получили nil")
	}

	var se *engine.StoreError
	if !stderrors.As(completeErr, &se) {
		t.Fatalf("completeErr = %v (%T) — НЕ оборачивает *engine.StoreError "+
			"(Unwrap-цепочка ОшибкаВыполнения.Cause→*StoreError разорвана)", completeErr, completeErr)
	}
	if !strings.HasPrefix(se.Error(), "сбой хранилища: ") {
		t.Errorf("se.Error() = %q, хотим начинающийся с «сбой хранилища: »", se.Error())
	}
}
