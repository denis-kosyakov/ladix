# Contract — Команда `ladix emit` (007b)

**Anchor**: EM-1 (emit→SQLiteStore), EM-17.3 (очередь `events`, `EnqueueEvent`), FR-015, SC-006.
Файл `src/cmd/ladix/emit.go`, диспетчер `main.go`.

## Сигнатура CLI

```
ladix emit <событие> [json] [--db путь]
```
- `<событие>` — обязательное имя события (матчится с `EventTrigger.Event.Name`, FR-016).
- `[json]` — необязательный сырой JSON-payload (по умолчанию пусто/`{}` — импл-факт, фиксируется кодом).
- `--db путь` — общий SQLite-файл (как `complete`/`serve`); дефолт `ladix.db` (defaultDBPath, main.go:60).
  Без durable-Store очередь эфемерна и невидима демону — реальный `emit` требует общего `--db`.

Диспетчер `realMain`: `case "emit": return emitMain(args[1:], stdout, stderr)`. `usage` дополняется.

## Жизненный цикл `emitMain` (короткоживущий, как `complete`)

```
1. Разбор: <событие> обязателен; [json] опционален; --db. Нет имени → stderr usage, exit 2.
2. Открыть Store (SQLite под --db; WAL+busy_timeout сериализует запись с serve, EM-17.7).
   Сбой открытия → "не удалось открыть хранилище", exit 2 (зеркало §EN-8.B).
3. guard(stderr, func() int {
     id, err := st.NextEventID()                 // "e-NNNNNN"
     if err != nil { ... exit 2 }
     e := &store.Event{ID:id, Name:событие, PayloadJSON:json, CreatedAt:clock.Now(), Processed:false}
     if err := st.EnqueueEvent(e); err != nil { ... exit 2 }
     return 0                                     // НЕ запускает демон (FR-015)
   })
```

## Exit-коды

| Код | Случай |
|---|---|
| 0 | событие записано в очередь |
| 2 | ошибка использования (нет имени события, сбой открытия/записи Store) |

## Инварианты

- `emit` **НЕ** запускает демон (FR-015): пишет событие и выходит. Доставка — фаза `drainEvents`
  демона (FR-016/017).
- Кросс-процессная конкуренция `emit`↔`serve`↔`complete` над общим SQLite — WAL+busy_timeout
  (FR-025), не блокировки в коде Ladix.
- Валидность JSON-payload в v1 строго не проверяется командой `emit` (сырой текст пишется как есть);
  ошибки парсинга всплывают на стороне демона при маппинге `payloadJSON→Запись` (лог/skip, импл-факт).

## Тесты

- `emit заявка_создана '{"клиент":"ООО"}'` → exit 0, в `events` одна необработанная строка с этим
  payload (проверка через Store).
- два `emit` подряд → две строки, FIFO-порядок по `CreatedAt` (читается `ListUnprocessedEvents`).
- `emit` без имени → exit 2.
- интеграция с `serve` (golden): эмитировать → поднять демон → `drainEvents` привязывает
  `событие.клиент="ООО"`, исполняет тело, помечает processed (US4).
