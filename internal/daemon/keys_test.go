package daemon

import (
	"testing"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
)

// trigKey возвращает контентный durable-ключ триггера в слоте idx у собранного демона
// (после ре-кея 027 ключи контентные, не позиционные «trg-N»). Существующие тесты
// демона, загружающие trigger_state по индексу, резолвят ключ через этот хелпер.
func trigKey(t *testing.T, d *Daemon, idx int) string {
	t.Helper()
	keys := buildTriggerKeys(d.interp.Triggers())
	if idx < 0 || idx >= len(keys) {
		t.Fatalf("trigKey: индекс %d вне диапазона (%d триггеров)", idx, len(keys))
	}
	if keys[idx] == "" {
		t.Fatalf("trigKey: триггер в слоте %d не имеет durable-ключа (событие/дедлайн?)", idx)
	}
	return keys[idx]
}

// parseTriggers компилирует исходник через НАСТОЯЩИЙ парсер и возвращает срез
// объявлений триггеров в порядке объявления (interp.Triggers()). Идёт через реальный
// фронтенд намеренно: это доказывает текстонезависимость канона (разный формат числа/
// пробелов на входе → одинаковый ключ на выходе). Пакет daemon (а не ast) — ast не
// может импортировать parser (листовость).
func parseTriggers(t *testing.T, src string) []*ast.TriggerDecl {
	t.Helper()
	tokens, errList := lexer.New(src).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		t.Fatalf("неожиданные лекс/синт ошибки: %s", errList.Error())
	}
	interp := eval.NewInterpreter(nil, 0, eval.SystemClock{})
	if err := interp.Analyze(prog); err != nil {
		t.Fatalf("неожиданная семантическая ошибка: %s", err.Error())
	}
	return interp.Triggers()
}

// metricTriggerIdx находит индекс слота ПЕРВОГО метрика-триггера в срезе (для сравнения
// keys[idx] между программами с разной раскладкой триггеров).
func metricTriggerIdx(t *testing.T, trig []*ast.TriggerDecl) int {
	t.Helper()
	for i, td := range trig {
		if _, ok := td.Spec.(*ast.MetricTrigger); ok {
			return i
		}
	}
	t.Fatalf("в программе нет метрика-триггера")
	return -1
}

// metricProg собирает минимальную программу с источником, метрикой и метрика-триггером
// «когда метрика <metric> <cond>». cond — например `< 10_000_000` (произвольный формат).
func metricProg(metric, cond string) string {
	return "источник s:\n    файл: \"d.json\"\n" +
		"метрика " + metric + ":\n    источник: s\n    агрегат: сумма(x)\n" +
		"когда метрика " + metric + " " + cond + ":\n    печать(\"X\")\n"
}

// TestBuildKeysCanonicalEquality — каноническое равенство (§FR-002): три записи ОДНОГО
// смысла с разным форматом числа/пробелов (`< 10_000_000`, `<10000000`, `<  10000000`)
// дают ОДИН и тот же контентный ключ. Идёт через настоящий парсер: текстонезависимость.
func TestBuildKeysCanonicalEquality(t *testing.T) {
	forms := []string{"< 10_000_000", "<10000000", "<  10000000"}
	var keys []string
	for _, f := range forms {
		k := buildTriggerKeys(parseTriggers(t, metricProg("выручка", f)))
		idx := metricTriggerIdx(t, parseTriggers(t, metricProg("выручка", f)))
		keys = append(keys, k[idx])
	}
	for i := 1; i < len(keys); i++ {
		if keys[i] != keys[0] {
			t.Fatalf("форма %q дала ключ %q, а форма %q — %q: канон обязан совпасть (FR-002)",
				forms[i], keys[i], forms[0], keys[0])
		}
	}
	// Санити: ключ непустой и контентного вида («trg-» + 16 hex).
	if len(keys[0]) != len("trg-")+16 || keys[0][:4] != "trg-" {
		t.Fatalf("ключ метрика-триггера не контентного вида: %q", keys[0])
	}
}

// TestBuildKeysCanonicalDifference — каноническое различие (§FR-002): другое имя метрики,
// другой оператор или другой порог → РАЗНЫЕ ключи. Базовая запись против трёх вариаций.
func TestBuildKeysCanonicalDifference(t *testing.T) {
	base := buildTriggerKeys(parseTriggers(t, metricProg("выручка", "< 10000000")))[0]

	t.Run("другое имя метрики", func(t *testing.T) {
		other := buildTriggerKeys(parseTriggers(t, metricProg("прибыль", "< 10000000")))[0]
		if other == base {
			t.Fatalf("разные метрики дали одинаковый ключ %q", base)
		}
	})
	t.Run("другой оператор", func(t *testing.T) {
		other := buildTriggerKeys(parseTriggers(t, metricProg("выручка", "> 10000000")))[0]
		if other == base {
			t.Fatalf("разные операторы дали одинаковый ключ %q", base)
		}
	})
	t.Run("другой порог", func(t *testing.T) {
		other := buildTriggerKeys(parseTriggers(t, metricProg("выручка", "< 9000000")))[0]
		if other == base {
			t.Fatalf("разные пороги дали одинаковый ключ %q", base)
		}
	})
}

// TestBuildKeysDuplicatesDisambiguated — дубликаты (§FR-004): два триггера с ИДЕНТИЧНЫМ
// условием в одной программе получают РАЗНЫЕ ключи (порядковый номер 0 vs 1 внутри
// группы канона). Иначе durable-состояние двух триггеров склеилось бы.
func TestBuildKeysDuplicatesDisambiguated(t *testing.T) {
	src := "источник s:\n    файл: \"d.json\"\n" +
		"метрика m:\n    источник: s\n    агрегат: сумма(x)\n" +
		"когда метрика m > 10:\n    печать(\"A\")\n" +
		"когда метрика m > 10:\n    печать(\"B\")\n"
	keys := buildTriggerKeys(parseTriggers(t, src))
	if len(keys) != 2 {
		t.Fatalf("ожидали 2 ключа, получили %d", len(keys))
	}
	if keys[0] == "" || keys[1] == "" {
		t.Fatalf("оба ключа должны быть непустыми: %q / %q", keys[0], keys[1])
	}
	if keys[0] == keys[1] {
		t.Fatalf("дубликаты-условия получили одинаковый ключ %q: ordinal не дизамбигуировал (FR-004)", keys[0])
	}
}

// TestBuildKeysStableAcrossReordering — 🔁 ЯДРОВОЙ ЗАМОК (§FR-001/004/005, SC-001):
// ключ метрика-триггера НЕ зависит от позиции объявления. Программа A — только
// метрика-триггер; программа B — НЕСВЯЗАННЫЙ событие-триггер, вставленный ПЕРЕД той же
// метрикой. Ключ метрики обязан быть ИДЕНТИЧНЫМ в обеих (контентный, не позиционный).
//
// 🔁 ИНВЕРСИОННЫЙ ЗАМОК: вернуть позиционный triggerID(idx) (keys[idx]="trg-<idx>") →
// индекс метрики сдвинется 0→1 (из-за вставленного событие-триггера) → ключи разойдутся
// → тест краснеет. Это и есть пойманный класс багов «ре-нумерация при правке списка».
func TestBuildKeysStableAcrossReordering(t *testing.T) {
	progA := "источник s:\n    файл: \"d.json\"\n" +
		"метрика выручка:\n    источник: s\n    агрегат: сумма(x)\n" +
		"когда метрика выручка < 10000000:\n    печать(\"X\")\n"
	progB := "источник s:\n    файл: \"d.json\"\n" +
		"метрика выручка:\n    источник: s\n    агрегат: сумма(x)\n" +
		"когда событие звонок:\n    печать(\"E\")\n" +
		"когда метрика выручка < 10000000:\n    печать(\"X\")\n"

	trigA := parseTriggers(t, progA)
	trigB := parseTriggers(t, progB)
	keysA := buildTriggerKeys(trigA)
	keysB := buildTriggerKeys(trigB)

	idxA := metricTriggerIdx(t, trigA)
	idxB := metricTriggerIdx(t, trigB)

	// Санити: вставка реально сдвинула позицию метрики (иначе тест не различал бы
	// позиционный и контентный ключи).
	if idxA == idxB {
		t.Fatalf("предусловие: метрика обязана сдвинуться по позиции (idxA=%d idxB=%d)", idxA, idxB)
	}
	if keysA[idxA] != keysB[idxB] {
		t.Fatalf("ключ метрики разошёлся при вставке несвязанного триггера: %q (поз %d) != %q (поз %d) — ключ позиционный, а должен быть контентным (FR-005)",
			keysA[idxA], idxA, keysB[idxB], idxB)
	}
}

// TestBuildKeysBaselineResetOnEdit — правка условия → чистая базовая линия (§FR-008/010
// ре-прайминг): редактирование порога даёт НОВЫЙ ключ, поэтому durable-загрузка для
// нового условия стартует с чистого листа (нет унаследованного LastBool → нет ложной
// кромки). Замок: ключ старого условия ≠ ключ нового условия.
func TestBuildKeysBaselineResetOnEdit(t *testing.T) {
	oldKey := buildTriggerKeys(parseTriggers(t, metricProg("выручка", "< 3000000")))[0]
	newKey := buildTriggerKeys(parseTriggers(t, metricProg("выручка", "< 5000000")))[0]
	if oldKey == newKey {
		t.Fatalf("правка порога не сменила ключ (%q): старое состояние утекло бы в новое условие (FR-008/010)", oldKey)
	}
}
