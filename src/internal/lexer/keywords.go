package lexer

// keywords — 34 ключевых слова Ladix → их виды токенов (SPEC §2, data-model §5.3).
// Сверка идёт по ПОЛНОМУ совпадению жадно прочитанного идентификатора (FR-006/
// FR-012): `тип_клиента`, `или_нет`, `вход` — это IDENT, а не ключевые слова.
//
// Внимание: `истина`/`ложь`/`пусто` сюда НЕ входят — они эмитятся как BOOL/NONE
// (FR-006) и обрабатываются в scanIdent отдельно.
var keywords = map[string]TokenType{
	// Объявления (3)
	"пусть": KW_LET, "функция": KW_FUNC, "вернуть": KW_RETURN,
	// Управление (6)
	"если": KW_IF, "иначе": KW_ELSE, "пока": KW_WHILE, "для": KW_FOR,
	"прервать": KW_BREAK, "продолжить": KW_CONTINUE,
	// Логика (3)
	"и": KW_AND, "или": KW_OR, "не": KW_NOT,
	// Декларатив (15)
	"источник": KW_SOURCE, "файл": KW_FILE, "метрика": KW_METRIC, "где": KW_WHERE,
	"агрегат": KW_AGGREGATE, "период": KW_PERIOD, "по_дате": KW_BY_DATE,
	"процесс": KW_PROCESS, "шаг": KW_STEP, "исполнитель": KW_ASSIGNEE, "срок": KW_DEADLINE,
	"после": KW_AFTER, "присвоить": KW_SET, "вызвать": KW_CALL, "уведомить": KW_NOTIFY,
	// Триггеры (7)
	"когда": KW_WHEN, "событие": KW_EVENT, "значение": KW_VALUE, "расписание": KW_SCHEDULE,
	"каждые": KW_EVERY, "в": KW_IN, "запустить": KW_RUN,
}

// reservedWords — 12 зарезервированных слов (FR-013): токена НЕ дают, дают
// LexError L-11. Защита совместимости для будущих версий Ladix.
var reservedWords = map[string]bool{
	"параллельно": true, "модуль": true, "импорт": true, "экспорт": true,
	"тип": true, "повторить": true, "выбрать": true, "пытаться": true,
	"словить": true, "бросить": true, "асинхронно": true, "ждать": true,
}

// durationUnits — 6 единиц длительности (FR-017). Суффикс читается за ЦЕЛЫМ
// числом и сверяется по полному совпадению run'а.
var durationUnits = map[string]bool{
	"сек": true, "мин": true, "час": true, "дн": true, "нед": true, "мес": true,
}
