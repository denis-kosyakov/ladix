# Research — Входящие события (HTTP-приём)

Фаза 0. Все «как» взяты из якоря `docs/inbound-events-model.md`; здесь — разрешение TODO-FACT якоря и выбор конкретных stdlib-механизмов. NEEDS CLARIFICATION: нет (анкор полон).

## R1. Маршрутизация и извлечение имени события

**Decision**: НЕ использовать `http.ServeMux` с method-pattern Go 1.22 (`POST /events/{имя}`). Вместо этого — единый `http.Handler` (замыкание), который сам проверяет путь/метод/имя. Имя = `strings.TrimPrefix(r.URL.Path, "/events/")`.

**Rationale**:
- Встроенный 405 от ServeMux отдаёт дефолтное тело «Method Not Allowed», а нам нужен **дословный RU-текст** `ladix: метод не поддерживается, только POST` (Принцип VIII). → метод проверяем сами.
- Паттерн `{имя}` НЕ матчит пустой сегмент `/events/` → ServeMux вернул бы 404, а якорь требует **400** `ladix: пустое имя события`. → пустоту ловим сами.
- `r.URL.Path` уже **percent-декодирован** `net/http` (один проход): для `POST /events/%D0%BF%D0%B0%D0%B4%D0%B5%D0%BD%D0%B8%D0%B5` `r.URL.Path == "/events/падение"`, → `name == "падение"` (UTF-8 байты совпадают с `Event.Name` в исходнике). Матч **строго байтовый**, без Unicode-нормализации (stdlib-only, без `golang.org/x/text`).

**Alternatives considered**:
- ServeMux `{имя}` + `r.PathValue` — отвергнут (405-тело и пустое имя не контролируемы).
- Ручной `url.PathUnescape(r.URL.EscapedPath())` — запасной путь, если `r.URL.Path` даст сюрприз на кириллице (golden FR-IE-3 поймает); по умолчанию `r.URL.Path` достаточно.
- Путь не под `/events/` → `404` (дефолт): handler возвращает 404 на чужой префикс (минимальная вежливость; якорь его не специфицирует).

## R2. Порядок проверок в хендлере

**Decision**: метод → auth → имя → чтение тела → enqueue. Конкретно:
1. `r.Method != http.MethodPost` → `405`.
2. токен задан и `subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Ladix-Token")), []byte(token)) != 1` → `401`.
3. `name == ""` (или содержит `/`) → `400`.
4. `body, _ := io.ReadAll(r.Body)` — сырой текст, **без парсинга** (битый JSON не роняет приём, FR-IE-7).
5. `id, err := enqueueEvent(st, name, string(body), clock)`; `err != nil` → `500`; иначе `202` + ack.

**Rationale**: GET-замок (FR-IE-10) шлёт GET без токена и ждёт 405 → метод раньше auth. Токен-замки (FR-IE-9) шлют POST → доходят до auth. Порядок детерминирован тестами. Раскрытие 405 до auth безопасно (нет чувствительной информации).

## R3. Время создания (CreatedAt) и FIFO

**Decision**: `CreatedAt = clock.Now()` через инъектированный `engine.Clock` (тот же, что у демона; прод — `engine.SystemClock{}`, тест — `fixedClock`). НЕ время клиента, НЕ голый `time.Now()`.

**Rationale** (D-IE-9, FR-IE-11): детерминизм golden (`fixedClock`). FIFO держит **не** `CreatedAt` (RFC3339 без долей секунды → коллизии в одну секунду), а монотонный `NextEventID` + `ORDER BY ... id ASC`. Хендлер и тик делят один `Store` с `MaxOpenConns=1` → конкурентный `EnqueueEvent`∥`ListUnprocessedEvents` сериализован, гонок на минте ID нет (подтверждённый факт якоря §IE-4.5).

## R4. Рефактор минта (D-IE-8)

**Decision**: вынести из `emitEvent` (emit.go:58-85) свободную функцию
`enqueueEvent(st store.Store, name, payload string, clock engine.Clock) (string, error)`:
`NextEventID` → `&store.Event{ID,Name,PayloadJSON,CreatedAt:clock.Now(),Processed:false}` → `EnqueueEvent` → вернуть `(id, nil)`.

**Rationale**: один путь минта для `emit` и HTTP → идентичная семантика `ID`/`CreatedAt` (FR-IE-3 неразличимость). **Ack-печать вне хелпера**: `emit` печатает «поставлено в очередь» (без изменений, emit.go:82), хендлер печатает «принято» — тексты **намеренно различны** (D-IE-8, не выравнивать → иначе ломается golden `emit`). Принимает `store.Store` (интерфейс), не `*SQLiteStore` → работает и над Store демона. Поведение `emitEvent` после рефактора байт-идентично (оба прежних сбоя `NextEventID`/`EnqueueEvent` давали один и тот же текст `ladix: сбой хранилища: <err>` exit 2 — единый путь ошибки сохраняет это).

## R5. Жизненный цикл (stdlib-only)

**Decision**: `net.Listen("tcp", host:port)` **вне** guard (рядом с открытием SQLite, serve.go:146-153); bind-ошибка → exit 2 `ladix: не удалось открыть сокет '<host:port>': <err>`. Сервер: `srv := &http.Server{Handler: eventsHandler(...)}`; `srv.Serve(ln)` в горутине под `sync.WaitGroup`. Остановка — хелпер `stop()`: `srv.Shutdown(ctxTimeout)` + `wg.Wait()`. Координация регистрируется `defer stop()` **внутри guard-замыкания** → по LIFO отрабатывает ДО внешнего `defer sq.Close()` (serve.go:152, в области serveFile). **Без `errgroup`** (`go.mod` не несёт `golang.org/x/sync`).

**Rationale** (D-IE-10, FR-IE-6/8): in-flight POST дослуживается до закрытия Store (иначе запись в закрытый коннект = потеря, нарушение at-least-once). LIFO-порядок: guard-замыкание возвращается → его defer'ы (stopListener, signalStop) → `guard()` возвращает → serveFile `defer sq.Close()`. Так Shutdown+join гарантированно до Close. `srv.Serve` возвращает `http.ErrServerClosed` при Shutdown — игнорируем.

**Тестовый шов**: вынести старт+стоп в `startEventListener(ln, st, clock, token) (stop func())` — тесты дёргают его напрямую с `net.Listen("127.0.0.1:0")` + `fixedClock`, проверяют no-leak через существующий `waitGoroutines(t, before)` (serve_golden_test.go:361). Барьеры FR-IE-1 (`daemon_test.go:15-47`, `serve_golden_test.go:310-330`) остаются зелёными: без `--listen` `ln == nil` → сервер не стартует.

## R6. Auth (§IE-6)

**Decision**: `--token СЕКРЕТ`; если пуст — env `LADIX_LISTEN_TOKEN` (флаг бьёт env, зеркало `--webhook`/`LADIX_WEBHOOK`). Заголовок `X-Ladix-Token`. Сравнение `crypto/subtle.ConstantTimeCompare`. Дефолт — токен пуст → auth выключен (любой POST → 202).

**Rationale**: TODO-FACT якоря (`X-Ladix-Token`/`LADIX_LISTEN_TOKEN`) разрешён в задаче kickoff. Constant-time против тайминг-атак на секрет.

## R7. Дефолт-граница loopback (§IE-3)

**Decision**: если host из `--listen` не loopback (`127.0.0.1`/`::1`/`localhost`; пустой host `:p` = все интерфейсы → НЕ loopback) И токен пуст → предупреждение в stderr `ladix: ВНИМАНИЕ: --listen на не-loopback адресе без --token — эндпоинт запускает процессы без аутентификации`. Не блокирует.

**Rationale**: безопасный дефолт без жёсткого отказа (локальная разработка). Парс host через `net.SplitHostPort`.

## R8. --listen требует --db (D-IE-7)

**Decision**: чистая CLI-проверка в `serveMain` (до `serveFile`/`net.Listen`): `listen != "" && dbPath == ""` → stderr `ladix: --listen требует --db`, exit 2.

**Rationale** (FR-IE-4): без `--db` `serve` поднимает эфемерный `MemoryStore` → `202` на событие, которое исчезнет при рестарте, нарушив at-least-once. Граница приёмки.

## R9. Мок-Store для 500 (FR-IE-6)

**Decision**: тестовый `type failEnqueueStore struct{ store.Store }` поверх реального `MemoryStore`, переопределяет `EnqueueEvent` → возврат ошибки (встраивание интерфейса даёт остальные 17 методов бесплатно). Хендлер на нём → `500`.

**Rationale**: минимальный мок без ручной реализации 18 методов; кусает FR-IE-6 (если 202 начнут возвращать до успешного enqueue — краснеет).
