# Contract: ПОТРЕБЛЕНИЕ `jsonval.PayloadToRecord` (без дубля)

Scope B3 СУЖЕН: `internal/jsonval` уже существует (B2/014, merge `aebac92`). B3 —
ЧИСТЫЙ ПОТРЕБИТЕЛЬ, НЕ создатель/лифтёр пакета.

## C-JV-1 — используемая поверхность

```text
func jsonval.PayloadToRecord(payload string) (value.Запись, error)   // decode.go:31, экспортирован
```

- Единственная функция jsonval, нужная B3. Вызывается из `cmd/ladix/completeTask`.
- Контракт jsonval (не меняется B3): пустой payload → пустая `Запись` без ошибки;
  верхний уровень не `{` → ошибка `payload не является JSON-объектом`; числа без
  `.eE`→Целое (overflow→Дробное); null→Пусто; объект→Запись (порядок ключей);
  массив→Список.

## C-JV-2 — запреты (анти-дубль)

- B3 НЕ создаёт `internal/jsonval` (уже создан).
- B3 НЕ лифтит декодер из daemon повторно (лифт сделан в B2).
- B3 НЕ пишет ВТОРОЙ JSON→`Запись` декодер. Замок: в B3-диффе нет нового
  `json.Unmarshal`/`json.Decoder` для payload вне jsonval; `complete` зовёт ровно
  `jsonval.PayloadToRecord`.
- B3 НЕ трогает `eval/source_loader.go` (второй декодер источников M1 — отдельная
  строгая семантика, §SM-9.B; не сливать).

## C-JV-3 — импорт-граф

- `cmd/ladix` импортирует `jsonval` (корень композиции — допустимо).
- `internal/eval` НЕ импортирует `jsonval` (`данные` входит как `value.Запись`).
- `jsonval` остаётся листовым (value+stdlib), цикла нет.

## Инвариант (замок)

| Инвариант | Тест | Нарушение → красный |
|-----------|------|---------------------|
| jsonval переиспользован | `complete` зовёт `jsonval.PayloadToRecord`; нет 2-го payload-декодера | поиск второго `json.Decoder`/`Unmarshal` для payload в B3-диффе → находка |
| eval не тянет jsonval | `go list`/импорт-проба eval | импорт jsonval/store/engine в eval → красный |
