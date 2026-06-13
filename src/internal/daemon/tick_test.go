package daemon

import (
	"testing"

	"github.com/denis-kosyakov/ladix/internal/store"
)

// TestTickIsolatesPanicSecondTriggerStillFires — изоляция сбоя (FR-004, SC-007):
// из двух метрика-триггеров первый паникует (тело «запустить процесс» → panicStore),
// второй (печать маркера) всё равно вычислен и сработал. Демон жив.
func TestTickIsolatesPanicSecondTriggerStillFires(t *testing.T) {
	path := fixturePath(t)
	out := &countWriter{marker: "OK2"}
	st := &panicStore{Store: store.NewMemoryStore()}

	// Триггер №0: тело «запустить процесс» → паника на NextInstanceID.
	// Триггер №1: тело печать("OK2") → штатно.
	src := "источник s:\n    файл: \"" + path + "\"\n" +
		"метрика m:\n    источник: s\n    агрегат: сумма(x)\n" +
		"процесс p:\n    шаг готово:\n        печать(\"шаг\")\n" +
		"когда метрика m > 10:\n    запустить процесс p\n" +
		"когда метрика m > 10:\n    печать(\"OK2\")\n"
	d, _ := buildDaemon(t, src, st, out)

	// Прайминг при лжи для обоих.
	writeFixture(t, path, `[{"x":1}]`)
	d.tick()
	if out.count() != 0 {
		t.Fatalf("прайминг: OK2 не должен печататься")
	}

	// Переход ложь→истина: триггер №0 паникует (изолирован), №1 печатает OK2.
	writeFixture(t, path, `[{"x":30}]`)
	d.tick()

	if got := out.count(); got != 1 {
		t.Fatalf("второй триггер должен сработать после паники первого: OK2 = %d, хотим 1", got)
	}
	if !out.contains("сбой триггера изолирован") {
		t.Fatalf("ожидали лог изоляции паники первого триггера, out=%q", out.String())
	}

	// Оба персистнуты true (база сдвинута ДО тела у обоих): следующий тик при той же
	// истине → 0 новых срабатываний (сбойный триггер не зацикливается).
	d.tick()
	if got := out.count(); got != 1 {
		t.Fatalf("истина→истина после изоляции: OK2 = %d, хотим 1 (без ре-фаера)", got)
	}
}

// TestTickPhaseOrder — строгий порядок фаз drainEvents → evalMetrics →
// checkSchedules (FR-002). В слайсе 3 drainEvents/checkSchedules — заглушки; здесь
// фиксируем, что tick() не падает на пустых фазах и evalMetrics реально вызывается
// (праймит trigger_state метрики). Полный порядок-замок — golden слайса 5.
func TestTickPhaseOrder(t *testing.T) {
	path := fixturePath(t)
	out := &countWriter{marker: "FIRE"}
	st := store.NewMemoryStore()
	d, _ := buildDaemon(t, metricSrc(path, "FIRE", 10), st, out)

	writeFixture(t, path, `[{"x":1}]`)
	d.tick() // drainEvents(stub) → evalMetrics(прайм) → checkSchedules(stub)

	// evalMetrics отработала: метрика-триггер trg-0 праймлен.
	if _, err := st.LoadTriggerState("trg-0"); err != nil {
		t.Fatalf("evalMetrics не вызвана в фазе тика: trg-0 не праймлен (%v)", err)
	}
}
