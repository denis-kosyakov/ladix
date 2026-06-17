# Data Model: B1 «Захват результата `вызвать` как выражения»

**Feature**: 013-call-result | **Date**: 2026-06-17 | **Источник**: `docs/automation-model.md` §AU-3

B1 вводит ОДИН новый AST-узел и ОДИН новый метод несущего шва. Никаких новых типов-значений, структур хранилища или persisted-сущностей (пустой дифф `store`). Ниже — структурное описание обеих сущностей и их инвариантов.

## E-1. AST-узел `CallExternalExpr` (`internal/ast/expr.go`)

Выражение захвата результата внешнего вызова `вызвать Ident(аргументы)`. По образцу `RunProcessExpr` (`ast/expr.go:69`).

```go
// CallExternalExpr — захват результата внешнего вызова: вызвать Target(Args) как
// ВЫРАЖЕНИЕ (B1, §AU-3). Имя CallExpr занято постфикс-вызовом f(args) (:31) →
// узел B1 называется CallExternalExpr. Цель — логическое имя (строка), не символ.
// Pos() = токен вызвать. Постфикс на результат — отдельной цепочкой parsePostfix.
type CallExternalExpr struct {
    exprBase
    Target Ident         // логическое имя цели (crm, ИТ, …)
    Args   []Expression  // позиционные аргументы (могут быть пусты)
}

// NewCallExternalExpr строит узел; pos — позиция токена вызвать.
func NewCallExternalExpr(pos Position, target Ident, args []Expression) *CallExternalExpr {
    return &CallExternalExpr{exprBase: exprBase{base{pos}}, Target: target, Args: args}
}
```

| Поле | Тип | Назначение | Инвариант |
|---|---|---|---|
| `exprBase` | встраивание | `Pos()` + маркер «это выражение» | `Pos()` = позиция токена `вызвать` (Принцип IV, в рунах с 1) |
| `Target` | `ast.Ident` | логическое имя цели | свободное имя-строка, НЕ резолвится в символ программы; арность не проверяется |
| `Args` | `[]Expression` | позиционные аргументы | может быть пустым (`вызвать сервис()`); порядок — исходный (вычисление слева направо в eval) |

**Инварианты узла**:
- **AST-1**: имя ОБЯЗАНО быть `CallExternalExpr`/`NewCallExternalExpr` (не `CallExpr` — занято постфикс-вызовом). Нарушение → redeclared в пакете `ast`.
- **AST-2**: узел реализует `ast.Expression` через `exprBase` (как `RunProcessExpr`); `Pos()` наследуется, не переопределяется.
- **AST-3**: листовость `ast` сохранена — узел НЕ импортирует `internal/errors` (Принцип VII).
- **AST-4**: структурно отличён от `CallExpr` (постфикс `f(args)`): у `CallExternalExpr` цель — `Ident` (имя), у `CallExpr` callee — произвольное `Expression`.

## E-2. Метод шва `CallExternalResult` (`internal/eval/runtime.go`, интерфейс `ProcessRuntime`)

Расширение несущего шва eval↔engine РОВНО одним методом (7→8, §AU-2 / D-AU-1).

```go
type ProcessRuntime interface {
    StartProcess(name string, args []value.Value) (string, error)              // существ. 1/7
    AssignProcessVar(name string, v value.Value) error                         // существ. 2/7
    CallExternal(target string, args []value.Value) error                      // существ. 3/7 — statement-форма (СОХРАНЯЕТСЯ)
    CallExternalResult(target string, args []value.Value) (value.Value, error) // ★ НОВЫЙ (B1) — выражение-форма; вставлен рядом с CallExternal
    Notify(target string, args []value.Value) error                            // существ. 4/7
    InstanceStatus(id string) (status string, ok bool, err error)              // существ. 5/7
    InstanceVariables(id string) (vars value.Запись, ok bool, err error)       // существ. 6/7
    UserTasks(assignee string) ([]value.Запись, error)                         // существ. 7/7 — итого 7 старых + 1 новый = 8
}
```

| Аспект | Контракт |
|---|---|
| Вход | `target string` (логическое имя), `args []value.Value` (уже вычислены eval'ом слева направо) |
| Выход | `(value.Value, error)` — значение результата внешней системы + ошибка |
| Под дефолт-стабом (B1/v1) | печатает прежнюю строку `[вызов] target(args)`; возвращает `(value.None, nil)` |
| Под HTTP-драйвером (B2, вне scope) | декодированный ответ (§AU-4.3) |
| Делегирование | существующий `CallExternal(t, a) error` ОБЯЗАН делать `_, err := CallExternalResult(t, a); return err` — эффект не дублируется |

**Инварианты шва**:
- **SEAM-1**: РОВНО +1 метод (7→8); 7 существующих сигнатур байт-в-байт неизменны.
- **SEAM-2**: метод объявлен в `eval` (пакет `eval` НЕ импортирует `store`/`engine`); реализация — в `engine`; ребро `engine→eval` однонаправленно.
- **SEAM-3**: `engine.Engine` удовлетворяет обновлённому интерфейсу — `var _ eval.ProcessRuntime = (*Engine)(nil)` (`engine/runtime.go:16`) компилируется.
- **SEAM-4**: `CallExternal` делегирует `CallExternalResult` (эффект один раз) → golden §EN-7 печать-стаба байт-в-байт цел.

## E-3. Поведение eval (кейс `*ast.CallExternalExpr` в `evalExpr`)

Не сущность данных, а правило вычисления; фиксируется для полноты контракта.

```
evalCallExternal(env, c *ast.CallExternalExpr) (value.Value, error):
  if i.runtime == nil: return runtimeErr(c.Pos(), "движок процессов не подключён")  // защитный инвариант
  args := вычислить c.Args слева направо (как evalArgs); при ошибке аргумента — пробросить
  v, err := i.runtime.CallExternalResult(c.Target.Name, args)
  if err != nil: return nil, runtimeErrWrap(c.Pos(), err)   // ОшибкаВыполнения{Pos, Msg, Cause}
  return v, nil
```

| Инвариант | Описание |
|---|---|
| **EVAL-1** | аргументы вычисляются строго слева направо (детерминизм) |
| **EVAL-2** | ошибка шва заворачивается `runtimeErrWrap(c.Pos(), err)` → единая категория `ОшибкаВыполнения` (§AU-3.3); под стабом ошибки нет → возвращается `value.None` |
| **EVAL-3** | значение возвращается как `value.Value` и присваивается переменной (`присвоить`) / используется как аргумент / элемент списка |

## Сводка изменений по сущностям

| Сущность | Тип изменения | Файл | Инвариант якоря |
|---|---|---|---|
| `CallExternalExpr` + `NewCallExternalExpr` | НОВАЯ | `internal/ast/expr.go` | аддитивность фронтенда (§3) |
| `ProcessRuntime.CallExternalResult` | НОВЫЙ метод (7→8) | `internal/eval/runtime.go` | шов §AU-2, eval без store/engine |
| `Engine.CallExternalResult` + делегирование `CallExternal` | НОВАЯ реализация + правка | `internal/engine/runtime.go` | golden §EN-7 цел |
| кейс `*ast.CallExternalExpr` | НОВЫЙ кейс | `internal/eval/expr.go` | детерминизм, обёртка ошибки |
| **`Store`, SQLite-кодек, value-типы** | **БЕЗ ИЗМЕНЕНИЙ** | — | Store=15, пустой дифф `store` |
