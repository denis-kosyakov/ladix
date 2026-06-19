package eval

import (
	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// requireList извлекает Список или даёт ОшибкаТипа с именем встроенной.
func requireList(name string, v value.Value, pos ast.Position) (value.Список, error) {
	if l, ok := v.(value.Список); ok {
		return l, nil
	}
	return value.Список{}, typeErr(pos, name+": ожидался Список, получено "+v.TypeName())
}

// сумма(s) — сумма чисел; пустой → 0 (Целое); есть Дробное → результат Дробное;
// нечисловой элемент → ОшибкаТипа; переполнение целой суммы → ОшибкаВыполнения.
func builtinSumma(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	lst, err := requireList("сумма", args[0], pos)
	if err != nil {
		return nil, err
	}
	// Сначала определяем режим: смешанное с Дробным → float-путь (без int-гарда,
	// переполнение int64 неприменимо); все Целые → int-путь с overflow-гардом.
	hasFloat := false
	for _, e := range *lst.Elems {
		switch e.(type) {
		case value.Целое, value.Дробное:
			if _, ok := e.(value.Дробное); ok {
				hasFloat = true
			}
		default:
			return nil, typeErr(pos, "сумма: элемент типа "+e.TypeName()+" не является числом")
		}
	}
	if hasFloat {
		var fSum float64
		for _, e := range *lst.Elems {
			switch x := e.(type) {
			case value.Целое:
				fSum += float64(x.V)
			case value.Дробное:
				fSum += x.V
			}
		}
		return value.Дробное{V: fSum}, nil
	}
	var iSum int64
	for _, e := range *lst.Elems {
		x := e.(value.Целое)
		s, ov := addInt64(iSum, x.V)
		if ov {
			return nil, runtimeErr(pos, "переполнение целого числа")
		}
		iSum = s
	}
	return value.Целое{V: iSum}, nil
}

// количество(s) — число элементов списка.
func builtinKolichestvo(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	lst, err := requireList("количество", args[0], pos)
	if err != nil {
		return nil, err
	}
	return value.Целое{V: int64(len(*lst.Elems))}, nil
}

// среднее(s) — среднее арифметическое (Дробное); пустой → «среднее: список пуст»;
// нечисловой → ОшибкаТипа.
func builtinSrednee(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	lst, err := requireList("среднее", args[0], pos)
	if err != nil {
		return nil, err
	}
	elems := *lst.Elems
	if len(elems) == 0 {
		return nil, runtimeErr(pos, "среднее: список пуст")
	}
	var sum float64
	for _, e := range elems {
		f, ok := toFloat(e)
		if !ok {
			return nil, typeErr(pos, "среднее: элемент типа "+e.TypeName()+" не является числом")
		}
		sum += f
	}
	return value.Дробное{V: sum / float64(len(elems))}, nil
}

// мин(s) — минимальный элемент; пустой → «мин: список пуст»; несравнимые → ОшибкаТипа.
func builtinMin(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	return aggMinMax("мин", false, args[0], pos)
}

// макс(s) — максимальный элемент; пустой → «макс: список пуст»; несравнимые → ОшибкаТипа.
func builtinMaks(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	return aggMinMax("макс", true, args[0], pos)
}

func aggMinMax(name string, wantMax bool, v value.Value, pos ast.Position) (value.Value, error) {
	lst, err := requireList(name, v, pos)
	if err != nil {
		return nil, err
	}
	elems := *lst.Elems
	if len(elems) == 0 {
		return nil, runtimeErr(pos, name+": список пуст")
	}
	best := elems[0]
	for _, e := range elems[1:] {
		c, ok := value.Compare(e, best)
		if !ok {
			return nil, typeErr(pos, name+": элементы несравнимы")
		}
		if (wantMax && c > 0) || (!wantMax && c < 0) {
			best = e
		}
	}
	return best, nil
}
