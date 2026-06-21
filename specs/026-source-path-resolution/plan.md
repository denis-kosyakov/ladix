# Implementation Plan: Разрешение путей источников относительно каталога программы

**Branch**: `026-source-path-resolution` | **Date**: 2026-06-21 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/026-source-path-resolution/spec.md`

## Summary

Сменить семантику разрешения относительных путей к файлам-источникам с **cwd-relative** на
**file-relative**: относительный путь источника (`источник продажи файл: "data/sales.json"`)
разрешается относительно каталога `.ladix`-программы, а не текущего рабочего каталога процесса.
Добавить CLI-флаг `--source-base <dir>` (приоритет над каталогом программы) во все 5 подкоманд,
грузящих источники. Абсолютный путь источника — как есть. Каталог `data/` переезжает в
`examples/data/` для самодостаточности витрины.

**Технический подход**: базовый каталог источников хранится как поле `sourceBase string` на
`eval.Interpreter`, инжектируется сеттером `SetSourceBase` (зеркало существующего
`SetProcessRuntime`). Сигнатура `NewInterpreter` не меняется → 37 call-sites остаются целы.
Резолвер `resolveSourcePath` (абсолютный — как есть; иначе `filepath.Join(base, rel)`) применяется
в трёх загрузчиках (JSON/CSV/NDJSON). Каждая CLI-подкоманда вычисляет базу
(`--source-base` ИЛИ `filepath.Dir(programPath)`) и зовёт `interp.SetSourceBase(base)` сразу после
конструктора. Breaking change дефолта отражается в канонах (`docs/source-metric-model.md §SM-8.1`,
`SPEC.md §9.1`, `README.md`, `examples/MANIFEST.md`, quickstart 004).

## Technical Context

**Language/Version**: Go 1.25.0 (модуль `github.com/denis-kosyakov/ladix`, go.mod в `src/`).

**Primary Dependencies**: stdlib-only. Единственная внешняя зависимость — `modernc.org/sqlite`
(не затрагивается). Новые: добавляется stdlib-импорт `path/filepath` (уже косвенно используется в
тестах). **0 новых внешних зависимостей.**

**Storage**: SQLite (`internal/store`) — **не затрагивается**; контракт Store (18 методов) цел.

**Testing**: `go test ./...` из `src/`. Table-driven юнит-тесты (Принцип VI), golden-тесты CLI
(детерминизм под FixedClock), интеграционный smoke (запуск из `t.TempDir()`).

**Target Platform**: один статический бинарник `ladix` (любая ОС, без CGO).

**Project Type**: интерпретатор DSL + CLI (single project, Go-раскладка `cmd/` + `internal/`).

**Performance Goals**: не релевантно (разрешение пути — одноразовая строковая операция на источник).

**Constraints**: детерминированный вывод под фиксированными часами; неизменность наблюдаемых
`stdout`-байтов примеров (SC-003); breaking change дефолта осознан и задокументирован.

**Scale/Scope**: ~2 прод-файла в `eval` (`source_loader.go`, `interpreter.go`), 3 прод-файла в
`cmd/ladix` (`main.go`, `serve.go`, `start.go`), `git mv` 5 файлов данных, ~6 тест-файлов,
~6 доков. Диффа в `internal/{store,engine,daemon}` — НЕТ.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| # | Принцип | Вердикт | Обоснование |
|---|---------|---------|-------------|
| I | Язык и сборка | ✅ PASS | Go 1.25, идиоматично, `go vet` чисто; только stdlib `path/filepath`; 0 новых внешних зависимостей; CGO не вводится. |
| II | Парсинг — ручной | ✅ PASS | Лексер/парсер не затронуты. CLI-флаг `--source-base` парсится тем же ручным switch-паттерном, что `--db`/`--interval` (без regex, без генераторов). |
| III | Ошибки — явные типы | ✅ PASS | Категория/код ошибки «файл не найден» (`runtimeErr` → `ОшибкаВыполнения`) не меняются; меняется только подставляемое **значение** пути. Recover-барьеры подкоманд целы. Новая CLI-usage-ошибка `--source-base` — тот же класс exit-2, что существующие флаг-ошибки. |
| IV | Позиции — сквозные | ✅ PASS | Позиция ошибки источника (`decl.Pos()`) сохраняется без изменений. |
| V | Без глобального состояния | ✅ PASS | `sourceBase` — поле значения-`Interpreter`, инжектируется сеттером (DI, зеркало `SetProcessRuntime`); пакетных мутабельных глобалов нет. |
| VI | Тесты — вперёд | ✅ PASS | Table-driven юнит на резолвер + CLI флаг-тесты, включая негативы (пропуск значения → exit 2). Каждый замок RED при инверсии фикса (мутпробы зафиксированы в research.md). |
| VII | Раскладка проекта | ✅ PASS | Раскладка не меняется; граф зависимостей ацикличен (`filepath` — листовой stdlib). `eval` уже импортирует `os`; добавление `path/filepath` листовость не нарушает. |
| VIII | Язык сообщений | ✅ PASS | Новое сообщение `ladix: флаг --source-base требует значение` — на русском, дословное зеркало существующих CLI-флаг-ошибок (`ladix: флаг --db требует значение`). Канон диагностики §13 не затрагивается (это CLI-usage, не позиционная диагностика). |
| IX | Спека — источник истины | ✅ PASS | Решения зафиксированы заказчиком; канон новой семантики — `docs/source-metric-model.md §SM-8.1`; doc-sync согласует SPEC/README/MANIFEST/quickstart. Недокументированных догадок нет. |

**Итог: 9/9 PASS.** Записей в Complexity Tracking не требуется.

## Project Structure

### Documentation (this feature)

```text
specs/026-source-path-resolution/
├── spec.md              # Спецификация (speckit specify)
├── plan.md              # Этот файл (speckit plan)
├── research.md          # Phase 0: решения + мутпробы
├── data-model.md        # Phase 1: сущность «базовый каталог источников»
├── quickstart.md        # Phase 1: проверка вручную
├── contracts/
│   └── cli-source-base.md   # Контракт CLI-флага --source-base + резолвер
├── checklists/
│   └── requirements.md  # Чек-лист качества спеки (9/9)
└── tasks.md             # Phase 2 (speckit tasks — отдельная команда)
```

### Source Code (repository root)

```text
src/
├── cmd/ladix/
│   ├── main.go          # run/metric/complete: +флаг --source-base, +filepath, +SetSourceBase
│   ├── serve.go         # serve: +флаг --source-base, +filepath, проброс base в buildServeDaemon
│   ├── start.go         # start: +флаг --source-base, +filepath, +SetSourceBase
│   └── *_test.go        # удалить/переработать withRepoRoot; абсолютные пути примеров; +флаг-тесты; +smoke t.TempDir()
└── internal/eval/
    ├── interpreter.go   # +поле sourceBase, +сеттер SetSourceBase (NewInterpreter цел)
    ├── source_loader.go # +import path/filepath, +resolveSourcePath, 3× замена path:=decl.File.Value
    └── source_loader_test.go  # table-driven резолвер; TestLoadSourceSalesJSON под examples/data + явная база

examples/
├── data/                # ← git mv data/ → examples/data/ (sales.json, orders.{csv,json,ndjson}, costs.json)
├── *.ladix              # пути "data/..." НЕ меняются (теперь file-relative)
└── MANIFEST.md          # doc-sync: убрать «из корня репо», отразить examples/data

docs/source-metric-model.md   # §SM-8.1 канон: cwd → каталог .ladix-файла; --source-base; абсолютный
SPEC.md                       # §9.1 «Разрешение пути»
README.md                     # «Примечание о путях к источникам», инструкции запуска
specs/004-source-metric/quickstart.md   # обновить под новую семантику
```

**ПУСТОЙ дифф**: `internal/store`, `internal/engine`, `internal/daemon` (источники резолвятся ниже
точки минта — `serve` перечитывает через тот же `ResetRunState`/`loadSource`, поле базы persist).

**Structure Decision**: single project, Go-раскладка. Изменения сосредоточены в `eval` (механизм
резолва) и `cmd/ladix` (CLI-флаг + вычисление базы), плюс `git mv` витрины данных и doc-sync.

## Complexity Tracking

> Нарушений Constitution Check нет — таблица не заполняется.
