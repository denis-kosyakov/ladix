# Contract: JSON-схема пакета `ir` (ir-schema)

Контракт сериализуемого вывода фронтенда — версионируемое промежуточное представление
`ir.Program`. Это стабильный контракт **через границу модуля и версий** (FR-006 / FR-007 /
FR-008 / FR-016). Все JSON-теги — `snake_case`. Понижение `ast.Program → ir.Program`
детерминировано; выражения — канонические строки (v1).

## Версия схемы

```go
package ir

const SchemaVersion = 1
```

Сериализуется в поле `schema_version` корня `Program`. В v1 значение всегда `1`.

## Типы (форма)

```go
type Program struct {
    SchemaVersion int        `json:"schema_version"`
    Metrics       []Metric   `json:"metrics"`
    Processes     []Process  `json:"processes"`
    Triggers      []Trigger  `json:"triggers"`
}

type Metric struct {
    Name      string   `json:"name"`
    Source    string   `json:"source"`
    Where     string   `json:"where"`      // каноническая строка выражения
    Aggregate string   `json:"aggregate"`  // каноническая строка
    Period    string   `json:"period"`
    ByDate    string   `json:"by_date"`
    Pos       Position `json:"pos"`
}

type Process struct {
    Name   string   `json:"name"`
    Params []string `json:"params"`
    Steps  []Step   `json:"steps"`
    Pos    Position `json:"pos"`
}

type Step struct {
    Name     string   `json:"name"`
    After    string   `json:"after"`
    Assignee string   `json:"assignee"`
    Deadline string   `json:"deadline"`
    Actions  []string `json:"actions"`    // канонические строки действий
    Pos      Position `json:"pos"`
}

type Trigger struct {
    Kind      string   `json:"kind"`       // metric | schedule | event | deadline
    Metric    string   `json:"metric"`
    Op        string   `json:"op"`
    Threshold string   `json:"threshold"`  // каноническая строка выражения
    Event     string   `json:"event"`
    Schedule  string   `json:"schedule"`   // каноническая строка
    Process   string   `json:"process"`
    Step      string   `json:"step"`
    Pos       Position `json:"pos"`
}

type Position struct {
    Line int `json:"line"`
    Col  int `json:"col"`
}

type Diagnostic struct {
    Severity string   `json:"severity"`  // ∈ {"error"} в v1
    Stage    string   `json:"stage"`     // ∈ {"lex","parse","semantic"}
    Message  string   `json:"message"`   // дословно SPEC §13
    Pos      Position `json:"pos"`
}
```

## Пример сериализованной `Program`

Одна метрика, один процесс с одним шагом, один метрик-триггер:

```json
{
  "schema_version": 1,
  "metrics": [
    {
      "name": "выручка",
      "source": "продажи",
      "where": "статус == \"оплачено\"",
      "aggregate": "сумма(сумма_заказа)",
      "period": "ежемесячно",
      "by_date": "дата_заказа",
      "pos": { "line": 3, "col": 1 }
    }
  ],
  "processes": [
    {
      "name": "обработка_заказа",
      "params": ["заказ"],
      "steps": [
        {
          "name": "подтвердить",
          "after": "",
          "assignee": "менеджер",
          "deadline": "2 дня",
          "actions": ["уведомить(заказ)"],
          "pos": { "line": 12, "col": 3 }
        }
      ],
      "pos": { "line": 11, "col": 1 }
    }
  ],
  "triggers": [
    {
      "kind": "metric",
      "metric": "выручка",
      "op": "<",
      "threshold": "1000000",
      "event": "",
      "schedule": "",
      "process": "обработка_заказа",
      "step": "подтвердить",
      "pos": { "line": 20, "col": 1 }
    }
  ]
}
```

Незаданные поля сериализуются пустыми (`""` для строк) — не опускаются; неприменимые к
данному `kind` триггера поля остаются пустыми (например `event`/`schedule` у `kind:"metric"`).

## Диагностика

- `severity` ∈ `{"error"}` (v1 — только ошибки уровня `error`; warning/info отложены).
- `stage` ∈ `{"lex","parse","semantic"}` — этап, на котором возникла проблема.
- `message` — **дословно** текст из SPEC §13 (русские формулировки, exact-match);
  переформулирование ЗАПРЕЩЕНО (Принцип VIII, FR-007).
- `pos` — позиция в исходнике (`line`/`col`, 1-based; Принцип IV).

## Инварианты

- **Выражения — канонические строки (v1).** Поля `where`/`aggregate`/`threshold`/`schedule`/
  `actions[]` сериализуются как канонические строки, а НЕ как структурные узлы AST. Канон
  детерминирован и стабилен — той же природы, что `ast.CanonicalTriggerCondition` (одинаковый
  вход даёт байт-идентичную строку; нормализация чисел/строк/пробелов). (FR-008)
- **Детерминизм.** Сериализация `ir.Program` детерминирована: порядок коллекций
  (`metrics`/`processes`/`triggers`/`steps`) и значения канонических строк воспроизводимы.

## Политика `schema_version`

- **breaking-изменение формы** → bump `SchemaVersion` (FR-016, Edge Case спеки):
  удаление/переименование поля, смена типа поля, смена семантики существующего поля, переход
  выражений из канонических строк в **структурное** представление.
- **аддитивное изменение** (новое опциональное поле при сохранении совместимости чтения) —
  версию схемы НЕ меняет; аддитивные изменения языка тоже НЕ меняют `SchemaVersion`.
- В v1 структурное представление выражений сознательно отложено (Out of Scope) — будущий bump.
