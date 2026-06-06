package eval

import (
	"fmt"
	"math"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// evalUnary — унарные операции (§3.2). Позиция ошибки = токен оператора.
func (i *Interpreter) evalUnary(env *Environment, u *ast.UnaryExpr) (value.Value, error) {
	operand, err := i.evalExpr(env, u.Operand)
	if err != nil {
		return nil, err
	}
	switch u.Op {
	case ast.OpNeg:
		switch v := operand.(type) {
		case value.Целое:
			if v.V == math.MinInt64 { // недостижимо как значение, но единообразно
				return nil, runtimeErr(u.Pos(), "переполнение целого числа")
			}
			return value.Целое{V: -v.V}, nil
		case value.Дробное:
			return value.Дробное{V: -v.V}, nil
		default:
			return nil, typeErr(u.Pos(), fmt.Sprintf("унарный '-' нельзя применить к %s", operand.TypeName()))
		}
	case ast.OpNot:
		if v, ok := operand.(value.Булево); ok {
			return value.Булево{V: !v.V}, nil
		}
		return nil, typeErr(u.Pos(), fmt.Sprintf("'не' нельзя применить к %s", operand.TypeName()))
	}
	return nil, runtimeErr(u.Pos(), "внутренняя ошибка: неизвестный унарный оператор")
}

// evalBinary — бинарные операции (§3.3). Сначала короткозамкнутая логика
// (правый операнд не всегда вычисляется), затем — арифметика/сравнения с
// вычислением обоих операндов слева направо.
func (i *Interpreter) evalBinary(env *Environment, b *ast.BinaryExpr) (value.Value, error) {
	if b.Op == ast.OpOr || b.Op == ast.OpAnd {
		return i.evalLogic(env, b)
	}
	left, err := i.evalExpr(env, b.Left)
	if err != nil {
		return nil, err
	}
	right, err := i.evalExpr(env, b.Right)
	if err != nil {
		return nil, err
	}
	switch b.Op {
	case ast.OpAdd:
		return i.evalAdd(b, left, right)
	case ast.OpSub, ast.OpMul:
		return i.evalSubMul(b, left, right)
	case ast.OpDiv:
		return i.evalDiv(b, left, right)
	case ast.OpFloorDiv:
		return i.evalFloorDiv(b, left, right)
	case ast.OpMod:
		return i.evalMod(b, left, right)
	case ast.OpEq:
		return value.Булево{V: value.Equal(left, right)}, nil
	case ast.OpNeq:
		return value.Булево{V: !value.Equal(left, right)}, nil
	case ast.OpLt, ast.OpLe, ast.OpGt, ast.OpGe:
		return i.evalOrder(b, left, right)
	}
	return nil, runtimeErr(b.Pos(), "внутренняя ошибка: неизвестный бинарный оператор")
}

// evalLogic — короткозамкнутые и/или (§3.3). Оба операнда строго Булево;
// результат — строго Булево (НЕ возврат операнда, как в Python).
func (i *Interpreter) evalLogic(env *Environment, b *ast.BinaryExpr) (value.Value, error) {
	left, err := i.evalExpr(env, b.Left)
	if err != nil {
		return nil, err
	}
	lb, ok := left.(value.Булево)
	if !ok {
		return nil, typeErr(b.Pos(), fmt.Sprintf("'%s' требует Булево, получено %s", b.Op.String(), left.TypeName()))
	}
	if b.Op == ast.OpOr && lb.V {
		return value.Булево{V: true}, nil // короткое замыкание: правый не вычисляется
	}
	if b.Op == ast.OpAnd && !lb.V {
		return value.Булево{V: false}, nil // короткое замыкание
	}
	right, err := i.evalExpr(env, b.Right)
	if err != nil {
		return nil, err
	}
	rb, ok := right.(value.Булево)
	if !ok {
		return nil, typeErr(b.Pos(), fmt.Sprintf("'%s' требует Булево, получено %s", b.Op.String(), right.TypeName()))
	}
	return value.Булево{V: rb.V}, nil
}

// evalAdd — оператор + (§3.3): Цел+Цел (с проверкой переполнения), промоушен при
// Дробном, Строка+Строка → конкатенация; списки + НЕ складываются.
func (i *Interpreter) evalAdd(b *ast.BinaryExpr, left, right value.Value) (value.Value, error) {
	if li, ok := left.(value.Целое); ok {
		if ri, ok := right.(value.Целое); ok {
			sum, ov := addInt64(li.V, ri.V)
			if ov {
				return nil, runtimeErr(b.Pos(), "переполнение целого числа")
			}
			return value.Целое{V: sum}, nil
		}
	}
	if lf, lok := toFloat(left); lok {
		if rf, rok := toFloat(right); rok {
			return value.Дробное{V: lf + rf}, nil
		}
	}
	if ls, ok := left.(value.Строка); ok {
		if rs, ok := right.(value.Строка); ok {
			return value.Строка{V: ls.V + rs.V}, nil
		}
	}
	return nil, typeErr(b.Pos(), fmt.Sprintf("'+' нельзя применить к %s и %s", left.TypeName(), right.TypeName()))
}

// evalSubMul — операторы - и * (§3.3): Цел○Цел (с переполнением) или промоушен.
func (i *Interpreter) evalSubMul(b *ast.BinaryExpr, left, right value.Value) (value.Value, error) {
	if li, ok := left.(value.Целое); ok {
		if ri, ok := right.(value.Целое); ok {
			var res int64
			var ov bool
			if b.Op == ast.OpSub {
				res, ov = subInt64(li.V, ri.V)
			} else {
				res, ov = mulInt64(li.V, ri.V)
			}
			if ov {
				return nil, runtimeErr(b.Pos(), "переполнение целого числа")
			}
			return value.Целое{V: res}, nil
		}
	}
	if lf, lok := toFloat(left); lok {
		if rf, rok := toFloat(right); rok {
			if b.Op == ast.OpSub {
				return value.Дробное{V: lf - rf}, nil
			}
			return value.Дробное{V: lf * rf}, nil
		}
	}
	return nil, typeErr(b.Pos(), binopMsg(b, left, right))
}

// evalDiv — оператор / (§3.3): ВСЕГДА Дробное; правый 0 → деление на ноль.
func (i *Interpreter) evalDiv(b *ast.BinaryExpr, left, right value.Value) (value.Value, error) {
	lf, lok := toFloat(left)
	rf, rok := toFloat(right)
	if !lok || !rok {
		return nil, typeErr(b.Pos(), binopMsg(b, left, right))
	}
	if rf == 0 {
		return nil, runtimeErr(b.Pos(), "деление на ноль")
	}
	return value.Дробное{V: lf / rf}, nil
}

// evalFloorDiv — оператор // (§3.3): только Цел//Цел → Целое; правый 0 → деление
// на ноль; иначе ОшибкаТипа.
func (i *Interpreter) evalFloorDiv(b *ast.BinaryExpr, left, right value.Value) (value.Value, error) {
	li, lok := left.(value.Целое)
	ri, rok := right.(value.Целое)
	if !lok || !rok {
		return nil, typeErr(b.Pos(), binopMsg(b, left, right))
	}
	if ri.V == 0 {
		return nil, runtimeErr(b.Pos(), "деление на ноль")
	}
	return value.Целое{V: floorDivInt64(li.V, ri.V)}, nil
}

// evalMod — оператор % (§3.3): Цел%Цел → Целое; Дроб%Дроб → Дробное (fmod);
// смешанное Цел/Дроб → ОшибкаТипа; правый 0 → деление на ноль.
func (i *Interpreter) evalMod(b *ast.BinaryExpr, left, right value.Value) (value.Value, error) {
	if li, ok := left.(value.Целое); ok {
		if ri, ok := right.(value.Целое); ok {
			if ri.V == 0 {
				return nil, runtimeErr(b.Pos(), "деление на ноль")
			}
			return value.Целое{V: modInt64(li.V, ri.V)}, nil
		}
	}
	if lf, ok := left.(value.Дробное); ok {
		if rf, ok := right.(value.Дробное); ok {
			if rf.V == 0 {
				return nil, runtimeErr(b.Pos(), "деление на ноль")
			}
			return value.Дробное{V: math.Mod(lf.V, rf.V)}, nil
		}
	}
	return nil, typeErr(b.Pos(), binopMsg(b, left, right))
}

// evalOrder — порядковые сравнения < <= > >= (§3.3): только взаимно-
// упорядочиваемые операнды, иначе ОшибкаТипа.
func (i *Interpreter) evalOrder(b *ast.BinaryExpr, left, right value.Value) (value.Value, error) {
	c, ok := value.Compare(left, right)
	if !ok {
		return nil, typeErr(b.Pos(), binopMsg(b, left, right))
	}
	var res bool
	switch b.Op {
	case ast.OpLt:
		res = c < 0
	case ast.OpLe:
		res = c <= 0
	case ast.OpGt:
		res = c > 0
	case ast.OpGe:
		res = c >= 0
	}
	return value.Булево{V: res}, nil
}

// binopMsg формирует TY-BINOP «'<оп>' нельзя применить к <тип> и <тип>».
func binopMsg(b *ast.BinaryExpr, left, right value.Value) string {
	return fmt.Sprintf("'%s' нельзя применить к %s и %s", b.Op.String(), left.TypeName(), right.TypeName())
}

// --- числовые помощники с явной проверкой переполнения int64 (конституция III) ---

func toFloat(v value.Value) (float64, bool) {
	switch x := v.(type) {
	case value.Целое:
		return float64(x.V), true
	case value.Дробное:
		return x.V, true
	}
	return 0, false
}

func addInt64(a, b int64) (int64, bool) {
	s := a + b
	if (b > 0 && s < a) || (b < 0 && s > a) {
		return 0, true
	}
	return s, false
}

func subInt64(a, b int64) (int64, bool) {
	d := a - b
	if (b < 0 && d < a) || (b > 0 && d > a) {
		return 0, true
	}
	return d, false
}

func mulInt64(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, false
	}
	p := a * b
	if p/b != a {
		return 0, true
	}
	return p, false
}

// floorDivInt64 — целочисленное деление с округлением к минус бесконечности
// (Python-семантика), согласовано с modInt64.
func floorDivInt64(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// modInt64 — остаток, согласованный с floorDivInt64 (знак результата = знак b).
func modInt64(a, b int64) int64 {
	r := a % b
	if r != 0 && ((r < 0) != (b < 0)) {
		r += b
	}
	return r
}
