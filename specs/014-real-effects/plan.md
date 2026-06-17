# Implementation Plan: B2 «Реальные эффекты `вызвать` / `уведомить` через HTTP-вебхук»

**Branch**: `014-real-effects` | **Date**: 2026-06-17 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/014-real-effects/spec.md`

## Summary

B2 (веха M2, `docs/automation-model.md` §AU-4, решение **D-AU-2**) включает РЕАЛЬНУЮ доставку внешних эффектов `вызвать` / `уведомить` через HTTP-вебхук, активируемую Option'ом движка, **сохраняя дефолт = печать-стаб** (держит §EN-7 golden байт-точно). B1 (фича 013, смержена в HEAD `38e1c78`) уже дал метод шва `CallExternalResult` (шов 7→8 закрыт), поэтому B2 целиком — ВНУТРИ `engine` + проводка CLI; шов не расширяется. Технический подход зеркалит существующий прецедент `WithClock` (Option-инжект драйвера):

1. **Интерфейс драйвера** — `ExternalCaller{ Call(target, args) (value.Value, error); Notify(target, args) error }` (новый тип в `engine`). Движок получает поле `caller ExternalCaller`, инжектируемое Option'ом `WithExternalCaller(c ExternalCaller) Option` (`engine/clock.go` — рядом с `Option`/`WithClock:19-24`). Без Option → `printCaller{out: e.out}` [FR-001/002].
2. **Дефолт-драйвер `printCaller`** — печать-стаб §AU-4.2: текущая печать-логика `engine/runtime.go:42-73` ПЕРЕНОСИТСЯ в тип `printCaller` байт-в-байт (форматы `[вызов] %s(%s)\n`, `[уведомление] %s\n`, `[уведомление] %s: %s\n`). `Call` → `(value.None, nil)`. Методы движка `CallExternalResult`/`CallExternal`/`Notify` делегируют `e.caller`; `CallExternal` = `CallExternalResult` с отбросом значения [FR-003/004/005].
3. **Реальный драйвер `webhookCaller{ baseURL, httpClient }`** — `POST <baseURL>` с `Content-Type: application/json` и телом `{"цель": target, "данные": [args]}`; ответ `Call` декодируется через `decodeValue` (пустое тело → `Пусто` ДО декодера); `Notify` best-effort [FR-006/008/009].
4. **Кодек `internal/jsonval`** (НОВЫЙ нейтральный пакет, СОЗДАЁТ B2) — лифт декодера `decodeValue`/`payloadToRecord` из `daemon/events.go` (без зависимости на `*Daemon`) + НОВЫЙ энкодер `value → plain-JSON` (нетегированный, в отличие от `store/codec.encodeValue`). Импортёр — `engine`; НЕ `eval`, НЕ `internal/value` [FR-007/010]. **Обоснование лифта (а не вызова на месте):** `engine` нуждается в `DecodeValue` для ответа вебхука (§AU-4.3) + в энкодере для тела, но `engine` НЕ может импортировать `daemon` — ребро `daemon→engine` уже существует (`daemon/daemon.go:19`, `daemon/restart.go:6`), обратный импорт дал бы циклическую зависимость. Поэтому декодер ОБЯЗАН переехать в нейтральный `internal/jsonval` (импортирует только `value`+stdlib), доступный обоим (§AU-4 / §AU-5.2). **Межфичевая граница:** пакет создаёт B2 (014); B3 (015) его НЕ создаёт и НЕ лифтит повторно — только потребляет (`PayloadToRecord` для `--данные`→`Запись`, §AU-5.3).
5. **Активация `runtimeErrWrap` (TODO D-14)** — на двух statement-точках `eval/stmt.go:118` (`evalCallAction`) и `:138` (`evalNotifyAction`): замена `runtimeErr(...)` → `runtimeErrWrap(c.Pos(), err)`, удаление TODO-комментариев `:113-115`/`:133-136`. `вызвать`-выражение уже использует `runtimeErrWrap` с B1. Все три точки → `errors.ОшибкаВыполнения` с цепочкой `Cause` [FR-012/013].
6. **CLI-проводка** — `run`/`serve`/`complete`/`start` принимают `--вебхук <URL>` или env `LADIX_WEBHOOK`; валидный URL → `WithExternalCaller(webhookCaller{...})`, иначе дефолт-стаб; невалидный URL → `ladix: неверный URL вебхука '<URL>'` stderr exit 2. Под `serve` вебхук — в ТОТ ЖЕ экземпляр движка (догон дедлайнов + тело триггеров) [FR-015/016/017].

`ProcessRuntime` остаётся 8 методов (B1 уже дал `CallExternalResult`). 0 новых зависимостей (`net/http`/`net/http/httptest` — stdlib). Дефолт без `--вебхук` — наблюдаемый вывод §EN-7 байт-в-байт (ГЛАВНЫЙ инвариант B2).

## Technical Context

**Language/Version**: Go 1.25 (`src/go.mod`, module `github.com/denis-kosyakov/ladix`). Идиоматичный Go: `gofmt`, `go vet ./...` без замечаний (Принцип I).

**Primary Dependencies**: 0 новых. Единственная внешняя — `modernc.org/sqlite` (B2 её не затрагивает). HTTP-драйвер и тесты — на stdlib `net/http` + `net/http/httptest`; кодек тела/ответа — `encoding/json` (stdlib).

**Storage**: N/A для логики B2. Store-схема, SQLite-кодек, durable-поля НЕ трогаются — пустой дифф `src/internal/store`. CLI-проводка `complete`/`start` касается их только добавлением флага `--вебхук` (Store-конструкция — забота сопутствующих B5/B6/B-DB, §AU-9).

**Testing**: `cd src && go test ./... -count=1` (+`-race` на критичных). Tests-first для драйвера/проводки (Принцип VI совместим — драйвер не lexer/parser, но дисциплина та же): юнит-тесты `printCaller` (форматы байт-точно) и `webhookCaller` (через `httptest`), контракт `ExternalCaller`/Option, golden-инвариант §EN-7 (дефолт=стаб), CLI-ошибка URL. Инверсные мутпробы на каждый замок (см. tasks.md). Реальный POST — только под `net/http/httptest`, без живой сети.

**Target Platform**: один статический бинарник `ladix` (кросс-платформенный, без CGO).

**Project Type**: интерпретатор DSL (compiler/CLI), ручной recursive-descent (Принцип II). B2 — engine/CLI-слой, грамматику не трогает.

**Performance Goals**: N/A. Дефолт-путь добавляет один уровень делегирования (`e.caller`), наблюдаемо нулевой. Реальный путь делает один `POST` на вызов с конечным тайм-аутом клиента.

**Constraints**: §EN-7 golden байт-в-байт под дефолт-драйвером (≥6 пинов: `engine_test 108/167/176/185/194`, `main_test 117/200/235`) — ГЛАВНЫЙ инвариант (FR-005); шов `ProcessRuntime` остаётся 8 методов (НЕ расширяется, B1 дал 8-й), `eval` без импорта `store`/`engine`; пустой дифф lexer/parser/ast/store-схемы; 0 новых зависимостей; реальный HTTP только под `httptest`; детерминизм; тексты CLI-ошибки/стаба — дословно §AU-10.C/§AU-4.2; позиции в рунах с 1 (Принцип IV).

**Scale/Scope**: 1 новый интерфейс + Option (`engine`); 2 драйвера (`printCaller` — перенос текущей логики; `webhookCaller` — новый); 1 новый пакет `internal/jsonval` (лифт декодера + новый энкодер); делегирование 3 методов движка драйверу; активация `runtimeErrWrap` на 2 statement-точках eval; CLI-проводка `--вебхук`/env + валидация URL в 4 командах (`run`/`serve`/`complete`/`start`). Тесты: `printCaller` форматы, `webhookCaller` POST/декод/пустое-тело/сбой через `httptest`, контракт Option, golden §EN-7 дефолт, CLI-ошибка URL, кодек типов.

## Constitution Check

*GATE: проверено до Phase 0 и повторно после Phase 1. Итог: **9/9 PASS**, 0 нарушений, Complexity Tracking пуст.*

- **I. Язык и сборка** — PASS. Go 1.25, `gofmt`/`go vet` чисто; 0 новых зависимостей (`net/http`/`httptest`/`encoding/json` — stdlib); CGO не вводится.
- **II. Парсинг — ручной** — PASS. B2 НЕ трогает lexer/parser (engine/CLI-слой); генераторы/regex не вводятся; грамматика и AST неизменны (FR-020).
- **III. Ошибки — явные типы** — PASS. Сбой драйвера → `error` → eval заворачивает существующим `runtimeErrWrap` → `errors.ОшибкаВыполнения` с цепочкой `Cause` (`errors.As/Is`); новая CLI-ошибка `неверный URL вебхука` идёт штатным CLI-каналом (stderr, exit 2, §EN-8.B), без новых Go-типов ошибок; паник в штатных путях нет; recover-барьер CLI цел.
- **IV. Позиции — сквозные** — PASS. `runtimeErrWrap(c.Pos(), err)` на statement-точках протаскивает позицию узла `вызвать`/`уведомить` (рантайм-ошибка несёт строку/колонку в рунах). CLI-ошибка URL — корень композиции (до парсинга программы), позиция неприменима (как прочие CLI-ошибки §AU-10.C).
- **V. Без глобального состояния** — PASS. Драйвер инжектируется Option'ом в инстанс движка (DI, паттерн `WithClock`); `httpClient` — поле `webhookCaller`, не пакет-глобал; env `LADIX_WEBHOOK` читается в корне композиции CLI и передаётся параметром, не глобалом. Пакет-уровневого изменяемого состояния не вводится.
- **VI. Тесты — вперёд** — PASS. Юнит-замки `printCaller`/`webhookCaller`, контракт Option, golden §EN-7 дефолт, CLI-ошибка URL, кодек типов — пишутся tests-first; каждый замок снабжён инверсной мутпробой (red→green), см. tasks.md. Особо: golden-дефолт (мутация «дефолт=webhookCaller»→golden краснеет), httptest-POST (мутация «не слать/слать не туда»), CLI-ошибка URL (дословный текст).
- **VII. Раскладка проекта** — PASS. Правки в `internal/engine` (+тесты), НОВЫЙ `internal/jsonval` (нейтральный, лифт из `daemon`), `internal/eval` (2 statement-точки `runtimeErrWrap`), `cmd/ladix` (проводка). Граф без циклов: `jsonval` импортирует только `value`+stdlib (листовой-совместим), `engine`→`jsonval`, `eval` НЕ импортирует `store`/`engine`/`jsonval` (FR-018). Листовость `ast`/`value`/`errors` цела.
- **VIII. Язык сообщений** — PASS. Единственное новое пользовательское сообщение — CLI-ошибка `ladix: неверный URL вебхука '<URL>'` — взято ДОСЛОВНО из §AU-10.C (не переформулировано). Печать-стаб `вызвать`/`уведомить` — байт-в-байт §AU-4.2/§EN-7 (русский канон цел). Тело вебхука (`{"цель","данные"}`) — машинный JSON, не пользовательское сообщение.
- **IX. Спека — источник истины** — PASS. Поведение фиксируется размещёнными доками: spec.md + `docs/automation-model.md` §AU-4 (D-AU-2, locked владельцем) + §AU-10.C (CLI-ошибка) + §AU-4.2/§EN-7 (форматы стаба). Развилки (форма даты/длительности в энкодере; конечный тайм-аут) разрешены явно в spec/research как «решение impl, задокументировать», не молча. Синк больших доков делегирован архитектору на M2-гейте письменно.

**Структурные инварианты якоря M2 §AU-2** (дрейф-аудит на гейте): `ProcessRuntime` остаётся 8 (B2 шов НЕ расширяет — B1 уже дал `CallExternalResult`, 8 сигнатур байт-в-байт); `Store` остаётся 15/16 (B2 его не касается, пустой дифф `src/internal/store`); ребро `engine→eval` однонаправленно; golden §EN-7 байт-в-байт под дефолтом (SC-001). B2 — аддитивный engine/CLI-слой, санкционированных исключений нет.

## Project Structure

### Documentation (this feature)

```text
specs/014-real-effects/
├── spec.md              # ✅ написана
├── plan.md              # ← этот файл
├── research.md          # Phase 0 (прецедент WithClock, лифт jsonval, нетегированный энкодер, httptest, форма даты)
├── data-model.md        # Phase 1 (ExternalCaller, Option, printCaller/webhookCaller, jsonval кодек)
├── quickstart.md        # Phase 1 (как поднять реальный драйвер; контрольный httptest-сниппет; дефолт-стаб)
├── contracts/           # Phase 1 (external-caller.md драйвер+Option, webhook-wire.md тело/ответ, cli-webhook.md проводка+ошибка, golden-en7.md инвариант)
└── tasks.md             # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
src/internal/engine/
│   ├── caller.go (НОВЫЙ)        # ← интерфейс ExternalCaller; printCaller (перенос печать-логики runtime.go:42-73);
│   │                            #    webhookCaller{baseURL, httpClient}: POST {"цель","данные"}, декод ответа, пустое тело→Пусто [FR-001/004/006/008/009]
│   ├── clock.go                 # ← +WithExternalCaller Option (рядом с Option/WithClock:19-24) [FR-002]
│   ├── engine.go                # ← поле caller ExternalCaller; дефолт printCaller{out} в NewEngine [FR-002]
│   ├── runtime.go               # ← CallExternalResult/CallExternal/Notify делегируют e.caller; CallExternal отбрасывает значение [FR-003]
│   ├── caller_test.go (НОВЫЙ)   # ← printCaller форматы байт-точно; webhookCaller POST/декод/пустое тело/сбой через httptest [SC-002/003/004]
│   └── engine_test.go           # ← §EN-7 пины 108/167/176/185/194 ЦЕЛЫ под дефолтом (без правки текста) [FR-005/SC-001]
src/internal/jsonval/ (НОВЫЙ пакет)
│   ├── decode.go                # ← лифт decodeValue/payloadToRecord/decodeObject/decodeArray/numberToValue из daemon/events.go:95-206 [FR-010]
│   ├── encode.go (НОВЫЙ)        # ← энкодер value→plain-JSON (нетегированный; дата/длит/период→строка) [FR-007]
│   ├── decode_test.go           # ← перенос TestPayloadToRecordValueTypes (был daemon/events_test.go:174)
│   └── encode_test.go (НОВЫЙ)   # ← каждый тип value → ожидаемый plain-JSON; БЕЗ обёртки {"т","зн"} [SC-002]
src/internal/daemon/
│   └── events.go                # ← делегирует jsonval (декодер лифтнут; импорт jsonval) — co-land, не «без изменений»
src/internal/eval/
│   └── stmt.go                  # ← :118 evalCallAction runtimeErr→runtimeErrWrap; :138 evalNotifyAction то же; удалить TODO :113-115/:133-135 [FR-013]
src/cmd/ladix/
│   ├── main.go                  # ← run/complete: флаг --вебхук + env LADIX_WEBHOOK; валидация URL→ошибка; openExternalCaller хелпер [FR-015/016]
│   ├── serve.go                 # ← serve: --вебхук в ТОТ ЖЕ движок (догон дедлайнов + триггеры) [FR-017]
│   ├── main_test.go             # ← §EN-7 пины 117/200/235 ЦЕЛЫ; CLI-ошибка URL exit 2 stderr [SC-001/006]
│   └── webhook_cli_test.go (НОВЫЙ) # ← run/serve/complete/start с --вебхук→httptest получает POST; невалидный URL→ошибка [SC-005/006]
```

**Structure Decision**: одна фича, четыре US (US1 P1 дефолт-стаб golden — несущий; US2/US3/US4 P2 реальный драйвер/ошибки/CLI). Слой — `engine` (интерфейс+Option+2 драйвера) + НОВЫЙ нейтральный `internal/jsonval` (лифт декодера + новый энкодер, импортёр `engine`/`daemon`) + 2 statement-точки `eval/stmt.go` (активация `runtimeErrWrap`) + проводка `cmd/ladix`. ПУСТОЙ дифф `src/internal/store` (Store-схема не трогается), `src/internal/lexer`/`parser`/`ast` (грамматика неизменна). Шов `ProcessRuntime` остаётся 8 методов. Большие доки (`SPEC.md`, `docs/grammar.md`, `docs/automation-model.md`, `docs/engine-model.md`, `README.md`) — зона архитектора на M2-гейте, не правятся. `examples/` и корневой quickstart — забота L1-Реализация/витрина, вне scope.

> **Прим. о `start`/`complete`-Store и `openStore` (§AU-9):** дефолт-Store и хелпер `openStore` поставляют сопутствующие подфичи B5/B6/B-DB. B2 трогает CLI этих команд ТОЛЬКО в части `--вебхук`/env-проводки; если на момент impl `start` ещё не существует, проводка в него — co-land с той подфичей (зафиксировать в tasks как зависимость, не дублировать команду).

## Complexity Tracking

> Заполняется ТОЛЬКО при нарушениях Constitution Check.

Нарушений нет — таблица пуста. B2 — аддитивный engine/CLI-слой: Option-инжект драйвера (прецедент `WithClock`), дефолт = перенос текущей печать-логики (golden §EN-7 байт-в-байт), реальный драйвер изолирован за интерфейсом, новый нейтральный пакет `internal/jsonval` (лифт + новый энкодер) без циклов. Шов `ProcessRuntime` НЕ расширяется (остаётся 8, B1 дал 8-й). Горячий инвариант panic-mode парсера НЕ модифицируется (B2 парсер не трогает). 0 новых зависимостей; реальный HTTP — только под `httptest`.
