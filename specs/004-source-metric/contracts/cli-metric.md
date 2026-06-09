# Contract: CLI `ladix metric <файл> <имя>` (§SM-11)

**Feature**: 004-source-metric | **Источники**: docs/source-metric-model.md §SM-11, §SM-9.D; README (раздел `ladix metric`); [cli-карта](/tmp/ladix004map/cli.md) (диспетчер `realMain`, recover-барьер, тест-конвенции). Решения **R2** (нет store, MemoryStore безусловно = без персиста/`--db`), **R4** (SystemClock).

Вторая подкоманда CLI поверх существующего `run` ([cli-карта](/tmp/ladix004map/cli.md) §1): вычисляет одну именованную метрику из файла и печатает её значение. Тот же recover-барьер, та же семантика кодов 0/1/2, тот же тест-стиль (`realMain`, байт-в-байт).

## CM-1. Синтаксис

```
ladix metric <файл.ladix> <имя_метрики> [--max-depth N]
```

- **Два позиционных** аргумента: путь к файлу и имя метрики. Меньше двух → ошибка использования, exit **2**.
- `--max-depth N` (и `--max-depth=N`) — как у `run`: `strconv.Atoi` + `n > 0`, иначе exit 2. Дефолт `eval.DefaultMaxDepth` (10000).
- Флага `--db` **НЕТ** (R2 — метрика не трогает инстансы процессов, персиста нет).
- Обновить `usage`: добавить форму `metric` рядом с `run` (например `использование: ladix run [--max-depth N] <файл> | ladix metric [--max-depth N] <файл> <имя>`).

## CM-2. Диспетчер подкоманд (`main.go`)

Сейчас `realMain` ([cli-карта](/tmp/ladix004map/cli.md) §1) — плоский guard `if len(args) < 1 || args[0] != "run" { usage; return 2 }`. Заменить на ветвление по `args[0]`:

```go
func realMain(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	switch args[0] {
	case "run":
		return runMain(args[1:], stdout, stderr)   // существующая логика run
	case "metric":
		return metricMain(args[1:], stdout, stderr) // новая ветвь
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
}
```

- `run`-логику (разбор `--max-depth`/`file`, `runFile`) вынести в `runMain`/оставить как есть; ключевая правка — диспетч-точка в начале `realMain`.
- Прочие подкоманды (`serve`/`complete`/`tasks`/`emit`) — `default` → `usage`, exit 2 (как сейчас; §SM-11 допускает оставить).

## CM-3. Ветвь `metric` — разбор и конвейер

```go
func metricMain(args []string, stdout, stderr io.Writer) int
```

Разбор: два позиционных (`file`, `metricName`) + опциональный `--max-depth`. Нет двух позиционных / лишний позиционный / неизвестный флаг → exit **2** (как `run`).

**Конвейер** (R2 — без store; конвейер лексер→парсер→Analyze→`evalMetric`):

```go
src, err := os.ReadFile(file)                          // err → exit 2 «ladix: не удалось прочитать файл %q»
tokens, errList := lexer.New(string(src)).Tokenize()   // 1) лексер
prog := parser.New(tokens, errList).Parse()            // 2) парсер (тот же errList; SourceDecl/MetricDecl парсятся, §SM-3)
if !errList.Empty() { errList.Error() → stderr; return 1 }
// внутри guard(stderr, func() int { ... }):
interp := eval.NewInterpreter(stdout, maxDepth, eval.SystemClock{})  // R4: SystemClock, фикс на вызов
if err := interp.Analyze(prog); err != nil { err.Error() → stderr; return 1 }  // 3) семпроход (регистрация источников/метрик, §SM-4)
v, err := interp.EvalMetricByName(metricName)          // 4) найти + вычислить метрику
if err != nil { err.Error() → stderr; return 1 }
fmt.Fprintln(stdout, value.String(v))                  // 5) печать строка(результат) + \n в stdout
return 0
```

- **Печать:** `value.String(результат)` (= `строка(x)`, [values](../../003-interpreter-eval/contracts/values.md) C-2) в **stdout** + перевод строки. Например `2000000`, `1000000.0`, `пусто` (§SM-10).
- **R2 (без store):** «MemoryStore безусловно» = нет персиста, нет `--db`, нет нового пакета `internal/store`. Окружение строится внутри `NewInterpreter` само; ветвь `metric` не заводит хранилище.
- **Clock (R4):** `eval.SystemClock{}`; `сегодня()` и окна метрики фиксируются на вызов (§10.6, CK-4).

## CM-4. Поиск метрики по имени (§SM-9.D)

```go
func (i *Interpreter) EvalMetricByName(name string) (value.Value, error)
```

- Имя не зарегистрировано в реестре метрик → `СемантическаяОшибка «неизвестная метрика '<имя>'»`, exit **1**.
- Имя занято переменной/функцией/источником (не метрика) → `СемантическаяОшибка «'<имя>' — не метрика»`, exit **1**.
- Найдено → `i.evalMetric(decl)` ([metric-engine](./metric-engine.md) ME-1) → значение или ошибка вычисления/загрузки (§SM-9.B/C), exit **1**.

**Реестр ошибок CLI (§SM-9.D, дословно — без завершающей точки):**
```
неизвестная метрика '<имя>'        // СемантическаяОшибка, exit 1
'<имя>' — не метрика               // имя — переменная/функция/источник, exit 1
```

## CM-5. Коды возврата и recover-барьер

| Код | Условие |
|---|---|
| **0** | успех: метрика вычислена, `value.String` напечатан в stdout |
| **1** | штатные ошибки Ladix: непустой `errList` (лекс/синт); `Analyze`-ошибка (§SM-4/§SM-9.A); неизвестная метрика / имя-не-метрика (§SM-9.D); загрузка/вычисление (§SM-9.B/C); Go-паника, пойманная `guard` → «внутренняя ошибка интерпретатора» |
| **2** | ошибки использования: нет двух позиционных; `--max-depth` без значения/неверное; неизвестный флаг; лишний аргумент; **файл не читается** (`os.ReadFile` ошибка) |

- **recover-барьер** — тот же `guard(stderr, fn)` ([cli-карта](/tmp/ladix004map/cli.md) §1, `main.go:120`): Go-паника внутри этапа интерпретации → дословно `внутренняя ошибка интерпретатора` (+`\n`), без stack trace, exit 1. Обёрнут этап `NewInterpreter`+`Analyze`+`EvalMetricByName`; чтение файла/лексер/парсер — вне барьера.
- **Куда печатается:** результат метрики → `stdout` (через инжектированный `out`); все ошибки (usage, синтаксис, семантика, рантайм, паника) → `stderr` через `fmt.Fprintln/Fprintf`.

## CM-6. Тест-конвенции (как `run`)

Те же конвенции, что у `run` ([cli-карта](/tmp/ladix004map/cli.md) §4): белый ящик `package main`, вызов **`realMain([]string{"metric", …}, &out, &errBuf)`** с `*bytes.Buffer`, сверка тройки **код / байты stdout / байты stderr** байт-в-байт.

- **Golden stdout** (§SM-10, фикстура метрик-онли + `FixedClock 2026-05-31`): таблица `{args, want}`, `realMain(["metric", fixturePath, metricName], &out, &errBuf)`, ждёт `code == 0` и `out.String() == want`. Значения из §SM-10: `2000000`, `2500000`, `2`, `1000000.0`, `800000`, `1200000`, пустое окно → `0`, пустое `среднее` → `пусто`.
  - **Важно про Clock в тестах:** прод `metricMain` строит `SystemClock` (дата-зависим). Для детерминированного golden либо `metricMain` принимает `Clock` через тест-хук (внутренняя функция с инъекцией `Clock`, аналог `runFile`), либо golden вызывается через слой ниже CLI (`realMain`-обёртка с фикс-датой). Контракт: golden метрик считается при `FixedClock{2026-05-31}`; прод-путь — `SystemClock`. Фикстуру класть в `testdata/` (НЕ `examples/` — вывод дата-зависим, §SM-10).
- **Краевые exit-кейсы** (§SM-9.D, exit 1): `неизвестная метрика 'нет'`; `'продажи' — не метрика` (имя источника). Сверять `errBuf.String()` дословно + `code == 1`.
- **Usage-кейсы** (exit 2): `["metric"]` (нет позиционных), `["metric", file]` (один позиционный), `["metric", file, name, "лишнее"]` (лишний), `["metric", "нет-файла.ladix", name]` (нечитаемый файл), `["metric", file, name, "--max-depth", "0"]`.
- **Паника:** переиспользовать `TestGuardRecoversPanic` стиль — `guard` уже покрыт; ветвь `metric` через тот же `guard`.

## CM-7. Границы

- `metric` вычисляет **одну** метрику за вызов (не batch, не все метрики файла).
- Не исполняет императив файла (`печать` верхнего уровня не выполняется в ветви `metric` — только регистрация + вычисление целевой метрики). Если файл содержит `процесс`/`когда` — `Analyze`/парсер упадёт раньше (§SM-0); метрик-онли фикстура парсится целиком.
- Нет `--db`, нет персиста, нет нового store-пакета (R2, §SM-12).
- Подкоманды `serve`/`complete`/`tasks`/`emit` — не в 004 (`default` → usage, exit 2).
