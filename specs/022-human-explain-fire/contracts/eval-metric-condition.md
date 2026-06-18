# Contract: EvalMetricCondition signature widening

**Feature**: 022-human-explain-fire
**File**: `src/internal/eval/trigger_daemon.go:31`
**Anchor**: §C-5.1 «единственная протяжка M3»

## Subject

`EvalMetricCondition` — СВОБОДНАЯ функция (method on `*Interpreter`), НЕ метод интерфейса
`eval.ProcessRuntime`. Расширяется на одно возвращаемое значение `threshold value.Value`.

## Before → After

```go
// BEFORE (live, trigger_daemon.go:31):
func (i *Interpreter) EvalMetricCondition(spec *ast.MetricTrigger) (cur bool, snapshot value.Value, ok bool, err error)

// AFTER:
func (i *Interpreter) EvalMetricCondition(spec *ast.MetricTrigger) (cur bool, snapshot value.Value, threshold value.Value, ok bool, err error)
```

## Behavioral contract

| Ветка | cur | snapshot | threshold | ok | err |
|---|---|---|---|---|---|
| успех (вычислимо) | bool результат сравнения | значение метрики | **значение порога** | true | nil |
| пустая метрика / нет данных | false | None/что было | **value.None** | false | nil |
| несравнимые типы | false | значение | **value.None** | false | nil |
| ошибка вычисления | false | — | **value.None** | false | err |

**Инвариант**: `threshold` возвращается во ВСЕХ ветках (тоталь-функция; не-success → `value.None`).

## Call-sites

- **ЕДИНСТВЕННЫЙ**: `src/internal/daemon/metrics.go:39`
  ```go
  // BEFORE: cur, snapshot, computable, err := d.interp.EvalMetricCondition(spec)
  // AFTER:  cur, snapshot, threshold, computable, err := d.interp.EvalMetricCondition(spec)
  ```
  `threshold` далее используется в serve-explain (`metrics.go:82`).

## Invariants (MUST hold)

- `eval.ProcessRuntime` остаётся **8 методов байт-цел** — эта функция в интерфейс НЕ входит.
- `eval` НЕ импортирует `store`/`engine` (граф без циклов; принцип VII).
- 0 новых SE/eval-кодов; explain — не ошибка.
- Детерминизм: возврат зависит только от `spec` и состояния интерпретатора, не от часов.

## Test impact

- run-путь НЕ зависит от этой функции (у `runMetricTrigger` свой `threshVal`).
- serve-путь: после расширения порог доступен в точке печати.
- Inversion-замок: убрать протяжку threshold (передать nil/мусор) → `TestServeTriggerExplain`
  печатает неверный/пустой порог → краснеет.
