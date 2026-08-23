package value

// Скалярные (значимые) типы Ladix: копируются по значению, алиасинга нет.
// Go-представление — по docs/eval-model.md §2.1 / contracts/values.md C-1.

// Целое — целое число (int64). Переполнение в + - * → ОшибкаВыполнения (eval).
type Целое struct{ V int64 }

func (Целое) TypeName() string { return "Целое" }

// Дробное — число с плавающей точкой (IEEE 754 float64).
type Дробное struct{ V float64 }

func (Дробное) TypeName() string { return "Дробное" }

// Строка — неизменяемая строка UTF-8; индексация [i] и длина — в рунах (eval).
type Строка struct{ V string }

func (Строка) TypeName() string { return "Строка" }

// Булево — логическое значение; условия strict-Булево (truthiness не действует).
type Булево struct{ V bool }

func (Булево) TypeName() string { return "Булево" }

// Пусто — единичный тип «пустоты». Используется как синглтон None.
type Пусто struct{}

func (Пусто) TypeName() string { return "Пусто" }

// None — единственный экземпляр Пусто (синглтон, §2.1). Функция без вернуть и
// голый вернуть дают именно его; отсутствующее поле Записи → None.
var None = Пусто{}
