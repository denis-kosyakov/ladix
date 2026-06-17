# Implementation Plan: B3 — payload задачи через `complete --данные`

**Branch**: `015-complete-payload` | **Date**: 2026-06-17 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/015-complete-payload/spec.md`; якорь
`docs/automation-model.md` §AU-5 / §AU-1 D-AU-3 / §AU-10.C.

## Summary

Оператор передаёт payload при завершении задачи: `ladix complete <файл> <id>
--данные '<json-объект>'`. JSON декодируется на CLI через **существующий**
`jsonval.PayloadToRecord` в read-only эфемерную `value.Запись` и инжектится под
именем `данные` в среду **первого шага догона** этого `complete`. Без флага → пустая
`Запись` (не ошибка). Эфемерность (D-AU-3): payload живёт только сквозь догон одного
complete, не персистится. Технически: расширить 4 функции движка параметром `data
value.Запись` (внутренний API, НЕ шов `ProcessRuntime`) и инжектить в per-step
`stepEnv` (а после первой итерации — пустую `Запись`).

## Technical Context

**Language/Version**: Go 1.22+ (модуль `github.com/denis-kosyakov/ladix`).

**Primary Dependencies**: stdlib + `modernc.org/sqlite` (без CGO). **0 новых
зависимостей.** Используется существующий `internal/jsonval` (`encoding/json` под
капотом, уже в дереве с B2/014).

**Storage**: SQLite через `internal/store` — **дифф ПУСТ** (нет новых полей/колонок;
payload НЕ персистится, D-AU-3).

**Testing**: `go test ./...` (table-driven, tests-first); golden run/complete; race.
Детерминизм — `FixedClock` в тестах.

**Target Platform**: CLI-бинарник `ladix` (Linux/macOS/Windows).

**Project Type**: Single project / интерпретатор + движок процессов (компилятор-CLI).

**Performance Goals**: N/A (детерминизм важнее throughput; payload-decode — однократный).

**Constraints**: `internal/eval` НЕ импортирует `store`/`engine`/`jsonval`; `данные`
входит как `value.Запись`. `ProcessRuntime` = 8 методов. Read-only барьер `данные`.

**Scale/Scope**: 3 слоя — CLI (`cmd/ladix/main.go`), engine (4 функции
`internal/engine/engine.go`), eval-инжект (точка `Define` в `advance`). ~6 правок,
без новых пакетов.

## Constitution Check

*GATE: проверено до Phase 0 и повторно после Phase 1. Итог — 9/9 PASS.*

| # | Принцип | Статус | Обоснование |
|---|---------|--------|-------------|
| I | Язык и сборка | PASS | Go 1.22+, gofmt/vet чисто, CGO нет, 0 новых зависимостей (jsonval уже в дереве). |
| II | Парсинг — ручной | PASS | Грамматика языка НЕ меняется; фронтенд (lexer/parser) не тронут. JSON-payload парсит stdlib `encoding/json` внутри jsonval — это НЕ парсер языка Ladix. |
| III | Ошибки — явные типы | PASS | Новая CLI-ошибка `ladix: неверный JSON в --данные: …` идёт штатным CLI-путём (stderr, exit 2) под guard/recover-барьером `completeTask`; ошибка декода — обычная error от `jsonval.PayloadToRecord`, без паники. |
| IV | Позиции — сквозные | PASS | Новых пользовательских позиционных диагностик языка нет; CLI-ошибка payload — не позиционная (вход — argv, не исходник). Существующие позиции не затрагиваются. |
| V | Без глобального состояния | PASS | `data` протягивается параметром через 4 функции (явная передача); никакого package-level состояния. Эфемерность исключает скрытое состояние. |
| VI | Тесты — вперёд | PASS | tasks-first: замки US1/US2/US3 (a-e) пишутся до правок; мутпробы инвертируют каждый. |
| VII | Раскладка проекта | PASS | Без новых пакетов. Граф зависимостей без циклов: `cmd/ladix`→`jsonval` (корень композиции, допустимо); `eval` НЕ получает `jsonval`/`store`/`engine`. Листовость `value` сохранена. |
| VIII | Язык сообщений | PASS | Текст ошибки дословно §AU-10.C: `ladix: неверный JSON в --данные: <деталь>`. Имя `данные` — русское. |
| IX | Спека — источник истины | PASS | Поведение залочено §AU-5/D-AU-3; точные имена/строки 4 функций и точка `Define` цитированы из §AU-5.3. Пробелов нет — решения locked владельцем 2026-06-16. |

**Complexity Tracking**: нарушений нет → таблица пуста (раздел ниже не заполняется).

## Project Structure

### Documentation (this feature)

```text
specs/015-complete-payload/
├── plan.md              # Этот файл
├── research.md          # Phase 0: прецеденты Complete/catchUp/advance, эмпирика строк
├── data-model.md        # Phase 1: сигнатуры 4 функций, точка инжекта, эфемерность
├── quickstart.md        # Phase 1: ручной прогон complete --данные
├── contracts/
│   ├── engine-payload-threading.md   # Контракт протяжки data через 4 функции + точка Define
│   ├── cli-complete-danные.md        # Контракт CLI-флага --данные + ошибка
│   └── jsonval-consumption.md        # Контракт ПОТРЕБЛЕНИЯ jsonval.PayloadToRecord
└── tasks.md             # Phase 2 (/speckit-tasks — отдельно)
```

### Source Code (repository root)

```text
src/
├── cmd/ladix/
│   └── main.go              # completeMain: парс --данные (зеркало --вебхук);
│                            # completeTask: jsonval.PayloadToRecord → data; eng.Complete(taskID, data)
├── internal/
│   ├── engine/
│   │   └── engine.go        # 4 функции +param data value.Запись:
│   │                        #   Complete(taskID, data) :108
│   │                        #   catchUp(inst, data, t) :177
│   │                        #   advanceAfterComplete(inst, data, caughtUp) :189
│   │                        #   advance(inst, data) :242 — точка Define: stepEnv.Define("данные", cur) :~262
│   ├── jsonval/
│   │   └── decode.go        # PayloadToRecord :31 — ПОТРЕБЛЯЕТСЯ как есть (НЕ менять)
│   └── eval/                # БЕЗ изменений (импорт-граф не трогаем; данные приходит как value.Запись)
```

**Structure Decision**: Single project, существующая Go-раскладка `cmd/ladix` +
`internal/*`. B3 правит ровно 2 файла (`main.go`, `engine.go`) + тесты; пакеты не
добавляются. `jsonval` и `eval` — только читаются/потребляются.

## Phase 0 — Research (research.md)

Прецеденты: путь `Complete`/`catchUp`/`advance` (006/007b), точка `stepEnv :=
eval.NewEnvironment(processEnv)` и `ExecStepBody`; read-only env-барьер тела
триггера (007a/B4). Эмпирика строк на `aebac92`. Подтверждение, что
`jsonval.PayloadToRecord` экспортирован и отвергает не-объект.

## Phase 1 — Design (data-model.md, contracts/, quickstart.md)

Сигнатуры 4 функций ДО/ПОСЛЕ; механизм `cur := data` / `cur = value.NewRecord(nil,nil)`
после первой итерации; точка `Define` именно в `stepEnv` (не `processEnv`); контракт
CLI-флага и ошибки; контракт потребления jsonval (без дубля).

## Complexity Tracking

> Нарушений Constitution нет — таблица пуста.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
