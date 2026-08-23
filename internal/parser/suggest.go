package parser

import "fmt"

// Дешёвые подсказки «возможно, вы имели в виду…» (DX2 A3, фича 012). Расстояние
// Левенштейна по рунам, детерминированно. Канон формата — docs/diagnostics-model.md
// §MDX-4. Скоуп вехи — парсер-side ограниченные словари (валидные атрибуты
// деклараций); подсказки по объявленным ИМЕНАМ отложены (eval заморожен, §MDX-4).

// suggestMaxDist — порог близости (малое расстояние правки). 2 ловит типовые
// опечатки (вставка/удаление/замена/транспозиция-как-2-правки) коротких имён
// атрибутов, не порождая ложных подсказок для явно других слов.
const suggestMaxDist = 2

// Словари валидных атрибутов — зеркало sourceAttrName / metricAttrName (parse_decl.go).
// Держатся здесь для подсказок closestWord; порядок задаёт детерминированный выбор
// при равенстве расстояний (раньше в срезе — выбор).
var (
	sourceAttrVocab = []string{"файл", "тип", "поля"}
	metricAttrVocab = []string{"источник", "где", "агрегат", "период", "по_дате"}
)

// levenshtein — классическое расстояние редактирования по рунам (две строки).
// Итеративная DP по одной строке-буферу; без аллокаций сверх двух срезов.
func levenshtein(a, b []rune) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// closestWord возвращает ближайшее по Левенштейну слово из vocab в пределах maxDist.
// Детерминированно: при равенстве расстояний выбирается слово, идущее РАНЬШЕ в vocab
// (строгое сравнение d < bestDist). ok=false, если близкого кандидата нет.
func closestWord(got string, vocab []string, maxDist int) (string, bool) {
	gr := []rune(got)
	best := ""
	bestDist := maxDist + 1
	for _, w := range vocab {
		if d := levenshtein(gr, []rune(w)); d < bestDist {
			best, bestDist = w, d
		}
	}
	if bestDist <= maxDist {
		return best, true
	}
	return "", false
}

// msgUnknownAttrHint — msgUnknownAttr с дешёвой подсказкой при близком валидном
// атрибуте (DX2 A3). Подсказка добавляется суффиксом к тексту диагностики;
// двухстрочный канон и позиция не нарушаются (одна строка сообщения).
func msgUnknownAttrHint(name string, vocab []string) string {
	msg := msgUnknownAttr(name)
	if cand, ok := closestWord(name, vocab, suggestMaxDist); ok {
		return msg + fmt.Sprintf("; возможно, вы имели в виду '%s'?", cand)
	}
	return msg
}
