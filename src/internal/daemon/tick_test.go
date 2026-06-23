package daemon

import (
	"testing"
	"time"

	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
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

	// evalMetrics отработала: метрика-триггер праймлен (контентный ключ слота 0).
	if _, err := st.LoadTriggerState(trigKey(t, d, 0)); err != nil {
		t.Fatalf("evalMetrics не вызвана в фазе тика: метрика не праймлена (%v)", err)
	}
}

// TestTickPhaseOrderAllThreeFire — строгий порядок исполнения тел при срабатывании
// ВСЕХ трёх фаз в одном тике: drainEvents («E») → evalMetrics («M») → checkSchedules
// («S»). Программа: событие-триггер, метрика-триггер, расписание-триггер. Прайм-тик
// якорит метрику (база ложь) и расписание; срабатывающий тик: событие в очереди +
// метрика ложь→истина + расписание просрочено → вывод строго «E\nM\nS\n» (FR-002).
func TestTickPhaseOrderAllThreeFire(t *testing.T) {
	path := fixturePath(t)
	out := &countWriter{marker: "E"}
	st := store.NewMemoryStore()

	// trg-0 событие; trg-1 метрика (сумма(x) > 10); trg-2 расписание каждые 1дн.
	src := "источник s:\n    файл: \"" + path + "\"\n" +
		"метрика m:\n    источник: s\n    агрегат: сумма(x)\n" +
		"когда событие тик:\n    печать(\"E\")\n" +
		"когда метрика m > 10:\n    печать(\"M\")\n" +
		"когда расписание каждые 1дн:\n    печать(\"S\")\n"
	d, clk := buildDaemon(t, src, st, out)

	start := time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)
	setClock(clk, start)

	// Прайм-тик при метрике=ложь (сумма=1 ≤ 10): метрика праймлена false, расписание
	// заякорено, без срабатываний.
	writeFixture(t, path, `[{"x":1}]`)
	d.tick()
	if out.String() != "" {
		t.Fatalf("прайм-тик: ожидали пусто, out=%q", out.String())
	}

	// Срабатывающий тик: событие в очереди + метрика ложь→истина (сумма=30>10) +
	// расписание просрочено (через 25ч). Все три тела исполняются в порядке фаз.
	enqueue(t, st, "тик", `{}`, time.Unix(100, 0))
	writeFixture(t, path, `[{"x":30}]`)
	setClock(clk, start.Add(25*time.Hour))
	d.tick()

	// always-on explain (§C-5, serve withEdge): печатается через d.logf при фронте
	// ложь→истина ДО тела метрики → строка-explain между «E» (drainEvents) и «M»
	// (тело метрики). m=сумма(x)=30, оп >, порог 10.
	want := "E\n" +
		"триггер 'm > 10' сработал (ребро ложь→истина): m = 30 (снимок) > 10 (порог) → истина\n" +
		"M\nS\n"
	if got := out.String(); got != want {
		t.Fatalf("порядок фаз drainEvents→evalMetrics→checkSchedules: out=%q, хотим %q", got, want)
	}
}

// TestTickFourPhasesOrder — INV-1 (016 B4b, §AU-6.2.1): ЧЕТЫРЕ фазы в одном тике в
// строгом порядке ResetRunState → drainEvents → evalMetrics → checkSchedules →
// checkDeadlines. Маркеры тел: событие «E», метрика «M», расписание «S», эскалация-
// дедлайн «D» (через уведомить руководитель). Деадлайн-фаза — В ХВОСТ под тем же d.mu:
// вывод строго «E\nM\nS\nD\n». Замок аддитивности: первые три отрабатывают как до B4.
func TestTickFourPhasesOrder(t *testing.T) {
	path := fixturePath(t)
	out := &countWriter{marker: "D"}
	st := store.NewMemoryStore()

	// trg-0 событие; trg-1 метрика; trg-2 расписание; trg-3 эскалация-дедлайн.
	// Процесс эскалация_плана(факт) с человеческим шагом и сроком 2дн.
	src := "источник s:\n    файл: \"" + path + "\"\n" +
		"метрика m:\n    источник: s\n    агрегат: сумма(x)\n" +
		"процесс эскалация_плана(факт):\n    шаг связаться:\n        исполнитель: \"менеджер\"\n        срок: 2дн\n" +
		"когда событие тик:\n    печать(\"E\")\n" +
		"когда метрика m > 10:\n    печать(\"M\")\n" +
		"когда расписание каждые 1дн:\n    печать(\"S\")\n" +
		"когда задача просрочена в эскалация_плана.связаться:\n    печать(\"D\")\n"
	d, clk, eng, _ := buildDeadlineDaemon(t, src, st, out)

	start := time.Date(2026, 1, 10, 8, 0, 0, 0, time.UTC)
	setClock(clk, start)

	// Создаём инстанс: задача со сроком created+2дн, ожидание исполнителя.
	if _, err := eng.Start("эскалация_плана", []value.Value{value.Целое{V: 2500000}}); err != nil {
		t.Fatalf("eng.Start: %v", err)
	}
	out.reset() // отбросить системную строку «[задача] …» старта (печать ДО тиков)

	// Прайм-тик при метрике=ложь, до просрочки: метрика праймлена false, расписание
	// заякорено, задача НЕ просрочена → без срабатываний.
	writeFixture(t, path, `[{"x":1}]`)
	d.tick()
	if out.String() != "" {
		t.Fatalf("прайм-тик: ожидали пусто, out=%q", out.String())
	}

	// Срабатывающий тик: событие в очереди + метрика ложь→истина + расписание
	// просрочено (25ч) + задача просрочена (через 3дн > срок 2дн). Все четыре тела
	// исполняются в порядке фаз, deadline в хвосте.
	enqueue(t, st, "тик", `{}`, time.Unix(100, 0))
	writeFixture(t, path, `[{"x":30}]`)
	setClock(clk, start.Add(3*24*time.Hour))
	d.tick()

	// always-on explain (§C-5, serve withEdge): строка-explain между «E» и «M».
	want := "E\n" +
		"триггер 'm > 10' сработал (ребро ложь→истина): m = 30 (снимок) > 10 (порог) → истина\n" +
		"M\nS\nD\n"
	if got := out.String(); got != want {
		t.Fatalf("порядок 4 фаз …→checkSchedules→checkDeadlines: out=%q, хотим %q", got, want)
	}
}
