---
description: "Task list — B3 payload через complete --данные (015)"
---

# Tasks: B3 — payload задачи через `complete --данные`

**Input**: `/specs/015-complete-payload/` (spec.md, plan.md, research.md,
data-model.md, contracts/, quickstart.md). Якорь — `docs/automation-model.md` §AU-5 /
D-AU-3 / §AU-10.C.

**Tests**: ВКЛЮЧЕНЫ и tests-first (Принцип VI). Каждый несущий инвариант имеет
замок + мутпробу (инверсия): сформулирована «мутация → красный». Замки a-e —
усиленные (см. §«Замки-инверсии»).

**Organization**: фазы Setup → Foundational → US1 (P1) → US2 (P1) → US3 (P2) →
Polish. US независимо тестируемы. `[P]` — разные файлы, без зависимостей.

## Path Conventions

- Прод: `src/cmd/ladix/main.go`, `src/internal/engine/engine.go`. Потребляемое:
  `src/internal/jsonval/decode.go` (НЕ менять).
- Тесты: `src/internal/engine/*_test.go`, `src/cmd/ladix/*_test.go`, golden-фикстуры.
- Все пути — от корня репо `/Users/denis/dev/ladix`.

---

## Phase 1: Setup

- [x] **T001** Сверить базу на `aebac92`: `jsonval.PayloadToRecord` экспортирован
  (`src/internal/jsonval/decode.go:31`); 4 функции на местах
  (`Complete`:108/`catchUp`:177/`advanceAfterComplete`:189/`advance`:242);
  `stepEnv := eval.NewEnvironment(processEnv)`:262 / `ExecStepBody`:275;
  `ProcessRuntime` = 8 методов. Зафиксировать строки (могут сдвинуться — импл сверяет).
- [x] **T002** `go test ./...` + `go vet ./...` + race на чистом дереве — зелёные
  (база регресса). Зафиксировать существующий complete-golden как baseline.

---

## Phase 2: Foundational (блокирует все US)

**Purpose**: расширить внутренний API движка параметром `data` (по умолчанию пустая
`Запись`) — БЕЗ изменения поведения. Это «несущий каркас» под US1-US3.

- [x] **T003** [Регресс-замок, ДО правок] Тест `TestCompleteNoPayloadRegress`
  (`engine_test.go`): существующий путь `Complete(taskID, value.NewRecord(nil,nil))`
  даёт прежний вывод/статусы (строки 7-10 §EN-7). Сейчас RED (сигнатура без data) —
  фиксирует целевую сигнатуру. **Инверсия (e): любое изменение поведения без
  `--данные` → красный.**
- [x] **T004** Протянуть `data value.Запись` через 4 функции
  (`src/internal/engine/engine.go`), сигнатуры по data-model §«Протяжка»:
  `Complete(taskID, data)` → `catchUp(inst, data, t)` /
  `advanceAfterComplete(inst, data, caughtUp)` → `advance(inst, data)`. Все 3
  внутренних вызова прокидывают `data`. Точку `Define` пока НЕ добавлять (T007).
- [x] **T005** Обновить ВСЕ существующие вызовы `Complete`/`catchUp`/
  `advanceAfterComplete`/`advance` (прод + тесты + CLI `completeTask`), передавая
  `value.NewRecord(nil, nil)` на старых путях. T003 → зелёный (регресс цел).
- [x] **T006** Проба-инвариант: `ProcessRuntime` остался 8 методов
  (`grep`/тест числа); `internal/eval` НЕ импортирует `engine`/`store`/`jsonval`
  (импорт-проба / `go list`). **Инверсия: рост шва или импорт в eval → красный.**

**Checkpoint**: каркас протяжки готов, поведение неизменно, регресс зелёный.

---

## Phase 3: US1 — payload виден первому шагу догона (P1)

**Goal**: первый шаг догона видит `данные` как `value.Запись`.

### Тесты (ДО реализации T010)

- [x] **T007** [Замок a, ДО реализации] `TestCompleteFirstStepSeesPayload`
  (`engine_test.go`): процесс, где первый авто-шаг догона читает `данные.итог` и
  делает `присвоить факт = данные.итог`; `Complete(t, {"итог":"готово"})` →
  переменная инстанса `факт == "готово"`. Сейчас RED. **Инверсия (a): мутация «убрать
  `stepEnv.Define("данные", cur)`» → `факт` пуст → красный.**
- [x] **T008** [P] `TestPayloadTypesInStep`: payload `{"сумма":2500000}` → шаг видит
  `данные.сумма` как `Целое`; `{"клиент":{"имя":"А"}}` → `данные.клиент.имя` →
  `Строка "А"` (вложенность/числа — семантика jsonval).
- [x] **T009** [P] `TestPayloadThroughCatchUp`: ветка догона `caughtUp=true`
  (хвост сбойного окна D-4) тоже доносит `данные` до шага (§AU-5.3 — на догоне ввод
  актуален). **Инверсия: не прокинуть data через `catchUp` → шаг видит пусто → красный.**

### Реализация

- [x] **T010** В `advance` (`engine.go`): `cur := data` перед циклом; внутри цикла
  ПОСЛЕ `stepEnv := eval.NewEnvironment(processEnv)` (:262) и ДО `ExecStepBody`
  (:275) — `stepEnv.Define("данные", cur)`. (Сброс `cur` — в US2/T013.) T007/T008/T009
  → зелёные.

**Checkpoint**: US1 самостоятельна — payload доходит до первого шага.

---

## Phase 4: US2 — эфемерность payload (P1)

**Goal**: только первый шаг догона видит `данные`; второй+ видит пусто; payload не
персистируется.

### Тесты (ДО реализации T013)

- [x] **T011** [Замок b-1, ДО реализации] `TestSecondStepSeesEmpty`
  (`engine_test.go`): догон с ДВУМЯ авто-шагами, оба читают `данные.метка`;
  `Complete(t, {"метка":"X"})` → первый шаг сохранил "X", второй сохранил Пусто.
  Сейчас (после T010 без сброса) RED — второй ещё видит "X". **Инверсия (b): мутация
  «убрать `cur = value.NewRecord(nil,nil)`» → второй видит "X" → красный.**
- [x] **T012** [Замок b-2] `TestPayloadNotPersisted`
  (`engine_test.go`/`*_persist_test.go`): после `Complete(t, {"итог":"да"})`
  перечитать инстанс и задачи из Store — ни одно поле не содержит "да"; `store.Task`
  и `store.ProcessInstance` не имеют payload-поля (white-box на структуры/схему).
  **Инверсия (b): мутация «`processEnv.Define` вместо `stepEnv`» ИЛИ «персист payload
  в Task» → красный (payload утёк/сохранён).**
- [x] **T013** [Замок c, ДО реализации] `TestNoFlagEmptyRecord`
  (`engine_test.go` + CLI-уровень): второй `complete` БЕЗ `--данные` на следующей
  задаче → шаг видит `данные` как пустую `Запись` (`данные.итог` → Пусто), exit 0,
  НЕ ошибка. **Инверсия (c): мутация «без флага → ошибка/nil-паника» → красный.**
- [x] **T014** [P] `TestPayloadReadOnly`: тело шага читает `данные.поле` (ок), но
  `присвоить данные = …` → ошибка барьера (read-only, как тело триггера). **Инверсия:
  снять барьер → переприсвоение проходит → замок ловит.**

### Реализация

- [x] **T015** В `advance`: после первой итерации цикла `cur = value.NewRecord(nil,
  nil)` (пустая `Запись`) — последующие шаги того же догона видят пусто. Подтвердить,
  что `Define` идёт в `stepEnv` (НЕ `processEnv`), payload нигде не сохраняется.
  T011/T012/T013/T014 → зелёные.

**Checkpoint**: US2 самостоятельна — эфемерность доказана (видим первый шаг,
второй пуст, не персист).

---

## Phase 5: US3 — CLI-флаг `--данные`, пусто/плохой JSON (P2)

**Goal**: CLI принимает `--данные`, декодирует через jsonval, дословная ошибка.

### Тесты (ДО реализации T019)

- [x] **T016** [Замок d, ДО реализации] `TestCompleteBadJSON`
  (`cmd/ladix/*_test.go`): `complete f t --данные '{не json'` → stderr ровно
  `ladix: неверный JSON в --данные: <деталь>`, exit 2, Store не изменён.
  Дополнительно `--данные '[1,2]'` (массив) → та же ошибка (не-объект). **Инверсия
  (d): переформулировать префикс / exit≠2 / валидация ПОСЛЕ мутаций → красный.**
- [x] **T017** [P] `TestCompleteFlagNeedsValue`: `complete f t --данные` (без
  значения) → stderr `ladix: флаг --данные требует значение`, exit 2 (зеркало
  `--вебхук`).
- [x] **T018** [P] `TestCompleteValidPayloadCLI`: `complete f t --данные
  '{"итог":"ок"}'` (формы `V` и `=V`) → exit 0, шаг получил payload (end-to-end CLI);
  golden-проба, если есть витрина.

### Реализация

- [x] **T019** В `completeMain` (`cmd/ladix/main.go`): парс `--данные` зеркально
  `--вебхук` (формы `X`/`X=`; без значения → `ladix: флаг --данные требует значение`
  exit 2); захватить `payloadRaw`, передать в `completeTask`.
- [x] **T020** В `completeTask`: `data, err := jsonval.PayloadToRecord(payloadRaw)`
  ПЕРЕД `eng.Complete`; err → `ladix: неверный JSON в --данные: <деталь>` exit 2 (до
  Store-мутаций); затем `eng.Complete(taskID, data)`. T016/T017/T018 → зелёные.

**Checkpoint**: US3 самостоятельна — CLI-флаг полностью работает.

---

## Phase 6: Polish & анти-дубль

- [x] **T021** [Замок jsonval-дубль] Проба: `complete`-путь зовёт ровно
  `jsonval.PayloadToRecord`; в B3-диффе НЕТ второго `json.Decoder`/`json.Unmarshal`
  для payload вне jsonval; `eval/source_loader.go` не тронут. **Инверсия (инвариант
  4): второй payload-декодер → находка.**
- [x] **T022** [P] Витрина (если в scope §AU-11): демо `complete --данные` покрыто
  ДЕТЕРМИНИРОВАННЫМИ end-to-end CLI-замками `TestCompleteValidPayloadCLI`/
  `TestCompleteBadJSON`/`TestCompleteFlagNeedsValue`/`TestCompleteNoFlagCLI` (свежая
  БД в `t.TempDir()` → монотонные id, как `TestProcessLifecycleGolden`). Отдельный
  `.ladix`+MANIFEST НЕ заводился: `complete` — DB-stateful поток (run→complete), не
  `ladix run`-golden; запись в большой `examples/MANIFEST.md` вне границы фичи.
  Ошибочную фикстуру НЕ трогали.
- [x] **T023** [P] `go vet ./...` чисто, `gofmt`, `go test -race ./...` зелёный;
  подтвердить 0 новых зависимостей (`go.mod`/`go.sum` без правок payload-related).
- [x] **T024** Финальный проход quickstart.md (4 сценария) вручную/скриптом; сверка
  дословности CLI-ошибки с §AU-10.C; сверка ProcessRuntime=8 и пустого диффа
  store/eval/value.

---

## Замки-инверсии (сводка a-e + инвариант 4)

| Код | Замок | Тест | Мутация → красный |
|-----|-------|------|-------------------|
| **a** | payload виден первым шагом | T007 | убрать `stepEnv.Define("данные", cur)` |
| **b** | эфемерность: второй пусто / не персист | T011 + T012 | `processEnv.Define` вместо `stepEnv` / убрать `cur=пустая Запись` / персист в Task |
| **c** | без флага → пустая `Запись`, не ошибка | T013 | без флага → ошибка/паника |
| **d** | плохой JSON → дословная ошибка exit 2 | T016 | переформулировать префикс / exit≠2 / валидация после мутаций |
| **e** | регресс существующего complete зелёный | T003 | изменение вывода/статусов без `--данные` |
| **inv4** | jsonval переиспользован (не дубль) | T021 | второй payload-декодер вне jsonval |

## Зависимости

- T001-T002 (Setup) → всё.
- T003 → T004 → T005 (Foundational; T003 RED до T004/T005). T006 после T005.
- US1 (T007-T010) после Foundational. T007-T009 ДО T010.
- US2 (T011-T015) после US1 (нужен Define из T010). T011-T014 ДО T015.
- US3 (T016-T020) после Foundational (зовёт `Complete(taskID, data)`); параллельна
  US2 по коду (CLI vs engine), но end-to-end T018 ждёт T010/T015.
- Polish (T021-T024) последними.

## Параллельность

- `[P]` внутри US: T008/T009 (US1), T014 (US2), T017/T018 (US3), T022/T023 (Polish) —
  разные файлы/аспекты.
- US3-CLI (`main.go`) и US2-engine-сброс (`engine.go`) — разные файлы → код можно
  вести параллельно после Foundational.

## Definition of Done

- Замки a-e + inv4 зелёные; каждая мутпроба кусает (RED при инверсии).
- `ProcessRuntime` = 8; дифф `internal/eval`/`internal/store`/`internal/value`/
  frontend пуст по существу; 0 новых зависимостей.
- CLI-ошибка дословно §AU-10.C; детерминизм (FixedClock); `go vet`/race зелёные.
