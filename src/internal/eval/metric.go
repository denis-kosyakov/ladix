package eval

import (
	"fmt"
	"strings"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// EvalMetricByName — публичная точка входа CLI «ladix metric» (§SM-11, CM-4):
// находит метрику по имени и вычисляет её. В отличие от внутреннего
// evalMetricByName, диагностики поиска — СемантическаяОшибка с текстами §SM-9.D
// (без позиции исходника — запрос приходит из CLI-аргумента):
//   - имя не зарегистрировано в реестре метрик → «неизвестная метрика '<имя>'»;
//   - имя занято переменной/функцией/источником (не метрика) → «'<имя>' — не метрика».
//
// Найдено → внутренний evalMetricByName (защита от цикла, §SM-9.B); ошибки
// загрузки/вычисления (§SM-9.B/C) пробрасываются как есть.
func (i *Interpreter) EvalMetricByName(name string) (value.Value, error) {
	if _, ok := i.metrics[name]; ok {
		return i.evalMetricByName(name, ast.Position{})
	}
	// Имя есть, но это не метрика (предопр. период/функция/встроенное/источник)
	// → §SM-9.D «'<имя>' — не метрика»; иначе — неизвестная метрика.
	if _, ok := i.global.Lookup(name); ok {
		return nil, semErr(ast.Position{}, fmt.Sprintf("'%s' — не метрика", name))
	}
	if i.isFunctionName(name) {
		return nil, semErr(ast.Position{}, fmt.Sprintf("'%s' — не метрика", name))
	}
	if _, ok := i.sources[name]; ok {
		return nil, semErr(ast.Position{}, fmt.Sprintf("'%s' — не метрика", name))
	}
	return nil, semErr(ast.Position{}, fmt.Sprintf("неизвестная метрика '%s'", name))
}

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
