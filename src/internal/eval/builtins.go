package eval

import (
	"fmt"
	"strings"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// ArityKind — класс арности встроенной (B-2): фиксированная проверяется
// семпроходом, вариативная/перегруженная — рантаймом.
type ArityKind int

const (
	ArityFixed      ArityKind = iota // ровно N аргументов (проверка на семпроходе)
	ArityVariadic                    // печать (любое число)
	ArityOverloaded                  // диапазон (1|2), длина (по типу) — проверка в рантайме
)

// BuiltinFn — реализация встроенной: получает интерпретатор (для out/итерации),
// вычисленные аргументы и позицию вызова (для ошибок).
type BuiltinFn func(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error)

// Builtin — запись встроенной функции (§6).
type Builtin struct {
	Name     string
	Arity    ArityKind
	N        int  // для ArityFixed: точное число аргументов
	Deferred bool // инертный backstop при пустом deferredNames; механизм НЕ удалён (008/§DB-6)
	Fn       BuiltinFn
}

func fixed(name string, n int, fn BuiltinFn) Builtin {
	return Builtin{Name: name, Arity: ArityFixed, N: n, Fn: fn}
}
func variadic(name string, fn BuiltinFn) Builtin {
	return Builtin{Name: name, Arity: ArityVariadic, Fn: fn}
}
func overloaded(name string, fn BuiltinFn) Builtin {
	return Builtin{Name: name, Arity: ArityOverloaded, Fn: fn}
}
func deferredBuiltin(name string) Builtin {
	return Builtin{Name: name, Deferred: true}
}

// deferredNames — список отложенных встроенных. ПУСТ с 008: 7 дата/время-функций
// активированы (§DB-6/D-D), 004 ранее активировал дата/сегодня (§SM-6), 006 — 3
// процессных (§EN-0/D-15). Поле Builtin.Deferred и backstop-ветки (analyze.go/
// call.go) НЕ удалены — при пустом списке недостижимы (инертны).
var deferredNames = []string{}

// registerBuiltins строит закрытый реестр: РОВНО 35 активных, 0 deferred = 35
// (D6/§DB-6, расширять/сокращать ЗАПРЕЩЕНО; длина в счёте ×1).
//
// Достижимость (честная сноска, 009/§docs-alignment A4): из 35 зарегистрированных
// синтаксически достижимы 34. Имя "тип" — зарезервированное слово лексера
// (internal/lexer/keywords.go: "тип": true), поэтому вызов тип(...) из исходника
// недостижим: печать(тип(5)) даёт лексическую ошибку «'тип' — зарезервированное
// слово…», а не "Целое". Builtin тип зарегистрирован и реализован (целевая
// семантика — имя типа как Строка), но активируется лишь в v2 снятием имени из
// reserved-набора лексера — аддитивно. Регистрацию/реализацию НЕ удалять.
func registerBuiltins() map[string]Builtin {
	m := map[string]Builtin{}
	add := func(b Builtin) { m[b.Name] = b }

	// вывод (1)
	add(variadic("печать", builtinPechat))
	// преобразование (6)
	add(fixed("строка", 1, builtinStroka))
	add(fixed("целое", 1, builtinTseloe))
	add(fixed("дробное", 1, builtinDrobnoe))
	add(fixed("число", 1, builtinChislo))
	add(fixed("булево", 1, builtinBulevo))
	add(fixed("тип", 1, builtinTip))
	// агрегаты (5)
	add(fixed("сумма", 1, builtinSumma))
	add(fixed("количество", 1, builtinKolichestvo))
	add(fixed("среднее", 1, builtinSrednee))
	add(fixed("мин", 1, builtinMin))
	add(fixed("макс", 1, builtinMaks))
	// списки (10)
	add(overloaded("длина", builtinDlina))
	add(fixed("добавить", 2, builtinDobavit))
	add(fixed("соединить", 2, builtinSoedinit))
	add(fixed("срез", 3, builtinSrez))
	add(fixed("содержит", 2, builtinSoderzhit))
	add(fixed("найти", 2, builtinNayti))
	add(fixed("копия", 1, builtinKopiya))
	add(fixed("обратить", 1, builtinObratit))
	add(fixed("сортировать", 1, builtinSortirovat))
	add(overloaded("диапазон", builtinDiapazon))
	// строки (1)
	add(fixed("подстрока", 3, builtinPodstroka))
	// дата/время (2, активированы 004 — §SM-6)
	add(fixed("сегодня", 0, builtinSegodnya))
	add(overloaded("дата", builtinData))
	// дата/время (7, активированы 008 — §DB-6/D-D)
	add(fixed("вчера", 0, builtinVchera))
	add(fixed("завтра", 0, builtinZavtra))
	add(fixed("длительность", 2, builtinDlitelnost))
	add(fixed("в_секундах", 1, конвертер("в_секундах", 1)))
	add(fixed("в_минутах", 1, конвертер("в_минутах", 60)))
	add(fixed("в_часах", 1, конвертер("в_часах", 3600)))
	add(fixed("в_днях", 1, конвертер("в_днях", 86400)))
	// процессные (3, активированы 006 — §EN-5/D-15; через i.runtime)
	add(fixed("статус_процесса", 1, builtinStatusProtsessa))
	add(fixed("состояние_процесса", 1, builtinSostoyanieProtsessa))
	add(fixed("задачи_пользователя", 1, builtinZadachiPolzovatelya))

	// deferred-заглушки: deferredNames пуст (008/§DB-6) → цикл регистрации заглушек
	// УДАЛЁН (D-D). Машинерия deferral сохранена дормантной: deferredBuiltin (:43),
	// поле Builtin.Deferred (:30) и backstop-ветки (analyze.go/call.go) НЕ удалены —
	// механизм цел на случай будущего повторного откладывания встроенной (§DB-6).
	return m
}

// builtinPechat — печать (вариативная): строковые представления §7 через один
// пробел + перевод строки; печать() → пустая строка; возвращает Пусто. Единственный
// штатный канал вывода (пишет в инжектированный out).
func builtinPechat(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	parts := make([]string, len(args))
	for k, a := range args {
		parts[k] = value.String(a)
	}
	fmt.Fprintln(i.out, strings.Join(parts, " "))
	return value.None, nil
}
