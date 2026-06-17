# Data Model: B3 payload `данные`

Источник истины §AU-5.3. Никаких персистентных сущностей B3 НЕ вводит (D-AU-3 —
эфемерность). Ниже — транзиентная модель и сигнатуры протяжки.

## Сущность: payload `данные` (транзиентная)

| Свойство | Значение |
|----------|----------|
| Тип | `value.Запись` (read-only) |
| Источник | `--данные '<json-объект>'` → `jsonval.PayloadToRecord` |
| Без флага | пустая `Запись` (`value.NewRecord(nil, nil)`) |
| Область жизни | первый шаг догона ОДНОГО `complete` |
| Персист | **нет** (нет полей в `Task`/`ProcessInstance`/SQLite) |
| Имя в среде | `данные` (в `stepEnv`, НЕ `processEnv`) |
| Мутабельность | read-only (чтение полей; `присвоить данные = …` запрещён) |

Маппинг декода (из `jsonval.PayloadToRecord`): объект→`Запись` (порядок ключей),
число без `.eE`→`Целое` (overflow→`Дробное`), дробь→`Дробное`, строка→`Строка`,
bool→`Булево`, null→`Пусто`, массив→`Список`, вложенный объект→`Запись`. Не-объект на
верхнем уровне → ошибка.

## Протяжка `data` — 4 функции движка (§AU-5.3, cite-точно)

Все сигнатуры — ВНУТРЕННИЙ API `*Engine` (НЕ `ProcessRuntime`). Расширяются
параметром `data value.Запись`. Существующие вызовы передают пустую `Запись`.

| Функция (engine.go) | ДО (на `aebac92`) | ПОСЛЕ (B3) |
|---------------------|-------------------|------------|
| `Complete` (:108) | `Complete(taskID string) (CompleteResult, error)` | `Complete(taskID string, data value.Запись) (CompleteResult, error)` |
| `catchUp` (:177) | `catchUp(inst *store.ProcessInstance, t *store.Task) (...)` | `catchUp(inst *store.ProcessInstance, data value.Запись, t *store.Task) (...)` |
| `advanceAfterComplete` (:189) | `advanceAfterComplete(inst *store.ProcessInstance, caughtUp bool) (...)` | `advanceAfterComplete(inst *store.ProcessInstance, data value.Запись, caughtUp bool) (...)` |
| `advance` (:242) | `advance(inst *store.ProcessInstance) error` | `advance(inst *store.ProcessInstance, data value.Запись) error` |

Цепочка вызовов (с `data`):

```text
Complete(taskID, data)
 ├─ ветка догона D-4:        catchUp(inst, data, t)
 │                            └─ advanceAfterComplete(inst, data, true)
 ├─ ветка гонки D-12:        catchUp(fresh, data, t)
 │                            └─ advanceAfterComplete(inst, data, true)
 └─ прямой путь:             advanceAfterComplete(inst, data, false)
                              └─ advance(inst, data)   [если есть next-шаг]
```

`advanceAfterComplete` зовёт `advance(inst, data)` (:202) ТОЛЬКО когда есть
следующий шаг (терминал → `выполнен`, `advance` не зовётся, `data` не используется —
норма).

## Точка инжекта (`advance`, §AU-5.3)

В `advance` ДО цикла шагов:

```text
cur := data                              // payload первого шага
for {
    ...
    stepEnv := eval.NewEnvironment(processEnv)   // :262 (per-step)
    stepEnv.Define("данные", cur)                // <-- НОВОЕ (НЕ processEnv)
    ... фаза атрибутов ...
    e.interp.ExecStepBody(processEnv, stepEnv, step.Body)   // :275
    ...
    cur = value.NewRecord(nil, nil)              // <-- после 1-й итерации: пусто
    inst.CurrentStep = next
}
```

Инварианты точки инжекта:
- `Define` в **`stepEnv`**, не `processEnv` (иначе переживёт догон через персист
  `inst.Variables` → нарушит эфемерность). **Замок US2-b: мутация на processEnv →
  второй шаг ВИДИТ данные / payload персистится → красный.**
- Первая итерация инжектит `data`; после неё `cur = пустая Запись` → второй+ шаг
  видит пусто. **Замок US2-a.**
- Read-only: `stepEnv` под существующим барьером тела (как триггер); `присвоить
  данные = …` запрещён, чтение `данные.поле` разрешено.

## Эфемерность — что НЕ меняется (D-AU-3)

- `store.Task` — БЕЗ нового поля; `store.ProcessInstance` — БЕЗ нового поля; схема
  SQLite — БЕЗ новой колонки. **Замок US2-b (персист-проба).**
- После `complete` перечтение Store не содержит payload; рестарт не воскрешает.
- `ProcessRuntime` — 8 методов (не растёт). `internal/eval` — без импорта
  store/engine/jsonval.

## Состояния и переходы

Без новых состояний инстанса/задачи. Поток complete тот же (`ожидает`→`выполняется`
→`ожидает`/`выполнен`/`провален`); `data` — лишь дополнительная переменная среды
первого шага догона.
