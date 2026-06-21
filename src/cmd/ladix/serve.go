package main

import (
	"context"
	"fmt"
	"io"
	"net"
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

// serveStore — Store с Close (закрытие durable-коннекта). store.Store сам Close не
// несёт (его реализует только *SQLiteStore); этот алиас даёт serveFile закрыть Store
// и допускает тест-подмену открывашки (N2b, пин §IE-5.2 Shutdown→Close).
type serveStore interface {
	store.Store
	Close() error
}

// openServeStore — индирекция открытия durable-Store под --db. Прод: SQLite. Тест
// подменяет на recorder-обёртку, фиксирующую порядок EnqueueEvent/Close (пин
// КРИТИЧНОГО инварианта §IE-5.2: in-flight POST дописывается ДО Close). Поведение
// прод-пути не меняет (дефолт = прежний store.NewSQLiteStore).
var openServeStore = func(dbPath string) (serveStore, error) {
	return store.NewSQLiteStore(dbPath)
}

// serveListenerReady — тест-хук готовности listener (вызывается ПОСЛЕ успешного
// net.Listen). Прод — no-op; тест читает выбранный ln.Addr() при --listen :0
// (дискавери порта). Поведение прод-пути не меняет.
var serveListenerReady = func(net.Listener) {}

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
	listen := ""
	token := ""
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
		case a == "--listen":
			if k+1 >= len(rest) {
				fmt.Fprintln(stderr, "ladix: флаг --listen требует значение")
				return 2
			}
			listen = rest[k+1]
			k++
		case strings.HasPrefix(a, "--listen="):
			listen = strings.TrimPrefix(a, "--listen=")
		case a == "--token":
			if k+1 >= len(rest) {
				fmt.Fprintln(stderr, "ladix: флаг --token требует значение")
				return 2
			}
			token = rest[k+1]
			k++
		case strings.HasPrefix(a, "--token="):
			token = strings.TrimPrefix(a, "--token=")
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
	// Токен сетевого приёмника: флаг бьёт env (§IE-6, зеркало --webhook/LADIX_WEBHOOK).
	if token == "" {
		token = os.Getenv("LADIX_LISTEN_TOKEN")
	}
	// Durability-граница (D-IE-7, FR-IE-4): --listen без --db поднял бы эфемерный
	// MemoryStore → 202 на событие, которое исчезнет при рестарте (нарушение at-least-once).
	// Проверка ПЕРЕД открытием сокета (в serveFile) — ошибка использования CLI, exit 2.
	if listen != "" && dbPath == "" {
		fmt.Fprintln(stderr, "ladix: --listen требует --db")
		return 2
	}
	caller, err := openExternalCaller(webhook)
	if err != nil {
		fmt.Fprintf(stderr, "ladix: %s\n", err.Error())
		return 2
	}
	// Грациозная остановка: SIGINT/SIGTERM → ctx.Done() (FR-003, SC-007). Контекст
	// строится ПОСЛЕ всех валидаций (ранние exit 2 обработчик сигналов не ставят) и
	// инъектируется в serveFile — тест отменяет ctx без ОС-сигнала (пин §IE-5.2).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serveFile(ctx, file, dbPath, interval, maxDepth, sourceBase, caller, listen, token, stdout, stderr)
}

// serveFile читает+компилирует файл и поднимает демон (serve-command.md §жизненный цикл).
// Чтение файла → exit 2 (использование); лекс/синт → двухстрочный Error(), exit 1 (как run).
// Открытие SQLite (под --db) вне guard: сбой — окружение, exit 2. Остальное — под guard:
// сборка стека, Analyze (вкл. новую семош "ЧЧ:ММ" SE-TIME-FORMAT → exit 1, демон НЕ
// стартует), Run (связать глобалы), рестарт-скан ДО тиков, затем блокирующий Run(ctx) с
// грациозной остановкой по SIGINT/SIGTERM (выход 0). Часы планировщика — прод
// engine.SystemClock (отсюда же дата метрик интерпретатора, FR-024, см. buildServeDaemon).
func serveFile(ctx context.Context, path, dbPath string, interval time.Duration, maxDepth int, sourceBase string, caller engine.ExternalCaller, listen, token string, stdout, stderr io.Writer) int {
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
	// Открывашка — через openServeStore (прод = NewSQLiteStore; тест подменяет, N2b).
	var st store.Store
	if dbPath != "" {
		sq, oerr := openServeStore(dbPath)
		if oerr != nil {
			fmt.Fprintf(stderr, "ladix: не удалось открыть хранилище '%s': %s\n", dbPath, oerr.Error())
			return 2
		}
		defer sq.Close()
		st = sq
	} else {
		st = store.NewMemoryStore()
	}
	// Часы планировщика — прод engine.SystemClock; ОДНИ и те же идут в демон (через
	// buildServeDaemon) И в сетевой приёмник (CreatedAt минтится ими, FR-IE-11).
	clock := engine.Clock(engine.SystemClock{})
	// Сетевой приёмник (трек B, §IE-3/§IE-5): bind синхронно ВНЕ guard, рядом с открытием
	// Store — сбой (порт занят/нет прав) = окружение → детерминированный exit 2 (НЕ exit 1
	// «внутренняя ошибка» из guard). Без --listen (ln == nil) сервер не стартует — нулевой
	// регресс (FR-IE-1). Уже открытый ln передаётся в startEventListener.
	var ln net.Listener
	if listen != "" {
		l, lerr := net.Listen("tcp", listen)
		if lerr != nil {
			fmt.Fprintf(stderr, "ladix: не удалось открыть сокет '%s': %s\n", listen, lerr.Error())
			return 2
		}
		ln = l
		serveListenerReady(ln) // тест-хук дискавери порта (прод no-op)
		// Дефолт-граница loopback (§IE-3): не-loopback host без токена → предупреждение,
		// НЕ блокируем (эндпоинт запускает процессы по сети).
		if token == "" && !isLoopbackListen(listen) {
			fmt.Fprintln(stderr, "ladix: ВНИМАНИЕ: --listen на не-loopback адресе без --token — эндпоинт запускает процессы без аутентификации")
		}
	}
	return guard(stderr, func() int {
		d, code := buildServeDaemon(prog, st, interval, maxDepth, sourceBaseDir(sourceBase, path), clock, caller, stdout, stderr)
		if d == nil {
			return code // ошибка компиляции/семпрохода (exit 1)
		}
		// КРИТИЧНЫЙ инвариант §IE-5.2 (FR-IE-6): defer stopListener() ВНУТРИ
		// guard-замыкания → по LIFO Shutdown+wg.Wait отрабатывают СТРОГО ДО внешнего
		// defer sq.Close() (выше, в области serveFile) → in-flight POST дописывается до
		// закрытия Store, горутина join-ится (FR-IE-8). НЕ выносить этот defer в область
		// serveFile: пин — TestServeListenerStopsBeforeStoreClose (events_http_test.go).
		if ln != nil {
			stopListener := startEventListener(ln, st, clock, token)
			defer stopListener()
		}
		// ctx (SIGINT/SIGTERM или тест-cancel) ловится в select цикла Run МЕЖДУ тиками
		// (FR-003, SC-007); создаётся/инъектируется serveMain.
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

// isLoopbackListen сообщает, привязан ли адрес --listen к loopback (127.0.0.1/::1/
// localhost или иной loopback-IP). Пустой host (":порт" = все интерфейсы) → НЕ loopback
// (срабатывает предупреждение §IE-3). Ошибка разбора адреса трактуется как не-loopback.
func isLoopbackListen(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
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
func buildServeDaemon(prog *ast.Program, st store.Store, interval time.Duration, maxDepth int, sourceBase string, clock engine.Clock, caller engine.ExternalCaller, stdout, stderr io.Writer) (*daemon.Daemon, int) {
	// Интерпретатор читает дату метрик ОТ инъектированных часов планировщика
	// (через адаптер engine.Clock→eval.Clock), а не из независимого eval.SystemClock:
	// иначе ResetRunState на тике перевычислял бы дату от собственных часов
	// интерпретатора, расходясь с движком и планировщиком (FR-024, двойные часы).
	// Драйвер внешних эффектов (B2): ОДИН eng на демон — догон дедлайнов, тела
	// триггеров и эскалации идут через ЭТОТ движок (FR-017, единый вебхук под флагом).
	interp := eval.NewInterpreter(stdout, maxDepth, evalClockFromEngine{clock})
	interp.SetSourceBase(sourceBase) // §SM-8.1: serveFile уже резолвил базу (sourceBaseDir), демон перечитывает источники по ней на тиках
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
