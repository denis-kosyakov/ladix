# Research: Разрешение путей источников относительно каталога программы

Все NEEDS CLARIFICATION закрыты зафиксированными решениями заказчика (D-1..D-7). Здесь —
консолидация решений с обоснованием, отвергнутыми альтернативами и мутпробами (каждый замок
обязан КРАСНЕТЬ при инверсии фикса).

## Решение 1: способ проброса базового каталога в Interpreter

- **Decision**: поле `sourceBase string` на `eval.Interpreter` + сеттер `SetSourceBase(dir string)`.
  Сигнатура `NewInterpreter(out, maxDepth, clock)` **не меняется**.
- **Rationale**: зеркалит существующий идиоматичный паттерн `SetProcessRuntime` (DI через сеттер,
  Принцип V). Не трогает 37 call-sites `NewInterpreter` (5 прод + 32 теста) → минимальный дифф и
  нулевой риск регресса в `engine`/`daemon`/`eval`-тестах, которым база безразлична. Дефолт `""`
  → `filepath.Join("", rel) == rel` → старое cwd-relative поведение для тестов, не задающих базу.
- **Alternatives rejected**:
  - *Доп. параметр конструктора* — заставил бы менять все 37 call-sites (включая
    `engine`/`daemon`-тесты, не относящиеся к источникам); больше шума и риска ради нулевой выгоды.
  - *Функциональная опция `WithSourceBase`* — `NewInterpreter` не использует variadic-Option
    (в отличие от `engine.NewEngine`); вводить новый стиль ради одного поля непоследовательно.
  - *Глобальная переменная пакета* — запрещено Принципом V.

## Решение 2: точка и форма резолва

- **Decision**: метод `func (i *Interpreter) resolveSourcePath(p string) string` —
  `if filepath.IsAbs(p) { return p }; return filepath.Join(i.sourceBase, p)`. Применяется заменой
  `path := decl.File.Value` → `path := i.resolveSourcePath(decl.File.Value)` в loadJSON
  (`source_loader.go:68`), loadCSV (`:241`), loadNDJSON (`:321`).
- **Rationale**: единая точка резолва; `os.Open(path)` далее не меняется; текст ошибки
  «источник '%s': файл «%s» не найден» (строки 74/245/325) подставляет ту же переменную `path`,
  поэтому **автоматически** показывает резолвленный путь (FR-008) без правки шаблона сообщения
  (Принцип III: код/категория ошибки целы). `loadSource` (`:27`) — диспетчер, не трогается;
  `ResetRunState` (`:124`) сбрасывает `today`+`recordCache`, но НЕ `sourceBase` → база persist
  между тиками `serve` (FR-009).
- **Alternatives rejected**:
  - *Резолв в `loadSource` с передачей готового пути в загрузчики* — потребовал бы менять сигнатуры
    трёх загрузчиков; замена одной строки в каждом проще и локальнее.
  - *Резолв в CLI до интерпретатора (переписать `decl.File.Value`)* — мутация AST,
    нарушает листовость/иммутабельность узлов; источники грузятся лениво внутри `eval`.

## Решение 3: разбор CLI-флага `--source-base`

- **Decision**: ручной switch-паттерн, дословное зеркало `--db` (`main.go:137-145`):
  `case a == "--source-base"` (если `k+1 >= len(rest)` → `fmt.Fprintln(stderr, "ladix: флаг
  --source-base требует значение")`, `return 2`; иначе `sourceBase = rest[k+1]; k++`) +
  `case strings.HasPrefix(a, "--source-base=")` (`strings.TrimPrefix`). Во всех 5 подкомандах.
- **Rationale**: Принцип II (ручной разбор, без regex/генераторов); единообразие с существующими
  флагами; RU-сообщение и exit 2 совпадают с каноном CLI-usage-ошибок.
- **Alternatives rejected**:
  - *Пакет `flag`* — CLI исторически парсит вручную; смешивать стили запрещено бы спекой и
    сломало бы порядок/совместимость существующих флагов.
  - *env-переменная `LADIX_SOURCE_BASE`* — явно вне скоупа (заказчик: только флаг).

## Решение 4: вычисление базы по умолчанию

- **Decision**: в каждой подкоманде `base := sourceBaseFlag; if base == "" { base =
  filepath.Dir(programPath) }; interp.SetSourceBase(base)` сразу после `NewInterpreter`.
  `programPath` — позиционный аргумент, берётся как есть.
- **Rationale**: `filepath.Dir("прог.ladix") == "."` → корректно резолвится к cwd (D-5, краевой
  случай «программа в текущем каталоге»); `filepath.Dir` абсолютного пути → абсолютный каталог →
  cwd-независимость (FR-011). Приоритет флага над каталогом программы (FR-005).
- **Alternatives rejected**:
  - *Абсолютизировать `programPath` через `filepath.Abs` в прод-коде* — не требуется: `filepath.Dir`
    относительного пути даёт относительную базу, которая корректна относительно cwd процесса;
    абсолютные пути программы дают абсолютную базу. Лишняя нормализация усложнила бы без выгоды.

## Решение 5: переезд витрины данных

- **Decision**: `git mv data/ examples/data/` (5 файлов). Пути `"data/..."` в `examples/*.ladix`
  не меняются — теперь они file-relative и резолвятся к `examples/data/...`.
- **Rationale**: самодостаточность витрины (FR-010); пример запускается по пути к нему из любого
  cwd. `git mv` сохраняет историю.
- **Alternatives rejected**:
  - *Оставить `data/` в корне + править пути в примерах на `../data/...`* — хрупко и
    контринтуитивно; данные должны жить рядом с программами витрины.

## Тест-стратегия и мутпробы (Принцип VI)

| Замок | Что проверяет | Инверсия (мутпроба) → RED |
|-------|---------------|----------------------------|
| `TestResolveSourcePath` (table-driven, eval) | file-relative от заданной базы; `--source-base` override; абсолютный как есть; пустая база = относит. | вернуть `decl.File.Value` без `Join`/`IsAbs` (cwd-резолв) → file-relative и абсолютный кейсы краснеют |
| `TestLoadSourceSalesJSON` (переписан, eval) | загрузка из `examples/data/sales.json` через явный `SetSourceBase` | если резолвер игнорирует базу → файл не найден → RED |
| CLI golden (`*_golden_test.go`, переработка withRepoRoot) | stdout-байты примеров неизменны при file-relative + абсолютный путь примера, без chdir | если прод резолвит от cwd → источник не найден без chdir → RED |
| `TestRunRevenueAbsolutePathFromTempDir` (новый, smoke) | `ladix run <abs>/выручка.ladix` из `t.TempDir()` → exit 0 | если резолв от cwd → exit ≠ 0 → RED |
| `TestSourceBaseFlagMissingValue` (новый, CLI) | `--source-base` без значения → exit 2 + RU-stderr | если флаг проглатывает отсутствие значения → RED |
| `TestSourceBaseFlagOverride` (новый, CLI/eval) | `--source-base=B` и `--source-base B` дают базу B поверх каталога программы | если флаг игнорируется → источник из каталога программы → RED |

## Подтверждённые границы (из картирования кода)

- Текст ошибки: `"источник '%s': файл «%s» не найден"` / `"источник '%s': не удалось прочитать
  файл «%s»"` (`runtimeErr`, `errors.ОшибкаВыполнения`), позиция `decl.Pos()`.
- `NewInterpreter(out io.Writer, maxDepth int, clock Clock)` — 37 call-sites, сигнатура цела.
- 5 прод call-sites: run `main.go:254`, metric `main.go:313`, complete `main.go:457`,
  start `start.go:136`, serve `serve.go:300`.
- `withRepoRoot` (`metric_test.go:29`) + `repoRoot()` (`metric_test.go:14`), 14 call-sites.
- `examplePath` (`main_test.go:16`) = `filepath.Join("..","..","..","examples",name)`.
- Данные: `sales.json`, `orders.csv`, `orders.json`, `orders.ndjson`, `costs.json` (в `data/`).
- `path/filepath` НЕ импортирован в прод-файлах `source_loader.go`/`main.go`/`serve.go`/`start.go`.
