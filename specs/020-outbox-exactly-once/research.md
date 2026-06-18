# Research: C2b — Outbox-леджер и exactly-once

**Метод**: вся неопределённость снята авторитетом `docs/reliability-model.md` §C-2b/§C-1 + швы из `.m3-ledger/digest-seams.md` (file:line, проверено против живого кода). NEEDS CLARIFICATION = 0. Ниже — зафиксированные решения.

## R1. Модель леджера: идемпотентность, не очередь (D-C-8)

- **Decision**: outbox — durable леджер доставленных эффектов, ключ `(instance_id, step_name, effect_index)`, консультируется при диспатче. Методы только `LoadOutbox`/`SaveOutbox` — без `ListUnprocessed`/воркера-дренажа.
- **Rationale**: переисполнение драйвит существующий рестарт-скан (`daemon/restart.go:28` → `ReactivateInstance` → `advance` переисполняет тело шага). Леджер решает «доставлять/пропустить». Форма таблицы = events-FIFO, но механизм иной (consult-at-dispatch).
- **Alternatives**: транзакционная очередь с воркером — отвергнута (дублирует механику 007b, плодит фоновый цикл).

## R2. Где живёт дедуп: effect-методы движка, не декоратор ExternalCaller (D-C-7)

- **Decision**: дедуп в `Engine.CallExternal`/`CallExternalResult`/`Notify` (`engine/runtime.go` ≈ :47-62), гейт `len(e.active) > 0`. **В каждом из 3 методов независимо.**
- **Rationale**: `ExternalCaller.Call/Notify` (`engine/caller.go:18-21`) принимают только `(target, args)` — нет контекста инстанс/шаг/индекс. Контекст у движка: стек `e.active` (кадр `activeFrame{inst, processEnv}`, `engine.go:31-34`). **Факт-сноска (проверено вживую):** `engine.CallExternal` зовёт `e.caller.Call` НАПРЯМУЮ (`runtime.go:53-58`), НЕ делегирует `CallExternalResult` — поэтому нельзя обернуть один метод и считать, что прочие наследуют.
- **Alternatives**: декоратор `ExternalCaller` — невозможен (нет контекста); делегирование одного метода — неверно (3 независимы).

## R3. ProcessRuntime неизменен (8 методов) — INV-1

- **Decision**: контракт `eval.ProcessRuntime` (`eval/runtime.go:9-45`, 8 методов: StartProcess, AssignProcessVar, CallExternal, CallExternalResult, Notify, InstanceStatus, InstanceVariables, UserTasks) НЕ меняется. Дедуп — поведение + новое поле кадра, не контракт.
- **Rationale**: eval не импортирует store/engine (ацикличность, Принцип VII). Дедуп живёт в engine над `e.st`. `*Engine` реализует интерфейс; effect-методы — на `*Engine`, не на ProcessRuntime-receiver.
- **Alternatives**: расширить интерфейс — отвергнуто (ломает INV-1, тянет store в eval).

## R4. effect_index — детерминированный счётчик (§C-2b.4)

- **Decision**: новое поле `activeFrame.effectIndex int`. Reset в **0 в начале каждой итерации шага в `advance`** (`engine.go:249`, per-step loop `for {` :262, push frame :255), перед `ExecStepBody`. Инкремент в каждом effect-методе при `len(e.active)>0`: `idx := fr.effectIndex; fr.effectIndex++`. Ключ `outboxKey(fr.inst.ID, fr.inst.CurrentStep, idx)` = `fmt.Sprintf("%s|%s|%d", …)`.
- **Rationale**: `ExecStepBody` (`eval/exports.go:38`) идёт по `body []ast.Statement` строго по порядку → на рестарте тот же эффект = тот же индекс. `step_name = fr.inst.CurrentStep` (advance ставит до тела). `effectIndex` сегодня НЕ существует (grep 0 hits) — новое M3-поле.
- **Alternatives**: глобальный счётчик — отвергнут (Принцип V, не детерминирован на рестарте); счётчик по позиции AST — избыточен (порядок итерации уже детерминирован).

## R5. Протокол dispatch: deliver-then-record + pre-check (D-C-9, §C-2b.5)

- **Decision**: (1) `key := (inst.ID, CurrentStep, effect_index++)`; (2) `LoadOutbox(key)` — если `err==nil && rec.Delivered` → НЕ звать caller, вернуть `rec.Result` (CallExternalResult) или nil (CallExternal/Notify), СТОП; (3) доставить `e.caller.Call|Notify`; (4) `derr!=nil` → вернуть derr (шаг провалится, D-14), НЕ помечать; (5) `derr==nil` → `SaveOutbox(delivered=1, result, CreatedAt:now, DeliveredAt:&now)` upsert; (6) вернуть v/nil.
- **Rationale**: deliver-then-record гарантирует POST ≤ 1 (record-then-deliver рисковал бы потерей эффекта). Зазор POST→SaveOutbox = осознанный at-least-once (§C-9 бэклог) — то же окно, что fire-then-persist `Escalated`. Гейт §2 крашится ПОСЛЕ SaveOutbox → ровно 1.
- **Alternatives**: record-then-deliver (риск потери) и 2PC (недостижим — не владеем приёмником) отвергнуты.

## R6. Кодек OutboxRecord через существующий store/codec.go (§C-2b.6)

- **Decision**: `Args []value.Value` → `encodeList(value.NewList(args))` (`codec.go:154`, нет хелпера для голого `[]value.Value` → заворачиваем в `Список`, `value/list.go:14`). `Result value.Value` → `encodeValue` (`codec.go:78`; None → tagged-`Пусто` blob, НЕ SQL NULL). Декод на пропуске — `decodeList`/`decodeValue` (`codec.go:309/224`). Сериализация ВНУТРИ SQLiteStore.
- **Rationale**: переиспользование доказанного value-кодека (INV-2); единый путь для statement- (`Result=value.None`) и expression-форм. `eval` не импортирует store.
- **Alternatives**: ad-hoc JSON / SQL NULL для None — отвергнуты (ломают единообразие декода, вводят NULL-ветвление).

## R7. Store 16→18 аддитивно, двойной compile-замок (INV-2)

- **Decision**: +`LoadOutbox(dedupKey string) (*OutboxRecord, error)` (не найдено → `ErrOutboxNotFound`), +`SaveOutbox(rec *OutboxRecord) error` (upsert). В обеих impl. Замок `internal/store/store.go:44-45` (`_ Store = (*MemoryStore)(nil)`; `_ Store = (*SQLiteStore)(nil)`) расширяется автоматически по интерфейсу → отсутствие метода в любой impl ломает сборку. MemoryStore: `map[string]*OutboxRecord` + глубокая копия `Args`/времён (как `copyTask`). SQLiteStore: `SELECT … WHERE dedup_key=?` → `ErrOutboxNotFound` на `sql.ErrNoRows`; `INSERT … ON CONFLICT(dedup_key) DO UPDATE` (как `SaveTask` `sqlite.go:161`).
- **Rationale**: базовые 16 сигнатур байт-целы; замок именно двойной (не тройной) — `MemoryStore`+`SQLiteStore`. Таблица `outbox` уже создана C2a (миграция 1→2). Глубокая копия Memory обязательна, иначе мутация значений в движке протекает в леджер.
- **Alternatives**: тройной замок / новый интерфейс — нет; миграция новой таблицы — не нужна (C2a уже создала).

## R8. Граница применимости: только тело шага (§C-2b.3)

- **Decision**: дедуп ⟺ `len(e.active) > 0`. Тело метрики-триггера / эскалации / расписание / top-level `запустить` (`len(e.active)==0`) → delegate напрямую.
- **Rationale**: идемпотентность не-шаговых путей обеспечивают существующие гарды (ребро `LastBool` 007, `Escalated` M2, `last_fired_date` 007). Не дублируем гарантии.
- **Alternatives**: дедуп везде — отвергнут (дублирует гарды, размывает модели).

## R9. Durable exactly-once гейт-тест (§C-2b.7)

- **Decision**: `TestStepEffectExactlyOnceRestart` — **inline-const** источник (образец `m2CLISrc`/`m2_golden_test.go:234` `driveServeToNoRepeat`), изолирован от файловых golden. Прогон §2 до `уведомить_crm` (POST 1) → новый Store на той же `--db` → `RunRestartScan` → реактивация → тики → POST остался 1. Зеркало `TestDeadlineDurableRestart`.
- **Rationale**: inline-const = низкий риск golden-churn. Демо-усиление §2 = эволюция `examples/контроль_плана.ladix` (отдельно).
- **Alternatives**: гейт на файловом примере — отвергнут (хрупкий golden-churn).

## R10. Стратегия примера и golden-churn (§C-2b.7)

- **Decision**: усиленный §2 как ДЕМО = эволюция `examples/контроль_плана.ladix` (+2 авто-шага). Обязательно переснять: `cmd/ladix/main_test.go:137` (`TestCLIGoldenDeadlineEscalation`, +строка эффекта `crm`) и `examples/MANIFEST.md:151`. Арность процесса не меняется (`start_golden_test.go:46` `TestStartArityMismatch` не затронут — параметр один).
- **Rationale**: текст §2 уже в charter (`docs/v2-charter.md` §2:86-89, не редактируем); переносим в исполнимый файл существующими конструкциями.
- **Alternatives**: новый файл-пример — допустим, но эволюция канонического — каноничнее (витрина).

## R11. 3 fault-теста checkDeadlines (§C-2b.8)

- **Decision**: новый файл `internal/daemon/checkdeadlines_fault_test.go` (нет `*fault*` сегодня), инъекция fault-Store. Ветка 1 (`:38-41`) `ListPendingTasks` error → лог `"checkDeadlines: листинг задач: %s"` + ранний return, демон жив. Ветка 2 (`:50-53`) `LoadInstance` error → `continue`. Ветка 3 (`:63-65`) `SaveTask`(Escalated) error → лог `"checkDeadlines: персист Escalated задачи %s: %s"`, fire-then-persist known window (комментарий теста).
- **Rationale**: `checkDeadlines` (`daemon/checkdeadlines.go:22`) корректен, но 3 ветки не покрыты; превращаем в реальные тесты надёжности (нет паники, точные лог-строки).
- **Alternatives**: моки-библиотеки — отвергнуты (ручной fault-Store, 0 новых зависимостей).

## R12. Детерминизм

- **Decision**: все новые тесты на `FixedClock`/`fixedClock` (engine.Clock fake, `serve_golden_test.go:21-23`). `CreatedAt`/`DeliveredAt` штампуются часами движка → детерминированы под FixedClock.
- **Rationale**: Принцип V + воспроизводимость golden.
- **Alternatives**: `time.Now()` — отвергнут (недетерминизм).

## Сводка switms (швы проверены, digest-seams.md)

| Шов | Файл:строка | Состояние |
|---|---|---|
| Store interface 16, замок | `store/store.go:13-40`, var block `:42-45` | расширяем 16→18, замок на оба новых |
| sentinel block | `store/types.go:85-92` (нет отдельного errors.go сегодня — **DRIFT см. ниже**) | +ErrOutboxNotFound |
| encodeValue/encodeList | `codec.go:78` / `:154` | переиспользуем |
| effect-методы | `engine/runtime.go:47-62` | дедуп в 3 методах |
| activeFrame | `engine.go:31-34`; стек `active` `:43` | +effectIndex |
| advance | `engine.go:249`, loop `:262`, push `:255` | reset effectIndex |
| checkDeadlines | `daemon/checkdeadlines.go:22` (:38-41/:50-53/:63-65) | 3 fault-теста |
| гейт-шаблон | `m2_golden_test.go:234` driveServeToNoRepeat | зеркало |

## DRIFT анкор↔репо (РЕПОРТ, не чиним здесь)

1. **`store/errors.go` не существует** — sentinel-блок живёт в `store/types.go:85-92` (рядом с `ErrTriggerStateNotFound :92`). §C-2b.6 / описание фичи говорят «`store/errors.go`». Решение для implement: добавить `ErrOutboxNotFound` туда, где реально живут sentinel'ы (`types.go:85-92`), ИЛИ создать `errors.go` — оба удовлетворяют Принципу III. Зафиксировано как анкор-шорткат, не блокер.
2. Минорные line-offsets §C-10 vs живой код (ddl :23-66 не :23-67; pragmas :69-73; NewSQLiteStore закрывается :99; interface ends :40, var block :42-45). Пути все верны. Не блокер.
3. `OutboxRecord`/`ErrOutboxNotFound`/`effectIndex` не существуют сегодня (grep 0 hits) — ожидаемо, C2b их вводит. Не дрейф.
