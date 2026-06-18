# Contract: OutboxRecord codec round-trip (через store/codec.go)

Источник: §C-2b.6. Сериализация ВНУТРИ SQLiteStore (eval не импортирует store). Переиспользуем существующий value-кодек — 0 новых форматов.

## Кодирование

| Поле записи | Кодек | Колонка | Примечание |
|---|---|---|---|
| `Args []value.Value` | `encodeList(value.NewList(args))` (`codec.go:154`, `value/list.go:14`) | `args_json TEXT NOT NULL` | нет хелпера для голого `[]value.Value` → заворачиваем в `Список` |
| `Result value.Value` | `encodeValue(result)` (`codec.go:78`) | `result_json TEXT` | None → tagged-`Пусто` blob (`codec.go:88/250`), **НЕ SQL NULL** |
| `Delivered bool` | int 0/1 | `delivered INTEGER` | |
| `CreatedAt time.Time` | RFC3339 | `created_at TEXT NOT NULL` | |
| `DeliveredAt *time.Time` | RFC3339 / NULL | `delivered_at TEXT` | nil → SQL NULL допустим (это указатель времени, не value) |

## Декодирование (на пропуске-по-дедупу)

| Колонка | Кодек | Поле |
|---|---|---|
| `args_json` | `decodeList` (`codec.go:309`) → `.Elems()` | `Args []value.Value` |
| `result_json` | `decodeValue` (`codec.go:224`) | `Result value.Value` |
| `delivered` | int→bool | `Delivered` |
| `delivered_at` | RFC3339/NULL→`*time.Time` | `DeliveredAt` |

## Инвариант round-trip

`decode(encode(v)) == v` для:
- `Args` = пустой срез, один элемент, несколько элементов (число/строка/булево/None).
- `Result` = число (от `вызвать`), строка, **None** (statement-форма `уведомить`/statement-`вызвать`) → tagged-`Пусто`, НЕ NULL.

## Контрактные тесты (замки)

- `TestOutboxCodecArgsRoundTrip`: `[]value.Value{число, строка, None}` → encode → decode → равенство поэлементно.
- `TestOutboxCodecResultRoundTrip`: result=число → round-trip равен.
- `TestOutboxCodecResultNoneIsTaggedBlob`: result=`value.None` → `result_json` НЕ NULL (непустой tagged-blob); decode → `value.None`. **Мутпроба:** хранить None как SQL NULL → decode падает/возвращает не-None → тест краснит.
- `TestOutboxCodecEmptyArgs`: пустой `Args` → encode → decode → пустой срез (не nil-паника).
