# Implementation Plan: Финализация v2 — золотой сквозной пример §2

**Branch**: `023-v2-finalization` | **Date**: 2026-06-18 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/023-v2-finalization/spec.md`

**Якорь истины**: `docs/v2-finalization-model.md` — **§F-1** (работа, импл-слайс пример + co-land
тест-замки) и **§F-6** (приёмка против §2-DoD), плюс блок «Предрешённые развилки». При любом
расхождении побеждает якорь. Эта фича реализует **только** `[импл]`-помеченные части §F-1
(расширенный пример + T-GOLD-METRIC + T-LIFECYCLE + регресс-верификация); `[архитектор]`-синки
(§F-2…§F-6: quickstart/charter/README/SPEC/MANIFEST/CHECKLIST) — **вне границы**.

## Summary

Финализация v2 **без новой языковой функциональности**: золотой сквозной сценарий §2 хартии (CSV →
окно-метрика → «когда метрика < план» → процесс с человеком+срок → эскалация → payload → реальный
эффект → exactly-once) сводится в **один запускаемый** `examples/контроль_плана.ladix` (расширение
существующего файла, D-1=a), и закрепляется детерминированными co-land тест-замками. Продакшн-логика
движка/хранилища/интерпретатора **не меняется** — правки только в примере и Go-тестах (CLI-слой
`src/cmd/ladix/`).

Технический подход (3 рабочих фазы + 2 верификационных, нумерация W1–W5 из §F-1):
- **W1** — расширить пример: впаять аналитическую половину дословно из `выручка_30д.ladix` (источник
  `заказы` поверх `data/orders.csv` + окно-метрика `выручка_30д последние 30дн`), добавить триггер-связку
  `когда метрика выручка_30д < 3_000_000: запустить процесс эскалация_плана(значение)`, **удалить**
  хардкод-литеральный старт (`пусть id = запустить процесс …(2500000)` + `печать`).
- **W2** — новый детерминированный `T-GOLD-METRIC` под `FixedClock{2026,6,15}`, пинит 3 фасета **двумя
  путями**: (i) скаляр `300000.0` через `runMetric`; (ii) строка explain + (iii) метрика-driven старт
  `p-000001` через RUN-путь (`runFile`). **Заменяет** дата-наивный `TestCLIGoldenDeadlineEscalation`.
- **W3** — новый герметичный `T-LIFECYCLE` в `quickstart_smoke_test.go` (build-from-README бинарь,
  temp `--db`): `start` → задача `t-000001` → `complete --данные '{"итог":"перезвонит"}'` → авто-шаги
  → строка эффекта `[уведомление] crm: итог звонка: перезвонит` → `инстанс p-000001: выполнен`.
- **W4** — верификация: терминальные гейты `TestM2GoldenEndToEnd`/`TestStepEffectExactlyOnceRestart`
  зелёные и **не переписаны**; golden'ы старта/инспекции/часов зелёные без правок; co-land замок
  `TestCompleteClockInjected` (прогоняет догон-авто-шаги) перепроверен на имена шагов/ключ payload.
- **W5** — финальные гейты: `go build/vet ./...`, `gofmt -l .`, `go test ./...` из `src/` — exit 0;
  рабочее дерево чистое (бинарь/БД не закоммичены).

## Technical Context

**Language/Version**: Go 1.22+ (конституция I); go-модуль в `src/`.

**Primary Dependencies**: только `modernc.org/sqlite` (чистый Go, без CGO). **0 новых зависимостей**
этой фичей (FR-015).

**Storage**: SQLite через `internal/store` (Memory + SQLite impl). Несущий шов `Store` = **18**
методов — цел (замок `internal/store/escalated_codec_test.go:134` `wantMethods = 18`). Эта фича Store
не трогает.

**Testing**: `go test` (стандартный); golden/exact-match замки в `src/cmd/ladix/`. Детерминизм —
через инъекцию `FixedClock` (см. research.md): run-путь `fixedClock{ time.Date(2026,6,15,12,0,0,0,
time.Local) }` (тип `serve_golden_test.go:21`); metric-путь `eval.FixedClock{D: value.Дата{Year:2026,
Month:6, Day:15}}` (`metric_window_golden_test.go:16`). Прогон из repo-root через `withRepoRoot(t, …)`
для резолва `data/orders.csv`; дедлайны маскируются `maskDeadlines(s)` → `срок до <DT>`.

**Target Platform**: один статический бинарник `ladix` (конституция I), кросс-платформенный CLI.

**Project Type**: компилятор/интерпретатор DSL — single project, CLI-точка входа `cmd/ladix/`.

**Performance Goals**: N/A — финализация, без изменения горячих путей; цель — детерминированные
воспроизводимые golden'ы.

**Constraints**:
- **Продакшн-логика НЕ меняется** — дифф строго в `examples/контроль_плана.ladix` (правка) +
  Go-тестах `src/cmd/ladix/` (новые/заменяемые замки). ПУСТОЙ прод-дифф в `internal/eval`,
  `internal/engine`, `internal/store`, `internal/daemon`, `cmd/ladix/*.go` (не-тестовые).
- **Несущие швы целы**: `ProcessRuntime` = **8** методов (замок `internal/engine/payload_invariants_test.go:14`
  «РОВНО 8»); `Store` = **18** (замок `escalated_codec_test.go:134`).
- **0 новых** ключевых слов / кодов ошибок (SE/L/eval) / встроенных функций / зависимостей (FR-015).
- **Детерминизм обязателен** (FR-014): все новые golden на FixedClock/фикс-данных; дата-зависимое —
  по exit/маске `<DT>`.
- **Граница доков (OUT, FR-016)**: НЕ трогать `SPEC.md`, `README.md`, `CHECKLIST.md`,
  `examples/MANIFEST.md`, `docs/quickstart.md`, `docs/v2-charter.md`, любые `docs/*-model.md`.

**Scale/Scope**: 1 правленый пример (`examples/контроль_плана.ladix`); ~2 новых/замещённых Go-теста
(`T-GOLD-METRIC` замещает `TestCLIGoldenDeadlineEscalation`; `T-LIFECYCLE` дополняет
`quickstart_smoke_test.go`); верификация ~5 существующих гейтов без правок.

## Constitution Check

*GATE: проверено до Phase 0 и переоценено после Phase 1.*

| # | Принцип | Вердикт | Обоснование |
|---|---------|---------|-------------|
| I | Язык и сборка | ✅ PASS | Go 1.22+, 0 новых зависимостей; `gofmt`/`go vet` в W5. Прод-код не трогаем. |
| II | Парсинг — ручной | ✅ PASS | Парсер не трогаем; расширенный пример парсится существующим recursive-descent. |
| III | Ошибки — явные типы | ✅ PASS | Нет новых кодов/типов ошибок; пути обработки не меняются. |
| IV | Позиции — сквозные | ✅ PASS | Не затрагивается (нет правок лексера/парсера/диагностик). |
| V | Без глобального состояния | ✅ PASS | Тесты инжектируют `Clock`/`--db` явно; глобалов не вводим. |
| VI | Тесты — вперёд | ✅ PASS | Фича сама по себе — тест-замки; T-GOLD-METRIC/T-LIFECYCLE доказуемо кусаются при инверсии (FR-010). Лексер/парсер не трогаем. |
| VII | Раскладка проекта | ✅ PASS | Файлы строго в `examples/` + `src/cmd/ladix/`; граф зависимостей не меняется. |
| VIII | Язык сообщений | ✅ PASS | Тексты (explain/эффект) снимаются **дословно** с живого бинаря; не переформулируем (см. contracts/). |
| IX | Спека — источник истины | ✅ PASS | Якорь `docs/v2-finalization-model.md §F-1/§F-6` + развилки; 0 кларификаций (Q1=B/Q2=A/Q3=A/D-1=a предрешены). |

**Итог: 9/9 PASS.** Отклонений нет → раздел Complexity Tracking не заполняется.

Примечание о наблюдаемости (не отклонение): explain-строка метрика-триггера под `run` —
**одностройчная** форма `§C-5.3` (run-вариант без ребра), а НЕ двухстрочный канон диагностик SPEC §13
(принцип VIII касается **ошибок** — двухстрочный «Ошибка в строке N»). explain — не ошибка, а строка
наблюдаемости; это уже зафиксированный факт фичи 022 (Complexity Tracking 022), здесь лишь
переиспользуется как контракт. Новых текстов фича не вводит.

## Project Structure

### Documentation (this feature)

```text
specs/023-v2-finalization/
├── plan.md              # Этот файл (/speckit-plan)
├── research.md          # Phase 0: дата-зависимость окна, ловушка голого run, выбор FixedClock-путей
├── data-model.md        # Phase 1: сущности §2-цепочки (источник→метрика→триггер→процесс→задача→payload→эффект→инстанс)
├── quickstart.md        # Phase 1: артефакт SpecKit (НЕ корневой docs/quickstart.md) — как прогнать замки
├── contracts/           # Phase 1: контракты explain/эффекта/скаляра/golden как exact-match
│   ├── example-source.md     # контракт расширенного контроль_плана.ladix (копи-реди §F-1)
│   ├── t-gold-metric.md       # контракт T-GOLD-METRIC (3 фасета, 2 пути, замена TestCLIGoldenDeadlineEscalation)
│   └── t-lifecycle.md         # контракт T-LIFECYCLE (start→task→complete→эффект→выполнен)
└── tasks.md             # Phase 2 (/speckit-tasks — НЕ создаётся этой командой)
```

### Source Code (repository root)

Затрагиваемые файлы строго по слайсу §F-1 (всё внутри `src/`-модуля + `examples/`):

```text
examples/
└── контроль_плана.ladix          # W1: РАСШИРИТЬ до полной §2-цепочки; удалить литеральный старт+печать
                                   #     (целевое содержимое — копи-реди блок §F-1, строки 73–118 анкора)
data/
└── orders.csv                    # переиспользуется как общая фикстура (НЕ менять)

src/cmd/ladix/
├── main_test.go                  # W2: УДАЛИТЬ/заменить TestCLIGoldenDeadlineEscalation (:139) → T-GOLD-METRIC
├── metric_window_golden_test.go  # W2: референс runMetric+FixedClock; фасет (i) скаляр живёт рядом/здесь
├── quickstart_smoke_test.go      # W3: ДОБАВИТЬ T-LIFECYCLE (build-from-README, temp --db)
├── clock_unify_test.go           # W4: TestCompleteClockInjected (:167) перепроверить (зелёный, не переписывать)
├── start_golden_test.go          # W4: верифицировать зелёным без правок
├── inspect_golden_test.go        # W4: верифицировать зелёным без правок
└── serve_golden_test.go          # источник типа fixedClock{t} (:21) — НЕ менять

src/internal/daemon/
└── m2_endtoend_test.go           # W4: TestM2GoldenEndToEnd — терминальный гейт, НЕ переписывать
src/internal/engine/             # TestStepEffectExactlyOnceRestart — терминальный гейт, НЕ переписывать
src/internal/parser/
└── examples_test.go              # W1-крит: TestExamplesParseCleanSet (:11) — пример входит в чистый набор
```

**ПУСТОЙ прод-дифф** (compile-замки целости должны остаться зелёными без правок):
`internal/eval`, `internal/engine` (кроме тестов), `internal/store`, `internal/daemon` (кроме тестов),
`src/cmd/ladix/main.go` и прочий не-тестовый CLI-код.

**Structure Decision**: single project, Go-модуль в `src/`. Точка инъекции — CLI-слой `src/cmd/ladix/`
(тесты) + декларативный пример `examples/`. RUN-путь: `runFile(path, dbPath, maxDepth, caller, clock,
stdout, stderr)` (`main.go:227`) → `interp.Run` → `interp.RunTriggers` → `ExplainFire` (эмитит explain +
метрика-driven старт). METRIC-путь: `runMetric(path, metricName, maxDepth, clock, stdout, stderr)`
(`main.go:300`) — только скаляр, explain/старт НЕ эмитит. Отсюда — необходимость **двух** путей в
T-GOLD-METRIC (один `runMetric` 3 фасета не закроет).

## Phases (рабочий план реализации — для /speckit-tasks)

### W1 — Расширенный пример (US1, FR-001..005)
Заменить содержимое `examples/контроль_плана.ladix` на копи-реди блок §F-1 (источник `заказы` со
схемой `поля:` → метрика `выручка_30д` окно `последние 30дн` → триггер-связка → 3-шаговый процесс →
эскалация-триггер). **Удалить** `пусть id = запустить процесс эскалация_плана(2500000)` + `печать(…)`.
Гейт фазы: `TestExamplesParseCleanSet` зелён (файл парсится без диагностик).

### W2 — T-GOLD-METRIC + замена дата-наивного golden (US2, FR-006..008, FR-010, FR-014)
Снять с **живого бинаря** точные байты explain-строки и старт-строки под `FixedClock{2026,6,15}` из
repo-root. Написать `T-GOLD-METRIC`: (i) `runMetric` → скаляр `300000.0`; (ii)/(iii) `runFile` (RUN-путь)
с `fixedClock{2026-06-15}` → explain-строка (§C-5.3 run-форма) + старт `p-000001`. **Удалить**
`TestCLIGoldenDeadlineEscalation` (`main_test.go:139`). Мутпроба: срыв снимка/строки → красный (FR-010).

### W3 — T-LIFECYCLE (US3, FR-009..010, FR-014)
В `quickstart_smoke_test.go` добавить `T-LIFECYCLE`: из repo-root, temp `--db` →
`start … эскалация_плана 2500000` → задача `t-000001` → `complete … t-000001 --данные
'{"итог":"перезвонит"}'` → авто-шаги → строка эффекта `[уведомление] crm: итог звонка: перезвонит` →
`инстанс p-000001: выполнен`. Дедлайн маскируется (`maskDeadlines`), дат в выводе эффекта нет.
Мутпроба: смена ключа payload/текста эффекта → красный.

### W4 — Верификация регресс-инвариантов (US4, FR-011..012)
Прогнать без правок: `TestM2GoldenEndToEnd`, `TestStepEffectExactlyOnceRestart` (терминальные гейты —
зелёные, не переписаны); `start_golden_test.go`, `inspect_golden_test.go`, `clock_unify_test.go`
(зелёные без правок). Перепроверить `TestCompleteClockInjected` (он реально гоняет догон-авто-шаги с
payload `{"итог":"перезвонит"}`) на согласованность имён шагов и ключа payload — остаётся зелёным.

### W5 — Финальные гейты (FR-013)
Из `src/`: `go build ./...`, `go vet ./...`, `gofmt -l .` (пусто), `go test ./...` — exit 0. Рабочее
дерево чистое: сборочный бинарь (`src/ladix`) и файлы БД не закоммичены (`.gitignore`).

## Complexity Tracking

> Не заполняется — Constitution Check 9/9 PASS, отклонений нет.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
