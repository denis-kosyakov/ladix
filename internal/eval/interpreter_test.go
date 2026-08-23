package eval

import (
	"bytes"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// testClock — фиксированная дата, нерелевантная этим тестам (нет сегодня()/метрик).
var testClock = FixedClock{D: value.Дата{Year: 2026, Month: 5, Day: 31}}

// out инжектируется (тест перехватывает через bytes.Buffer); maxDepth по умолчанию
// 10000 (D9, FR-021).
func TestNewInterpreterDefaults(t *testing.T) {
	var buf bytes.Buffer
	i := NewInterpreter(&buf, 0, testClock)
	if i.maxDepth != DefaultMaxDepth {
		t.Errorf("maxDepth по умолчанию = %d, хотим %d", i.maxDepth, DefaultMaxDepth)
	}
	if i.out != &buf {
		t.Errorf("out не инжектирован")
	}
	if i.global == nil || i.funcs == nil || i.builtins == nil || i.iterating == nil {
		t.Errorf("незаполненные поля интерпретатора")
	}
	if i.clock == nil {
		t.Errorf("clock не инжектирован")
	}
}

func TestNewInterpreterExplicitMaxDepth(t *testing.T) {
	i := NewInterpreter(&bytes.Buffer{}, 50, testClock)
	if i.maxDepth != 50 {
		t.Errorf("maxDepth = %d, хотим 50", i.maxDepth)
	}
}

// 5 предопределённых Период регистрируются в global как read-only иденты (§SM-5
// §2.2); ежемесячно резолвится в value.Период{Name:"ежемесячно"}.
func TestNewInterpreterRegistersPeriods(t *testing.T) {
	i := NewInterpreter(&bytes.Buffer{}, 0, testClock)
	v, ok := i.global.Lookup("ежемесячно")
	if !ok {
		t.Fatalf("ежемесячно не зарегистрирован в global")
	}
	p, ok := v.(value.Период)
	if !ok {
		t.Fatalf("ежемесячно резолвится в %T, хотим value.Период", v)
	}
	if p.Name != "ежемесячно" {
		t.Errorf("Период.Name = %q, хотим \"ежемесячно\"", p.Name)
	}
	for _, name := range value.PeriodNames {
		if _, ok := i.global.Lookup(name); !ok {
			t.Errorf("период %q не зарегистрирован", name)
		}
	}
}
