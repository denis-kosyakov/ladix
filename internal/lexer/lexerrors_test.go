package lexer

import (
	"strings"
	"testing"
)

// T025 [US2]: каталог лексических ошибок L-1…L-11 ДОСЛОВНО (двухстрочный формат
// через позицию+текст) — contracts/lexical-errors.md, SC-004. Тексты захардкожены
// независимо от реализации, чтобы ловить любое отклонение от канона.
// DX2 (фича 012): бизнес-формулировки scope A (L-5/L-6/L-8), канон —
// docs/diagnostics-model.md §MDX-1. Поле code — для инвентарь-замка полноты ниже.
func TestLexErrorCatalog(t *testing.T) {
	for _, tt := range lexCatalogCases {
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

// TestLexCatalogInventory — инвентарь-замок полноты каталога лексики (DX2, FR-014,
// SC-006): каталог покрывает РОВНО 11 кодов L-1..L-11. Замок «кусается» при
// выпадении кода из покрытия или добавлении нового без обновления числа (образец —
// eval/errors_golden_test.go:205 len(seen)!=28).
func TestLexCatalogInventory(t *testing.T) {
	// Сверка с КАНОНИЧЕСКИМ множеством {L-1..L-11}, а не только с числом: ловит и
	// выпадение/добавление кода, И переименование (напр. L-8→L-99) — замок «кусается»
	// на любом дрейфе реестра, не вакуумно по кардинальности.
	want := map[string]bool{
		"L-1": true, "L-2": true, "L-3": true, "L-4": true, "L-5": true, "L-6": true,
		"L-7": true, "L-8": true, "L-9": true, "L-10": true, "L-11": true,
	}
	seen := make(map[string]bool, len(want))
	for _, tt := range lexCatalogCases {
		seen[tt.code] = true
	}
	for code := range want {
		if !seen[code] {
			t.Errorf("каталог лексики НЕ покрывает %s (реестр L-1..L-11 неполон)", code)
		}
	}
	for code := range seen {
		if !want[code] {
			t.Errorf("каталог лексики содержит неожиданный код %q (ожидались только L-1..L-11)", code)
		}
	}
	// Замок DX2 (SC-007): scope-A лексические тексты без жаргона «литерал»/«токен»
	// и без кодов (L-). «escape-последовательность»/«суффикс длительности» сохранены
	// осознанно (docs/diagnostics-model.md §MDX-1) — не класс «токен/литерал».
	for _, tt := range lexCatalogCases {
		for _, bad := range []string{"литерал", "токен"} {
			if strings.Contains(tt.wantMsg, bad) {
				t.Errorf("L-текст %q содержит жаргон %q: %q", tt.code, bad, tt.wantMsg)
			}
		}
	}
}

type lexCatalogCase struct {
	code    string
	name    string
	src     string
	line    int
	col     int
	wantMsg string
}

var lexCatalogCases = []lexCatalogCase{
	{
		code: "L-1",
		name: "L-1 таб в ведущих пробелах",
		src:  "\tx = 1",
		line: 1, col: 1,
		wantMsg: "табы в отступах запрещены, используйте пробелы",
	},
	{
		code: "L-2",
		name: "L-2 таб между токенами",
		src:  "x\t= 1",
		line: 1, col: 2,
		wantMsg: "табы запрещены, используйте пробелы",
	},
	{
		code: "L-2",
		name: "L-2 таб внутри строкового литерала",
		src:  "\"a\tb\"",
		line: 1, col: 3,
		wantMsg: "табы запрещены, используйте пробелы",
	},
	{
		code: "L-3",
		name: "L-3 отступ не кратен 4",
		src:  "  x = 1",
		line: 1, col: 1,
		wantMsg: "отступ должен быть кратен 4 пробелам",
	},
	{
		code: "L-4",
		name: "L-4 возврат на отсутствующий уровень",
		src:  "если истина:\n        x = 1\n    y = 2",
		line: 3, col: 1,
		wantMsg: "отступ не соответствует ни одному внешнему уровню",
	},
	{
		code: "L-5",
		name: "L-5 незакрытая строка до EOF",
		src:  "x = \"abc",
		line: 1, col: 5,
		wantMsg: "незакрытая строка в кавычках",
	},
	{
		code: "L-5",
		name: "L-5 незакрытая строка в кавычках (бэкслэш перед EOF)",
		src:  "\"abc\\",
		line: 1, col: 1,
		wantMsg: "незакрытая строка в кавычках",
	},
	{
		code: "L-6",
		name: "L-6 перевод строки внутри строки",
		src:  "\"abc\nx",
		line: 1, col: 1,
		wantMsg: "незакрытая строка в кавычках (перевод строки внутри строки запрещён)",
	},
	{
		code: "L-7",
		name: "L-7 неизвестная escape",
		src:  `"\q"`,
		line: 1, col: 2,
		wantMsg: "неизвестная escape-последовательность '\\q'",
	},
	{
		code: "L-8",
		name: "L-8 неверная запись числа (форма)",
		src:  "1__000",
		line: 1, col: 1,
		wantMsg: "неверная запись числа '1__000'",
	},
	{
		code: "L-9",
		name: "L-9 неизвестный суффикс длительности",
		src:  "5XYZ",
		line: 1, col: 1,
		wantMsg: "'5XYZ' — неизвестный суффикс длительности. Допустимые: сек, мин, час, дн, нед, мес.",
	},
	{
		code: "L-10",
		name: "L-10 неожиданный символ",
		src:  "@",
		line: 1, col: 1,
		wantMsg: "неожиданный символ '@'",
	},
	{
		code: "L-10",
		name: "L-10 одиночный '!'",
		src:  "!",
		line: 1, col: 1,
		wantMsg: "неожиданный символ '!'",
	},
	{
		code: "L-11",
		name: "L-11 зарезервированное слово как имя",
		src:  "параллельно",
		line: 1, col: 1,
		wantMsg: "'параллельно' — зарезервированное слово, появится в будущих версиях Ladix. Использование как имени не допускается.",
	},
}
