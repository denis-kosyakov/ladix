package eval

import "github.com/denis-kosyakov/ladix/internal/value"

// SignalKind — вид штатного управляющего сигнала (control-flow без panic, D4).
type SignalKind int

const (
	// SigNormal — обычное завершение, исполнение продолжается.
	SigNormal SignalKind = iota
	// SigBreak — прервать (поглощается ближайшим циклом).
	SigBreak
	// SigContinue — продолжить (поглощается ближайшим циклом).
	SigContinue
	// SigReturn — вернуть (несёт значение; ловится вызовом функции).
	SigReturn
)

// Signal — штатный поток управления, отдельный от канала error. Value значимо
// ТОЛЬКО при SigReturn (Guardrail 8).
type Signal struct {
	Kind  SignalKind
	Value value.Value
}
