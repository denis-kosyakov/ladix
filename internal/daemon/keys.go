package daemon

import (
	"fmt"
	"hash/fnv"
	"strconv"

	"github.com/denis-kosyakov/ladix/internal/ast"
)

// buildTriggerKeys минтит стабильные контентные durable-ключи триггеров (§FR-001/004/005,
// EM-17.2.1): один проход по триггерам в порядке объявления, группировка по каноническому
// условию (ast.CanonicalTriggerCondition), порядковый номер внутри группы для дизамбигуации
// дубликатов, FNV-1a-64 от «канон#ordinal». Событие/дедлайн (пустой канон) durable-ключа не
// имеют — их слот остаётся пустым и на тиках не читается. Считается ОДИН раз при инициализации
// демона; per-tick — только чтение keys[idx].
func buildTriggerKeys(trig []*ast.TriggerDecl) []string {
	keys := make([]string, len(trig))
	ordinals := map[string]int{}
	for idx, td := range trig {
		c := ast.CanonicalTriggerCondition(td.Spec)
		if c == "" {
			continue
		}
		ord := ordinals[c]
		ordinals[c]++
		h := fnv.New64a()
		h.Write([]byte(c + "#" + strconv.Itoa(ord)))
		keys[idx] = "trg-" + fmt.Sprintf("%016x", h.Sum64())
	}
	return keys
}
