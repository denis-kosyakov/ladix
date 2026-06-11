package eval

import "github.com/denis-kosyakov/ladix/internal/value"

// ProcessRuntime — мост исполнения процессов (D-1, §EN-4). Реализуется пакетом
// engine, инжектируется сеттером SetProcessRuntime. Объявлен ЗДЕСЬ (в eval), чтобы
// разорвать цикл импортов eval↔engine: eval НЕ импортирует ни store, ни engine.
// Все вызовы синхронны, в одной горутине.
type ProcessRuntime interface {
	// StartProcess запускает процесс по имени: создаёт инстанс, синхронно доводит
	// до первого ожидания/терминала, возвращает id ("p-NNNNNN"). Ошибка — уже
	// типизированная ошибка Ladix (всплывает как есть) либо сбой Store.
	StartProcess(name string, args []value.Value) (string, error)

	// AssignProcessVar — хук персиста «присвоить»: значение уже записано в
	// processEnv интерпретатором; движок обновляет Variables активного инстанса
	// (вершина стека active) и персистит (▼SaveInstance).
	AssignProcessVar(name string, v value.Value) error

	// CallExternal — стаб «вызвать» (D-13): одна строка в stdout; в v1 всегда nil.
	// Контракт на будущее: не-nil ошибка → шаг провален (D-14, недостижимо в v1).
	CallExternal(target string, args []value.Value) error

	// Notify — стаб «уведомить» (D-13): одна строка в stdout; всегда nil (best-effort).
	Notify(target string, args []value.Value) error

	// InstanceStatus — статус инстанса по id; ok=false → вызывающий builtin даёт
	// «процесс '<id>' не найден» (D-15). err — только сбой Store.
	InstanceStatus(id string) (status string, ok bool, err error)

	// InstanceVariables — переменные инстанса как Запись, ключи по возрастанию (D-21).
	InstanceVariables(id string) (vars value.Запись, ok bool, err error)

	// UserTasks — открытые задачи исполнителя (""=все), по возрастанию id (D-15);
	// поля Записи — таблица «Task → Запись» (EM-13/ARCH §7.7, включая «просрочена»).
	UserTasks(assignee string) ([]value.Запись, error)
}
