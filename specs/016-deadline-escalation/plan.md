# Implementation Plan: B4 — эскалация дедлайна (B4a фронтенд + B4b бэкенд, durable)

**Branch**: `016-deadline-escalation` | **Date**: 2026-06-17 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/016-deadline-escalation/spec.md`; якорь
`docs/automation-model.md` §AU-6 (B4) / §AU-2 (Task.Escalated + 4 точки кодека) / §AU-10
(диагностики/stdout) / §AU-12 (golden A/B/C) / §AU-1 D-AU-4/D-AU-5/D-AU-6.

## Summary

B4 даёт язык эскалации просроченных задач и его durable-исполнение под `serve`. Две
фаза-группы, реализуемые ДВУМЯ проходами A→B:

- **B4a (фронтенд)**: новый вид триггера `когда задача просрочена в <Процесс>.<Шаг>:`.
  `задача`/`просрочена` остаются IDENT (D-AU-4) — распознаёт ПАРСЕР по контексту после
  `когда`, лексер НЕ трогается. AST `DeadlineTrigger`, `parseDeadlineTrigger` + новый хелпер
  `expectLexeme`, кейс в `checkTrigger`, run-заглушка, расширение текста `msgTriggerKind`
  (SE-TRIGGER-KIND каскад зеркал).
- **B4b (бэкенд, durable)**: поле `Task.Escalated` (+колонка SQLite, 4 точки кодека), 4-я
  фаза `tick()` `checkDeadlines` (скан просроченных), `fireDeadlineBody` с инжектом ВСЕХ
  `InstanceVariables`, одноразовость + переживание рестарта.

## Technical Context

**Language/Version**: Go 1.22+ (модуль `github.com/denis-kosyakov/ladix`).

**Primary Dependencies**: stdlib + `modernc.org/sqlite` (без CGO). **0 новых зависимостей.**

**Storage**: SQLite через `internal/store` — B4b ДОБАВЛЯЕТ колонку `escalated INTEGER NOT
NULL DEFAULT 0` к таблице `tasks` и протягивает её через 4 точки кодека `Task`. Миграция =
сброс схемы (D-AU-9, БД тестовые). Store-контракт 15 методов НЕ растёт (колонка, не метод).

**Testing**: `go test ./...` (table-driven, tests-first); golden run; живой daemon-тест
(tick-фазы); durable Go-API тест (SQLite restart); race. Детерминизм — `FixedClock`.

**Target Platform**: CLI-бинарник `ladix` (Linux/macOS/Windows).

**Project Type**: Single project / интерпретатор + движок процессов + демон.

**Performance Goals**: N/A (детерминизм > throughput; `checkDeadlines` — один листинг до циклов).

**Constraints**: `internal/eval` НЕ импортирует `store`/`engine`; `ProcessRuntime` = 8 методов
(НЕ растёт); `задача`/`просрочена` остаются IDENT (L=11); SE=14, eval=28 целы; новых SE-кодов
нет; read-only барьер тела триггера (TR-BODY-RO) наследуется.

**Scale/Scope**: B4a — 5 файлов (`ast/trigger.go`, `parser/parse_decl.go`, `parser/errors.go`,
`eval/analyze.go`, `eval/trigger_run.go`) + зеркала-замки. B4b — 5 файлов (`store/types.go`,
`store/sqlite.go`, `store/memory.go`, `daemon/tick.go`, новый `daemon/checkdeadlines.go`).

## Constitution Check

*GATE: проверено до Phase 0 и повторно после Phase 1. Итог — 9/9 PASS.*

| # | Принцип | Статус | Обоснование |
|---|---------|--------|-------------|
| I | Язык и сборка | PASS | Go 1.22+, gofmt/vet чисто, CGO нет, 0 новых зависимостей. Колонка SQLite — через существующий `modernc.org/sqlite`. |
| II | Парсинг — ручной | PASS | `parseDeadlineTrigger`/`expectLexeme` — ручной recursive descent, без генераторов/regex. `задача`/`просрочена` распознаёт посимвольно-лексированный IDENT + контекст парсера. |
| III | Ошибки — явные типы | PASS | `SE-EXPECTED`/`SE-TRIGGER-KIND` через `p.error`→`ParseError`; семошибки через `semErr`→`СемантическаяОшибка`. Без паник в штатных путях; `safeFire` изолирует тело эскалации. |
| IV | Позиции — сквозные | PASS | `DeadlineTrigger.Pos()`=токен `задача` (specBase); `msgExpected` несёт `got.Pos`; семошибки — позиция `Process`/`Step` Ident. Колонки в рунах сохранены. |
| V | Без глобального состояния | PASS | `checkDeadlines`/`fireDeadlineBody` — методы `*Daemon`; `Task.Escalated` — поле структуры, не package-state. Clock инжектируется. |
| VI | Тесты — вперёд | PASS | tasks-first: замки B4a (AST/parse/analyze/run/SE-каскад/v1-IDENT) и B4b (4 точки кодека/4-я фаза/durable/инжект) пишутся ДО правок; каждый имеет инверсию (мутпробу). |
| VII | Раскладка проекта | PASS | Без новых пакетов вне `internal/daemon`. Граф ацикличен: `daemon`→`store`/`engine`/`eval` (как сейчас); `eval` НЕ получает store/engine. Листовость `value`/`ast` цела. |
| VIII | Язык сообщений | PASS | Все тексты дословно §AU-10: run-заглушка, SE-TRIGGER-KIND расширение, SE-EXPECTED шаблон, семошибки, `[уведомление]`-формат. Русские имена `задача`/`просрочена`/`руководитель`. |
| IX | Спека — источник истины | PASS | Поведение залочено §AU-6/§AU-2/§AU-10/§AU-12 + D-AU-4/5/6; имена/строки/точки цитированы из якоря. Решения locked владельцем 2026-06-16. Расхождение §AU-11.1 (22 vs живые 24 файла clean[]) зафиксировано в spec как устаревший якорь, без догадок. |

**Complexity Tracking**: нарушений нет → таблица пуста.

## Project Structure

### Documentation (this feature)

```text
specs/016-deadline-escalation/
├── plan.md              # Этот файл
├── research.md          # Phase 0: прецеденты (триггеры/tick-фазы/кодек Task/RunRestartScan/часы)
├── data-model.md        # Phase 1: DeadlineTrigger AST; Task.Escalated + 4 точки кодека; 4-я фаза
├── quickstart.md        # Phase 1: ручной прогон run-заглушки + Go-API durable-сценарий
├── contracts/
│   ├── grammar-parser.md          # B4a: грамматика + parseDeadlineTrigger + expectLexeme
│   ├── analyze-semantics.md       # B4a: кейс DeadlineTrigger в checkTrigger
│   ├── se-trigger-kind-cascade.md # B4a: каскад зеркал SE-TRIGGER-KIND (свои + доковые)
│   ├── task-escalated-codec.md    # B4b: Task.Escalated + 4 точки SQLite-кодека
│   ├── tick-phase-checkdeadlines.md # B4b: 4-я фаза tick + checkDeadlines + fireDeadlineBody
│   └── durable-restart.md         # B4b: durable-контракт × рестарт (Go-API тест)
└── tasks.md             # Phase 2 (/speckit-tasks — отдельно)
```

### Source Code (repository root)

```text
src/
├── internal/
│   ├── ast/
│   │   └── trigger.go       # B4a: DeadlineTrigger{specBase;Process,Step Ident} + New + triggerSpec()
│   ├── parser/
│   │   ├── parse_decl.go    # B4a: ветка задача→parseDeadlineTrigger в parseTriggerDecl :406; expectLexeme
│   │   └── errors.go        # B4a: msgTriggerKind :29 расширение + коммент :26
│   ├── eval/
│   │   ├── analyze.go       # B4a: кейс *ast.DeadlineTrigger в checkTrigger :319 (процесс/шаг/тело)
│   │   └── trigger_run.go   # B4a: run-заглушка "задача триггер '%s.%s' требует serve (фича 007b)"
│   ├── store/
│   │   ├── types.go         # B4b: Task.Escalated bool :48-57
│   │   ├── sqlite.go        # B4b: 4 точки — DDL :33, SaveTask :161, ON CONFLICT :165, buildTask/scanTask :296/310
│   │   └── memory.go        # B4b: copyTask несёт Escalated тривиально (cp := *t)
│   ├── daemon/
│   │   ├── tick.go          # B4b: 4-я фаза d.checkDeadlines() в хвост tick() :10-16
│   │   └── checkdeadlines.go # B4b НОВЫЙ: checkDeadlines (скан) + fireDeadlineBody (инжект InstanceVariables)
│   └── engine/
│       └── format.go        # B4b: ЧИТАЕТСЯ engine.Overdue :35 (без правок)
├── examples/                # B4a опц.: контроль_плана.ladix/ошибка_эскалация.ladix → clean[]/golden
└── cmd/ladix/               # БЕЗ изменений (start/inspect — B5/B6; run уже проводит триггеры)
```

**Structure Decision**: Single project, существующая Go-раскладка. B4a правит фронтенд
(`ast`/`parser`/`eval`), B4b — `store`/`daemon`. Новый файл — только `daemon/checkdeadlines.go`
(новых пакетов нет). `engine`/`value`/`cmd/ladix` — читаются/не трогаются.

## Последовательность с B5 и M2-гейтом (осознанная, НЕ дефект)

§AU-12.B durable-golden в каноне использует `ladix start эскалация_плана 2500000 --db demo.db`
для создания инстанса. Но `ladix start` — это **B5**, идущая ПОСЛЕ B4 в поезде
(топо-порядок §AU-0: `B4a → B4b … B5 → B6`). Поэтому B4b закрывает durable-инвариант
БЕЗ зависимости от ещё не реализованной B5:

- **B4b сейчас (этот прогон)**: durable-поведение на уровне **Go-API**. Тест создаёт
  инстанс+задачу с дедлайном напрямую через `engine.Start`/`Store.SaveInstance`/`SaveTask` в
  SQLite (или `MemoryStore` для не-durable-частей), прогоняет `d.tick()` с `FixedClock`,
  продвигает часы за дедлайн, проверяет РОВНО одну эскалацию; пересоздаёт `SQLiteStore` на
  той же `--db`-файле, `RunRestartScan` + tick → нет повтора. Мутпроба `if t.Escalated` →
  красный. Это полностью покрывает §AU-12.B замки (а)–(г) на Go-уровне.
- **M2-гейт (ПОСЛЕ B5/B6, не этот прогон)**: полная CLI-сборка §AU-12.B (через `ladix
  start`/`ladix serve`/`ladix inspect`) и end-to-end §AU-12.C (контроль_плана.ladix под serve
  + реальный CSV + edge метрики + complete --данные + вебхук + inspect). Собирает архитектор/
  интегратор на гейте, когда B5/B6 влиты.

Это осознанная последовательность поезда: B4b durable-ядро доказуемо на Go-API ДО появления
CLI-обвязки; CLI-golden — терминальный гейт M2, не критерий приёмки отдельной B4b.

## Доковые зеркала для архитектора (синк на M2-гейте — НЕ зона B4)

Расширение `msgTriggerKind` (`метрика, событие или расписание` → `… расписание или задача`)
имеет зеркала в БОЛЬШИХ доках, которые B4 НЕ редактирует, но перечисляет для синка:
- `docs/trigger-model.md:432`, `:433`, `:1070`, `:1087` (цитаты старого текста SE-TRIGGER-KIND).
- `SPEC §13` / `docs/diagnostics-model.md` — любые цитаты старого текста.
- Каскад §AU-12 (финальный): `grammar.md` (DeadlineTrigger, контекст-разбор задача/просрочена),
  `execution-model.md` (4-я фаза tick, Task.Escalated), `examples/MANIFEST.md` (новые примеры),
  `README.md` (эскалация добавлена — co-land на M2-гейте).

**Зеркала-замки B4a (МОЯ зона, ОБЯЗАТЕЛЬНЫ к со-обновлению, иначе `go test` красный):**
`parser/errors.go:29` (+коммент `:26`), `parser/inventory_test.go:34`,
`parser/parse_decl_test.go:1549/1622/1666` (ТРИ exact-match golden).

## Phase 0 — Research (research.md)

Прецеденты: 3 существующих вида триггера (Metric/Event/Schedule) — AST/парсинг/семпроход/
run-заглушка как образец DeadlineTrigger; `checkTriggerBody(false,false)` расписания —
точный шаблон тела эскалации; tick-фазы 007b и `safeFire`/`fireBody`/`NewTriggerBodyEnv`;
кодек `Task` (DDL/SaveTask/ON CONFLICT/buildTask/scanTask) — 4 точки; `RunRestartScan`/
`ListInstancesByStatus`; `engine.Overdue`; часы `d.clock`. Эмпирика строк @a92ad50.

## Phase 1 — Design (data-model.md, contracts/, quickstart.md)

`DeadlineTrigger` AST (поля/конструктор/маркер); `Task.Escalated` + 4 точки кодека ДО/ПОСЛЕ;
4-я фаза tick (псевдокод `checkDeadlines` + `fireDeadlineBody`); durable-контракт × рестарт;
SE-TRIGGER-KIND каскад (свои + доковые зеркала); грамматика+парсер контракт; семпроход контракт.

## Complexity Tracking

> Нарушений Constitution нет — таблица пуста.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
