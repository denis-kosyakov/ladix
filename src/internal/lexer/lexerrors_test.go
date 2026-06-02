package lexer

import "testing"

// T025 [US2]: каталог лексических ошибок L-1…L-11 ДОСЛОВНО (двухстрочный формат
// через позицию+текст) — contracts/lexical-errors.md, SC-004. Тексты захардкожены
// независимо от реализации, чтобы ловить любое отклонение от канона (FR-024).
func TestLexErrorCatalog(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		line    int
		col     int
		wantMsg string
	}{
		{
			name: "L-1 таб в ведущих пробелах",
			src:  "\tx = 1",
			line: 1, col: 1,
			wantMsg: "табы в отступах запрещены, используйте пробелы",
		},
		{
			name: "L-2 таб между токенами",
			src:  "x\t= 1",
			line: 1, col: 2,
			wantMsg: "табы запрещены, используйте пробелы",
		},
		{
			name: "L-2 таб внутри строкового литерала",
			src:  "\"a\tb\"",
			line: 1, col: 3,
			wantMsg: "табы запрещены, используйте пробелы",
		},
		{
			name: "L-3 отступ не кратен 4",
			src:  "  x = 1",
			line: 1, col: 1,
			wantMsg: "отступ должен быть кратен 4 пробелам",
		},
		{
			name: "L-4 возврат на отсутствующий уровень",
			src:  "если истина:\n        x = 1\n    y = 2",
			line: 3, col: 1,
			wantMsg: "отступ не соответствует ни одному внешнему уровню",
		},
		{
			name: "L-5 незакрытая строка до EOF",
			src:  "x = \"abc",
			line: 1, col: 5,
			wantMsg: "незакрытый строковый литерал",
		},
		{
			name: "L-5 незакрытый строковый литерал (бэкслэш перед EOF)",
			src:  "\"abc\\",
			line: 1, col: 1,
			wantMsg: "незакрытый строковый литерал",
		},
		{
			name: "L-6 перевод строки внутри строки",
			src:  "\"abc\nx",
			line: 1, col: 1,
			wantMsg: "незакрытый строковый литерал (перевод строки внутри строки запрещён)",
		},
		{
			name: "L-7 неизвестная escape",
			src:  `"\q"`,
			line: 1, col: 2,
			wantMsg: "неизвестная escape-последовательность '\\q'",
		},
		{
			name: "L-8 неверный числовой литерал (форма)",
			src:  "1__000",
			line: 1, col: 1,
			wantMsg: "неверный числовой литерал '1__000'",
		},
		{
			name: "L-9 неизвестный суффикс длительности",
			src:  "5XYZ",
			line: 1, col: 1,
			wantMsg: "'5XYZ' — неизвестный суффикс длительности. Допустимые: сек, мин, час, дн, нед, мес.",
		},
		{
			name: "L-10 неожиданный символ",
			src:  "@",
			line: 1, col: 1,
			wantMsg: "неожиданный символ '@'",
		},
		{
			name: "L-10 одиночный '!'",
			src:  "!",
			line: 1, col: 1,
			wantMsg: "неожиданный символ '!'",
		},
		{
			name: "L-11 зарезервированное слово как имя",
			src:  "параллельно",
			line: 1, col: 1,
			wantMsg: "'параллельно' — зарезервированное слово, появится в будущих версиях Ladix. Использование как имени не допускается.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errs := lexAll(tt.src)
			le := onlyError(t, errs)
			if le.Pos.Line != tt.line || le.Pos.Col != tt.col {
				t.Errorf("позиция = %+v, хотим {Line:%d Col:%d}", le.Pos, tt.line, tt.col)
			}
			if le.Msg != tt.wantMsg {
				t.Errorf("Msg = %q,\nхотим = %q", le.Msg, tt.wantMsg)
			}
		})
	}
}
