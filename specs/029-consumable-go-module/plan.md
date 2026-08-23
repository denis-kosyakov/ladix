# Implementation Plan: Потребляемый Go-модуль LADIX

**Branch**: `029-consumable-go-module` | **Date**: 2026-06-29 (ревизия под Clarifications: путь A → путь B) | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/029-consumable-go-module/spec.md`

## Summary

Рефактор-упаковка LADIX в **потребляемый Go-модуль** `github.com/denis-kosyakov/ladix`
**без изменения семантики/синтаксиса языка** — только перемещение, экспорт и упаковка. По
Clarifications (сессия 2026-06-29) выбран **путь B (минимальная публичная поверхность)** +
поправка конституции до **1.1.0** (MINOR, аддитивная): из `internal/` НЕ выносится НИЧЕГО,
публичными становятся РОВНО два НОВЫХ пакета — корневой фасад `ladix` и версионируемый `ir`.

Конкретика переупаковки:

- **`go.mod` переезжает из `src/` в корень репозитория** (схлопывание `src/` → корень).
  Module-path **неизменен** (`github.com/denis-kosyakov/ladix`, без сегмента `/src`), поэтому
  переезд внутренние импорт-пути НЕ ломает — **массовой правки импортов НЕТ** (FR-001/FR-002/FR-003).
- **Добавляются два новых публичных пакета**: `ir/` (типы IR со `SchemaVersion == 1`) и корневой
  `ladix` (узкая точка входа `Compile`/`CompileFile`). Существующие пакеты не двигаются (FR-003).
- **Всё остальное остаётся под `internal/`**: фронтенд `lexer`, `parser`, `ast`, `value`, `errors`
  и backend `eval`, `engine`, `store`, `daemon`, `jsonval`; CLI — `cmd/ladix` (FR-004/FR-012).
- **Инвариант границы**: публичное замыкание (`ladix`, `ir`) НЕ тянет `modernc.org/sqlite` и
  `internal/{store,engine,daemon}`; `internal/eval` ЯВНО РАЗРЕШЁН (фасад зовёт его `Analyze`) —
  закреплён тест-стражем `boundary_test.go` через `go list -deps` (FR-009/FR-010).
- **go-директива ОСТАЁТСЯ `1.25.0`** — FR-011 (`→ 1.23`) на стадии имплементации оказался
  **невыполним**: `modernc.org/sqlite v1.52.0` требует `go 1.25.0`, а go-директива у модуля одна
  на всех. Решение владельца (2026-08-23): не понижать sqlite. Запись в Complexity Tracking.
- **Первый тег `v0.1.0`**; политика semver + аддитивность языка + bump `ir.SchemaVersion` при
  breaking-изменении формата IR (FR-016); JSON round-trip `ir` — golden-locked контракт v1 (FR-021).

**Технический подход**: тексты диагностик SPEC §13 переиспользуются **дословно** в
`ir.Diagnostic.Message`; выражения IR `SchemaVersion == 1` — канонические строки (как
`ast.CanonicalTriggerCondition`), структурное представление откладывается на будущий bump (v2).
Весь существующий тестовый корпус (~25000 LOC) сохраняется зелёным; риск рефактора минимален —
единственное физическое перемещение это `src/*` → корень при стабильном module-path.

## Technical Context

**Language/Version**: Go **1.25.0** (понижение до `1.23` отменено, см. Complexity Tracking;
конституция допускает `1.22+` — Принцип I, порог сверху не ограничен);
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
импортируемая библиотека (публичная поверхность — РОВНО `ladix` + `ir`).

**Project Type**: Go-библиотека + reference CLI (single module, раскладка корневой `go.mod` +
публичные `ladix`/`ir` + `internal/*` (фронтенд и backend) + `cmd/ladix`).

**Performance Goals**: N/A (упаковка/перемещение; ни одного hot-path-кода не добавляется).

**Constraints**: **НЕ менять** семантику/синтаксис языка, числовую модель, wire-форматы и
контракт Store (FR-015); тексты диагностик **дословно** SPEC §13 — переформулирование ЗАПРЕЩЕНО
(Принцип VIII); **НЕ удалять** движок процессов/CLI/SQLite-стор/демон (standalone reference-impl
цел); публичное замыкание `ladix`+`ir` **НЕ зависит** от `internal/{store,engine,daemon}`/`sqlite`
(страж границы; `internal/eval` разрешён);
module-path **стабилен** (без `/src`).

**Scale/Scope**: **0 перенесённых пакетов** (по FR-003 из `internal/` не выносится ничего) +
**2 новых публичных пакета** (`ladix`, `ir`) + схлопывание `src/` → корень с переездом `go.mod` +
починка тест-хелперов путей `cmd/ladix` (FR-014) + doc-sync
(`ARCHITECTURE`/`README`/`AGENTS`/`docs`).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.* Проверка ведётся против
**конституции 1.1.0** (Принцип VII ДОПОЛНЕН разрешением двух публичных пакетов + инвариантом
изоляции; Принцип IV ДОПОЛНЕН `ir.Position`; Принцип III НЕ меняется — `internal/errors`).

| # | Принцип | Вердикт | Обоснование |
|---|---------|---------|-------------|
| I | Язык и сборка | **PASS** | Go **1.25.0** (коридор `1.22+` открыт сверху); `gofmt`/`go vet ./...` чисто (SC-002). **0 новых внешних зависимостей** (фасад + `ir` + страж — только stdlib + `os/exec`→`go list`). `modernc.org/sqlite` **изолирован** от потребителя. CGO не вводится; один статический бинарник `ladix` сохранён (FR-012). |
| II | Парсинг — ручной | **PASS** | Лексер/парсер **не трогаются вовсе** (остаются в `internal/`, байт-в-байт целы); ни генераторов, ни regex не вводится. |
| III | Ошибки — явные типы | **PASS** | Пакет ошибок **ОСТАЁТСЯ** `internal/errors` — принцип не затронут. `errors.Is`/`errors.As`, recover-барьер на границе CLI-подкоманд, паника-как-инвариант — целы. `ir.Diagnostic` несёт `Position` (`Line`/`Col`). |
| IV | Позиции — сквозные | **PASS** | `Position` сохраняется (отсчёт **с 1**, в **рунах**, на каждом токене/узле, протаскивается до рантайма). `ir.Position` — **ещё один локальный дубль** (как у `ast`), `ir` не импортирует `errors` и остаётся листовым — **санкционировано 1.1.0**. Тип дублируется, не разделяется. |
| V | Без глобального состояния | **PASS** | Фасад `ladix.Compile` строит `eval.Interpreter` **явно** (`io.Discard` как вывод, дефолтная глубина, системные часы), без пакетных изменяемых `var`. Страж границы — тест, не глобальное состояние. |
| VI | Тесты — вперёд | **PASS** | Новые тесты (`Compile`, `ir`, JSON round-trip, граница) пишутся **tests-first** для публичной поверхности. Существующий корпус лексера/парсера остаётся зелёным (пакеты не двигаются). Негативные кейсы (невалидный исходник → дословные диагностики) — первоклассны. |
| VII | Раскладка проекта | **PASS** | Это **ЦЕЛЕВОЙ принцип фичи** в редакции **1.1.0**: `go.mod` в корне; публичны РОВНО `ladix`+`ir`; всё прочее (`lexer`/`parser`/`ast`/`value`/`errors`/`eval`/`engine`/`store`/`daemon`/`jsonval`) **ПОД** `internal/`; листовые `value`/`errors`/`ast`/`ir`; граф **ацикличен**; инвариант изоляции `sqlite` закреплён стражем (FR-009/FR-010). |
| VIII | Язык сообщений | **PASS** | Тексты диагностик — русские, **дословно** SPEC §13; `ir.Diagnostic.Message` **не переформулируется** (FR-007), берётся как есть из существующего слоя ошибок. Двухстрочный канон §13 при печати CLI не затрагивается. |
| IX | Спека — источник истины | **PASS** | Doc-sync источников истины (`ARCHITECTURE`/`README`/`AGENTS`+`src/AGENTS`/`docs/*-model`) — **в скоупе** плана. Расхождение «план 2026-06-28 (путь A) ↔ Clarifications 2026-06-29 (путь B)» разрешено **в пользу спеки** — план приведён к пути B, конституция понижена `2.0.0` → `1.1.0` (FR-017). Находка «контракта LADIX в репозитории Уклада НЕТ» зафиксирована как **явное допущение** в spec (Assumptions). |

**Итог: 9/9 PASS. Complexity Tracking ПУСТ.** Конфликт «весь код под `internal/` ↔ нужен
публичный импорт» разрешён **ЗАКОННО** — аддитивной поправкой конституции до **1.1.0** (путь B:
наружу выходят только два НОВЫХ пакета, ничего не переопределяется), а не через санкционированную
девиацию. Ни одной записи в Complexity Tracking не требуется.

## Project Structure

### Documentation (this feature)

```text
specs/029-consumable-go-module/
├── plan.md              # Этот файл (/speckit-plan)
├── research.md          # Phase 0: путь A vs B (выбран B), рецепт стража, понижение go-директивы
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

**ПОСЛЕ** (корень репозитория == Go-модуль; публичны РОВНО `ladix`+`ir`, всё прочее — `internal/`):

```text
ladix/                         # корень репозитория == Go-модуль
├── go.mod                     # module github.com/denis-kosyakov/ladix (БЕЗ /src); go 1.25.0
├── go.sum
├── ladix.go                   # пакет ladix: фасад Compile / CompileFile (узкая точка входа) — ПУБЛИЧНЫЙ
├── ladix_test.go              # golden-тесты фасада (валидный/невалидный исходник, CompileFile)
├── boundary_test.go           # страж границы: go list -deps доказывает изоляцию sqlite/internal-backend
├── ir/                        # ПУБЛИЧНЫЙ: SchemaVersion, Program/Metric/Process/Trigger, Diagnostic, Position (лист)
├── cmd/
│   └── ladix/                 # CLI (reference-implementation) — тест-хелперы путей починены (FR-014)
├── internal/                  # ВНУТРЕННИЙ: фронтенд + backend (НИЧЕГО не выносится наружу)
│   ├── lexer/                 # ручной сканер
│   ├── parser/                # recursive descent
│   ├── ast/                   # AST + локальная Position (лист)
│   ├── value/                 # значения (лист)
│   ├── errors/                # типы ошибок + Position (лист)
│   ├── eval/                  # интерпретатор + семантика + движок метрик/окон (разрешён фасаду)
│   ├── engine/                # движок процессов (ЗАПРЕЩЁН публичному замыканию)
│   ├── store/                 # SQLite-стор (modernc.org/sqlite ИЗОЛИРОВАН здесь; ЗАПРЕЩЁН)
│   ├── daemon/                # демон serve/триггеры (ЗАПРЕЩЁН)
│   └── jsonval/               # декодер payload
├── examples/                  # .ladix + data/ (file-relative, фича 026 — безопасны)
├── docs/                      # doc-sync: раскладка/команды/IR-контракт
├── specs/                     # SpecKit-артефакты (001–029)
├── README.md  SPEC.md  ARCHITECTURE.md  AGENTS.md
└── .specify/                  # конституция 1.1.0, шаблоны
```

**Structure Decision**: **Один Go-модуль в корне репозитория** (`go.mod` в корне, module-path
без `/src` → схлопывание `src/` → корень импорт-стабильно, массовой правки импортов НЕТ).
Введена **минимальная публичная поверхность** (Конституция VII ред. 1.1.0): наружу выходят
РОВНО два НОВЫХ пакета — `ladix` (узкий фасад) и `ir` (версионируемый контракт вывода); весь
существующий код — фронтенд `lexer`/`parser`/`ast`/`value`/`errors` и backend `eval`/`engine`/
`store`/`daemon`/`jsonval` — **ОСТАЁТСЯ под `internal/`**. Весь semver-риск стянут в `ir`, потому
его JSON-форма защищена golden-замком (FR-021). Изоляция `modernc.org/sqlite` и
`internal/{store,engine,daemon}` от публичного замыкания закреплена тест-стражем `boundary_test.go`
(`internal/eval` — явное исключение, фасаду нужен его `Analyze`).

## Complexity Tracking

> Нарушений **Конституции** нет — конфликт раскладки разрешён аддитивной поправкой конституции до
> **1.1.0** (путь B: наружу выходят только два НОВЫХ пакета, Принцип VII не переопределяется),
> а не девиацией. Единственная запись ниже — отклонение от **спеки** (FR-011), а не от конституции.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| **FR-011 не выполнен**: go-директива остаётся `1.25.0` вместо `1.23` (A7 не достигнут) | `modernc.org/sqlite v1.52.0` объявляет `go 1.25.0`; go-директива в модуле **одна на весь модуль**, поэтому `go 1.23` физически невозможен при этой версии зависимости (`go mod tidy` откатывает директиву обратно). Обнаружено на стадии имплементации, решение владельца 2026-08-23. Практического барьера для потребителя нет: Go 1.21+ авто-докачивает тулчейн (кроме `GOTOOLCHAIN=local`), а публичное замыкание `ladix`+`ir` — stdlib-only и `sqlite` не тянет | **Понижение `modernc.org/sqlite` до `v1.38.0`** (последняя с `go 1.23.0`) — откат зависимости durable-стора на 14 минорных версий ради директивы; цена (потерянные багфиксы/CVE, риск регресса SQLite-стора) не оправдана выигрышем. **Выделение `internal/store` в отдельный модуль** — отвергнуто в research §3 (один модуль) и по объёму = отдельная фича |
