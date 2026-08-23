package eval

import (
	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// builtinSegodnya — сегодня() → Дата (§SM-6, BD-2). Арность 0 (ArityFixed N=0:
// лишние аргументы отсекаются семпроходом/защитой callBuiltin формой §8.3).
// Возвращает дату запуска через i.now() — зафиксирована ОДИН раз на run (§SM-7/§10.6),
// НЕ time.Now() напрямую.
func builtinSegodnya(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	return i.now(), nil
}

// builtinData — дата(...) → Дата | Пусто (перегруженная, §SM-6/BD-3). Диспетчеризация
// по len(args) и типу аргумента ВНУТРИ Fn (механизм overloaded, как длина/диапазон):
//   - дата(Строка)         — парс ровно YYYY-MM-DD (BD-4), невалид → ОшибкаВыполнения;
//   - дата(Пусто)          — Пусто (ключ к §10.4: поле-даты без значения → вне окна);
//   - дата(Целое×3)        — сборка с валидацией календаря, невалид → ОшибкаВыполнения;
//   - 1 арг иного типа      — ОшибкаТипа «ожидается Строка или Пусто, получено <тип>»;
//   - арность 0/2/≥4 (и 3 не-Целое) — «'дата': неверное число аргументов» / §SM-9.C по типу.
//
// Позиция всех ошибок = pos (CallExpr.Pos()).
func builtinData(i *Interpreter, args []value.Value, pos ast.Position) (value.Value, error) {
	switch len(args) {
	case 1:
		switch a := args[0].(type) {
		case value.Строка:
			d, ok := parseISODate(a.V)
			if !ok {
				return nil, runtimeErr(pos, "дата: «"+a.V+"» не является датой")
			}
			return d, nil
		case value.Пусто:
			return value.None, nil
		default:
			return nil, typeErr(pos, "дата: ожидается Строка или Пусто, получено "+a.TypeName())
		}
	case 3:
		y, ok1 := args[0].(value.Целое)
		m, ok2 := args[1].(value.Целое)
		d, ok3 := args[2].(value.Целое)
		if !ok1 || !ok2 || !ok3 {
			return nil, runtimeErr(pos, "'дата': неверное число аргументов")
		}
		dt, ok := makeDate(y.V, m.V, d.V)
		if !ok {
			return nil, runtimeErr(pos, "дата: некорректные год/месяц/день")
		}
		return dt, nil
	default:
		return nil, runtimeErr(pos, "'дата': неверное число аргументов")
	}
}

// parseISODate разбирает строку как ROВНО YYYY-MM-DD (BD-4): ровно 10 символов,
// маска \d{4}-\d{2}-\d{2}, строгий григорианский календарь (високосный февраль).
// Время/таймзона/'T…'/лишние символы/нет паддинга → невалидно (ok == false).
// Собственный парс (НЕ time.Parse), чтобы «2026-13-40», «2026-5-3», «…T10:00» отвергались.
func parseISODate(s string) (value.Дата, bool) {
	b := []byte(s)
	if len(b) != 10 {
		return value.Дата{}, false
	}
	if b[4] != '-' || b[7] != '-' {
		return value.Дата{}, false
	}
	for k, c := range b {
		if k == 4 || k == 7 {
			continue
		}
		if c < '0' || c > '9' {
			return value.Дата{}, false
		}
	}
	year := digits4(b[0:4])
	month := digits2(b[5:7])
	day := digits2(b[8:10])
	return makeDate(int64(year), int64(month), int64(day))
}

// makeDate собирает Дата{Y,M,D} с валидацией календаря: M∈1..12; D∈1..daysInMonth
// (високосный февраль = 29). Любое нарушение → ok == false.
func makeDate(y, m, d int64) (value.Дата, bool) {
	if m < 1 || m > 12 {
		return value.Дата{}, false
	}
	if d < 1 || d > int64(daysInMonth(y, int(m))) {
		return value.Дата{}, false
	}
	return value.Дата{Year: int(y), Month: int(m), Day: int(d)}, true
}

// daysInMonth — число дней в месяце m года y (m уже валиден 1..12).
func daysInMonth(y int64, m int) int {
	switch m {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if isLeapYear(y) {
			return 29
		}
		return 28
	}
	return 0
}

// isLeapYear — григорианское правило: делится на 4, кроме делящихся на 100 без 400.
func isLeapYear(y int64) bool {
	return y%4 == 0 && (y%100 != 0 || y%400 == 0)
}

// digits4/digits2 — числовое значение группы из 4/2 ASCII-цифр (после проверки маски).
func digits4(b []byte) int {
	return int(b[0]-'0')*1000 + int(b[1]-'0')*100 + int(b[2]-'0')*10 + int(b[3]-'0')
}

func digits2(b []byte) int {
	return int(b[0]-'0')*10 + int(b[1]-'0')
}
