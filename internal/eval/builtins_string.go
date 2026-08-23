package eval

import (
	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// подстрока(s, i, j) — подстрока из рун [i, j); требует 0 ≤ i ≤ j ≤ длина(s),
// иначе «подстрока: индексы вне диапазона». Индексы — в РУНАХ.
func builtinPodstroka(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	s, ok := args[0].(value.Строка)
	if !ok {
		return nil, typeErr(pos, "подстрока: первый аргумент должен быть Строка, получено "+args[0].TypeName())
	}
	lo, hi, err := twoIndices("подстрока", args[1], args[2], pos)
	if err != nil {
		return nil, err
	}
	runes := []rune(s.V)
	n := int64(len(runes))
	if lo < 0 || hi < lo || hi > n {
		return nil, runtimeErr(pos, "подстрока: индексы вне диапазона")
	}
	return value.Строка{V: string(runes[lo:hi])}, nil
}
