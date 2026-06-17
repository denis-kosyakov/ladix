# Implementation Plan: B1 «Захват результата `вызвать` как выражения»

**Branch**: `013-call-result` | **Date**: 2026-06-17 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/013-call-result/spec.md`

## Summary

B1 (веха M2, `docs/automation-model.md` §AU-3, решение **D-AU-1**) даёт `вызвать` ВТОРОЙ контекст разбора — выражение — для **захвата результата** внешнего вызова (`присвоить r = вызвать crm(x)`), сохраняя при этом statement-форму `CallAction` (fire-and-forget) без изменений. Технический подход чисто аддитивен и зеркалит уже существующий прецедент `RunProcessExpr` («запустить процесс» как выражение):

1. **AST** — новый узел `CallExternalExpr` + конструктор `NewCallExternalExpr` (`ast/expr.go`), по образцу `RunProcessExpr` (поля `Target Ident`, `Args []Expression`, `exprBase`). Имя `CallExpr`/`NewCallExpr` УЖЕ ЗАНЯТО постфикс-вызовом `f(args)` (`ast/expr.go:31`) → новый узел ОБЯЗАН называться `CallExternalExpr`, иначе redeclared.
2. **Парсер** — ветка `case lexer.KW_CALL → parseCallExternalExpr()` в `parsePrimary` (`parse_expr.go:165`, рядом с `case lexer.KW_RUN`@197), новый метод `parseCallExternalExpr` зеркалит `parseRunProcess` (`parse_expr.go:240`), плюс `lexer.KW_CALL` в `startsExpression` (`parse_expr.go:18-22`, рядом с `KW_RUN`/`KW_VALUE`/`KW_EVENT`). Постфиксы навешиваются штатной цепочкой `parsePostfix`.
3. **Шов eval↔engine** — интерфейс `ProcessRuntime` (`eval/runtime.go`) расширяется РОВНО одним методом `CallExternalResult(target string, args []value.Value) (value.Value, error)` (7→8); 7 существующих сигнатур не трогаются; `eval` НЕ импортирует `store`/`engine`. В `engine` существующий `CallExternal` делегирует `CallExternalResult` с отбросом значения (эффект не дублируется); `var _ eval.ProcessRuntime = (*Engine)(nil)`.
4. **eval** — новый кейс `*ast.CallExternalExpr` в `evalExpr` (`eval/expr.go`): вычислить `args` слева направо (как `evalArgs`), вызвать `i.runtime.CallExternalResult(c.Target.Name, args)`, вернуть `value.Value`; ошибка через `runtimeErrWrap(c.Pos(), err)`. Под дефолт-стабом — `value.None`.

`уведомить` остаётся ТОЛЬКО statement. Новых статических диагностик нет. Драйвер движка не трогается → golden §EN-7 целы (B1 меняет грамматику, не драйвер). 0 новых зависимостей. B1 **гейтит B2**.

## Technical Context

**Language/Version**: Go 1.25 (`src/go.mod`, module `github.com/denis-kosyakov/ladix`). Идиоматичный Go: `gofmt`, `go vet ./...` без замечаний (Принцип I).

**Primary Dependencies**: 0 новых. Единственная внешняя — `modernc.org/sqlite` (B1 её не затрагивает). Узел/парсер/eval/шов — чистый stdlib.

**Storage**: N/A. `store`/`engine`-store не трогаются — пустой дифф `src/internal/store`. SQLite-кодек, Store-методы, durable-поля вне scope B1.

**Testing**: `cd src && go test ./... -count=1`. Tests-first для парсера (Принцип VI): табличные тесты узла/парсера + негативные кейсы и контрактные замки шва пишутся вместе с кодом. Инверсные мутпробы на каждый замок (см. tasks.md).

**Target Platform**: один статический бинарник `ladix` (кросс-платформенный, без CGO).

**Project Type**: интерпретатор DSL (compiler/CLI), ручной recursive-descent (Принцип II).

**Performance Goals**: N/A. Узел добавляет один `case` в первичный разбор и один в `evalExpr` — тот же один проход, без изменения сложности горячего пути.

**Constraints**: аддитивность фронтенда (§3-инвариант: v1-выражения и постфикс-вызов `f(x)` не ломаются); шов `ProcessRuntime` строго +1 метод (7→8), 7 старых сигнатур байт-в-байт; `eval` без импорта `store`/`engine` (ребро `engine→eval` однонаправленно); печать-стаб `вызвать`/`уведомить` байт-в-байт (golden §EN-7); позиции в рунах с 1 (Принцип IV); цель — строка-имя, без резолва символа/проверки арности; детерминизм.

**Scale/Scope**: 1 новый AST-узел + конструктор; 1 новый метод парсера + 2 точечные правки (`parsePrimary` case, `startsExpression`); 1 новый метод интерфейса шва + делегирующая обёртка в `engine`; 1 новый кейс `evalExpr`. Тесты: AST-конструктор, парсер (выражение/statement-развязка/постфикс/аддитивность), контракт шва (8 методов, делегирование), eval (стаб→`Пусто`, левый-направо, обёртка ошибки).

## Constitution Check

*GATE: проверено до Phase 0 и повторно после Phase 1. Итог: **9/9 PASS**, 0 нарушений, Complexity Tracking пуст.*

- **I. Язык и сборка** — PASS. Go 1.25, `gofmt`/`go vet` чисто; 0 новых зависимостей; CGO не вводится.
- **II. Парсинг — ручной** — PASS. Новая ветка внутри существующего ручного recursive-descent (`parsePrimary`/`parseCallExternalExpr`); генераторы/regex не вводятся.
- **III. Ошибки — явные типы** — PASS. Сбой внешнего вызова заворачивается существующим `runtimeErrWrap` → `errors.ОшибкаВыполнения` с цепочкой `Cause` (фактический Go-тип, §AU-3.3); новых типов ошибок нет; паник в штатных путях нет; recover-барьер CLI цел.
- **IV. Позиции — сквозные** — PASS. `CallExternalExpr` несёт локальную `ast.Position` (токен `вызвать`); `Pos()` протаскивается до `runtimeErrWrap`; листовость `ast` сохранена (узел не импортирует `errors`).
- **V. Без глобального состояния** — PASS. Узел/парсер-метод/eval-кейс — чистые значения и методы инстансов; новый метод шва инжектируется через существующий `ProcessRuntime` (интерфейс, DI); пакет-уровневого состояния не вводится.
- **VI. Тесты — вперёд** — PASS. Табличные тесты парсера (узел, развязка statement↔выражение, постфикс, аддитивность v1) и контрактные замки шва/eval пишутся tests-first; каждый замок снабжён инверсной мутпробой (red→green), см. tasks.md.
- **VII. Раскладка проекта** — PASS. Правки в `internal/ast`, `internal/parser`, `internal/eval`, `internal/engine` (+ их тесты); граф зависимостей без циклов; `eval` НЕ импортирует `store`/`engine` (FR-012); листовость `ast`/`value`/`errors` цела.
- **VIII. Язык сообщений** — PASS. B1 НЕ вводит новых пользовательских сообщений и НЕ меняет существующие; печать-стаб `вызвать`/`уведомить` байт-в-байт (русский канон §EN-7 цел). Дословность текстов из SPEC/docs соблюдена тривиально (нет новых текстов).
- **IX. Спека — источник истины** — PASS. Поведение фиксируется размещёнными доками: spec.md + `docs/automation-model.md` §AU-3 (D-AU-1, locked владельцем); развилки разрешены явно в spec (Assumptions/Edge Cases), не молча; синк больших доков делегирован архитектору на M2-гейте письменно.

**Структурные инварианты якоря M2 §AU-2** (дрейф-аудит на гейте): `ProcessRuntime` 7→8 (РОВНО +`CallExternalResult`, 7 старых не трогаются); `Store` остаётся 15 (B1 его не касается); ребро `engine→eval` однонаправленно; пустой дифф `src/internal/store` и драйвера движка; golden §EN-7 байт-в-байт (SC-004). B1 — чистая аддитивность, санкционированных исключений нет.

## Project Structure

### Documentation (this feature)

```text
specs/013-call-result/
├── spec.md              # ✅ написана
├── plan.md              # ← этот файл
├── research.md          # Phase 0 (прецедент RunProcessExpr, развязка контекста, дефолт-стаб)
├── data-model.md        # Phase 1 (узел CallExternalExpr, метод шва CallExternalResult)
├── quickstart.md        # Phase 1 (как добавить узел B1, контрольный сниппет)
├── contracts/           # Phase 1 (process-runtime.md шов 7→8, parser-call-expr.md грамматика)
├── checklists/
│   └── requirements.md  # ✅ из specify
└── tasks.md             # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
src/internal/ast/
│   ├── expr.go                  # ← НОВЫЙ узел CallExternalExpr + NewCallExternalExpr (по образцу RunProcessExpr:69) [FR-001]
│   └── expr_test.go             # ← конструктор: Pos()=токен вызвать, Target/Args сохранены
src/internal/parser/
│   ├── parse_expr.go            # ← case lexer.KW_CALL в parsePrimary (рядом с KW_RUN:197);
│   │                            #    parseCallExternalExpr зеркалит parseRunProcess:240 [FR-002];
│   │                            #    lexer.KW_CALL в startsExpression:18-22 [FR-003]
│   ├── parse_expr_test.go       # ← узел в RHS присвоить/аргумент/элемент списка/постфикс [FR-001/006/007]
│   └── parse_stmt_test.go       # ← развязка: ведущий вызвать → CallAction (statement цел) [FR-004];
│                                #    уведомить в позиции выражения → ошибка (как v1) [FR-005]
src/internal/eval/
│   ├── runtime.go               # ← +метод CallExternalResult в ProcessRuntime (7→8); eval без store/engine [FR-010/012]
│   ├── expr.go                  # ← кейс *ast.CallExternalExpr в evalExpr: evalArgs→CallExternalResult→value;
│   │                            #    runtimeErrWrap(c.Pos(), err) [FR-013/014]
│   └── expr_test.go             # ← фейк ProcessRuntime: стаб→value.None, левый-направо args, обёртка ошибки [SC-001]
src/internal/engine/
│   └── runtime.go               # ← реализация CallExternalResult; CallExternal делегирует с отбросом;
│                                #    var _ eval.ProcessRuntime = (*Engine)(nil) [FR-011]
```

**Structure Decision**: одна фича, одна US (P1) в едином дереве. Аддитивный фронтенд (`ast` + `parser`) + один новый метод несущего шва (`eval/runtime.go` объявление, `engine/runtime.go` реализация + делегирование) + один кейс исполнения (`eval/expr.go`). ПУСТОЙ дифф `src/internal/store` и драйвера движка по всей фиче (инвариант). Большие доки (`SPEC.md`, `docs/grammar.md`, `docs/automation-model.md`) — зона архитектора на M2-гейте, не правятся. `examples/` и корневой quickstart — забота L1-Реализация/витрина, вне scope.

## Complexity Tracking

> Заполняется ТОЛЬКО при нарушениях Constitution Check.

Нарушений нет — таблица пуста. B1 — чистая аддитивность: новый AST-узел (имя отлично от занятого `CallExpr`), +1 метод шва (7→8, прецедент `RunProcessExpr`/`AssignProcessVar`), +1 кейс eval. Горячий инвариант panic-mode НЕ модифицируется (в отличие от 012/DX1). Шов eval↔engine остаётся однонаправленным.
