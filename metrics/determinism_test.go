package metrics

// Задача 2.4 — Детерминизм исполнения (design.md Д-3, spec.md Requirement
// «Детерминизм исполнения»). НИКАКОГО time.Now() — дата инжектируется через
// Options.Today; тесты доказывают, что: (1) повторный запуск даёт идентичный
// результат; (2) порядок ключей внутри map не влияет на результат; (3) окно
// периода строится от инжектированной даты, а не от системного времени.

import (
	"reflect"
	"testing"
)

// TestDeterminismRepeatedRun — Scenario «Повторный запуск»: 100 итераций
// одного входа дают идентичные Result и диагностики.
func TestDeterminismRepeatedRun(t *testing.T) {
	m := baseMetric(`(статус == "оплачен")`, "сумма(сумма_заказа)", "", "")
	records := salesRecords()
	opts := Options{Today: may31()}

	first, firstDiags, firstErr := Evaluate(m, records, opts)
	for i := 0; i < 100; i++ {
		got, diags, err := Evaluate(m, records, opts)
		if (err == nil) != (firstErr == nil) {
			t.Fatalf("итерация %d: наличие ошибки разошлось: %v vs %v", i, err, firstErr)
		}
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("итерация %d: Result разошёлся: %+v vs %+v", i, got, first)
		}
		if !reflect.DeepEqual(diags, firstDiags) {
			t.Fatalf("итерация %d: диагностики разошлись: %+v vs %+v", i, diags, firstDiags)
		}
	}
}

// TestDeterminismMapKeyOrderIndependent — spec.md «без зависимости от
// порядка обхода map» (design.md Д-9: ключи записи сортируются лексикографически
// до построения value.Запись). Одни и те же записи, поданные как map-литералы с
// разным порядком добавления ключей (в Go порядок итерации map недетерминирован
// в принципе, но сама КАРТА map[string]any не хранит порядок вставки — здесь
// строим две карты буквально в разном коде, значение одинаковое), должны дать
// идентичный результат.
func TestDeterminismMapKeyOrderIndependent(t *testing.T) {
	recA := map[string]any{}
	recA["статус"] = "оплачен"
	recA["сумма_заказа"] = int64(1200000)
	recA["ид"] = int64(1)
	recA["дата_заказа"] = "2026-05-03"

	recB := map[string]any{}
	recB["дата_заказа"] = "2026-05-03"
	recB["ид"] = int64(1)
	recB["сумма_заказа"] = int64(1200000)
	recB["статус"] = "оплачен"

	m := baseMetric(`(статус == "оплачен")`, "сумма(сумма_заказа)", "", "")
	opts := Options{Today: may31()}

	gotA, diagsA, errA := Evaluate(m, []map[string]any{recA}, opts)
	gotB, diagsB, errB := Evaluate(m, []map[string]any{recB}, opts)

	if (errA == nil) != (errB == nil) {
		t.Fatalf("наличие ошибки разошлось между порядками ключей: %v vs %v", errA, errB)
	}
	if !reflect.DeepEqual(gotA, gotB) {
		t.Errorf("Result разошёлся при разном порядке ключей map: %+v vs %+v", gotA, gotB)
	}
	if !reflect.DeepEqual(diagsA, diagsB) {
		t.Errorf("диагностики разошлись при разном порядке ключей map: %+v vs %+v", diagsA, diagsB)
	}
}

// TestDeterminismWindowFromInjectedClock — Scenario «Окно периода от
// инжектированных часов»: одна и та же метрика с период: ежемесячно, две
// разные Options.Today (май и июнь 2026) над записями только в мае → два
// РАЗНЫХ результата, каждый стабилен при повторном вызове. Системное время
// не участвует нигде (никакого time.Now() в этом пакете и в тестах).
func TestDeterminismWindowFromInjectedClock(t *testing.T) {
	m := baseMetric("", "сумма(сумма_заказа)", "ежемесячно", "дата_заказа")
	records := salesRecords() // все три записи датированы маем 2026

	mayResult, mayDiags, mayErr := Evaluate(m, records, Options{Today: may31(), Fields: maySchema()})
	if mayErr != nil {
		t.Fatalf("май: Evaluate вернул ошибку: %v", mayErr)
	}
	if len(mayDiags) != 0 {
		t.Fatalf("май: диагностики %+v, ожидалось ни одной", mayDiags)
	}

	juneResult, juneDiags, juneErr := Evaluate(m, records, Options{Today: jun15(), Fields: maySchema()})
	if juneErr != nil {
		t.Fatalf("июнь: Evaluate вернул ошибку: %v", juneErr)
	}
	if len(juneDiags) != 0 {
		t.Fatalf("июнь: диагностики %+v, ожидалось ни одной", juneDiags)
	}

	if reflect.DeepEqual(mayResult, juneResult) {
		t.Errorf("окно мая и окно июня дали ОДИНАКОВЫЙ результат %+v — окно не привязано к Options.Today", mayResult)
	}

	// Стабильность каждого результата при повторном вызове с той же датой.
	mayAgain, _, _ := Evaluate(m, records, Options{Today: may31(), Fields: maySchema()})
	if !reflect.DeepEqual(mayAgain, mayResult) {
		t.Errorf("майский результат нестабилен: %+v vs %+v", mayAgain, mayResult)
	}
	juneAgain, _, _ := Evaluate(m, records, Options{Today: jun15(), Fields: maySchema()})
	if !reflect.DeepEqual(juneAgain, juneResult) {
		t.Errorf("июньский результат нестабилен: %+v vs %+v", juneAgain, juneResult)
	}

	// Ожидаемые значения зафиксированы явно (не только "разные"): май включает
	// все 3 записи (2500000), июнь исключает все майские записи (0 — Целое,
	// т.к. корневой агрегат — единичный сумма(...), §10.5).
	if mayResult.Text != "2500000" {
		t.Errorf("майский Text = %q, хотим %q", mayResult.Text, "2500000")
	}
	if juneResult.Text != "0" {
		t.Errorf("июньский Text = %q, хотим %q", juneResult.Text, "0")
	}
}
