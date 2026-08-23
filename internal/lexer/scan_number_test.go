package lexer

import "testing"

// T015 [US1]: числа и длительности (C-4).

func TestScanNumberValues(t *testing.T) {
	t.Run("5_000 → INT нормализован", func(t *testing.T) {
		toks, errs := lexAll("5_000")
		requireNoErrors(t, errs)
		requireTypes(t, toks, INT, NEWLINE, EOF)
		if toks[0].Lexeme != "5000" {
			t.Errorf("INT.Lexeme = %q, хотим 5000", toks[0].Lexeme)
		}
		if toks[0].Value != nil {
			t.Errorf("INT.Value = %v, хотим nil (без int64)", toks[0].Value)
		}
	})
	t.Run("3.14 → FLOAT", func(t *testing.T) {
		toks, errs := lexAll("3.14")
		requireNoErrors(t, errs)
		requireTypes(t, toks, FLOAT, NEWLINE, EOF)
		if f, ok := toks[0].Value.(float64); !ok || f != 3.14 {
			t.Errorf("FLOAT.Value = %v, хотим 3.14", toks[0].Value)
		}
	})
	t.Run("3дн → DURATION", func(t *testing.T) {
		toks, errs := lexAll("3дн")
		requireNoErrors(t, errs)
		requireTypes(t, toks, DURATION, NEWLINE, EOF)
		dv, ok := toks[0].Value.(DurationValue)
		if !ok || dv.Amount != "3" || dv.Unit != "дн" {
			t.Errorf("DURATION.Value = %+v, хотим {Amount:3 Unit:дн}", toks[0].Value)
		}
	})
	t.Run("1_000сек → DURATION нормализован", func(t *testing.T) {
		toks, errs := lexAll("1_000сек")
		requireNoErrors(t, errs)
		requireTypes(t, toks, DURATION, NEWLINE, EOF)
		dv := toks[0].Value.(DurationValue)
		if dv.Amount != "1000" || dv.Unit != "сек" {
			t.Errorf("DURATION.Value = %+v, хотим {Amount:1000 Unit:сек}", dv)
		}
	})
}

func TestScanNumberBoundaries(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []TokenType
	}{
		{"5.поле → INT·DOT·IDENT", "5.поле", []TokenType{INT, DOT, IDENT, NEWLINE, EOF}},
		{"5. → INT·DOT", "5.", []TokenType{INT, DOT, NEWLINE, EOF}},
		{"1.5дн → FLOAT·IDENT", "1.5дн", []TokenType{FLOAT, IDENT, NEWLINE, EOF}},
		{"пусть мин = 5 → мин это IDENT", "пусть мин = 5", []TokenType{KW_LET, IDENT, ASSIGN, INT, NEWLINE, EOF}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks, errs := lexAll(tt.src)
			requireNoErrors(t, errs)
			requireTypes(t, toks, tt.want...)
		})
	}
}

func TestScanNumberBoundaryDetails(t *testing.T) {
	t.Run("5.поле — части", func(t *testing.T) {
		toks, _ := lexAll("5.поле")
		if toks[0].Lexeme != "5" {
			t.Errorf("INT.Lexeme = %q, хотим 5", toks[0].Lexeme)
		}
		if toks[2].Lexeme != "поле" {
			t.Errorf("IDENT.Lexeme = %q, хотим поле", toks[2].Lexeme)
		}
	})
	t.Run("1.5дн — части", func(t *testing.T) {
		toks, _ := lexAll("1.5дн")
		if f, ok := toks[0].Value.(float64); !ok || f != 1.5 {
			t.Errorf("FLOAT.Value = %v, хотим 1.5", toks[0].Value)
		}
		if toks[1].Lexeme != "дн" {
			t.Errorf("IDENT.Lexeme = %q, хотим дн (суффикс за дробным НЕ читается)", toks[1].Lexeme)
		}
	})
}
