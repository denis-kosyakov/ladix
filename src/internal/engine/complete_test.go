package engine_test

import (
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/store"
)

// twoHumanSrc — процесс из двух человеческих шагов: после Start инстанс ожидает на
// первом шаге; Complete первой задачи → создаётся вторая задача, инстанс снова
// ожидает (переход ожидает→выполняется→ожидает); Complete второй → терминал.
const twoHumanSrc = `процесс p(x):
    шаг первый:
        исполнитель: "Иванов"
        срок:        2дн
    шаг второй после первый:
        исполнитель: "Петров"

пусть id = запустить процесс p(1)
`

// TestCompleteChain — цепочка Complete: пробуждение → следующая задача → терминал.
// Проверяет переход ожидает→выполнен через выполняется при наличии следующего шага.
func TestCompleteChain(t *testing.T) {
	_, st, eng, out := buildStack(t, twoHumanSrc, goldenMoment())
	if _, err := eng.Start("p", argsInt(1)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// После Start: инстанс ожидает на шаге 'первый', задача t-000001 открыта.
	inst, err := st.LoadInstance("p-000001")
	if err != nil {
		t.Fatalf("LoadInstance: %v", err)
	}
	if inst.Status != store.StatusWaiting || inst.CurrentStep != "первый" {
		t.Fatalf("после Start: статус=%q шаг=%q, хотим ожидает/первый", inst.Status, inst.CurrentStep)
	}
	out.Reset()

	// Complete t-000001: задача завершена → продвижение к шагу 'второй' (есть
	// следующий шаг) → новая задача t-000002 → инстанс снова ожидает.
	res, err := eng.Complete("t-000001")
	if err != nil {
		t.Fatalf("Complete t-000001: %v", err)
	}
	if res.CaughtUp {
		t.Errorf("CaughtUp=true на штатном пути (не догон)")
	}
	if res.Instance == nil || res.Instance.Status != store.StatusWaiting || res.Instance.CurrentStep != "второй" {
		t.Errorf("после Complete t-000001: инстанс=%+v, хотим ожидает/второй", res.Instance)
	}
	want1 := "" +
		"задача t-000001 завершена\n" +
		"[задача] t-000002 → Петров, шаг 'второй'\n" +
		"инстанс p-000001: ожидает, шаг 'второй'\n"
	if got := out.String(); got != want1 {
		t.Errorf("stdout после Complete t-000001 байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want1)
	}
	// Первая задача завершена в Store.
	t1, _ := st.LoadTask("t-000001")
	if t1.Status != store.TaskCompleted {
		t.Errorf("t-000001 статус=%q, хотим завершена", t1.Status)
	}
	out.Reset()

	// Complete t-000002: задача завершена → нет следующего шага → терминал (выполнен).
	res2, err := eng.Complete("t-000002")
	if err != nil {
		t.Fatalf("Complete t-000002: %v", err)
	}
	if res2.Instance.Status != store.StatusDone {
		t.Errorf("после Complete t-000002: статус=%q, хотим выполнен", res2.Instance.Status)
	}
	want2 := "" +
		"задача t-000002 завершена\n" +
		"инстанс p-000001: выполнен\n"
	if got := out.String(); got != want2 {
		t.Errorf("stdout после Complete t-000002 байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want2)
	}
}

// oneHumanSrc — процесс из единственного человеческого шага: переход
// ожидает→выполнен НАПРЯМУЮ (нет следующего шага → терминал без выполняется).
const oneHumanSrc = `процесс solo(x):
    шаг единственный:
        исполнитель: "Сидоров"

пусть id = запустить процесс solo(1)
`

// TestCompleteTerminalDirect — Complete единственной задачи → терминал напрямую
// (ожидает→выполнен), без промежуточного выполняется/новой задачи.
func TestCompleteTerminalDirect(t *testing.T) {
	_, st, eng, out := buildStack(t, oneHumanSrc, goldenMoment())
	if _, err := eng.Start("solo", argsInt(1)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	out.Reset()

	res, err := eng.Complete("t-000001")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if res.Instance.Status != store.StatusDone {
		t.Errorf("статус=%q, хотим выполнен", res.Instance.Status)
	}
	want := "" +
		"задача t-000001 завершена\n" +
		"инстанс p-000001: выполнен\n"
	if got := out.String(); got != want {
		t.Errorf("stdout байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}
	// Никакой второй задачи не создано; инстанс выполнен в Store.
	if _, lerr := st.LoadTask("t-000002"); lerr == nil {
		t.Errorf("создана лишняя задача t-000002")
	}
	inst, _ := st.LoadInstance("p-000001")
	if inst.Status != store.StatusDone {
		t.Errorf("Store: инстанс статус=%q, хотим выполнен", inst.Status)
	}
}

// TestCompleteNotFound — отсутствующая задача → ошибка (для CLI → §EN-8.B).
func TestCompleteNotFound(t *testing.T) {
	_, _, eng, _ := buildStack(t, oneHumanSrc, goldenMoment())
	if _, err := eng.Start("solo", argsInt(1)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, err := eng.Complete("t-999999")
	if err == nil {
		t.Fatalf("ожидали ошибку на несуществующую задачу")
	}
	if !strings.Contains(err.Error(), "t-999999") && !strings.Contains(err.Error(), "not found") {
		// Точная Go-форма на усмотрение импл (§EN-3); CLI формирует текст §EN-8.B.
		t.Logf("ошибка Complete несуществующей задачи: %v", err)
	}
}
