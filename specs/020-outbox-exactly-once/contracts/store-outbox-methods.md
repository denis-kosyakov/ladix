# Contract: Store.LoadOutbox / Store.SaveOutbox (16 → 18)

Источник: §C-2b.6. Зеркалят `LoadTriggerState`/`SaveTriggerState`.

## Сигнатуры (добавляются в interface `Store`, `internal/store/store.go:13-40`)

```go
LoadOutbox(dedupKey string) (*OutboxRecord, error) // не найдено → ErrOutboxNotFound
SaveOutbox(rec *OutboxRecord) error                // upsert по dedup_key
```

## Compile-замок (двойной, `store.go:44-45`)

```go
var (
    _ Store = (*MemoryStore)(nil)
    _ Store = (*SQLiteStore)(nil)
)
```

Обе реализации ОБЯЗАНЫ иметь оба метода; отсутствие любого ломает `go build` (это и есть замок). Базовые 16 сигнатур остаются байт-целыми.

## LoadOutbox

| Вход | Условие | Выход |
|---|---|---|
| dedupKey существует | запись найдена | `(*OutboxRecord, nil)` — глубокая копия (Memory) / десериализованная (SQLite) |
| dedupKey отсутствует | не найдено | `(nil, ErrOutboxNotFound)` |
| ошибка хранилища | sql / I/O ошибка | `(nil, <обёрнутая ошибка>)` (НЕ ErrOutboxNotFound) |

- SQLite: `SELECT … WHERE dedup_key=?`; `sql.ErrNoRows` → `ErrOutboxNotFound`. Десериализация `args_json`→`decodeList`→`[]value.Value`; `result_json`→`decodeValue`. `delivered` int→bool. `delivered_at` nullable→`*time.Time`.
- Memory: `m, ok := s.outbox[dedupKey]`; `!ok` → `ErrOutboxNotFound`. Возврат — глубокая копия (новый `[]value.Value` для Args, копия указателей `DeliveredAt`).

## SaveOutbox

| Вход | Поведение |
|---|---|
| новый dedup_key | INSERT |
| существующий dedup_key | UPDATE (upsert) |

- SQLite: `INSERT … ON CONFLICT(dedup_key) DO UPDATE SET …` (как `SaveTask` `sqlite.go:161`). Сериализация `Args`→`encodeList(value.NewList(rec.Args))`; `Result`→`encodeValue(rec.Result)`; `Delivered`→0/1; времена→RFC3339 (DeliveredAt nil→NULL колонка).
- Memory: глубокая копия `rec` в map (новый срез Args, копия указателей времён), чтобы последующая мутация значений в движке не протекла в леджер.

## Контрактные тесты (замки)

- `TestSaveLoadOutboxRoundTrip` (обе impl): Save → Load → поля совпадают (DedupKey/InstanceID/StepName/EffectIndex/Kind/Target/Args/Result/Delivered/CreatedAt/DeliveredAt).
- `TestLoadOutboxNotFound` (обе impl): Load несуществующего ключа → `errors.Is(err, ErrOutboxNotFound)`.
- `TestSaveOutboxUpsert` (обе impl): Save дважды с одним dedup_key (разный Delivered/Result) → Load возвращает последнее; одна строка в таблице.
- `TestMemoryOutboxDeepCopy` (Memory): мутировать `Args[0]` / `*DeliveredAt` после Save → повторный Load возвращает неизменённое (изоляция).
- `TestStoreHas18Methods` / двойной compile-замок: сборка обеих impl с обоими методами; мутпроба — удалить метод из одной impl → сборка падает.
