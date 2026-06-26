package engine

import (
	"bytes"
	stderrors "errors"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// trycatch_outbox_seam_test.go — ИНТЕГРАЦИОННЫЙ замок шва eval→engine→outbox для
// пытаться/словить (029 Уровень 2). В ОТЛИЧИЕ от изолированных слоёв:
//   - eval/try_test.go гоняет тело через fakeRuntime (НЕ касается outbox/effectIndex);
//   - engine/outbox_test.go зовёт e.Notify/e.CallExternalResult напрямую с ручными
//     сбросами fr.effectIndex (НЕ проходит через пытаться/словить + ExecStepBody/advance).
//
// Здесь шов замкнут целиком: РЕАЛЬНОЕ тело шага с пытаться/словить прогоняется через
// Engine.advance, переживает крэш-рестарт (НОВЫЙ Engine поверх ТОГО ЖЕ Store, повторный
// прогон advance того же шага) и доказывает детерминированный exactly-once. Регресс на
// шве (сдвиг сброса effectIndex в engine.go:281 ИЛИ двойная обёртка в runtimeErrWrap)
// поймается тут, но НЕ изолированными тестами.
//
// Тело (full trap form, 4 РАЗНЫХ цели, statement-форма «вызвать» → ExternalCaller.Call):
//
//	пытаться:
//	    вызвать a(1)   # A — всегда успех
//	    вызвать b(1)   # B — внешний вызов, ПАДАЕТ на 1-м прогоне (транзиент)
//	    вызвать c(1)   # C — после B; НЕ исполняется (B бросает → REDIRECT в словить)
//	словить:
//	    вызвать f(1)   # F — fallback
const trapFormProcSrc = `процесс онбординг:
    шаг отправить:
        пытаться:
            вызвать a(1)
            вызвать b(1)
            вызвать c(1)
        словить:
            вызвать f(1)
`

// perTargetCaller — fake ExternalCaller с per-target счётчиками. Цель B даёт ошибку на
// ПЕРВОМ Call (транзиент, который БЫ преуспел на рестарте) и nil далее; A/C/F — успех.
// failB управляет «выздоровлением» B между прогонами (на рестарте мы выставляем false,
// чтобы доказать: B НЕ пере-вызывается, а пере-бросается замороженный вердикт).
type perTargetCaller struct {
	calls map[string]int
	failB bool // пока true — B падает; на рестарте ставим false (B «выздоровел»)
}

func newPerTargetCaller() *perTargetCaller {
	return &perTargetCaller{calls: make(map[string]int), failB: true}
}

func (c *perTargetCaller) Call(target string, args []value.Value) (value.Value, error) {
	c.calls[target]++
	if target == "b" && c.failB {
		return nil, stderrors.New("B упал на первом вызове (транзиент)")
	}
	return value.None, nil
}

func (c *perTargetCaller) Notify(target string, args []value.Value) error {
	c.calls[target]++
	return nil
}

// trapFormEngine компилирует trapFormProcSrc и собирает белоящичный Engine поверх
// переданного Store с инъекцией caller + детерминированных часов (зеркало outboxEngine,
// но с РЕАЛЬНЫМ процессом из исходника, чтобы ExecStepBody/advance исполняли тело).
func trapFormEngine(t *testing.T, st store.Store, caller ExternalCaller) *Engine {
	t.Helper()
	tokens, errList := lexer.New(trapFormProcSrc).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные лекс/синт ошибки: %s", errList.Error())
	}
	var out bytes.Buffer
	interp := eval.NewInterpreter(&out, 0, eval.SystemClock{})
	e := NewEngine(st, interp, &out,
		WithClock(fixedOutboxClock{time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)}),
		WithExternalCaller(caller))
	interp.SetProcessRuntime(e)
	if err := interp.Analyze(prog); err != nil {
		t.Fatalf("неожиданная семантическая ошибка: %s", err.Error())
	}
	return e
}

// TestTryCatchOutboxSeamCrashRestart замыкает шов eval→engine→outbox для пытаться/словить
// через РЕАЛЬНЫЙ advance + крэш-рестарт.
//
// Прогон 1 (свежий MemoryStore, Engine #1, Start → advance): B падает → вердикт сбоя
// durable-заморожен → словить ловит → инстанс НЕ провален. Ожидаем a=1,b=1,c=0,f=1.
//
// Прогон 2 = крэш-рестарт (НОВЫЙ Engine #2 поверх ТОГО ЖЕ Store, повторный advance того
// же шага): A дедупится (a остаётся 1) · B НЕ зовётся вновь, хотя теперь вернул бы nil
// (b остаётся 1) — доказательство ЗАМОРОЖЕННОГО вердикта, не retry-on-restart · C по-
// прежнему не доставлен (c==0) — нет тихого пропуска / коллизии индексов · F дедупится
// (f остаётся 1) · статус идентичен Прогону 1.
//
// МУТАЦИОННАЯ ПРОБА: если прод откатится к retry-on-restart (убрать outboxRecordFailure
// из effect-метода ИЛИ frozenVerdict из outboxPrecheck), то на рестарте B пере-вызовется.
// Поскольку caller на рестарте «выздоровел» (failB=false), B вернёт nil → словить НЕ
// сработает → этот тест покраснеет на callsB==2 (B пере-отправлен) И на callsF (fallback
// больше не вызван → f==1 ожидаемо, но c станет 1, а b станет 2). Минимальный красный
// сигнал — assert callsB==1 на рестарте.
func TestTryCatchOutboxSeamCrashRestart(t *testing.T) {
	st := store.NewMemoryStore()
	c := newPerTargetCaller()

	// --- Прогон 1: свежий движок, реальный advance тела с пытаться/словить ---
	e1 := trapFormEngine(t, st, c)
	id, err := e1.Start("онбординг", nil)
	if err != nil {
		t.Fatalf("Прогон 1 Start: %v (сбой B должен быть пойман словить, шаг завершиться)", err)
	}

	if c.calls["a"] != 1 {
		t.Errorf("Прогон 1: callsA=%d, хотим 1 (A всегда успех)", c.calls["a"])
	}
	if c.calls["b"] != 1 {
		t.Errorf("Прогон 1: callsB=%d, хотим 1 (B упал на первом вызове)", c.calls["b"])
	}
	if c.calls["c"] != 0 {
		t.Errorf("Прогон 1: callsC=%d, хотим 0 (B бросил → REDIRECT, C после B НЕ исполнен)", c.calls["c"])
	}
	if c.calls["f"] != 1 {
		t.Errorf("Прогон 1: callsF=%d, хотим 1 (словить → fallback F исполнен)", c.calls["f"])
	}

	inst1, err := st.LoadInstance(id)
	if err != nil {
		t.Fatalf("Прогон 1 LoadInstance: %v", err)
	}
	if inst1.Status == store.StatusFailed {
		t.Fatalf("Прогон 1: инстанс провален, хотим НЕ провален (словить поймал сбой B)")
	}
	run1Status := inst1.Status

	// Вердикт сбоя B durable-заморожен: запись idx1 есть, Delivered=false. Это и есть
	// топливо exactly-once на рестарте (пере-брос без доставки).
	rec, lerr := st.LoadOutbox(id + "|отправить|1")
	if lerr != nil {
		t.Fatalf("Прогон 1: вердикт сбоя B не заморожен (LoadOutbox idx1: %v)", lerr)
	}
	if rec.Delivered {
		t.Errorf("Прогон 1: вердикт сбоя B помечен Delivered=true, хотим false (заморожен)")
	}

	// --- Прогон 2: КРЭШ-РЕСТАРТ. НОВЫЙ Engine поверх ТОГО ЖЕ Store; caller «выздоровел»
	// (B вернул бы nil). Повторно прогоняем advance того же шага через ReactivateInstance. ---
	c.failB = false // B теперь успешен — но замороженный вердикт не должен дать его вызвать
	e2 := trapFormEngine(t, st, c)
	inst2, err := st.LoadInstance(id)
	if err != nil {
		t.Fatalf("Прогон 2 LoadInstance: %v", err)
	}
	if err := e2.ReactivateInstance(inst2); err != nil {
		t.Fatalf("Прогон 2 ReactivateInstance: %v (реплей тела должен снова поймать вердикт)", err)
	}

	// A дедуплицирован (Delivered) → caller.Call НЕ вызван вновь.
	if c.calls["a"] != 1 {
		t.Errorf("Рестарт: callsA=%d, хотим 1 (A дедуплицирован, не пере-отправлен)", c.calls["a"])
	}
	// ЯДРО: B НЕ вызван вновь, хотя вернул бы nil → доказан ЗАМОРОЖЕННЫЙ вердикт, не retry.
	if c.calls["b"] != 1 {
		t.Errorf("Рестарт: callsB=%d, хотим 1 (замороженный вердикт пере-брошен БЕЗ доставки; "+
			"==2 ⇒ retry-on-restart — мутпроба должна краснеть здесь)", c.calls["b"])
	}
	// C по-прежнему не доставлен — нет тихого пропуска / коллизии индексов.
	if c.calls["c"] != 0 {
		t.Errorf("Рестарт: callsC=%d, хотим 0 (B пере-брошен до C → REDIRECT снова в словить)", c.calls["c"])
	}
	// F дедуплицирован → fallback не задвоен.
	if c.calls["f"] != 1 {
		t.Errorf("Рестарт: callsF=%d, хотим 1 (fallback F дедуплицирован, не задвоен)", c.calls["f"])
	}

	inst2after, err := st.LoadInstance(id)
	if err != nil {
		t.Fatalf("Рестарт LoadInstance: %v", err)
	}
	if inst2after.Status != run1Status {
		t.Errorf("Рестарт: статус=%q, хотим идентичный Прогону 1 %q", inst2after.Status, run1Status)
	}
	if inst2after.Status == store.StatusFailed {
		t.Errorf("Рестарт: инстанс провален, хотим НЕ провален (детерминированный реплей словить)")
	}
}
