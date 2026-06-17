# Contract: протяжка `data` через движок + точка инжекта

Слой: `internal/engine/engine.go`. ВНУТРЕННИЙ API `*Engine` (НЕ `ProcessRuntime`).

## C-ENG-1 — 4 функции принимают `data value.Запись`

```text
Complete(taskID string, data value.Запись) (CompleteResult, error)
catchUp(inst *store.ProcessInstance, data value.Запись, t *store.Task) (CompleteResult, error)
advanceAfterComplete(inst *store.ProcessInstance, data value.Запись, caughtUp bool) (CompleteResult, error)
advance(inst *store.ProcessInstance, data value.Запись) error
```

- Все вызовы внутри `Complete` прокидывают `data` (3 точки `catchUp`/`advanceAfterComplete`).
- Терминальная ветка `advanceAfterComplete` (next==∅) НЕ зовёт `advance` — `data`
  не используется (норма).

## C-ENG-2 — точка инжекта в `advance`

- Перед циклом: `cur := data`.
- В каждой итерации ПОСЛЕ `stepEnv := eval.NewEnvironment(processEnv)` и ДО
  `ExecStepBody`: `stepEnv.Define("данные", cur)`.
- ПОСЛЕ тела (в конце итерации/перед переходом к next): `cur = value.NewRecord(nil, nil)`.
- Инжект — в **`stepEnv`** (per-step), НИКОГДА в `processEnv`.

## C-ENG-3 — read-only `данные`

`данные` в `stepEnv` подчиняется существующему read-only барьеру тела (как тело
триггера): чтение `данные.поле` разрешено; `присвоить данные = …` запрещён (ошибка
барьера). B3 НЕ вводит новый барьер.

## Инварианты (замки)

| Инвариант | Тест | Мутация → красный |
|-----------|------|-------------------|
| Первый шаг видит payload | первый авто-шаг читает `данные.x`, сохраняет в var процесса | убрать `stepEnv.Define("данные", cur)` → var пуста |
| Второй+ шаг видит пусто | второй авто-шаг того же догона читает `данные.x` | убрать `cur = value.NewRecord(nil,nil)` → второй видит payload |
| Инжект в stepEnv, не processEnv | проба эфемерности (см. cli/jsonval контракты) | заменить на `processEnv.Define` → персист/утечка |
| `data` протянута всеми 4 | компиляция + поведение догона | удалить param из любой из 4 → compile-gap или payload не доходит |
| Регресс без `--данные` | golden complete 007b | пустая `Запись` по умолчанию даёт прежний вывод |
