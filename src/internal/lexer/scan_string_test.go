package lexer

import "testing"

// T016 [US1]: STRING с раскрытыми escape; `#` внутри строки — обычный символ.

func TestScanStringValue(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"простая", `"привет"`, "привет"},
		{"escape раскрыты", `"a\nb\tc\\d\"e"`, "a\nb\tc\\d\"e"},
		{"диез внутри строки — обычный символ", `"a#b"`, "a#b"},
		{"пустая строка", `""`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks, errs := lexAll(tt.src)
			requireNoErrors(t, errs)
			requireTypes(t, toks, STRING, NEWLINE, EOF)
			got, ok := toks[0].Value.(string)
			if !ok {
				t.Fatalf("STRING.Value не string: %T", toks[0].Value)
			}
			if got != tt.want {
				t.Errorf("значение = %q, хотим %q", got, tt.want)
			}
		})
	}
}
