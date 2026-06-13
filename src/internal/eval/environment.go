package eval

import "github.com/denis-kosyakov/ladix/internal/value"

// Environment — одна область видимости переменных; цепочка через parent даёт
// лексический поиск (§2.2, D3). Новую область создают ТОЛЬКО глобал и кадр вызова
// функции — блоки если/иначе/пока/для своей области НЕ создают.
type Environment struct {
	vars     map[string]value.Value
	parent   *Environment // nil у глобальной области
	boundary bool         // граница записи (§TR-5): Assign не выходит за этот env (тело триггера)
}

// NewEnvironment создаёт область с данным parent.
func NewEnvironment(parent *Environment) *Environment {
	return &Environment{vars: make(map[string]value.Value), parent: parent}
}

// markBoundary помечает область границей ЗАПИСИ (§TR-5): Assign имени, которого нет
// среди ЛОКАЛЕЙ этой области, НЕ поднимается к parent (возвращает false). Чтение
// (Lookup) барьер НЕ трогает — оно по-прежнему поднимается вверх (глобалы/метрики
// видны). Ставится ТОЛЬКО на env тела триггера (trigger_run.go) — read-only глобалов
// из тела; все прочие env (слои 001-006) создаются без флага, их Assign-семантика
// прежняя. Барьер не распространяется на кадры вызова функций: они создаются с
// parent=global (call.go), а не от env тела, поэтому в их цепочку этот env не попадает
// (§TR-5 «Граница гарантии»).
func (e *Environment) markBoundary() {
	e.boundary = true
}

// Define создаёт привязку в ЭТОЙ области (пусть/параметр/переменная для). Повтор
// в той же области ловит семпроход (§9), не сам Define.
func (e *Environment) Define(name string, v value.Value) {
	e.vars[name] = v
}

// Assign ищет существующую привязку вверх по цепочке и обновляет её; возвращает
// false, если имя не объявлено (вызывающий порождает «'x' не объявлено»). Граница
// записи (§TR-5): обход прерывается на boundary-env — присваивание имени, которого нет
// среди его локалей, не мутирует предка (read-only глобалов из тела триггера). На
// неуспехе вызывающий различает резолв Lookup-ом: нашёл → TR-BODY-RO, нет → «не объявлено».
func (e *Environment) Assign(name string, v value.Value) bool {
	for env := e; env != nil; env = env.parent {
		if _, ok := env.vars[name]; ok {
			env.vars[name] = v
			return true
		}
		if env.boundary {
			return false
		}
	}
	return false
}

// Lookup читает значение вверх по цепочке.
func (e *Environment) Lookup(name string) (value.Value, bool) {
	for env := e; env != nil; env = env.parent {
		if v, ok := env.vars[name]; ok {
			return v, true
		}
	}
	return nil, false
}

// hasLocal сообщает, объявлено ли имя именно в ЭТОЙ области (для «Define-если-нет»
// переменной цикла для, §4.3).
func (e *Environment) hasLocal(name string) bool {
	_, ok := e.vars[name]
	return ok
}

// Locals — снапшот ЛОКАЛЬНОГО слоя области (копия карты; значения разделяются —
// ссылочность Список/Запись сохраняется). ТОЛЬКО для тестов/сверки Variables
// движком (§EN-4): в алгоритмах §EN-3 движок его НЕ зовёт — канал персиста
// processEnv только хук AssignProcessVar.
func (e *Environment) Locals() map[string]value.Value {
	m := make(map[string]value.Value, len(e.vars))
	for k, v := range e.vars {
		m[k] = v
	}
	return m
}
