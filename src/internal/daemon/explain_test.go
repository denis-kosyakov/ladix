package daemon

import (
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/store"
)

// TestServeTriggerExplain (§C-5.4, T006a) — ALWAYS-ON explain на пути serve: при
// фронте ложь→истина метрика-триггера через d.logf печатается строка-explain §C-5.3
// С маркером ребра «(ребро ложь→истина)», ДО тела. Прайм-тик (метрика ложь) не даёт
// explain; срабатывающий тик (ложь→истина) — даёт ровно одну, ПЕРЕД маркером тела.
//
// 🔁 ИНВЕРСИЯ (мутпроба §C-5.4): (1) снять d.logf-emit в metrics.go → строки нет →
// краснеет; (2) не протянуть порог (threshold→None) → «порог» пустой/неверный →
// exact-substring краснеет.
func TestServeTriggerExplain(t *testing.T) {
	path := fixturePath(t)
	out := &countWriter{marker: "FIRE"}
	st := store.NewMemoryStore()
	// метрика m = сумма(x); триггер m > 10; тело печатает «FIRE».
	d, _ := buildDaemon(t, metricSrc(path, "FIRE", 10), st, out)

	// Прайм-тик при метрике=ложь (сумма=1 ≤ 10): без срабатывания → без explain.
	writeFixture(t, path, `[{"x":1}]`)
	d.tick()
	if out.String() != "" {
		t.Fatalf("прайм-тик: ожидали пусто (нет ребра), out=%q", out.String())
	}

	// Срабатывающий тик: ложь→истина (сумма=30 > 10). explain-строка serve-формы, затем
	// тело «FIRE». снимок=30, порог=10, оп >.
	writeFixture(t, path, `[{"x":30}]`)
	d.tick()

	wantExplain := "триггер 'm > 10' сработал (ребро ложь→истина): m = 30 (снимок) > 10 (порог) → истина\n"
	want := wantExplain + "FIRE\n"
	if got := out.String(); got != want {
		t.Errorf("serve-explain байт-не-точен:\n--- получено ---\n%s\n--- ожидание ---\n%s", got, want)
	}
	// explain строго ДО тела (порядок).
	if i := strings.Index(out.String(), "FIRE"); i < strings.Index(out.String(), "сработал") {
		t.Errorf("explain должен печататься ДО тела триггера, out=%q", out.String())
	}
}

// TestServeTriggerExplainSilenceWhenAlreadyTrue (§C-5.4, T006b) — explain привязан к
// РЕБРУ ложь→истина, не к проходу: на тике уже-истина (LastBool=true, cur=true, ребра
// нет) НОВОЙ explain-строки НЕТ. Доказательство тишины: фаер дал ровно одну строку,
// последующий истина→истина тик не добавляет вторую.
//
// 🔁 ИНВЕРСИЯ: если печатать explain вне ветки fired (на каждом истинном тике) →
// вторая строка появится → счётчик «сработал» = 2 → красный.
func TestServeTriggerExplainSilenceWhenAlreadyTrue(t *testing.T) {
	path := fixturePath(t)
	out := &countWriter{marker: "сработал"}
	st := store.NewMemoryStore()
	d, _ := buildDaemon(t, metricSrc(path, "FIRE", 10), st, out)

	// Прайм (ложь).
	writeFixture(t, path, `[{"x":1}]`)
	d.tick()

	// Ребро ложь→истина: ровно одна explain-строка.
	writeFixture(t, path, `[{"x":30}]`)
	d.tick()
	if got := out.count(); got != 1 {
		t.Fatalf("после ребра ложь→истина: explain-строк = %d, хотим 1", got)
	}

	// Истина→истина (метрика всё ещё 30 > 10, ребра нет): НОВОЙ explain-строки нет.
	d.tick()
	if got := out.count(); got != 1 {
		t.Errorf("истина→истина (нет ребра): explain-строк = %d, хотим 1 (тишина без ребра)", got)
	}
}
