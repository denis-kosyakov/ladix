# Contract: Шов `ProcessRuntime` 7→8 (метод `CallExternalResult`)

**Feature**: 013-call-result | **Граница**: `internal/eval` ↔ `internal/engine` | **Источник**: §AU-2 / §AU-3.3 (D-AU-1)

## C-SEAM-1. Расширение интерфейса (объявление в `eval`)

`ProcessRuntime` (`internal/eval/runtime.go`) получает РОВНО один новый метод. Существующие 7 — дословно как сейчас, НЕ трогаются.

```go
// CallExternalResult — выражение-форма «вызвать» (B1, §AU-3): эффект внешнего
// вызова + захват результата. Под дефолт-стабом печатает прежнюю строку и
// возвращает (value.None, nil); под HTTP-драйвером (B2) — декодированный ответ.
CallExternalResult(target string, args []value.Value) (value.Value, error)
```

| Пункт | Контракт |
|---|---|
| Количество методов | 7 → **8** (РОВНО +1) |
| 7 существующих | сигнатуры байт-в-байт неизменны (`StartProcess`, `AssignProcessVar`, `CallExternal`, `Notify`, `InstanceStatus`, `InstanceVariables`, `UserTasks`) |
| Где объявлен | в `eval` (не в `engine`/`store`) — чтобы `eval` не импортировал `store`/`engine` |
| Импорты `eval` | после правки `eval` НЕ импортирует `store`, НЕ импортирует `engine` (статически проверяемо) |

## C-SEAM-2. Реализация и делегирование (в `engine`)

```go
// engine/runtime.go
func (e *Engine) CallExternalResult(target string, args []value.Value) (value.Value, error) {
    // дефолт-стаб (B1/v1): прежняя печать + value.None
    parts := make([]string, len(args))
    for k, a := range args { parts[k] = value.String(a) }
    fmt.Fprintf(e.out, "[вызов] %s(%s)\n", target, strings.Join(parts, ", "))
    return value.None, nil
}

// существующий statement-метод делегирует — эффект НЕ дублируется
func (e *Engine) CallExternal(target string, args []value.Value) error {
    _, err := e.CallExternalResult(target, args)
    return err
}
```

| Инвариант | Проверка |
|---|---|
| **C-SEAM-2.1** делегирование | `CallExternal` вызывает `CallExternalResult` и отбрасывает значение; печать `[вызов] …` происходит РОВНО один раз |
| **C-SEAM-2.2** соответствие интерфейсу | `var _ eval.ProcessRuntime = (*Engine)(nil)` (`engine/runtime.go:16`) компилируется |
| **C-SEAM-2.3** golden §EN-7 | строка `[вызов] %s(%s)` байт-в-байт прежняя; `Notify` НЕ трогается |
| **C-SEAM-2.4** возврат стаба | `CallExternalResult` под стабом возвращает `(value.None, nil)` |

## C-SEAM-3. Контракт eval-стороны (вызов через шов)

```
кейс *ast.CallExternalExpr в evalExpr:
  args ← вычислить c.Args слева направо
  v, err ← i.runtime.CallExternalResult(c.Target.Name, args)
  err≠nil → return nil, runtimeErrWrap(c.Pos(), err)   // ОшибкаВыполнения{Pos,Msg,Cause}
  return v, nil
```

| Инвариант | Проверка |
|---|---|
| **C-SEAM-3.1** | аргументы слева направо (детерминизм) |
| **C-SEAM-3.2** | ошибка шва → `runtimeErrWrap(c.Pos(), err)`, единая категория `ОшибкаВыполнения` (§AU-3.3) |
| **C-SEAM-3.3** | под стабом err=nil → возвращается `value.None`, исполнение без ошибки |
| **C-SEAM-3.4** | защита `i.runtime == nil` → `runtimeErr(c.Pos(), …)` (как `evalRunProcess`) |

## Тест-замки контракта (tests-first; инверсии — в tasks.md)

| Замок | Что фиксирует | Инверсная мутация (обязана покраснить) |
|---|---|---|
| `TestProcessRuntimeHas8Methods` / компиляц. фейк | интерфейс = 8 методов, фейк реализует все | удалить `CallExternalResult` из интерфейса → фейк не компилируется / тест red |
| `TestCallExternalDelegatesToResult` | `CallExternal` печатает РОВНО одну строку (нет двойного эффекта) | вернуть `CallExternal` к собственной `Fprintf` (не делегировать) → двойная печать / расхождение |
| `TestEvalCallExternalStubReturnsNone` | под фейк-стабом результат = `value.None`, err=nil | стаб вернуть `value.Строка{…}` → значение ≠ None |
| `TestEvalCallExternalArgsLeftToRight` | порядок вычисления аргументов | поменять порядок цикла аргументов → порядок ≠ исходный |
| `TestEvalCallExternalWrapsError` | ошибка фейк-runtime → `ОшибкаВыполнения` с `Pos` узла `вызвать` | убрать `runtimeErrWrap` (вернуть raw err) → нет Pos/тип ≠ ОшибкаВыполнения |
| golden §EN-7 (существующие) | печать-стаб `[вызов]`/`[уведомление]` байт-в-байт | изменить строку стаба → golden red |
