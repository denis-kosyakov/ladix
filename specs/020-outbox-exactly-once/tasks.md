---
description: "Task list — C2b Outbox-леджер и exactly-once доставка эффектов тела шага"
---

# Tasks: C2b — Outbox-леджер и exactly-once доставка эффектов тела шага

**Input**: Design documents from `/specs/020-outbox-exactly-once/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/ (все присутствуют)
**Authority**: `docs/reliability-model.md` §C-2b/§C-1; швы `.m3-ledger/digest-seams.md` (file:line)

**Tests**: ВКЛЮЧЕНЫ — Принцип VI (tests-first) + явное требование кикоффа (durable-гейт, 3 fault-теста, codec round-trip, мутпробы). Все пути абсолютны от `src/`.

## Format: `[ID] [P?] [Story] Описание + путь`

- **[P]**: можно параллелить (разные файлы, нет зависимости от незавершённых задач).
- **[US1]** = P1 (exactly-once доставка); **[US2]** = P2 (устойчивость checkDeadlines).
- Setup/Foundational/Polish — без story-метки.

---

## Phase 1: Setup

**Purpose**: подтвердить базу (C2a смержена, таблица outbox есть, дерево чисто).

- [X] T001 Подтвердить базовую линию: `cd /Users/denis/dev/ladix/src && go build ./... && go vet ./...` зелёные; ветка `020-outbox-exactly-once`; `grep -rn 'OutboxRecord\|effectIndex\|ErrOutboxNotFound' src/` = 0 hits (ожидаемо — C2b вводит); таблица `outbox` уже создана миграцией C2a (`src/internal/store/sqlite.go` schemaMigrations 1→2).
- [X] T002 Зафиксировать инвариант-якоря до правок (мутпробная база): `grep -c` методов в `src/internal/store/store.go` interface = 16; ProcessRuntime в `src/internal/eval/runtime.go` = 8; двойной compile-замок `store.go:44-45` присутствует.

---

## Phase 2: Foundational — типы, sentinel, кодек Store (BLOCKING)

**Purpose**: данные леджера до методов и движка. Блокирует US1.

- [X] T003 [P] Добавить тип `OutboxRecord` в `src/internal/store/types.go` (рядом с `Event`/`TriggerState`) — поля DedupKey/InstanceID/StepName/EffectIndex/Kind/Target/Args []value.Value/Result value.Value/Delivered bool/CreatedAt time.Time/DeliveredAt *time.Time (data-model.md).
- [X] T004 [P] Добавить sentinel `ErrOutboxNotFound = errors.New("outbox-запись не найдена")` в sentinel-блок `src/internal/store/types.go:85-92` рядом с `ErrTriggerStateNotFound` (research R-DRIFT-1: отдельного `errors.go` нет; §C-2b.6 говорит `errors.go` — кладём туда, где реально живут sentinel'ы). `errors.Is`-совместим (Принцип III).
- [X] T005 Написать codec round-trip тесты (tests-first) в `src/internal/store/codec_test.go` (или `outbox_codec_test.go`): `TestOutboxCodecArgsRoundTrip` (Args=[число,строка,None] через encodeList(value.NewList)→decodeList), `TestOutboxCodecResultRoundTrip` (Result=число), `TestOutboxCodecResultNoneIsTaggedBlob` (Result=value.None → result_json НЕ NULL, непустой tagged-blob, decode→None), `TestOutboxCodecEmptyArgs` (пустой Args, не nil-паника). Контракт: `contracts/outbox-codec.md`. (Реализация кодека — переиспользование существующих encodeList/encodeValue/decodeList/decodeValue, новых хелперов API нет.)

---

## Phase 3 (US1, P1): Exactly-once доставка эффекта тела шага

**Goal**: реальный эффект В ТЕЛЕ ШАГА доставляется ровно один раз через рестарт демона (POST=1).
**Independent Test**: durable-рестарт mid-advance на той же `--db` → счётчик POST остаётся 1.

### Store-методы 16→18 + двойной compile-замок (tests-first)

- [X] T006 [US1] Написать контрактные тесты Store-методов (tests-first) в `src/internal/store/outbox_test.go`: `TestSaveLoadOutboxRoundTrip` (обе impl, все поля), `TestLoadOutboxNotFound` (обе impl, `errors.Is(err, ErrOutboxNotFound)`), `TestSaveOutboxUpsert` (обе impl, повторный Save → одна строка/последнее значение), `TestMemoryOutboxDeepCopy` (Memory: мутация Args[0]/*DeliveredAt после Save не протекает). Контракт: `contracts/store-outbox-methods.md`.
- [X] T007 [US1] Расширить interface `Store` в `src/internal/store/store.go:13-40`: добавить `LoadOutbox(dedupKey string) (*OutboxRecord, error)` и `SaveOutbox(rec *OutboxRecord) error` (после `ListTasksByInstance`). Базовые 16 сигнатур байт-целы. Двойной compile-замок `store.go:44-45` (`_ Store=(*MemoryStore)(nil)`; `_ Store=(*SQLiteStore)(nil)`) расширяется по интерфейсу автоматически.
- [X] T008 [US1] Реализовать `MemoryStore.LoadOutbox`/`SaveOutbox` в `src/internal/store/memory.go`: новое поле `outbox map[string]*OutboxRecord` (+инициализация в `NewMemoryStore`); глубокая копия Args (новый `[]value.Value`) и указателей времён при Save/Load (как `copyTask`); `LoadOutbox` отсутствующего ключа → `ErrOutboxNotFound`.
- [X] T009 [US1] Реализовать `SQLiteStore.LoadOutbox`/`SaveOutbox` в `src/internal/store/sqlite.go`: `SELECT … WHERE dedup_key=?` → `ErrOutboxNotFound` на `sql.ErrNoRows`; `INSERT … ON CONFLICT(dedup_key) DO UPDATE` (как `SaveTask` `sqlite.go:161`); сериализация ВНУТРИ store — Args→`encodeList(value.NewList(args))`, Result→`encodeValue` (None→tagged-Пусто blob, НЕ SQL NULL), delivered int 0/1, времена RFC3339 (DeliveredAt nil→NULL колонка); декод обратными decodeList/decodeValue. Таблица `outbox` уже создана C2a — НЕ создавать. Убедиться T006 зелёный на обеих impl.
- [X] T010 [US1] Двойной compile-замок: подтвердить `go build ./...` зелёный с обоими методами в обеих impl; добавить замок-комментарий у `store.go:44-45`. Мутпроба: временно удалить `SaveOutbox` из одной impl → `go build` падает → вернуть.

### Движок: effectIndex + дедуп в 3 effect-методах (tests-first)

- [X] T011 [US1] Написать Go-API тесты дедупа (tests-first) в `src/internal/engine/outbox_test.go`: `TestOutboxLedgerSkipsDelivered` (дважды `Engine.Notify` под одним кадром+ключом → `e.caller` вызван ОДИН раз), `TestOutboxResultReplay` (`CallExternalResult` под дедупом возвращает сохранённый Result без повторного Call), `TestDedupOnlyInsideStepBody` (`len(e.active)==0` → delegate напрямую, outbox не консультируется), `TestTwoEffectsIndependentKeys` (два эффекта в теле → idx 0 и 1, независимо). Контракт: `contracts/dispatch-protocol.md`. Использовать fake `ExternalCaller`-счётчик + `FixedClock`.
- [X] T012 [US1] Добавить поле `effectIndex int` в `activeFrame` (`src/internal/engine/engine.go:31-34`); сброс `frame.effectIndex = 0` в `advance` (`engine.go:249`, начало каждой итерации шага, перед `ExecStepBody`). Инстанс-локально (Принцип V).
- [X] T013 [US1] Реализовать дедуп-протокол в `Engine.CallExternalResult` (`src/internal/engine/runtime.go` ≈:47-49) при `len(e.active)>0`: pre-check `LoadOutbox(key)` → если `Delivered` вернуть `rec.Result` без Call; иначе Call→при ошибке вернуть derr (не помечать)→при успехе `SaveOutbox(Kind:"вызвать", Result:v, Delivered:true, CreatedAt/DeliveredAt:now)`; вернуть v. Ключ `fmt.Sprintf("%s|%s|%d", fr.inst.ID, fr.inst.CurrentStep, idx)` с `idx := fr.effectIndex; fr.effectIndex++`.
- [X] T014 [US1] Реализовать дедуп в `Engine.CallExternal` (`runtime.go` ≈:53-56) НЕЗАВИСИМО (зовёт `e.caller.Call` напрямую, не делегирует CallExternalResult): тот же протокол, Kind:"вызвать", Result хранится как None (отбрасывается), на пропуске вернуть nil.
- [X] T015 [US1] Реализовать дедуп в `Engine.Notify` (`runtime.go` ≈:60-62) НЕЗАВИСИМО: тот же протокол, Kind:"уведомить", Result:value.None, на пропуске вернуть nil. Убедиться T011 зелёный.

### Durable exactly-once гейт-тест (§C-2b.7)

- [X] T016 [US1] Написать гейт-тест `TestStepEffectExactlyOnceRestart` в `src/cmd/ladix/` (inline-const источник по образцу `m2CLISrc`/`m2_golden_test.go:234` `driveServeToNoRepeat`, изолирован от файловых golden): прогнать усиленный §2 до `уведомить_crm` (POST 1) → открыть НОВЫЙ Store на той же `--db` → `RunRestartScan` → реактивация → тики → счётчик POST остался 1. Зеркало `TestDeadlineDurableRestart`. `FixedClock`. Гейт §2 крашится ПОСЛЕ SaveOutbox → ровно 1.

### Мутпробы US1 (инверсии)

- [X] T017 [US1] Мутпроба pre-check: временно снять `if rec.Delivered → skip` → доставка дважды → `TestStepEffectExactlyOnceRestart` + `TestOutboxLedgerSkipsDelivered` КРАСНЫЕ → вернуть. Зафиксировать как замок exactly-once.
- [X] T018 [US1] Мутпроба effect_index: временно `effectIndex ≡ 0` (не инкрементировать) при ≥2 эффектах → коллизия ключей → `TestTwoEffectsIndependentKeys` КРАСНЫЙ → вернуть.

**Checkpoint US1**: durable exactly-once гейт зелёный; Store=18 двойной замок; дедуп в 3 методах; мутпробы кусают.

---

## Phase 4 (US2, P2): Устойчивость checkDeadlines к fault'ам (3 ветки)

**Goal**: 3 fault-ветки `checkDeadlines` устойчивы (нет паники, точные лог-строки, остальные задачи обработаны).
**Independent Test**: инъекция fault-Store в каждую из 3 веток → нет паники, лог-строка, продолжение.

- [X] T019 [US2] Создать `src/internal/daemon/checkdeadlines_fault_test.go` (нет `*fault*` файла сегодня) + ручной fault-Store (обёртка над `MemoryStore`/Store с инъекцией ошибки в один метод; 0 новых зависимостей; `FixedClock`).
- [X] T020 [P] [US2] `TestCheckDeadlinesListError` (ветка 1, `checkdeadlines.go:38-41`): `ListPendingTasks` → ошибка → фаза НЕ паникует, лог-строка `"checkDeadlines: листинг задач: %s"` присутствует, ранний return, демон жив (следующий тик идёт).
- [X] T021 [P] [US2] `TestCheckDeadlinesLoadInstanceError` (ветка 2, `:50-53`): `LoadInstance` падает для одной задачи → задача пропущена (continue), нет эскалации этой, нет паники, остальные обработаны.
- [X] T022 [P] [US2] `TestCheckDeadlinesSaveTaskError` (ветка 3, `:63-65`): `SaveTask`(Escalated) падает после fire → лог-строка `"checkDeadlines: персист Escalated задачи %s: %s"` присутствует; **комментарий теста фиксирует** известное окно fire-then-persist (пара к §C-2b.5 dispatch-зазору / §C-9 бэклог), НЕ дефект. Контракт: `contracts/checkdeadlines-faults.md`.

**Checkpoint US2**: 3 fault-ветки покрыты реальными тестами; нет паники; лог-строки дословны (Принцип VIII).

---

## Phase 5: Исполнимое усиление §2 (демо-пример + golden-churn)

**Purpose**: усиленный §2 исполним; затронутые golden переснятыми co-land. Обслуживает US1 (демо).

- [X] T023 [US1] Эволюционировать `examples/контроль_плана.ladix`: добавить два авто-шага в процесс `эскалация_плана` — `шаг зафиксировать_итог после связаться_с_клиентом:` (`присвоить итог = данные.итог`) и `шаг уведомить_crm после зафиксировать_итог:` (`уведомить crm("итог звонка: " + итог)`). ТОЛЬКО существующие конструкции; арность не меняется (один параметр). Текст в `docs/v2-charter.md` §2 НЕ редактировать.
- [X] T024 [US1] Обновить запись `контроль_плана.ladix` в `examples/MANIFEST.md` (≈:151) под эволюцию (новые авто-шаги/golden-stdout).
- [X] T025 [US1] Переснять golden `TestCLIGoldenDeadlineEscalation` (`src/cmd/ladix/main_test.go:137`): exact stdout + добавившаяся строка эффекта `crm`. Подтвердить `start_golden_test.go:46` `TestStartArityMismatch` НЕ затронут (параметр один). `go test ./cmd/ladix/...` зелёный.

---

## Phase 6: Polish & инвариант-проверки (M3-гейт, §C-7)

**Purpose**: дрейф-аудит и финальные замки.

- [X] T026 ProcessRuntime = ровно 8, сигнатуры байт-целы; `git diff --stat -- src/internal/eval` ПУСТО (eval не импортирует store/engine — `grep` zero matches).
- [X] T027 Store: двойной compile-замок; interface = 18; базовые 16 сигнатур целы; сериализация внутри SQLiteStore.
- [X] T028 Фронтенд не тронут: 0 новых KW/SE/eval-кодов/builtins/зависимостей; усиление §2 = существующие конструкции; v1-программы валидны. `cmd` прод-код не менялся (кроме тестов); serve clock-путь цел (`serve_golden_test.go:216` зелёный).
- [X] T029 Полный прогон: `cd /Users/denis/dev/ladix/src && go build ./... && go vet ./... && go test ./...` (опц. `-race`) — всё зелёное; детерминизм (FixedClock) подтверждён повторным прогоном гейта.
- [X] T030 Финальная мутпроба-свод: подтвердить, что снятие любой гарантии (pre-check / effect_index / deliver-then-record order) краснит соответствующий замок (T017/T018 + record-then-deliver красит тест провала доставки).

---

## Dependencies

- **Setup (T001-T002)** → всё.
- **Foundational (T003-T005)** → US1 (типы/sentinel/кодек до методов).
- **US1 (T006-T018, T023-T025)**: T006(тесты)→T007(interface)→T008/T009(impl, [P] разные файлы)→T010(замок); T011(тесты)→T012(effectIndex)→T013/T014/T015(3 метода, последовательно — один файл runtime.go); T016(гейт) после T009+T013-T015; T017/T018(мутпробы) после T016; T023-T025(пример+golden) после T013-T016.
- **US2 (T019-T022)**: НЕЗАВИСИМА от US1 (другой пакет/файл) — может идти параллельно после Setup; T020/T021/T022 [P] (один новый файл, но независимые тест-функции — писать последовательно если один файл, [P] помечает логическую независимость).
- **Polish (T026-T030)** → после US1+US2.

## Parallel opportunities

- T003 ∥ T004 (разные секции types.go — координировать; помечены [P] как логически независимые).
- T008 ∥ T009 (memory.go ∥ sqlite.go).
- US2 (T019-T022) ∥ US1 целиком (разные пакеты: daemon vs store/engine/cmd).

## Implementation strategy (MVP)

- **MVP = US1** (T001-T018 + T023-T025): exactly-once доставка — несущий критерий §2. Самодостаточен, durable-гейт зелёный.
- **US2** (T019-T022) — устойчивость checkDeadlines, инкремент надёжности, независим.
- Полировка (T026-T030) — дрейф-аудит M3-гейта.

## Coverage map (требования кикоффа → задачи)

| Требование | Задача(и) |
|---|---|
| (1) durable exactly-once гейт `TestStepEffectExactlyOnceRestart` (§C-2b.7) | **T016** (+мутпроба T017) |
| (2) 3 fault-теста checkDeadlines (§C-2b.8) | **T020, T021, T022** (+T019 fault-Store) |
| (3) двойной compile-замок LoadOutbox/SaveOutbox обе impl | **T007, T008, T009, T010** |
| (4) codec round-trip тесты | **T005** |
| (5) examples/контроль_плана.ladix эволюция + MANIFEST + golden main_test.go:137 | **T023, T024, T025** |
| OutboxRecord тип + ErrOutboxNotFound sentinel | **T003, T004** |
| activeFrame.effectIndex + reset в advance | **T012** |
| дедуп в 3 effect-методах независимо | **T013, T014, T015** |
| мутпробы (pre-check / effect_index) | **T017, T018, T030** |
| ProcessRuntime=8 / Store 16→18 / пустой дифф eval | **T026, T027, T028** |

---

## Analyze gate (speckit-analyze, read-only verdict)

- 23 FR / 8 SC / 30 tasks. Coverage 100% (все 23 FR имеют ≥1 задачу). 0 CRITICAL, 0 HIGH, 0 duplication, 0 ambiguity.
- Constitution 9/9 PASS.
- Mandatory test-locks все покрыты: durable gate §C-2b.7 (T016, мутпроба T017); 3 fault-теста checkDeadlines §C-2b.8 (T020/T021/T022); двойной compile-замок (T007-T010); codec round-trip (T005); пример+MANIFEST+golden (T023-T025).
- False-positives (зафиксированы, не блокеры): `store/errors.go` vs sentinel в `types.go:85-92` (анкор-шорткат §C-2b.6, учтён в research R-DRIFT-1 + T004); FR-012/FR-015 — структурные/документируемые границы, не пропуски покрытия.
- DRIFT анкор↔репо (репорт, НЕ чинится в design): (1) sentinel в types.go, не errors.go; (2) минорные line-offsets §C-10 (ddl :23-66, pragmas :69-73, NewSQLiteStore закрывается :99, interface ends :40 / var block :42-45); (3) OutboxRecord/effectIndex/ErrOutboxNotFound отсутствуют сегодня — ожидаемо (C2b вводит).
