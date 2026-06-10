---
description: "Список задач реализации: Фронтенд процессов v1 — процесс, шаг, действия, запуск (фича 005)"
---

# Tasks: Фронтенд процессов v1 — процесс, шаг, действия, запуск

**Input**: Design documents from `specs/005-process-frontend/`
**Prerequisites**: [plan.md](./plan.md) ✅ (Guardrails 1–21, Phasing A–D, Project Structure), [spec.md](./spec.md) ✅ (FR-001..028, SC-001..007, US1–US3 все P1), [data-model.md](./data-model.md) ✅, contracts/ ✅ (ast-process.md, analyze-process.md, diagnostics-process.md, cli-process.md)
**Якорь поведения**: `docs/process-model.md` §PM-0…§PM-8 (тексты §PM-6 — дословно; решения D-1…D-13 закрыты, не переоткрываются; при расхождении побеждает якорь; для координат файл:строка план следует коду).

**Tests**: ВКЛЮЧЕНЫ обязательно — конституция VI (tests-first). Табличные `*_test.go` пишутся **в паре** с реализацией каждой задачи и обязаны падать до кода; exact-match §PM-6.A/B/C (16 байт-точных payload'ов, источник — [contracts/diagnostics-process.md](./contracts/diagnostics-process.md) §DP-1..§DP-3; deferred-тексты — §DP-4, eval-model §8.3) — переформулировать ЗАПРЕЩЕНО (конституция VIII). **Golden-чисел нет** — фронтенд ничего не вычисляет (§PM-7); приёмка = «парсится? + семантика чиста? + узлы/диагностика» + три CLI-кода (0/1/2). Вывод — через инжектированный `out` (`bytes.Buffer`); `go test -race` зелёный.

**Organization**: задачи сгруппированы по фазам A–D плана (фаза A — фундамент AST; B — парсер; C — семпроход + граница deferred; D — демо + синк-доки + CLI). Все три истории — **P1**. Все пути Go-кода относительны корня репозитория (`src/...` — корень Go-модуля, команды `go` из `cd src`); `examples/` — на корне репозитория, СИБЛИНГ `src/` (тесты читают через `filepath.Join("..","..","..","examples",name)`). Существующие пакеты правятся **аддитивно**; новых пакетов нет (`internal/engine`/`internal/store` НЕ заводятся, D-7/§PM-8).

## Format: `[ID] [P?] [Story] Описание`

- **[P]**: можно выполнять параллельно (разные файлы, нет зависимостей от незавершённых задач)
- **[Story]**: к какой User Story относится задача (US1-P1 / US2-P1 / US3-P1); фазы Foundational/Polish-части — без метки Story
- Каждая задача указывает точный путь к файлу и привязку: **Guardrail N** (плана) и/или **FR-NNN** и/или **§PM-раздел**

## Соответствие фаз, историй и гейтов (из plan.md «Phasing»)

| Фаза | История | Приоритет | Гейт |
|---|---|---|---|
| A. Фундамент (AST + синк union) | — | блокирует всё | AST-тесты зелёные; `go build`/`go vet`/`gofmt` чисто; `ProcessDecl` ∈ `Decl`/`TopLevelItem`, `StepDecl` — только `Pos()` |
| B. US1/US2 ч.1 (парсер + регресс) | US1, US2 | P1 🎯 | `онбординг.ladix` парсится с 0 ошибок (SC-004 частично); дубль атрибута/пустой блок/не-шаг → exact-match §PM-6.A (SC-001); канонические формы §PM-7 (SC-006); `выручка.ladix → 'когда'`; parse-тесты 002/004 зелёные |
| C. US1/US2/US3 ч.2 (семпроход + граница deferred) | US1, US2, US3 | P1 🎯 | уник шагов / `после` / `срок`-без-`исполнитель` / действие-вне-шага / `вернуть`-в-шаге / арность запуска → exact-match §PM-6.B/C (SC-002/003); действие в шаге + параметр в шаге + `после` назад + арность 1==1 → чисто; eval (`stmt.go`/`expr.go`) без диффа |
| D. Демо + синк-доки + CLI | US3 (CLI-граница) | финал | `ladix run онбординг.ladix` → код 1, `конструкция запустить процесс не поддерживается в этой версии` (SC-004); `ladix run выручка.ladix` → код 1, `неожиданный токен 'когда'` (SC-005); регрессы 001–004 зелёные; `go vet`/`gofmt`/`-race` чисто (SC-007) |

> **Зависимости фаз** (plan.md): A блокирует всё (узлы для парсера и семпрохода). B зависит от A (узлы для разбора). C зависит от A+B (узлы + разобранный AST). D зависит от C (семантика чиста → CLI-граница на рантайме).

---

## Phase A: Фундамент — AST-узлы процесса + синк union (Foundational, блокирует всё)

**Purpose**: два новых плоских AST-узла + вспомогательная структура позиций (§PM-2, D-1) + синк doc-комментария union. Соответствует фазе A plan.md.

**⚠️ CRITICAL**: ни одна задача фаз B–D не начинается до завершения фазы A. Узлы `StepLine`/`StepAttr{Kind}` НЕ вводятся (D-1); `AssignAction`/`CallAction`/`NotifyAction` (`ast/step.go`) и `RunProcessExpr` (`ast/expr.go`) **уже построены** в 003/004 — не дублировать (D-2/D-10, FR-009).

- [x] T001 **Тест+импл AST-узлов** в `src/internal/ast/process.go` (+тест `src/internal/ast/process_test.go`): `ProcessDecl{declBase; Name Ident; Params []Ident; Steps []*StepDecl}` — встраивает `declBase` первым полем → автоматически `Decl`/`TopLevelItem`, `Pos()`=токен `процесс`; `StepDecl{base; Name Ident; After []Ident; Assignee Expression; Deadline Expression; Attrs StepAttrPos; Body []Statement}` — встраивает **`base`** (НЕ `declBase`/`stmtBase`), реализует **только** `Pos()`=токен `шаг`; `StepAttrPos{AssigneePos Position; DeadlinePos Position}` — вспомогательная плоская структура, **НЕ** `Node` (нулевая `Position{}` ⟺ атрибут отсутствует); конструкторы `NewProcessDecl(pos, name, params, steps)` / `NewStepDecl(pos, name, after, assignee, deadline, attrs, body)` (дословно §PM-2, возвращают указатель, НЕ валидируют). Тест `TestProcessDeclPos`/`TestStepDeclPos` по образцу `ast/decl_test.go`: `Pos()` через `!=`, поля прямым доступом (`pd.Params[i].Name`, `pd.Steps[0].After[0].Name`, `sd.Attrs.AssigneePos`); компайл-тайм маркеры `var _ Decl = (*ProcessDecl)(nil)`, `var _ TopLevelItem = (*ProcessDecl)(nil)`, `var _ Node = (*StepDecl)(nil)` (StepDecl НЕ `Decl`/`Statement` — отсутствие маркеров); `процесс P:` без скобок → `Params==nil`; `Assignee==nil`/`Deadline==nil` при отсутствии атрибута. *(Guardrail 2, FR-008/009, §PM-2, data-model §1/§4, контракт ast-process.md)*
- [x] T002 [P] **Синк union `Decl`** в `src/internal/ast/node.go` (строка 24): обновить doc-комментарий «В подмножестве B единственная — FunctionDecl» (устарел ещё на 004) → «union: FunctionDecl | SourceDecl | MetricDecl | ProcessDecl». Интерфейс и маркеры НЕ меняются — только комментарий. *(Guardrail 3, FR-028, §PM-2)*

**Checkpoint (Гейт фазы A)**: `cd src && go build ./...`/`go vet ./...` чисто; AST-тесты зелёные; `ProcessDecl` участвует в union `Decl`, `StepDecl` достижим только через `ProcessDecl.Steps`.

---

## Phase B: US1/US2 ч.1 — парсер процессов + регресс (Priority: P1) 🎯 MVP

**Goal**: снять cut **только** с `KW_PROCESS`; `parseProcessDecl`/`parseStepDecl`/`parseAfterList` по образцу `parseFunctionDecl` (заголовок) + `parseMetricDecl` (блок с backstop); структурная диагностика §PM-6.A на переиспользовании; регресс `выручка.ladix → 'когда'`, `онбординг.ladix → позитив`.

**Independent Test**: подать процесс-программу (без триггеров) на разбор → ноль ошибок, канонические формы `ProcessDecl`/`StepDecl` (§PM-7 позитив); варианты с дублем атрибута / пустым блоком / не-шагом в блоке → exact-match §PM-6.A с позицией.

- [x] T003 [US1-P1] **Снять cut + ветвь диспетчера** в `src/internal/parser/parse_stmt.go` (+тесты в `src/internal/parser/parse_decl_test.go`): убрать из `isUnexpectedTopLevel` (строки 37-45) **только** `KW_PROCESS` (обновить doc-комментарий — `процесс` теперь парсится); `KW_WHEN`/`KW_VALUE`/`LBRACE`/`RBRACE` **остаются** отвергаемыми (D-6, триггеры — 007); добавить в `parseTopLevelItem` (строки 12-31) ветвь `if p.check(lexer.KW_PROCESS) { return p.parseProcessDecl() }` **перед** проверкой `isUnexpectedTopLevel` (рядом с `KW_FUNC`/`KW_SOURCE`/`KW_METRIC`). Тест-инварианты: top-level `когда …`/`значение …` по-прежнему → `неожиданный токен 'когда'`/`неожиданный токен 'значение'` (FR-003); `parseStepAction` (parse_stmt.go:86-107) и `parseRunProcess` (parse_expr.go:209-221) НЕ трогаются. *(Guardrail 1/4, FR-003/004/005, §PM-3, D-6)*
- [x] T004 [US1-P1] **`parseProcessDecl`** в `src/internal/parser/parse_decl.go` (+позитивные тесты `src/internal/parser/parse_decl_test.go`): зеркало `parseFunctionDecl` (заголовок, parse_decl.go:10-20) + `parseMetricDecl` (блок с backstop, :107-161): `advance()` (`процесс`, `Pos`); `expect(IDENT, "имя процесса")`; **опц. параметры** — `check(LPAREN)` → `advance`; `parseParamList()` (**переиспользовать**, :23-36); `expect(RPAREN, ")")`; иначе `Params=nil`; `expect(COLON, ":")`; блок через `openAttrBlock()` (:165-173; `false` → `msgEmptyBlock`, вернуть `ProcessDecl` с пустыми `Steps`); цикл `!DEDENT && !EOF`: `check(KW_STEP)` → `parseStepDecl()` в `steps`, иначе → `p.error(peek().Pos, msgUnexpected(peek())); break` + backstop прогресса (`before := p.pos`; не сдвинулись → `advance`); `expect(DEDENT, "конец блока")`. Тест: `процесс P:` (без скобок) и `процесс P(x, y):` → канонические `ProcessDecl` (параметры опц.). *(depends: T001, T003; Guardrail 5, FR-001/005/007, §PM-3, §PM-6.A)*
- [x] T005 [US1-P1] **`parseStepDecl` + `parseAfterList`** в `src/internal/parser/parse_decl.go` (+позитивные тесты `parse_decl_test.go`): `advance()` (`шаг`, `Pos`); `expect(IDENT, "имя шага")`; **`после`** — `check(KW_AFTER)` → `advance`; `parseAfterList()` в `After`; `expect(COLON, ":")`; `openAttrBlock()`; цикл `!DEDENT && !EOF` — диспетчер по ведущему токену строки: `KW_ASSIGNEE`/`KW_DEADLINE` → **StepAttr** (`attrTok := peek()`; дубль → см. T007; иначе `advance`, `expect(COLON, ":")`, `parseExpression()` → `Assignee`/`Deadline`, `Attrs.AssigneePos`/`DeadlinePos = toASTPos(attrTok.Pos)`, `expect(NEWLINE)`, `seen[lexeme]=true`); иначе → `parseStatement()` → в `Body` если `!=nil`; backstop прогресса; `expect(DEDENT, "конец блока")`. **`parseAfterList`** (новый хелпер): `StepAfter ::= "после" Ident ("," Ident)*` **без скобок** (отличие от `parseParamList`): цикл `expect(IDENT, "имя шага")` → `!ok` → `break`; `match(COMMA)` → продолжить, иначе стоп. **«Неизвестного атрибута» в шаге НЕТ** (любая не-`исполнитель`/`срок` строка — `Statement`). Тесты: `после` 0/1/N имён; атрибуты-только шаг (§PM-7 P2: `Assignee:StringLit`, `Deadline:DurationLit`, `Attrs:{AssigneePos≠0, DeadlinePos≠0}`, `Body:nil`); инвариант пэйринга `Attrs.*Pos.Line != 0` ⟺ атрибут присутствует. *(depends: T001, T004; Guardrail 6/7, FR-002/006, §PM-3, data-model §2)*
- [x] T006 [US2-P1] **Позитивные кейсы тела шага — действия и запуск (переиспользование форм)** в `src/internal/parser/parse_decl_test.go`: табличные тесты канонических форм §PM-7 (SC-006): P1 — `процесс P:`⏎`  шаг A:`⏎`    печать(1)` → `Steps:[StepDecl{Body:[ExpressionStmt]}]`; P3 — `присвоить y = 1` в теле A + `шаг B после A:` с `исполнитель:` → `Steps:[A{Body:[AssignAction]}, B{After:[A], Assignee:StringLit}]`; чередование attr/statement в одном шаге (атрибут после оператора синтаксически легален); `запустить процесс P("Петров")` в `пусть id = …` → `LetStmt{Value: RunProcessExpr}` (P4). Действия идут через **существующий** `parseStepAction`, запуск — через **существующий** `parseRunProcess`; формы `AssignAction`/`CallAction`/`NotifyAction`/`RunProcessExpr` НЕ меняются — задача только тестовая (фиксация переиспользования). *(depends: T005; Guardrail 4, FR-009, §PM-2/§PM-3, §PM-7, SC-006)*
- [x] T007 [US1-P1] **Негативы парсера — exact-match §PM-6.A** в `src/internal/parser/parse_decl_test.go` (+правка цикла атрибутов T005 в `parse_decl.go`): **дубль атрибута** (`исполнитель:` дважды) → `p.error(attrTok.Pos, msgDuplicateAttr(lexeme)); break` — **БАЙТ-идентично `parseMetricDecl:129-132`, `break`, НЕ `continue`** (D-8 — `continue` рассинхронил бы парсер); payload `атрибут 'исполнитель' уже задан`, позиция повторного атрибута; **пустой блок** процесса (`процесс P:` без шагов) и шага (`шаг A:` без строк) → `пустой блок не допускается, добавьте хотя бы один оператор`; **не-шаг в блоке процесса** (`процесс P:`⏎`  печать(1)`) → `неожиданный токен 'печать'` (ведущий токен строки, `peek().Pos` до `advance`); **`после` без имени** (`шаг A после :`) → SE-EXPECTED от `expect(IDENT, "имя шага")` (текст `msgExpected` — реестр 002, сверять с фактическим `parser/errors.go`, **НЕ** новый текст слоя); `After` пуст, восстановление штатное; **`foo: bar` в шаге** → `foo` как выражение-оператор, затем `:` — SE-UNEXPECTED (не реестр процессов); **вложенный `процесс`** в теле функции → SE-UNEXPECTED через путь выражения (FR-004, отдельной диагностики нет). Все три текста §PM-6.A — переиспользование `msgDuplicateAttr`/`msgEmptyBlock`/`msgUnexpected` (`parser/errors.go:29-31/17/45-47`); **ничего нового в `parser/errors.go` не добавляется**. *(depends: T005; Guardrail 6/8/20, FR-004/006/007, §PM-6.A / контракт diagnostics-process §DP-1, CP-5.2 N1–N4, SC-001)*
- [x] T008 [US1-P1] **РЕГРЕСС примеров** в `src/internal/parser/examples_test.go`: `TestDeclarativeExamplesUnexpected` (строки 35-54) — кейс `выручка.ladix` обновить ожидание `«неожиданный токен 'процесс'»` → **`«неожиданный токен 'когда'»`** (`процесс` теперь парсится, падение позже на триггере 007); кейс `онбординг.ladix` — **снять** из «unexpected»-набора и перенести в позитивные parse-тесты (парс с **нулём** ошибок — в `TestExamplesParseCleanSet` (строки 12-33) или отдельным позитивным кейсом). Пропуск любого из двух → красный тест (страховка). *(depends: T003–T005; Guardrail 19, FR-027, §PM-3, SC-004/SC-005, CP-5.3 R1–R3)*

**Checkpoint (Гейт фазы B)**: `онбординг.ladix` парсится с 0 ошибок; дубль атрибута / пустой блок / не-шаг в блоке → exact-match §PM-6.A с позицией (SC-001); табличные кейсы дают канонические формы (SC-006); `выручка.ladix → 'когда'`; **все** parse-тесты 002/004 зелёные.

---

## Phase C: US1/US2/US3 ч.2 — семпроход + граница deferred (Priority: P1) 🎯 MVP

**Goal**: реестр `i.processes`; регистрация в Шаге 1 (общий namespace, D-5); Шаг 1c `checkProcessDecl` (уник шагов → резолв `после` → `срок`-без-`исполнитель` → `analyzeStep`); прокидка `inStep`; контекст-гард действий; `вернуть`-в-шаге; `checkRunProcess` (арность, D-10); eval **НЕ трогать** (§PM-5).

**Independent Test**: процесс с действиями/параметрами/`после`-назад/корректной арностью запуска → семантика чиста; дубль шага / `после` вперёд/неизвестный / `срок`-без-`исполнитель` / действие-вне-шага / `вернуть`-в-шаге / 4 ошибки запуска → exact-match §PM-6.B/C.

- [x] T009 [US1-P1] **Реестр + регистрация процесса в Шаге 1** в `src/internal/eval/interpreter.go` и `src/internal/eval/analyze.go` (+exact-match тесты `src/internal/eval/analyze_decl_test.go`): поле `processes map[string]*ast.ProcessDecl` в `Interpreter` (рядом с `sources`/`metrics`, ~:31) + `processes: make(map[string]*ast.ProcessDecl)` в `NewInterpreter` (~:67); `case *ast.ProcessDecl` в **оба** type-switch'а Шага 1 `Analyze`: первый (:29-38) — `name, pos = d.Name.Name, d.Name.Pos()` (НЕ `isFunc`; общий глобальный namespace, D-5); второй (:49-62) — `i.processes[name]=d` + `i.checkReservedDeclName(name, d.Pos())` (:141-151, переиспользуется). Exact-match тесты: повтор имени процесс↔процесс и процесс↔метрика (`процесс выручка_месяца:` после метрики) → `'выручка_месяца' уже объявлено в строке N` (CP-5.2 N14); имя процесса = встроенная → `имя '<имя>' зарезервировано встроенной функцией`; = период → `имя '<имя>' зарезервировано предопределённым периодом`. *(depends: фаза B; Guardrail 9/10, FR-010, §PM-4, §PM-6.B / §DP-2)*
- [x] T010 [US2-P1] **Прокинуть `inStep bool`** в `src/internal/eval/analyze.go`: добавить параметр `inStep` в сигнатуры `checkStmts` (:222), `checkStmt` (:231), `checkElse` (:281) — все рекурсивные вызовы передают полученный `inStep` **без изменения**; вызовы из `analyzeArea` (:166, глобаль/функции) → `inStep=false`. **`checkExpr` НЕ трогать** (он `inStep` не принимает; иначе сломается арность вложенных вызовов вне шага — риск (в)). Чистый рефакторинг сигнатур: поведенческих изменений нет — гейт = компиляция + **все существующие eval-тесты 003/004 зелёные**. *(depends: T009; Guardrail 13, §PM-4)*
- [x] T011 [US1-P1] **Шаг 1c — `checkProcessDecl` (структурные проверки 1–3)** в `src/internal/eval/analyze.go` (+exact-match тесты `analyze_decl_test.go`): после Шага 1b `checkMetricDecl` (:68-76) — цикл по `prog.Items`: `pd, ok := item.(*ast.ProcessDecl)` → `i.checkProcessDecl(pd)`. Внутри (fail-fast): **(1)** уникальность шагов — `имя → строка` первого; повтор → `semErr(step.Name.Pos(), "шаг '<имя>' уже объявлен в строке N")`; **(2)** резолв `после` (D-4, валидатор): `X` не среди шагов → `semErr(Xident.Pos(), "шаг '<S>' после '<X>', но шаг '<X>' не объявлен")`; `X` на индексе `j>=i` (позже/сам на себя) → `semErr(Xident.Pos(), "шаг '<S>' после '<X>', но '<X>' объявлен позже")` (ацикличность автоматически от `j<i`; топосорт НЕ делать); **(3)** `срок`-без-`исполнитель`: `Attrs.DeadlinePos.Line != 0 && Attrs.AssigneePos.Line == 0` → `semErr(step.Attrs.DeadlinePos, "шаг '<имя>': срок без исполнитель не имеет эффекта")` — позиция на строке `срок:`, НЕ на начале шага. Exact-match тесты (CP-5.2 N5–N8): `шаг 'A' уже объявлен в строке N`; `шаг 'A' после 'B', но шаг 'B' не объявлен`; `шаг 'A' после 'B', но 'B' объявлен позже` (forward И self-ссылка); `шаг 'A': срок без исполнитель не имеет эффекта` (+ проверка позиции `DeadlinePos`). Позитив: `после A` назад → чисто; `срок`+`исполнитель` вместе → чисто. *(depends: T009; Guardrail 11, FR-011/012/013, §PM-4, §PM-6.B / §DP-2, SC-002)*
- [x] T012 [US2-P1] **`analyzeStep` + подключение как шаг (4)** в `src/internal/eval/analyze.go` (+тесты `analyze_decl_test.go`): зеркало `analyzeArea` (:156-167), отличия (D-12): `vars` **засевается параметрами процесса** (`pd.Params` → `vars[p.Name]=true`), `letLine` параметрами **НЕ** засевается; `collectVars(step.Body, letLine={}, vars)` (:173-205, переиспользуется); `checkStmts(step.Body, vars, false /*inFunction*/, true /*inStep*/, 0 /*loopDepth*/)`. Вызов из `checkProcessDecl` шагом (4) для каждого шага. Тесты: чтение параметра `x` в теле шага → **без** «не объявлено» (US2 acceptance 4); `пусть x` с именем параметра процесса в шаге → **разрешён** (теняет, §6.4 — отличие от тела функции, диагностики нет); дубль шаг-локальных `пусть`/`для` → общий текст `'<имя>' уже объявлено в строке N` (через `collectVars`, не новый текст слоя). *(depends: T010, T011; Guardrail 12, FR-016, §PM-4, D-12)*
- [x] T013 [US2-P1] **Контекст-гард действий** в `src/internal/eval/analyze.go` (заменить :275-276; +exact-match тесты `analyze_decl_test.go`): `case *ast.AssignAction, *ast.CallAction, *ast.NotifyAction:` — заменить безусловный `return i.deferredConstruct(st)` на: `if !inStep { return semErr(st.Pos(), fmt.Sprintf("действие '%s' допустимо только в шаге процесса", constructName(st))) }; return nil`. `constructName` (`interpreter.go:150-164`) уже даёт `присвоить`/`вызвать`/`уведомить` — НЕ трогать. **В шаге payload (`Args`/`Value`) НЕ обходится** (`return nil`; резолв/арность/deferred аргументов — рантайму 006, риск (д)). Exact-match тесты: top-level `присвоить x = 1` → `действие 'присвоить' допустимо только в шаге процесса` (CP-5.2 N9); `вызвать`/`уведомить` в теле функции → тот же текст с `'вызвать'`/`'уведомить'`; позитив: `присвоить y = 1` + `уведомить ИТ("…")` в шаге → чисто (US2 acceptance 1); **payload-инвариант**: действие в шаге с payload, который в обходимой позиции дал бы семантическую ошибку (напр. вызов deferred-builtin в аргументе) → **семантика чиста** (точка трения 5). **НЕ писать недостижимый рантайм-тест «действие в шаге → deferred»** (тело шага в рантайме 005 не исполняется — точка трения 2). *(depends: T010, T012; Guardrail 14, FR-014/022, §PM-4/§PM-5, §PM-6.B / §DP-2)*
- [x] T014 [US2-P1] **`вернуть` в шаге** в `src/internal/eval/analyze.go` (обновить :239-242 `case *ast.ReturnStmt`; +exact-match тесты `analyze_decl_test.go`): при `!inFunction` — `msg := "'вернуть' допустимо только внутри функции"; if inStep { msg += "; в шаге процесса используйте 'присвоить'" }; return semErr(st.Pos(), msg)`. Exact-match: `вернуть 1` в шаге → `'вернуть' допустимо только внутри функции; в шаге процесса используйте 'присвоить'` (полная двухконтекстная форма, CP-5.2 N10); регресс: `вернуть` на top-level вне шага → **только** базовый текст `'вернуть' допустимо только внутри функции` (003, без суффикса); `прервать`/`продолжить` внутри `пока`/`для` в шаге → чисто (по `loopDepth`, не трогать); вне цикла в шаге → общий текст 003. *(depends: T010, T012; Guardrail 15, FR-015, §PM-6.B / §DP-2)*
- [x] T015 [US3-P1] **`checkRunProcess` — арность запуска** в `src/internal/eval/analyze.go` (заменить :330-331; +exact-match тесты `analyze_decl_test.go`): `case *ast.RunProcessExpr:` в `checkExpr` → `return i.checkRunProcess(ex, vars)`. Новый метод: сначала `checkExpr` каждого `r.Args` (args-first, fail-fast — как `checkCall`); `name := r.Process.Name`; резолв **ТОЛЬКО** против `i.processes` (НЕ `vars`, НЕ `builtins` — риск (г), точка трения 4): `pd` найден → арность `len(r.Args) != len(pd.Params)` → `semErr(r.Pos(), "'<P>' принимает N аргументов, передано M")`, иначе `nil`; иначе `i.funcs[name]` → `semErr(r.Pos(), "'<P>' — функция, не процесс")`; иначе `i.metrics[name]`/`i.sources[name]` → `semErr(r.Pos(), "'<P>' — не процесс")`; иначе → `semErr(r.Pos(), "процесс '<P>' не объявлен")`. `checkRunProcess` **НЕ принимает `inStep`** (работает в любой области — реестр готов с Шага 1, точка трения 3). `DurationLit` (:332-333) **НЕ трогать**. Exact-match тесты (CP-5.2 N11–N13): `процесс 'Q' не объявлен`; `'f' — функция, не процесс`; `'P' принимает 1 аргументов, передано 0`; `'<P>' — не процесс` (имя метрики И имя источника); **builtin-инвариант**: `запустить процесс печать()` → `процесс 'печать' не объявлен` (ветку builtins НЕ добавлять); **args-first**: ошибка в аргументе возвращается до проверки имени/арности. Позитив: `пусть id = запустить процесс P("Петров")` при 1 параметре → чисто (US3 acceptance 1); запуск из тела функции и тела шага → резолв/арность работают одинаково. *(depends: T009, T010; Guardrail 16, FR-018, §PM-4, §PM-6.C / §DP-3, SC-003)*
- [x] T016 [US3-P1] **Инварианты границы deferred — eval НЕ тронут** (тесты в `src/internal/eval/analyze_decl_test.go`; правок кода НЕТ): (а) `срок: 2дн` в шаге → семантика **чиста** (атрибуты шага не обходятся, D-11/FR-017 — `DurationLit` в `срок:` не достигает deferred-проверки); (б) `пусть x = 2дн` в обходимой позиции → `конструкция литерал длительности не поддерживается в этой версии` (003/004-поведение БЕЗ изменений); (в) вызов `статус_процесса(1)` → `функция 'статус_процесса' не поддерживается в этой версии` (D-9; механизм deferred-builtin НЕ меняется, `TestBuiltinDeferredAll` НЕ трогается); (г) процедурный чек: `src/internal/eval/stmt.go` (:63-64) и `src/internal/eval/expr.go` (:48-51) — **нулевой дифф** (`git diff` пуст по этим файлам); `deferredConstruct`/`constructName` (`interpreter.go:146-164`) без изменений. **НЕ писать** рантайм-тест «действие в шаге → deferred» — недостижим в 005 (точка трения 2). *(depends: T013, T015; Guardrail 17, FR-017/020/021/024/025, §PM-5, §DP-4)*

**Checkpoint (Гейт фазы C)**: уник шагов / `после` вперёд/неизвестный / `срок`-без-`исполнитель` / действие-вне-шага / `вернуть`-в-шаге / арность запуска (все 4 текста) → exact-match §PM-6.B/C (SC-002/SC-003); действие в шаге + чтение параметра + `после` назад + арность 1==1 → семантика чиста (US2/US3 acceptance); eval-файлы без диффа; все тесты 003/004 зелёные.

---

## Phase D: Демо + синк-доки + сквозной CLI (финал; US3 — CLI-граница)

**Purpose**: подрезать флагман-демо (D-9); CLI-приёмка наблюдаемой рантайм-границы (`запустить процесс` top-level); синк `MANIFEST.md` и зоны якоря; конституционные гейты. CLI **НЕ расширяется** (D-7: нет `ladix check`/`start`/`complete`; `src/cmd/ladix/main.go` БЕЗ изменений, FR-026).

- [ ] T017 [US3-P1] **Подрезать демо `examples/онбординг.ladix`** (+тест семантической чистоты в `src/internal/eval/analyze_decl_test.go`): убрать последнюю строку `печать("статус:", статус_процесса(id))` (строка 16) — `статус_процесса` остаётся deferred-builtin (D-9), её вызов делает семантику не чистой и валит SC-004; синхронизировать комментарий строки 13 (добавить «(исполнение — рантайм-deferred до 006)») — файл должен быть байт-идентичен листингу §PM-7/CP-4. Целевой вид файла — ДОСЛОВНО §PM-7 (3 шага: `завести_доступы` с `присвоить`+`уведомить`; `провести_встречу`/`закрыть_адаптацию` с `после`+`исполнитель`+`срок`; top-level `пусть id = запустить процесс онбординг("Петров")` + `печать`). Тест: файл (через `filepath.Join("..","..","..","examples","онбординг.ladix")`) парсится с нулём ошибок И `Analyze` → `nil` (семантика чиста, SC-004 частично; CP-4 приёмка узлов). *(depends: фаза C; Guardrail 18, FR-024, §PM-7, D-9, SC-004)*
- [ ] T018 [US3-P1] **CLI-приёмка через `realMain`** в `src/cmd/ladix/main_test.go` (правок `src/cmd/ladix/main.go` НЕТ — FR-026/D-7): конвенции 003/004 — белый ящик `package main`, `realMain([]string{"run", …}, &out, &errBuf)`, сверка тройки код/stdout/stderr: (1) `run онбординг.ladix` → **код 1**, stderr содержит двухстрочную ошибку с payload `конструкция запустить процесс не поддерживается в этой версии` (рантайм-граница ПОСЛЕ чистых парс+семантики — SC-004, CP-5.2 N15); (2) `run выручка.ladix` → **код 1**, парс-ошибка `неожиданный токен 'когда'` (SC-005); (3) программа, только **объявляющая** процесс (без top-level `запустить процесс`) → **код 0** (рантайм отрабатывает, `ProcessDecl` пропускается `Run()` — FR-023). **НЕ тестировать** рантайм действий в шаге (недостижимо, §PM-5/CP-2). *(depends: T017; Guardrail 17, FR-021/023/025/026, §PM-5, CP-2/CP-3/CP-5.4, SC-004/SC-005)*
- [ ] T019 [P] **Синк `examples/MANIFEST.md`**: расщепить совмещённую строку (:54-79, особенно :58-60 и :18-19): (а) `онбординг.ladix` теперь **парсится и проходит семантику чисто**, `ladix run` → код 1 в **рантайме** на `запустить процесс` (исполнение — 006); (б) `выручка.ladix` **остаётся** парс-ошибкой код 1, ожидаемый токен сдвигается `источник`/`процесс` → **`когда`** (триггеры — 007). Оцениваемый чеклист не трогается (обе — демо, не golden). *(depends: T017, T018; Guardrail 21, FR-028, §PM-7, CP-6)*
- [ ] T020 [P] **Синк зоны якоря `SPEC.md`/`ARCHITECTURE.md`** (репрезентативные тексты, при расхождении побеждает §PM-6 — §DP-6): `SPEC §13.4` — пополнить Семантическую-строку текстами §PM-6.B/C; `§7.4` — заменить тексты запуска на §PM-6.C (вместо `…параметров… в строке L`/`…не найдено.`); `§7.3` — текст `вернуть`-в-шаге; `§11.4`/`§12` — терминология; `ARCHITECTURE §4.4/§4.8` — **верифицировать** (по якорю синк форм уже выполнен, data-model «Дрейф документации»; править только при фактическом расхождении с кодом). *(depends: фаза C; Guardrail 21, FR-028, §PM-6 / §DP-6)*
- [ ] T021 **Конституционные гейты + регрессы (SC-007)**: `cd src && go build ./...` и `go vet ./...` без замечаний; `gofmt -l .` без диффа; `go test -race ./...` зелёное, включая **все** регресс-тесты 001/002/003/004 (FR-027; `TestBuiltinDeferredAll` нетронут); финальная сверка: все exact-match тексты тестов байт-идентичны [contracts/diagnostics-process.md](./contracts/diagnostics-process.md) §DP-1/§DP-2/§DP-3 (16 строк §PM-6.A/B/C) и `eval-model §8.3` (§DP-4); `errors.ОшибкаПроцесса` в кодовой базе ОТСУТСТВУЕТ (D-3); новых пакетов/CLI-команд НЕТ (D-7). *(depends: T017–T020; Guardrail 20, FR-019/026/027/028, SC-001/SC-002/SC-003/SC-007)*

**Checkpoint (Гейт фазы D)**: `ladix run онбординг.ladix` → код 1, рантайм-текст `конструкция запустить процесс не поддерживается в этой версии`; `ladix run выручка.ladix` → код 1, `неожиданный токен 'когда'`; процесс-онли программа → код 0; все регрессы 001–004 зелёные; `go vet`/`gofmt`/`-race` чисто.

---

## Dependencies & Execution Order

### Зависимости фаз

- **Фаза A (Foundational)**: без зависимостей — старт немедленно. **БЛОКИРУЕТ всё** (узлы для парсера и семпрохода).
- **Фаза B (US1/US2 ч.1)**: зависит от A (узлы для разбора).
- **Фаза C (US1/US2/US3 ч.2)**: зависит от A+B (узлы + разобранный AST для валидации).
- **Фаза D (демо+CLI+доки)**: зависит от C (семантика чиста → CLI-граница на рантайме).

### Ключевые внутрифазовые зависимости

- T001 блокирует T004/T005 (узлы для конструкторов); T002 — параллелен T001 (другой файл).
- T003 → T004 → T005 — последовательны (`parse_stmt.go` → заголовок процесса → шаг; один файл `parse_decl.go` у T004/T005/T007); T006/T007 — после T005; T008 — после T003–T005 (нужен парсящийся `процесс`).
- T009 → T010 → T011 → T012 — последовательны (один файл `analyze.go`; реестр → сигнатуры `inStep` → структурные проверки → тела шагов); T013/T014 — после T010+T012 (нужны `inStep` и `analyzeStep`-контекст); T015 — после T009+T010 (реестр; независим от T011–T014 логически, но тот же файл — последовательно); T016 — после T013+T015 (фиксирует границу).
- T017 → T018 (CLI-кейс SC-004 требует чистой семантики демо); T019 — после T017/T018 (описывает их результат); T020 — после фазы C (тексты финальны); T021 — последний (финальный гейт).

### Within Each Phase

- Табличные/exact-match тесты ПИШУТСЯ В ПАРЕ с импл-задачей и должны падать до кода (tests-first, конституция VI).
- AST → парсер → семпроход → демо/CLI/доки. Фаза завершается своим гейтом до перехода к следующей.

---

## Parallel Opportunities

- **Фаза A**: T001 (`ast/process.go`) и T002 (`ast/node.go`) — параллельно (разные файлы).
- **Фаза B**: в основном последовательна (общие файлы `parse_decl.go`/`parse_decl_test.go`); T008 (`examples_test.go`) можно вести параллельно с T006/T007 после T005.
- **Фаза C**: последовательна (общий `analyze.go`) — параллелить не пытаться (конфликт одного файла).
- **Фаза D**: T019 (`examples/MANIFEST.md`) и T020 (`SPEC.md`/`ARCHITECTURE.md`) — параллельно между собой (разные doc-файлы); T020 — также параллельно T017/T018 (зависит только от фазы C); T019 — строго после T017/T018.

### Parallel Example: Phase A

```bash
# Разные файлы пакета ast — параллельно:
Task: "ProcessDecl/StepDecl/StepAttrPos + конструкторы + тесты в src/internal/ast/process.go"
Task: "Doc-комментарий union Decl в src/internal/ast/node.go:24"
```

---

## Implementation Strategy

### MVP First (все три истории — P1)

1. Фаза A (фундамент — узлы AST; быстрая, 2 задачи).
2. Фаза B (парсер: `процесс`/`шаг` парсятся, диагностика §PM-6.A, регрессы примеров).
3. Фаза C (семпроход: полные acceptance US1/US2/US3 на уровне парс+семантика; eval не тронут).
4. **STOP & VALIDATE**: exact-match §PM-6.A/B/C (16 строк) + позитивы §PM-7 + тесты 001–004 зелёные.
5. Фаза D (наблюдаемая CLI-граница + демо + синк доков) — завершение DoD (SC-004/005/007).

### Incremental Delivery

1. A → узлы готовы (компиляция, маркеры union).
2. + B → процессы парсятся (канонические формы §PM-7, `онбординг.ladix` 0 ошибок парса).
3. + C → процессы валидируются (SC-001/002/003); семантика демо — после подрезки в D (T017).
4. + D → демо подрезано (семантика чиста) + CLI/доки (SC-004/005/007) — фича закрыта.

---

## Notes

- **Tests-first обязателен** (конституция VI): табличные + негативные `*_test.go` пишутся в паре с кодом и падают до реализации; co-located; вывод через инжектированный `out` (`bytes.Buffer`).
- **Тексты ошибок — ДОСЛОВНО** из [contracts/diagnostics-process.md](./contracts/diagnostics-process.md) (§DP-1..§DP-4 = §PM-6.A/B/C/D): payload без завершающей точки; `'…'` — идентификаторы/ключевые слова; `N`/`M` с 1; позиция = строка/колонка в рунах. Переформулировать запрещено (конституция VIII); при расхождении побеждает якорь `docs/process-model.md`.
- **Без новых типов ошибок** (D-3): только `ОшибкаРазбора` (парсер, `p.error`) и `СемантическаяОшибка` (`semErr`); `errors.ОшибкаПроцесса` НЕ вводится (категория исполнения — 006).
- **Без новых пакетов/CLI/встроенных**: `internal/engine`/`internal/store`/`ladix check`/`start`/`complete`/`serve` НЕ заводятся (D-7/§PM-8); 0 новых builtins; `cmd/ladix/main.go` и `internal/{lexer,errors,value}` БЕЗ изменений.
- **Границы scope (НЕ делать)**: триггеры (`KW_WHEN`/`KW_VALUE` остаются отвергаемыми; `TriggerDecl`/… — 007); узлы `StepLine`/`StepAttr{Kind}` (D-1); топосорт/переупорядочивание `после` (D-4); дубль параметра процесса (D-13); declaredness чтений/процесс-scope (006, FR-020); механизм deferred-builtin (D-9) и `DurationLit`-deferred (D-11) НЕ меняются; конструктор `Длительность` НЕ вводится. Решения D-1…D-13 НЕ переоткрываются.
- **Граница deferred §PM-5 — сквозной контроль**: eval (`stmt.go:63-64`, `expr.go:48-51`) с нулевым диффом; тело шага в рантайме 005 НЕ исполняется → недостижимый рантайм-тест «действие в шаге → deferred» НЕ писать; единственная наблюдаемая рантайм-граница = top-level `запустить процесс`.
- Все `go`-команды из `cd src`; `examples/` — сиблинг `src/` на корне репозитория; коммит после каждой задачи или логической группы; останов на любом checkpoint для автономной валидации фазы.

---

**Итого: 21 задача** (T001–T021). По фазам: **A** — 2 (T001–T002), **B** — 6 (T003–T008), **C** — 8 (T009–T016), **D** — 5 (T017–T021).
