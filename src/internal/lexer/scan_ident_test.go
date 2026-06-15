package lexer

import "testing"

// T014 [US1]: IDENT / ключевые / короткие слова; BOOL/NONE.

func TestScanIdentFullWordMatch(t *testing.T) {
	// Сверка по ПОЛНОЙ строке: это IDENT, а не зарезервированные/ключевые слова.
	idents := []string{"тип_клиента", "или_нет", "вход", "мин", "x1", "_local", "Заказ"}
	for _, w := range idents {
		t.Run(w, func(t *testing.T) {
			toks, errs := lexAll(w)
			requireNoErrors(t, errs)
			requireTypes(t, toks, IDENT, NEWLINE, EOF)
			if toks[0].Lexeme != w {
				t.Errorf("IDENT.Lexeme = %q, хотим %q", toks[0].Lexeme, w)
			}
		})
	}
}

func TestKeywordTable(t *testing.T) {
	table := map[string]TokenType{
		"пусть": KW_LET, "функция": KW_FUNC, "вернуть": KW_RETURN,
		"если": KW_IF, "иначе": KW_ELSE, "пока": KW_WHILE, "для": KW_FOR,
		"прервать": KW_BREAK, "продолжить": KW_CONTINUE,
		"и": KW_AND, "или": KW_OR, "не": KW_NOT,
		"источник": KW_SOURCE, "файл": KW_FILE, "метрика": KW_METRIC, "где": KW_WHERE,
		"агрегат": KW_AGGREGATE, "период": KW_PERIOD, "по_дате": KW_BY_DATE,
		"процесс": KW_PROCESS, "шаг": KW_STEP, "исполнитель": KW_ASSIGNEE, "срок": KW_DEADLINE,
		"после": KW_AFTER, "присвоить": KW_SET, "вызвать": KW_CALL, "уведомить": KW_NOTIFY,
		"когда": KW_WHEN, "событие": KW_EVENT, "значение": KW_VALUE, "расписание": KW_SCHEDULE,
		"каждые": KW_EVERY, "в": KW_IN, "запустить": KW_RUN,
		"тип": KW_TYPE, // 010-A1 §SC-D-RESERVE: разрезервировано из reservedWords
	}
	if len(table) != 35 {
		t.Fatalf("в таблице теста %d ключевых слов, хотим 35", len(table))
	}
	for w, want := range table {
		t.Run(w, func(t *testing.T) {
			toks, errs := lexAll(w)
			requireNoErrors(t, errs)
			requireTypes(t, toks, want, NEWLINE, EOF)
			if toks[0].Lexeme != w {
				t.Errorf("Lexeme = %q, хотим %q", toks[0].Lexeme, w)
			}
		})
	}
}

func TestBoolNoneLiterals(t *testing.T) {
	t.Run("истина → BOOL true", func(t *testing.T) {
		toks, errs := lexAll("истина")
		requireNoErrors(t, errs)
		requireTypes(t, toks, BOOL, NEWLINE, EOF)
		if b, ok := toks[0].Value.(bool); !ok || b != true {
			t.Errorf("Value = %v, хотим true", toks[0].Value)
		}
	})
	t.Run("ложь → BOOL false", func(t *testing.T) {
		toks, errs := lexAll("ложь")
		requireNoErrors(t, errs)
		requireTypes(t, toks, BOOL, NEWLINE, EOF)
		if b, ok := toks[0].Value.(bool); !ok || b != false {
			t.Errorf("Value = %v, хотим false", toks[0].Value)
		}
	})
	t.Run("пусто → NONE без значения", func(t *testing.T) {
		toks, errs := lexAll("пусто")
		requireNoErrors(t, errs)
		requireTypes(t, toks, NONE, NEWLINE, EOF)
		if toks[0].Value != nil {
			t.Errorf("NONE.Value = %v, хотим nil", toks[0].Value)
		}
	})
}

func TestShortKeywordsOnlyAsFullWord(t *testing.T) {
	// и/или/не/в как полные слова — ключевые; в составе длинного слова — часть IDENT.
	toks, errs := lexAll("и или не в")
	requireNoErrors(t, errs)
	requireTypes(t, toks, KW_AND, KW_OR, KW_NOT, KW_IN, NEWLINE, EOF)
}

// T001 (010-A1, §SC-3/§SC-D-RESERVE/§SC-D-LEX-MIN, конституция VI): ТЕСТ-ЗАМОК
// разрезервирования `тип`. `тип` становится ключевым словом KW_TYPE (НЕ IDENT —
// иначе `тип(5)` распарсился бы как вызов builtin и СЛУЧАЙНО активировал бы
// `тип(x)`, что хартия запрещает; НЕ lex-ошибка — он более НЕ зарезервирован).
// `поля` ОБЯЗАН остаться IDENT (v1-программа может звать переменную `поля`).
func TestTypeKeywordReserveLift(t *testing.T) {
	t.Run("тип → KW_TYPE (а не IDENT, не lex-ошибка)", func(t *testing.T) {
		toks, errs := lexAll("тип")
		requireNoErrors(t, errs)
		requireTypes(t, toks, KW_TYPE, NEWLINE, EOF)
		if toks[0].Lexeme != "тип" {
			t.Errorf("KW_TYPE.Lexeme = %q, хотим %q", toks[0].Lexeme, "тип")
		}
	})
	t.Run("поля остаётся IDENT", func(t *testing.T) {
		toks, errs := lexAll("поля")
		requireNoErrors(t, errs)
		requireTypes(t, toks, IDENT, NEWLINE, EOF)
		if toks[0].Lexeme != "поля" {
			t.Errorf("IDENT.Lexeme = %q, хотим %q", toks[0].Lexeme, "поля")
		}
	})
	t.Run("тип более НЕ даёт L-11 «зарезервированное слово»", func(t *testing.T) {
		_, errs := lexAll("тип")
		requireNoErrors(t, errs)
	})
	t.Run("аннотации типов полей и значения тип: — IDENT", func(t *testing.T) {
		for _, w := range []string{
			"Целое", "Дробное", "Строка", "Логическое", "Дата",
			"json", "csv", "ndjson",
		} {
			toks, errs := lexAll(w)
			requireNoErrors(t, errs)
			requireTypes(t, toks, IDENT, NEWLINE, EOF)
			if toks[0].Lexeme != w {
				t.Errorf("%q: Lexeme = %q, хотим IDENT %q", w, toks[0].Lexeme, w)
			}
		}
	})
}
