package eval

import (
	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// builtinVchera — вчера() → Дата (§DB-2/DB-3). Арность 0 (ArityFixed N=0).
// Дата запуска через i.now() (зафиксирована ОДИН раз на run, §SM-7/§10.6, НЕ
// time.Now() напрямую) минус один день — календарной арифметикой через time.Time
// (хелперы window.go), корректно перешагивая границу месяца/года.
func builtinVchera(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	return timeToDate(dateToTime(i.now()).AddDate(0, 0, -1)), nil
}

// builtinZavtra — завтра() → Дата (§DB-2/DB-3). Арность 0; дата запуска i.now()
// плюс один день (см. builtinVchera про границы и Clock).
func builtinZavtra(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	return timeToDate(dateToTime(i.now()).AddDate(0, 0, 1)), nil
}

// builtinDlitelnost — длительность(значение, единица) → Длительность (§DB-4).
// Арность 2. Порядок валидации СТРОГО арность→тип→единица:
//   - тип: arg0 — Целое, arg1 — Строка; иначе ОшибкаТипа с ОБОИМИ фактическими
//     именами типов (даже если неверен лишь один; Дробное — ошибка, промоушена нет);
//   - единица ∈ {сек,мин,час,дн,нед,мес}; иначе ОшибкаВыполнения (гильемы «»).
//
// Результат — value.Длительность{Amount, Unit} БЕЗ нормализации (0/отрицательные
// допустимы). Позиция всех ошибок = pos (CallExpr.Pos()).
func builtinDlitelnost(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	n, ok1 := args[0].(value.Целое)
	u, ok2 := args[1].(value.Строка)
	if !ok1 || !ok2 {
		return nil, typeErr(pos, "длительность: ожидается Целое и Строка, получено "+
			args[0].TypeName()+" и "+args[1].TypeName())
	}
	if !знаемЕдиницу(u.V) {
		return nil, runtimeErr(pos, "длительность: неизвестная единица «"+u.V+"»")
	}
	return value.Длительность{Amount: n.V, Unit: u.V}, nil
}

// знаемЕдиницу — единица допустима для конструкции длительности (включая мес).
func знаемЕдиницу(u string) bool {
	switch u {
	case "сек", "мин", "час", "дн", "нед", "мес":
		return true
	}
	return false
}

// конвертер строит builtin-функцию «в_<целевая>»: арность 1, аргумент — Длительность.
// Локальная (НЕ глобал пакета) карта секундВЕдинице — множители единиц к секундам;
// «мес» в карте НЕТ (не приводится без даты-якоря). Алгоритм:
//   - тип: arg0 — Длительность; иначе ОшибкаТипа «<имя>: ожидается Длительность…»;
//   - d.Unit == "мес" → ОшибкаВыполнения «<имя>: месяцы не приводятся без даты-якоря»;
//   - totalSec = mulInt64(d.Amount, множитель); overflow → «переполнение целого числа»;
//   - результат = totalSec / делитель (целочисленное деление Go, усечение к нулю).
func конвертер(имя string, делитель int64) BuiltinFn {
	секундВЕдинице := map[string]int64{
		"сек": 1,
		"мин": 60,
		"час": 3600,
		"дн":  86400,
		"нед": 604800,
	}
	return func(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
		d, ok := args[0].(value.Длительность)
		if !ok {
			return nil, typeErr(pos, имя+": ожидается Длительность, получено "+args[0].TypeName())
		}
		if d.Unit == "мес" {
			return nil, runtimeErr(pos, имя+": месяцы не приводятся без даты-якоря")
		}
		totalSec, overflow := mulInt64(d.Amount, секундВЕдинице[d.Unit])
		if overflow {
			return nil, runtimeErr(pos, "переполнение целого числа")
		}
		return value.Целое{V: totalSec / делитель}, nil
	}
}
