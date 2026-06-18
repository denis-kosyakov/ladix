# Contract: §C-5.5 golden-churn co-land (MANDATORY)

**Feature**: 022-human-explain-fire
**Anchor**: `docs/reliability-model.md` §C-5.5 + `.m3-ledger/digest-anchor.md` (C5 must-update /
NOT-affected списки).

> Always-on explain добавляет НОВУЮ строку в вывод путей со срабатыванием → exact-match существующих
> golden'ов ломается. Обновить co-land РОВНО эти 8 (и только эти). Без co-land гейт `go test`
> недостижим.

## MUST UPDATE — 8 тестов в 4 файлах

### serve (exact-match `out.String()`; ребро+тело в один writer `daemon.go:53-58`)

| # | Файл | Тест | Текущее ожидание | Правка |
|---|---|---|---|---|
| 1 | `src/internal/daemon/tick_test.go` | `TestTickPhaseOrderAllThreeFire` | `"E\nM\nS\n"` | вставить serve-explain строку метрики в ожидаемый вывод (порядок фаз сохранить) |
| 2 | `src/internal/daemon/tick_test.go` | `TestTickFourPhasesOrder` | `"E\nM\nS\nD\n"` | то же |

### run (exact-match stdout / `i.out`)

| # | Файл | Тест | Правка |
|---|---|---|---|
| 3 | `src/cmd/ladix/trigger_golden_test.go` | `TestRunTriggerFiresGolden` | добавить run-explain строку §C-5.3 в ожидаемый stdout |
| 4 | `src/cmd/ladix/trigger_golden_test.go` | `TestRunTriggerDBRepeatEphemeral` | то же |
| 5 | `src/cmd/ladix/trigger_golden_test.go` | `TestRunTriggerMultiMetricOrderGolden` | **два fire → две explain-строки** (порядок) |
| 6 | `src/cmd/ladix/trigger_golden_test.go` | `TestRunTriggerMixedKindsOrderGolden` | добавить run-explain строку(и) |
| 7 | `src/cmd/ladix/trigger_golden_test.go` | `TestRunTriggerBodyReadShadowGolden` | **два fire → две explain-строки** |
| 8 | `src/internal/eval/metric_window_golden_test.go` | `TestWindowMetricTriggerFires` | ожидание `"оконная метрика: 23\n"` И `stubs.Len()==0` → explain ДОЛЖЕН идти в **out** (не в w); обновить ожидаемый out, `stubs.Len()==0` остаётся |

**ИТОГО must-update = 8** (tick_test.go ×2, trigger_golden_test.go ×5, metric_window_golden_test.go ×1).

## NOT-AFFECTED — НЕ ТРОГАТЬ (явный guard)

| Категория | Тесты | Почему не ломается |
|---|---|---|
| count()/contains() демона | `metrics_test.go`, `schedule_test.go`, `daemon_test.go` (MFIRE), `m2_endtoend` (sink.count) | проверяют счётчики/подстроки, не exact-match → explain не влияет |
| no-fire / error | `source_negatives` (runtimeForceTrigger), `TestWindowMetricTriggerSilent`, events-FIFO (`want=A\nB\nC`) | нет срабатывания / другой канал → explain не печатается |
| inspect | `inspect.go:85-117` и его тесты | «где», C5 не меняет |

**Guard-критерий**: после co-land `git diff` НЕ затрагивает файлы/строки NOT-affected тестов (0
правок); прогон NOT-affected тестов зелёный без изменений.

## Verification

- Полный `go test ./...` зелёный ПОСЛЕ co-land (компиляция + exact-match).
- Инверсия §C-7 п.6: если co-land НЕ сделан — затронутые 8 краснеют (доказывает обязательность).
- Новые замки (`TestRunTriggerExplain`/`TestServeTriggerExplain`/silence/inversion) зелёные.
