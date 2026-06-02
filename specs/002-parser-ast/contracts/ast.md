# Contract: AST Ladix (интерфейс парсер → eval)

**Feature**: 002-parser-ast | **Источники**: ARCHITECTURE §4, SPEC §3/§5, docs/grammar.md §3/§4/§9/§10,
spec.md FR-001..FR-022

Этот контракт — стабильный интерфейс, который парсер отдаёт eval (фича 003, потребитель класса B). Eval
вправе на него опираться. Нарушение — регресс, ловится табличным тестом «вход → ожидаемый AST».

## C-1. Гарантии дерева

| # | Гарантия | Источник |
|---|---|---|
| C-1.1 | Корень — `Program{Items []TopLevelItem; EOFPos Position}`; `Items` в порядке исходника; разбор завершается ровно на `EOF`, `EOFPos` = позиция токена `EOF` | FR-007, SC-006 |
| C-1.2 | Каждый узел реализует `Node` и несёт `ast.Position` (1-based, руны) по конвенции `Pos()` (C-4) | FR-001/FR-005, SC-004 |
| C-1.3 | `ast.Position` — ЛОКАЛЬНЫЙ тип `ast`; пакет `ast` НЕ импортирует `errors` (листовость, D1) | FR-001 |
| C-1.4 | `GroupExpr` в дереве отсутствует — `( E )` свёрнуто в обёрнутое выражение | FR-006 |
| C-1.5 | `BinaryExpr` лево-ассоциативен; `Comparison` — максимум один `CompOp` (цепочки нет в дереве) | FR-018/FR-019 |
| C-1.6 | `IntLit` несёт значение в диапазоне int64 (вне диапазона — ошибка при сборке, узел всё равно есть) | FR-022 |
| C-1.7 | Зарезервированные `StepAction`/`RunProcessExpr` строятся, но семантически НЕ валидированы (eval-later) | FR-003/FR-015/FR-016 |

## C-2. Каскад приоритетов (контрольные инварианты, SC-002)

Низший→высший: `LogicOr → LogicAnd → LogicNot → Comparison → Additive → Multiplicative → Unary →
Postfix → Primary` (SPEC §5, grammar §9).

| Вход | Дерево | Правило |
|---|---|---|
| `2 + 3 * 4` | `BinaryExpr(+, IntLit(2), BinaryExpr(*, IntLit(3), IntLit(4)))` | `*` глубже `+` |
| `(2 + 3) * 4` | `BinaryExpr(*, BinaryExpr(+, 2, 3), 4)` | группировка свёрнута |
| `a - b - c` | `BinaryExpr(-, BinaryExpr(-, a, b), c)` | лево-ассоциативность |
| `10 / 2 / 5` | `BinaryExpr(/, BinaryExpr(/, 10, 2), 5)` | то же |
| `не x и y` | `BinaryExpr(и, UnaryExpr(не, x), y)` | `не` выше `и` |
| `-5` | `UnaryExpr(-, IntLit(5))` | знак НЕ сворачивается |
| `x > -10 и x < 0` | `BinaryExpr(и, BinaryExpr(>, x, UnaryExpr(-,10)), BinaryExpr(<, x, 0))` | сравнение выше `и` |
| `данные[i].поле(1, 2,)` | `CallExpr(FieldExpr(IndexExpr(данные, i), поле), [1, 2])` | левоассоциативный постфикс, висящая запятая |
| `[1, "две", истина,]` | `ListLit{[IntLit, StringLit, BoolLit]}` | гетерогенность + висящая запятая |
| `[]` | `ListLit{}` | пустой список |

## C-3. Операторы (D3)

| Тип | Значения |
|---|---|
| `BinOp` | `или` `и` `+` `-` `*` `/` `//` `%` `==` `!=` `<` `<=` `>` `>=` (единый enum, 14) |
| `CompOp` | `==` `!=` `<` `<=` `>` `>=` (подмножество `BinOp`, 6 — для будущего `MetricTrigger`) |
| `UnOp` | `не`, унарный `-` |

`BinaryExpr.Op` — `BinOp`; сравнения несут одно из значений `CompOp`-подмножества. Один источник истины:
`CompOp` отбирается из `BinOp`, константы не дублируются.

## C-4. Конвенция `Pos()` узла (D4 — load-bearing для диагностик eval)

| Узел | `Pos()` |
|---|---|
| `BinaryExpr`, `UnaryExpr` | токен **оператора** (не операнда) |
| `CallExpr` | позиция `Callee` |
| `IndexExpr`, `FieldExpr` | позиция `Target` |
| литералы, `Ident` | свой токен |
| `LetStmt`/`IfStmt`/`WhileStmt`/`ForStmt`/`ReturnStmt`/`BreakStmt`/`ContinueStmt` | ведущий ключевой токен |
| `AssignStmt` | позиция lvalue (токен `Name`) — без ведущего ключевого слова |
| `ExpressionStmt` | `Expr.Pos()` — без ведущего ключевого слова |
| `AssignAction`/`CallAction`/`NotifyAction` | ведущий ключевой токен |
| `FunctionDecl` | токен `функция` |
| `Block` | позиция `INDENT`/первого оператора |

> Зачем: runtime-диагностика eval указывает на оператор — деление на ноль рапортуется на колонке `/`
> (`examples/ошибка.ladix` стр. 5 кол. 14).

## C-5. Узлы дерева (полный список scope B)

- **Структурные**: `Program`, `Block` (≥1 statement).
- **Statements**: `LetStmt`, `AssignStmt` (lvalue=`Ident`), `ExpressionStmt`, `IfStmt`(+`ElseClause`),
  `WhileStmt`, `ForStmt`, `ReturnStmt` (`Value` nil = голый), `BreakStmt`, `ContinueStmt`.
- **Зарезервированные statements**: `AssignAction`, `CallAction`, `NotifyAction`.
- **Decl**: `FunctionDecl` (top-level, позиционные параметры).
- **Expressions**: `BinaryExpr`, `UnaryExpr`, `CallExpr`, `IndexExpr`, `FieldExpr`, `RunProcessExpr`.
- **Literals/Primary**: `IntLit`, `FloatLit`, `StringLit`, `BoolLit`, `NoneLit`, `DurationLit`,
  `ListLit`, `Ident`.

## C-6. Чего дерево НЕ содержит / парсер НЕ делает (границы)

- НЕТ узла `GroupExpr` (свёрнут, C-1.4).
- НЕ свёрнут знак на целом литерале (`-X` = `UnaryExpr(-, IntLit(X))`); `MinInt64` литералом невыразим.
- НЕ проверяется семантика: объявленность/дубли имён, типы, арность, «`вернуть` только в функции»,
  «`присвоить`/`вызвать`/`уведомить` только в шаге», легальность `прервать`/`продолжить` (eval-later).
- НЕ проверяется диапазон `DurationLit` (backlog `float-overflow-spec-gap`).
- НЕТ узлов деклараций `источник`/`метрика`/`процесс`/`триггер` (вне scope B; их ведущие ключевые слова
  на верхнем уровне → синтаксическая ошибка, см. syntax-errors.md).
- `печать`/`длина`/`диапазон`/`добавить`/`сумма` и имена периодов — обычные `Ident` (резолв в eval).
