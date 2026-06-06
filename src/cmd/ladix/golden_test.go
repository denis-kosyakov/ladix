package main

import (
	"bytes"
	"testing"
)

// T048/SC-001: сквозной CLI-golden всех 6 обязательных примеров — байт-в-байт
// stdout (§10.3) и код возврата 0. выручка/онбординг НЕ включены (отдельный трек).
func TestCLIGoldenStdout(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"hello.ladix", "Привет, Уклад!\n"},
		{"арифметика.ladix", "14 20\n3 2\n3.4\n25\nистина\nистина\n"},
		{"условие.ladix", "категория: средний\nчётная сумма\n"},
		{"цикл.ladix", "сумма чётных: 30\nдорогие: [250, 400]\nпервая степень двойки > 16: 32\n"},
		{"функция.ladix", "2175.0\n0\n"},
		{"факториал.ladix", "120\n1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := realMain([]string{"run", examplePath(tt.name)}, &out, &errBuf)
			if code != 0 {
				t.Fatalf("%s: код = %d, хотим 0; stderr=%q", tt.name, code, errBuf.String())
			}
			if out.String() != tt.want {
				t.Errorf("%s:\nполучено %q\nхотим   %q", tt.name, out.String(), tt.want)
			}
		})
	}
}
