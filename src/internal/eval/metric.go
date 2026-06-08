package eval

import (
	"fmt"
	"strings"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// evalMetricByName запускает пересчёт метрики по имени (D-8/R6): из evalIdent
// (метрика-как-значение) и из CLI (фаза F). Защита от цикла зависимостей метрик
// через i.metricStack: повторный вход в уже считающуюся метрику → ОшибкаВыполнения
// «цикл зависимостей метрик: <стек+name через " → ">» (§SM-9.B). pos — позиция
// подвыражения (Ident или объявления), куда привязывается ошибка цикла.
func (i *Interpreter) evalMetricByName(name string, pos ast.Position) (value.Value, error) {
	m, ok := i.metrics[name]
	if !ok {
		// Защитно: вызывающий (evalIdent/CLI) обязан проверить наличие заранее.
		return nil, runtimeErr(pos, fmt.Sprintf("неизвестная метрика '%s'", name))
	}
	for _, active := range i.metricStack {
		if active == name {
			chain := append(append([]string{}, i.metricStack...), name)
			return nil, runtimeErr(pos,
				fmt.Sprintf("цикл зависимостей метрик: %s", strings.Join(chain, " → ")))
		}
	}
	i.metricStack = append(i.metricStack, name)
	defer func() { i.metricStack = i.metricStack[:len(i.metricStack)-1] }()
	return i.evalMetric(m)
}
