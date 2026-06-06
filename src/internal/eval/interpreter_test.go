package eval

import (
	"bytes"
	"testing"
)

// out инжектируется (тест перехватывает через bytes.Buffer); maxDepth по умолчанию
// 10000 (D9, FR-021).
func TestNewInterpreterDefaults(t *testing.T) {
	var buf bytes.Buffer
	i := NewInterpreter(&buf, 0)
	if i.maxDepth != DefaultMaxDepth {
		t.Errorf("maxDepth по умолчанию = %d, хотим %d", i.maxDepth, DefaultMaxDepth)
	}
	if i.out != &buf {
		t.Errorf("out не инжектирован")
	}
	if i.global == nil || i.funcs == nil || i.builtins == nil || i.iterating == nil {
		t.Errorf("незаполненные поля интерпретатора")
	}
}

func TestNewInterpreterExplicitMaxDepth(t *testing.T) {
	i := NewInterpreter(&bytes.Buffer{}, 50)
	if i.maxDepth != 50 {
		t.Errorf("maxDepth = %d, хотим 50", i.maxDepth)
	}
}
