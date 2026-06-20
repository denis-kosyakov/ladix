---
description: "Задачи реализации трека B «Входящие события» (HTTP-приём) — фича 025-inbound-events"
---

# Tasks: Входящие события (HTTP-приём)

**Input**: Design documents from `/specs/025-inbound-events/`
**Anchor**: `docs/inbound-events-model.md` §IE-0..§IE-8 (D-IE-1..D-IE-10)
**Prerequisites**: plan.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: ВКЛЮЧЕНЫ (явно требуются §IE-7 FR-IE-1..11; httptest, симметрично `webhook_cli_test.go`). Каждый замок обязан кусаться (мутпроба: снять проверку → тест краснеет).

**Границы (барьеры, не ломать)**:
- Прод-дифф СТРОГО в `src/cmd/ladix/` (`emit.go` рефактор · `serve.go` · НОВЫЙ `events_http.go`).
- ПУСТОЙ дифф `src/internal/{store,engine,daemon,eval}` (контракт `Store`=18, `drainEvents` целы).
- 0 новых зависимостей (`grep require src/go.mod` → только `modernc.org/sqlite`). stdlib-only, без `errgroup`/`x/text`.
- Существующие barrier-замки `daemon_test.go:15-47`, `serve_golden_test.go:310-330/361-371` (NumGoroutine after≤before) — зелёные НЕТРОНУТЫ.

**Path conventions**: рабочая директория сборки/тестов — `src/`. Все пути ниже относительны корня репо.

---

## Phase 1: Setup

- [x] T001 Зафиксировать зелёную базу: `cd src && go build ./... && go vet ./... && go test ./...` — всё зелёное ДО правок (baseline для регресс-сравнения).
- [x] T002 Подтвердить инвариант зависимостей: `grep require src/go.mod` показывает единственную прямую `modernc.org/sqlite` (запомнить как замок для T021).

---

## Phase 2: Foundational (Blocking) — общий хелпер минта (D-IE-8)

**Назначение**: единый путь минта для `emit` и HTTP-хендлера. Блокирует US1/US2/US3 (хендлер зовёт `enqueueEvent`).

- [x] T003 В `src/cmd/ladix/emit.go` добавить СВОБОДНУЮ функцию `enqueueEvent(st store.Store, name, payload string, clock engine.Clock) (string, error)` по контракту `contracts/enqueue-helper.md`: `NextEventID` → `&store.Event{ID,Name,PayloadJSON,CreatedAt:clock.Now(),Processed:false}` → `EnqueueEvent` → `(id, nil)`; ack-печать НЕ входит.
- [x] T004 В `src/cmd/ladix/emit.go` переписать тело `emitEvent` (строки 65-83) на использование `enqueueEvent(sq, name, payload, clock)`; при ошибке — прежний текст `ladix: сбой хранилища: <err>` + `return 2`; при успехе — прежний ack `событие %s '%s' поставлено в очередь` (сигнатура `emitEvent` неизменна, поведение байт-идентично).
- [x] T005 Регресс-замок рефактора: в `src/cmd/ladix/emit_golden_test.go` (существующий emit-тест) убедиться/добавить, что `emit <имя> <json> --db D` печатает дословно `событие e-000001 '<имя>' поставлено в очередь` (ack `emit` НЕ изменился). `go test ./cmd/ladix/ -run Emit` зелёный.

**Checkpoint**: `enqueueEvent` готов; `emit` без регресса.

---

## Phase 3: User Story 1 — Приём по сети → тик → срабатывание триггера (P1) 🎯 MVP

**Goal**: внешний `POST /events/{имя}` кладёт событие в очередь (`202`), на тике тело триггера срабатывает; источник неразличим от `emit`.

**Independent Test**: поднять serve с `--listen`+`--db`, `POST /events/<кириллица>` `{json}` → `202`, тик → тело триггера сработало с теми же значениями, что `emit`.

### Реализация US1

- [x] T006 [US1] Создать `src/cmd/ladix/events_http.go`: функция `eventsHandler(st store.Store, clock engine.Clock, token string) http.Handler` по `contracts/http-endpoint.md` (R2). Порядок: метод≠POST→405 (`ladix: метод не поддерживается, только POST`); [auth — задел для US2, пока `token==""`]; имя=`strings.TrimPrefix(r.URL.Path, "/events/")`, пусто→400 (`ladix: пустое имя события`); `body,_:=io.ReadAll(r.Body)`; `id,err:=enqueueEvent(st,name,string(body),clock)`; err→500 (`ladix: сбой хранилища`); ok→202 (`событие %s '%s' принято\n`). НЕ импортировать `engine.Engine`/`eval` (FR-IE-2).
- [x] T007 [US1] В `src/cmd/ladix/events_http.go` добавить `startEventListener(ln net.Listener, st store.Store, clock engine.Clock, token string) (stop func())` по `contracts/cli-flags.md` (R5): `srv:=&http.Server{Handler: eventsHandler(...)}`; `go srv.Serve(ln)` под `sync.WaitGroup`; `stop`=`srv.Shutdown(ctxTimeout 5s)`+`wg.Wait()`. stdlib-only.
- [x] T008 [US1] В `src/cmd/ladix/serve.go` `serveMain` добавить парсинг `--listen` (формы `--listen v` и `--listen=v`, зеркало `--interval` :82); протянуть `listen string` в `serveFile` (расширить сигнатуру `serveFile(... , listen, token string, ...)`).
- [x] T009 [US1] В `src/cmd/ladix/serve.go` `serveFile`: после открытия Store, ВНЕ guard (рядом с :146-153) — если `listen!=""`: `ln,err:=net.Listen("tcp",listen)`; err→`ladix: не удалось открыть сокет '%s': %s` + `return 2`. Завести `var clock engine.Clock = engine.SystemClock{}`, передать в `buildServeDaemon` (вместо литерала) И в `startEventListener`.
- [x] T010 [US1] В `src/cmd/ladix/serve.go` ВНУТРИ guard-замыкания (после `buildServeDaemon`, до `signal.NotifyContext`): если `ln!=nil` → `stopListener:=startEventListener(ln,st,clock,token)`; `defer stopListener()`. Подтвердить LIFO: эта defer регистрируется → отрабатывает ДО внешнего `defer sq.Close()` (:152). Без `--listen` (`ln==nil`) — ничего не стартует (FR-IE-1).

### Тесты US1 (`src/cmd/ladix/events_http_test.go`, новый; httptest)

- [x] T011 [P] [US1] FR-IE-3 golden: `httptest.NewServer(eventsHandler(st, fixedClock{...}, ""))`; `http.Post(srv.URL+"/events/"+url.PathEscape("падение_выручки"), …, body)` → `202`, тело `событие e-000001 'падение_выручки' принято`; затем `buildServeDaemon(prog, st, …, fixedClock{...})` над ТЕМ ЖЕ `st`, `go func(){ done<-d.Run(ctx) }()` + `waitUntil(t, func()bool{ evs,_:=st.ListUnprocessedEvents(); return len(evs)==0 })` + `cancel()`+`<-done` → тело триггера `когда событие падение_выручки` сработало (вывод в out). **НЕ `d.tick()`** — он неэкспортирован (пакет `daemon`); гонять через `d.Run(ctx)`, как webhook_cli_test.go:191. Кириллица percent-кодирована (ловит регресс декода).
- [x] T012 [P] [US1] FR-IE-3 эквивалентность: то же событие через `enqueueEvent`/`emit`-путь и через HTTP дают идентичную `store.Event` (Name/PayloadJSON/детерм. CreatedAt от fixedClock) — сравнение по `ListUnprocessedEvents`.
- [x] T013 [P] [US1] FR-IE-7: `POST /events/x` с телом `{битый` (невалидный JSON) → `202`; затем гнать тик через `d.Run(ctx)`+`waitUntil(ListUnprocessedEvents==0)` (НЕ `d.tick()`) — тело всё равно исполняется, событие `processed` (замок кусается, если приём начнёт валидировать JSON).
- [x] T014 [P] [US1] FR-IE-10: `GET /events/x` → `405` (`ladix: метод не поддерживается, только POST`); `POST /events/` (пустой сегмент) → `400` (`ladix: пустое имя события`).
- [x] T015 [P] [US1] FR-IE-6 (500): `type failEnqueueStore struct{ store.Store }` поверх `NewMemoryStore()`, `EnqueueEvent` → ошибка (R9); `POST` → `500` (`ladix: сбой хранилища`), событие НЕ в очереди (202 строго после успешного enqueue).
- [x] T016 [P] [US1] FR-IE-2 (статическая изоляция): компилируемый замок — `var _ = func() http.Handler { return eventsHandler(store.NewMemoryStore(), engine.SystemClock{}, "") }` (сигнатура принимает Store+Clock+string, НЕ `*Engine`). Комментарий-барьер: если в сигнатуру протечёт движок — не скомпилится.
- [x] T017 [US1] FR-IE-8 (no-leak приёмника): `before:=runtime.NumGoroutine()`; `ln,_:=net.Listen("tcp","127.0.0.1:0")`; `stop:=startEventListener(ln,st,fixedClock{...},"")`; (опц. один POST); `stop()`; `waitGoroutines(t, before)` (переиспользовать хелпер serve_golden_test.go:361). Кусается, если `stop` не делает `wg.Wait()`/`Shutdown`.

**Checkpoint**: US1 = работающий MVP (приём по сети + асинхронное исполнение + негативные коды + no-leak).

---

## Phase 4: User Story 2 — Защита токеном (P2)

**Goal**: опциональный `--token`/env защищает эндпоинт; дефолт — выключен.

**Independent Test**: serve `--token СЕКРЕТ`; POST без/с неверным заголовком → `401`; с верным → `202`; без `--token` → `202`.

- [x] T018 [US2] В `src/cmd/ladix/serve.go` `serveMain` добавить парсинг `--token` (формы `--token v`/`--token=v`); если пуст — fallback `os.Getenv("LADIX_LISTEN_TOKEN")` (флаг бьёт env); протянуть `token` в `serveFile`→`startEventListener`.
- [x] T019 [US2] В `src/cmd/ladix/events_http.go` `eventsHandler`: активировать auth-ветку (R6) — `if token != "" { if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Ladix-Token")), []byte(token)) != 1 { 401 `ladix: неверный токен`; return } }` ПОСЛЕ проверки метода, ДО проверки имени.
- [x] T020 [P] [US2] FR-IE-9 тест (`events_http_test.go`): `eventsHandler(st, clock, "СЕКРЕТ")` — POST без заголовка → `401`; с неверным `X-Ladix-Token` → `401`; с верным → `202`. Отдельно `eventsHandler(st, clock, "")` — POST без заголовка → `202` (auth выкл). Инверсия (constant-time → обычное `==`) функционально не краснит, но замок auth-веток кусается на удаление проверки.

**Checkpoint**: US2 независимо проверяема поверх US1.

---

## Phase 5: User Story 3 — Безопасные дефолты и диагностика запуска (P3)

**Goal**: `--listen` требует `--db`; занятый порт → exit 2; не-loopback без токена → предупреждение.

**Independent Test**: `serve --listen` без `--db` → exit 2; занятый порт → exit 2; не-loopback host без `--token` → stderr-warn, демон стартует.

- [x] T021 [US3] В `src/cmd/ladix/serve.go` `serveMain`: ПОСЛЕ цикла парсинга, ДО `serveFile`/`net.Listen` — `if listen!="" && dbPath=="" { fmt.Fprintln(stderr,"ladix: --listen требует --db"); return 2 }` (FR-IE-4, R8).
- [x] T022 [US3] В `src/cmd/ladix/serve.go` (`serveFile` или `serveMain`): loopback-warning (R7) — `host,_,_:=net.SplitHostPort(listen)`; если host не в {`127.0.0.1`,`::1`,`localhost`} (пустой host = не-loopback) И `token==""` → stderr `ladix: ВНИМАНИЕ: --listen на не-loopback адресе без --token — эндпоинт запускает процессы без аутентификации`. НЕ блокировать.
- [x] T023 [P] [US3] FR-IE-4 тест (`events_http_test.go` или `serve_golden_test.go`): `realMain([]string{"serve", fixture, "--listen","127.0.0.1:0"}, &out,&err)` БЕЗ `--db` → код `2`, stderr содержит `ladix: --listen требует --db`. (Проверка ДО открытия сокета.)
- [x] T024 [P] [US3] FR-IE-5 тест: предварительно занять порт `ln0,_:=net.Listen("tcp","127.0.0.1:0")`; `addr:=ln0.Addr().String()`; `realMain([]string{"serve", fixture, "--db", tmpdb, "--listen", addr})` → код `2`, stderr содержит `ladix: не удалось открыть сокет '<addr>'`.
- [x] T025 [P] [US3] (опц.) warning-тест: `realMain` serve на `0.0.0.0:0`-эквиваленте без `--token` печатает warning в stderr (или unit-тест хелпера loopback). Если тяжело детерминировать порт — unit-тест `isLoopbackHost`.

**Checkpoint**: все три истории независимо проверяемы.

---

## Phase 6: Polish & Cross-Cutting

- [x] T026 `cd src && gofmt -l cmd/ladix/` пусто; `go vet ./...` чисто.
- [x] T027 Регресс-барьеры FR-IE-1: `go test ./cmd/ladix/ -run 'Serve|Webhook'` и `go test ./internal/daemon/ -run Shutdown` (вкл. `TestServeGracefulShutdownNoLeak`, `TestRunGracefulShutdown`) зелёные НЕТРОНУТЫ (без правки сути). Подтвердить: serve без `--listen` не стартует сервер.
- [x] T028 Полный прогон: `cd src && go build ./... && go vet ./... && go test ./...` — всё зелёное. `go test -race ./cmd/ladix/ ./internal/daemon/` зелёный (хендлер∥тик).
- [x] T029 Замок зависимостей: `grep require src/go.mod` — по-прежнему только `modernc.org/sqlite` (0 новых). ПУСТОЙ дифф `internal/*`: `git diff --stat master -- src/internal/` пуст.
- [x] T030 Мутпробы (выборочно): снять проверку метода → T014 краснеет; снять auth-ветку → T020 краснеет; вернуть `time.Now()` вместо `clock.Now()` → T011/T012 краснеют; убрать `wg.Wait()` → T017 краснеет. Восстановить.

---

## Dependencies & Execution Order

- **Phase 1 (Setup)** → **Phase 2 (Foundational: enqueueEvent)** блокирует всё.
- **US1 (Phase 3)** зависит от Phase 2; даёт MVP.
- **US2 (Phase 4)** зависит от US1 (handler существует) — добавляет auth-ветку + флаг.
- **US3 (Phase 5)** зависит от US1 (serveMain/serveFile плумбинг) — добавляет CLI-валидации.
- **Phase 6 (Polish)** — после всех.

## Parallel Example

Внутри US1 тесты T011–T016 помечены `[P]` (разные ассерты, один новый файл — писать секциями, гонять параллельно `go test`). Аналогично T023–T025 в US3. Реализационные T006–T010 последовательны (один файл `serve.go`/`events_http.go`).

## Implementation Strategy

**MVP** = Phase 1+2+3 (US1): приём по сети + исполнение + негативные коды + no-leak. Поставляется самостоятельно. US2 (auth) и US3 (дефолты/диагностика) — инкрементальные слои поверх, каждый независимо проверяем.

## Notes

- Детерминизм: все golden — на `fixedClock{time.Date(2026, …)}` (serve_golden_test.go:21-23).
- Ack-тексты `emit` («поставлено в очередь») и HTTP («принято») различны НАМЕРЕННО (D-IE-8) — не выравнивать.
- doc-sync канонов (execution-model/SPEC/README/automation-model/trigger-model) — ВНЕ scope (спарринг-чат после мержа).
