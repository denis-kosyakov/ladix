# Contract: Драйвер `ExternalCaller` + Option движка

**Feature**: 014-real-effects | **Граница**: `internal/engine` | **Источник**: §AU-4.1

## Интерфейс

```go
type ExternalCaller interface {
    Call(target string, args []value.Value) (value.Value, error)  // вызвать → результат
    Notify(target string, args []value.Value) error               // уведомить → эффект
}
```

## Option и дефолт

```go
func WithExternalCaller(c ExternalCaller) Option   // engine/clock.go, рядом с WithClock
// NewEngine: e.caller = printCaller{out: e.out}    // дефолт ДО применения opts...
```

## Делегирование методов движка

```go
func (e *Engine) CallExternalResult(target string, args []value.Value) (value.Value, error) { return e.caller.Call(target, args) }
func (e *Engine) CallExternal(target string, args []value.Value) error { _, err := e.caller.Call(target, args); return err }
func (e *Engine) Notify(target string, args []value.Value) error { return e.caller.Notify(target, args) }
```

## Контрактные обязательства

| # | Обязательство | Проверка |
|---|---|---|
| C-1 | `NewEngine` БЕЗ `WithExternalCaller` → `e.caller` = `printCaller{out}` | контракт-тест: дефолт-вывод = стаб §EN-7 |
| C-2 | `WithExternalCaller(c)` ставит `e.caller = c` (после дефолта) | тест: с фейк-драйвером методы движка зовут фейк |
| C-3 | `CallExternal` = `Call` с отбросом значения (эффект РОВНО один раз) | тест: счётчик вызовов фейка = 1 на `вызвать`-statement |
| C-4 | `var _ eval.ProcessRuntime = (*Engine)(nil)` держится (8 методов, B1) | компиляция + контракт-замок шва |
| C-5 | `printCaller`/`webhookCaller` оба реализуют `ExternalCaller` | `var _ ExternalCaller = printCaller{}` / `= webhookCaller{}` |

## Инверсные мутпробы

- C-1: дефолт `e.caller = webhookCaller{}` (вместо `printCaller`) → §EN-7 golden КРАСНЕЕТ (нет печати стаба).
- C-3: `CallExternal` зовёт `Call` дважды или печатает сам → счётчик/golden КРАСНЕЕТ (двойной эффект).
- C-2: `WithExternalCaller` игнорирует аргумент → httptest-тест видит стаб вместо POST → КРАСНЕЕТ.

## Границы

- Поле `caller` — на инстансе движка (DI, Принцип V); не пакет-глобал.
- Шов `ProcessRuntime` НЕ расширяется — интерфейс `ExternalCaller` ВНУТРЕННИЙ для `engine`, eval о нём не знает.
