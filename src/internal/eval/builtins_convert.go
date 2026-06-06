package eval

import (
	"strconv"
	"strings"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// строка(x) — строковое представление §7 (всегда успешно).
func builtinStroka(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	return value.Строка{V: value.String(args[0])}, nil
}

// тип(x) — каноническое имя типа.
func builtinTip(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	return value.Строка{V: args[0].TypeName()}, nil
}

// булево(x) — ЕДИНСТВЕННАЯ точка мягкого truthy (B-5). 0/0.0/""/[]/пусто/ложь →
// ложь; прочее → истина.
func builtinBulevo(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	return value.Булево{V: truthy(args[0])}, nil
}

func truthy(v value.Value) bool {
	switch x := v.(type) {
	case value.Булево:
		return x.V
	case value.Целое:
		return x.V != 0
	case value.Дробное:
		return x.V != 0
	case value.Строка:
		return x.V != ""
	case value.Пусто:
		return false
	case value.Список:
		return len(*x.Elems) != 0
	}
	return true
}

// целое(x) — Дробное→отсечение, Строка→парсинг целого, Булево→1/0, Целое→как есть.
func builtinTseloe(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	switch x := args[0].(type) {
	case value.Целое:
		return x, nil
	case value.Дробное:
		return value.Целое{V: int64(x.V)}, nil
	case value.Булево:
		if x.V {
			return value.Целое{V: 1}, nil
		}
		return value.Целое{V: 0}, nil
	case value.Строка:
		n, err := strconv.ParseInt(strings.TrimSpace(x.V), 10, 64)
		if err != nil {
			return nil, typeErr(pos, "целое: строка «"+x.V+"» не является целым числом")
		}
		return value.Целое{V: n}, nil
	}
	return nil, typeErr(pos, "целое: значение типа "+args[0].TypeName()+" не преобразуется в Целое")
}

// дробное(x) — Целое→n.0, Строка→парсинг, Булево→1.0/0.0, Дробное→как есть.
func builtinDrobnoe(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	switch x := args[0].(type) {
	case value.Дробное:
		return x, nil
	case value.Целое:
		return value.Дробное{V: float64(x.V)}, nil
	case value.Булево:
		if x.V {
			return value.Дробное{V: 1}, nil
		}
		return value.Дробное{V: 0}, nil
	case value.Строка:
		f, err := strconv.ParseFloat(strings.TrimSpace(x.V), 64)
		if err != nil {
			return nil, typeErr(pos, "дробное: строка «"+x.V+"» не является числом")
		}
		return value.Дробное{V: f}, nil
	}
	return nil, typeErr(pos, "дробное: значение типа "+args[0].TypeName()+" не преобразуется в Дробное")
}

// число(x) — Строка→автовыбор Целое/Дробное; Целое/Дробное→как есть.
func builtinChislo(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	switch x := args[0].(type) {
	case value.Целое:
		return x, nil
	case value.Дробное:
		return x, nil
	case value.Строка:
		s := strings.TrimSpace(x.V)
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return value.Целое{V: n}, nil
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return value.Дробное{V: f}, nil
		}
		return nil, typeErr(pos, "число: строка «"+x.V+"» не является числом")
	}
	return nil, typeErr(pos, "число: значение типа "+args[0].TypeName()+" не преобразуется в число")
}
