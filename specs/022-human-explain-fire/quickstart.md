# Quickstart: C5 — человеко-explain срабатывания

**Feature**: 022-human-explain-fire | **Date**: 2026-06-18
**Build**: `cd /Users/denis/dev/ladix/src && go build ./...`
**Test**: `cd /Users/denis/dev/ladix/src && go test ./...`

## Что делает фича (в двух словах)

При срабатывании метрик-триггера ВСЕГДА печатается одна человеко-читаемая строка «почему»:
имя метрики, снимок, оператор, порог, итог. На `run` — в stdout; на `serve` — в журнал демона с
маркером ребра `ложь→истина`. `inspect` («где») не меняется.

## Демо — путь run

Программа с метрик-триггером, который срабатывает:

```
$ ladix run пример.ladix --источник данные.csv
...
триггер 'выручка_30д < 3000000' сработал: выручка_30д = 2500000 (снимок) < 3000000 (порог) → истина
<вывод тела триггера>
```

Строка-explain печатается ДО тела.

## Демо — путь serve

В режиме демона при тике, где условие пересекает ребро `ложь→истина`:

```
$ ladix serve пример.ladix --db state.db
...
[лог] триггер 'выручка_30д < 3000000' сработал (ребро ложь→истина): выручка_30д = 2500000 (снимок) < 3000000 (порог) → истина
```

На следующем тике, где условие ВСЁ ЕЩЁ истинно (ребра нет) — новой строки НЕТ (тишина).

## Проверка приёмки (acceptance)

| Сценарий | Команда/тест | Ожидание |
|---|---|---|
| run fire → explain | `TestRunTriggerExplain` | run-строка §C-5.3 в out, ДО тела |
| serve fire по ребру → explain | `TestServeTriggerExplain` | serve-строка §C-5.3 в logf |
| serve уже-истина (нет ребра) → тишина | silence-замок | новой explain-строки нет |
| забыли протянуть порог | inversion-замок | serve-строка неверна → краснеет |
| golden co-land | 8 тестов §C-5.5 | обновлены, зелёные |
| NOT-affected целы | count/no-fire тесты | 0 правок, зелёные |

## Гейт-критерий (DoD)

1. `go build ./...` → OK; `go vet ./...` без замечаний; `gofmt` чисто.
2. `go test ./...` зелёный (включая 8 обновлённых golden + новые замки).
3. `EvalMetricCondition` возвращает 5 значений (`+threshold`); call-site `daemon/metrics.go:39`
   обновлён; других call-site нет.
4. `ProcessRuntime` = 8 методов (grep сигнатур); `Store` = 18; 0 новых SE/eval-кодов/KW/builtins/
   зависимостей; прод-дифф `store`/`engine` пустой.
5. `inspect` не тронут; NOT-affected golden (§C-5.5) не тронуты.
6. Инверсии кусают: снять протяжку порога → `TestServeTriggerExplain` краснеет; не делать co-land
   → 8 затронутых краснеют.

## Карта изменений (для имплементатора)

| Файл | Тип | Изменение |
|---|---|---|
| `internal/eval/trigger_daemon.go:31` | прод | `EvalMetricCondition` `+threshold value.Value` (все ветки; не-success → None) |
| `internal/eval/trigger_run.go:78-92` | прод | при fire печатать run-строку §C-5.3 в `i.out`; протянуть `i.out` в `runMetricTrigger` |
| `internal/daemon/metrics.go:39` | прод | обновить call-site (5 значений) |
| `internal/daemon/metrics.go:82` | прод | в ветке `if fired` печатать serve-строку §C-5.3 через `d.logf` |
| `internal/daemon/tick_test.go` | тест | MUST UPDATE ×2 |
| `cmd/ladix/trigger_golden_test.go` | тест | MUST UPDATE ×5 |
| `internal/eval/metric_window_golden_test.go` | тест | MUST UPDATE ×1 |
| новый `*explain_test.go` (eval + daemon) | тест | TestRunTriggerExplain / TestServeTriggerExplain + silence + inversion |
| `inspect.go` | — | НЕ ТРОГАТЬ |
| прод `store`/`engine` | — | ПУСТОЙ дифф |
