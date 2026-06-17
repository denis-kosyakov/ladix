---
description: "Task list — 014-real-effects (веха M2, B2)"
---

# Tasks: B2 «Реальные эффекты `вызвать` / `уведомить` через HTTP-вебхук»

**Input**: Design documents from `/specs/014-real-effects/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/ (external-caller.md, webhook-wire.md, cli-webhook.md, golden-en7.md)

**Tests**: ВКЛЮЧЕНЫ (Принцип VI tests-first; юнит-замки драйверов/кодека/проводки + golden-инвариант §EN-7 пишутся вместе с кодом). Для КАЖДОГО тест-замка указана инверсная мутация продукт-кода, обязанная его покраснить (исполняется в L1-Реализация). Реальный POST — только под `net/http/httptest` (stdlib).

**Organization**: четыре user-story. US1 (P1) — дефолт-стаб `printCaller` держит §EN-7 golden (несущий инвариант). US2 (P2) — реальная доставка `webhookCaller` через httptest. US3 (P2) — сбой → `ОшибкаВыполнения` (активация `runtimeErrWrap`). US4 (P2) — CLI-проводка `--вебхук`/`LADIX_WEBHOOK` + ошибка неверного URL. Фаза 2 = кодек `internal/jsonval` (blocking для US2). US1 не зависит от US2/3/4.

**Команды**: сборка/тесты из `src/` (`cd src && go build -o ../ladix ./cmd/ladix`; `cd src && go test ./... -count=1`). Источник истины — `docs/automation-model.md` §AU-4 (D-AU-2); тексты CLI-ошибки/стаба — дословно §AU-10.C / §AU-4.2.

## Format: `[ID] [P?] [Story] Description`
- **[P]**: разные файлы, нет зависимостей от незавершённых задач.
- Каждая задача — путь файла + проверяемый критерий. Тест-задачи несут блок **Инверсия:** (мутация, обязанная покраснить замок).

---

## Phase 1: Setup

- [ ] T001 Подтвердить стартовую базу на ветке `014-real-effects`: `cd src && gofmt -l . && go vet ./... && go test ./... -count=1 && go build ./...` — всё зелёное («зелёный до правок»). Зафиксировать вывод в леджере. Подтвердить HEAD = B1-мерж `38e1c78` (метод шва `CallExternalResult` уже есть).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: новый нейтральный кодек `internal/jsonval` (лифт декодера из `daemon` + новый энкодер) — общая инфра для US2 (тело/ответ вебхука). Также фиксация неприкосновенного.

- [ ] T002 [P] Зафиксировать в леджере карту неприкосновенного (НЕ трогать ни в одной фазе): `src/internal/store` (пустой дифф, Store-схема не меняется), `src/internal/lexer`/`parser`/`ast` (грамматика неизменна, FR-020), шов `ProcessRuntime` = 8 методов (B1 уже дал `CallExternalResult`; НЕ расширять, FR-018), второй JSON-декодер источников M1 (`eval/source_loader.go` — НЕ сливать с `jsonval`), тегированный `store/codec.encodeValue` (`{"т","зн"}` — НЕ переиспользовать для тела вебхука), `errors_golden` счётчик `len(seen)=28` (не трогать). `eval` НЕ импортирует `store`/`engine`/`jsonval`.
- [ ] T003 [P] Red→green C-JSONVAL-1 (декодер лифтнут) в `src/internal/jsonval/decode_test.go`: перенести `TestPayloadToRecordValueTypes` (был `daemon/events_test.go:174`) — зовёт `jsonval.PayloadToRecord`/`DecodeValue`; объект→`Запись` (порядок ключей), число без `.eE`→`Целое` иначе `Дробное`, `null`→Пусто, массив→`Список`. Критерий: СЕЙЧАС red (пакета `jsonval` нет).
  **Инверсия:** при лифте потерять порядок ключей `Запись` ИЛИ перепутать `Целое`/`Дробное` → значения ≠ ожидаемым → red.
- [ ] T004 [P] Red→green C-JSONVAL-2 (новый энкодер) в `src/internal/jsonval/encode_test.go`: `TestEncodeBodyPlainJSON` — `EncodeBody("crm", []value.Value{value.Строка("клиент")})` == `{"цель":"crm","данные":["клиент"]}` байт-в-байт; и `TestEncodeValueTypes` по таблице (Целое→число, Дробное→число, Строка→quoted, Булево→true/false, Пусто→null, Список→array, Запись→object с порядком ключей); БЕЗ обёртки `{"т","зн"}`. Критерий: red (энкодера нет).
  **Инверсия:** энкодер пишет тегированный `{"т":"Строка","зн":"клиент"}` (как `store/codec`) → тело ≠ plain-JSON → red; ИЛИ `Запись` теряет порядок ключей → детерминизм-замок red.
- [ ] T005 Реализация `internal/jsonval`: создать пакет — `decode.go` (лифт `PayloadToRecord`/`DecodeValue`/`decodeObject`/`decodeArray`/`numberToValue` из `daemon/events.go:95-206`, экспортировать нужное) + `encode.go` (новый `EncodeBody(target, args)` и `encodeValue(v)` — нетегированный; `Дата`/`Длительность`/`Период`→строковая форма, ЗАДОКУМЕНТИРОВАТЬ выбор в doc-комментарии). Импортировать только `value`+stdlib. Критерий: T003/T004 зеленеют.
- [ ] T006 Co-land `daemon`: переписать `src/internal/daemon/events.go` на делегирование `jsonval` (удалить лифтнутые функции, импортировать `jsonval`); убрать осиротевший `TestPayloadToRecordValueTypes` из `daemon/events_test.go` (переехал в T003). Критерий: `cd src && go test ./internal/daemon/... -count=1` зелёный; импорт-граф без циклов.
- [ ] T007 Инвариант листовости `jsonval`: `cd src && go list -deps ./internal/jsonval | grep -E 'internal/(eval|engine|store|ast|parser|lexer)$'` — пусто (импортирует только `value`+stdlib). И `go list -deps ./internal/eval | grep 'internal/jsonval$'` — пусто (eval не тянет jsonval). Критерий: обе команды без вывода.

**Checkpoint**: кодек готов; можно начинать US1-US4.

---

## Phase 3: User Story 1 — дефолт-стаб `printCaller` держит §EN-7 golden (Priority: P1) 🎯 MVP

**Goal**: интерфейс `ExternalCaller` + Option `WithExternalCaller`; дефолт `printCaller` = перенос печать-логики `runtime.go:42-73` байт-в-байт; методы движка делегируют `e.caller`; golden §EN-7 цел.

**Independent Test**: движок БЕЗ `WithExternalCaller` печатает `вызвать`/`уведомить` байт-в-байт §EN-7; `вызвать`-выражение под стабом → `Пусто`; пины `engine_test 108/167/176/185/194`, `main_test 117/200/235` зелёные.

### Tests for US1 (tests-first — ДОЛЖНЫ упасть до реализации) ⚠️

- [ ] T008 [P] [US1] Red→green C-CALLER-1 (контракт интерфейса/реализаций) в `src/internal/engine/caller_test.go`: компиляц.-замки `var _ ExternalCaller = printCaller{}` и `var _ ExternalCaller = webhookCaller{}`; `var _ eval.ProcessRuntime = (*Engine)(nil)` цел. Критерий: СЕЙЧАС red (типов `ExternalCaller`/`printCaller`/`webhookCaller` нет).
  **Инверсия:** убрать метод `Notify` из интерфейса/реализации → `printCaller`/`webhookCaller` не реализуют `ExternalCaller` → не компилируется → red.
- [ ] T009 [P] [US1] Red→green C-EN7-1 (форматы стаба байт-точно) в `src/internal/engine/caller_test.go`: `TestPrintCallerFormats` — `printCaller{out}.Call("crm", [клиент,5])` печатает ровно `[вызов] crm(клиент, 5)\n` и возвращает `(value.None, nil)`; `.Notify("ИТ", [x])` → `[уведомление] ИТ: x\n`; `.Notify("дежурный", [])` → `[уведомление] дежурный\n` (без двоеточия/хвоста). Критерий: red до реализации `printCaller`.
  **Инверсия:** изменить формат (`[call]`, разделитель `,` без пробела, двоеточие при пустых args) → exact-match строк → red.
- [ ] T010 [P] [US1] Red→green C-OPT-1 (дефолт + Option) в `src/internal/engine/caller_test.go`: `TestDefaultCallerIsPrintCaller` — `NewEngine(...)` БЕЗ `WithExternalCaller` → исполнение `вызвать`/`уведомить` даёт строки стаба в `out` (сеть не трогается). `TestWithExternalCallerOverrides` — `NewEngine(..., WithExternalCaller(fake))` → методы движка зовут `fake`, не печатают стаб. Критерий: red (Option/поля нет).
  **Инверсия:** дефолт `e.caller = webhookCaller{}` → `TestDefaultCallerIsPrintCaller` КРАСНЕЕТ (паника на nil-URL/нет печати); `WithExternalCaller` игнорирует аргумент → `TestWithExternalCallerOverrides` red.
- [ ] T011 [P] [US1] Red→green C-SEAM-DELEG (делегирование без двойного эффекта) в `src/internal/engine/caller_test.go` (или `runtime_test.go`): `TestCallExternalDelegatesNoDouble` — `engine.CallExternal("crm", args)` под стабом печатает РОВНО одну строку `[вызов] crm(...)\n` (нет двойной печати); `TestCallExternalResultReturnsNone` — `CallExternalResult` под стабом → `(value.None, nil)`. Критерий: red до делегирования.
  **Инверсия:** `CallExternal` печатает САМ И зовёт `Call` (тоже печатает) → две строки → red; `Call` стаба возвращает не-None → второй замок red.

### Implementation for US1

- [ ] T012 [US1] **Интерфейс + драйверы (объявления)**: создать `src/internal/engine/caller.go` — `type ExternalCaller interface { Call(...) (value.Value, error); Notify(...) error }`; `type printCaller struct{ out io.Writer }` с переносом печать-логики из `runtime.go:42-73` (форматы `[вызов] %s(%s)`/`[уведомление] %s`/`[уведомление] %s: %s` байт-в-байт; `Call`→`(value.None, nil)`; `Notify`→печать+nil); каркас `type webhookCaller struct{ baseURL string; httpClient *http.Client }` (тело — в US2 T018). Критерий: T008/T009 зеленеют.
- [ ] T013 [US1] **Option + поле движка**: в `src/internal/engine/clock.go` добавить `func WithExternalCaller(c ExternalCaller) Option { return func(e *Engine){ e.caller = c } }` (рядом с `WithClock`); в `src/internal/engine/engine.go` — поле `caller ExternalCaller` + дефолт `e.caller = printCaller{out: e.out}` в `NewEngine` ДО применения `opts...`. Критерий: T010 зеленеет.
- [ ] T014 [US1] **Делегирование методов движка**: в `src/internal/engine/runtime.go` переписать `CallExternalResult`→`return e.caller.Call(target, args)`; `CallExternal`→`{ _, err := e.caller.Call(target, args); return err }`; `Notify`→`return e.caller.Notify(target, args)`. Удалить старую инлайн-печать (она переехала в `printCaller`). `var _ eval.ProcessRuntime = (*Engine)(nil)` цел. Критерий: T011 зеленеет.
- [ ] T015 [US1] Мутант-доказательство §EN-7-дефолта (ЯКОРЬ B2): временно сделать дефолтом `webhookCaller{}` (вместо `printCaller`) → `cd src && go test ./internal/engine/... ./cmd/ladix/... -count=1` ДОЛЖЕН упасть (golden §EN-7 пины краснеют); вернуть. Зафиксировать в леджере (анти-регресс реально кусается, не «полый»).
- [ ] T016 [US1] Инвариант FR-005 / golden §EN-7: `cd src && go test ./internal/engine/... ./cmd/ladix/... -count=1` — пины `engine_test 108/167/176/185/194`, `main_test 117/200/235` и семья `TestNotifyCallFormats` зелёные, тексты байт-в-байт без правки. Критерий: 0 изменений golden-текстов под дефолт-драйвером.

**Checkpoint**: US1 (MVP) функционален; дефолт = стаб байт-в-байт, §EN-7 цел. B2 безопасно вливаем даже без US2-4.

---

## Phase 4: User Story 2 — реальная доставка `webhookCaller` (Priority: P2)

**Goal**: `webhookCaller` делает POST JSON `{"цель","данные"}` на baseURL; ответ `Call` декодируется (пустое тело→Пусто); `Notify` best-effort; через `httptest`.

**Independent Test**: `httptest`-сервер фиксирует POST/Content-Type/тело; `вызвать crm(клиент)` → тело `{"цель":"crm","данные":["клиент"]}`; ответ-объект → `Запись`; пустое тело → `Пусто`.

### Tests for US2 (tests-first) ⚠️

- [ ] T017 [P] [US2] Red→green C-WIRE-1 (POST + тело) в `src/internal/engine/caller_test.go`: `TestWebhookCallerCallPostsBody` — `httptest`-сервер; `webhookCaller{srv.URL, srv.Client()}.Call("crm", [клиент])` → сервер получил `POST`, `Content-Type: application/json`, тело ровно `{"цель":"crm","данные":["клиент"]}`. `TestWebhookCallerNotifyPostsBody` — `Notify("ИТ", [x])` → тело `{"цель":"ИТ","данные":["x"]}`, ответ игнорируется. Критерий: red (тело `webhookCaller` не реализовано).
  **Инверсия:** `webhookCaller.Call` не шлёт POST (или шлёт GET / не на baseURL / без Content-Type / тегированное тело) → сервер не получил ожидаемое → red.
- [ ] T018 [P] [US2] Red→green C-WIRE-2 (декод ответа) в `src/internal/engine/caller_test.go`: `TestWebhookCallerDecodesObject` — сервер отвечает `{"статус":"ок"}` → `Call` вернул `Запись{статус:"ок"}`. `TestWebhookCallerEmptyBodyIsNone` — сервер отвечает пустым телом → `Call` вернул `value.None`. Критерий: red до реализации декода.
  **Инверсия:** пустое тело идёт прямо в `DecodeValue` (без guard) → ошибка вместо `Пусто` → `TestWebhookCallerEmptyBodyIsNone` red; декод через `PayloadToRecord` (требует объект) → падает на скалярном ответе → red.
- [ ] T019 [P] [US2] Red→green C-WIRE-3 (типы тела) в `src/internal/engine/caller_test.go` (или опираясь на `jsonval` T004): `TestWebhookBodyArgTypes` — аргументы `Целое`/`Дробное`/`Строка`/`Булево`/`Пусто`/`Список`/`Запись` сериализуются в plain-JSON в `данные[]` (число/quoted/bool/null/array/object), БЕЗ `{"т","зн"}`. Критерий: red до интеграции энкодера в драйвер.
  **Инверсия:** `webhookCaller` кодирует `данные` через `value.String` (дисплей) или `store/codec` (тегированный) → тело ≠ plain-JSON → red.

### Implementation for US2

- [ ] T020 [US2] **Тело `webhookCaller`**: реализовать в `src/internal/engine/caller.go` методы `Call`/`Notify`: построить тело `jsonval.EncodeBody(target, args)`; `httpClient.Post(baseURL, "application/json", body)`; сетевой/HTTP-сбой → `error`; для `Call` — пустое тело ответа (проверка длины/EOF ДО декода) → `value.None`, иначе `jsonval.DecodeValue(resp.Body)`; `Notify` — ответ закрыть/игнорировать. Критерий: T017/T018/T019 зеленеют.
- [ ] T021 [US2] Мутант-доказательство провода: временно заставить `Call` слать на пустой/чужой URL (или GET) → `cd src && go test ./internal/engine/ -run Webhook -count=1` ДОЛЖЕН упасть (сервер не получил POST); вернуть. Зафиксировать в леджере.

**Checkpoint**: реальный драйвер доставляет POST и декодирует ответ под httptest.

---

## Phase 5: User Story 3 — сбой → `ОшибкаВыполнения` (активация `runtimeErrWrap`) (Priority: P2)

**Goal**: активировать `runtimeErrWrap` на двух statement-точках eval; сбой реального драйвера во всех трёх точках → `errors.ОшибкаВыполнения` с цепочкой `Cause`; под стабом ошибки нет.

**Independent Test**: сбойный `webhookCaller` → `вызвать`-выражение / statement `вызвать` / `уведомить` дают `ОшибкаВыполнения` (через `errors.As`) с непустым `Cause`; стаб → nil.

### Tests for US3 (tests-first) ⚠️

- [ ] T022 [P] [US3] Red→green C-ERR-1 (statement-точки оборачивают) в `src/internal/eval/stmt_test.go`: `TestCallActionWrapsError` — фейк-runtime, чей `CallExternal` возвращает `errors.New("boom")` → исполнение statement `вызвать crm(x)` даёт `errors.ОшибкаВыполнения` (через `errors.As`) с `Pos` == токен `вызвать` и `Cause` == исходная ошибка. `TestNotifyActionWrapsError` — то же для `уведомить` (`Notify` фейка возвращает ошибку). Критерий: red СЕЙЧАС (точки на голом `runtimeErr(...err.Error())`, цепочки `Cause` нет).
  **Инверсия:** оставить `runtimeErr(c.Pos(), err.Error())` (без `Wrap`) → тип не `ОшибкаВыполнения` / нет `Cause` → `errors.As` не находит → red.
- [ ] T023 [P] [US3] Red→green C-ERR-2 (единая категория, реальный драйвер) в `src/internal/engine/caller_test.go`: `TestWebhookCallerCallErrorPropagates` — `webhookCaller` на неотвечающий адрес (или сервер 5xx) → `Call`/`Notify` возвращают непустой `error`. Интеграционно (через движок+фейк-eval или прямой вызов) убедиться, что ошибка драйвера достигает обёртки. Критерий: red до реализации тела (US2) — связка с T020.
  **Инверсия:** `webhookCaller` глотает сетевой сбой (возвращает nil-error) → `errors.As`-замок выше не получает причину → red.
- [ ] T024 [P] [US3] Red→green C-ERR-3 (стаб без ошибки) в `src/internal/engine/caller_test.go`: `TestPrintCallerNeverErrors` — `printCaller.Call`/`.Notify` ВСЕГДА `nil`-ошибка (Call→`(None,nil)`). Критерий: зелёный после T012; здесь как регресс-замок.
  **Инверсия:** `printCaller.Call` начинает возвращать ошибку → дефолт-путь падает → red.

### Implementation for US3

- [ ] T025 [US3] **Активация `runtimeErrWrap`**: в `src/internal/eval/stmt.go` заменить `:118` `runtimeErr(c.Pos(), err.Error())`→`runtimeErrWrap(c.Pos(), err)` (`evalCallAction`) и `:138` `runtimeErr(n.Pos(), err.Error())`→`runtimeErrWrap(n.Pos(), err)` (`evalNotifyAction`); удалить TODO(D-14)-комментарии `:113-115` / `:133-135`. `вызвать`-выражение уже на `runtimeErrWrap` (B1) — не трогать. Критерий: T022 зеленеет; все три точки → `ОшибкаВыполнения`.
- [ ] T026 [US3] Мутант-доказательство обёртки: временно вернуть одну точку на `runtimeErr(...err.Error())` → `cd src && go test ./internal/eval/ -run 'Wraps' -count=1` ДОЛЖЕН упасть; вернуть. Зафиксировать в леджере. Также grep-замок: на `:118`/`:138` нет `runtimeErr(…err.Error())`, TODO(D-14)-комментарии отсутствуют.

**Checkpoint**: сбой реального вызова диагностируется как `ОшибкаВыполнения` с причиной; стаб цел.

---

## Phase 6: User Story 4 — CLI-проводка `--вебхук` / `LADIX_WEBHOOK` + ошибка URL (Priority: P2)

**Goal**: `run`/`serve`/`complete`/`start` принимают `--вебхук`/env; валидный URL→`webhookCaller`, иначе стаб; невалидный→`ladix: неверный URL вебхука '<URL>'` exit 2; serve=единый движок.

**Independent Test**: `run --вебхук <httptest>`→POST; env активирует; невалидный URL→stderr дословно+exit 2; без флага→стаб; `serve`-эскалация→тот же вебхук.

### Tests for US4 (tests-first) ⚠️

- [ ] T027 [P] [US4] Red→green C-CLI-1 (флаг+env активируют POST) в `src/cmd/ladix/webhook_cli_test.go`: `TestRunWebhookFlagPosts` — `run <файл с уведомить> --вебхук <httptest>` → сервер получил `POST` `{"цель":"ИТ","данные":["x"]}`, stdout БЕЗ `[уведомление]` (стаб не печатается). `TestRunWebhookEnvPosts` — env `LADIX_WEBHOOK=<httptest>` без флага → POST. `TestWebhookFlagBeatsEnv` — оба заданы → POST на флаг-URL. Критерий: red (проводки нет).
  **Инверсия:** env не читается (только флаг) → `TestRunWebhookEnvPosts` red; env читается поверх флага → `TestWebhookFlagBeatsEnv` red; флаг не активирует драйвер → `TestRunWebhookFlagPosts` red (стаб печатается, POST нет).
- [ ] T028 [P] [US4] Red→green C-CLI-2 (ошибка неверного URL, ДОСЛОВНО) в `src/cmd/ladix/webhook_cli_test.go` (или `main_test.go`): `TestWebhookInvalidURL` — `run <файл> --вебхук '://мусор'` → stderr ровно `ladix: неверный URL вебхука '://мусор'\n`, exit 2, stdout пуст, движок не запускается. Критерий: red (валидации нет).
  **Инверсия:** изменить текст (`неправильный URL` / без кавычек / в stdout / exit 1) → exact-match golden red; пропустить валидацию (URL не проверяется) → движок строится с битым драйвером → нет ошибки → red.
- [ ] T029 [P] [US4] Red→green C-CLI-3 (serve = единый движок) в `src/cmd/ladix/webhook_cli_test.go` (или `serve_golden_test.go`-семья): `TestServeWebhookEscalationPosts` — `serve <файл, эскалирующий дедлайн> --db <tmp> --вебхук <httptest>` (FixedClock/короткий interval) → догон дедлайна → тело эскалации `уведомить`/`вызвать` идёт `POST` на вебхук (не стаб). Критерий: red до проводки serve.
  **Инверсия:** `serve` строит ОТДЕЛЬНЫЙ движок для догона без вебхука → эскалация печатает стаб → httptest не получил POST → red.
- [ ] T030 [P] [US4] Red→green C-CLI-4 (без флага → стаб) в `src/cmd/ladix/webhook_cli_test.go`: `TestRunNoWebhookUsesStub` — `run <файл>` БЕЗ флага/env → stdout содержит `[вызов]`/`[уведомление]` стаба (§EN-7), сеть не трогается. Критерий: зелёный после T031 (регресс-замок дефолта на CLI-уровне).
  **Инверсия:** проводка ставит `webhookCaller` даже без флага/env → стаб не печатается → red.

### Implementation for US4

- [ ] T031 [US4] **CLI-хелпер проводки**: добавить в `src/cmd/ladix/main.go` хелпер `openExternalCaller(webhookFlag string) (engine.ExternalCaller, error)`: URL = флаг, иначе `os.Getenv("LADIX_WEBHOOK")`; пусто→`(nil,nil)`; невалидный URL (`net/url.ParseRequestURI` или экв.)→`fmt.Errorf("неверный URL вебхука '%s'", url)`; валидный→`webhookCaller{baseURL:url, httpClient:&http.Client{Timeout:…}}`. Подключить в `run`/`complete` (флаг `--вебхук`); ошибку печатать штатным CLI-каналом `ladix: <текст>` stderr exit 2; `WithExternalCaller(c)` только при `c!=nil`. Критерий: T027/T028/T030 зеленеют.
- [ ] T032 [US4] **Проводка `serve`**: в `src/cmd/ladix/serve.go` подключить `--вебхук`/env через тот же хелпер; провести результат в ТОТ ЖЕ экземпляр движка, чьи `Notify`/`Call` зовёт догон дедлайнов и тело триггеров (НЕ создавать отдельный движок). Критерий: T029 зеленеет.
- [ ] T033 [US4] **Проводка `complete`/`start`**: подключить `--вебхук`/env в `complete` (`main.go`); для `start` (B5) — если команда уже поставлена, подключить тем же хелпером; иначе зафиксировать в леджере зависимость co-land с B5 (проводка `--вебхук` в `start` едет с той подфичей, не дублировать команду здесь). Критерий: `complete` несёт `--вебхук`; статус `start` зафиксирован.
- [ ] T034 [US4] Мутант-доказательство CLI: временно сделать так, чтобы хелпер игнорировал env (читал только флаг) → `cd src && go test ./cmd/ladix/ -run Webhook -count=1` ДОЛЖЕН упасть (`TestRunWebhookEnvPosts`); вернуть. Также временно убрать валидацию URL → `TestWebhookInvalidURL` падает; вернуть. Зафиксировать в леджере.

**Checkpoint**: реальный драйвер достижим из CLI; невалидный URL диагностируется дословно; serve доставляет на вебхук.

---

## Phase 7: Интеграция и инварианты (все US)

- [ ] T035 Инвариант FR-018 (шов цел): `cd src && go list -deps ./internal/eval | grep -E 'internal/(store|engine|jsonval)$'` — пусто; подтвердить `ProcessRuntime` = РОВНО 8 методов (B1, без изменений). Критерий: команда без вывода; интерфейс 8 сигнатур.
- [ ] T036 Инвариант FR-019 (0 зависимостей, httptest): `git diff master -- src/go.mod src/go.sum` пуст (единственная `modernc.org/sqlite`); `grep -rn 'httptest' src/internal/engine src/cmd/ladix` — только в `_test.go` (реальный HTTP только под httptest). Критерий: go.mod не изменён; httptest только в тестах.
- [ ] T037 Инвариант пустого диффа фронтенда/store: `git diff --stat master -- src/internal/lexer src/internal/parser src/internal/ast src/internal/store` — пусто (грамматика/AST/Store-схема не тронуты, FR-020). Критерий: пустой дифф.
- [ ] T038 Полный гейт + интеграция: `cd src && gofmt -l . && go vet ./... && go test ./... -count=1 -race && go build -o ../ladix ./cmd/ladix`. Прогнать quickstart-сниппеты (дефолт-стаб без флага → §EN-7 + `Пусто`; `--вебхук <httptest>` → POST + декод; `--вебхук '://мусор'` → ошибка exit 2; env активирует). Критерий: всё зелёное; поведение совпадает с quickstart.md.
- [ ] T039 Дрейф-аудит §AU-2 / §AU-4: подтвердить `ProcessRuntime` = 8 (не расширён); `Store` не тронут; ребро `engine→eval` однонаправленно; golden §EN-7 байт-в-байт под дефолтом; CLI-ошибка дословно §AU-10.C; форматы стаба дословно §AU-4.2; новый драйвер/jsonval — 0 новых внешних зависимостей. Зафиксировать в леджере для M2-гейта (синк больших доков — архитектор).

**Checkpoint**: B2 завершён; инварианты 1-3 закрыты; дефолт-стаб держит §EN-7, реальный драйвер доставляет, сбой диагностируется, CLI проведён.

---

## Карта покрытия (FR → задачи)

| FR | Задачи |
|---|---|
| FR-001 интерфейс `ExternalCaller` | T008, T012 |
| FR-002 `WithExternalCaller` + дефолт `printCaller` | T010, T013 |
| FR-003 делегирование методов движка | T011, T014 |
| FR-004 `printCaller` форматы байт-точно | T009, T012 |
| FR-005 golden §EN-7 цел дефолтом | T015, T016 |
| FR-006 `webhookCaller` POST `{"цель","данные"}` | T017, T020 |
| FR-007 энкодер value→plain-JSON | T004, T019, T005 |
| FR-008 декод ответа + пустое тело→Пусто | T018, T020 |
| FR-009 `Notify` best-effort | T017, T020 |
| FR-010 `internal/jsonval` нейтральный | T003, T005, T007 |
| FR-011 httpClient timeout + httptest | T017, T036 |
| FR-012 сбой→`ОшибкаВыполнения` с `Cause` | T022, T023, T025 |
| FR-013 `runtimeErrWrap` 3 точки | T022, T025, T026 |
| FR-014 стаб без ошибки | T024 |
| FR-015 CLI `--вебхук`/env run/serve/complete/start | T027, T031, T032, T033 |
| FR-016 ошибка неверного URL (дословно) | T028, T031 |
| FR-017 serve = единый движок | T029, T032 |
| FR-018 шов 8, eval без store/engine/jsonval | T007, T035 |
| FR-019 0 зависимостей, httptest | T036 |
| FR-020 грамматика/AST/lexer/parser неизменны | T037 |

## Зависимости (порядок)

- T001 → T002 → (фаза 2 кодек T003-T007) → US1 (фаза 3) → US2/US3/US4 (фазы 4-6) → инварианты (фаза 7).
- **US1 (T008-T016) НЕ зависит от US2-4** — дефолт-стаб самодостаточен (MVP). US2 (T017-T021) требует кодек (T005) и каркас `webhookCaller` (T012). US3 (T022-T026) — `runtimeErrWrap`-правка eval независима, но C-ERR-2/3 опираются на тело драйвера (T020). US4 (T027-T034) требует драйвер (T020) и интерфейс (T012).
- Мутант-доказательства (T015, T021, T026, T034) — после соответствующей реализации.
- Инварианты (T035-T039) — последними, после зелёной реализации.
- Тесты [P] независимы по файлам внутри фазы.

## Итог

**39 задач** (T001-T039). Фазы: Setup (1), Foundational/кодек (6: T002-T007, из них 2 тест-замка кодека), US1-дефолт-стаб (9: T008-T016, 4 теста + 5 импл/инвариант), US2-вебхук (5: T017-T021, 3 теста + 2 импл), US3-ошибки (5: T022-T026, 3 теста + 2 импл), US4-CLI (8: T027-T034, 4 теста + 4 импл), Интеграция/инварианты (5: T035-T039).

**Тест-замков — 16**: кодек T003/T004 (2); US1 T008/T009/T010/T011 (4); US2 T017/T018/T019 (3); US3 T022/T023/T024 (3); US4 T027/T028/T029/T030 (4). Каждый с явной инверсной мутацией. **Мутант-доказательств — 4** (T015 §EN-7-дефолт-якорь, T021 провод, T026 обёртка, T034 CLI).

**Ключевые инверсии (по заданию дирижёра)**:
- **(a) golden-дефолт**: T015 — дефолт = `webhookCaller` вместо `printCaller` → §EN-7 golden КРАСНЕЕТ (якорный анти-регресс B2).
- **(b) httptest-POST**: T017/T021 — `webhookCaller` не шлёт POST / шлёт не туда / тегированное тело → httptest-замок КРАСНЕЕТ.
- **(c) CLI-ошибка URL**: T028 — текст изменён / валидация пропущена → exact-match `ladix: неверный URL вебхука '<URL>'` КРАСНЕЕТ.
