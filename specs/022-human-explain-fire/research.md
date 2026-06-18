# Research: C5 — человеко-explain срабатывания

**Feature**: 022-human-explain-fire | **Date**: 2026-06-18
**Anchor**: `docs/reliability-model.md` §C-5 + `.m3-ledger/digest-anchor.md` (C5) +
`.m3-ledger/digest-seams.md`. Все NEEDS CLARIFICATION разрешены анкором (источник истины,
non-interactive). Ниже — консолидированные решения.

---

## R-1. Кто/где печатает на пути `run`

- **Decision**: Печатать строку-explain в `i.out` (стандартный поток интерпретатора), протянув
  `i.out` в `runMetricTrigger` (`eval/trigger_run.go:78-92`, точка fire-if-true). Печать ДО тела
  триггера.
- **Rationale**: В точке fire всё уже в области видимости: `spec.Metric.Name`, `metricVal`
  (снимок), `threshVal` (порог), `ast.BinOp(spec.Op)`, `fired`. Писатель тела — `i.out`, НЕ
  параметр `w` из `RunTriggers` (`w` несёт только заглушки событий/расписания и сводку).
  `metric_window_golden_test.go:102-104` явно различает: тело→`out`, RunTriggers-сводка→`w`.
  Поэтому explain ОБЯЗАН идти в `out`, иначе `TestWindowMetricTriggerFires` ломает обе проверки
  (`"оконная метрика: 23\n"` в out И `stubs.Len()==0`).
- **Alternatives considered**: печать в `w` — отвергнуто (ломает writer-инвариант и
  `TestWindowMetricTriggerFires`, который требует `stubs.Len()==0`).

## R-2. Кто/где печатает на пути `serve`

- **Decision**: Печатать через `d.logf` (`daemon.go:53`) в ветке `if fired {`
  (`daemon/metrics.go:82`), ДО `safeFire`/`fireBody`. Формулировка С упоминанием ребра
  `ложь→истина`.
- **Rationale**: На `serve` срабатывание = переход через ребро `fired := ts.LastBool!=nil &&
  !*ts.LastBool && cur` (`metrics.go:72`). Печать только в этой ветке гарантирует FR-008 (тишина
  на тике уже-истина без ребра). Упоминание ребра обязательно: иначе оператор спутает тик-fire с
  тиком, где `cur` истинно, но триггер не сработал (ребра нет).
- **Alternatives considered**: печать при `cur==true` (без проверки ребра) — отвергнуто: шумит на
  каждом тике уже-истина, вводит в заблуждение (FR-008 нарушен).

## R-3. Откуда взять порог на пути `serve` → расширение EvalMetricCondition

- **Decision**: Расширить возврат свободной функции `EvalMetricCondition`
  (`eval/trigger_daemon.go:31`):
  ```
  // было:
  func (i *Interpreter) EvalMetricCondition(spec *ast.MetricTrigger)
      (cur bool, snapshot value.Value, ok bool, err error)
  // станет:
  func (i *Interpreter) EvalMetricCondition(spec *ast.MetricTrigger)
      (cur bool, snapshot value.Value, threshold value.Value, ok bool, err error)
  ```
  Обновить ЕДИНСТВЕННЫЙ call-site `daemon/metrics.go:39`.
- **Rationale**: Порог (`threshVal`) сейчас вычисляется и ВЫБРАСЫВАЕТСЯ внутри функции
  (`trigger_daemon.go:~42`, не возвращается). serve-печать нуждается в нём. Это СВОБОДНАЯ функция
  на `*Interpreter`, НЕ метод интерфейса `ProcessRuntime` → `ProcessRuntime` остаётся 8 методов
  байт-цел; threshold швов не пересекает. Один call-site → дешёвая правка.
- **Alternatives considered**: (a) пере-вычислять порог в `daemon` — дублирование eval-логики,
  нарушает раскладку/детерминизм. (b) сделать порог полем `TriggerState`/`Store` — переусложнение,
  нарушает «empty diff store» и Store=18. Оба отвергнуты.

## R-4. Поведение «нет данных»/несравнимые типы (порог во всех ветках)

- **Decision**: `EvalMetricCondition` возвращает `threshold` во ВСЕХ ветках. В ветках пустой
  метрики/несравнимых типов → `threshold = value.None` (`Пусто`), `cur=false`/`ok=false`. Тогда
  `fired=false` → explain НЕ печатается.
- **Rationale**: FR-007/FR-009: при «нет данных» триггер не срабатывает, строка-explain не
  выводится. Возврат None во всех ветках сохраняет тоталь-функцию (нет частичного возврата) и
  детерминизм. inversion-замок (R-7) проверяет, что забыть протянуть порог → serve-explain
  печатает неверный/пустой порог → краснеет.
- **Alternatives considered**: возвращать порог только в success-ветке (zero value в остальных) —
  семантически то же (None), но менее явно; явный None во всех ветках предпочтён для читаемости.

## R-5. Точный формат строки (golden, exact-match)

- **Decision** (дословно из §C-5.3):
  - **run** (`i.out`):
    `триггер '<имя> <оп> <порог>' сработал: <имя> = <снимок> (снимок) <оп> <порог> (порог) → истина`
  - **serve** (`d.logf`):
    `триггер '<имя> <оп> <порог>' сработал (ребро ложь→истина): <имя> = <снимок> (снимок) <оп> <порог> (порог) → истина`
  - `<имя>` = `spec.Metric.Name`; `<оп>` = `BinOp.String()` (`ast/op.go:35`); `<снимок>`/`<порог>` =
    `value.String(v)` (`value/repr.go:20`) — числа БЕЗ подчёркиваний (`3000000`, не `3_000_000`).
  - Пример run: `триггер 'выручка_30д < 3000000' сработал: выручка_30д = 2500000 (снимок) < 3000000 (порог) → истина`
  - Пример serve: `триггер 'выручка_30д < 3000000' сработал (ребро ложь→истина): выручка_30д = 2500000 (снимок) < 3000000 (порог) → истина`
- **Rationale**: Дословный канон анкора (принцип VIII — тексты берутся дословно из docs).
  Детерминизм (FR-013): `value.String` и `BinOp.String` чисты, не зависят от часов.
- **Alternatives considered**: иные формулировки — отвергнуто (нарушает дословность §C-5.3 и
  ломает планируемые exact-match замки).

## R-6. Одно- vs двухстрочный канон (принцип VIII / §13)

- **Decision**: Explain — ОДНОстрочный (по §C-5.3). НЕ применять двухстрочный «Ошибка в строке N,
  колонка M:» формат §13.
- **Rationale**: §13 двухстрочный формат — для ОШИБОК с позицией исходника. Explain — диагностика
  наблюдаемости при УСПЕШНОМ срабатывании, не ошибка (нет SE/eval-кода, нет позиции). §C-5.2/§C-5.3
  задают одну строку дословно. Записано в Complexity Tracking плана как санкционированное анкором
  отступление (принцип IX: анкор — источник истины).
- **Alternatives considered**: навязать двухстрочный формат — семантически неверно (не ошибка),
  противоречит дословному §C-5.3. Отвергнуто.

## R-7. Стратегия тестов: новые замки + инверсии

- **Decision**:
  - `TestRunTriggerExplain` (eval, exact-match) — fire через run → строка run-формы §C-5.3 в out.
  - `TestServeTriggerExplain` (daemon, exact-match) — fire по ребру → строка serve-формы §C-5.3 в
    logf-буфере.
  - **Silence-замок**: тик `serve` без ребра (уже-истина, `LastBool=true`, `cur=true`) → новой
    explain-строки НЕТ (FR-008).
  - **Inversion-замок**: НЕ протянуть `threshold` (nil/мусор) → serve-explain печатает неверный/
    пустой порог → `TestServeTriggerExplain` краснеет. Мутпроба §C-7 п.6.
- **Rationale**: §C-5.4. Конституция VI — тесты вместе с правкой. Инверсии доказывают, что замок
  кусает (мутационная плотность).
- **Alternatives considered**: только happy-path замки — отвергнуто (не ловит регресс протяжки
  порога / шум на не-fire).

## R-8. §C-5.5 golden-churn co-land (ОБЯЗАТЕЛЕН)

- **Decision**: Always-on explain добавляет строку в вывод путей со срабатыванием → exact-match
  существующих golden'ов ломается. Обновить co-land РОВНО 8 тестов в 4 файлах; «НЕ затронутые»
  не трогать (см. contracts/golden-churn.md). Без co-land гейт `go test` недостижим.
- **Rationale**: §C-5.5 — обязательный инвентарь. Разделение must-update/NOT-affected исходит из
  канала (out/logf vs count/contains) и наличия срабатывания.
- **Alternatives considered**: флаг отключения explain в тестах — отвергнуто (нарушает always-on
  FR-001 и вводит условную логику; анкор требует always-on D-C-6).

### MUST UPDATE (8 в 4 файлах)

| # | Файл | Тест | Почему ломается |
|---|---|---|---|
| 1 | `daemon/tick_test.go` | `TestTickPhaseOrderAllThreeFire` | exact `out.String()` `"E\nM\nS\n"`; metric-fire добавит serve-explain строку |
| 2 | `daemon/tick_test.go` | `TestTickFourPhasesOrder` | exact `"E\nM\nS\nD\n"`; то же |
| 3 | `cmd/ladix/trigger_golden_test.go` | `TestRunTriggerFiresGolden` | exact stdout run-fire |
| 4 | `cmd/ladix/trigger_golden_test.go` | `TestRunTriggerDBRepeatEphemeral` | exact stdout run-fire |
| 5 | `cmd/ladix/trigger_golden_test.go` | `TestRunTriggerMultiMetricOrderGolden` | два fire → две explain-строки |
| 6 | `cmd/ladix/trigger_golden_test.go` | `TestRunTriggerMixedKindsOrderGolden` | exact stdout run-fire |
| 7 | `cmd/ladix/trigger_golden_test.go` | `TestRunTriggerBodyReadShadowGolden` | два fire → две explain-строки |
| 8 | `eval/metric_window_golden_test.go` | `TestWindowMetricTriggerFires` | exact `"оконная метрика: 23\n"` в out И `stubs.Len()==0` → explain ДОЛЖЕН идти в out, не в w |

### NOT-AFFECTED guard (НЕ ТРОГАТЬ — fix явно)

- count()/contains()-тесты демона: `metrics_test.go`, `schedule_test.go`, `daemon_test.go` MFIRE,
  `m2_endtoend` sink.count — проверяют счётчики/подстроки, не exact-match → explain не ломает.
- no-fire/error-тесты: `source_negatives` runtimeForceTrigger, `TestWindowMetricTriggerSilent`,
  events-FIFO `want=A\nB\nC` — нет срабатывания/другой канал → explain не печатается.
- `inspect` (`inspect.go:85-117`) — «где», не меняется.
- Guard-задача: после co-land прогнать эти тесты и подтвердить `git diff` НЕ трогает их файлы в
  затронутых местах (0 правок в NOT-affected тестах).

## R-9. Инварианты реестров (§C-6/§C-7)

- **Decision**: `ProcessRuntime` 8→8 (байт-цел; EvalMetricCondition — свободная функция, не
  интерфейс-метод). `Store` 18→18. Схема БД без изменений. 0 новых KW/SE/eval-кодов/builtins/
  зависимостей. Дифф в проде `store`/`engine` ПУСТОЙ.
- **Rationale**: §C-6/§C-7 дрейф-аудит M3-гейта. C5 — наблюдаемость, не функциональность.
- **Alternatives considered**: нет — это жёсткие инварианты вехи.

---

## Сводка решений (все NEEDS CLARIFICATION разрешены)

| Вопрос | Решение | Источник |
|---|---|---|
| Канал run | `i.out` (тело), не `w` | §C-5.1 / R-1 |
| Канал serve | `d.logf`, ветка `if fired` | §C-5.1/§C-5.2 / R-2 |
| Порог на serve | расширить `EvalMetricCondition +threshold` | §C-5.1 / R-3 |
| Нет данных | порог None, explain не печатается | FR-007/009 / R-4 |
| Формат | дословно §C-5.3, `value.String`/`BinOp.String` | §C-5.3 / R-5 |
| Одно/двухстрочный | одна строка (не §13) | §C-5.2 / R-6, Complexity Tracking |
| Тесты | новые замки + silence + inversion | §C-5.4 / R-7 |
| Golden-churn | 8 must-update + NOT-affected guard | §C-5.5 / R-8 |
| Реестры | PR=8, Store=18, 0 новых кодов | §C-6/§C-7 / R-9 |
