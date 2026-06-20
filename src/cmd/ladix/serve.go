package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/denis-kosyakov/ladix/internal/ast"
	"github.com/denis-kosyakov/ladix/internal/daemon"
	"github.com/denis-kosyakov/ladix/internal/engine"
	"github.com/denis-kosyakov/ladix/internal/eval"
	"github.com/denis-kosyakov/ladix/internal/lexer"
	"github.com/denis-kosyakov/ladix/internal/parser"
	"github.com/denis-kosyakov/ladix/internal/store"
)

// evalClockFromEngine (адаптер engine.Clock→eval.Clock, FR-024) вынесен в
// clock_adapter.go (C4 §C-4.2) — общий для всех путей CLI. serve продолжает
// пользоваться тем же неэкспортированным типом без изменений поведения.

// defaultInterval — период тика демона по умолчанию (--interval, FR-001).
const defaultInterval = time.Minute

// serveMain разбирает аргументы подкоманды serve и поднимает демон (007b, serve-command.md).
// rest — аргументы ПОСЛЕ «serve»: один позиционный (<файл>) + --db/--interval/--max-depth.
// Зеркало runMain по флагам: без --db — MemoryStore (эфемерно; durability и кросс-процессный
// emit требуют --db); --interval — Go-длительность (time.ParseDuration, дефолт 1m);
// невалидный флаг/нет файла → exit 2. Поведение run/metric/complete/tasks НЕ затрагивается
// (FR-001/026): serve — отдельный путь, run не зовёт демон.
func serveMain(rest []string, stdout, stderr io.Writer) int {
	maxDepth := eval.DefaultMaxDepth
	dbPath := ""
	webhook := ""
	interval := defaultInterval
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
		case a == "--interval":
			if k+1 >= len(rest) {
				fmt.Fprintln(stderr, "ladix: флаг --interval требует значение")
				return 2
			}
			d, err := time.ParseDuration(rest[k+1])
			if err != nil || d <= 0 {
				fmt.Fprintln(stderr, "ladix: неверное значение --interval")
				return 2
			}
			interval = d
			k++
		case strings.HasPrefix(a, "--interval="):
			d, err := time.ParseDuration(strings.TrimPrefix(a, "--interval="))
			if err != nil || d <= 0 {
				fmt.Fprintln(stderr, "ladix: неверное значение --interval")
				return 2
			}
			interval = d
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
	return serveFile(file, dbPath, interval, maxDepth, caller, stdout, stderr)
}

// serveFile читает+компилирует файл и поднимает демон (serve-command.md §жизненный цикл).
// Чтение файла → exit 2 (использование); лекс/синт → двухстрочный Error(), exit 1 (как run).
// Открытие SQLite (под --db) вне guard: сбой — окружение, exit 2. Остальное — под guard:
// сборка стека, Analyze (вкл. новую семош "ЧЧ:ММ" SE-TIME-FORMAT → exit 1, демон НЕ
// стартует), Run (связать глобалы), рестарт-скан ДО тиков, затем блокирующий Run(ctx) с
// грациозной остановкой по SIGINT/SIGTERM (выход 0). Часы планировщика — прод
// engine.SystemClock (отсюда же дата метрик интерпретатора, FR-024, см. buildServeDaemon).
func serveFile(path, dbPath string, interval time.Duration, maxDepth int, caller engine.ExternalCaller, stdout, stderr io.Writer) int {
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
	// без --db — эфемерный MemoryStore (durability/emit недоступны, граница FR-010).
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
		d, code := buildServeDaemon(prog, st, interval, maxDepth, engine.SystemClock{}, caller, stdout, stderr)
		if d == nil {
			return code // ошибка компиляции/семпрохода (exit 1)
		}
		// Грациозная остановка: SIGINT/SIGTERM → ctx.Done() ловится в select цикла Run
		// МЕЖДУ тиками (FR-003, SC-007). defer stop() освобождает обработчик сигналов.
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		d.RunRestartScan() // рестарт-скан ДО тиков (FR-019)
		// Защитный no-op: Run в текущей реализации возвращает только nil (тик глотает
		// Store-сбои в лог намеренно — serve долгоживущий, §EN-8 не нарушен). Ветка
		// `return 1` недостижима сегодня, но оставлена на случай, если Run станет
		// возвращать ошибку подъёма (тогда — двухстрочный вывод + exit 1, как у run).
		if rerr := d.Run(ctx); rerr != nil {
			fmt.Fprintln(stderr, rerr.Error())
			return 1
		}
		return 0
	})
}

// buildServeDaemon собирает стек интерпретатор+движок+демон над данным Store (общая
// для прод-пути serveFile и интеграционных тестов: тесты строят демон и дёргают
// tick()/RunRestartScan без блокирующего Run). Возвращает (nil, 1) при ошибке
// семпрохода (вкл. SE-TIME-FORMAT) — диагностика уже напечатана в stderr. Зеркало
// сборки стека runFile (§EN-6): SetProcessRuntime ДО Run, interp.Run связывает
// глобалы (как run), затем демон владеет реестром триггеров и метриками. clock —
// инъекция часов планировщика (прод engine.SystemClock; тест — фиксированные):
// одни и те же часы идут И в движок (WithClock), И в интерпретатор (через адаптер
// evalClockFromEngine), И в демон — двойные часы едины (FR-024).
func buildServeDaemon(prog *ast.Program, st store.Store, interval time.Duration, maxDepth int, clock engine.Clock, caller engine.ExternalCaller, stdout, stderr io.Writer) (*daemon.Daemon, int) {
	// Интерпретатор читает дату метрик ОТ инъектированных часов планировщика
	// (через адаптер engine.Clock→eval.Clock), а не из независимого eval.SystemClock:
	// иначе ResetRunState на тике перевычислял бы дату от собственных часов
	// интерпретатора, расходясь с движком и планировщиком (FR-024, двойные часы).
	// Драйвер внешних эффектов (B2): ОДИН eng на демон — догон дедлайнов, тела
	// триггеров и эскалации идут через ЭТОТ движок (FR-017, единый вебхук под флагом).
	interp := eval.NewInterpreter(stdout, maxDepth, evalClockFromEngine{clock})
	opts := append([]engine.Option{engine.WithClock(clock)}, withExternalCallerOpt(caller)...)
	eng := engine.NewEngine(st, interp, stdout, opts...)
	interp.SetProcessRuntime(eng)
	if err := interp.Analyze(prog); err != nil {
		// Семпроход: вкл. новую семош формата "ЧЧ:ММ" (SE-TIME-FORMAT, FR-014) — демон
		// НЕ стартует. Канонический двухстрочный Error() (§13).
		fmt.Fprintln(stderr, err.Error())
		return nil, 1
	}
	if err := interp.Run(prog); err != nil {
		// Связать глобальные «пусть» (как run); ошибка top-level → двухстрочный Error().
		fmt.Fprintln(stderr, err.Error())
		return nil, 1
	}
	d := daemon.New(st, eng, interp, clock, interval, stdout)
	return d, 0
}
