package main

import (
	"strings"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/value"
)

// TestParseArgLiteralTypes — табличный замок порядка распознавания форм argv
// (data-model §1.2, инверсия b): целое → дробное → булево/пусто → дата(ISO) →
// строка(fallback). Каждая форма парсится В СВОЙ тип; нераспознанное → Строка.
func TestParseArgLiteralTypes(t *testing.T) {
	cases := []struct {
		argv string
		want value.Value
	}{
		{"2500000", value.Целое{V: 2500000}},
		{"-42", value.Целое{V: -42}},
		{"0", value.Целое{V: 0}},
		{"3.14", value.Дробное{V: 3.14}},
		{"-0.5", value.Дробное{V: -0.5}},
		{"1e3", value.Дробное{V: 1000}},
		{"истина", value.Булево{V: true}},
		{"ложь", value.Булево{V: false}},
		{"пусто", value.None},
		{"2026-01-01", value.Дата{Year: 2026, Month: 1, Day: 1}},
		{"перезвонит", value.Строка{V: "перезвонит"}},
		{"2 500 000", value.Строка{V: "2 500 000"}},
	}
	for _, c := range cases {
		got, err := parseArgLiteral(c.argv)
		if err != nil {
			t.Errorf("parseArgLiteral(%q) → ошибка %v, хотим значение %v", c.argv, err, c.want)
			continue
		}
		if got != c.want {
			t.Errorf("parseArgLiteral(%q) = %#v, хотим %#v", c.argv, got, c.want)
		}
	}
}

// TestParseArgLiteralErrors — замок ошибок распознанной формы (инверсия b):
// целое вне Int64 и невалидная ISO-дата → ошибка (НЕ молчаливый fallback в Строку).
func TestParseArgLiteralErrors(t *testing.T) {
	cases := []struct {
		argv    string
		wantSub string
	}{
		{"99999999999999999999", "целое вне диапазона типа Целое"},
		{"2026-13-45", "дата"}, // невалидный календарь → ошибка парса даты
	}
	for _, c := range cases {
		v, err := parseArgLiteral(c.argv)
		if err == nil {
			t.Errorf("parseArgLiteral(%q) → значение %#v, хотим ошибку (%s)", c.argv, v, c.wantSub)
			continue
		}
		if !strings.Contains(err.Error(), c.wantSub) {
			t.Errorf("parseArgLiteral(%q) ошибка = %q, хотим подстроку %q", c.argv, err.Error(), c.wantSub)
		}
	}
}
