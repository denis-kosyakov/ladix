# Contract: грамматика + parseDeadlineTrigger + expectLexeme (B4a)

Источник §AU-6.1.1. Группа: **B4a-фронтенд**.

## Грамматика (контекстный разбор, D-AU-4)

```text
DeadlineTrigger := "задача"ᴵᴰᴱᴺᵀ "просрочена"ᴵᴰᴱᴺᵀ "в"ᴷᵂ_ᴵᴺ Ident "."ᴰᴼᵀ Ident
```

`задача`/`просрочена` — IDENT-лексемы; лексер контекстно-независим (НЕ трогается, L=11). Контекст
применяет ПАРСЕР по лексеме после `когда`.

## Диспетчер `parseTriggerDecl` (parse_decl.go:406)

ДО (на @a92ad50):
```go
switch p.peek().Type {
case lexer.KW_METRIC:    return … parseMetricTrigger
case lexer.KW_EVENT:     return … (событие)
case lexer.KW_SCHEDULE:  return … parseScheduleTrigger
default:                 p.error(p.peek().Pos, msgExpected(msgTriggerKind, p.peek())) // SE-TRIGGER-KIND
}
```

ПОСЛЕ (B4a — ветка ПЕРЕД default):
```go
case lexer.IDENT:
    if p.peek().Lexeme == "задача" {
        spec = p.parseDeadlineTrigger()
    } else {
        p.error(p.peek().Pos, msgExpected(msgTriggerKind, p.peek()))   // SE-TRIGGER-KIND
    }
```
(Точная форма ветки — решение impl; инвариант: IDENT-лексема `задача` → `parseDeadlineTrigger`,
любой иной ведущий токен → SE-TRIGGER-KIND.)

## `parseDeadlineTrigger`

```go
func (p *Parser) parseDeadlineTrigger() ast.TriggerSpec {
    pos := p.peek().Pos                  // токен 'задача' → Pos узла
    p.advance()                          // (1) потребить IDENT 'задача'
    p.expectLexeme("просрочена")         // (2) НОВЫЙ хелпер
    p.expect(lexer.KW_IN, "в")           // (3)
    proc, _ := p.expect(lexer.IDENT, "имя процесса")  // (4)
    p.expect(lexer.DOT, ".")             // (5)
    step, _ := p.expect(lexer.IDENT, "имя шага")      // (6)
    return ast.NewDeadlineTrigger(pos, identFrom(proc), identFrom(step))
}
```

## НОВЫЙ хелпер `expectLexeme` (по образцу expectCompOp :441)

```go
// expectLexeme: сверяет, что текущий токен — IDENT с заданной лексемой; иначе SE-EXPECTED.
// Нужен, т.к. expect(IDENT,…) матчит ЛЮБУЮ идентификатор-лексему, а 'просрочена' —
// конкретная IDENT-лексема (D-AU-4: не ключевое слово).
func (p *Parser) expectLexeme(want string) (lexer.Token, bool) {
    tok := p.peek()
    if tok.Type == lexer.IDENT && tok.Lexeme == want {
        p.advance()
        return tok, true
    }
    p.error(tok.Pos, msgExpected(want, tok))   // ожидалось '<want>', получено '<лексема>'
    return tok, false
}
```

## Диагностики (SE-EXPECTED, без новых кодов — SE=14)

| Вход | want | Сообщение |
|------|------|-----------|
| `когда задача X…` | `просрочена` | `ожидалось 'просрочена', получено 'X'` |
| `когда задача просрочена P.S` (нет `в`) | `в` | `ожидалось 'в', получено 'P'` |
| `когда задача просрочена в .S` (нет процесса) | `имя процесса` | `ожидалось 'имя процесса', получено '.'` |
| `когда задача просрочена в P S` (нет `.`) | `.` | `ожидалось '.', получено 'S'` |
| `когда задача просрочена в P.` (нет шага) | `имя шага` | `ожидалось 'имя шага', получено '<лексема>'` |

## Тесты (B4a-группа)

- AST-конструкция: `когда задача просрочена в P.S:\n    печать(1)` → `*ast.DeadlineTrigger{
  Process:"P", Step:"S"}`, `Pos()`=токен `задача` (line/col).
- 5 негативов SE-EXPECTED (exact-match + позиция).
- v1-замок: `пусть задача = 10` parse-clean; `задача()` parse-clean (IDENT, не триггер).
- Инверсия: убрать `expectLexeme("просрочена")` → `когда задача в P.S:` ложно парсится → тест красный.
