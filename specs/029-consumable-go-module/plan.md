# Implementation Plan: Потребляемый Go-модуль LADIX

**Branch**: `029-consumable-go-module` | **Date**: 2026-06-28 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/029-consumable-go-module/spec.md`

## Summary

Рефактор-упаковка LADIX в **потребляемый Go-модуль** `github.com/denis-kosyakov/ladix`
**без изменения семантики/синтаксиса языка** — только перемещение, экспорт и упаковка. Выбран
**путь A (широкий публичный фронтенд)** + поправка конституции до версии **2.0.0** (уже сделана,
FR-017): Принцип VII переопределён (публичный фронтенд вне `internal/`, граница «публичный
контракт vs internal reference-impl», инвариант изоляции `sqlite`/`internal`-backend + тест-страж).

Конкретика переупаковки:

- **`go.mod` переезжает из `src/` в корень репозитория.** Module-path **неизменен**
  (`github.com/denis-kosyakov/ladix`, без сегмента `/src`), поэтому сам переезд модуля внутренние
  импорты НЕ ломает (FR-001/FR-002).
- **Из `internal/` наружу выносятся** `lexer`, `parser`, `ast`, `value`, `errors` + **новый
  публичный пакет `ir`** (типы IR со `schema_version`) + **корневой пакет-фасад `ladix`** (узкая
  точка входа `Compile`/`CompileFile`). Все затронутые импорт-пути обновляются механически (FR-003).
- **Backend остаётся internal reference-impl**: `eval`, `engine`, `store`, `daemon`, `jsonval`
  под `internal/`; CLI — `cmd/ladix` (FR-004/FR-012). `eval` НЕ раскалывается: наружу выносится
  IR-контракт, исполнитель остаётся внутри.
- **Инвариант границы**: публичное замыкание (`ladix`, `ir`, `lexer`, `parser`, `ast`, `value`,
  `errors`) НЕ тянет `modernc.org/sqlite` и `internal/{store,engine,daemon}` — закреплён
  тест-стражем `boundary_test.go` через `go list -deps` (FR-009/FR-010).
- **go-директива `1.25.0` → `1.23`** (согласование `go.mod`/README/конституция, FR-011).
- **Первый тег `v0.1.0`**; политика semver + аддитивность языка + bump `ir.SchemaVersion` при
  breaking-изменении формата IR (FR-016).

**Технический подход**: тексты диагностик SPEC §13 переиспользуются **дословно** в
`ir.Diagnostic.Message`; выражения IR `SchemaVersion == 1` — канонические строки (как
`ast.CanonicalTriggerCondition`), структурное представление откладывается на будущий bump. Весь
существующий тестовый корпус (~25000 LOC) сохраняется зелёным; перенос пакетов ведётся **листьями**
(каждый шаг компилируется, тесты зелёные).

## Technical Context

**Language/Version**: Go **1.23** (понижение с `1.25.0`; конституция допускает `1.22+` — Принцип I);
`gofmt`-форматирование обязательно, `go vet ./...` без замечаний.

**Primary Dependencies**: только **stdlib** для нового кода (фасад `ladix` + пакет `ir` +
страж через `os/exec` → `go list -deps`). Прод-зависимость хранилища `modernc.org/sqlite v1.52.0`
**ИЗОЛИРОВАНА** в `internal/store` и **не входит** в публичное замыкание фронтенда.
**0 новых внешних зависимостей** (SC-009/FR-019).

**Storage**: N/A для публичного фронтенда (SQLite — только internal reference-impl;
durable-хранилище не затрагивается этой фичей).

**Testing**: `go test ./...` из **КОРНЯ** репозитория (после переезда `go.mod`). Новые тесты:
golden-тесты `ladix.Compile` (валидный исходник → `*ir.Program` со `SchemaVersion == 1` без
`error`-диагностик; невалидный → `program == nil` + `[]ir.Diagnostic` дословно §13 с позицией),
unit-тесты пакета `ir` (`SchemaVersion == 1`, JSON-теги `snake_case`), `boundary_test.go`
(страж границы через `go list -deps`). ~25000 LOC существующих тестов **сохраняются зелёными**;
тест-хелперы путей `cmd/ladix` (`../../../`) чинятся под новую корневую раскладку (FR-014).

**Target Platform**: один статический бинарник `ladix` (`cmd/ladix`, reference CLI) +
импортируемая библиотека (публичный фронтенд `ladix`/`ir`/…).

**Project Type**: Go-библиотека + reference CLI (single module, раскладка корневой `go.mod` +
публичные фронтенд-пакеты + `internal/*` backend + `cmd/ladix`).

**Performance Goals**: N/A (упаковка/перемещение; ни одного hot-path-кода не добавляется).

**Constraints**: **НЕ менять** семантику/синтаксис языка, числовую модель, wire-форматы и
контракт Store (FR-015); тексты диагностик **дословно** SPEC §13 — переформулирование ЗАПРЕЩЕНО
(Принцип VIII); **НЕ удалять** движок процессов/CLI/SQLite-стор/демон (standalone reference-impl
цел); публичный фронтенд **НЕ зависит** от internal-backend/`sqlite` (страж границы);
module-path **стабилен** (без `/src`).

**Scale/Scope**: перенос **5 пакетов** из `internal/` наружу (`lexer`/`parser`/`ast`/`value`/
`errors`) + **2 новых пакета** (`ladix`, `ir`) + переезд `go.mod` в корень + понижение go-директивы
+ механическая правка импорт-путей у всех импортёров + doc-sync (`ARCHITECTURE`/`README`/`AGENTS`/
`docs`/`SPEC §13`).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.* Проверка ведётся против
**конституции 2.0.0** (Принцип VII переопределён; III — `errors` публичный; IV — `ir.Position`).

| # | Принцип | Вердикт | Обоснование |
|---|---------|---------|-------------|
| I | Язык и сборка | **PASS** | Go **1.23** (в коридоре `1.22+`); `gofmt`/`go vet ./...` чисто (SC-002). **0 новых внешних зависимостей** (фасад + `ir` + страж — только stdlib + `os/exec`→`go list`). `modernc.org/sqlite` **изолирован** от потребителя (усиление принципа — потребитель библиотеки не тянет SQLite). CGO не вводится; один статический бинарник `ladix` сохранён (FR-012). |
| II | Парсинг — ручной | **PASS** | Лексер/парсер **только ПЕРЕНОСЯТСЯ** из `internal/` наружу — логика recursive descent и посимвольный сканер **байт-в-байт целы**; ни генераторов, ни regex не вводится. |
| III | Ошибки — явные типы | **PASS** | Типы ошибок переезжают в **ПУБЛИЧНЫЙ** пакет `errors` (путь `internal/errors` → `errors`, **санкционировано конституцией 2.0.0**); `errors.Is`/`errors.As`, recover-барьер на границе CLI-подкоманд, паника-как-инвариант — целы. `ir.Diagnostic` несёт `Position` (`Line`/`Col`). |
| IV | Позиции — сквозные | **PASS** | `Position` сохраняется (отсчёт **с 1**, в **рунах**, на каждом токене/узле, протаскивается до рантайма). `ir.Position` — **ещё один локальный дубль** (как у `ast`), `ir` не импортирует `errors` и остаётся листовым — **санкционировано 2.0.0**. Тип дублируется, не разделяется. |
| V | Без глобального состояния | **PASS** | Фасад `ladix.Compile` строит `eval.Interpreter` **явно** (`io.Discard` как вывод, дефолтная глубина, системные часы), без пакетных изменяемых `var`; `Store` инжектируется в backend как интерфейс. Страж границы — тест, не глобальное состояние. |
| VI | Тесты — вперёд | **PASS** | Новые тесты (`Compile`, `ir`, граница) пишутся **tests-first** для публичной поверхности; **переносимые** табличные тесты лексера/парсера остаются зелёными **на каждом шаге** переноса (листовой порядок). Негативные кейсы (невалидный исходник → дословные диагностики) — первоклассны. |
| VII | Раскладка проекта | **PASS** | Это **ЦЕЛЕВОЙ принцип фичи** в редакции **2.0.0**: раскладка ровно соответствует ей — публичный фронтенд (`ladix`/`ir`/`lexer`/`parser`/`ast`/`value`/`errors`) **ВНЕ** `internal/`, backend (`eval`/`engine`/`store`/`daemon`/`jsonval`) **ПОД** `internal/`, листовые `value`/`errors`/`ast`/`ir`, граф **ацикличен**, `go.mod` в корне, инвариант изоляции `sqlite` закреплён стражем (FR-009/FR-010). |
| VIII | Язык сообщений | **PASS** | Тексты диагностик — русские, **дословно** SPEC §13; `ir.Diagnostic.Message` **не переформулируется** (FR-007), берётся как есть из существующего слоя ошибок. Двухстрочный канон §13 при печати CLI не затрагивается. |
| IX | Спека — источник истины | **PASS** | Doc-sync источников истины (`ARCHITECTURE`/`README`/`AGENTS`+`src/AGENTS`/`docs/*-model`/`SPEC §13`) — **в скоупе** плана. Находка «контракта LADIX в репозитории Уклада НЕТ» зафиксирована как **явное допущение** в spec (Assumptions) — не молчим (Принцип IX). Конституция приведена в соответствие (2.0.0) **ДО** плана как предусловие. |

**Итог: 9/9 PASS. Complexity Tracking ПУСТ.** Конфликт «весь код под `internal/` ↔ нужен
публичный импорт» разрешён **ЗАКОННО** — поправкой конституции до **2.0.0** (путь A,
переопределение Принципа VII), а **не** через санкционированную девиацию. Ни одной записи в
Complexity Tracking не требуется.

## Project Structure

### Documentation (this feature)

```text
specs/029-consumable-go-module/
├── plan.md              # Этот файл (/speckit-plan)
├── research.md          # Phase 0: путь A vs B, листовой порядок переноса, рецепт стража, понижение go
├── data-model.md        # Phase 1: типы ir (Program/Metric/Process/Trigger/Diagnostic/Position) + контракт фасада
├── quickstart.md        # Phase 1: go get + пример ladix.Compile + прогон тестов/стража из корня
├── contracts/           # Phase 1: контракт фасада Compile/CompileFile + схема ir v1 + контракт стража границы
└── tasks.md             # Phase 2 (/speckit-tasks — НЕ создаётся этим планом)
```

### Source Code (repository root)

**ДО** (текущее состояние — `ladix/` НЕ является Go-модулем; модуль спрятан в `src/`):

```text
ladix/                         # корень репозитория — НЕ модуль (go.mod отсутствует)
├── src/                       # Go-модуль здесь
│   ├── go.mod                 # module github.com/denis-kosyakov/ladix; go 1.25.0
│   ├── go.sum
│   ├── cmd/
│   │   └── ladix/             # CLI (reference-implementation)
│   └── internal/
│       ├── ast/               # AST + локальная Position (лист)
│       ├── errors/            # типы ошибок + Position (лист) — ВНУТРЕННИЙ
│       ├── value/             # значения (лист)
│       ├── lexer/             # ручной сканер
│       ├── parser/            # recursive descent
│       ├── eval/              # интерпретатор + семантика + движок метрик/окон
│       ├── engine/            # движок процессов
│       ├── store/             # SQLite-стор (modernc.org/sqlite)
│       ├── daemon/            # демон serve/триггеры
│       └── jsonval/           # декодер payload
├── examples/                  # программы .ladix + data/ + MANIFEST.md
├── docs/                      # *-model.md, grammar, stdlib, quickstart
├── specs/                     # SpecKit-артефакты (001–029)
├── README.md  SPEC.md  ARCHITECTURE.md  AGENTS.md
└── .specify/                  # конституция, шаблоны, расширения
```

**ПОСЛЕ** (корень репозитория == Go-модуль; публичный фронтенд снаружи, backend под `internal/`):

```text
ladix/                         # корень репозитория == Go-модуль
├── go.mod                     # module github.com/denis-kosyakov/ladix (БЕЗ /src); go 1.23
├── go.sum
├── ladix.go                   # пакет ladix: фасад Compile / CompileFile (узкая точка входа) — ПУБЛИЧНЫЙ
├── boundary_test.go           # страж границы: go list -deps доказывает изоляцию sqlite/internal-backend
├── ir/                        # ПУБЛИЧНЫЙ: Program(schema_version)+Metric/Process/Trigger, Diagnostic, Position (лист)
├── lexer/                     # ПУБЛИЧНЫЙ (перенесён из internal/) — ручной сканер
├── parser/                    # ПУБЛИЧНЫЙ (перенесён из internal/) — recursive descent
├── ast/                       # ПУБЛИЧНЫЙ (перенесён из internal/) — AST + локальная Position (лист)
├── value/                     # ПУБЛИЧНЫЙ (перенесён из internal/) — значения (лист)
├── errors/                    # ПУБЛИЧНЫЙ (перенесён из internal/) — типы ошибок + Position (лист)
├── cmd/
│   └── ladix/                 # CLI (reference-implementation) — тест-хелперы путей починены (FR-014)
├── internal/                  # ВНУТРЕННИЙ backend (reference-impl)
│   ├── eval/                  # интерпретатор + семантика + движок метрик/окон (НЕ раскалывается)
│   ├── engine/                # движок процессов
│   ├── store/                 # SQLite-стор (modernc.org/sqlite ИЗОЛИРОВАН здесь)
│   ├── daemon/                # демон serve/триггеры
│   └── jsonval/               # декодер payload
├── examples/                  # .ladix + data/ (file-relative, фича 026 — безопасны)
├── docs/                      # doc-sync: раскладка/команды/IR-контракт
├── specs/                     # SpecKit-артефакты (001–029)
├── README.md  SPEC.md  ARCHITECTURE.md  AGENTS.md
└── .specify/                  # конституция 2.0.0, шаблоны
```

**Structure Decision**: **Один Go-модуль в корне репозитория** (`go.mod` в корне, module-path
без `/src` → переезд не ломает внутренние импорты). Введена **явная граница «публичный фронтенд
vs internal backend»** (Конституция VII в редакции 2.0.0): публичные `ladix`/`ir`/`lexer`/`parser`/
`ast`/`value`/`errors` снаружи, backend `eval`/`engine`/`store`/`daemon`/`jsonval` под `internal/`,
граф ацикличен с листьями `value`/`errors`/`ast`/`ir`. **Порядок переноса — листьями**
(`value` → `errors` → `ast` → `lexer` → `parser`), каждый шаг компилируется и его тесты зелёные.
**`eval` НЕ раскалывается**: наружу выносится IR-контракт (`ir`), исполнитель остаётся internal.
Изоляция `modernc.org/sqlite` от публичного замыкания закреплена тест-стражем `boundary_test.go`.

## Complexity Tracking

> Нарушений Конституции нет — конфликт раскладки разрешён поправкой конституции до **2.0.0**
> (путь A, переопределение Принципа VII), а не девиацией; таблица намеренно пуста.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| _(нет)_ | _(нет)_ | _(нет)_ |
