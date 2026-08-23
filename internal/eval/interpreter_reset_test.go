package eval

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// TestResetRunStateRecomputesMetric — мутационная проверка ResetRunState (T011,
// решение #2, US1 №4, SC-001/SC-004). Моделирует «тик» демона: метрика над
// JSON-источником вычисляется, потом файл-фикстура меняется между «тиками».
//
// БЕЗ сброса метрика возвращает тот же снимок (i.recordCache живёт «на запуск»,
// §9.6) → edge-детект мёртв. ПОСЛЕ ResetRunState кеш источника пуст → источник
// перечитывается → метрика отдаёт свежий снимок. Тест доказывает оба плеча:
// (1) кеш реально кешировал (промежуточное чтение всё ещё старое); (2) сброс
// реально активен (после него — новое значение).
func TestResetRunStateRecomputesMetric(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	// «тик 1»: две записи x=1, x=2 → сумма(x)=3.
	if err := os.WriteFile(path, []byte(`[{"x":1},{"x":2}]`), 0o644); err != nil {
		t.Fatalf("запись фикстуры: %v", err)
	}

	src := "источник s:\n    файл: \"" + path + "\"\n" +
		"метрика m:\n    источник: s\n    агрегат: сумма(x)\n"
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные ошибки разбора: %v", errList.Error())
	}
	interp := NewInterpreter(&bytes.Buffer{}, 0, testClock)
	if err := interp.Run(prog); err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}

	// Первый расчёт метрики — кеширует записи (3) и фиксирует i.today.
	v1, err := interp.EvalMetricByName("m")
	if err != nil {
		t.Fatalf("EvalMetricByName(тик1): %v", err)
	}
	if got, ok := v1.(value.Целое); !ok || got.V != 3 {
		t.Fatalf("сумма(x) тик1 = %v (%T), хотим Целое 3", v1, v1)
	}

	// «тик 2»: меняем файл (теперь x=10, x=20, x=30 → 60).
	if err := os.WriteFile(path, []byte(`[{"x":10},{"x":20},{"x":30}]`), 0o644); err != nil {
		t.Fatalf("мутация фикстуры: %v", err)
	}

	// БЕЗ сброса — старый снимок из кеша-на-запуск (доказывает, что кеш реален).
	vStale, err := interp.EvalMetricByName("m")
	if err != nil {
		t.Fatalf("EvalMetricByName(до сброса): %v", err)
	}
	if got, ok := vStale.(value.Целое); !ok || got.V != 3 {
		t.Fatalf("без ResetRunState сумма(x) = %v, ожидали устаревшие 3 (кеш-на-запуск)", vStale)
	}

	// ПОСЛЕ сброса — кеш пуст, источник перечитан, свежий снимок 60.
	interp.ResetRunState()
	if interp.today != nil {
		t.Errorf("ResetRunState не сбросил i.today")
	}
	if len(interp.recordCache) != 0 {
		t.Errorf("ResetRunState не очистил recordCache (len=%d)", len(interp.recordCache))
	}
	v2, err := interp.EvalMetricByName("m")
	if err != nil {
		t.Fatalf("EvalMetricByName(тик2): %v", err)
	}
	if got, ok := v2.(value.Целое); !ok || got.V != 60 {
		t.Fatalf("после ResetRunState сумма(x) = %v (%T), хотим свежие Целое 60", v2, v2)
	}
}

// TestTriggersAccessor — Triggers() отдаёт реестр, собранный Analyze Шаг 1d, в
// порядке объявления (T010). Демон перечисляет его для исполнения тиков.
func TestTriggersAccessor(t *testing.T) {
	const metric = "источник s:\n    файл: \"d.json\"\nметрика m:\n    источник: s\n    агрегат: сумма(x)\n"
	src := metric +
		"когда метрика m > 1:\n    печать(1)\n" +
		"когда событие e:\n    печать(2)\n" +
		"когда расписание каждые 1час:\n    печать(3)\n"
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные ошибки разбора: %v", errList.Error())
	}
	interp := NewInterpreter(&bytes.Buffer{}, 0, testClock)
	if err := interp.Analyze(prog); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	trs := interp.Triggers()
	if len(trs) != 3 {
		t.Fatalf("Triggers() len = %d, хотим 3", len(trs))
	}
	// Порядок объявления сохранён: метрика, событие, расписание.
	if _, ok := trs[0].Spec.(*ast.MetricTrigger); !ok {
		t.Errorf("trs[0].Spec = %T, хотим *ast.MetricTrigger", trs[0].Spec)
	}
	if _, ok := trs[1].Spec.(*ast.EventTrigger); !ok {
		t.Errorf("trs[1].Spec = %T, хотим *ast.EventTrigger", trs[1].Spec)
	}
	if _, ok := trs[2].Spec.(*ast.ScheduleTrigger); !ok {
		t.Errorf("trs[2].Spec = %T, хотим *ast.ScheduleTrigger", trs[2].Spec)
	}
}
