# Phase 1 — Data Model: Парсер + AST Ladix

**Feature**: 002-parser-ast | **Date**: 2026-06-02 | **Plan**: [plan.md](./plan.md)

Сущности пакета `ast`, дополнения `errors` и `Parser`. Поля описаны концептуально (контракт), без
обязательных Go-сигнатур — те конкретизирует `/speckit-tasks`/реализация. Имена типов — английские
(D-R… как 001 D-R5), тексты сообщений — русские (SPEC §13). Источник эскиза — ARCHITECTURE §4.

---

## 1. `Position` (пакет `internal/ast`) — ЛОКАЛЬНАЯ (D1)

| Поле | Тип | Описание |
|---|---|---|
| `Line` | `int` | Номер строки, **1-based** |
| `Col` | `int` | Номер колонки, **1-based**, в **рунах** |

**Инварианты**:
- `Line ≥ 1`, `Col ≥ 1` для любого реального узла.
- **Локальна для `ast`**: пакет `ast` НЕ импортирует `errors` (листовость, D1/D-R1, конституция IV/VII).
  Дублирует `errors.Position` структурно, но не разделяет тип.
- Конвертер `errors.Position → ast.Position` живёт в `parser` (`pos.go`), НЕ в `ast`.

## 2. Интерфейсы узлов (пакет `internal/ast`)

| Интерфейс | Контракт | Реализуют |
|---|---|---|
| `Node` | `Pos() Position` | все узлы |
| `Statement` | `Node` + маркер `stmtNode()` | `LetStmt`, `AssignStmt`, `ExpressionStmt`, `IfStmt`, `WhileStmt`, `ForStmt`, `ReturnStmt`, `BreakStmt`, `ContinueStmt`, `AssignAction`, `CallAction`, `NotifyAction` |
| `Expression` | `Node` + маркер `exprNode()` | `BinaryExpr`, `UnaryExpr`, `CallExpr`, `IndexExpr`, `FieldExpr`, `RunProcessExpr`, `IntLit`, `FloatLit`, `StringLit`, `BoolLit`, `NoneLit`, `DurationLit`, `ListLit`, `Ident` |
| `Decl` | `Node` + маркер `declNode()` | `FunctionDecl` (в scope B — единственная) |
| `TopLevelItem` | `Node` (union: `Statement \| Decl`) | элементы `Program.Items` |

**Идиома**: пустые маркер-методы (`stmtNode()`/`exprNode()`/`declNode()`) — sum-type через интерфейс
(компилятор не даст `IntLit` пролезть туда, где ждут `Statement`). Встраиваемая база `base{position
Position}` даёт `Pos()` через embedding — без копипасты в каждом узле.

## 3. Верхний уровень и структурные узлы

| Узел | Поля | `Pos()` (D4) |
|---|---|---|
| `Program` | `Items []TopLevelItem; EOFPos Position` | — (корень; `EOFPos` = позиция токена `EOF`) |
| `Block` | `Stmts []Statement` (**минимум 1**; пустые запрещены) | позиция `INDENT`/первого оператора |

> `Block` — тело `если`/`пока`/`для`/функции. Блок-формы деклараций (`шаг`/`источник`/`метрика`/`процесс`)
> — НЕ `Block` и НЕ в scope B (ARCHITECTURE §4.8).

## 4. Statements (реализуют `Statement`)

| Узел | Поля | `Pos()` |
|---|---|---|
| `LetStmt` | `Name Ident; Value Expression` | токен `пусть` |
| `AssignStmt` | `Name Ident; Value Expression` (lvalue только `Ident`) | токен `Name`/`=` (ведущий) |
| `ExpressionStmt` | `Expr Expression` | `Expr.Pos()` |
| `IfStmt` | `Cond Expression; Then *Block; Else *ElseClause` (nil → без `иначе`) | токен `если` |
| `WhileStmt` | `Cond Expression; Body *Block` | токен `пока` |
| `ForStmt` | `Var Ident; Iterable Expression; Body *Block` | токен `для` |
| `ReturnStmt` | `Value Expression` (**nil** → голый `вернуть`) | токен `вернуть` |
| `BreakStmt` | — | токен `прервать` |
| `ContinueStmt` | — | токен `продолжить` |

**`ElseClause`** — union (ARCHITECTURE §4.3):
- финальный `иначе`: `Body *Block`;
- `иначе если`: `Cond Expression; Then *Block; Else *ElseClause` (рекурсивная цепочка).

**Инварианты/границы**:
- lvalue `AssignStmt` — только `Ident` (поля/индексы запрещены v1 → SE-ASSIGN-TARGET, D-R11).
- Контекстная легальность `вернуть`/`прервать`/`продолжить` — НЕ проверяется (eval-later, guardrail 7).

## 5. Declarations (реализуют `Decl`)

| Узел | Поля | `Pos()` |
|---|---|---|
| `FunctionDecl` | `Name Ident; Params []Ident; Body *Block` (позиционные параметры; вложенных нет) | токен `функция` |

- Только на верхнем уровне; вложенная `функция` → SE-NESTED-FN (синтаксис, grammar §4, D-R12).
- `SourceDecl`/`MetricDecl`/`ProcessDecl`/`TriggerDecl` — **вне scope B** (их ведущие ключевые слова на
  верхнем уровне → SE-UNEXPECTED).

## 6. Step actions (зарезервированы, реализуют `Statement`) — ARCHITECTURE §4.6

| Узел | Поля | Семантика (eval-later) |
|---|---|---|
| `AssignAction` | `Name Ident; Value Expression` | `присвоить` → мутирует переменные процесса |
| `CallAction` | `Name Ident; Args []Expression` | `вызвать` → внешняя система (fire-and-forget) |
| `NotifyAction` | `Name Ident; Args []Expression` | `уведомить` → стаб-лог |

- **Парсятся и строятся** в `parseStatement`; гард «только в шаге процесса» — **НЕ** в парсере (eval-later,
  guardrail 6). `Pos()` — ведущий ключевой токен (`присвоить`/`вызвать`/`уведомить`).

## 7. Expressions (реализуют `Expression`) — ARCHITECTURE §4.7, SPEC §5

| Узел | Поля | `Pos()` (D4) |
|---|---|---|
| `BinaryExpr` | `Op BinOp; Left, Right Expression` | токен **оператора** |
| `UnaryExpr` | `Op UnOp; Operand Expression` (`не` / унарный `-`) | токен **оператора** |
| `CallExpr` | `Callee Expression; Args []Expression` | `Callee.Pos()` |
| `IndexExpr` | `Target Expression; Index Expression` (срезов нет v1) | `Target.Pos()` |
| `FieldExpr` | `Target Expression; Field Ident` (`.поле`) | `Target.Pos()` |
| `RunProcessExpr` | `Process Ident; Args []Expression` (зарезервирован; скобки — часть узла) | токен `запустить` |

**Правила**:
- Бинарные — **лево-ассоциативны** (цикл в каскаде); `и`/`или` — короткозамкнутые (семантика — eval).
- `Comparison` — **максимум один** `CompOp`; цепочка → SE-CHAIN (FR-019).
- Постфикс-цепочка `CallExpr`/`IndexExpr`/`FieldExpr` — лево-ассоциативно навешивается (FR-020).
- `GroupExpr` НЕ существует как узел — `( E )` сворачивается (D5/D-R6).

## 8. Literals / Primary (реализуют `Expression`)

| Узел | Терминал лексера | Несёт |
|---|---|---|
| `IntLit` | `INT` (цифровая строка) | int64 после `ParseInt` (диапазон — D2/D-R2); вне диапазона → SE-INT-RANGE |
| `FloatLit` | `FLOAT` | предразобранное `float64` (`Token.Value`) |
| `StringLit` | `STRING` | развёрнутая строка (`Token.Value`) |
| `BoolLit` | `BOOL` (`истина`/`ложь`) | `bool` (`Token.Value`) |
| `NoneLit` | `NONE` (`пусто`) | — |
| `DurationLit` | `DURATION` | величина-лексема + единица (`DurationValue`; диапазон НЕ проверяется, D-R3) |
| `ListLit` | `[` … `]` | `Elements []Expression` (висящая запятая, гетерогенность, `[]`) |
| `Ident` | `IDENT` | имя (период/встроенная функция/поле — резолв в eval) |

`Pos()` литералов и `Ident` — свой токен (D4).

## 9. Операторы (пакет `internal/ast`) — D3/D-R4

| Тип | Значения | Назначение |
|---|---|---|
| `BinOp` | `или` `и` `+` `-` `*` `/` `//` `%` `==` `!=` `<` `<=` `>` `>=` (14) | единый enum всех бинарных |
| `CompOp` | подмножество `BinOp`: `==` `!=` `<` `<=` `>` `>=` (6) | сравнения; отбор из `BinOp` без дублирования; для будущего `MetricTrigger.Op` |
| `UnOp` | `не`, унарный `-` (2) | унарные операторы |

- `CompOp` реализуется как **подмножество** `BinOp` (alias + предикат `IsComparison()` или эквивалент),
  НЕ как независимый enum-дубль констант.
- Все три имеют `String()` для диагностики и табличных тестов.

## 10. `ParseError` (пакет `internal/errors`) — НОВЫЙ файл `parserror.go`

| Поле | Тип | Описание |
|---|---|---|
| `Pos` | `errors.Position` | позиция проблемного токена (в рунах) |
| `Msg` | `string` | русское описание БЕЗ заголовка (см. contracts/syntax-errors.md) |

**Поведение**:
- `Error()` → канонический двухстрочный вид (идентичен `LexError`):
  ```
  Ошибка в строке N, колонка M:
  <Msg>
  ```
- Реализует `error`; разворачивается через `errors.As(&ParseError{})` (FR-023).
- Складывается в **тот же** `errors.ErrorList` (агрегат `Add(error)`/`Unwrap()[]error` — БЕЗ правок).
- В штатных путях паника ЗАПРЕЩЕНА (конституция III).

> Категория **Синтаксическая** (§13.1) использует тот же заголовок `Ошибка в строке N, колонка M:`, что
> и Лексическая — различие категорий по слою-источнику, не по формату.

## 11. `ErrorList` — общий накопитель (пакет `internal/errors`, БЕЗ изменений)

| Аспект | Контракт (из 001) |
|---|---|
| `Add(err error)` | принимает любой `error` (вкл. `ParseError`) до исчерпания бюджета |
| `Unwrap() []error` | развёртка `errors.As`/`errors.Is` |
| бюджет | `DefaultErrorBudget = 20`, **общий** лексика+синтаксис (§13.2) |

**Инварианты**: накопление ≤ ≈20 (FR-025, SC-005); в собранном пайплайне один `*ErrorList`
протаскивается лексер→парсер (D-R7/D-R10). Парсер 002 принимает `*ErrorList` параметром либо создаёт
свой в изоляции (тесты).

## 12. `Parser` (пакет `internal/parser`)

Ручной recursive-descent разборщик: принимает поток токенов, выдаёт `*ast.Program` (best-effort даже при
ошибках) и накопленные ошибки.

| Аспект | Контракт |
|---|---|
| конструктор | явная инстанциация; токены `[]lexer.Token` и (опц.) `*errors.ErrorList` через параметры; без пакет-уровневого состояния (FR-029) |
| вход | `[]lexer.Token` (контракт `001/contracts/token-stream.md`: ровно один `EOF`, баланс `INDENT`/`DEDENT`, `INT` строкой) |
| выход | `*ast.Program` (всегда; завершается на `EOF`) + `*errors.ErrorList` |
| зависимости | `ast`, `errors`, `lexer` (граф `parser → {ast, errors, lexer}`, без циклов) |
| предпросмотр | один токен (`peek`); без бэктрекинга; `advance`/`expect(type)` |
| границы | НЕ семантика (области/типы/контекст), НЕ eval, НЕ декларации `источник`/…/`когда` (guardrail 7/12) |

**Внутреннее состояние** (не публичное): курсор по слайсу токенов; флаг подавления ошибок (panic-mode);
ссылка на `*ErrorList`.

## 13. Множество синхро-токенов panic-mode (внутренний набор `parser/recover.go`)

Явный набор точек синхронизации (FR-026, D-R8); полный список — contracts/syntax-errors.md.

| Группа | Токены | Семантика при синхронизации |
|---|---|---|
| Структурные | `NEWLINE`, `DEDENT`, `EOF` | `NEWLINE`/`DEDENT` **потребляются**; на `EOF` разбор завершается |
| Ведущие ключевые слова statements | `пусть`, `если`, `пока`, `для`, `вернуть`, `прервать`, `продолжить` | НЕ потребляются (разбор начинается с них) |
| Декларация scope B | `функция` | НЕ потребляется |
| Зарезервированные step-action | `присвоить`, `вызвать`, `уведомить` | НЕ потребляются |
| Отложенные top-level декларации | `источник`, `метрика`, `процесс`, `когда` | НЕ потребляются (сами дают SE-UNEXPECTED) |

---

## Сводка узлов и счёт

- **Statements (9 + 3 зарезервированных)**: `LetStmt`, `AssignStmt`, `ExpressionStmt`, `IfStmt`,
  `WhileStmt`, `ForStmt`, `ReturnStmt`, `BreakStmt`, `ContinueStmt` + `AssignAction`/`CallAction`/
  `NotifyAction`; `ElseClause` (union, не Statement).
- **Decl (1)**: `FunctionDecl`.
- **Expressions (6)**: `BinaryExpr`, `UnaryExpr`, `CallExpr`, `IndexExpr`, `FieldExpr`, `RunProcessExpr`.
- **Literals/Primary (8)**: `IntLit`, `FloatLit`, `StringLit`, `BoolLit`, `NoneLit`, `DurationLit`,
  `ListLit`, `Ident`.
- **Структурные (2)**: `Program`, `Block`.
- **Операторы**: `BinOp` (14), `CompOp` (6, подмножество), `UnOp` (2).
- **Ошибка**: `ParseError`; накопитель — общий `ErrorList`.
- **`GroupExpr` НЕ материализуется** (D5).
