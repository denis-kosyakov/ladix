# Quickstart: Интерпретатор Ladix (003-interpreter-eval)

**Feature**: 003-interpreter-eval | **Plan**: [plan.md](./plan.md)

Как разработчику начать реализацию интерпретатора и проверять её. На этом шаге создаются пакеты `internal/value` (листовой) и `internal/eval`, команда `cmd/ladix`, и расширяется `internal/errors` (три категории). Лексер (001), парсер (002) и AST **НЕ** меняются — это стабильный вход (контракт [../002-parser-ast/contracts/ast.md](../002-parser-ast/contracts/ast.md)). Все примеры команд — от корня Go-модуля `src/`.

## 0. Каркас (Фаза 0)

```bash
cd src

# Новый листовой пакет значений (только stdlib Go)
#   internal/value/value.go     — интерфейс Value{ TypeName() string }
#   internal/value/scalar.go    — Целое/Дробное/Строка/Булево/Пусто (+ var None)
#   internal/value/list.go      — Список (*[]Value, ссылочный)
#   internal/value/record.go    — Запись (смоделирована, не конструируется)
#   internal/value/deferred.go  — Дата/Длительность/Период (определены, не конструируются)
#   internal/value/repr.go      — строковое представление §7
#   internal/value/equal.go     — равенство/упорядочивание §3.3

# Расширение пакета ошибок (по образцу lexerror.go/parserror.go)
#   internal/errors/semanticerror.go  — СемантическаяОшибка (стадия 3)
#   internal/errors/typeerror.go      — ОшибкаТипа (стадия 4)
#   internal/errors/runtimeerror.go   — ОшибкаВыполнения (стадия 4)

# Новый пакет интерпретатора (→ ast, value, errors)
#   internal/eval/environment.go interpreter.go signal.go
#   internal/eval/expr.go arith.go stmt.go call.go analyze.go
#   internal/eval/builtins*.go

# Команда
#   cmd/ladix/main.go  — run + --max-depth, recover-барьер, коды 0/1/2
```

Граф зависимостей без циклов: `value`→stdlib; `errors`→stdlib; `eval`→`ast,value,errors`; `cmd/ladix`→`lexer,parser,ast,eval,errors`. `value`/`errors`/`ast` остаются листовыми (конституция VII, Guardrail 1).

## 1. Цикл tests-first (конституция VI, FR-040)

Тесты — часть **каждой** задачи (table-driven, co-located `*_test.go`), пишутся первыми и обязаны падать до реализации. Печать перехватывается через инжектированный `Interpreter.out` (`bytes.Buffer`).

```bash
cd src
go test ./internal/value/...    # типы, печать §7 (2175.0!), равенство/упорядочивание
go test ./internal/eval/...     # выражения, statements, scope, функции/рекурсия, стдлиб, ошибки
go test ./cmd/ladix/...         # golden-прогон run: 6 примеров (код 0) + ошибка.ladix (код 1)
go test ./...                   # весь модуль
```

Эталоны сверки:

- Значения и печать §7 — [contracts/values.md](./contracts/values.md) (C-2/C-3/C-4/C-5).
- Встроенные (поведение, арность, ошибки) — [contracts/builtins.md](./contracts/builtins.md) + `docs/stdlib.md`.
- Тексты и позиции ошибок — [contracts/runtime-errors.md](./contracts/runtime-errors.md) (RE-1, exact-match) + `docs/eval-model.md §8.3`.
- Golden stdout/stderr — таблица spec.md «Golden-прогон» + `docs/eval-model.md §10.3`.

## 2. Гейты качества (конституция I, FR-039, SC-009)

```bash
cd src
gofmt -l .          # пусто = ок
go vet ./...        # без замечаний
go build ./...      # собирается
go test -race ./... # без гонок
```

Все четыре ОБЯЗАНЫ быть чистыми в каждой задаче.

## 3. Чек-лист приёмки по фазам (по приоритетам US)

- **Фаза 0 (каркас)**
  - [ ] 10 типов `Value`, 6 конструируемых; `var None = Пусто{}`; `Список` ссылочный (Guardrail 2)
  - [ ] печать §7: `6/3`→`3.0`, `2175.0` с принудительной `.0`; строки без кавычек в списках (SC-001)
  - [ ] три типа ошибок с двухстрочным `Error()`, `errors.As` (Guardrail 14)
- **Фаза P1 (US1+US2, MVP)** 🎯
  - [ ] арифметика §3.3: `2+3*4`→`14`; `7/2`→`3.5`; `7//2`→`3`; `7%2`→`1`; `Дроб%Дроб`→fmod; смешанное `%`→`ОшибкаТипа` (SC-004, Guardrail 5)
  - [ ] промоушен `Цел+Дроб`; короткое замыкание `или`/`и` не вычисляет правый операнд (Guardrail 6)
  - [ ] индексация в рунах; `[1,2][5]`→`RT-INDEX-RANGE`
  - [ ] `пусть`/`=`/`печать`; функциональный scope; `печать(b)` до `пусть b`→`RT-VAR-UNDECLARED` (SC-005)
  - [ ] golden `hello`, `арифметика` (код 0); `ошибка.ladix` стр.5 кол.14 (SC-002)
- **Фаза P2 (US3)**
  - [ ] `если/иначе если/иначе` по `IsFinal()`; strict-`Булево` (`если 1:`→`TY-COND`)
  - [ ] `пока`; `прервать`/`продолжить` поглощаются; `для x` Define-если-нет, видна после цикла, на `[]` не создаётся (Guardrail 9)
  - [ ] мутация во время итерации→`RT-LIST-MUTATED`; `прервать` вне цикла→`SEM-BREAK-CTX`
  - [ ] golden `условие`, `цикл` (код 0)
- **Фаза P3 (US4)**
  - [ ] резолв вызова (переменные→funcs→builtins); кадр `parent=global`; рекурсия независима
  - [ ] `факториал(5)`→`120`; голый `вернуть`→`пусто`; убегающая рекурсия→`RT-DEPTH`; арность→`SEM-ARITY` (SC-006)
  - [ ] 23 активных встроенных по `docs/stdlib.md`; `булево(x)` truthy; 12 deferred→`SEM-DEFERRED-BUILTIN` (SC-007)
  - [ ] golden `функция` (`2175.0`/`0`), `факториал` (`120`/`1`) (код 0)
- **Фаза US5-сквозная + CLI**
  - [ ] `Analyze` fail-fast (forward-функции, повтор `пусть`, контекст сигналов, фикс. арность)
  - [ ] deferred-граница: reserved-узлы/декларатив/`DurationLit`→`SEM-DEFERRED-CONSTRUCT`; period-имена→`RT-VAR-UNDECLARED` (Guardrail 16)
  - [ ] весь реестр §8.3 (RE-1) по golden-кейсу на сообщение (SC-003)
  - [ ] recover→`внутренняя ошибка интерпретатора` без stack trace; коды 0/1/2 (SC-008)
- **Фаза Polish**
  - [ ] сквозной golden-прогон 6 примеров + `ошибка.ladix` (SC-001); `gofmt`/`go vet`/`-race` зелёные (SC-009)

## 4. Границы (НЕ делать на этом шаге)

- НЕ менять грамматику/приоритеты/AST (стабильный вход 002).
- НЕ реализовывать `store`/`engine`/`serve`/декларатив/движок процессов/таймеры/SQLite (engine 004+).
- НЕ конструировать `Запись`/`Дата`/`Длительность`/`Период` (типы смоделированы/определены, ворота закрыты).
- НЕ реализовывать 12 deferred-встроенных — только заглушки «не поддерживается».
- НЕ предопределять period-константы (`ежедневно`…`ежегодно`).
- НЕ паниковать в штатных путях; НЕ использовать `panic/recover` для control-flow (только Signal).
- НЕ накапливать ошибки в eval (fail-fast); НЕ синхронизировать `§13.4` (D1 — тексты из §8.3).

## 5. Куда смотреть при сомнении (конституция IX)

- **Поведение**: `docs/eval-model.md §1–§10` (единственный связывающий источник; §8.3 — тексты, §10.3 — golden).
- **Встроенные**: `docs/stdlib.md`.
- **Типы/выражения/области/функции**: `SPEC.md §4–§8` (фон).
- **Архитектура/граф пакетов/Value**: `ARCHITECTURE.md §2.1/§3/§4/§5/§7.7`.
- **AST-вход**: `specs/002-parser-ast/contracts/ast.md` + `specs/002-parser-ast/data-model.md`.
- **Решения**: [research.md](./research.md) (D-R1..D-R16) и [plan.md](./plan.md) (Guardrails 1–18).

Пробел/противоречие → **остановиться и спросить**, не додумывать. Любое доопределение фиксируется явно (research.md/contracts), не молча.
