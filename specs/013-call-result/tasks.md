---
description: "Task list — 013-call-result (веха M2, B1)"
---

# Tasks: B1 «Захват результата `вызвать` как выражения»

**Input**: Design documents from `/specs/013-call-result/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/ (process-runtime.md, parser-call-expr.md)

**Tests**: ВКЛЮЧЕНЫ (Принцип VI tests-first; табличные тесты узла/парсера + контрактные замки шва/eval пишутся вместе с кодом). Для КАЖДОГО тест-замка указана инверсная мутация продукт-кода, обязанная его покраснить (исполняется в L1-Реализация).

**Organization**: одна user-story US1 (P1) — захват результата `вызвать` как выражения. Фаза 3 = AST+парсер (фронтенд), фаза 4 = шов+eval (исполнение), фаза 5 = интеграция/инварианты.

**Команды**: сборка/тесты из `src/` (`cd src && go build -o ../ladix ./cmd/ladix`; `cd src && go test ./... -count=1`). Источник истины — `docs/automation-model.md` §AU-3 (D-AU-1).

## Format: `[ID] [P?] [Story] Description`
- **[P]**: разные файлы, нет зависимостей от незавершённых задач.
- Каждая задача — путь файла + проверяемый критерий. Тест-задачи несут блок **Инверсия:** (мутация, обязанная покраснить замок).

---

## Phase 1: Setup

- [x] T001 Подтвердить стартовую базу на ветке `013-call-result`: `cd src && gofmt -l . && go vet ./... && go test ./... -count=1 && go build ./...` — всё зелёное («зелёный до правок»). Зафиксировать вывод в леджере.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: общей новой инфры нет — `ProcessRuntime` (`eval/runtime.go`), `exprBase`/`Pos()` (`ast/expr.go`), `evalExpr` type-switch, `runtimeErrWrap` (`interpreter.go:189`), golden §EN-7 (`engine`) уже на месте. Узел B1 — клон прецедента `RunProcessExpr`. Зафиксировать неприкосновенное.

- [x] T002 [P] Зафиксировать в леджере карту неприкосновенного (НЕ трогать ни в одной фазе): `src/internal/store` (пустой дифф, Store=15), реальный драйвер/HTTP (это B2), `Notify` (`engine/runtime.go:54`), step-only контекст-гард действий (`analyze.go:584-592`) и его замки (`analyze_decl_test.go:422`, `analyze_trigger_test.go:92`), read-only барьер тела триггера (007a), существующие 7 сигнатур `ProcessRuntime`, eval-`errors_golden` счётчик `len(seen)!=28`. Также: имя `CallExpr`/`NewCallExpr` (`ast/expr.go:31`) — занято, НЕ переиспользовать.

**Checkpoint**: можно начинать US1.

---

## Phase 3: User Story 1 — фронтенд (AST + парсер) (Priority: P1) 🎯 MVP

**Goal**: `вызвать` получает выражение-форму `CallExternalExpr`; statement-форма `CallAction` и `уведомить`-только-statement целы; аддитивность v1.

**Independent Test**: `присвоить r = вызвать crm(x)` → RHS = `*CallExternalExpr`; `вызвать crm(x)` отдельной строкой → `CallAction`; `уведомить` в позиции выражения → ошибка; v1-выражения/`f(x)` не регрессируют.

### Tests for US1 — фронтенд (tests-first — ДОЛЖНЫ упасть до реализации) ⚠️

- [x] T003 [P] [US1] Red→green C-AST-1 в `src/internal/ast/expr_test.go`: `TestNewCallExternalExpr` — `NewCallExternalExpr(pos, target, args)` строит узел; `Pos()` == `pos` (токен `вызвать`); `Target`/`Args` сохранены; узел реализует `ast.Expression`. Критерий: СЕЙЧАС не компилируется (узла нет) → red.
  **Инверсия:** конструктор кладёт `target.Pos()` вместо `pos` ИЛИ теряет `Args` → `Pos()`/`Args` ≠ ожидаемым → red.
- [x] T004 [P] [US1] Red→green C-AST-1.1 (имя) — статический замок: узел зовётся `CallExternalExpr` (не `CallExpr`); проверяется самим фактом компиляции пакета `ast` (отдельный тип, без redeclared). Критерий: попытка назвать `CallExpr` → `redeclared in this block` (компилятор).
  **Инверсия:** назвать узел `CallExpr` → пакет `ast` не компилируется.
- [x] T005 [P] [US1] Red→green C-PARSE-2/3 в `src/internal/parser/parse_expr_test.go`: `TestParseCallExprInAssignRHS` — `присвоить r = вызвать crm("к")` (в теле шага) → RHS = `*ast.CallExternalExpr` с `Target.Name=="crm"`, 1 аргумент; НЕ `*ast.CallAction`, НЕ `*ast.CallExpr`. Критерий: СЕЙЧАС red (нет ветки `case KW_CALL`).
  **Инверсия:** убрать `case lexer.KW_CALL` из `parsePrimary` → `вызвать` уходит в default-ветку (ошибка) → RHS не `CallExternalExpr` → red.
- [x] T006 [P] [US1] Red→green C-PARSE-2 (FIRST-set) в `src/internal/parser/parse_expr_test.go`: `TestStartsExpressionCall` — `startsExpression(lexer.KW_CALL)==true`; и интеграционно `печать(вызвать сервис())` + `пусть xs = [вызвать a(), 1]` разбираются с `CallExternalExpr` в позиции аргумента/элемента. Критерий: СЕЙЧАС red (KW_CALL не в `startsExpression`).
  **Инверсия:** не добавлять `lexer.KW_CALL` в `startsExpression` → `false`; `вызвать` как аргумент/элемент не распознаётся как начало выражения → red.
- [x] T007 [P] [US1] Red→green C-PARSE-3 (постфикс) в `src/internal/parser/parse_expr_test.go`: `TestParseCallExprPostfix` — `вызвать crm(x).статус` → `*ast.FieldExpr{Target: *ast.CallExternalExpr}` (постфикс цепочкой `parsePostfix`, скобки — часть узла). Критерий: red до реализации.
  **Инверсия:** не возвращать узел из `parsePrimary` (или поглотить `.статус` внутрь `parseCallExternalExpr`) → структура ≠ `FieldExpr{Target:CallExternalExpr}` → red.
- [x] T008 [P] [US1] Контроль развязки (зелёный СЕЙЧАС, остаётся зелёным) в `src/internal/parser/parse_stmt_test.go`: `TestLeadingCallStaysStatement` — `вызвать ИТ("з")` отдельной строкой как ведущий токен шага → `*ast.CallAction` (путь v1). `TestNotifyNotExpression` — `присвоить x = уведомить кто("a")` → синтаксическая ошибка (`уведомить` не выражение). Критерий: оба зелёные ДО и ПОСЛЕ реализации (правка не перехватывает ведущий `вызвать`, не вводит `уведомить`-выражение).
  **Инверсия:** если реализация ошибочно перехватит ведущий `вызвать` в выражение → `TestLeadingCallStaysStatement` red; если добавит `case KW_NOTIFY` в primary → `TestNotifyNotExpression` red.

### Implementation for US1 — фронтенд

- [x] T009 [US1] **AST**: добавить в `src/internal/ast/expr.go` узел `CallExternalExpr{ exprBase; Target Ident; Args []Expression }` + `NewCallExternalExpr(pos Position, target Ident, args []Expression) *CallExternalExpr` (по образцу `RunProcessExpr:69-77`; имя НЕ `CallExpr`). Не импортировать `errors`. Критерий: T003/T004 зеленеют; пакет `ast` компилируется.
- [x] T010 [US1] **Парсер метод**: добавить в `src/internal/parser/parse_expr.go` (рядом с `parseRunProcess:240`) метод `parseCallExternalExpr`: `advance()` `вызвать` → `expect(lexer.IDENT, "имя цели")` → опц. `"(" parseArgList(lexer.RPAREN) ")"` → `ast.NewCallExternalExpr(toASTPos(callTok.Pos), *target, args)`. Критерий: компилируется.
- [x] T011 [US1] **Парсер диспетч**: в `parsePrimary` (`parse_expr.go:165`) добавить `case lexer.KW_CALL: return p.parseCallExternalExpr()` (рядом с `case lexer.KW_RUN`@197). Критерий: T005/T007 зеленеют.
- [x] T012 [US1] **FIRST-set**: в `startsExpression` (`parse_expr.go:18-22`) добавить `lexer.KW_CALL` в case-список (рядом с `KW_RUN`/`KW_VALUE`/`KW_EVENT`). Критерий: T006 зеленеет; T008 остаётся зелёным.
- [x] T013 [US1] Мутант-доказательство фронтенда: временно откатить T011 → `cd src && go test ./internal/parser/ -run 'CallExpr|StartsExpression' -count=1` ДОЛЖЕН упасть; вернуть. Зафиксировать в леджере (замки реально кусаются, не «полые»).
- [x] T014 [US1] Аддитивность фронтенда (§3-инвариант): `cd src && go test ./internal/ast/... ./internal/parser/... -count=1` — все существующие v1-тесты (выражения, постфикс-вызов `f(x)`, statement-развязка) зелёные без правки ожиданий. Критерий: 0 регрессий.

**Checkpoint**: фронтенд B1 функционален; `CallExternalExpr` строится в позиции выражения, statement/`уведомить` целы.

---

## Phase 4: User Story 1 — исполнение (шов + eval) (Priority: P1)

**Goal**: `ProcessRuntime` 7→8 (`CallExternalResult`); `CallExternal` делегирует; кейс `evalExpr` вычисляет узел через шов; под стабом → `value.None`; golden §EN-7 цел; `eval` без `store`/`engine`.

**Independent Test**: фейк-runtime: стаб→`value.None`/err=nil; левый-направо args; ошибка→`ОшибкаВыполнения` с Pos; `CallExternal` печатает РОВНО одну строку.

### Tests for US1 — исполнение (tests-first) ⚠️

- [x] T015 [P] [US1] Red→green C-SEAM-1 в `src/internal/eval/runtime_test.go` (или `expr_test.go`): компиляц.-замок — фейк `ProcessRuntime` реализует все 8 методов, включая `CallExternalResult(target string, args []value.Value) (value.Value, error)`; `var _ eval.ProcessRuntime = (*fakeRuntime)(nil)`. Критерий: СЕЙЧАС не компилируется (метода в интерфейсе нет) → red.
  **Инверсия:** удалить `CallExternalResult` из интерфейса `ProcessRuntime` → кейс eval (T017/T018/T019) не компилируется (`i.runtime.CallExternalResult` неизвестен) → red. (Счёт «ровно 8» закрепляется связкой T015+T017.)
- [x] T016 [P] [US1] Red→green C-SEAM-2.1 в `src/internal/engine/runtime_test.go`: `TestCallExternalDelegatesToResult` — вызвать `engine.CallExternal("crm", args)` с буфером `out`; ожидать РОВНО одну строку `[вызов] crm(...)\n` (нет двойного эффекта). Дополнительно `TestCallExternalResultStubPrintsAndReturnsNone` — `CallExternalResult` печатает ту же строку и возвращает `(value.None, nil)`. Критерий: red до реализации делегирования.
  **Инверсия:** оставить у `CallExternal` собственную `Fprintf` И вызвать `CallExternalResult` (тоже печатает) → две строки → red; ИЛИ `CallExternalResult` вернуть не-None → второй замок red.
- [x] T017 [P] [US1] Red→green C-SEAM-3.3 в `src/internal/eval/expr_test.go`: `TestEvalCallExternalStubReturnsNone` — исполнить `присвоить r = вызвать сервис(1)` (или прямой `evalExpr` узла) с фейк-runtime, чьим `CallExternalResult` возвращает `(value.None, nil)`; ожидать значение `value.None`, err=nil. Критерий: red (нет кейса в `evalExpr`).
  **Инверсия:** не добавлять `case *ast.CallExternalExpr` в `evalExpr` → узел падает в default «неизвестное выражение» → err ≠ nil → red.
- [x] T018 [P] [US1] Red→green C-SEAM-3.1 (порядок) в `src/internal/eval/expr_test.go`: `TestEvalCallExternalArgsLeftToRight` — фейк-runtime записывает порядок аргументов через побочный эффект eval (напр. вызовы-счётчики); ожидать строго слева-направо. Критерий: red до реализации.
  **Инверсия:** в кейсе вычислять `c.Args` в обратном порядке → порядок ≠ исходному → red.
- [x] T019 [P] [US1] Red→green C-SEAM-3.2 (обёртка ошибки) в `src/internal/eval/expr_test.go`: `TestEvalCallExternalWrapsError` — фейк-`CallExternalResult` возвращает `(nil, errors.New("boom"))`; ожидать `errors.ОшибкаВыполнения` (через `errors.As`) с `Pos` == позиция токена `вызвать` и `Cause` == исходная ошибка. Критерий: red до реализации.
  **Инверсия:** вернуть raw `err` без `runtimeErrWrap(c.Pos(), err)` → тип ≠ `ОшибкаВыполнения` / нет Pos → red.

### Implementation for US1 — исполнение

- [x] T020 [US1] **Шов объявление**: в `src/internal/eval/runtime.go` добавить в интерфейс `ProcessRuntime` метод `CallExternalResult(target string, args []value.Value) (value.Value, error)` (рядом с `CallExternal`), с doc-комментарием. 7 существующих сигнатур НЕ трогать. Критерий: T015 компилируется; `eval` НЕ импортирует `store`/`engine` (см. T024).
- [x] T021 [US1] **Шов реализация + делегирование**: в `src/internal/engine/runtime.go` реализовать `func (e *Engine) CallExternalResult(target string, args []value.Value) (value.Value, error)` — печать `[вызов] %s(%s)` (как нынешний `CallExternal:41`, разделитель `", "`) + `return value.None, nil`; ПЕРЕПИСАТЬ `CallExternal` на делегирование `{ _, err := e.CallExternalResult(target, args); return err }`. `var _ eval.ProcessRuntime = (*Engine)(nil)` (`:16`) должен компилироваться. Критерий: T016 зеленеет.
- [x] T022 [US1] **eval кейс**: в `src/internal/eval/expr.go` добавить в `evalExpr` (type-switch) `case *ast.CallExternalExpr:` → метод `evalCallExternal(env, c)`: защита `i.runtime == nil` → `runtimeErr(c.Pos(), "…движок процессов не подключён")`; вычислить `c.Args` слева направо (как `evalArgs`/`evalRunProcess:198-205`); `v, err := i.runtime.CallExternalResult(c.Target.Name, args)`; `err != nil → return nil, runtimeErrWrap(c.Pos(), err)`; иначе `return v, nil`. Критерий: T017/T018/T019 зеленеют.
- [x] T023 [US1] Мутант-доказательство исполнения: временно откатить T022 → `cd src && go test ./internal/eval/ -run CallExternal -count=1` ДОЛЖЕН упасть; временно вернуть `CallExternal` к собственной печати + делегированию → `TestCallExternalDelegatesToResult` падает (двойная строка); вернуть. Зафиксировать в леджере.

**Checkpoint**: исполнение B1 функционально; захват результата работает под стабом → `value.None`.

---

## Phase 5: Интеграция и инварианты (Priority: P1)

- [x] T024 [US1] Инвариант FR-012 (eval без store/engine): `cd src && go list -deps ./internal/eval | grep -E 'github.com/denis-kosyakov/ladix/internal/(store|engine)$'` — пусто (eval не зависит от store/engine). Критерий: команда ничего не печатает.
- [x] T025 [US1] Инвариант FR-015 / golden §EN-7: `cd src && go test ./internal/engine/... -count=1` — все golden печать-стаба `[вызов]`/`[уведомление]` зелёные, тексты байт-в-байт. Критерий: 0 изменений golden-текстов.
- [x] T026 [US1] Инвариант пустого диффа store/драйвера: `git diff --stat master -- src/internal/store` пуст; реальный драйвер/HTTP не введён (`grep -rn 'http\|webhook\|вебхук' src/internal/engine/` — нет нового драйвера B2). Критерий: пустой дифф store; драйвер не тронут.
- [x] T027 [US1] Полный гейт + интеграция: `cd src && gofmt -l . && go vet ./... && go test ./... -count=1 && go build -o ../ladix ./cmd/ladix`. Прогнать quickstart-сниппеты (`присвоить ответ = вызвать crm("к")` → стаб + `Пусто`, exit 0; `вызвать ИТ("з")` → `CallAction`; `уведомить` в выражении → ошибка). Критерий: всё зелёное; поведение совпадает с quickstart.md.
- [x] T028 [US1] Дрейф-аудит §AU-2: подтвердить `ProcessRuntime` = РОВНО 8 методов (был 7); `Store` = 15 (не тронут); ребро `engine→eval` однонаправленно. Зафиксировать в леджере для M2-гейта.

**Checkpoint**: B1 завершён, инварианты 1-3 закрыты, готов гейтить B2.

---

## Карта покрытия (FR → задачи)

| FR | Задачи |
|---|---|
| FR-001 узел `CallExternalExpr` | T003, T004, T009 |
| FR-002 `parseCallExternalExpr` | T005, T010, T011 |
| FR-003 `startsExpression += KW_CALL` | T006, T012 |
| FR-004 statement `CallAction` цел | T008, T014 |
| FR-005 `уведомить` только statement | T008 |
| FR-006 постфиксы на узле | T007, T011 |
| FR-007 разрешено где выражение | T005, T006 |
| FR-008 цель=имя, без резолва/арности | T005, T002 |
| FR-009 не под step-гардом действий | T002, T008 |
| FR-010 шов 7→8 `CallExternalResult` | T015, T020 |
| FR-011 `CallExternal` делегирует | T016, T021 |
| FR-012 eval без store/engine | T024 |
| FR-013 кейс `evalExpr` (args л→п) | T017, T018, T022 |
| FR-014 `runtimeErrWrap` | T019, T022 |
| FR-015 golden §EN-7 цел | T016, T025 |

## Зависимости (порядок)

- T001 → T002 → (фаза 3) → (фаза 4) → (фаза 5).
- Фронтенд (T003-T014) и исполнение (T015-T023) частично параллельны, НО eval-кейс (T022) требует узла (T009) и метода шва (T020). Тесты [P] независимы по файлам.
- Все мутант-доказательства (T013, T023) — после соответствующей реализации.
- Инварианты (T024-T028) — последними, после зелёной реализации.

## Итог

**28 задач** (T001-T028). Фазы: Setup (1), Foundational (1), US1-фронтенд (12: 6 тестов + 6 импл), US1-исполнение (9: 5 тестов + 4 импл), Интеграция/инварианты (5). Тест-замков — **11** (T003-T008 фронтенд = 6; T015-T019 исполнение = 5), каждый с явной инверсной мутацией. Мутант-доказательств — 2 (T013, T023).
