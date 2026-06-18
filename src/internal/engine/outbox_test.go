package engine

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// outbox_test.go — Go-API замки дедупа эффектов тела шага (M3-C2b, §C-2b.2/.4/.5,
// contracts/dispatch-protocol.md). Дедуп активен ⟺ len(e.active)>0; ключ
// (inst.ID, CurrentStep, effectIndex); deliver-then-record + pre-check.

// countingCaller — fake ExternalCaller со счётчиками и опциональной ошибкой/возвратом.
type countingCaller struct {
	calls    int
	notifies int
	ret      value.Value
	callErr  error
	notErr   error
}

func (c *countingCaller) Call(target string, args []value.Value) (value.Value, error) {
	c.calls++
	if c.callErr != nil {
		return nil, c.callErr
	}
	if c.ret != nil {
		return c.ret, nil
	}
	return value.None, nil
}

func (c *countingCaller) Notify(target string, args []value.Value) error {
	c.notifies++
	return c.notErr
}

// fixedOutboxClock — детерминированные часы движка для in-package outbox-тестов
// (engine_test.go::fixedClock в другом пакете engine_test, недоступен здесь).
type fixedOutboxClock struct{ t time.Time }

func (c fixedOutboxClock) Now() time.Time { return c.t }

// outboxEngine строит in-package Engine с fake caller + MemoryStore + fixedClock
// и (опционально) проталкивает активный кадр шага в e.active.
func outboxEngine(t *testing.T, caller ExternalCaller, withFrame bool) (*Engine, *activeFrame) {
	t.Helper()
	var out bytes.Buffer
	interp := eval.NewInterpreter(&out, 0, eval.SystemClock{})
	st := store.NewMemoryStore()
	e := NewEngine(st, interp, &out,
		WithClock(fixedOutboxClock{time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)}),
		WithExternalCaller(caller))
	var fr *activeFrame
	if withFrame {
		inst := &store.ProcessInstance{ID: "p-000001", ProcessName: "тест", CurrentStep: "уведомить_crm"}
		fr = &activeFrame{inst: inst}
		e.active = append(e.active, fr)
	}
	return e, fr
}

// TestOutboxLedgerSkipsDelivered — дважды Engine.Notify под одним кадром+ключом
// (effectIndex сброшен в 0 перед каждым «телом»): caller.Notify вызван РОВНО один раз;
// второй вызов — пропуск по LoadOutbox.Delivered.
func TestOutboxLedgerSkipsDelivered(t *testing.T) {
	c := &countingCaller{}
	e, fr := outboxEngine(t, c, true)

	fr.effectIndex = 0
	if err := e.Notify("crm", []value.Value{value.Строка{V: "первый"}}); err != nil {
		t.Fatalf("Notify #1: %v", err)
	}
	// Повторное «исполнение тела» того же шага: сброс индекса → тот же ключ.
	fr.effectIndex = 0
	if err := e.Notify("crm", []value.Value{value.Строка{V: "первый"}}); err != nil {
		t.Fatalf("Notify #2: %v", err)
	}
	if c.notifies != 1 {
		t.Errorf("caller.Notify вызван %d раз, хотим 1 (второй — пропуск по дедупу)", c.notifies)
	}
}

// TestOutboxResultReplay — CallExternalResult под дедупом возвращает СОХРАНЁННЫЙ
// Result без повторного Call (логика процесса не разойдётся).
func TestOutboxResultReplay(t *testing.T) {
	c := &countingCaller{ret: value.Целое{V: 777}}
	e, fr := outboxEngine(t, c, true)

	fr.effectIndex = 0
	v1, err := e.CallExternalResult("crm", nil)
	if err != nil {
		t.Fatalf("CallExternalResult #1: %v", err)
	}
	if vi, ok := v1.(value.Целое); !ok || vi.V != 777 {
		t.Fatalf("первый результат = %#v, хотим Целое{777}", v1)
	}
	fr.effectIndex = 0
	v2, err := e.CallExternalResult("crm", nil)
	if err != nil {
		t.Fatalf("CallExternalResult #2: %v", err)
	}
	if vi, ok := v2.(value.Целое); !ok || vi.V != 777 {
		t.Errorf("реплей результата = %#v, хотим сохранённое Целое{777}", v2)
	}
	if c.calls != 1 {
		t.Errorf("caller.Call вызван %d раз, хотим 1 (второй — реплей по дедупу)", c.calls)
	}
}

// TestDedupOnlyInsideStepBody — len(e.active)==0 (вне тела шага) → delegate напрямую,
// outbox НЕ консультируется (нет записи). Дважды Notify → caller дважды.
func TestDedupOnlyInsideStepBody(t *testing.T) {
	c := &countingCaller{}
	e, _ := outboxEngine(t, c, false) // без кадра

	if err := e.Notify("crm", nil); err != nil {
		t.Fatalf("Notify #1: %v", err)
	}
	if err := e.Notify("crm", nil); err != nil {
		t.Fatalf("Notify #2: %v", err)
	}
	if c.notifies != 2 {
		t.Errorf("вне тела шага caller.Notify вызван %d раз, хотим 2 (дедуп НЕ активен)", c.notifies)
	}
	// Леджер пуст: запись с ключом «нет кадра» не появлялась.
	if _, err := e.st.LoadOutbox("|"); !errors.Is(err, store.ErrOutboxNotFound) {
		t.Errorf("вне тела шага outbox-запись появилась (err=%v)", err)
	}
}

// TestTwoEffectsIndependentKeys — два эффекта в одном теле шага → effectIndex 0 и 1,
// два разных ключа, дедуплицируются независимо (второй эффект НЕ «съеден» первым).
func TestTwoEffectsIndependentKeys(t *testing.T) {
	c := &countingCaller{}
	e, fr := outboxEngine(t, c, true)

	// Первое исполнение тела: два уведомления, idx 0 и 1.
	fr.effectIndex = 0
	if err := e.Notify("crm", []value.Value{value.Строка{V: "A"}}); err != nil {
		t.Fatalf("Notify A: %v", err)
	}
	if err := e.Notify("crm", []value.Value{value.Строка{V: "B"}}); err != nil {
		t.Fatalf("Notify B: %v", err)
	}
	if c.notifies != 2 {
		t.Fatalf("два эффекта в теле дали %d вызовов, хотим 2 (независимые ключи)", c.notifies)
	}
	// Повторное исполнение тела (рестарт-имитация): оба — пропуск по дедупу.
	fr.effectIndex = 0
	_ = e.Notify("crm", []value.Value{value.Строка{V: "A"}})
	_ = e.Notify("crm", []value.Value{value.Строка{V: "B"}})
	if c.notifies != 2 {
		t.Errorf("после реисполнения тела caller.Notify вызван %d раз, хотим 2 (оба дедуплицированы)", c.notifies)
	}

	// Ключи различны: idx 0 и 1 под тем же инстансом/шагом.
	if _, err := e.st.LoadOutbox("p-000001|уведомить_crm|0"); err != nil {
		t.Errorf("ключ idx0 не найден: %v", err)
	}
	if _, err := e.st.LoadOutbox("p-000001|уведомить_crm|1"); err != nil {
		t.Errorf("ключ idx1 не найден: %v", err)
	}
}

// TestOutboxDeliverFailsNotMarked — ошибка доставки → derr наверх, ключ НЕ delivered
// (deliver-then-record: SaveOutbox только при успехе). Повтор после восстановления
// caller → доставка происходит (не «съедена» ложной записью).
func TestOutboxDeliverFailsNotMarked(t *testing.T) {
	c := &countingCaller{notErr: errors.New("доставка упала")}
	e, fr := outboxEngine(t, c, true)

	fr.effectIndex = 0
	if err := e.Notify("crm", nil); err == nil {
		t.Fatalf("Notify при сбое доставки вернул nil, хотим ошибку")
	}
	// Запись не помечена delivered → её нет (deliver-then-record).
	if _, err := e.st.LoadOutbox("p-000001|уведомить_crm|0"); !errors.Is(err, store.ErrOutboxNotFound) {
		t.Errorf("после сбоя доставки запись помечена delivered (err=%v)", err)
	}
	// Восстанавливаем caller; повтор «тела» доставляет.
	c.notErr = nil
	fr.effectIndex = 0
	if err := e.Notify("crm", nil); err != nil {
		t.Fatalf("Notify после восстановления: %v", err)
	}
	if c.notifies != 2 {
		t.Errorf("caller.Notify вызван %d раз, хотим 2 (сбой не пометил delivered)", c.notifies)
	}
}
