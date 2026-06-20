# Data Model — Входящие события (HTTP-приём)

Фаза 1. Фича **не вводит новых durable-сущностей**: переиспользует `store.Event` и очередь `events`. Ниже — сущности и сигнатуры, затрагиваемые диффом.

## Сущности

### `store.Event` (переиспользуется как есть, `internal/store/types.go:75-81`)

| Поле | Тип | Источник на пути HTTP |
|------|-----|------------------------|
| `ID` | `string` (`e-NNNNNN`) | `Store.NextEventID()` (монотонный счётчик) |
| `Name` | `string` | сегмент пути после `/events/`, URL-декодирован (байт-точно) |
| `PayloadJSON` | `string` | сырое тело запроса (`io.ReadAll`), **без парсинга** |
| `CreatedAt` | `time.Time` | `clock.Now()` (часы демона) |
| `Processed` | `bool` | `false` при минте; `true` ставит `drainEvents` после тела |

Инвариант: событие из HTTP и из `emit` имеют идентичную структуру и путь обработки (FR-IE-3). Контракт `Store` (18 методов) **не расширяется**.

### Конфигурация приёмника (значения, не durable)

| Имя | Тип | Дефолт | Источник |
|-----|-----|--------|----------|
| `listen` | `string` (`host:port`) | `""` (выкл) | `--listen` |
| `token` | `string` | `""` (auth выкл) | `--token`, иначе env `LADIX_LISTEN_TOKEN` |

## Новые/изменённые функции (пакет `main`, `cmd/ladix`)

### `enqueueEvent` — НОВАЯ свободная функция (рефактор D-IE-8, `emit.go`)
```go
func enqueueEvent(st store.Store, name, payload string, clock engine.Clock) (string, error)
```
Минт: `NextEventID` → `&store.Event{...CreatedAt: clock.Now()...}` → `EnqueueEvent` → `(id, nil)`. Ack-печать НЕ входит. Зовут `emitEvent` и `eventsHandler`.

### `emitEvent` — ИЗМЕНЕНА (тело, не сигнатура; `emit.go:58-85`)
Использует `enqueueEvent(sq, name, payload, clock)` вместо инлайн-минта; печатает «поставлено в очередь» при успехе. Поведение байт-идентично прежнему (единый текст ошибки `ladix: сбой хранилища: <err>` exit 2).

### `eventsHandler` — НОВАЯ (`events_http.go`)
```go
func eventsHandler(st store.Store, clock engine.Clock, token string) http.Handler
```
Изоляция от движка (FR-IE-2): принимает **только** `store.Store` + `engine.Clock` + строку токена; НЕ принимает `*engine.Engine`/`*eval.Interpreter`. Логика — R2 (метод→auth→имя→тело→enqueue→коды).

### `startEventListener` — НОВАЯ (`serve.go` или `events_http.go`)
```go
func startEventListener(ln net.Listener, st store.Store, clock engine.Clock, token string) (stop func())
```
`http.Server{Handler: eventsHandler(...)}`; `go srv.Serve(ln)` под `sync.WaitGroup`; `stop` = `srv.Shutdown(ctx)`+`wg.Wait()`.

### `serveMain` — ИЗМЕНЕНА (`serve.go:36-122`)
+парсинг `--listen`/`--token` (формы `--x v` и `--x=v`, зеркало `--interval`); +env `LADIX_LISTEN_TOKEN` (если `--token` пуст); +проверка `--listen` без `--db` → exit 2 (R8); проброс `listen, token` в `serveFile`.

### `serveFile` — ИЗМЕНЕНА (`serve.go:131-177`)
+параметры `listen, token string`; +`net.Listen` вне guard (bind-ошибка → exit 2, R5); +loopback-warning (R7); +`defer stopListener()` внутри guard-замыкания (ДО `defer sq.Close()`).

## Состояния и переходы (HTTP-запрос → событие)

```
POST /events/{имя} ─┬─ method≠POST ───────────────→ 405 (нет события)
                    ├─ token mismatch ─────────────→ 401 (нет события)
                    ├─ имя пусто ──────────────────→ 400 (нет события)
                    ├─ enqueue err ────────────────→ 500 (событие НЕ создано)
                    └─ ok → Event{Processed:false} → 202 «принято»
                                   │
                          (следующий тик, под d.mu)
                                   ▼
                      drainEvents → eventTriggers(Name)
                          ├─ есть триггеры → fireBody(payload→Запись) → MarkEventProcessed
                          └─ нет триггеров → лог + MarkEventProcessed (молча)
```
