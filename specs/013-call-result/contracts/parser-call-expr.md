# Contract: Грамматика и AST `CallExternalExpr` (фронтенд)

**Feature**: 013-call-result | **Граница**: `internal/ast` + `internal/parser` | **Источник**: §AU-3.1 (D-AU-1)

## C-AST-1. Узел `CallExternalExpr` (`internal/ast/expr.go`)

```go
type CallExternalExpr struct {
    exprBase
    Target Ident
    Args   []Expression
}
func NewCallExternalExpr(pos Position, target Ident, args []Expression) *CallExternalExpr
```

| Инвариант | Проверка |
|---|---|
| **C-AST-1.1** имя | `CallExternalExpr`/`NewCallExternalExpr` (НЕ `CallExpr` — занят постфикс-вызовом `:31`) |
| **C-AST-1.2** Pos() | = позиция токена `вызвать` (Принцип IV, руны с 1) |
| **C-AST-1.3** поля | `Target Ident` (логическое имя), `Args []Expression` (может быть пуст) |
| **C-AST-1.4** листовость | не импортирует `internal/errors` |

## C-PARSE-1. Грамматика выражения

```
CallExternalExpr ::= "вызвать" IDENT ( "(" ArgList? ")" )?
ArgList          ::= Expression ( "," Expression )* ","?     # как у RunProcessExpr
```

Зеркалит `parseRunProcess` (`parse_expr.go:240`), но БЕЗ служебного слова (`запустить` требует `expect(KW_PROCESS)`; `вызвать` — нет).

## C-PARSE-2. Точки правки парсера

| Сайт | Файл:строка | Правка |
|---|---|---|
| FIRST-set | `parse_expr.go:18-22` `startsExpression` | добавить `lexer.KW_CALL` рядом с `KW_RUN`/`KW_VALUE`/`KW_EVENT` |
| диспетч primary | `parse_expr.go:165` `parsePrimary` | `case lexer.KW_CALL: return p.parseCallExternalExpr()` (рядом с `case lexer.KW_RUN`@197) |
| новый метод | `parse_expr.go` (рядом с `parseRunProcess`) | `parseCallExternalExpr`: `advance()` `вызвать` → `expect(IDENT,"имя цели")` → опц. `"(" parseArgList(RPAREN) ")"` → `NewCallExternalExpr(toASTPos(callTok.Pos), *target, args)` |

```go
func (p *Parser) parseCallExternalExpr() ast.Expression {
    callTok := p.advance() // вызвать
    nameTok, _ := p.expect(lexer.IDENT, "имя цели")
    target := p.identFrom(nameTok)
    var args []ast.Expression
    if p.check(lexer.LPAREN) {
        p.advance()
        args = p.parseArgList(lexer.RPAREN)
        p.expect(lexer.RPAREN, ")")
    }
    return ast.NewCallExternalExpr(toASTPos(callTok.Pos), *target, args)
}
```

## C-PARSE-3. Развязка контекста statement ↔ выражение (БЕЗ неоднозначности)

| Ввод | Путь разбора | Результат |
|---|---|---|
| `вызвать crm(x)` (ведущий токен шага) | `parseStatement`/`parseStepAction` ловит `вызвать` ДО `parseExpression` | `ast.CallAction` (statement, v1 без изменений) |
| `присвоить r = вызвать crm(x)` | `KW_SET`→`AssignAction`, RHS → `parseExpression`→`parsePrimary`→`case KW_CALL` | `CallExternalExpr` в RHS присвоения |
| `печать(вызвать сервис())` | аргумент → `parseExpression`→`parsePrimary`→`case KW_CALL` | `CallExternalExpr` как аргумент |
| `[вызвать a(), 1]` | элемент списка → `parseExpression`→…→`case KW_CALL` | `CallExternalExpr` как элемент |
| `вызвать crm(x).статус` | `CallExternalExpr` + постфикс `.статус` цепочкой `parsePostfix` | `FieldExpr{Target: CallExternalExpr}` |
| `уведомить` в позиции выражения | `KW_NOTIFY` НЕ в `parsePrimary`/`startsExpression` → default-ветка | синтаксическая ошибка (как v1) — `уведомить` только statement |

**Ключевой инвариант развязки**: добавление `KW_CALL` в `parsePrimary` НЕ перехватывает statement-путь, т.к. `parseStatement` ловит ведущий `вызвать` раньше. Никакой эвристики — структурная позиция токена.

## Тест-замки контракта (tests-first; инверсии — в tasks.md)

| Замок | Что фиксирует | Инверсная мутация (обязана покраснить) |
|---|---|---|
| `TestNewCallExternalExpr` | конструктор: Pos=токен `вызвать`, Target/Args сохранены | передать иную позицию / потерять Args → поля ≠ ожидаемым |
| `TestParseCallExprInAssignRHS` | `присвоить r = вызвать crm(x)` → RHS = `*CallExternalExpr` (не `CallAction`, не `CallExpr`) | убрать `case KW_CALL` из `parsePrimary` → RHS не `CallExternalExpr` / ошибка разбора |
| `TestStartsExpressionCall` | `startsExpression(KW_CALL)==true` | не добавить `KW_CALL` в `startsExpression` → false; `вызвать` как аргумент не начинает выражение |
| `TestParseCallExprAsArgAndListElem` | `вызвать` допустим как аргумент и элемент списка | то же (FIRST-set / primary case) → red |
| `TestParseCallExprPostfix` | `вызвать crm(x).статус` → `FieldExpr{Target:*CallExternalExpr}` | сделать скобки/постфикс частью узла некорректно → структура ≠ ожидаемой |
| `TestLeadingCallStaysStatement` | `вызвать crm(x)` отдельной строкой → `CallAction` (statement цел) | если правка перехватит ведущий `вызвать` в выражение → не `CallAction` |
| `TestNotifyNotExpression` | `уведомить` в позиции выражения → ошибка (как v1) | ошибочно добавить `KW_NOTIFY` в primary → `уведомить` парсится как выражение |
| аддитивность v1 (существующие parser-тесты) | v1-выражения и постфикс-вызов `f(x)` разбираются прежним результатом | любая регрессия FIRST-set/primary → существующие тесты red |
