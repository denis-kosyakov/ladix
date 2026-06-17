# Research: B2 «Реальные эффекты `вызвать` / `уведомить` через HTTP-вебхук»

**Feature**: 014-real-effects | **Date**: 2026-06-17 | **Источник истины**: `docs/automation-model.md` §AU-4 (D-AU-2)

Все технические решения B2 залочены владельцем (§AU-1, D-AU-2). Этот документ фиксирует прецеденты и эмпирику кодовой базы (master @`38e1c78`, B1 влита), на которые опирается реализация, чтобы исключить пере-открытие и дрейф.

## R-1. Прецедент: драйвер через Option = клон `WithClock`

**Решение**: драйвер внешних эффектов инжектируется Option'ом движка ПО ОБРАЗЦУ уже работающего `WithClock` — не изобретается новый механизм конфигурации.

**Эмпирика (живые точки кода)**:
- `src/internal/engine/clock.go:19` — `type Option func(*Engine)`; `:21-24` — `func WithClock(c Clock) Option { return func(e *Engine) { e.clock = c } }`. Это место для `WithExternalCaller(c ExternalCaller) Option`.
- `NewEngine(st, interp, &out, opts ...Option)` применяет опции после дефолтов (тесты зовут `engine.NewEngine(st, interp, &out, engine.WithClock(fixedClock{now}))`, `engine_test.go:43`). Дефолт драйвера ставится ДО применения опций: `e.caller = printCaller{out: e.out}`.

**Вывод**: B2 добавляет поле `caller ExternalCaller`, дефолт `printCaller{out}`, Option `WithExternalCaller`. Механизм DI существует — расширяется на один параметр (Принцип V: без глобалов).

## R-2. Печать-логика стаба переносится в `printCaller` байт-в-байт (§EN-7 golden)

**Эмпирика**: текущая печать живёт прямо в методах движка `src/internal/engine/runtime.go`:
- `:43-49` `CallExternalResult` — `fmt.Fprintf(e.out, "[вызов] %s(%s)\n", target, strings.Join(parts, ", "))`, `parts[k]=value.String(...)`, возвращает `(value.None, nil)`.
- `:55` `CallExternal` — делегирует `CallExternalResult` с отбросом значения (комментарий `:53`: «печать `[вызов]` происходит РОВНО один раз»).
- `:64-73` `Notify` — `len(args)==0` → `[уведомление] %s\n`; иначе `[уведомление] %s: %s\n` (разделитель аргументов — один пробел).

**§EN-7-пины (НЕ ломать)**: `engine_test.go:108` (`уведомление` с аргументами в составном выводе), `:167` (`уведомление` с аргументами), `:176` (`уведомление` без аргументов `[уведомление] дежурный\n`), `:185`/`:194` (формы `вызов`); `main_test.go:117/200/235` (CLI golden). Это ≥6 пинов §EN-7 (раздел `docs/engine-model.md`, exact-match канон).

**Вывод**: дефолт-драйвер `printCaller` — МЕХАНИЧЕСКИЙ перенос этих строк в тип, реализующий `ExternalCaller`. Методы движка делегируют `e.caller`. Под дефолтом наблюдаемый вывод идентичен v1 → пины зелёные без правки ожидаемого текста (ГЛАВНЫЙ инвариант B2, FR-005/SC-001).

## R-3. Реальный драйвер `webhookCaller` — POST JSON через `net/http`/`httptest`

**Решение (D-AU-2 / §AU-4.3)**: `webhookCaller{ baseURL string, httpClient *http.Client }` делает `POST <baseURL>` с `Content-Type: application/json` и телом `{"цель": <target>, "данные": [<args>]}`. Детерминизм тестов — `net/http/httptest` (stdlib, без живой сети, без новой зависимости).

**Контракт ответа**:
- `Call` декодирует тело ответа через `decodeValue` (одно значение любого типа): объект → `Запись`, скаляр → `Value`. ПУСТОЕ тело проверяется ДО `decodeValue` (тот на пустом потоке вернёт `io.EOF`, `events.go:149`-семантика) → `Пусто` (`value.None`).
- `Notify` ответ игнорирует (best-effort).
- Сетевой/HTTP-сбой → `error` (eval заворачивает в `ОшибкаВыполнения`, R-5). Тайм-аут `httpClient` — конечный (напр. 10с); конфигурируемость отложена в M3.

**Вывод**: реальный драйвер изолирован за интерфейсом `ExternalCaller`; единственный потребитель сети; тестируется `httptest`-сервером, фиксирующим полученные `POST` (URL, заголовок, тело).

## R-4. Кодек `internal/jsonval` — лифт декодера + НОВЫЙ энкодер

**Почему B2 СОЗДАЁТ пакет (а не вызывает декодер на месте, §AU-4 / §AU-5.2)**: `engine` функционально нуждается И в `DecodeValue` (декод ответа вебхука, §AU-4.3), И в энкодере (тело POST). Декодер сейчас живёт в `daemon`, но `engine` НЕ может импортировать `daemon`: ребро `daemon→engine` уже существует (`src/internal/daemon/daemon.go:19`, `restart.go:6`; подтверждено `go list -deps ./internal/daemon | grep internal/engine`), обратный импорт дал бы циклическую зависимость. Поэтому декодер ОБЯЗАН переехать в НЕЙТРАЛЬНЫЙ `internal/jsonval` (импортирует только `value`+stdlib), доступный обоим. В последовательном поезде B2 идёт ПЕРЕД B3, и кодек нужен уже B2 → **пакет создаёт B2 (014)**; B3 (015) его НЕ лифтит повторно, только потребляет (`PayloadToRecord` для `--данные`, §AU-5.3).

**Декодер (лифт как есть, §AU-5.2)**: `src/internal/daemon/events.go` содержит пакетные функции без зависимости на `*Daemon`: `payloadToRecord` (`:95`), `decodeObject` (`:120`), `decodeValue` (`:148`), `decodeArray` (`:177`), `numberToValue` (`:195`). Импортируют только `bytes/encoding/json/fmt/strings` + `value` → лифтятся в НОВЫЙ нейтральный пакет `internal/jsonval` чисто. Потребители: B2 (ответ вебхука через `decodeValue`), B3 (payload через `payloadToRecord`), события 007b (`daemon` теперь импортирует `jsonval`). Перенос теста `TestPayloadToRecordValueTypes` (`daemon/events_test.go:174`) обязателен (звал неэкспортированную функцию) — co-land.

**Энкодер (НОВЫЙ, value → plain-JSON, §AU-4.3)**: существующий `store/codec.encodeValue` (`src/internal/store/codec.go:78`) даёт ТЕГИРОВАННЫЙ `{"т":"<Type>","зн":<payload>}` (`:17`, `:105-114`) — НЕ годится для внешней системы; `value.String` — дисплей-репр, не валидный JSON. Парный энкодер пишется с нуля: `Целое/Дробное`→число, `Строка`→quoted, `Булево`→`true`/`false`, `Пусто`→`null`, `Список`→array, `Запись`→object; `Дата/Длительность/Период` → строковая форма.

**Развилка (разрешена явно)**: точная строковая форма `Дата`/`Длительность`/`Период` в энкодере — решение impl (напр. ISO-дата, `value.String`-форма длительности `3дн`), **задокументировать в data-model/коде**. Не молчаливое — §AU-4.3 прямо делегирует выбор impl. Тегированный кодек `store` НЕ переиспользуется (несовместим). НЕ «один декодер на всё»: второй JSON→value декодер в `eval/source_loader.go` (источники M1) НЕ сливается (другая семантика overflow/дат).

**Вывод**: `internal/jsonval` импортирует только `value`+stdlib (листовой-совместим, Принцип VII); импортёр — `engine` (декод ответа + энкод тела) и `daemon` (события). НЕ импортируется из `eval` (иначе утечка JSON во фронтенд, нарушение хартии §5) и НЕ кладётся в `internal/value`.

## R-5. Активация `runtimeErrWrap` (закрытие TODO D-14, §AU-4.4)

**Эмпирика (живые точки)**:
- `runtimeErrWrap` существует: `src/internal/eval/interpreter.go:189` — `func runtimeErrWrap(p ast.Position, cause error) error` (несёт цепочку `Cause` для `errors.As/Is`). `runtimeErr(p, msg)` (`:180`) — без причины.
- `вызвать`-выражение (B1) уже использует `runtimeErrWrap`: `eval/stmt.go:97` — `evalRunProcess`/expr-путь обёрнут (`evalExpr(*CallExternalExpr)` зеркалит).
- statement-точки ВСЁ ЕЩЁ на голом `runtimeErr`: `evalCallAction` (`stmt.go:105`) — `:118` `return Signal{}, runtimeErr(c.Pos(), err.Error())`, TODO-комментарий `:113-115` («стаб всегда nil — ветка мёртвая; при активации заменить на `runtimeErrWrap`»); `evalNotifyAction` (`stmt.go:125`) — `:138` `runtimeErr(n.Pos(), err.Error())`, TODO `:133-135`.

**Решение (§AU-4.4)**: заменить `runtimeErr(...err.Error())` → `runtimeErrWrap(<pos>, err)` на `:118` и `:138`; удалить TODO-комментарии `:113-115`/`:133-135`. Теперь сбой реального драйвера несёт `Cause`. Все три точки (`вызвать`-выражение + 2 statement) → `errors.ОшибкаВыполнения`, ЕДИНАЯ категория (не «категория Процесса»). Под дефолт-стабом ветка по-прежнему недостижима (стаб → nil) — поведение v1 не меняется.

**Вывод**: правка чисто на стороне `eval` (2 строки + 2 удалённых комментария), активируется реальным драйвером B2. Пустой дифф сигнатур; обёртка делает рантайм-ошибку диагностируемой через `errors.As`.

## R-6. CLI-проводка `--вебхук` / `LADIX_WEBHOOK` (§AU-4.5 / §AU-9)

**Эмпирика (команды и их Store, §AU-9)**:
- `run` (`cmd/ladix/main.go`, дефолт MemoryStore) — несёт `--вебхук`, `--max-depth`.
- `serve` (`cmd/ladix/serve.go`, демон, дефолт MemoryStore) — `--вебхук`, `--interval`, `--max-depth`. **КРИТИЧНО**: `--вебхук` → ТОТ ЖЕ движок, чьи `Notify`/`Call` зовёт догон дедлайнов (`checkDeadlines → fireDeadlineBody`) и тело триггеров. Иначе эскалация печатает стаб (тихий разрыв §AU-12.C).
- `complete` (`main.go`, дефолт SQLite) — несёт `--данные` (B3), `--вебхук` (эффекты на догоне).
- `start` (B5, дефолт SQLite) — `--вебхук`, `--max-depth`. Если на момент impl `start` ещё не поставлена сопутствующей подфичей — проводка `--вебхук` в неё co-land (зависимость, не дублировать команду).
- `metric`/`tasks`/`emit` — БЕЗ `--вебхук`.

**Решение**: общий CLI-хелпер (напр. `openExternalCaller(webhookFlag, env) (engine.ExternalCaller, error)`): читает `--вебхук`, иначе env `LADIX_WEBHOOK`; пусто → `nil` (движок берёт дефолт-стаб); задан → валидирует URL (`net/url.ParseRequestURI` или эквивалент), невалидный → CLI-ошибка; валидный → `webhookCaller{baseURL, &http.Client{Timeout:…}}`. Команда передаёт результат в `NewEngine(..., WithExternalCaller(c))` ТОЛЬКО если `c != nil`.

**CLI-ошибка (ДОСЛОВНО, §AU-10.C / §EN-8.B)**: `ladix: неверный URL вебхука '<URL>'` — stderr, exit 2. Идёт штатным CLI-каналом ошибок (как `ladix: неверный JSON в --данные: …`), без позиции (корень композиции, до парсинга программы).

**Вывод**: проводка — в корне композиции CLI (env читается тут, передаётся параметром — Принцип V). Валидация URL до запуска движка; невалидный URL → движок не строится, stdout пуст.

## R-7. Шов `ProcessRuntime` НЕ расширяется; eval-граница цела

**Эмпирика**: B1 (фича 013, влита `38e1c78`) уже дал 8-й метод `CallExternalResult` (`eval/runtime.go`, шов 7→8 закрыт). `engine/runtime.go:43` его реализует. B2 — внутри `engine` (драйвер за `e.caller`) + `cmd/ladix` (проводка) + `eval/stmt.go` (2 точки `runtimeErrWrap`, БЕЗ изменения сигнатур шва).

**Вывод**: `ProcessRuntime` остаётся 8 методов (дрейф-аудит §AU-2). `eval` НЕ импортирует `store`/`engine`/`jsonval` (FR-018) — статически проверяемо. Ребро `engine→eval` однонаправленно. Пустой дифф `internal/store` (Store-схема не трогается), `internal/lexer`/`parser`/`ast` (грамматика неизменна).

## Сводка решений (все залочены §AU-4 / D-AU-2)

| # | Решение | Источник |
|---|---|---|
| R-1 | Драйвер через Option (клон `WithClock`); поле `e.caller`, дефолт `printCaller` | §AU-4.1 |
| R-2 | `printCaller` = перенос печать-логики `runtime.go:42-73` байт-в-байт (§EN-7 golden) | §AU-4.2 |
| R-3 | `webhookCaller` POST JSON `{"цель","данные"}`; ответ через `decodeValue`; пустое тело→Пусто; httptest | §AU-4.3 |
| R-4 | `internal/jsonval`: лифт декодера + НОВЫЙ нетегированный энкодер; дата/длит→строка (impl, задокумент.) | §AU-4.3/§AU-5.2 |
| R-5 | Активация `runtimeErrWrap` на `eval/stmt.go:118`/`:138`; единая `ОшибкаВыполнения` | §AU-4.4 |
| R-6 | CLI `--вебхук`/`LADIX_WEBHOOK` в run/serve/complete/start; serve = единый движок; ошибка URL дословно | §AU-4.5/§AU-9/§AU-10.C |
| R-7 | Шов остаётся 8; eval без store/engine; пустой дифф store/lexer/parser/ast | §AU-2 |
