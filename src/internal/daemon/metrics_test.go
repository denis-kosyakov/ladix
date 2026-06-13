package daemon

import (
	stderrors "errors"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/store"
)

// TestEvalMetricsEdgeFiresOnce — фронт ложь→истина срабатывает РОВНО один раз
// (FR-006, SC-001/003). тик1 метрика=ложь → 0; тик2 метрика=истина → 1; тик3
// истина → 0 (нет нового перехода).
func TestEvalMetricsEdgeFiresOnce(t *testing.T) {
	path := fixturePath(t)
	out := &countWriter{marker: "FIRE"}
	d, _ := buildDaemon(t, metricSrc(path, "FIRE", 10), store.NewMemoryStore(), out)

	// Прайминг-тик: сумма(x)=3 (3 > 10 == ложь). Первый тик праймит базу.
	writeFixture(t, path, `[{"x":1},{"x":2}]`)
	d.tick()
	if got := out.count(); got != 0 {
		t.Fatalf("прайминг-тик: срабатываний = %d, хотим 0", got)
	}

	// тик ложь→ложь (всё ещё 3 > 10 == ложь): база остаётся false, тело не исполнено.
	d.tick()
	if got := out.count(); got != 0 {
		t.Fatalf("ложь→ложь: срабатываний = %d, хотим 0", got)
	}

	// тик ложь→истина: сумма(x)=30 (30 > 10 == истина) → РОВНО одно срабатывание.
	writeFixture(t, path, `[{"x":10},{"x":20}]`)
	d.tick()
	if got := out.count(); got != 1 {
		t.Fatalf("ложь→истина: срабатываний = %d, хотим 1", got)
	}

	// тик истина→истина: нет нового перехода → 0 новых срабатываний.
	d.tick()
	if got := out.count(); got != 1 {
		t.Fatalf("истина→истина: всего срабатываний = %d, хотим 1 (ре-фаер недопустим)", got)
	}
}

// TestEvalMetricsPrimingNoFalsePositive — первый тик БЕЗ предсостояния не срабатывает
// даже при cur==true (FR-007, SC-002): праймит LastBool=cur. Последующий переход
// срабатывает.
func TestEvalMetricsPrimingNoFalsePositive(t *testing.T) {
	path := fixturePath(t)
	out := &countWriter{marker: "FIRE"}
	d, _ := buildDaemon(t, metricSrc(path, "FIRE", 10), store.NewMemoryStore(), out)

	// Прайминг-тик при УЖЕ истинном условии (30 > 10): 0 срабатываний (FR-007).
	writeFixture(t, path, `[{"x":30}]`)
	d.tick()
	if got := out.count(); got != 0 {
		t.Fatalf("прайминг при cur==true: срабатываний = %d, хотим 0", got)
	}

	// Уведём в ложь (база станет false), затем обратно в истину → срабатывание.
	writeFixture(t, path, `[{"x":1}]`)
	d.tick()
	writeFixture(t, path, `[{"x":30}]`)
	d.tick()
	if got := out.count(); got != 1 {
		t.Fatalf("переход после прайминга: срабатываний = %d, хотим 1", got)
	}
}

// TestEvalMetricsPrimedStatePersisted — состояние прайминга персистится (FR-010,
// SC-002): новый Daemon на той же БД НЕ повторяет прайминг. Условие УЖЕ истинно и
// база true → нового перехода нет (0 срабатываний на рестарте).
func TestEvalMetricsPrimedStatePersisted(t *testing.T) {
	path := fixturePath(t)
	st := store.NewMemoryStore()

	// Демон №1: прайминг при истинном условии (30 > 10) → база true, 0 срабатываний.
	out1 := &countWriter{marker: "FIRE"}
	writeFixture(t, path, `[{"x":30}]`)
	d1, _ := buildDaemon(t, metricSrc(path, "FIRE", 10), st, out1)
	d1.tick()
	if got := out1.count(); got != 0 {
		t.Fatalf("демон1 прайминг: срабатываний = %d, хотим 0", got)
	}

	// Состояние осталось в Store: LastBool=true.
	ts, err := st.LoadTriggerState("trg-0")
	if err != nil {
		t.Fatalf("LoadTriggerState после прайминга: %v", err)
	}
	if ts.LastBool == nil || !*ts.LastBool {
		t.Fatalf("прайминг записал LastBool=%v, хотим true", ts.LastBool)
	}

	// Демон №2 на той же БД: НЕ повторяет прайминг (база уже true), условие истинно
	// → нет перехода → 0 срабатываний (рестарт без ложного фаера, FR-010).
	out2 := &countWriter{marker: "FIRE"}
	d2, _ := buildDaemon(t, metricSrc(path, "FIRE", 10), st, out2)
	d2.tick()
	if got := out2.count(); got != 0 {
		t.Fatalf("демон2 (рестарт, истина→истина): срабатываний = %d, хотим 0", got)
	}
}

// TestEvalMetricsFreeze — невычислимая метрика (агрегат над ПУСТЫМ множеством →
// value.Пусто) замораживает триггер (FR-009): persist пропущен, тело не исполнено.
func TestEvalMetricsFreeze(t *testing.T) {
	path := fixturePath(t)
	out := &countWriter{marker: "FIRE"}
	st := store.NewMemoryStore()
	// среднее(x) над ПУСТЫМ набором → value.Пусто (§SM-8 шаг 5: деривативный/среднее
	// агрегат на пустом окне ⇒ Пусто, не 0). Сравнение Пусто > 10 невычислимо.
	src := "источник s:\n    файл: \"" + path + "\"\n" +
		"метрика m:\n    источник: s\n    агрегат: среднее(x)\n" +
		"когда метрика m > 10:\n    печать(\"FIRE\")\n"
	d, _ := buildDaemon(t, src, st, out)

	// Пустой источник → среднее(x) = Пусто → заморозка.
	writeFixture(t, path, `[]`)
	d.tick()
	if got := out.count(); got != 0 {
		t.Fatalf("заморозка: срабатываний = %d, хотим 0", got)
	}
	// persist пропущен: trigger_state НЕ создан (даже не праймлен).
	if _, err := st.LoadTriggerState("trg-0"); !stderrors.Is(err, store.ErrTriggerStateNotFound) {
		t.Fatalf("заморозка должна пропустить persist: err = %v, хотим ErrTriggerStateNotFound", err)
	}
}

// TestEvalMetricsAtMostOncePanicAfterPersist — at-most-once (FR-008): паника тела
// ПОСЛЕ персиста LastBool=true → recover изолирует → следующий тик НЕ ре-файрит
// (база уже true, перехода нет). База сдвинута ДО тела, поэтому сбойный триггер не
// зацикливается на повторном ложь→истина.
func TestEvalMetricsAtMostOncePanicAfterPersist(t *testing.T) {
	path := fixturePath(t)
	out := &countWriter{marker: "FIRE"}
	// panicStore паникует на NextInstanceID: тело «запустить процесс» уронит фаер.
	st := &panicStore{Store: store.NewMemoryStore()}

	src := "источник s:\n    файл: \"" + path + "\"\n" +
		"метрика m:\n    источник: s\n    агрегат: сумма(x)\n" +
		"процесс p:\n    шаг готово:\n        печать(\"шаг\")\n" +
		"когда метрика m > 10:\n    запустить процесс p\n"
	d, _ := buildDaemon(t, src, st, out)

	// Прайминг при лжи (3 > 10 == ложь).
	writeFixture(t, path, `[{"x":1},{"x":2}]`)
	d.tick()

	// Переход ложь→истина: persist LastBool=true ДО тела, затем тело паникует →
	// safeFire изолирует. Тик НЕ падает.
	writeFixture(t, path, `[{"x":30}]`)
	d.tick() // паника внутри изолирована — тест не падает

	// База персистнута true ДО паники.
	base := st.Store.(*store.MemoryStore)
	ts, err := base.LoadTriggerState("trg-0")
	if err != nil {
		t.Fatalf("LoadTriggerState после паники: %v", err)
	}
	if ts.LastBool == nil || !*ts.LastBool {
		t.Fatalf("persist ДО тела: LastBool=%v, хотим true (at-most-once)", ts.LastBool)
	}
	if !out.contains("сбой триггера изолирован") {
		t.Fatalf("ожидали лог изоляции паники, out=%q", out.String())
	}

	// Следующий тик при той же истине: перехода нет → тело не вызывается → паники
	// нет (сбойный триггер не зацикливается, FR-008).
	d.tick() // не должно паниковать/ре-файрить
}
