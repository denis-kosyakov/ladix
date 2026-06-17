# Contract — `openStore` хелпер (B5, §AU-9)

Цель: устранить инлайн-дубли конструкции Store (5 мест, §AU-9) и дать `start` единую точку. Реализация —
узкий снимок логики runFile (`main.go:235-244`), НЕ вся функция.

## Сигнатура (решение plan Р-3)

```go
// openStore конструирует Store по dbPath: непустой → SQLiteStore (closeFn = sq.Close),
// пустой → MemoryStore (closeFn = no-op). Ошибка открытия SQLite пробрасывается.
func openStore(dbPath string) (st store.Store, closeFn func() error, err error)
```

Альтернатива (если impl предпочтёт) — вернуть `(store.Store, error)` и `*store.SQLiteStore` отдельно для
defer Close; точная форма — решение impl, но контракт фиксирует поведение ниже.

## Поведение

| dbPath | Store | closeFn | примечание |
|--------|-------|---------|-----------|
| `""` | `store.NewMemoryStore()` | no-op (`func() error { return nil }`) | эфемерно (run/serve семья) |
| непустой | `store.NewSQLiteStore(dbPath)` | `sq.Close` | персист (complete/tasks/emit/start/inspect семья) |

Ошибка `NewSQLiteStore` → `(nil, nil, err)`; вызывающий печатает CLI-ошибку открытия БД (паритет
существующих команд: `ladix: не удалось открыть БД …` или текущий текст runFile).

## Потребитель в B5

`start` зовёт `openStore(dbPath)` с `dbPath` дефолтом `defaultDBPath="ladix.db"` (D-AU-10):
```go
st, closeStore, err := openStore(dbPath)
if err != nil { /* CLI-ошибка открытия БД */ return 2 }
defer closeStore()
```

## Регресс-инвариант (US4-3, замок)

- Введение `openStore` НЕ меняет наблюдаемое поведение существующих команд (complete/tasks/emit/run/serve).
- Рефактор существующих команд под `openStore` — ОПЦИОНАЛЕН и допустим ТОЛЬКО при зелёном golden всех
  затронутых команд. Если рефактор рискует регрессом — `start` использует хелпер, инлайн-дубли остаются
  (минимальная правка). Решение по объёму рефактора — impl, под защитой golden.
- Golden-замки complete/tasks/emit (`cmd/ladix/*_test.go`) ДОЛЖНЫ остаться зелёными после изменения.

## Инварианты

- НЕ новый метод Store (=16 после B6 цел). НЕ меняет интерфейс `store.Store`.
- Единая `--db` консистентна (INV-2): start → SQLite ladix.db; metric → всегда Memory (без флага).
- 0 новых зависимостей.
