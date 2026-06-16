package parser

import (
	"strings"
	"testing"
)

// T021 [US2]: подсказки «возможно, вы имели в виду…» (DX2 A3). Левенштейн +
// closestWord детерминированы; интеграция в SE-UNKNOWN-ATTR по словарям источника/
// метрики. Подсказки по объявленным именам — отложены (eval заморожен, §MDX-4).

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"тип", "тип", 0},
		{"тип", "тп", 1},   // удаление
		{"файл", "фал", 1}, // удаление
		{"тип", "тпи", 2},  // транспозиция = 2 правки
		{"где", "тип", 3},  // полностью разные
		{"период", "пероид", 2},
	}
	for _, c := range cases {
		if got := levenshtein([]rune(c.a), []rune(c.b)); got != c.want {
			t.Errorf("levenshtein(%q,%q) = %d, хотим %d", c.a, c.b, got, c.want)
		}
	}
}

func TestClosestWord(t *testing.T) {
	// близкий кандидат (опечатка атрибута источника)
	if got, ok := closestWord("тп", sourceAttrVocab, suggestMaxDist); !ok || got != "тип" {
		t.Errorf("closestWord(тп) = %q,%v; хотим тип,true", got, ok)
	}
	// близкий кандидат атрибута метрики
	if got, ok := closestWord("пероид", metricAttrVocab, suggestMaxDist); !ok || got != "период" {
		t.Errorf("closestWord(пероид) = %q,%v; хотим период,true", got, ok)
	}
	// далёкое слово → нет кандидата
	if got, ok := closestWord("мусор", sourceAttrVocab, suggestMaxDist); ok {
		t.Errorf("closestWord(мусор) = %q,%v; хотим '',false", got, ok)
	}
	// детерминизм при равенстве: пустой ввод равноудалён — выбирается первый по vocab
	if got, ok := closestWord("", []string{"аб", "вг"}, 5); !ok || got != "аб" {
		t.Errorf("closestWord('') = %q,%v; хотим аб,true (стабильный выбор)", got, ok)
	}
}

// Интеграция: опечатка атрибута даёт диагностику с подсказкой; далёкий атрибут — без.
func TestUnknownAttrSuggestionIntegration(t *testing.T) {
	// «тп» близко к «тип» → подсказка
	_, el := parseProgramSrc(t, "источник з:\n    тп: csv\n")
	got := el.Error()
	if !strings.Contains(got, "неизвестный атрибут 'тп'") {
		t.Fatalf("нет базовой диагностики:\n%s", got)
	}
	if !strings.Contains(got, "возможно, вы имели в виду 'тип'?") {
		t.Errorf("ожидалась подсказка про 'тип':\n%s", got)
	}
	// «мусор» далеко от валидных → без подсказки
	_, el2 := parseProgramSrc(t, "источник з:\n    мусор: csv\n")
	if strings.Contains(el2.Error(), "возможно, вы имели в виду") {
		t.Errorf("ложная подсказка для далёкого атрибута:\n%s", el2.Error())
	}
	// метрика: «исток» близко к «источник»? расстояние 3 (>2) → без подсказки;
	// «по_дата» близко к «по_дате» (1) → подсказка
	_, el3 := parseProgramSrc(t, "метрика м:\n    по_дата: x\n")
	if !strings.Contains(el3.Error(), "возможно, вы имели в виду 'по_дате'?") {
		t.Errorf("ожидалась подсказка про 'по_дате':\n%s", el3.Error())
	}
}
