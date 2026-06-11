// Command ladix — CLI интерпретатора Ladix (фичи 003/004).
//
// Подкоманда run исполняет файл .ladix: конвейер лексер → парсер (накопленные
// синтаксические ошибки → печать и выход) → Analyze (семпроход, первая ошибка →
// печать и выход) → Interpreter.Run. Подкоманда metric (фича 004) вычисляет одну
// именованную метрику из файла и печатает её значение в stdout: лексер → парсер →
// Analyze (регистрация источников/метрик) → EvalMetricByName → value.String. Флаг
// --max-depth N задаёт лимит глубины рекурсии. Граница подкоманды обёрнута
// recover-барьером: штатные ошибки Ladix → stderr/код 1; Go-паника → «внутренняя
// ошибка интерпретатора» без stack trace/код 1; ошибка использования CLI → код 2;
// успех → код 0 (§10, §SM-11/§SM-9.D).
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

func main() {
	os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = "использование: ladix run [--max-depth N] [--db путь] <файл> | ladix metric [--max-depth N] <файл> <имя> | ladix complete [--db путь] [--max-depth N] <файл> <task-id> | ladix tasks [--db путь] [исполнитель]"

// realMain — диспетчер подкоманд (§SM-11 CM-2): ветвление по args[0]. Возвращает
// код возврата. Вынесен из main и параметризован вводом/выводом для тестируемости.
func realMain(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	switch args[0] {
	case "run":
		return runMain(args[1:], stdout, stderr)
	case "metric":
		return metricMain(args[1:], stdout, stderr)
	case "complete":
		return completeMain(args[1:], stdout, stderr)
	case "tasks":
		return tasksMain(args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
}

// defaultDBPath — путь к БД по умолчанию для complete/tasks (README:195), когда
// --db не задан.
const defaultDBPath = "ladix.db"

// runMain разбирает аргументы подкоманды run и исполняет файл (логика фичи 003 +
// флаг --db фичи 006). rest — аргументы ПОСЛЕ «run». Без --db — MemoryStore
// (эфемерно, как 003); с --db — SQLiteStore (Q2). Один позиционный (файл).
func runMain(rest []string, stdout, stderr io.Writer) int {
	maxDepth := eval.DefaultMaxDepth
	dbPath := ""
	file := ""
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
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "ladix: неизвестный флаг %s\n", a)
			return 2
		default:
			if file != "" {
				fmt.Fprintln(stderr, "ladix: лишний аргумент "+a)
				return 2
			}
			file = a
		}
	}
	if file == "" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	return runFile(file, dbPath, maxDepth, stdout, stderr)
}

// metricMain разбирает аргументы подкоманды metric и вычисляет одну метрику (§SM-11
// CM-3). rest — аргументы ПОСЛЕ «metric»: два позиционных (файл, имя метрики) +
// опциональный --max-depth. Прод-Clock — eval.SystemClock{} (R4, фиксируется на
// запуск). Меньше/больше двух позиционных, неверный флаг, лишний аргумент → код 2.
func metricMain(rest []string, stdout, stderr io.Writer) int {
	maxDepth := eval.DefaultMaxDepth
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
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "ladix: неизвестный флаг %s\n", a)
			return 2
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) != 2 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	return runMetric(positional[0], positional[1], maxDepth, eval.SystemClock{}, stdout, stderr)
}

// runFile исполняет один файл и возвращает код возврата (0/1/2). dbPath=="" —
// MemoryStore (эфемерно, как 003); иначе SQLiteStore (Q2, §EN-6). Открытие/инициа-
// лизация SQLite вне guard: ошибка → «не удалось открыть хранилище», exit 2.
func runFile(path, dbPath string, maxDepth int, stdout, stderr io.Writer) int {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "ladix: не удалось прочитать файл %q\n", path)
		return 2 // ошибка использования CLI
	}
	tokens, errList := lexer.New(string(src)).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		fmt.Fprintln(stderr, errList.Error())
		return 1 // накопленные лексические/синтаксические ошибки
	}
	// Store: с --db открываем SQLite ДО guard (ошибка открытия — окружение, exit 2);
	// без --db — эфемерный MemoryStore. defer Close() только после успешного открытия.
	var st store.Store
	if dbPath != "" {
		sq, oerr := store.NewSQLiteStore(dbPath)
		if oerr != nil {
			fmt.Fprintf(stderr, "ladix: не удалось открыть хранилище '%s': %s\n", dbPath, oerr.Error())
			return 2
		}
		defer sq.Close()
		st = sq
	} else {
		st = store.NewMemoryStore()
	}
	return guard(stderr, func() int {
		interp := eval.NewInterpreter(stdout, maxDepth, eval.SystemClock{})
		// Стек движка процессов (006, §EN-6): Store + Engine + инъекция
		// ProcessRuntime, чтобы «запустить процесс» исполнялся.
		eng := engine.NewEngine(st, interp, stdout)
		interp.SetProcessRuntime(eng)
		if err := interp.Run(prog); err != nil {
			// Все типы eval/lexer/parser реализуют канонический двухстрочный Error() (§8.1).
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		// Сводка висящих задач (§EN-6 шаг 4, §EN-7 строки 5/6): только при N ≥ 1.
		pending, perr := st.ListPendingTasks("")
		if perr != nil {
			fmt.Fprintf(stderr, "ladix: сбой хранилища: %s\n", perr.Error())
			return 2
		}
		if len(pending) > 0 {
			now := engine.SystemClock{}.Now()
			fmt.Fprintf(stdout, "открытых задач: %d\n", len(pending))
			for _, t := range pending {
				fmt.Fprintln(stdout, engine.FormatTaskLine(t, now))
			}
		}
		return 0
	})
}

// runMetric вычисляет одну именованную метрику из файла и печатает её значение в
// stdout (§SM-11 CM-3/CM-5). Конвейер: чтение файла → лексер → парсер → (под guard)
// Analyze → EvalMetricByName → value.String + '\n'. Clock инжектируется (прод —
// SystemClock; тест — FixedClock для детерминированного golden, аналог runFile).
// Коды: чтение файла → 2 (использование); лекс/синт, Analyze, поиск метрики
// (§SM-9.D), загрузка/вычисление (§SM-9.B/C), Go-паника → 1; успех → 0.
func runMetric(path, metricName string, maxDepth int, clock eval.Clock, stdout, stderr io.Writer) int {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "ladix: не удалось прочитать файл %q\n", path)
		return 2 // ошибка использования CLI (вне recover-барьера, CM-5)
	}
	tokens, errList := lexer.New(string(src)).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		fmt.Fprintln(stderr, errList.Error())
		return 1 // накопленные лексические/синтаксические ошибки
	}
	return guard(stderr, func() int {
		interp := eval.NewInterpreter(stdout, maxDepth, clock)
		if err := interp.Analyze(prog); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1 // семпроход (§SM-4/§SM-9.A)
		}
		v, err := interp.EvalMetricByName(metricName)
		if err != nil {
			// §SM-9.D (поиск метрики) / §SM-9.B/C (загрузка/вычисление):
			// канонический двухстрочный Error().
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		fmt.Fprintln(stdout, value.String(v))
		return 0
	})
}

// completeMain разбирает аргументы подкоманды complete и завершает задачу (Q3,
// §EN-6). rest — аргументы ПОСЛЕ «complete»: ровно ДВА позиционных (файл, task-id)
// + опциональные --db (дефолт ladix.db) и --max-depth. Компиляция файла обязана
// пройти чисто (лексер→парсер→Analyze); interp.Run НЕ вызывается (top-level не
// исполняется). Печать строк 7-10 делает сам eng.Complete.
func completeMain(rest []string, stdout, stderr io.Writer) int {
	maxDepth := eval.DefaultMaxDepth
	dbPath := defaultDBPath
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
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "ladix: неизвестный флаг %s\n", a)
			return 2
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) != 2 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	return completeTask(positional[0], positional[1], dbPath, maxDepth, stdout, stderr)
}

// completeTask компилирует файл, собирает стек (SQLiteStore) и завершает задачу
// (§EN-6). Компиляция (лексер→парсер→Analyze) обязана пройти чисто (ошибка → канон
// §13, exit 1); interp.Run НЕ вызывается. eng.Complete печатает строки 7-10 сам.
// Ошибки-гарды Store (§EN-8.B) → exit 2; runtime-ошибка продвижения (D-14) → канон
// §13, exit 1. Подкоманда обёрнута guard/recover-барьером (конституция III).
func completeTask(path, taskID, dbPath string, maxDepth int, stdout, stderr io.Writer) int {
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "ladix: не удалось прочитать файл %q\n", path)
		return 2 // ошибка использования CLI
	}
	tokens, errList := lexer.New(string(src)).Tokenize()
	prog := parser.New(tokens, errList).Parse()
	if !errList.Empty() {
		fmt.Fprintln(stderr, errList.Error())
		return 1 // накопленные лексические/синтаксические ошибки
	}
	// Store: открытие SQLite вне guard (ошибка открытия — окружение, exit 2).
	sq, oerr := store.NewSQLiteStore(dbPath)
	if oerr != nil {
		fmt.Fprintf(stderr, "ladix: не удалось открыть хранилище '%s': %s\n", dbPath, oerr.Error())
		return 2
	}
	defer sq.Close()
	return guard(stderr, func() int {
		interp := eval.NewInterpreter(stdout, maxDepth, eval.SystemClock{})
		if err := interp.Analyze(prog); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1 // семпроход (§SM-9.A) — компиляция обязана пройти чисто
		}
		eng := engine.NewEngine(sq, interp, stdout)
		interp.SetProcessRuntime(eng)
		// interp.Run НЕ вызывается (Q3): top-level не исполняется, чтобы complete не
		// плодил новые инстансы. Печать строк 7-10 (§EN-7) делает сам Complete.
		if _, cerr := eng.Complete(taskID); cerr != nil {
			return completeError(cerr, taskID, stderr)
		}
		return 0
	})
}

// completeError транслирует ошибку eng.Complete в CLI-вывод и exit-код. Сентинелы
// Store → CLI-тексты §EN-8.B (exit 2, инстанс не тронут — базовый путь US2);
// прочие (runtime-ошибки тела/атрибута из advance, D-14) — канонический §13 Error()
// в stderr, exit 1. taskID известен CLI-слою — подставляется в текст напрямую.
// Полный маппинг дрейф-гардов Q3 / D-8 / гард-догона D-4 (включая id инстанса) — US3.
func completeError(err error, taskID string, stderr io.Writer) int {
	switch {
	case errors.Is(err, store.ErrTaskNotFound):
		fmt.Fprintf(stderr, "ladix: задача '%s' не найдена\n", taskID)
		return 2
	case errors.Is(err, store.ErrTaskAlreadyCompleted):
		fmt.Fprintf(stderr, "ladix: задача '%s' уже завершена\n", taskID)
		return 2
	case errors.Is(err, store.ErrInstanceNotFound):
		// id инстанса из чужой/битой БД печатает US3 (типизированный гард); в US2
		// базовый путь предполагает корректный вход — ветка недостижима в сценарии Б.
		fmt.Fprintln(stderr, "ladix: сбой хранилища: "+err.Error())
		return 2
	default:
		// Runtime-ошибка продвижения (D-14): канон §13, двухстрочный Error(), exit 1.
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
}

// tasksMain разбирает аргументы подкоманды tasks и печатает открытые задачи (§EN-6).
// rest — аргументы ПОСЛЕ «tasks»: один опциональный позиционный (фильтр-исполнитель)
// + опциональный --db (дефолт ladix.db). Файл НЕ принимается — всё из БД; движок/
// интерпретатор НЕ строятся. Обёрнута guard/recover-барьером.
func tasksMain(rest []string, stdout, stderr io.Writer) int {
	dbPath := defaultDBPath
	var positional []string
	for k := 0; k < len(rest); k++ {
		a := rest[k]
		switch {
		case a == "--db":
			if k+1 >= len(rest) {
				fmt.Fprintln(stderr, "ladix: флаг --db требует значение")
				return 2
			}
			dbPath = rest[k+1]
			k++
		case strings.HasPrefix(a, "--db="):
			dbPath = strings.TrimPrefix(a, "--db=")
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "ladix: неизвестный флаг %s\n", a)
			return 2
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) > 1 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	assignee := ""
	if len(positional) == 1 {
		assignee = positional[0]
	}
	return listTasks(assignee, dbPath, stdout, stderr)
}

// listTasks открывает SQLiteStore и печатает открытые задачи фильтра (§EN-6).
// st.ListPendingTasks(фильтр) → FormatTaskLine на задачу (строка 6 §EN-7); пусто →
// «открытых задач нет» (строка 11). Exit 0 в обоих случаях. now — от SystemClock
// (инвариант D-2/D-22). Обёрнута guard/recover-барьером (конституция III).
func listTasks(assignee, dbPath string, stdout, stderr io.Writer) int {
	sq, oerr := store.NewSQLiteStore(dbPath)
	if oerr != nil {
		fmt.Fprintf(stderr, "ladix: не удалось открыть хранилище '%s': %s\n", dbPath, oerr.Error())
		return 2
	}
	defer sq.Close()
	return guard(stderr, func() int {
		tasks, lerr := sq.ListPendingTasks(assignee)
		if lerr != nil {
			fmt.Fprintf(stderr, "ladix: сбой хранилища: %s\n", lerr.Error())
			return 2
		}
		if len(tasks) == 0 {
			fmt.Fprintln(stdout, "открытых задач нет")
			return 0
		}
		now := engine.SystemClock{}.Now()
		for _, t := range tasks {
			fmt.Fprintln(stdout, engine.FormatTaskLine(t, now))
		}
		return 0
	})
}

// guard — recover-барьер на границе подкоманды (§10.2): непредвиденная Go-паника
// → дженерик «внутренняя ошибка интерпретатора» без stack trace, код 1.
func guard(stderr io.Writer, fn func() int) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(stderr, "внутренняя ошибка интерпретатора")
			code = 1
		}
	}()
	return fn()
}
