package eval

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
)

// runExample исполняет examples/<name> на уровне интерпретатора (конвейер
// лексер→парсер→Run, без CLI) и возвращает stdout.
func runExample(t *testing.T, name string) (string, error) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", name))
	if err != nil {
		t.Fatalf("чтение %s: %v", name, err)
	}
	tokens, errList := lexer.New(string(data)).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("%s: лексические/синтаксические ошибки: %v", name, errList.Error())
	}
	var buf bytes.Buffer
	interp := NewInterpreter(&buf, 0)
	runErr := interp.Run(prog)
	return buf.String(), runErr // stdout (включая успевшее напечататься) + ошибка
}

// Golden-прогон на уровне интерпретатора (§10.3): 6 примеров → эталонный stdout.
func TestGoldenInterpreter(t *testing.T) {
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
			out, err := runExample(t, tt.name)
			if err != nil {
				t.Fatalf("%s: ошибка исполнения: %v", tt.name, err)
			}
			if out != tt.want {
				t.Errorf("%s:\nполучено %q\nхотим   %q", tt.name, out, tt.want)
			}
		})
	}
}

// ошибка.ladix — деление на ноль на строке 5, колонке 14 (контрольный SC-002).
func TestGoldenErrorExample(t *testing.T) {
	_, err := runExample(t, "ошибка.ladix")
	if err == nil {
		t.Fatalf("ожидалась ошибка деления на ноль")
	}
	line, col, msg := evalErr(t, err)
	if msg != "деление на ноль" {
		t.Errorf("msg = %q", msg)
	}
	if line != 5 || col != 14 {
		t.Errorf("позиция = (%d,%d), хотим (5,14)", line, col)
	}
}
