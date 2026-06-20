# Implementation Plan: Входящие события (HTTP-приём)

**Branch**: `025-inbound-events` | **Date**: 2026-06-20 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/025-inbound-events/spec.md`

**Anchor**: `docs/inbound-events-model.md` §IE-0..§IE-8 (решения D-IE-1..D-IE-10 залочены; источник истины для всех «как»)

## Summary

Добавить **входящего сетевого продюсера** событий внутри уже существующего демона `serve`: opt-in флаг `--listen host:port` поднимает HTTP-эндпоинт `POST /events/{имя}`, который декодирует имя из пути, читает сырое тело и кладёт событие в durable-очередь `events` через **общий хелпер минта**. На ближайшем тике существующий `drainEvents` матчит триггеры `когда событие E { … }` и исполняет их тела — путь обработки не меняется. Технический подход: тонкий stdlib-`net/http`-хендлер, касающийся **только** `store.Store`+`engine.Clock` (движок/интерпретатор не потокобезопасны → не трогаем из второй горутины); жизненный цикл сервера — `sync.WaitGroup`+`srv.Shutdown` (без `errgroup`), завершение **строго до** `Store.Close`; опциональный `--token` (constant-time `crypto/subtle`). Нулевой регресс без флага.

## Technical Context

**Language/Version**: Go 1.25 (`src/go.mod`)

**Primary Dependencies**: только stdlib для этой фичи — `net/http`, `net/http/httptest` (тесты), `net/url`, `sync`, `crypto/subtle`, `context`. Единственная прямая зависимость репозитория остаётся `modernc.org/sqlite` (замок: `grep require src/go.mod` неизменён). **errgroup (`golang.org/x/sync`) и `golang.org/x/text` (norm) запрещены.**

**Storage**: SQLite (`internal/store.SQLiteStore`, `MaxOpenConns=1` сериализует конкурентный доступ хендлер∥тик); durable-очередь `events` переиспользуется как есть (контракт `Store` не расширяется).

**Testing**: `go test ./...` (table-driven + httptest); детерминизм через инъекцию `engine.Clock` (`fixedClock`, `serve_golden_test.go:21-23`).

**Target Platform**: один статический бинарник CLI (`cmd/ladix`), демон `serve` живёт в одном процессе ОС.

**Project Type**: компилятор/интерпретатор языка + CLI (single project).

**Performance Goals**: не цель этой итерации (приём — enqueue-only, тело асинхронно на тике; задержка до `--interval`). Лимит размера тела, push-пробуждение — вне scope.

**Constraints**: stdlib-only; хендлер изолирован от движка (FR-IE-2); shutdown+join до `Store.Close` (FR-IE-6); детерминизм времени через инъектируемые часы (FR-IE-11).

**Scale/Scope**: микро-фича. Прод-дифф сконцентрирован в `src/cmd/ladix/` (рефактор `emit.go`, расширение `serve.go`, новый файл хендлера). Контракт `Store` (18 методов), `ProcessRuntime` и любой код в `internal/{store,engine,daemon,eval}` — **не трогаются** (кроме нуля). Замки-инварианты: Store=18, 0 новых зависимостей, нулевой регресс serve без `--listen`.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| # | Принцип | Статус | Обоснование |
|---|---------|--------|-------------|
| I | Язык и сборка (Go, gofmt+vet, CGO off, 1 зависимость) | ✅ PASS | stdlib-only; новых зависимостей 0 (`net/http` входит в stdlib); `modernc.org/sqlite` единственная прямая. |
| II | Парсинг — ручной | ✅ PASS | Фронтенд не трогаем; грамматика `когда событие` без изменений. HTTP-парсинг — stdlib `net/http`, не парсер языка. |
| III | Ошибки — явные типы + recover-барьер | ✅ PASS | CLI-пути под `guard` (recover). HTTP-хендлер: ошибки → коды/тексты, не паника; `net/http` сам ловит панику в хендлере (не роняет процесс). `net.Listen` вне guard → exit 2 (категория окружения, не «внутренняя ошибка»). |
| IV | Позиции — сквозные | ✅ PASS | Не применимо: фича не порождает диагностик с позициями исходника. |
| V | Без глобального состояния | ✅ PASS | Хендлер — замыкание/значение с инъекцией `Store`+`Clock`+токена; нет package-level mutable state. |
| VI | Тесты — вперёд | ✅ PASS | Замки FR-IE-1..10 (httptest, симметрично `webhook_cli_test.go`) пишутся как часть задач; включая негативные кейсы (400/401/405/500). |
| VII | Раскладка проекта | ✅ PASS | Дифф в `cmd/ladix/` (+1 файл хендлера); граф зависимостей без новых рёбер (cmd→store, cmd→engine уже есть). |
| VIII | Язык сообщений — русский, дословно | ✅ PASS | Все RU-тексты ответов дословно из §IE-2 таблицы; ack «принято» намеренно ≠ «поставлено в очередь» (D-IE-8). Двухстрочный канон §13 не применим (HTTP-тела одностроч, не диагностики исходника — запись ниже). |
| IX | Спека — источник истины | ✅ PASS | Всё выведено из `docs/inbound-events-model.md` (анкор) + spec.md; пробелов нет (0 NEEDS CLARIFICATION). |

**Complexity Tracking note (Принцип VIII)**: HTTP-тела ответов (`событие … принято`, `ladix: неверный токен`, …) — **однострочные** и НЕ следуют двухстрочному канону «Ошибка в строке N, колонка M:» (§13). Это не нарушение: канон §13 описывает диагностики ошибок ИСХОДНИКА с позицией; ответы сетевого протокола — иная категория (нет позиции в `.ladix`). Тексты по-русски, дословно из якоря. Запись для прозрачности, гейт VIII = PASS.

**Итог гейта: Constitution 9/9 PASS** (нарушений нет; Complexity Tracking пуст, кроме пояснительной записки выше).

## Project Structure

### Documentation (this feature)

```text
specs/025-inbound-events/
├── plan.md              # Этот файл
├── research.md          # Phase 0: TODO-FACT якоря + выбор stdlib-механизмов
├── data-model.md        # Phase 1: Event (переиспользуется) + конфиг приёмника + сигнатуры
├── quickstart.md        # Phase 1: ручной прогон curl→202→тик→триггер
├── contracts/
│   ├── http-endpoint.md     # POST /events/{имя}: коды/тексты/декод имени/auth
│   ├── cli-flags.md         # --listen/--token парсинг, --db-граница, bind-ошибка, loopback-warn
│   └── enqueue-helper.md    # рефактор D-IE-8: enqueueEvent(Store,name,payload,Clock)(id,err)
└── tasks.md             # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
src/
├── cmd/ladix/
│   ├── emit.go              # ПРАВКА: extract enqueueEvent из emitEvent (D-IE-8), поведение emit неизменно
│   ├── serve.go             # ПРАВКА: парсинг --listen/--token; --db-граница; net.Listen вне guard;
│   │                        #         lifecycle (WaitGroup+Shutdown) в guard-замыкании ДО Store.Close
│   ├── events_http.go       # НОВЫЙ: eventsHandler(store.Store, engine.Clock, token) http.Handler
│   ├── events_http_test.go  # НОВЫЙ: замки FR-IE-1..10 (httptest)
│   ├── serve.go (golden)    # существующие serve_golden/daemon goroutine-leak замки — БАРЬЕР (не править суть)
│   └── main.go              # НЕ трогаем (usage уже описывает serve; диспетчер цел)
└── internal/{store,engine,daemon,eval}/   # ПУСТОЙ дифф (контракт Store=18 цел, drainEvents цел)
```

**Structure Decision**: Single project. Вся новая логика — в пакете `main` (`cmd/ladix`): рефактор минта в `emit.go`, расширение `serve.go`, новый файл хендлера `events_http.go` + тесты `events_http_test.go`. `internal/*` остаётся с нулевым диффом — это инвариант (контракт очереди `Store` и фаза `drainEvents` переиспользуются). Новый файл хендлера держит хендлер изолированным и тестируемым (FR-IE-2: сигнатура принимает `store.Store`+`engine.Clock`, статический замок изоляции от движка).

## Complexity Tracking

> Нарушений конституции нет. Единственная запись — пояснительная (НЕ violation):

| Запись | Что | Почему не нарушение |
|--------|-----|---------------------|
| HTTP-тела одностроч (не §13) | Ответы протокола `событие … принято` / `ladix: …` — одна строка | §13 (двухстрочный канон) описывает диагностики ИСХОДНИКА с позицией; сетевой ответ — иная категория без позиции в `.ladix`. Тексты по-русски, дословно из якоря §IE-2. |
