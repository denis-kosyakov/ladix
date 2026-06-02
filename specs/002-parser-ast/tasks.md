# Tasks: Парсер + AST Ladix (ручной recursive descent, подмножество B)

**Input**: Design documents from `/specs/002-parser-ast/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/ast.md, contracts/syntax-errors.md, quickstart.md

**Tests**: ВКЛЮЧЕНЫ как обязательные. Конституция VI + FR-028 предписывают tests-first: табличные «вход → AST»
и golden «вход → (строка, колонка, текст)» — часть каждой задачи. Тесты пишутся ПЕРЕД реализацией и обязаны
падать до неё.

**Organization**: задачи сгруппированы по user stories (spec.md). Фазы упорядочены по приоритетам US
(plan.md «Phasing»). US5 (диагностика+восстановление) — **пронизывающая**: каждая фаза эмитит свои ошибки,
US5 финализирует полный golden-каталог и многоошибочное накопление.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: можно выполнять параллельно (разные файлы, нет зависимостей от незавершённых задач)
- **[Story]**: к какой user story относится задача (US1–US5); Setup/Foundational/Polish — без метки
- Все пути — относительно корня репозитория; корень Go-модуля — `src/` (S1), пакеты в `src/internal/...`

## Path Conventions

- Пакеты: `src/internal/ast/`, `src/internal/parser/`, дополнение `src/internal/errors/`
- Тесты co-located (`*_test.go` рядом с кодом) — идиома Go + конституция VI (S2)
- Учебные примеры — `examples/*.ladix` в корне репозитория; из пакета `parser` относительный путь
  `../../../examples/<имя>.ladix`
- Гейты качества (везде): `gofmt -l .` пусто, `go vet ./...` и `go build ./...` без замечаний (FR-027, SC-007)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: каркас двух новых пакетов и фиксация чистой базовой линии перед изменениями.

- [X] T001 [P] Создать пакет `src/internal/ast/doc.go` с пакет-комментарием: листовой пакет AST подмножества B, несёт ЛОКАЛЬНУЮ `Position`, НЕ импортирует `internal/errors` (D1, guardrail 1)
- [X] T002 [P] Создать пакет `src/internal/parser/doc.go` с пакет-комментарием: ручной recursive-descent разборщик, граф `parser → {ast, errors, lexer}`, без пакет-уровневого состояния (FR-029, guardrail 9)
- [X] T003 Зафиксировать базовую линию: из `src/` прогнать `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...` — всё зелёное (лексер 001 не сломан) до начала работ

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: каркас, на который опираются ВСЕ user stories — типы `ast` (Position/интерфейсы/операторы/корень `Program`), `errors.ParseError`, шаблоны 7 текстов ошибок и сам `Parser` (курсор, `peek`/`advance`/`expect`, регистрация ошибок, конвертер позиций).

**⚠️ CRITICAL**: ни одна US не начинается до завершения этой фазы.

**Гейт фазы**: пустой/только-`EOF` ввод → `Program{Items:[], EOFPos}`; `ParseError.Error()` даёт канонический двухстрочный текст; все 7 строителей текстов дают дословный текст контракта.

### Тесты (пишутся первыми, обязаны падать)

- [X] T004 [P] Табличный тест `src/internal/ast/op_test.go`: `BinOp`(14)/`CompOp`(6)/`UnOp`(2) `String()` и предикат принадлежности `CompOp` подмножеству `BinOp` (D3, contracts/ast.md §C-3)
- [X] T005 [P] Тест `src/internal/errors/parserror_test.go`: `ParseError.Error()` — двухстрочный канон `Ошибка в строке N, колонка M:`+`\n`+`Msg`; разворачивание `errors.As(&ParseError{})`; складывание в существующий `ErrorList` через `Add(error)` (FR-023, data-model §10)
- [X] T006 [P] Тест `src/internal/parser/errors_test.go`: каждый из 7 строителей текста даёт ДОСЛОВНО текст из contracts/syntax-errors.md (3 канона §13.4 байт-в-байт + 4 эталона), включая псевдо-лексемы виртуальных токенов (`NEWLINE`→`конец строки`, `INDENT`→`увеличение отступа`, `DEDENT`→`конец блока`, `EOF`→`конец файла`)

### Реализация

- [X] T007 [P] `src/internal/ast/position.go`: локальный `Position{Line, Col int}` (1-based, руны); БЕЗ импорта `internal/errors` (D1, FR-001, data-model §1)
- [X] T008 [P] `src/internal/ast/op.go`: единый enum `BinOp` (14: `или и + - * / // % == != < <= > >=`), `CompOp` как подмножество `BinOp` (6 сравнений; `IsComparison()` или defined type `type CompOp BinOp`, без дублирования констант; НЕ type-alias `= BinOp` — принял бы любой `BinOp`), `UnOp` (2: `не`, унарный `-`); `String()` у всех трёх (D3, FR-004; зависит от T004)
- [X] T009 `src/internal/ast/node.go`: интерфейс `Node{Pos() Position}`, встраиваемая база `base{position Position}` с `Pos()`, маркер-подынтерфейсы `Statement`/`Expression`/`Decl`/`TopLevelItem` (sum-type через пустые маркер-методы; FR-001, data-model §2; зависит от T007)
- [X] T010 `src/internal/ast/block.go`: корень `Program{Items []TopLevelItem; EOFPos Position}` (узел `Block` добавляется в US3); FR-007, data-model §3 (зависит от T009)
- [X] T011 [P] `src/internal/errors/parserror.go`: `ParseError{Pos Position; Msg string}` + `Error()` → тот же двухстрочный канон, что у `LexError`; складывается в неизменённый `ErrorList` (правок `aggregate.go` НЕ делать — guardrail 8; FR-023; зависит от T005)
- [X] T012 [P] `src/internal/parser/errors.go`: строители текстов 7 кодов SE (3 канона дословно из SPEC §13.4 + 4 эталона из contracts/syntax-errors.md) + helper отображения лексемы токена (реальная лексема либо псевдо-лексема виртуального токена) для `<лексема>`/`<X>` (FR-024, contracts/syntax-errors.md; зависит от T006)
- [X] T013 `src/internal/parser/pos.go`: конвертер `errors.Position → ast.Position` (D1: helper живёт в `parser`, НЕ в `ast`; FR-029; зависит от T007)
- [X] T014 Тест `src/internal/parser/parser_test.go`: пустой и только-`EOF` ввод → `Program{Items:[], EOFPos=поз. EOF}`; базовое поведение `peek`/`advance`/`check`; `expect` при несовпадении эмитит SE-EXPECTED в `ErrorList` (FR-007, SC-006; пишется перед T015)
- [X] T015 `src/internal/parser/parser.go`: структура `Parser` + явный конструктор (`tokens []lexer.Token`, опц. `*errors.ErrorList` — если nil, создаёт свой), курсор по слайсу, `peek`/`advance`/`check`/`expect(type, ожид.лексема)` (mismatch → SE-EXPECTED через T012), регистрация ошибки `(*Parser).error(pos, msg)` в `ErrorList`, флаг подавления panic-mode (stub; полная синхронизация — US5), точка входа `Parse() *ast.Program` (top-level-цикл — заглушка до `EOF`, фиксирует `EOFPos`); FR-029, data-model §12 (зависит от T009/T010/T011/T012/T013)

**Checkpoint**: каркас готов — реализация user stories может начинаться.

---

## Phase 3: User Story 1 — Выражения с приоритетами (Priority: P1) 🎯 MVP

**Goal**: парсер строит AST выражений, точно отражающее каскад приоритетов и лево-ассоциативность (SPEC §5): литералы, унарные/бинарные операторы, постфикс-цепочка, список, свёртка группировки; диапазон int64 у `IntLit`; запрет цепочечных сравнений.

**Independent Test**: таблица выражений → эталонный AST; инвариант `2 + 3 * 4` → `BinaryExpr(+, 2, BinaryExpr(*, 3, 4))`; лево-ассоциативность `a - b - c` → `((a-b)-c)`; `1 < x < 10` → SE-CHAIN; литерал-переполнение → SE-INT-RANGE.

### Тесты (пишутся первыми, обязаны падать)

- [X] T016 [P] [US1] `src/internal/ast/expr_test.go`: конструкторы `BinaryExpr`/`UnaryExpr`/`CallExpr`/`IndexExpr`/`FieldExpr` и конвенция `Pos()` (оператор для Binary/Unary; `Callee` для Call; `Target` для Index/Field) — D4, contracts/ast.md §C-4, SC-004
- [X] T017 [P] [US1] `src/internal/ast/literal_test.go`: конструкторы `IntLit`/`FloatLit`/`StringLit`/`BoolLit`/`NoneLit`/`DurationLit`/`ListLit`/`Ident`, `Pos()` = свой токен; гетерогенный и пустой `ListLit` (data-model §8)
- [X] T018 [US1] `src/internal/parser/parse_expr_test.go`: таблица приоритетов из contracts/ast.md §C-2 (`2+3*4`, `(2+3)*4` со свёрткой, `a-b-c`, `10/2/5`, `не x и y`, `-5` без свёртки знака, `x > -10 и x < 0`, постфикс `данные[i].поле(1,2,)`, `[1,"две",истина,]`, `[]`); golden SE-CHAIN `сравнения нельзя сцеплять, используйте 'и': 1 < x и x < 10` на позиции второго `CompOp` (SC-002, FR-018/FR-019)
- [X] T019 [US1] `src/internal/parser/literals_test.go`: `IntLit` на границе int64; golden SE-INT-RANGE `целочисленный литерал вне диапазона типа Целое '<лексема>'` (30 девяток; `9223372036854775808` для `MinInt64`-кейса) на первой руне литерала; `Float/String/Bool/Duration` берут готовое значение из `Token.Value` (FR-021/FR-022, SC-003)

### Реализация

- [X] T020 [P] [US1] `src/internal/ast/expr.go`: `BinaryExpr{Op BinOp; Left, Right}`, `UnaryExpr{Op UnOp; Operand}`, `CallExpr{Callee; Args}`, `IndexExpr{Target; Index}`, `FieldExpr{Target; Field Ident}` с `Pos()` по D4 (`RunProcessExpr` — в US4); data-model §7
- [X] T021 [P] [US1] `src/internal/ast/literal.go`: `IntLit`, `FloatLit`, `StringLit`, `BoolLit`, `NoneLit`, `DurationLit`, `ListLit{Elements []Expression}`, `Ident`; реализуют `Expression`; `Pos()` = свой токен (data-model §8)
- [X] T022 [US1] `src/internal/parser/literals.go`: сборка `IntLit` через `strconv.ParseInt(Token.Lexeme, 10, 64)` с проверкой диапазона → при `ErrRange` накапливаемая SE-INT-RANGE (узел всё равно создаётся, чтобы разбор продолжился, D2); `Float/String/Bool/None/Duration` из `Token.Value`/`Lexeme` (диапазон `DurationLit` НЕ проверяется); `ListLit` (висящая запятая, гетерогенность, `[]`); `Ident` (FR-021/FR-022; зависит от T021)
- [X] T023 [US1] `src/internal/parser/parse_expr.go`: каскад `parseLogicOr → parseLogicAnd → parseLogicNot → parseComparison → parseAdditive → parseMultiplicative → parseUnary → parsePostfix → parsePrimary`; лево-ассоциативность циклом, унарные `не`/`-` рекурсией; `Comparison` ≤ одного `CompOp` (второй подряд → SE-CHAIN); постфикс-цепочка `Call`/`Index`/`Field` навешивается лево-ассоциативно (висящая запятая в `ArgList`); `( E )` сворачивается без узла `GroupExpr` (D5); `parsePrimary` зовёт строители из `literals.go` (FR-018/FR-019/FR-020/FR-006; зависит от T020/T022)

**Checkpoint**: выражения строятся в корректное дерево; SE-CHAIN/SE-INT-RANGE воспроизводятся.

---

## Phase 4: User Story 2 — Переменные, присваивание, печать, каркас программы (Priority: P1)

**Goal**: `Program` из top-level-элементов, завершающаяся ровно на `EOF`; `LetStmt`/`AssignStmt`/`ExpressionStmt`; `печать(...)` — обычный `CallExpr`; lvalue присваивания — только `Ident`.

**Independent Test**: программа из `пусть`/присваивания/`печать(...)` → эталонный `Program.Items`; завершение на `EOF` (`EOFPos` зафиксирован); `examples/hello.ladix` и `examples/арифметика.ladix` parse-clean; `x.поле = 5` → SE-ASSIGN-TARGET.

### Тесты (пишутся первыми, обязаны падать)

- [X] T024 [P] [US2] `src/internal/ast/stmt_test.go` (подмножество US2): конструкторы `LetStmt`/`AssignStmt`/`ExpressionStmt` и `Pos()` (ведущий токен / `Expr.Pos()`) — data-model §4
- [X] T025 [US2] `src/internal/parser/parse_stmt_test.go` (US2-часть): `печать("Привет, Уклад!")` → `Program` из `ExpressionStmt(CallExpr(Ident(печать), …))` (нет отдельного print-узла); `пусть a = 2+3*4` → `LetStmt`; `x = x + 1` → `AssignStmt`(lvalue=`Ident`); `печать(a, b)` → 2 аргумента; завершение на `EOF` + `EOFPos` (SC-006); golden SE-ASSIGN-TARGET `неверная цель присваивания: слева от '=' допустима только переменная` для `x.поле = 5` и `x[i] = 5`; `examples/hello.ladix`, `examples/арифметика.ladix` → 0 синтаксических ошибок (SC-001)

### Реализация

- [X] T026 [P] [US2] `src/internal/ast/stmt.go` (US2-часть): `LetStmt{Name Ident; Value Expression}`, `AssignStmt{Name Ident; Value Expression}`, `ExpressionStmt{Expr Expression}`; реализуют `Statement`; `Pos()` по D4
- [X] T027 [US2] `src/internal/parser/parse_stmt.go` (US2-часть): top-level-цикл `Parse()` строит `Program.Items` до `EOF` (фиксирует `EOFPos`, FR-007); `parseStatement` — диспетчер по ведущему ключевому слову (заглушки для US3/US4) + ветка «начинается с выражения»: разобрать выражение; если следующий `ASSIGN` → `AssignStmt`, причём lvalue ОБЯЗАН быть `*ast.Ident`, иначе SE-ASSIGN-TARGET; иначе `ExpressionStmt` + завершающий `NEWLINE`; `LetStmt` = `пусть Ident "=" Expression NEWLINE`; `печать` — обычный `Ident` в `Callee` (FR-008/FR-009/FR-010, D-R11; зависит от T026, US1)

**Checkpoint**: US1+US2 = MVP — выражения встроены в каркас программы; `hello`/`арифметика` parse-clean.

---

## Phase 5: User Story 3 — Условия, циклы, блоки на INDENT/DEDENT (Priority: P2)

**Goal**: `если`/`иначе если`/`иначе`, `пока`, `для` с телами-`Block` на виртуальных токенах; пустой блок → ошибка; `вернуть`/`прервать`/`продолжить` синтаксически (без проверки контекста).

**Independent Test**: вложенные `если`/`пока`/`для` → корректная структура `Block`/`ElseClause`; пустой блок → SE-EMPTY-BLOCK; `иначеесли` слитно → SE-EXPECTED; `examples/условие.ladix`, `examples/цикл.ladix` parse-clean.

### Тесты (пишутся первыми, обязаны падать)

- [X] T028 [P] [US3] `src/internal/ast/stmt_test.go` (US3-часть) + `src/internal/ast/block_test.go`: конструкторы `IfStmt`(+`ElseClause`-union)/`WhileStmt`/`ForStmt`/`ReturnStmt`(nil=голый)/`BreakStmt`/`ContinueStmt`; `Block` (≥1 statement) `Pos()` = `INDENT`/первый оператор (data-model §3/§4, contracts/ast.md §C-4)
- [X] T029 [US3] `src/internal/parser/parse_stmt_test.go` (US3-часть): `если C: <блок> иначе если D: <блок> иначе: <блок>` → `IfStmt{Then, Else: ElseClause}`; вложенные блоки (тело `если` внутри тела `для`) с числом уровней по `INDENT`/`DEDENT`; `для i в диапазон(1, 11):` → `ForStmt`; `пока истина:` с `прервать` в теле; голый `вернуть` → `ReturnStmt{Value:nil}` и `вернуть E` → `ReturnStmt{Value:…}`; golden SE-EMPTY-BLOCK `пустой блок не допускается, добавьте хотя бы один оператор`; golden `иначеесли C:` слитно → `ожидалось 'конец строки', получено '<лексема>'`; `examples/условие.ladix`, `examples/цикл.ladix` → 0 ошибок (SC-001/SC-003)

### Реализация

- [X] T030 [P] [US3] `src/internal/ast/stmt.go` (US3-часть): `IfStmt{Cond; Then *Block; Else *ElseClause}`, `ElseClause`-union (финальный `иначе`: `Body *Block`; `иначе если`: `Cond; Then *Block; Else *ElseClause`), `WhileStmt{Cond; Body *Block}`, `ForStmt{Var Ident; Iterable; Body *Block}`, `ReturnStmt{Value Expression}` (nil=голый), `BreakStmt`, `ContinueStmt` (data-model §4)
- [X] T031 [P] [US3] `src/internal/ast/block.go` (дополнение): `Block{Stmts []Statement}` (минимум 1; `Pos()` = `INDENT`/первый оператор) — data-model §3
- [X] T032 [US3] `src/internal/parser/parse_stmt.go` (US3-часть): `parseBlock` = `NEWLINE → INDENT → Statement+ → DEDENT`; отсутствие `INDENT` после `:`+`NEWLINE` → SE-EMPTY-BLOCK (D-R12); `IfStmt`+`ElseClause`-цепочка; `WhileStmt`; `ForStmt` (`для Ident "в" Expression ":" Block`); `ReturnStmt` (голый vs выражение по множеству FIRST(Expression)), `BreakStmt`, `ContinueStmt`; подключить ветки в диспетчер `parseStatement` (FR-011/FR-012/FR-013; зависит от T030/T031, US2)

**Checkpoint**: управляющие конструкции и блоки работают; `условие`/`цикл` parse-clean.

---

## Phase 6: User Story 4 — Функции: объявление, параметры, возврат, рекурсия (Priority: P2)

**Goal**: `FunctionDecl` только на верхнем уровне (вложенная → ошибка), позиционные параметры с висящей запятой, тело-`Block`, рекурсивные вызовы; зарезервированные `StepAction`/`RunProcessExpr` парсятся (семантика — eval-later).

**Independent Test**: `examples/функция.ladix`, `examples/факториал.ladix` parse-clean; вложенная `функция` → SE-NESTED-FN дословно; `StepAction`/`RunProcessExpr` строятся.

### Тесты (пишутся первыми, обязаны падать)

- [X] T033 [P] [US4] `src/internal/ast/decl_test.go` + `step_test.go` + `expr_test.go` (US4-часть): `FunctionDecl{Name; Params []Ident; Body *Block}`; `AssignAction`/`CallAction`/`NotifyAction` (реализуют `Statement`); `RunProcessExpr{Process Ident; Args []Expression}` (`Pos()`=`запустить`) — data-model §5/§6/§7
- [X] T034 [US4] `src/internal/parser/parse_decl_test.go`: `функция факториал(n): <тело>` → `FunctionDecl{Params:[n]}`; `функция f(a, b, c,):` → 3 параметра (висящая запятая); `вернуть n * факториал(n - 1)` → `ReturnStmt{BinaryExpr}` с рекурсивным `CallExpr`; голый `вернуть` в функции → `ReturnStmt{nil}`; golden SE-NESTED-FN `вложенные функции не поддерживаются в v1` для `функция` в теле функции; `StepAction` (`присвоить`/`вызвать`/`уведомить`) и `RunProcessExpr` (`запустить процесс P(...)`) строятся; `examples/функция.ladix`, `examples/факториал.ladix` → 0 ошибок (SC-001/SC-003)

### Реализация

- [X] T035 [P] [US4] `src/internal/ast/decl.go`: `FunctionDecl{Name Ident; Params []Ident; Body *Block}` (реализует `Decl`/`TopLevelItem`; `Pos()`=токен `функция`) — data-model §5
- [X] T036 [P] [US4] `src/internal/ast/step.go`: `AssignAction{Name Ident; Value Expression}`, `CallAction{Name Ident; Args []Expression}`, `NotifyAction{Name Ident; Args []Expression}` (реализуют `Statement`; `Pos()`=ведущий токен) — зарезервированы, не валидируются (guardrail 6), data-model §6
- [X] T037 [P] [US4] `src/internal/ast/expr.go` (дополнение): `RunProcessExpr{Process Ident; Args []Expression}` (реализует `Expression`/Primary; `Pos()`=токен `запустить`; скобки — часть узла) — data-model §7
- [X] T038 [US4] `src/internal/parser/parse_decl.go`: `parseTopLevelItem` диспетчит `функция` → `FunctionDecl` (ТОЛЬКО верхний уровень; `функция Ident "(" ParamList? ")" ":" Block`, висящая запятая в `ParamList`); встреча `функция` внутри любого `Block` (в `parseStatement`) → SE-NESTED-FN (синтаксис, grammar §4, D-R12); FR-014 (зависит от T035, US3)
- [X] T039 [US4] `src/internal/parser/parse_stmt.go` (дополнение): разбор зарезервированных `StepAction` (`присвоить`/`вызвать`/`уведомить`) в `parseStatement` — строятся и доступны внутри `если`/`пока`/`для`; гард «только в шаге процесса» НЕ проверяется (FR-015, guardrail 6; зависит от T036)
- [X] T040 [US4] `src/internal/parser/parse_expr.go` (дополнение): `RunProcessExpr` в `parsePrimary` при ведущем `запустить` (`запустить процесс Ident ("(" ArgList? ")")?`); скобки — часть узла, не постфикс-вызов (FR-016; зависит от T037)

**Checkpoint**: функции и зарезервированные узлы разбираются; `функция`/`факториал` parse-clean; SE-NESTED-FN дословно.

---

## Phase 7: User Story 5 — Понятные синтаксические ошибки + восстановление (Priority: P2, пронизывающая)

**Goal**: финализировать panic-mode восстановление по явному множеству синхро-токенов, top-level-диспетчер SE-UNEXPECTED для отложенных деклараций/`значение`/`{`, полный golden-каталог 7 кодов и многоошибочное накопление без фантомов/без stack trace.

**Independent Test**: по одному golden-кейсу на каждый из 7 кодов (позиции в рунах, двухстрочный формат); программа с ≥2 ошибками на разных строках → несколько диагностик без каскада, накопитель ≤≈20, без Go stack trace.

### Тесты (пишутся первыми, обязаны падать)

- [X] T041 [US5] `src/internal/parser/errors_golden_test.go`: полный каталог — все 7 кодов (SE-CHAIN, SE-NESTED-FN, SE-INT-RANGE, SE-EXPECTED, SE-UNEXPECTED, SE-EMPTY-BLOCK, SE-ASSIGN-TARGET) ДОСЛОВНО в двухстрочном формате с корректной `(строка, колонка)` 1-based в рунах; SE-EXPECTED-кейсы: незакрытый `печать(1, 2` на `EOF` → `ожидалось ')', получено 'конец файла'`, `если x` без `:` → `ожидалось ':', получено '<лексема>'`; SE-UNEXPECTED: `источник`/`метрика`/`процесс`/`когда`/`значение`/`{` на верхнем уровне → `неожиданный токен '<лексема>'` (SC-003, FR-017/FR-024)
- [X] T042 [US5] `src/internal/parser/recover_test.go`: вход с ≥2 независимыми ошибками на разных строках → столько же диагностик с верными позициями, без фантомного каскада; накопитель не превышает бюджет (≈20, общий с лексером); в выводе нет Go stack trace; `Program` возвращается best-effort (SC-005, FR-025)

### Реализация

- [X] T043 [US5] `src/internal/parser/recover.go`: явное множество синхро-токенов (структурные `NEWLINE`/`DEDENT`/`EOF`; ведущие statements `пусть`/`если`/`пока`/`для`/`вернуть`/`прервать`/`продолжить`; `функция`; step-action `присвоить`/`вызвать`/`уведомить`; отложенные `источник`/`метрика`/`процесс`/`когда`); `synchronize()` отбрасывает токены до ближайшего синхро-токена, потребляя `NEWLINE`/`DEDENT` и НЕ потребляя ведущие ключевые слова; подавление регистрации НОВЫХ ошибок до точки синхронизации (panic-mode флаг); интеграция в `(*Parser).error` (FR-025/FR-026, D-R8, contracts/syntax-errors.md «Множество синхро-токенов»)
- [X] T044 [US5] `src/internal/parser/parse_stmt.go` (дополнение): top-level-диспетчер SE-UNEXPECTED — ведущие `источник`/`метрика`/`процесс`/`когда` (`KW_SOURCE`/`KW_METRIC`/`KW_PROCESS`/`KW_WHEN`), `значение` (`KW_VALUE`), `{`/`}` (`LBRACE`/`RBRACE`) и прочий недопустимый старт → `неожиданный токен '<лексема>'` (FR-017, guardrail 12)
- [X] T045 [US5] Подключить panic-mode ко всем точкам ошибок (`parse_expr.go`/`parse_stmt.go`/`parse_decl.go`/`literals.go`): после каждой зарегистрированной ошибки срабатывает `synchronize()`; в штатных путях паника отсутствует, `Program` всегда возвращается; убедиться, что `examples/ошибка.ladix` парсится с 0 синтаксических ошибок (дефект — runtime), SC-001

**Checkpoint**: все 7 кодов дословно с позициями в рунах; многоошибочное накопление без фантомов и stack trace.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: финальные гейты качества и сквозная проверка приёмки.

- [ ] T046 [P] `gofmt -l .` из `src/` — пусто (SC-007); `go vet ./...` — без замечаний
- [ ] T047 `go build ./...` и полный `go test ./...` из `src/` — зелёные, лексер 001 не сломан (SC-007)
- [ ] T048 Сквозная проверка набора parse-clean (SC-001): `examples/{hello,арифметика,условие,цикл,функция,факториал}.ladix` и `examples/ошибка.ladix` → 0 синтаксических ошибок; декларативные `examples/{выручка,онбординг}.ladix` ожидаемо дают `неожиданный токен` (в набор не входят) — закрепить в `src/internal/parser/examples_test.go`
- [ ] T049 [P] Прогон чек-листа приёмки quickstart.md §3 (гейты P1/P2a/P2b/P2c) и сверка с SC-002…SC-006
- [ ] T050 [P] Полировка пакет-документации `src/internal/ast/doc.go` и `src/internal/parser/doc.go`: границы (НЕ eval/семантика/декларации), ссылки на contracts; сверить отсутствие импорта `errors` в `ast` (листовость, D1)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: без зависимостей — можно начинать сразу
- **Foundational (Phase 2)**: зависит от Setup; БЛОКИРУЕТ все user stories
- **US1 (Phase 3, P1)**: зависит от Foundational
- **US2 (Phase 4, P1)**: зависит от Foundational + US1 (statement-ветка разбирает выражение)
- **US3 (Phase 5, P2)**: зависит от US1 (условия — выражения) и US2 (диспетчер `parseStatement`, `Program`)
- **US4 (Phase 6, P2)**: зависит от US3 (`Block`, `ReturnStmt`) и US1 (выражения/постфикс)
- **US5 (Phase 7, P2)**: финализирует восстановление поверх US1–US4 (нужны источники ошибок всех кодов)
- **Polish (Phase 8)**: после всех желаемых US

### Within Each User Story

- Тесты пишутся ПЕРВЫМИ и обязаны падать до реализации (конституция VI)
- `ast`-узлы (модели) → строители литералов → `parser`-правила (сервисы) → интеграция/golden
- В `parser`: `literals.go` до `parse_expr.go` (`parsePrimary` зовёт строители); `parse_expr`/`parse_stmt` до `parse_decl`

### Parallel Opportunities

- Setup T001/T002 — параллельно
- Foundational: тесты T004/T005/T006 — параллельно; реализация T007/T011/T012 — параллельно (разные пакеты); T008 после T004; T009 после T007; T010 после T009; T013 после T007
- US1: T016/T017 параллельно; T020/T021 параллельно; затем T022 → T023
- US2: T024 ∥ далее; T026 параллельно, затем T027
- US3: T030/T031 параллельно, затем T032
- US4: T035/T036/T037 параллельно, затем T038/T039/T040
- Polish: T046/T049/T050 параллельно

---

## Parallel Example: Foundational (Phase 2)

```bash
# Тесты-первыми (разные пакеты, параллельно):
Task T004: "op_test.go — String()/подмножество CompOp в src/internal/ast/"
Task T005: "parserror_test.go — формат/errors.As/ErrorList в src/internal/errors/"
Task T006: "errors_test.go — 7 строителей текста дословно в src/internal/parser/"

# Реализация без взаимозависимостей (параллельно):
Task T007: "position.go — локальная Position в src/internal/ast/"
Task T011: "parserror.go — ParseError+Error() в src/internal/errors/"
Task T012: "errors.go — строители 7 текстов + helper лексемы в src/internal/parser/"
```

## Parallel Example: User Story 1 (Phase 3)

```bash
# AST-узлы выражений и литералов (разные файлы, параллельно):
Task T020: "expr.go — Binary/Unary/Call/Index/Field в src/internal/ast/"
Task T021: "literal.go — Int/Float/String/Bool/None/Duration/List/Ident в src/internal/ast/"
```

---

## Implementation Strategy

### MVP First (US1 + US2 — P1)

1. Phase 1: Setup
2. Phase 2: Foundational (КРИТИЧНО — блокирует все US)
3. Phase 3: US1 (выражения) → **STOP & VALIDATE**: таблица приоритетов (SC-002), SE-CHAIN/SE-INT-RANGE (SC-003)
4. Phase 4: US2 (каркас программы) → **STOP & VALIDATE**: `hello`/`арифметика` parse-clean, завершение на `EOF` (SC-001/SC-006), SE-ASSIGN-TARGET
5. MVP готов: разбор выражений + императивного каркаса

### Incremental Delivery

1. Setup + Foundational → каркас готов
2. + US1 → выражения (демо: таблица приоритетов)
3. + US2 → MVP (демо: `hello`/`арифметика`)
4. + US3 → условия/циклы/блоки (`условие`/`цикл`)
5. + US4 → функции (`функция`/`факториал`)
6. + US5 → полный golden-каталог + восстановление
7. Каждая US добавляет ценность, не ломая предыдущие

---

## Notes

- [P] = разные файлы, нет зависимостей от незавершённых задач
- [Story] связывает задачу с user story для трассируемости
- Каждая US независимо завершаема и тестируема
- Тесты обязаны падать до реализации (tests-first, конституция VI)
- `ast` НЕ импортирует `internal/errors` (листовость, D1); конвертер позиций — в `parser/pos.go`
- `errors` дополняется ТОЛЬКО файлом `parserror.go` (+тест); `aggregate.go` НЕ трогать (guardrail 8)
- Три канонических текста (SE-CHAIN/SE-NESTED-FN/SE-INT-RANGE) — байт-в-байт из SPEC §13.4
- Коммит после каждой задачи или логической группы; на каждом checkpoint можно остановиться и провалидировать US
- Избегать: расплывчатых задач, конфликтов по одному файлу, кросс-стори зависимостей, ломающих независимость
