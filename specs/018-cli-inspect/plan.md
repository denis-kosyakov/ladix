# Implementation Plan: B6 — `ladix inspect <id>` (снимок инстанса + лёгкая история задач)

**Branch**: `018-cli-inspect` | **Date**: 2026-06-17 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/018-cli-inspect/spec.md`

## Summary

Добавить read-only CLI-подкоманду `ladix inspect <id> [--db <path>]`, печатающую человекочитаемый
снимок инстанса (имя процесса, статус, текущий шаг, переменные) + лёгкую историю его задач (открытые +
завершённые) в порядке ID. Подкоманда `inspectMain` (в `cmd/ladix`) читает Store НАПРЯМУЮ
(`LoadInstance` + НОВЫЙ `ListTasksByInstance`), БЕЗ движка/eval — зеркало `tasksMain`/`listTasks`.
Контракт `Store` получает РОВНО один новый read-only метод
`ListTasksByInstance(instanceID string) ([]*Task, error)` (порядок ID ASC), реализованный в ОБОИХ
бэкендах (`MemoryStore`, `SQLiteStore`); число методов 15→16 аддитивно (§AU-2). Дефолт хранилища —
SQLite `ladix.db` через хелпер `openStore` (B5). Все stdout/stderr-тексты — дословно из §AU-10.D/§AU-10.C
(exact-match golden; маска `<время>`). Слой — CLI + 1 метод Store; новой языковой функциональности нет.

## Technical Context

**Language/Version**: Go (модуль `github.com/denis-kosyakov/ladix`, `src/go.mod`; toolchain как в репо).

**Primary Dependencies**: только stdlib + уже подключённый `modernc.org/sqlite` (SQLiteStore). **0 новых
зависимостей** (Принцип I, SC-007).

**Storage**: SQLite (`ladix.db` по умолчанию) через `openStore`; MemoryStore — для тестов/паритета
контракта. Метод `ListTasksByInstance` — read-only `SELECT … WHERE instance_id = ? ORDER BY id ASC`.

**Testing**: `go test ./...` из `src/`; golden exact-match (stdout/stderr подкоманды) в
`cmd/ladix/*_golden_test.go`; контракт-тесты Store в `internal/store/*_test.go` (оба бэкенда). Tests-first
(Принцип VI распространяется на новый поведенческий слой по практике 002–017).

**Target Platform**: CLI-бинарь `ladix` (как существующие подкоманды).

**Project Type**: компилятор/интерпретатор + CLI (single project, `src/` 11 пакетов).

**Performance Goals**: N/A (single-instance read; масштаб демо). Детерминизм важнее скорости (golden).

**Constraints**: exact-match тексты дословно §AU-10; аддитивность Store (15 сигнатур не трогать);
ProcessRuntime=8 не трогать; eval без store/engine; пустой дифф eval/engine; read-only inspect;
детерминизм id/времени (golden маскирует `<время>`).

**Scale/Scope**: ~1 новая CLI-функция `inspectMain` + helper печати; +1 метод Store × 2 бэкенда;
golden + контракт-тесты. Без изменений грамматики/лексера/парсера/eval/engine.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| # | Принцип | Статус | Обоснование |
|---|---------|--------|-------------|
| I | Язык и сборка (Go, без новых зависимостей) | ✅ PASS | Только Go stdlib + уже подключённый sqlite; 0 новых зависимостей. |
| II | Парсинг — ручной | ✅ PASS | Грамматика/парсер не трогаются; фича — CLI + Store-метод. |
| III | Ошибки — явные типы | ✅ PASS | `LoadInstance` уже возвращает `ErrInstanceNotFound` (сентинел); CLI транслирует в §AU-10.C. Новый метод — `([]*Task, error)`. |
| IV | Позиции — сквозные | ✅ PASS | Диагностик парсера не добавляется; CLI-ошибки позиций не несут (паритет существующих). |
| V | Без глобального состояния | ✅ PASS | `inspectMain` берёт Store через `openStore`, без глобалов; read-only, без мутаций. |
| VI | Тесты — вперёд | ✅ PASS | tasks.md tests-first: golden inspect + контракт `ListTasksByInstance` (оба бэкенда) пишутся до impl (красные). |
| VII | Раскладка проекта | ✅ PASS | Код только в `src/cmd/ladix` (+`internal/store`). Без новых пакетов. |
| VIII | Язык сообщений (русский, дословно из якоря) | ✅ PASS | Все тексты дословно из §AU-10.D/§AU-10.C (Принцип VIII канон). |
| IX | Спека — источник истины | ✅ PASS | spec.md + §AU-8/§AU-2/§AU-9/§AU-10 — единый источник; contracts фиксируют дословный канон. |

**Итог: 9/9 PASS.** Нарушений нет → Complexity Tracking пуст.

## Project Structure

### Documentation (this feature)

```text
specs/018-cli-inspect/
├── plan.md              # этот файл
├── research.md          # Phase 0: прецеденты (LoadInstance/ListPendingTasks/openStore/value.String/FormatTaskLine)
├── data-model.md        # Phase 1: метод ListTasksByInstance (оба бэкенда) + формат вывода inspect
├── quickstart.md        # Phase 1: демо B5→B6 (start → inspect)
├── contracts/
│   ├── store-list-tasks-by-instance.md   # контракт нового Store-метода (оба бэкенда, ID ASC)
│   └── inspect-cli.md                     # контракт CLI inspect: грамматика, stdout канон, CLI-ошибка
└── tasks.md             # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
src/
├── cmd/ladix/
│   ├── main.go            # switch: +case "inspect" → inspectMain; usage-строка +inspect
│   ├── inspect.go         # НОВЫЙ: inspectMain + хелпер печати снимка/задач (read-only)
│   ├── inspect_golden_test.go  # НОВЫЙ: golden stdout (снимок+история, эскалирована, без дедлайна) + неизв.инстанс
│   ├── start.go           # openStore (существ., B5) — переиспользуется inspect; НЕ меняется по сути
│   └── …                  # прочие подкоманды — НЕ трогаются (регресс-проверка)
└── internal/store/
    ├── store.go           # интерфейс Store: +ListTasksByInstance (15→16); 15 старых не трогать
    ├── memory.go          # +MemoryStore.ListTasksByInstance (фильтр InstanceID, sort ID ASC)
    ├── sqlite.go          # +SQLiteStore.ListTasksByInstance (SELECT … WHERE instance_id ORDER BY id ASC, escalated)
    ├── memory_test.go     # +контракт-тест (порядок/фильтр/смешанные статусы/пусто/escalated)
    ├── sqlite_test.go     # +контракт-тест (зеркало memory; escalated сохраняется)
    └── contract_test.go   # счётный замок Store=16 (если присутствует) / table-driven по обоим бэкендам
```

**Structure Decision**: Single project (`src/`, опция 1). Новый поведенческий код — в существующих
пакетах `cmd/ladix` (CLI-подкоманда) и `internal/store` (один read-only метод × два бэкенда). Новых
пакетов/зависимостей нет. Грамматика/лексер/парсер/eval/engine — пустой дифф.

## Complexity Tracking

> Constitution Check: 9/9 PASS, нарушений нет. Таблица пуста.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
