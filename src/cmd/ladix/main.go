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
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/jsonval"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/store"
	"github.com/denis-kosyakov/ladix/internal/value"
)

// sourceBaseDir выбирает базовый каталог разрешения относительных путей источников
// (§SM-8.1, фича 026): явный --source-base имеет приоритет; иначе каталог .ladix-файла
// (filepath.Dir(path); путь без каталога вида "прог.ladix" → "." → cwd). Передаётся в
// interp.SetSourceBase до Run/Analyze (абсолютный путь источника базу игнорирует).
func sourceBaseDir(sourceBase, path string) string {
	if sourceBase != "" {
		return sourceBase
	}
	return filepath.Dir(path)
}

// webhookTimeout — конечный таймаут HTTP-клиента вебхука (FR-011: реальный драйвер
// не висит на неотвечающем адресе). Композиционный корень — единственное место выбора.
const webhookTimeout = 30 * time.Second

// openExternalCaller разрешает драйвер внешних эффектов из CLI (B2, §AU-4.5 / §AU-10.C):
// URL = флаг --webhook, иначе env LADIX_WEBHOOK, иначе пусто → (nil, nil) (движок берёт
// дефолт-стаб printCaller). Невалидный URL → ошибка (движок НЕ строится). Валидный →
// webhookCaller с конечным таймаутом. Чтение env — в корне композиции, передача
// параметром (Принцип V). Вызывающий применяет WithExternalCaller только при c != nil.
func openExternalCaller(webhookFlag string) (engine.ExternalCaller, error) {
	raw := webhookFlag
	if raw == "" {
		raw = os.Getenv("LADIX_WEBHOOK")
	}
	if raw == "" {
		return nil, nil // нет вебхука → дефолт-стаб
	}
	if _, err := url.ParseRequestURI(raw); err != nil {
		return nil, fmt.Errorf("неверный URL вебхука '%s'", raw)
	}
	return engine.NewWebhookCaller(raw, &http.Client{Timeout: webhookTimeout}), nil
}

// withExternalCallerOpt — список Option для NewEngine: WithExternalCaller(caller) только
// при caller != nil, иначе пусто (движок берёт дефолт-стаб printCaller). Так дефолтный
// §EN-7 путь не зависит от вебхука (FR-002).
func withExternalCallerOpt(caller engine.ExternalCaller) []engine.Option {
	if caller == nil {
		return nil
	}
	return []engine.Option{engine.WithExternalCaller(caller)}
}

func main() {
	os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr))
}

const usage = "использование: ladix run [--max-depth N] [--db путь] [--webhook URL] <файл> | ladix metric [--max-depth N] <файл> <имя> | ladix start [--db путь] [--webhook URL] [--max-depth N] <файл> <процесс> [аргументы...] | ladix complete [--db путь] [--max-depth N] [--webhook URL] <файл> <task-id> | ladix tasks [--db путь] [исполнитель] | ladix inspect <id> [--db путь] | ladix serve [--db путь] [--interval D] [--max-depth N] [--webhook URL] <файл> | ladix emit <событие> [json] [--db путь]"

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
	case "start":
		return startMain(args[1:], engine.SystemClock{}, stdout, stderr)
	case "complete":
		return completeMain(args[1:], engine.SystemClock{}, stdout, stderr)
	case "tasks":
		return tasksMain(args[1:], engine.SystemClock{}, stdout, stderr)
	case "inspect":
		return inspectMain(args[1:], stdout, stderr)
	case "serve":
		return serveMain(args[1:], stdout, stderr)
	case "emit":
		return emitMain(args[1:], stdout, stderr)
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
	webhook := ""
	sourceBase := ""
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
		case a == "--webhook":
			if k+1 >= len(rest) {
				fmt.Fprintln(stderr, "ladix: флаг --webhook требует значение")
				return 2
			}
			webhook = rest[k+1]
			k++
		case strings.HasPrefix(a, "--webhook="):
			webhook = strings.TrimPrefix(a, "--webhook=")
		case a == "--source-base":
			if k+1 >= len(rest) {
				fmt.Fprintln(stderr, "ladix: флаг --source-base требует значение")
				return 2
			}
			sourceBase = rest[k+1]
			k++
		case strings.HasPrefix(a, "--source-base="):
			sourceBase = strings.TrimPrefix(a, "--source-base=")
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
	caller, err := openExternalCaller(webhook)
	if err != nil {
		fmt.Fprintf(stderr, "ladix: %s\n", err.Error())
		return 2
	}
	return runFile(file, dbPath, maxDepth, sourceBase, caller, engine.SystemClock{}, stdout, stderr)
}

// metricMain разбирает аргументы подкоманды metric и вычисляет одну метрику (§SM-11
// CM-3). rest — аргументы ПОСЛЕ «metric»: два позиционных (файл, имя метрики) +
// опциональный --max-depth. Прод-Clock — eval.SystemClock{} (R4, фиксируется на
// запуск). Меньше/больше двух позиционных, неверный флаг, лишний аргумент → код 2.
func metricMain(rest []string, stdout, stderr io.Writer) int {
	maxDepth := eval.DefaultMaxDepth
	sourceBase := ""
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
		case a == "--source-base":
			if k+1 >= len(rest) {
				fmt.Fprintln(stderr, "ladix: флаг --source-base требует значение")
				return 2
			}
			sourceBase = rest[k+1]
			k++
		case strings.HasPrefix(a, "--source-base="):
			sourceBase = strings.TrimPrefix(a, "--source-base=")
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
	return runMetric(positional[0], positional[1], maxDepth, sourceBase, eval.SystemClock{}, stdout, stderr)
}

// runFile исполняет один файл и возвращает код возврата (0/1/2). dbPath=="" —
// MemoryStore (эфемерно, как 003); иначе SQLiteStore (Q2, §EN-6). Открытие/инициа-
// лизация SQLite вне guard: ошибка → «не удалось открыть хранилище», exit 2.
// clock — единые часы пути (C4 §C-4.2, прод engine.SystemClock{}): дата метрик
// интерпретатора (через evalClockFromEngine), lifecycle-штампы движка (WithClock)
// и «сейчас» сводки задач берутся ОТ ОДНИХ И ТЕХ ЖЕ часов (тест — fixedClock).
func runFile(path, dbPath string, maxDepth int, sourceBase string, caller engine.ExternalCaller, clock engine.Clock, stdout, stderr io.Writer) int {
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
		interp := eval.NewInterpreter(stdout, maxDepth, evalClockFromEngine{clock})
		interp.SetSourceBase(sourceBaseDir(sourceBase, path)) // §SM-8.1: пути источников от каталога файла / --source-base
		// Стек движка процессов (006, §EN-6): Store + Engine + инъекция
		// ProcessRuntime, чтобы «запустить процесс» исполнялся. Драйвер внешних эффектов
		// (B2): webhookCaller под --webhook/env, иначе дефолт-стаб (caller == nil).
		// engine.WithClock(clock) — те же часы, что и у интерпретатора (C4 §C-4.2).
		eng := engine.NewEngine(st, interp, stdout, append([]engine.Option{engine.WithClock(clock)}, withExternalCallerOpt(caller)...)...)
		interp.SetProcessRuntime(eng)
		if err := interp.Run(prog); err != nil {
			// Все типы eval/lexer/parser реализуют канонический двухстрочный Error() (§8.1).
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		// Проход fire-if-true по триггерам (§TR-6/§TR-8.1, шаг 9): СТРОГО после
		// interp.Run (глобальные «пусть» связаны) и ДО сводки задач (процессы,
		// запущенные триггером, ещё попадут в сводку). База ЛОЖЬ эфемерно: trigger_state
		// не читается/не пишется даже под --db. Ошибка — зеркало обработки interp.Run.
		if err := interp.RunTriggers(stdout); err != nil {
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
			now := clock.Now()
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
// Analyze → сборка стека движка §EN-6 (MemoryStore+Engine, SetProcessRuntime) →
// EvalMetricByName → value.String + '\n'. Движок собирается ОБЩИМ для run/complete/
// metric (§EN-6): формула метрики может через функцию дёрнуть process-builtin/
// «запустить процесс», поэтому runtime обязателен (иначе nil-runtime §EN-8.A:685).
// Clock инжектируется (прод — SystemClock; тест — FixedClock для детерминированного
// golden). Коды: чтение файла → 2 (использование); лекс/синт, Analyze, поиск метрики
// (§SM-9.D), загрузка/вычисление (§SM-9.B/C), Go-паника → 1; успех → 0.
func runMetric(path, metricName string, maxDepth int, sourceBase string, clock eval.Clock, stdout, stderr io.Writer) int {
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
		interp.SetSourceBase(sourceBaseDir(sourceBase, path)) // §SM-8.1: пути источников от каталога файла / --source-base
		if err := interp.Analyze(prog); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1 // семпроход (§SM-4/§SM-9.A)
		}
		// Стек движка процессов (006, §EN-6): сборка ОБЩАЯ для run/complete/metric.
		// У metric Store — всегда MemoryStore (флага --db нет). SetProcessRuntime ДО
		// EvalMetricByName: формула метрики может через функцию дёрнуть process-builtin/
		// «запустить процесс» (семпроход разрешает) — иначе nil-runtime (§EN-8.A:685).
		// engine.WithClock — ТЕ ЖЕ часы, что у интерпретатора (C4 §C-4.1/T009): эффект
		// латентный (движок «сейчас» на metric-пути не штампует), но единые на будущее.
		st := store.NewMemoryStore()
		eng := engine.NewEngine(st, interp, stdout, engine.WithClock(engineClockFromEval{clock}))
		interp.SetProcessRuntime(eng)
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
// clock — единые часы пути (C4 §C-4.2, прод engine.SystemClock{}): движок штампует
// MarkTaskCompleted/UpdatedAt от ЭТИХ часов (WithClock), интерпретатор — дату метрик
// от тех же (evalClockFromEngine); тест инжектирует fixedClock для детерминизма.
func completeMain(rest []string, clock engine.Clock, stdout, stderr io.Writer) int {
	maxDepth := eval.DefaultMaxDepth
	dbPath := defaultDBPath
	webhook := ""
	sourceBase := ""
	payloadRaw := "" // сырое значение --data (декод в completeTask через jsonval)
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
		case a == "--webhook":
			if k+1 >= len(rest) {
				fmt.Fprintln(stderr, "ladix: флаг --webhook требует значение")
				return 2
			}
			webhook = rest[k+1]
			k++
		case strings.HasPrefix(a, "--webhook="):
			webhook = strings.TrimPrefix(a, "--webhook=")
		case a == "--data":
			if k+1 >= len(rest) {
				fmt.Fprintln(stderr, "ladix: флаг --data требует значение")
				return 2
			}
			payloadRaw = rest[k+1]
			k++
		case strings.HasPrefix(a, "--data="):
			payloadRaw = strings.TrimPrefix(a, "--data=")
		case a == "--source-base":
			if k+1 >= len(rest) {
				fmt.Fprintln(stderr, "ladix: флаг --source-base требует значение")
				return 2
			}
			sourceBase = rest[k+1]
			k++
		case strings.HasPrefix(a, "--source-base="):
			sourceBase = strings.TrimPrefix(a, "--source-base=")
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
	caller, err := openExternalCaller(webhook)
	if err != nil {
		fmt.Fprintf(stderr, "ladix: %s\n", err.Error())
		return 2
	}
	return completeTask(positional[0], positional[1], dbPath, maxDepth, sourceBase, payloadRaw, caller, clock, stdout, stderr)
}

// completeTask компилирует файл, собирает стек (SQLiteStore) и завершает задачу
// (§EN-6). Компиляция (лексер→парсер→Analyze) обязана пройти чисто (ошибка → канон
// §13, exit 1); interp.Run НЕ вызывается. eng.Complete печатает строки 7-10 сам.
// Ошибки-гарды Store (§EN-8.B) → exit 2; runtime-ошибка продвижения (D-14) → канон
// §13, exit 1. Подкоманда обёрнута guard/recover-барьером (конституция III).
// clock — единые часы пути (C4 §C-4.2): движок штампует MarkTaskCompleted/UpdatedAt
// от ЭТИХ часов (WithClock), интерпретатор — дату метрик от тех же (evalClockFromEngine).
func completeTask(path, taskID, dbPath string, maxDepth int, sourceBase, payloadRaw string, caller engine.ExternalCaller, clock engine.Clock, stdout, stderr io.Writer) int {
	// Декод --data (B3, §AU-5.3) на CLI — корень композиции, импорт jsonval допустим;
	// движок получает уже готовую value.Запись. Пустой payload → пустая Запись без
	// ошибки (поведение jsonval). Невалидный JSON / не-объект → дословная ошибка exit 2
	// ДО любой мутации Store (валидация прежде Engine).
	data, derr := jsonval.PayloadToRecord(payloadRaw)
	if derr != nil {
		fmt.Fprintf(stderr, "ladix: неверный JSON в --data: %s\n", derr.Error())
		return 2
	}
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
		interp := eval.NewInterpreter(stdout, maxDepth, evalClockFromEngine{clock})
		interp.SetSourceBase(sourceBaseDir(sourceBase, path)) // §SM-8.1: пути источников от каталога файла / --source-base
		if err := interp.Analyze(prog); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1 // семпроход (§SM-9.A) — компиляция обязана пройти чисто
		}
		// engine.WithClock(clock) — те же часы: MarkTaskCompleted/UpdatedAt детерминир. (C4).
		eng := engine.NewEngine(sq, interp, stdout, append([]engine.Option{engine.WithClock(clock)}, withExternalCallerOpt(caller)...)...)
		interp.SetProcessRuntime(eng)
		// interp.Run НЕ вызывается (Q3): top-level не исполняется, чтобы complete не
		// плодил новые инстансы. Печать строк 7-10 (§EN-7) делает сам Complete.
		// Эффекты вызвать/уведомить на догоне дедлайнов идут через eng (вебхук под флагом).
		if _, cerr := eng.Complete(taskID, data); cerr != nil {
			return completeError(cerr, taskID, stderr)
		}
		return 0
	})
}

// completeError транслирует ошибку eng.Complete в CLI-вывод и exit-код (§EN-8.B, D-20,
// FR-018). Маршрутизация по типу/сентинелу:
//   - *engine.GuardError — нарушение гарда (дрейф Q3 / D-8 / «уже завершена» / инстанс
//     не найден): текст УЖЕ совпадает с §EN-8.B минус префикс → «ladix: <текст>», exit 2.
//   - *engine.StoreError — сбой Store на CLI-пути complete (B9): «ladix: <текст>»
//     (= «ladix: сбой хранилища: <причина>»), exit 2.
//   - сентинелы Store ErrTaskNotFound/ErrTaskAlreadyCompleted (B1/B2): taskID известен
//     CLI → формируем текст здесь, exit 2.
//   - прочее (ОшибкаТипа/ОшибкаВыполнения тела/атрибута из advance, D-14): канон §13,
//     двухстрочный Error(), exit 1.
//
// GuardError/StoreError проверяются errors.As ПЕРЕД сентинелами: StoreError.Unwrap
// возвращает причину, которая сентинелом Store не является (сбой — не «не найдена»).
func completeError(err error, taskID string, stderr io.Writer) int {
	var ge *engine.GuardError
	if errors.As(err, &ge) {
		fmt.Fprintln(stderr, "ladix: "+ge.Error())
		return 2
	}
	var se *engine.StoreError
	if errors.As(err, &se) {
		fmt.Fprintln(stderr, "ladix: "+se.Error())
		return 2
	}
	switch {
	case errors.Is(err, store.ErrTaskNotFound):
		fmt.Fprintf(stderr, "ladix: задача '%s' не найдена\n", taskID)
		return 2
	case errors.Is(err, store.ErrTaskAlreadyCompleted):
		fmt.Fprintf(stderr, "ladix: задача '%s' уже завершена\n", taskID)
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
// clock — единые часы пути (C4 §C-4.2, прод engine.SystemClock{}): «сейчас» для
// FormatTaskLine берётся от инъектированных часов (тест — fixedClock).
func tasksMain(rest []string, clock engine.Clock, stdout, stderr io.Writer) int {
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
	return listTasks(assignee, dbPath, clock, stdout, stderr)
}

// listTasks открывает SQLiteStore и печатает открытые задачи фильтра (§EN-6).
// st.ListPendingTasks(фильтр) → FormatTaskLine на задачу (строка 6 §EN-7); пусто →
// «открытых задач нет» (строка 11). Exit 0 в обоих случаях. now — от инъектированных
// часов clock (C4 §C-4.2, прод engine.SystemClock{}, инвариант D-2/D-22; тест —
// fixedClock). Обёрнута guard/recover-барьером (конституция III).
func listTasks(assignee, dbPath string, clock engine.Clock, stdout, stderr io.Writer) int {
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
		now := clock.Now()
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
