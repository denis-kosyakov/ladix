package eval

import (
	"sort"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// длина(s) — перегружена по типу (Список|Строка), счёт ×1. Число аргументов
// проверяется в рантайме (не на семпроходе).
func builtinDlina(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	if len(args) != 1 {
		return nil, runtimeErr(pos, "'длина': неверное число аргументов")
	}
	switch x := args[0].(type) {
	case value.Список:
		return value.Целое{V: int64(len(*x.Elems))}, nil
	case value.Строка:
		return value.Целое{V: int64(len([]rune(x.V)))}, nil
	default:
		return nil, typeErr(pos, "длина: ожидался Список или Строка, получено "+x.TypeName())
	}
}

// добавить(s, x) — ЕДИНСТВЕННЫЙ мутатор: меняет *s.Elems на месте, возвращает
// Пусто. На помеченном «итерируется» списке → «список изменён во время итерации».
func builtinDobavit(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	lst, err := requireList("добавить", args[0], pos)
	if err != nil {
		return nil, err
	}
	if i.isIterating(lst.Elems) {
		return nil, runtimeErr(pos, "список изменён во время итерации")
	}
	*lst.Elems = append(*lst.Elems, args[1])
	return value.None, nil
}

// соединить(s1, s2) — НОВЫЙ список = элементы s1, затем s2.
func builtinSoedinit(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	a, err := requireList("соединить", args[0], pos)
	if err != nil {
		return nil, err
	}
	b, err := requireList("соединить", args[1], pos)
	if err != nil {
		return nil, err
	}
	out := make([]value.Value, 0, len(*a.Elems)+len(*b.Elems))
	out = append(out, *a.Elems...)
	out = append(out, *b.Elems...)
	return value.NewList(out), nil
}

// срез(s, i, j) — НОВЫЙ список из [i, j); требует 0 ≤ i ≤ j ≤ длина(s).
func builtinSrez(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	lst, err := requireList("срез", args[0], pos)
	if err != nil {
		return nil, err
	}
	lo, hi, err := twoIndices("срез", args[1], args[2], pos)
	if err != nil {
		return nil, err
	}
	n := int64(len(*lst.Elems))
	if lo < 0 || hi < lo || hi > n {
		return nil, runtimeErr(pos, "срез: индексы вне диапазона")
	}
	out := make([]value.Value, hi-lo)
	copy(out, (*lst.Elems)[lo:hi])
	return value.NewList(out), nil
}

// twoIndices извлекает два Целое-индекса для срез/подстрока.
func twoIndices(name string, a, b value.Value, pos ast.Position) (int64, int64, error) {
	ai, ok := a.(value.Целое)
	if !ok {
		return 0, 0, typeErr(pos, name+": индекс должен быть Целое, получено "+a.TypeName())
	}
	bi, ok := b.(value.Целое)
	if !ok {
		return 0, 0, typeErr(pos, name+": индекс должен быть Целое, получено "+b.TypeName())
	}
	return ai.V, bi.V, nil
}

// содержит(s, x) — истина, если какой-то элемент == x.
func builtinSoderzhit(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	lst, err := requireList("содержит", args[0], pos)
	if err != nil {
		return nil, err
	}
	for _, e := range *lst.Elems {
		if value.Equal(e, args[1]) {
			return value.Булево{V: true}, nil
		}
	}
	return value.Булево{V: false}, nil
}

// найти(s, x) — индекс первого элемента == x или −1.
func builtinNayti(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	lst, err := requireList("найти", args[0], pos)
	if err != nil {
		return nil, err
	}
	for k, e := range *lst.Elems {
		if value.Equal(e, args[1]) {
			return value.Целое{V: int64(k)}, nil
		}
	}
	return value.Целое{V: -1}, nil
}

// копия(s) — поверхностная копия (новый список, те же ссылки на вложенные).
func builtinKopiya(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	lst, err := requireList("копия", args[0], pos)
	if err != nil {
		return nil, err
	}
	out := make([]value.Value, len(*lst.Elems))
	copy(out, *lst.Elems)
	return value.NewList(out), nil
}

// обратить(s) — НОВЫЙ список в обратном порядке.
func builtinObratit(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	lst, err := requireList("обратить", args[0], pos)
	if err != nil {
		return nil, err
	}
	src := *lst.Elems
	out := make([]value.Value, len(src))
	for k := range src {
		out[len(src)-1-k] = src[k]
	}
	return value.NewList(out), nil
}

// сортировать(s) — НОВЫЙ список по возрастанию, устойчиво; несравнимые → ОшибкаТипа.
func builtinSortirovat(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	lst, err := requireList("сортировать", args[0], pos)
	if err != nil {
		return nil, err
	}
	src := *lst.Elems
	out := make([]value.Value, len(src))
	copy(out, src)
	var cmpErr error
	sort.SliceStable(out, func(a, b int) bool {
		c, ok := value.Compare(out[a], out[b])
		if !ok {
			if cmpErr == nil {
				cmpErr = typeErr(pos, "сортировать: элементы несравнимы")
			}
			return false
		}
		return c < 0
	})
	if cmpErr != nil {
		return nil, cmpErr
	}
	return value.NewList(out), nil
}

// диапазон — перегружена по арности (1|2). диапазон(stop) → [0..stop); диапазон(
// start, stop) → [start..stop). Неверное число аргументов → RT-VARIADIC-ARITY.
func builtinDiapazon(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	switch len(args) {
	case 1:
		stop, ok := args[0].(value.Целое)
		if !ok {
			return nil, typeErr(pos, "диапазон: аргумент должен быть Целое, получено "+args[0].TypeName())
		}
		return makeRange(0, stop.V), nil
	case 2:
		start, ok := args[0].(value.Целое)
		if !ok {
			return nil, typeErr(pos, "диапазон: аргумент должен быть Целое, получено "+args[0].TypeName())
		}
		stop, ok := args[1].(value.Целое)
		if !ok {
			return nil, typeErr(pos, "диапазон: аргумент должен быть Целое, получено "+args[1].TypeName())
		}
		return makeRange(start.V, stop.V), nil
	default:
		return nil, runtimeErr(pos, "'диапазон': неверное число аргументов")
	}
}

func makeRange(start, stop int64) value.Список {
	elems := []value.Value{}
	for k := start; k < stop; k++ {
		elems = append(elems, value.Целое{V: k})
	}
	return value.NewList(elems)
}
