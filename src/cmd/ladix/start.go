package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// startMain — подкоманда `ladix start <файл> <процесс> [аргументы...]` (B5, §AU-7):
// CLI-запуск инстанса процесса с типизированными argv-литералами и сверкой арности.
// rest — аргументы ПОСЛЕ «start». Дефолт Store — SQLite `ladix.db` (D-AU-10, НЕ
// Memory); --вебхук/LADIX_WEBHOOK включает HTTP-драйвер внешних эффектов (§AU-4.5).
//
// Поток: разбор флагов/позиционных → компиляция файла (lex→parse→Analyze; interp.Run
// НЕ зовётся — top-level не исполняется, чтобы start не плодил инстансы) → типизация
// argv → openStore → caller → сборка стека → СВЕРКА АРНОСТИ/неизв.процесс ДО
// eng.Start (иначе движок даёт др. текст «не найден в определении» engine.go:69, не
// §AU-10) → eng.Start (печатает [задача]-строки сам) → «запущен инстанс <id>».
func startMain(rest []string, stdout, stderr io.Writer) int {
	maxDepth := eval.DefaultMaxDepth
	dbPath := defaultDBPath // §AU-9/D-AU-10: дефолт SQLite ladix.db (НЕ Memory)
	webhook := ""
	var positional []string
	for k := 0; k < len(rest); k++ {
		a := rest[k]
		switch {
		case a == "--max-depth":
			if k+1 >= len(rest) {
				fmt.Fprintln(stderr, "ladix: флаг --max-depth требует значение")
				return 2
			}
			n, err := strconv.Atoi(rest[k+1])
			if err != nil || n <= 0 {
				fmt.Fprintln(stderr, "ladix: неверное значение --max-depth")
				return 2
			}
			maxDepth = n
			k++
		case strings.HasPrefix(a, "--max-depth="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--max-depth="))
			if err != nil || n <= 0 {
				fmt.Fprintln(stderr, "ladix: неверное значение --max-depth")
				return 2
			}
			maxDepth = n
		case a == "--db":
			if k+1 >= len(rest) {
				fmt.Fprintln(stderr, "ladix: флаг --db требует значение")
				return 2
			}
			dbPath = rest[k+1]
			k++
		case strings.HasPrefix(a, "--db="):
			dbPath = strings.TrimPrefix(a, "--db=")
		case a == "--вебхук":
			if k+1 >= len(rest) {
				fmt.Fprintln(stderr, "ladix: флаг --вебхук требует значение")
				return 2
			}
			webhook = rest[k+1]
			k++
		case strings.HasPrefix(a, "--вебхук="):
			webhook = strings.TrimPrefix(a, "--вебхук=")
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "ladix: неизвестный флаг %s\n", a)
			return 2
		default:
			positional = append(positional, a)
		}
	}
	// Минимум: <файл> <процесс>. Остальные позиционные → аргументы процесса.
	if len(positional) < 2 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	file := positional[0]
	name := positional[1]
	argvs := positional[2:]

	// Компиляция файла (как complete): лексер → парсер → Analyze (interp.Run НЕ
	// зовётся). Лекс/синт-ошибки → канон §13, exit 1.
	src, rerr := os.ReadFile(file)
	if rerr != nil {
		fmt.Fprintf(stderr, "ladix: не удалось прочитать файл %q\n", file)
		return 2
	}
	tokens, errList := lexer.New(string(src)).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		fmt.Fprintln(stderr, errList.Error())
		return 1
	}

	// Типизация позиционных argv → []value.Value (data-model §1). Ошибка распознанной
	// формы (целое вне диапазона / невалидная дата) → CLI-ошибка exit 2 ДО engine.Start.
	posArgs := make([]value.Value, len(argvs))
	for i, argv := range argvs {
		v, perr := parseArgLiteral(argv)
		if perr != nil {
			fmt.Fprintf(stderr, "ladix: не удалось разобрать аргумент '%s': %s\n", argv, perr.Error())
			return 2
		}
		posArgs[i] = v
	}

	// Store: дефолт SQLite ladix.db (D-AU-10), defer close. Ошибка открытия → exit 2.
	st, closeStore, oerr := openStore(dbPath)
	if oerr != nil {
		fmt.Fprintf(stderr, "ladix: не удалось открыть хранилище '%s': %s\n", dbPath, oerr.Error())
		return 2
	}
	defer closeStore()

	// Драйвер внешних эффектов (§AU-4.5): webhookCaller под --вебхук/env, иначе
	// дефолт-стаб. Невалидный URL → exit 2 (паритет complete, §EN-8.B).
	caller, cerr := openExternalCaller(webhook)
	if cerr != nil {
		fmt.Fprintf(stderr, "ladix: %s\n", cerr.Error())
		return 2
	}

	return guard(stderr, func() int {
		interp := eval.NewInterpreter(stdout, maxDepth, eval.SystemClock{})
		if err := interp.Analyze(prog); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1 // семпроход (§SM-9.A) — компиляция обязана пройти чисто
		}
		eng := engine.NewEngine(st, interp, stdout, withExternalCallerOpt(caller)...)
		interp.SetProcessRuntime(eng)

		// СВЕРКА ДО engine.Start (data-model §2, §AU-7.3): CLI формирует §AU-10-тексты
		// САМ. Иначе движок даёт «процесс '%s' не найден в определении» (engine.go:69)
		// и не различает арность. Обе проверки строго перед eng.Start.
		pd, ok := interp.Process(name)
		if !ok {
			fmt.Fprintf(stderr, "ladix: процесс '%s' не объявлен\n", name)
			return 2
		}
		if len(pd.Params) != len(posArgs) {
			fmt.Fprintf(stderr, "ladix: процесс '%s' ожидает %d аргументов, получено %d\n",
				name, len(pd.Params), len(posArgs))
			return 2
		}

		// Запуск: движок печатает [задача]-строки сам (printTaskCreated, §EN-7).
		id, serr := eng.Start(name, posArgs)
		if serr != nil {
			fmt.Fprintln(stderr, serr.Error())
			return 1
		}
		// §AU-10.D: startMain печатает строку подтверждения старта ПОСЛЕ успеха.
		fmt.Fprintf(stdout, "запущен инстанс %s\n", id)
		return 0
	})
}

// parseArgLiteral разбирает один позиционный argv-токен в типизированный литерал
// value.Value (data-model §1.2, §AU-7.2). Порядок первого совпадения: целое →
// дробное → булево/пусто → дата(ISO) → строка(fallback). Ошибка ТОЛЬКО при
// невалидном числовом/датовом литерале распознанной формы (целое вне Int64,
// невалидная ISO-дата); любой нераспознанный токен → value.Строка (не ошибка).
func parseArgLiteral(argv string) (value.Value, error) {
	// Форма 1: целое ^-?\d+$ → value.Целое (int64). Вне Int64 → ошибка диапазона.
	if isIntLiteral(argv) {
		n, err := strconv.ParseInt(argv, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("целое вне диапазона типа Целое")
		}
		return value.Целое{V: n}, nil
	}
	// Форма 2: дробное ^-?\d+\.\d+$ или экспонента → value.Дробное (float64).
	// Маркер ('.'/'e'/'E') + цифра ⇒ распознанная числовая форма (НЕ имя): сбой
	// ParseFloat — это малформированное/внедиапазонное число → ошибка по образцу
	// §AU-10.C (формулировка-аналог ERR-INT-RANGE, data-model §4), НЕ молчаливый
	// fallback в Строку (иначе `1.2.3` тихо станет именем).
	if isFloatLiteral(argv) {
		f, err := strconv.ParseFloat(argv, 64)
		if err != nil {
			return nil, fmt.Errorf("не удалось разобрать как Дробное")
		}
		return value.Дробное{V: f}, nil
	}
	// Формы 3/4: точные слова булево/пусто (ДО строки).
	switch argv {
	case "истина":
		return value.Булево{V: true}, nil
	case "ложь":
		return value.Булево{V: false}, nil
	case "пусто":
		return value.None, nil
	}
	// Форма 5: ISO-дата ^\d{4}-\d{2}-\d{2}$ (ДО строки). Невалидный календарь
	// (2026-13-45) / нестрогий формат → ошибка парса даты. time.Parse + re-format
	// (отвергает «2026-1-3» без паддинга и невалидные дни/месяцы).
	if isISODateShape(argv) {
		t, err := time.Parse("2006-01-02", argv)
		if err != nil || t.Format("2006-01-02") != argv {
			return nil, fmt.Errorf("невалидная дата '%s' (ожидается ГГГГ-ММ-ДД)", argv)
		}
		return value.Дата{Year: t.Year(), Month: int(t.Month()), Day: t.Day()}, nil
	}
	// Форма 6: терминальный fallback — вся строка целиком (никогда не ошибка).
	return value.Строка{V: argv}, nil
}

// isIntLiteral — argv матчит ^-?\d+$ (хотя бы одна цифра, опц. ведущий минус).
func isIntLiteral(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '-' {
		i = 1
	}
	if i >= len(s) {
		return false
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// isFloatLiteral — argv похож на дробное: содержит '.' или 'e'/'E', а вне минуса/
// цифр/'.'/'eE'/'+' нет иных символов. Дискриминатор формы (точную валидность даёт
// strconv.ParseFloat). Исключает чистое целое (форма 1 уже отработала) и строки.
func isFloatLiteral(s string) bool {
	if s == "" {
		return false
	}
	hasDigit := false
	hasMarker := false // '.' или 'e'/'E'
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			hasDigit = true
		case c == '.' || c == 'e' || c == 'E':
			hasMarker = true
		case c == '+' || c == '-':
			// знак допустим (ведущий или после экспоненты)
		default:
			return false
		}
	}
	return hasDigit && hasMarker
}

// isISODateShape — argv матчит маску ^\d{4}-\d{2}-\d{2}$ (10 символов, дефисы на
// позициях 4 и 7, остальное — цифры). Календарную валидность проверяет time.Parse.
func isISODateShape(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if i == 4 || i == 7 {
			continue
		}
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// openStore конструирует Store по dbPath (§AU-9, open-store.md): непустой →
// SQLiteStore (closeFn = sq.Close, персист); пустой → MemoryStore (closeFn = no-op,
// эфемерно). Ошибка открытия SQLite пробрасывается. Узкий снимок логики runFile
// (main.go), единая точка для start; НЕ новый метод Store (INV-1).
func openStore(dbPath string) (st store.Store, closeFn func() error, err error) {
	if dbPath == "" {
		return store.NewMemoryStore(), func() error { return nil }, nil
	}
	sq, oerr := store.NewSQLiteStore(dbPath)
	if oerr != nil {
		return nil, nil, oerr
	}
	return sq, sq.Close, nil
}
