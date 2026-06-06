package eval

import (
	"testing"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// Signal.Value значим только при SigReturn (Guardrail 8).
func TestSignalReturnCarriesValue(t *testing.T) {
	s := Signal{Kind: SigReturn, Value: value.Целое{V: 7}}
	if s.Kind != SigReturn {
		t.Fatalf("Kind = %v", s.Kind)
	}
	if !value.Equal(s.Value, value.Целое{V: 7}) {
		t.Errorf("Value = %v, хотим 7", s.Value)
	}
	// нулевой сигнал — SigNormal
	var zero Signal
	if zero.Kind != SigNormal {
		t.Errorf("нулевой Signal.Kind = %v, хотим SigNormal", zero.Kind)
	}
}
