# Research: B1 «Захват результата `вызвать` как выражения»

**Feature**: 013-call-result | **Date**: 2026-06-17 | **Источник истины**: `docs/automation-model.md` §AU-3 (D-AU-1)

Все технические решения B1 залочены владельцем (§AU-1, 2026-06-16). Этот документ фиксирует прецеденты и эмпирику кодовой базы, на которые опирается реализация, чтобы исключить пере-открытие и дрейф.

## R-1. Прецедент: `вызвать`-выражение = клон `RunProcessExpr`

**Решение**: новый узел/парсер/eval B1 строятся ПО ОБРАЗЦУ уже работающего узла «запустить процесс как выражение» (`RunProcessExpr`), а не изобретаются заново.

**Эмпирика (живые точки кода, master @95f61e7)**:
- AST-узел: `src/internal/ast/expr.go:69` — `type RunProcessExpr struct { exprBase; Process Ident; Args []Expression }`, конструктор `NewRunProcessExpr(pos Position, process Ident, args []Expression) *RunProcessExpr` (строка 76). `Pos()` = токен-инициатор, наследуется от `exprBase`. Узел НЕ импортирует `errors` (листовость `ast`, Принцип VII).
- Парсер-ветка: `src/internal/parser/parse_expr.go:197` — `case lexer.KW_RUN: return p.parseRunProcess()` внутри `parsePrimary` (`:165`).
- Парсер-метод: `parseRunProcess` (`parse_expr.go:240-252`): потребляет ведущий токен (`p.advance()`), `expect(...)` цели, опционально `"(" parseArgList(RPAREN) ")"`, возвращает узел. Скобки — часть узла, постфикс на результат навешивается отдельной цепочкой `parsePostfix`.
- FIRST-set: `startsExpression` (`parse_expr.go:18-22`) содержит `lexer.KW_RUN, lexer.KW_VALUE, lexer.KW_EVENT` — место для `lexer.KW_CALL`.
- eval-кейс: `src/internal/eval/expr.go:51` — `case *ast.RunProcessExpr:` → `evalRunProcess` (`:194`): проверка `i.runtime != nil`, цикл аргументов слева направо, вызов метода шва, обёртка ошибки.

**Вывод**: B1 = механический аналог. Отличия: имя узла (`CallExternalExpr`, т.к. `CallExpr` занят), отсутствие `expect(KW_PROCESS)` (у `вызвать` нет служебного слова), метод шва (`CallExternalResult` вместо `StartProcess`), под стабом возвращается `value.None` (а не `value.Строка{id}`).

## R-2. Имя `CallExpr` занято — обязателен `CallExternalExpr`

**Эмпирика**: `src/internal/ast/expr.go:31` — `type CallExpr struct { exprBase; Callee Expression; Args []Expression }` + `NewCallExpr(callee, args)` — это ПОСТФИКС-вызов `f(args)` по результату любого выражения (`evalExpr` кейс `*ast.CallExpr`@`expr.go:45`). Имена `CallExpr`/`NewCallExpr` заняты в пакете `ast`.

**Вывод (D-AU-1)**: новый узел B1 ОБЯЗАН называться `CallExternalExpr`/`NewCallExternalExpr`. Любое другое имя ломает выбор владельца; имя `CallExpr` — компиляционный конфликт (redeclared). Семантическое отличие реально: `CallExpr` — вызов символа-функции программы (резолв, арность), `CallExternalExpr` — внешний эффект по логическому имени (без резолва, без арности).

## R-3. Развязка контекста statement ↔ выражение (без неоднозначности)

**Вопрос**: не возникнет ли двусмысленности `вызвать crm(x)` между действием и выражением?

**Эмпирика и решение (§AU-3.1)**:
- Ведущий `вызвать` (KW_CALL первым токеном утверждения) перехватывается в `parseStatement`/`parseStepAction` ДО входа в `parseExpression` → строит действие `ast.CallAction` (statement-форма, `step.go:21-25`, путь v1 без изменений). `вызвать crm(x)` отдельной строкой → действие.
- `присвоить` = `lexer.KW_SET` → `parseStepAction` строит `AssignAction` (НЕ `AssignStmt`), правая часть которого идёт через `parseExpression` → `parsePrimary` → новый `case KW_CALL` → `CallExternalExpr`. Поэтому `присвоить r = вызвать crm(x)` захватывает результат.
- `вызвать` как аргумент/элемент списка попадает в `parseExpression` тем же путём → `CallExternalExpr`.
- Постфиксы (`.поле`, `(…)`, `[…]`) навешиваются цепочкой `parsePostfix` на любой primary, включая `CallExternalExpr` (как на `RunProcessExpr`).

**Вывод**: контекст разрешается СТРУКТУРНО позицией токена в грамматике (statement-lead vs expression-position), не эвристикой. Неоднозначности нет. Добавление `KW_CALL` в `parsePrimary` НЕ перехватывает statement-путь, т.к. `parseStatement` ловит ведущий `вызвать` раньше.

## R-4. Шов `ProcessRuntime` 7→8: аддитивность и делегирование

**Эмпирика**: `src/internal/eval/runtime.go` — интерфейс `ProcessRuntime` с 7 методами (`StartProcess`, `AssignProcessVar`, `CallExternal`, `Notify`, `InstanceStatus`, `InstanceVariables`, `UserTasks`). Объявлен В `eval`, чтобы разорвать цикл импортов: `eval` НЕ импортирует ни `store`, ни `engine` (комментарий в файле + ребро `engine→eval` однонаправленно). `engine.Engine` реализует интерфейс: `var _ eval.ProcessRuntime = (*Engine)(nil)` (`engine/runtime.go:16`).

**Решение (§AU-2, D-AU-1)**: добавить РОВНО один метод:
```go
CallExternalResult(target string, args []value.Value) (value.Value, error) // НОВЫЙ (B1)
```
7 существующих сигнатур НЕ трогаются. Чтобы НЕ дублировать эффект, существующий `CallExternal` делегирует:
```go
func (e *Engine) CallExternal(t string, a []value.Value) error { _, err := e.CallExternalResult(t, a); return err }
```
Прецедент сосуществования statement+expression-методов уже есть в интерфейсе (`AssignProcessVar` — хук statement-присвоения).

**Тонкость стаба (§AU-3.3 + §AU-4)**: в B1 драйвер движка НЕ активируется (это B2). Дефолт-реализация `CallExternalResult` под стабом печатает прежнюю строку `[вызов] target(args)` (как нынешний `CallExternal`, `engine/runtime.go:41`) и возвращает `(value.None, nil)`. `CallExternal` после делегирования печатает ту же строку ровно один раз → golden §EN-7 байт-в-байт цел (R-6).

## R-5. eval-кейс: вычисление и обёртка ошибки

**Эмпирика**: `evalRunProcess` (`eval/expr.go:194`) — образец. Аргументы вычисляются слева направо (локальный цикл, идентичный `evalArgs`@`stmt.go:144`). Ошибка метода шва оборачивается `runtimeErrWrap(r.Pos(), err)` (`interpreter.go:189`) → `errors.ОшибкаВыполнения{Pos, Msg, Cause}`.

**Нюанс located-ошибки**: `evalRunProcess` сперва проверяет `errors.Расположенная` (`stderrors.As`) и пропускает уже-позиционированную ошибку тела шага без перезаписи позиции, оборачивая только не-located. Для B1 это НЕ применимо в v1: под дефолт-стабом `CallExternalResult` ВСЕГДА возвращает `nil`-ошибку, located-путь недостижим. §AU-3.3 предписывает прямую обёртку `runtimeErrWrap(c.Pos(), err)` — единая категория сбоя внешнего вызова (`ОшибкаВыполнения` с цепочкой `Cause`, закрывает TODO(D-14)). Реальный сбой появится только под HTTP-драйвером B2; B1 пишет обёртку «на будущее», тестируется через фейк-runtime, возвращающий ошибку.

**Вывод**: кейс B1 проще, чем `evalRunProcess` (нет фазы атрибутов, нет `value.Строка{id}`): evalArgs → `CallExternalResult` → при ошибке `runtimeErrWrap(c.Pos(), err)`, иначе вернуть значение. Проверка `i.runtime == nil` — как в `evalRunProcess` (защитный инвариант).

## R-6. Инвариант: golden §EN-7 печать-стаба цел

**Эмпирика**: `engine/runtime.go:41` (`CallExternal`) и `:54` (`Notify`) печатают фиксированные строки `[вызов] %s(%s)` / `[уведомление] %s…`. Это golden-пины §EN-7 (≥6, §AU-2/§AU-10.D).

**Риск и митигация**: B1 меняет грамматику (фронтенд + кейс eval + метод шва), НЕ драйвер. Единственная точка риска — делегирование `CallExternal → CallExternalResult`: печать должна остаться РОВНО одной строкой и байт-в-байт прежней. Митигация: дефолт-`CallExternalResult` несёт ту же `Fprintf`, `CallExternal` лишь отбрасывает значение (печать не дублируется и не теряется). Замок — существующие golden §EN-7 + новый замок «двойного эффекта нет» (см. tasks.md). `Notify` НЕ трогается (B1 не вводит выражение-форму `уведомить`).

## R-7. Граница scope B1: что НЕ делается

- НЕ трогается `store` (пустой дифф `src/internal/store`), Store остаётся 15 методов.
- НЕ трогается реальный драйвер/HTTP-вебхук — это B2 (§AU-4). B1 ставит ТОЛЬКО грамматику захвата + метод шва + стаб-возврат `value.None`.
- НЕ вводится выражение-форма `уведомить`.
- НЕ вводятся новые статические диагностики; цель — строка-имя (без резолва символа/арности).
- НЕ ослабляется read-only барьер тела триггера (007a) и step-only контекст-гард действий (`analyze.go`).
- НЕ правятся большие доки (`SPEC.md`, `docs/grammar.md`, `docs/automation-model.md`) — синк архитектора на M2-гейте.
- НЕ добавляются зависимости (go.mod: единственная `modernc.org/sqlite`).

## Открытые вопросы

Нет. Все развилки закрыты §AU-1 (D-AU-1) и §AU-3; эмпирика прецедента `RunProcessExpr` подтверждена на master @95f61e7.
