# Contract — Команда `ladix serve` (007b)

**Anchor**: EM-1 (serve→SQLiteStore), EM-17.1 (тик/`--interval`), EM-11 (грациозная остановка),
FR-001/003/019, SC-007/008. Файл `src/cmd/ladix/serve.go`, диспетчер `main.go`.

## Сигнатура CLI

```
ladix serve <файл> [--db путь] [--interval D] [--max-depth N]
```
- `<файл>` — обязательный позиционный `.ladix` (как `run`).
- `--db путь` — SQLite-файл; без `--db` → MemoryStore (эфемерно; durability только под `--db`, FR-010).
- `--interval D` — Go-длительность (`time.ParseDuration`), дефолт `1m` (FR-001). Невалидное → exit 2.
- `--max-depth N` — как у `run` (зеркало флага 003/006).

Диспетчер `realMain` (main.go:43): `case "serve": return serveMain(args[1:], stdout, stderr)`.
`usage` (main.go:34) дополняется формой `serve`.

## Жизненный цикл `serveMain`

```
1. Разбор флагов (зеркало runMain: --db/--interval/--max-depth/файл).
   Невалидный --interval/--max-depth/нет файла → stderr + exit 2.
2. Прочитать+лексировать+распарсить файл → prog (зеркало run). Ошибка → двухстрочный Error(), exit 1.
3. guard(stderr, func() int {                       // recover-барьер CLI (Принцип III)
     // ОДНИ часы планировщика schedClock (engine.SystemClock{} в проде) → И в eval (через
     // адаптер engine.Clock→eval.Clock), И в движок (WithClock), И в демон: двойные часы
     // едины (FR-024). Независимый eval.SystemClock сломал бы это — ResetRunState на тике
     // считал бы дату метрик от собственных часов, расходясь с движком/планировщиком.
     interp := eval.NewInterpreter(stdout, maxDepth, evalClockFromEngine{schedClock})
     eng := engine.NewEngine(st, interp, stdout, engine.WithClock(schedClock)); interp.SetProcessRuntime(eng)
     if err := interp.Analyze(prog); err != nil { ... exit 1 }   // СЕМПРОХОД: вкл. новую семош "ЧЧ:ММ" (FR-014)
       // невалидный формат "ЧЧ:ММ" → СемантическаяОшибка здесь, exit 1, демон НЕ стартует
     if err := interp.Run(prog); err != nil { ... exit 1 }        // связать глобалы (как run)
     d := daemon.New(st, eng, interp, schedClock, interval, stdout)
     ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
     defer stop()
     d.RunRestartScan()        // рестарт-скан ДО тиков (FR-019)
     d.Run(ctx)                // НЕМЕДЛЕННЫЙ первый тик в t=0 (CONC-3), затем цикл тикера; выход по ctx.Done()
     return 0
   })
```

## Грациозная остановка (FR-003, SC-007)

- `signal.NotifyContext(SIGINT/SIGTERM)` → `ctx.Done()` ловится в `select` цикла `Run` МЕЖДУ тиками.
- Тикер: `defer ticker.Stop()` — горутина не утекает.
- Выход с кодом 0 (грациозно). Без полу-записанного состояния: `tick()` синхронен под `d.mu`.

## Exit-коды

| Код | Случай |
|---|---|
| 0 | грациозная остановка по сигналу |
| 1 | ошибка программы Ladix (парс/семпроход вкл. формат `"ЧЧ:ММ"`/рантайм при подъёме) |
| 2 | ошибка использования (нет файла, невалидный `--interval`/`--max-depth`, сбой открытия Store) |

## Инварианты

- Поведение `run` НЕ меняется (FR-001/026): `serve` — отдельный путь, `run` не зовёт демон.
- Двойные часы инъектируемы (тесты подменяют `schedClock` и eval-Clock); FixedClock-CLI-флаг НЕ
  вводится (FR-024, §TR-9).
- Под MemoryStore (без `--db`) демон работает, но `trigger_state`/`events` эфемерны — durability и
  кросс-процессный `emit` требуют `--db` (FR-010, граница).

## Тесты

- golden: `serve` на детерминированной testdata-фикстуре с управляемыми часами (прямой `tick()`),
  Memory+SQLite паритет (US1/US3/US4).
- graceful-stop: `Run(ctx)` + `cancel()` → возврат без утечки горутин (счётчик горутин до/после).
- семош формата: `serve` файла с `в "25:99"` → двухстрочная диагностика, exit 1, демон не стартует.
