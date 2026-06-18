# Implementation Plan: C5 — человеко-explain срабатывания (наблюдаемость «почему»)

**Branch**: `022-human-explain-fire` | **Date**: 2026-06-18 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/022-human-explain-fire/spec.md`

**Anchor**: `docs/reliability-model.md` §C-5 (§C-5.1..§C-5.5) + on-disk digests
`.m3-ledger/digest-anchor.md` (C5) и `.m3-ledger/digest-seams.md`. Веха M3 «Надёжность», пункт
C5 (размер M, наблюдаемость). Предшественники C2a/C2b/C4 уже смержены в master.

## Summary

Когда метрик-триггер срабатывает, ВСЕГДА печатать одну человеко-читаемую строку-explain «почему»
(имя метрики, снимок, оператор, порог, итог), в дополнение к существующему `inspect` («где»).
Технический подход минимален (size M): снимок и порог УЖЕ вычисляются в точке fire — фича только
ПЕЧАТАЕТ их. Три точечных шва:

1. **run** (`eval/trigger_run.go:78-92`, fire-if-true): протянуть `i.out` в `runMetricTrigger` и
   при истинности условия напечатать строку §C-5.3 (формулировка БЕЗ ребра) ДО тела.
2. **serve** (`daemon/metrics.go`): в ветке `if fired {` (`:82`, до `safeFire`/`fireBody`)
   напечатать через `d.logf` строку §C-5.3 (формулировка С ребром `ложь→истина`).
3. **EvalMetricCondition** (`eval/trigger_daemon.go:31`, СВОБОДНАЯ функция на `*Interpreter`, НЕ
   метод `ProcessRuntime`): расширить возврат `+threshold value.Value`, чтобы serve-печать имела
   порог (он сейчас выбрасывается внутри функции). ЕДИНСТВЕННЫЙ call-site `daemon/metrics.go:39`.

Always-on explain ломает exact-match существующих golden'ов → **ОБЯЗАТЕЛЬНЫЙ co-land** обновления
8 golden-тестов в 4 файлах (§C-5.5) + guard, что «НЕ затронутые» count/no-fire тесты НЕ тронуты.

## Technical Context

**Language/Version**: Go 1.22+ (конституция I). `gofmt` + `go vet ./...` без замечаний.

**Primary Dependencies**: stdlib + `modernc.org/sqlite` (уже в графе). **0 новых зависимостей**
(C5 их не вводит — конституция I).

**Storage**: SQLite/Memory `Store` — НЕ затрагивается. Контракт `Store` = 18 методов (цел).
Дифф в проде `store`/`engine` ПУСТОЙ.

**Testing**: `go test ./...`; table-driven + golden exact-match. Новые замки
`TestRunTriggerExplain` / `TestServeTriggerExplain`, замок тишины при не-fire, инверсия порога.

**Target Platform**: один статический бинарник `ladix` (Linux/macOS/Windows), без CGO.

**Project Type**: интерпретатор DSL (компилятор/CLI), single project, стандартная Go-раскладка.

**Performance Goals**: N/A (печать одной строки на срабатывание; без горячих циклов; детерминизм
важнее throughput).

**Constraints**: детерминизм вывода (FR-013) — тот же снимок/порог → та же строка, независимо от
часов/порядка; пригодность для exact-match golden. Без новых SE/eval-кодов, KW, builtins (FR-010).
`ProcessRuntime` = 8 методов байт-цел; `Store` = 18. `inspect` не меняется.

**Scale/Scope**: 3 точечных шва (1 расширение сигнатуры свободной функции + 1 call-site + 2 точки
печати) + co-land 8 golden-тестов + 2-3 новых замка. Дифф ограничен пакетами `eval` и `daemon`
(+ `cmd/ladix` golden-тесты).

### Ключевые швы (из digest-seams, подтверждены вживую)

| Шов | Файл:строка | Действие |
|---|---|---|
| run fire-if-true | `eval/trigger_run.go:78-92` | печать в `i.out`; протянуть `i.out` в `runMetricTrigger` |
| run writer-различие | `eval/metric_window_golden_test.go:102-104` | тело→`out`, RunTriggers-сводка→`w`: explain ИДЁТ в `out` |
| EvalMetricCondition | `eval/trigger_daemon.go:31` | `+threshold value.Value` в возврат (СВОБОДНАЯ функция, не интерфейс) |
| serve snapshot | `daemon/metrics.go:39` | единственный call-site → обновить деструктуризацию (+threshold) |
| serve edge | `daemon/metrics.go:72` | `fired := ts.LastBool!=nil && !*ts.LastBool && cur` |
| serve print point | `daemon/metrics.go:82` (ветка `if fired {`) | печать через `d.logf` (`daemon.go:53`) ДО `fireBody` |
| формат значения | `value/repr.go:20` (`value.String`) | снимок/порог БЕЗ подчёркиваний |
| формат оператора | `ast/op.go:35` (`BinOp.String`) | `<оп>` |
| inspect (НЕ трогать) | `inspect.go:85-117` | «где» — без изменений |

## Constitution Check

*GATE: пройти до Phase 0. Перепроверено после Phase 1 design.*

| # | Принцип | Вердикт | Обоснование |
|---|---|---|---|
| I | Язык и сборка | ✅ PASS | Go 1.22+, gofmt/vet; CGO нет; **0 новых зависимостей**; один бинарник. |
| II | Парсинг — ручной | ✅ PASS | C5 не трогает лексер/парсер; нет генераторов/regex. |
| III | Ошибки — явные типы | ✅ PASS | explain — НЕ ошибка (диагностика наблюдаемости); 0 новых SE/eval-кодов; recover-барьеры подкоманд целы; в штатном пути не паникуем. |
| IV | Позиции — сквозные | ✅ PASS | Не вводит новых пользовательских ошибок → позиций explain не требует; existing Position-инвариант не затрагивается. |
| V | Без глобального состояния | ✅ PASS | `i.out`/`d.logf` — инъектированные писатели (поля Interpreter/Daemon), не пакет-глобалы; threshold протягивается возвратом, не глобалом. |
| VI | Тесты — вперёд | ✅ PASS | Новые замки `TestRunTriggerExplain`/`TestServeTriggerExplain` + silence + inversion пишутся вместе с правкой; golden co-land §C-5.5 — часть задач. |
| VII | Раскладка проекта | ✅ PASS | Дифф в `internal/eval`, `internal/daemon`, `cmd/ladix` (тесты); граф без циклов; `value`/`ast` остаются листовыми (только читаем `value.String`/`BinOp.String`). |
| VIII | Язык сообщений | ✅ PASS | Строка-explain на русском; формат §C-5.3 берётся **дословно** из `docs/reliability-model.md`; двухстрочный канон §13 — для ОШИБОК, explain — не ошибка (одна строка), позиция/контекст по §C-5.2 (см. research R-6). |
| IX | Спека — источник истины | ✅ PASS | Источник — `docs/reliability-model.md` §C-5 (анкор пишет/санкционирует тексты); spec/plan привязаны к нему; пробелов-догадок нет. |

**Итог Constitution Check: 9/9 PASS.** Нарушений нет.

## Project Structure

### Documentation (this feature)

```text
specs/022-human-explain-fire/
├── plan.md              # Этот файл (/speckit-plan)
├── research.md          # Phase 0 (/speckit-plan)
├── data-model.md        # Phase 1 (/speckit-plan)
├── quickstart.md        # Phase 1 (/speckit-plan)
├── contracts/           # Phase 1 (/speckit-plan)
│   ├── eval-metric-condition.md   # расширение сигнатуры свободной функции
│   ├── explain-strings.md         # точный golden-формат §C-5.3 (run + serve)
│   └── golden-churn.md            # §C-5.5 must-update 8 + NOT-affected guard
├── checklists/
│   └── requirements.md  # из /speckit-specify
└── tasks.md             # Phase 2 (/speckit-tasks — НЕ создаётся этим планом)
```

### Source Code (repository root)

```text
src/
├── cmd/ladix/
│   ├── trigger_golden_test.go      # MUST UPDATE ×5 (run goldens, §C-5.5)
│   ├── trigger_run.go / trigger.go # run-путь explain (если печать в обёртке)
│   └── inspect.go                  # НЕ ТРОГАТЬ (B6 «где»)
└── internal/
    ├── eval/
    │   ├── trigger_run.go          # SE: run fire-if-true → печать в i.out (:78-92)
    │   ├── trigger_daemon.go       # SE: EvalMetricCondition +threshold (:31)
    │   ├── metric_window_golden_test.go  # MUST UPDATE ×1 (TestWindowMetricTriggerFires)
    │   └── (new) explain_test.go   # TestRunTriggerExplain + silence/inversion (новый)
    └── daemon/
        ├── metrics.go              # SE: serve call-site (:39) + edge (:72) + print (:82)
        ├── daemon.go               # logf (:53) — писатель explain
        ├── tick_test.go            # MUST UPDATE ×2 (PhaseOrderAllThreeFire, FourPhasesOrder)
        └── (new) explain_test.go   # TestServeTriggerExplain (новый, если не в metrics_test)
```

**Structure Decision**: single project, стандартная Go-раскладка (конституция VII). Реальный корень
`src/internal/...` и `src/cmd/ladix/...`. Изменения ограничены тремя продакшн-файлами (`eval/
trigger_run.go`, `eval/trigger_daemon.go`, `daemon/metrics.go` — плюс возможная протяжка writer в
`cmd/ladix` run-обёртке), co-land 8 golden-тестов в 4 файлах, и 2-3 новых тест-замка. Прод-дифф в
`store`/`engine` ПУСТОЙ.

## Complexity Tracking

> Constitution Check: 9/9 PASS — нарушений принципов НЕТ. Раздел заполнен НЕ из-за нарушений, а для
> явной фиксации единственного отступления от дефолта канона сообщений (санкционировано анкором).

| Отступление | Зачем нужно | Почему более простой путь отвергнут |
|---|---|---|
| Explain — ОДНОстрочный, не двухстрочный канон SPEC §13 (принцип VIII) | §13 двухстрочный формат («Ошибка в строке N, колонка M:») предназначен для ОШИБОК с позицией. Explain — диагностика наблюдаемости при срабатывании, НЕ ошибка: нет позиции исходника, нет SE/eval-кода. Анкор §C-5.2/§C-5.3 задаёт точную ОДНОстрочную форму дословно. | Навязать двухстрочный «Ошибка…» формат было бы СЕМАНТИЧЕСКИ НЕВЕРНО (это не ошибка) и противоречило бы дословному канону §C-5.3 (принцип VIII требует брать тексты дословно из docs). Анкор — санкционирующий источник истины (принцип IX). Записано здесь по требованию Governance. |
| `EvalMetricCondition` получает +1 возвращаемое значение (`threshold value.Value`) | serve-печать (`daemon/metrics.go:82`) нуждается в пороге; сейчас он вычисляется и ВЫБРАСЫВАЕТСЯ внутри функции. | Альтернатива — пере-вычислять порог в daemon — дублировала бы eval-логику в `daemon` (нарушила бы раскладку/листовость и детерминизм одной точки истины). Расширение возврата — eval-внутреннее, 1 call-site, НЕ метод интерфейса → `ProcessRuntime` = 8 цел, швов не пересекает. |

**Примечание реестров (§C-6/§C-7):** `ProcessRuntime` 8→8 (EvalMetricCondition — свободная функция,
не интерфейс-метод); `Store` 18→18; схема БД без изменений; новых KW/SE/eval-кодов/builtins нет;
дифф в проде `store`/`engine` пустой.
