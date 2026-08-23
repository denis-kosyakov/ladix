package parser

import (
	"strings"
	"testing"
)

// TestSECatalogInventory — инвентарь-замок полноты синтаксического каталога (DX2,
// FR-014, SC-006): реестр SE-* покрывает РОВНО 15 distinct диагностик — 8 ядровых
// (CHAIN/NESTED-FN/EMPTY-BLOCK/ASSIGN-TARGET/INT-RANGE/EXPECTED/UNEXPECTED/CATCH-NO-TRY)
// + 7 декларативных/триггерных (SOURCE-NAME/UNKNOWN-ATTR/DUP-ATTR/DUP-FIELD/TRIGGER-KIND/
// EXPECT-COMPOP/SCHEDULE-SPEC). Категория «Процесса» исключена (зарезервирована,
// в v1 не порождается — docs/diagnostics-model.md §MDX-3). Образец — eval
// len(seen)!=28. Каждый кейс парсит представительный исходник и проверяет, что
// среди диагностик есть распознаваемый фрагмент кода → seen[code]; замок «кусается»
// при выпадении кода из реестра.
func TestSECatalogInventory(t *testing.T) {
	cases := []struct {
		code     string
		src      string
		fragment string
	}{
		{"SE-CHAIN", "1 < y < 10\n", "сравнения нельзя сцеплять"},
		{"SE-NESTED-FN", "функция f():\n    функция g():\n        вернуть 1\n", "вложенные функции не поддерживаются"},
		{"SE-EMPTY-BLOCK", "если истина:\nx = 1\n", "пустой блок не допускается"},
		{"SE-ASSIGN-TARGET", "5 = 1\n", "неверная цель присваивания"},
		{"SE-INT-RANGE", "пусть x = 999999999999999999999999999\n", "вне диапазона типа Целое"},
		{"SE-EXPECTED", "пусть x 5\n", "ожидалось '='"},
		{"SE-UNEXPECTED", "значение\n", "неожиданный элемент 'значение'"},
		{"SE-CATCH-NO-TRY", "словить:\n    печать(1)\n", "допустимо только после блока 'пытаться'"},
		{"SE-SOURCE-NAME", "метрика м:\n    источник: 5\n", "ожидается имя источника"},
		{"SE-UNKNOWN-ATTR", "источник з:\n    мусор: 1\n", "неизвестный атрибут 'мусор'"},
		{"SE-DUP-ATTR", "источник з:\n    тип: csv\n    тип: json\n", "атрибут 'тип' уже задан"},
		{"SE-DUP-FIELD", "источник з:\n    тип: csv\n    файл: \"x\"\n    поля:\n        а: Целое\n        а: Строка\n", "поле 'а' уже объявлено"},
		{"SE-TRIGGER-KIND", "когда мусор:\n    печать(1)\n", "метрика, событие, расписание или задача"},
		{"SE-EXPECT-COMPOP", "когда метрика m:\n    печать(1)\n", "оператор сравнения"},
		{"SE-SCHEDULE-SPEC", "когда расписание мусор:\n    печать(1)\n", "каждые или в"},
	}
	const wantCodes = 15
	seen := make(map[string]bool, wantCodes)
	for _, c := range cases {
		_, el := parseProgramSrc(t, c.src)
		if !strings.Contains(el.Error(), c.fragment) {
			t.Errorf("%s: исходник %q не дал фрагмент %q:\n%s", c.code, c.src, c.fragment, el.Error())
			continue
		}
		seen[c.code] = true
	}
	if len(seen) != wantCodes {
		t.Errorf("каталог синтаксиса покрывает кодов = %d, хотим РОВНО %d: %v", len(seen), wantCodes, seen)
	}
}

// TestScopeANoJargon — замок DX2 (SC-007): наружу в синтаксических диагностиках
// (scope A — парсер эмитит только SE-*) НЕ протекают внутренний жаргон «токен»/
// «литерал» и коды диагностик (L-/SE-). Прогоняет те же представительные исходники
// каталога. Кусается, если де-жаргонизация регрессирует.
func TestScopeANoJargon(t *testing.T) {
	srcs := []string{
		"значение\n", "пусть x 5\n", "5 = 1\n", "1 < y < 10\n",
		"пусть x = 999999999999999999999999999\n",
		"источник з:\n    мусор: 1\n", "когда мусор:\n    печать(1)\n",
		"если вернуть:\n    печать(1)\n",
	}
	for _, src := range srcs {
		_, el := parseProgramSrc(t, src)
		out := el.Error()
		for _, bad := range []string{"токен", "литерал", "SE-", "L-"} {
			if strings.Contains(out, bad) {
				t.Errorf("scope-A диагностика содержит жаргон/код %q для %q:\n%s", bad, src, out)
			}
		}
	}
}
