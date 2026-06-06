package value

import "testing"

// TypeName — канонические имена всех 10 типов (для тип(x) и подстановки <тип>
// в сообщениях §8.3). Канон: «Целое»/«Дробное», НЕ «Число» (Guardrail 2, §1.3.10).
func TestTypeNameCanonical(t *testing.T) {
	tests := []struct {
		v    Value
		want string
	}{
		{Целое{V: 42}, "Целое"},
		{Дробное{V: 3.14}, "Дробное"},
		{Строка{V: "привет"}, "Строка"},
		{Булево{V: true}, "Булево"},
		{None, "Пусто"},
		{Пусто{}, "Пусто"},
		{NewList(nil), "Список"},
		{NewRecord(nil, nil), "Запись"},
		{Дата{}, "Дата"},
		{Длительность{}, "Длительность"},
		{Период{}, "Период"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.v.TypeName(); got != tt.want {
				t.Errorf("TypeName() = %q, хотим %q", got, tt.want)
			}
		})
	}
}

// None — синглтон Пусто и реализует Value.
func TestNoneIsPustoSingleton(t *testing.T) {
	var _ Value = None
	if None.TypeName() != "Пусто" {
		t.Fatalf("None.TypeName() = %q", None.TypeName())
	}
	if !Equal(None, Пусто{}) {
		t.Errorf("None != Пусто{}")
	}
}
