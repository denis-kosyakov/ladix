# Implementation Plan: C2b — Outbox-леджер и exactly-once доставка эффектов тела шага

**Branch**: `020-outbox-exactly-once` | **Date**: 2026-06-18 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/020-outbox-exactly-once/spec.md`

**Authority**: `docs/reliability-model.md` §C-2b / §C-1 (+ §C-0, §C-6, §C-7); on-disk digests `.m3-ledger/digest-anchor.md` (C2b) + `digest-seams.md` (file:line швы). Веха M3 «Надёжность», пункт C2b (после смерженной C2a).

## Summary

Закрыть надёжность золотого сценария §2: реальный внешний эффект В ТЕЛЕ ШАГА процесса (например `уведомить crm(...)`) доставляется **exactly-once** через рестарт демона. Подход — **durable outbox-леджер идемпотентности** (таблица `outbox` уже создана миграцией C2a): дедуп консультируется в момент диспатча эффекта в effect-методах движка (`CallExternal`/`CallExternalResult`/`Notify`) при активном кадре шага (`len(e.active) > 0`), по ключу `(instance_id, step_name, effect_index)`. Протокол — **deliver-then-record + pre-check** (D-C-9): pre-check `LoadOutbox`; если delivered — вернуть сохранённый результат без повторной доставки; иначе доставить, затем `SaveOutbox(delivered=1)`. Store растёт 16→18 аддитивно с двойным compile-замком; ProcessRuntime неизменно 8 (дедуп — поведение + новое поле кадра `effectIndex`, не контракт eval). Плюс: исполнимое усиление §2 (эволюция `examples/контроль_плана.ladix`), durable exactly-once гейт-тест (зеркало `driveServeToNoRepeat`), 3 реальных fault-теста `checkDeadlines`.

## Technical Context

**Language/Version**: Go 1.22+ (идиоматичный, `gofmt`, `go vet ./...` без замечаний; CGO запрещён).

**Primary Dependencies**: stdlib + `modernc.org/sqlite` (чистый Go, уже в графе). **0 новых зависимостей.**

**Storage**: SQLite через `internal/store` (durable, `--db`) + in-memory `MemoryStore`. Таблица `outbox` уже создана миграцией схемы 1→2 (C2a). Сериализация значений — существующий `internal/store/codec.go`.

**Testing**: `go test ./...`; durable-гейт зеркалит `driveServeToNoRepeat` (`cmd/ladix/m2_golden_test.go:234`) / `TestDeadlineDurableRestart`; fault-тесты `checkDeadlines` через инъекцию fault-Store; детерминизм через `FixedClock`/`fixedClock`.

**Target Platform**: один статический бинарник `ladix` (Linux/macOS/Windows, кросс-сборка одной командой).

**Project Type**: интерпретатор DSL (single project, Go-раскладка `cmd/` + `internal/`).

**Performance Goals**: N/A (надёжность, не пропускная способность). Дедуп — один `LoadOutbox` + при недоставленном один `SaveOutbox` на эффект тела шага; вне тела шага — нулевой оверхед.

**Constraints**: детерминизм (FixedClock); ПУСТОЙ дифф пакета eval (ProcessRuntime=8, eval не импортирует store/engine); продакшен-код `cmd` не меняется (кроме тестов serve при необходимости; serve.go clock-путь не ломать); 0 новых KW/SE-кодов/eval-кодов/builtins/зависимостей; дифф ограничен `internal/store` + `internal/engine` + `internal/daemon` (тесты) + `examples` + тесты.

**Scale/Scope**: Store 16→18 (+тип `OutboxRecord`, +sentinel `ErrOutboxNotFound`, +кодек round-trip); 3 effect-метода движка + поле кадра `effectIndex`; 3 fault-ветки `checkDeadlines`; 1 эволюция примера + MANIFEST; ~6 новых тест-замков + мутпробы.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| # | Принцип | Вердикт | Обоснование |
|---|---------|---------|-------------|
| I | Язык и сборка | ✅ PASS | Go 1.22+, idiomatic; 0 новых зависимостей (только stdlib + уже-в-графе `modernc.org/sqlite`); CGO не вводится; один бинарник. |
| II | Парсинг — ручной | ✅ PASS | Парсер/лексер не трогаются; 0 новых KW/SE-кодов; усиление §2 — существующие конструкции. |
| III | Ошибки — явные типы | ✅ PASS | Новый sentinel `ErrOutboxNotFound` в `store/errors.go` рядом с `ErrTriggerStateNotFound` (`errors.Is`-совместим). Штатные пути не паникуют; fault-тесты `checkDeadlines` подтверждают отсутствие паники (recover-границы целы). |
| IV | Позиции — сквозные | ✅ PASS | Не затрагивается: фича рантайм/store-уровня, без новых пользовательских диагностик с позициями. |
| V | Без глобального состояния | ✅ PASS | `effectIndex` — поле кадра `activeFrame` (инстанс-локальное, не пакетное); Store инжектируется как интерфейс; нет mutable package-state. |
| VI | Тесты — вперёд | ✅ PASS | Замки/мутпробы спроектированы до кода (durable-гейт, 3 fault-теста, codec round-trip, skip-delivered, result-replay; инверсии краснят). Лексер/парсер не трогаются — tests-first к ним не применим, но дух соблюдён. |
| VII | Раскладка проекта | ✅ PASS | Дифф в `internal/store` + `internal/engine` + `internal/daemon` (тесты) + `examples`. Граф ацикличен: `store` и `engine` уже зависят от `value`; eval НЕ импортирует store/engine (дедуп в engine, не в eval — ацикличность цела). |
| VIII | Язык сообщений | ✅ PASS | Лог-строки `checkDeadlines: …` — на русском, дословно из §C-2b.8. Новых пользовательских диагностик нет. |
| IX | Спека — источник истины | ✅ PASS | Всё выведено из `docs/reliability-model.md` §C-2b/§C-1 (D-C-7/8/9) + charter §2 (не редактируется). Расхождений нет; новые тексты — в `reliability-model.md` (якорь M3). |

**Итог Constitution Check: 9/9 PASS** (post-design re-check — без изменений, см. ниже).

## Project Structure

### Documentation (this feature)

```text
specs/020-outbox-exactly-once/
├── plan.md              # Этот файл
├── research.md          # Phase 0
├── data-model.md        # Phase 1
├── quickstart.md        # Phase 1
├── contracts/           # Phase 1
│   ├── store-outbox-methods.md
│   ├── outbox-codec.md
│   ├── dispatch-protocol.md
│   └── checkdeadlines-faults.md
├── checklists/
│   └── requirements.md  # из speckit-specify
└── tasks.md             # Phase 2 (speckit-tasks)
```

### Source Code (repository root)

```text
src/
├── internal/
│   ├── store/
│   │   ├── types.go         # +type OutboxRecord (рядом с Event/TriggerState)
│   │   ├── errors.go        # +ErrOutboxNotFound (рядом с ErrTriggerStateNotFound)
│   │   ├── store.go         # interface 16→18; ДВОЙНОЙ compile-замок :44-45 → оба новых метода
│   │   ├── memory.go        # MemoryStore.LoadOutbox/SaveOutbox (map + глубокая копия)
│   │   ├── sqlite.go        # SQLiteStore.LoadOutbox/SaveOutbox (SELECT / INSERT ON CONFLICT)
│   │   ├── codec.go         # переиспользуем encodeList/encodeValue/decodeList/decodeValue (без правок API)
│   │   └── *_test.go        # codec round-trip, Load/Save, ErrOutboxNotFound
│   ├── engine/
│   │   ├── engine.go        # activeFrame +effectIndex; reset в advance перед телом
│   │   ├── runtime.go       # CallExternal/CallExternalResult/Notify: pre-check + deliver-then-record при len(e.active)>0
│   │   └── *_test.go        # TestOutboxLedgerSkipsDelivered, TestOutboxResultReplay
│   └── daemon/
│       └── checkdeadlines_fault_test.go  # НОВЫЙ: 3 fault-ветки (нет *fault* файла сегодня)
├── cmd/ladix/
│   └── *_test.go            # TestStepEffectExactlyOnceRestart (inline-const, зеркало driveServeToNoRepeat)
└── examples/
    ├── контроль_плана.ladix # ЭВОЛЮЦИЯ: +2 авто-шага (зафиксировать_итог, уведомить_crm)
    └── MANIFEST.md          # обновить запись :151
```

**Structure Decision**: Single-project Go-раскладка. Дедуп живёт в `internal/engine` (есть контекст кадра), не в `internal/eval` (ProcessRuntime неизменен) и не в декораторе `ExternalCaller` (нет контекста инстанс/шаг/индекс). Сериализация — внутри `SQLiteStore` (eval не импортирует store). Гейт-тест — `cmd/ladix` (inline-const, изолирован от файловых golden). Эволюция примера — `examples/`.

## Complexity Tracking

> Constitution Check 9/9 PASS — формальных нарушений нет. Ниже фиксируются осознанные сложности, санкционированные авторитетом (Принцип IX), как того требует кикофф.

| Сложность | Зачем нужна | Отвергнутая простая альтернатива |
|-----------|-------------|----------------------------------|
| **Леджер идемпотентности (не очередь): только Load/Save, без `ListUnprocessed`/воркера-дренажа** (D-C-8, §C-2b.1) | Переисполнение недоставленного эффекта уже драйвится существующим рестарт-сканом (`advance` переисполняет тело шага); леджер лишь решает «доставлять/пропустить». Добавлять воркер-дренаж = дублировать уже-доказанную механику 007b и плодить фоновый цикл. Форма таблицы совпадает с events-FIFO, но механизм иной (consult-at-dispatch, не drain) → методов меньше. | Полноценная транзакционная очередь с воркером-дренажём отвергнута: нет нужды в фоновом цикле, рестарт-скан уже liveness-драйвер; меньше методов (2 vs 4) = меньше поверхность Store, проще двойной замок. |
| **Протокол deliver-then-record + pre-check с осознанным at-least-once зазором** (D-C-9, §C-2b.5) | Альтернатива «record-then-deliver» создала бы риск «помечено доставленным, но POST не ушёл» = потеря эффекта (хуже, чем дубль). deliver-then-record гарантирует POST ≤ 1 и при крахе после POST — at-least-once (повтор), что закрываемо идемпотентностью приёмника. Зазор POST→SaveOutbox — то же окно, что fire-then-persist `Escalated` (§C-2b.8 ветка-3). | Двухфазный коммит / распределённая транзакция с внешней системой отвергнуты: мы не владеем приёмником (`crm`), 2PC недостижим. Идемпотентность приёмника — единственный полный закрыватель окна (§C-9 бэклог); гейт §2 крашится ПОСЛЕ SaveOutbox → ровно 1, окно гейт не трогает. |
| **`result_json` хранит результат `вызвать` (включая None как tagged-blob, не SQL NULL)** (§C-2b.6) | `CallExternalResult` (выражение-`вызвать`) захватывает результат в переменную; на пропуске-по-дедупу метод обязан вернуть ТОТ ЖЕ результат, иначе логика процесса разойдётся. None как детерминированный tagged-`Пусто` blob (через `encodeValue`) даёт единый путь декода для statement- и expression-форм. | Хранить только флаг delivered без результата отвергнуто: пропуск-по-дедупу для `вызвать` вернул бы неверное значение. SQL NULL для None отвергнут: ломает единообразие кодека (`decodeValue` ожидает tagged-blob), вводит ветвление NULL-обработки. |

---

*Post-Design Constitution Re-Check (после Phase 1)*: артефакты Phase 1 (data-model/contracts/quickstart) не вводят новых нарушений — Store аддитивен (16→18, двойной замок), ProcessRuntime=8 цел, eval-дифф пуст, 0 новых зависимостей/KW/SE/eval-кодов, детерминизм через FixedClock. **Constitution Check остаётся 9/9 PASS.**
