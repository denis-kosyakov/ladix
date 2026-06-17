# Implementation Plan: B5 — `ladix start <процесс> [аргументы]` (CLI)

**Branch**: `017-cli-start` | **Date**: 2026-06-17 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/017-cli-start/spec.md`. Источник истины:
`docs/automation-model.md` §AU-7, §AU-9, §AU-10, §AU-4.5, D-AU-7, D-AU-10.

## Summary

Добавить CLI-подкоманду `start` (`startMain` в `cmd/ladix`), запускающую инстанс объявленного процесса
в SQLite через существующий `engine.Start`. Позиционные argv парсятся в типизированные литералы
`value.Value` (Целое/Дробное/Булево/Пусто/Дата/Строка); число аргументов сверяется с `len(pd.Params)`
ДО запуска. `start` дефолтит в SQLite `ladix.db` (семья complete/tasks/emit, D-AU-10), проводит
`--вебхук` тем же helper'ом `parseWebhookCaller`/`withExternalCallerOpt`, что run/serve/complete (B2).
Конструкция Store выделяется в хелпер `openStore(dbPath)` (устранение 5 инлайн-дублей, §AU-9). Все
CLI-ошибки и stdout канон — дословно §AU-10. CLI-only: НЕ трогаем `ProcessRuntime` (=8), Store (=16),
каталог диагностик (L=11/SE=14/eval=28), eval/engine/store-логику.

## Technical Context

**Language/Version**: Go 1.22+ (Constitution I); `gofmt` + `go vet ./...` без замечаний.

**Primary Dependencies**: stdlib only. Хранилище — `modernc.org/sqlite` (уже в графе). **0 новых
зависимостей** (Constitution I; SC-007).

**Storage**: SQLite (дефолт `ladix.db`) через `store.NewSQLiteStore`; `start` — семья complete/tasks/emit
(D-AU-10). MemoryStore не используется в `start` дефолте.

**Testing**: `go test ./...` + race на demon-путях (не затрагивается B5). Golden — `cmd/ladix/*_test.go`
(exact-match stdout с маскированием `<DT>`/`<ID>` существующими regexp `idMaskRE`). Tests-first
(Constitution VI) для парсера литералов и CLI-ошибок.

**Target Platform**: один статический бинарник `ladix` (любая ОС), CGO запрещён.

**Project Type**: CLI-подкоманда интерпретатора DSL (single project, `src/`).

**Performance Goals**: N/A (одноразовый CLI-запуск инстанса).

**Constraints**: детерминизм golden (засев id p-000001/t-000001 на свежей БД; `FixedClock` для дедлайна
дат); русские сообщения дословно §AU-10 (Constitution VIII).

**Scale/Scope**: ~1 новый файл `cmd/ladix/start.go` + хелпер `openStore` + парсер argv-литералов
(в `start.go` или соседнем), 1 ветка switch в `main.go`, расширение `usage`-строки, golden-тесты,
1 пример-фикстура для golden. Прод-логика eval/engine/store — ПУСТОЙ дифф.

## Constitution Check

*GATE: пройдено до Phase 0; повторная проверка после Phase 1.*

| # | Принцип | Статус | Обоснование |
|---|---------|--------|-------------|
| I | Язык и сборка | PASS | Go 1.22+, stdlib only, 0 новых зависимостей, CGO нет. |
| II | Парсинг — ручной | PASS | argv-литералы парсятся ВРУЧНУЮ (посимвольная проверка форм + `strconv`), БЕЗ regexp на токенизацию языка. Формы argv (^-?\d+$ и т.п.) описаны как грамматика, реализация — ручные проверки/`strconv.ParseInt/ParseFloat`; regexp на argv-форму НЕ применяется к исходнику языка (Constitution II касается токенизации языка, не CLI-argv). См. Complexity Tracking, запись CT-1. |
| III | Ошибки — явные типы | PASS | CLI-ошибки B5 — `ladix: …` stderr exit 2 (§EN-8.B уровень команды), НЕ программные типы. recover-барьер подкоманды наследуется из realMain. Паники нет. |
| IV | Позиции — сквозные | PASS | B5 не вводит языковых диагностик с позициями; argv-ошибки — CLI-уровня без Line/Col (вне исходника). |
| V | Без глобального состояния | PASS | `startMain` строит Store/interp/engine явно, инжект через параметры; пакет-глобалов нет. |
| VI | Тесты — вперёд | PASS | tests-first для парсера литералов (табличный) и CLI-ошибок (exact-match), включая негативы (плохой литерал/арность/неизв.процесс). |
| VII | Раскладка проекта | PASS | Новый код — `cmd/ladix/`; eval/engine/store/value не меняются (листовость цела). |
| VIII | Язык сообщений | PASS | Все тексты — дословно §AU-10 (русский). CLI-ошибки — `ladix: …`; stdout канон — §AU-10.D. |
| IX | Спека — источник истины | PASS | Решения залочены §AU-7/§AU-9/§AU-10/§AU-4.5/D-AU-7/D-AU-10; пробелов нет, додумывания нет. |

**Итог: 9/9 PASS.** Единственная запись в Complexity Tracking — CT-1 (форма argv через regexp на
CLI-уровне, не на токенизации языка); это НЕ нарушение Constitution II, фиксируется для прозрачности.

## Решения плана

### Р-1. Вебхук в `start` — НУЖЕН (FR-013)
§AU-4.5 явно перечисляет `start` среди команд, принимающих `--вебхук`. `engine.Start` синхронно гонит
`advance` от первого шага; терминальный/автоматический первый шаг (с `вызвать`/`уведомить`) исполнит
эффекты НА СТАРТЕ → драйвер должен быть проведён в тот же движок, иначе тихий стаб. Реализация — паритет
с complete: `parseWebhookCaller(flag, stderr)` (main.go:39-54, уже существует) → `withExternalCallerOpt`
→ `NewEngine(...)`. Невалидный URL → `ladix: неверный URL вебхука '<URL>'` exit 2 (уже формируется тем же
хелпером). НЕ дублировать логику вебхука — переиспользовать существующие хелперы B2.

### Р-2. Арность/неизвестный процесс проверяет `startMain`, НЕ `engine.Start` (FR-010/011)
`engine.Start` САМ зовёт `e.interp.Process(name)` и на `!ok` возвращает ДРУГОЙ текст
`процесс '%s' не найден в определении` (engine.go:69, защитный). Поэтому `startMain` ОБЯЗАН:
(1) `pd, ok := interp.Process(name)`; `!ok` → `ladix: процесс '<имя>' не объявлен` exit 2 ДО `engine.Start`;
(2) `len(pd.Params)` vs `len(argv)`; mismatch → `ladix: процесс '<имя>' ожидает <N> аргументов, получено <M>`
exit 2 ДО `engine.Start`. Так пользователь видит §AU-10-текст, а не движковый.

### Р-3. `openStore(dbPath)` — хелпер конструкции Store (FR-020, §AU-9)
Узкий снимок логики runFile (main.go:235-244): `dbPath != "" → store.NewSQLiteStore(dbPath)` (defer Close
у вызывающего), иначе `store.NewMemoryStore()`. Сигнатура (решение impl, зафиксировать в contract):
`openStore(dbPath string) (st store.Store, closeFn func() error, err error)` — closeFn = `sq.Close` или
no-op для Memory. `start` зовёт с дефолтом `defaultDBPath="ladix.db"`. Опциональный рефактор существующих
команд под `openStore` — ТОЛЬКО без регресса golden (замок); если рискованно — `start` использует хелпер,
дубли остаются (минимальная правка). Решение: ввести хелпер + переиспользовать в `start`; рефактор
остальных — по усмотрению impl при зелёном golden.

### Р-4. Стек `start` = стек `run` без top-level Run (как complete)
`startMain` компилирует файл (lex→parse→Analyze), при ошибке → диагностика exit 1 (зеркало complete,
main.go:408-445). top-level `interp.Run` НЕ зовётся. Затем `interp.SetProcessRuntime(eng)` ДО
`engine.Start`. `engine.Start` печатает `[задача]`-строки сам (printTaskCreated); `startMain` печатает
`запущен инстанс <id>` ПОСЛЕ успешного `Start`.

### Р-5. Парсер argv-литералов (FR-005..009, §AU-7.2)
Функция `parseArgLiteral(argv string) (value.Value, error)` — порядок проверок (первое совпадение
выигрывает): целое → дробное → булево/пусто → дата(ISO) → строка (fallback). Целое вне Int64 →
ошибка `целое вне диапазона типа Целое`. Дата формы ISO с невалидным календарём → ошибка (единообразно
M1). Детали — data-model.

## Project Structure

### Documentation (this feature)

```text
specs/017-cli-start/
├── plan.md              # этот файл
├── research.md          # Phase 0: прецеденты engine.Start/interp.Process/M1-дата/вебхук/CLI-флаги
├── data-model.md        # Phase 1: парсер argv-литералов (грамматика+типы) + сверка арности
├── quickstart.md        # Phase 1: ручной прогон ladix start (демо §AU-12.B шаг 1)
├── contracts/
│   ├── start-cli.md       # CLI-контракт start (грамматика вызова, флаги, exit-коды, stdout канон, ошибки)
│   └── open-store.md      # контракт хелпера openStore (сигнатура, поведение, регресс-инвариант)
└── tasks.md             # Phase 2 (/speckit-tasks — не создаётся планом)
```

### Source Code (repository root, под `src/`)

```text
src/cmd/ladix/
├── main.go              # +ветка switch "start" → startMain; +usage-строка с start; (опц.) openStore
├── start.go             # НОВЫЙ: startMain + parseArgLiteral (или отдельный литерал-файл)
├── start_golden_test.go # НОВЫЙ: exact-match stdout канон + арность + типы литералов + плохой литерал
└── (existing: emit.go, serve.go, …)  # ПУСТОЙ дифф прод-логики (кроме опц. рефактора под openStore)

src/internal/engine/  store/  eval/  value/   # ПУСТОЙ дифф (CLI потребляет существующий API)

src/examples/
├── эскалация_плана.ladix  # НОВАЯ фикстура (или переиспользовать существующую из 016) для start-golden
└── MANIFEST.md            # регистрация новой фикстуры (если добавлена)
```

**Structure Decision**: single project, весь новый код — в `cmd/ladix/`. Несущий инвариант:
`internal/engine`, `internal/store`, `internal/eval`, `internal/value` — ПУСТОЙ дифф (CLI-only, INV-1).

## Complexity Tracking

| Запись | Что | Зачем | Почему проще нельзя |
|--------|-----|-------|---------------------|
| CT-1 | regexp/форм-проверка argv-литералов на CLI-уровне | различить Целое/Дробное/Дата/Строка из плоской argv-строки | Constitution II запрещает regexp на ТОКЕНИЗАЦИИ ЯЗЫКА (исходник `.ladix`); argv — не исходник языка, а CLI-ввод. Альтернатива (полный лексер на argv) избыточна для 6 скалярных форм. Реализация может быть и ручными проверками + `strconv` (предпочтительно), regexp — опционально. НЕ нарушение, запись для прозрачности (§AU-7.2 задаёт формы как ^…$). |

> Прочих нарушений нет. Constitution 9/9 PASS.
