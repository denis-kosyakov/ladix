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
	}
	if len(table) != 34 {
		t.Fatalf("в таблице теста %d ключевых слов, хотим 34", len(table))
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
