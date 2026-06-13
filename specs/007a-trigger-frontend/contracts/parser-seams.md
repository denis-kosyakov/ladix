# Контракт: швы парсера (top-level + Primary)

**Фаза**: 1 (design) | **Якорь**: `docs/trigger-model.md §TR-1, §TR-3` | **Решения**: D-TR-1, D-25 | **FR**: FR-001…004, FR-019, FR-020, FR-021

> Триггеры подключаются **двумя независимыми швами**: `когда` (`KW_WHEN`) уходит из `isUnexpectedTopLevel` в `parseTopLevelItem` (шов A, объявление); `значение`/`событие` (`KW_VALUE`/`KW_EVENT`) уходят в `parsePrimary` (шов B, выражения). Оба зеркалят существующие прецеденты — `KW_PROCESS` (005) и `KW_RUN` (US4).

## Назначение

Грамматические продукции трёх форм триггера (EBNF, дословно `grammar.md:178-192`) и две точки врезки в ручной recursive-descent парсер. Никаких генераторов/regex — ветки `if/switch` по ведущему токену. Синтаксические диагностики идут через существующий `msgExpected`/`msgUnexpected`/`msgEmptyBlock` (точные тексты — в diagnostics.md §TR-7.F).

**Размещение врезок**:

- `src/internal/parser/parse_stmt.go` — `isUnexpectedTopLevel` (минус `KW_WHEN`, строки ~40-48); `parseTopLevelItem` (+ветка `когда`, строки ~12-34).
- `src/internal/parser/parse_decl.go` — `parseTriggerDecl` + `parseMetricTrigger`/`parseEventTrigger`/`parseScheduleTrigger` + `expectCompOp` + `parseScheduleSpec` (НОВЫЕ функции, по соседству с `parseProcessDecl`).
- `src/internal/parser/parse_expr.go` — `parsePrimary` (+`KW_VALUE`/`KW_EVENT`, строки ~196-198); `startsExpression` (+оба, строки ~18-26).

## Грамматика (EBNF, §TR-1)

```ebnf
TriggerDecl     ::= "когда" TriggerSpec ":" Block

TriggerSpec     ::= MetricTrigger
                  | EventTrigger
                  | ScheduleTrigger

MetricTrigger   ::= "метрика" Ident CompOp Expression
EventTrigger    ::= "событие" Ident
ScheduleTrigger ::= "расписание" ScheduleSpec
ScheduleSpec    ::= "каждые" DurationLiteral
                  | "в" StringLiteral

CompOp          ::= "==" | "!=" | "<" | "<=" | ">" | ">="
Block           ::= Newline Indent Statement Statement* Dedent
```

- **Тело триггера — индентный `Block`, НЕ `{}`-скобки.** После `TriggerSpec` идёт `:`, далее обязательный `NEWLINE`, `INDENT` (ровно 4 пробела на ступень, табы запрещены — `grammar.md:63`), минимум один `Statement`; пустой блок → `TR-SYN-EMPTYBLOCK` (`grammar.md:256`). `{` / `}` на верхнем уровне остаются в `isUnexpectedTopLevel` → `неожиданный токен`. Фигурных скобок в Ladix нет.
- **Условие метрики — РОВНО одно сравнение** (`метрика Ident CompOp Expression`), разбирается **плоско** (имя → оператор → выражение), а НЕ через общий `parseExpression`. Составное `X<Y и Z>W` невыразимо структурно (FR-021) — см. таблицу прецедентов и diagnostics.md §TR-7.F.
- **Все 6 единиц `каждые`** (сек/мин/час/дн/нед/мес) принимаются без ограничений: `DurationLiteral` — уже жадно-читанный токен лексера (`keywords.go:37-39`), парсеру достаточно `expect(DURATION)`. Правило «нед/мес → ошибка» НЕ вводится (FR-004).
- **`в "<строка>"`** парсится как любой `StringLiteral`; формат содержимого НЕ проверяется парсером (и не проверяется семпроходом в 007a — FR-005).

## Шов A — top-level: `когда` → `parseTriggerDecl`

### Правка `isUnexpectedTopLevel` (parse_stmt.go:40-48)

Снять **только** `KW_WHEN`, оставив `KW_VALUE`/`LBRACE`/`RBRACE`:

```go
func isUnexpectedTopLevel(t lexer.TokenType) bool {
	switch t {
	case lexer.KW_VALUE, // KW_WHEN снят — теперь объявление (шов A)
		lexer.LBRACE, lexer.RBRACE:
		return true
	default:
		return false
	}
}
```

`KW_EVENT` в этом списке **исходно не значился** (в 005 событие как ведущий top-level-токен падало в `default` `parsePrimary` → `SE-UNEXPECTED`); после шва B оно тоже режется default-веткой при использовании вне выражения-оператора. Текст диагностики идентичен (`msgUnexpected`).

### Правка `parseTopLevelItem` (parse_stmt.go:12-34)

Новая ветка перед `isUnexpectedTopLevel` (прецедент — `KW_PROCESS`, parse_stmt.go:22-24):

```go
if p.check(lexer.KW_WHEN) {
	return p.parseTriggerDecl()
}
```

### `parseTriggerDecl` (НОВАЯ, parse_decl.go)

Декларация верхнего уровня, узел `TriggerDecl` (`declBase`). Позиция = токен `когда`. Вид триггера — по **второму токену** (после `когда`):

```
parseTriggerDecl():
    whenTok := advance()                       // потребить «когда», pos узла
    switch peek().Type:
      case KW_METRIC:   spec := parseMetricTrigger()    // когда метрика …
      case KW_EVENT:    spec := parseEventTrigger()     // когда событие …
      case KW_SCHEDULE: spec := parseScheduleTrigger()  // когда расписание …
      default:
          // SE-TRIGGER-KIND (§TR-7.F): msgExpected("метрика, событие или расписание", peek())
          error(peek().Pos, msgExpected("метрика, событие или расписание", peek()))
          synchronize(); return nil            // поглощённая ошибочная конструкция
    expect(COLON, ":")
    body := parseBlock()                        // NEWLINE INDENT Statement+ DEDENT (parse_stmt.go:157)
    return NewTriggerDecl(toASTPos(whenTok.Pos), spec, body)
```

### `parseMetricTrigger` (НОВАЯ)

`MetricTrigger ::= "метрика" Ident CompOp Expression`. Плоский разбор (НЕ `parseExpression` сверху):

```
parseMetricTrigger():
    metTok := advance()                         // «метрика», pos спеки
    nameTok := expect(IDENT, "имя метрики")
    op := expectCompOp()                        // == != < <= > >=
    rhs := parseExpression()                    // правая часть сравнения (любое выражение)
    return NewMetricTrigger(toASTPos(metTok.Pos), *identFrom(nameTok), op, rhs)
```

### `parseEventTrigger` (НОВАЯ)

`EventTrigger ::= "событие" Ident`:

```
parseEventTrigger():
    evtTok := advance()                         // «событие», pos спеки
    nameTok := expect(IDENT, "имя события")
    return NewEventTrigger(toASTPos(evtTok.Pos), *identFrom(nameTok))
```

### `parseScheduleTrigger` + `parseScheduleSpec` (НОВЫЕ)

`ScheduleTrigger ::= "расписание" ScheduleSpec`; `ScheduleSpec ::= "каждые" DurationLiteral | "в" StringLiteral`:

```
parseScheduleTrigger():
    schTok := advance()                         // «расписание», pos спеки
    spec := parseScheduleSpec()
    return NewScheduleTrigger(toASTPos(schTok.Pos), spec)

parseScheduleSpec():
    switch peek().Type:
      case KW_EVERY:                            // каждые DurationLiteral
          everyTok := advance()                 // «каждые»
          durTok := expect(DURATION, "длительность")
          return NewEverySchedule(toASTPos(everyTok.Pos), buildDurationLit(durTok)) // все 6 единиц
      case KW_IN:                               // в StringLiteral
          inTok := advance()                    // «в»
          strTok := expect(STRING, "время в кавычках")
          return NewAtSchedule(toASTPos(inTok.Pos), buildStringLit(strTok)) // формат не проверяется
      default:
          // SE-SCHEDULE-SPEC (§TR-7.F): msgExpected("каждые или в", peek())
          error(peek().Pos, msgExpected("каждые или в", peek()))
          synchronize(); return nil
```

### `expectCompOp` (НОВЫЙ хелпер)

CompOp — exact-match из шести токенов через существующий `compOpOf` (parse_expr.go:236-249, отображает `EQ/NEQ/LT/LE/GT/GE` → `BinOp`); при не-CompOp-токене → `SE-EXPECT-COMPOP`:

```
expectCompOp():
    binop, ok := compOpOf(peek().Type)          // переиспользуем parse_expr.go:236
    if !ok:
        // SE-EXPECT-COMPOP (§TR-7.F): msgExpected("оператор сравнения", peek())
        error(peek().Pos, msgExpected("оператор сравнения", peek()))
        return CompOp(0)                         // поглощённая ошибка; синхронизация выше
    advance()
    return ast.CompOp(binop)                      // CompOp = defined type над BinOp (op.go:56)
```

### Замечания по шву A

- **`parseBlock` НЕ меняется** — тело триггера разбирается тем же `parseBlock()` (parse_stmt.go:157-177), что и тела функции/шага: `NEWLINE INDENT Statement+ DEDENT`, пустой блок → `msgEmptyBlock`. Контекст (`inStep=false`, запрет действий-шага, гард значение/событие) — целиком на семпроходе (см. analyze-trigger.md).
- **Формы — вспомогательные узлы** (встраивают `base`, как `ElseClause`), живут внутри `TriggerDecl` как разновидность спеки. Точная Go-форма — в trigger-ast.md.

## Шов B — Primary: `значение`/`событие` → `ValueExpr`/`EventExpr`

### Правка `parsePrimary` (parse_expr.go, после `KW_RUN` ~196, перед `default`)

Прецедент — `case lexer.KW_RUN: return p.parseRunProcess()` (parse_expr.go:196-197):

```go
case lexer.KW_VALUE:
	t := p.advance()
	return ast.NewValueExpr(toASTPos(t.Pos))   // pos = токен «значение»
case lexer.KW_EVENT:
	t := p.advance()
	return ast.NewEventExpr(toASTPos(t.Pos))   // pos = токен «событие»
```

(Тривиальные первичные выражения без внутренней структуры — как `NoneLit`, parse_expr.go:180-181.)

### Правка `startsExpression` (parse_expr.go:18-26)

Добавить `KW_VALUE`/`KW_EVENT` в FIRST(Expression) рядом с `KW_RUN` — иначе оператор-выражение, начинающееся с `значение`/`событие` (напр. голый `значение` как ExpressionStmt в теле), не распознается:

```go
case lexer.INT, lexer.FLOAT, lexer.STRING, lexer.BOOL, lexer.NONE, lexer.DURATION,
	lexer.IDENT, lexer.KW_NOT, lexer.MINUS, lexer.LPAREN, lexer.LBRACKET, lexer.KW_RUN,
	lexer.KW_VALUE, lexer.KW_EVENT:
	return true
```

### Ключевые свойства шва B

- **Принимаются синтаксически ВЕЗДЕ.** `parsePrimary` не знает контекста — `значение`/`событие` парсятся в любой позиции выражения (тело метрики, тело шага, глобальный `пусть`, аргумент вызова). Контекст-гард — целиком на семпроходе (FR-006), как зеркало гарда действий-шага `inStep`. НЕ контекстный лексер, НЕ переиспользование `Ident`.
- **`событие.клиент` собирается даром** — `EventExpr` это Primary; постфиксный `.поле` навешивает существующий `FieldExpr` без правок постфикс-парсера.
- **`значение` как аргумент** (`запустить процесс P(значение)`) собирается даром — `ValueExpr` попадает в `parseArgList` как любой аргумент.

## D-TR-1 — токены разводятся по двум швам

| Токен | 005 (было) | 007a (стало) | Шов | Прецедент |
|---|---|---|---|---|
| `KW_WHEN` | отвергался в `isUnexpectedTopLevel` | объявление верхнего уровня → `parseTriggerDecl` | A | `KW_PROCESS` (parse_stmt.go:22) |
| `KW_VALUE` | отвергался в `isUnexpectedTopLevel` | первичное выражение → `ValueExpr`; на top-level всё ещё `SE-UNEXPECTED` | B | `KW_RUN` (parse_expr.go:196) |
| `KW_EVENT` | падал в `default` `parsePrimary` | первичное выражение → `EventExpr`; на top-level `SE-UNEXPECTED` | B | `KW_RUN` (parse_expr.go:196) |

**Обоснование:** `значение`/`событие` обязаны работать как Primary внутри тел триггеров (`P(значение)`, `событие.клиент`). Поэтому они мигрируют в `parsePrimary` (становятся выражениями) и остаются недопустимыми на верхнем уровне. `когда` — самостоятельная декларация, ему место в каскаде `parseTopLevelItem`.

## Синтаксические диагностики шва (триггерящее условие)

Точные тексты — в diagnostics.md §TR-7.F; здесь только условие срабатывания и механизм.

| id | условие | механизм |
|---|---|---|
| SE-TRIGGER-KIND | после `когда` нет `метрика`/`событие`/`расписание` (`parseTriggerDecl` default) | `msgExpected("метрика, событие или расписание", peek())` |
| SE-EXPECT-COMPOP | после `метрика Ident` не CompOp-токен (`expectCompOp`) | `msgExpected("оператор сравнения", peek())` |
| SE-SCHEDULE-SPEC | после `расписание` нет `каждые`/`в` (`parseScheduleSpec` default) | `msgExpected("каждые или в", peek())` |
| TR-SYN-EMPTYBLOCK | тело триггера после `:` пустое (нет INDENT) | `msgEmptyBlock` (parseBlock, errors.go:17) |
| TR-SYN-UNEXPECTED | `значение`/`событие`/`{`/`}` на top-level | `msgUnexpected` (errors.go:45) |

> **Тексты-наземная правда.** Три новые синтаксические диагностики реализуются через **существующий** `msgExpected(expected, got)` (errors.go:40-41, формат `ожидалось '%s', получено '%s'`), подставляя `expected`. Отдельные const/функции (`msgTriggerKind`/`msgScheduleSpec`) — деталь оформления импл-чата; **текст фиксирован** через `msgExpected`. Имя `expected`-строки в `expectCompOp` — выбор импл-чата, дословный текст второй строки канонизирован.

## Точки врезки (файл:строки)

| Действие | Файл | Строки |
|---|---|---|
| `isUnexpectedTopLevel` минус `KW_WHEN` | `parse_stmt.go` | ~40-48 |
| `parseTopLevelItem` +ветка `когда` | `parse_stmt.go` | ~12-34 (перед `isUnexpectedTopLevel`) |
| `parseTriggerDecl`/`parseMetricTrigger`/`parseEventTrigger`/`parseScheduleTrigger`/`parseScheduleSpec`/`expectCompOp` | `parse_decl.go` | НОВЫЕ (рядом с `parseProcessDecl`) |
| `parsePrimary` +`KW_VALUE`/`KW_EVENT` | `parse_expr.go` | ~196-198 (после `KW_RUN`, перед `default`) |
| `startsExpression` +`KW_VALUE`/`KW_EVENT` | `parse_expr.go` | ~18-26 |

`parseBlock` (parse_stmt.go:157-177), `compOpOf` (parse_expr.go:236-249), `parseArgList`, `FieldExpr`-постфикс — **переиспользуются без изменений**.
