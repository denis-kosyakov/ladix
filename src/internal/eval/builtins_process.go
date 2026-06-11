package eval

import (
	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// Процессные встроенные (006, §EN-5/D-15). Активированы в реестре; вычисляются
// через инжектированный ProcessRuntime (мост eval↔engine, D-1). Все — ArityFixed
// N=1, аргумент обязан Строка (иначе ОшибкаТипа §EN-8.A, позиция вызова, как у
// дата). Неизвестный id → ОшибкаВыполнения «процесс '<id>' не найден» (D-15).

// requireString извлекает Строку-аргумент процессной встроенной или порождает
// ОшибкаТипа «<имя>: ожидается Строка, получено <тип>» (§EN-8.A #2/#3/#4).
func requireString(name string, arg value.Value, pos ast.Position) (string, error) {
	s, ok := arg.(value.Строка)
	if !ok {
		return "", typeErr(pos, name+": ожидается Строка, получено "+arg.TypeName())
	}
	return s.V, nil
}

// builtinStatusProtsessa — статус_процесса(id) → Строка статуса инстанса (§EN-8.A #1/#2).
func builtinStatusProtsessa(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	id, err := requireString("статус_процесса", args[0], pos)
	if err != nil {
		return nil, err
	}
	if i.runtime == nil {
		return nil, runtimeErr(pos, "внутренняя ошибка: движок процессов не подключён")
	}
	status, ok, err := i.runtime.InstanceStatus(id)
	if err != nil {
		return nil, runtimeErr(pos, err.Error())
	}
	if !ok {
		return nil, runtimeErr(pos, "процесс '"+id+"' не найден")
	}
	return value.Строка{V: status}, nil
}

// builtinSostoyanieProtsessa — состояние_процесса(id) → Запись переменных инстанса
// (ключи по возрастанию, D-21; §EN-8.A #1/#3).
func builtinSostoyanieProtsessa(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	id, err := requireString("состояние_процесса", args[0], pos)
	if err != nil {
		return nil, err
	}
	if i.runtime == nil {
		return nil, runtimeErr(pos, "внутренняя ошибка: движок процессов не подключён")
	}
	vars, ok, err := i.runtime.InstanceVariables(id)
	if err != nil {
		return nil, runtimeErr(pos, err.Error())
	}
	if !ok {
		return nil, runtimeErr(pos, "процесс '"+id+"' не найден")
	}
	return vars, nil
}

// builtinZadachiPolzovatelya — задачи_пользователя(исполнитель) → Список Записей
// открытых задач (""=все, по возрастанию id, D-15; §EN-8.A #4).
func builtinZadachiPolzovatelya(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	assignee, err := requireString("задачи_пользователя", args[0], pos)
	if err != nil {
		return nil, err
	}
	if i.runtime == nil {
		return nil, runtimeErr(pos, "внутренняя ошибка: движок процессов не подключён")
	}
	tasks, err := i.runtime.UserTasks(assignee)
	if err != nil {
		return nil, runtimeErr(pos, err.Error())
	}
	elems := make([]value.Value, len(tasks))
	for k, t := range tasks {
		elems[k] = t
	}
	return value.NewList(elems), nil
}
