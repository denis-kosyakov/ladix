# Data Model: Стабильные контентные ключи триггеров

Фича **не вводит новых durable-сущностей** и **не меняет схему** таблицы `trigger_state` (FR-007):
тип `store.TriggerState`, DDL (`sqlite.go:52-58`) и контракт `Store` остаются байт-целы. Меняется
только **формат значения** поля-ключа (`TriggerID`) и добавляется ступень миграции. Ниже — концепции
данных и инварианты.

## Сущность 1: Каноническая форма условия триггера (рантайм, не durable)

- **Что**: детерминированная строка, выведенная из **AST-условия** триггера (не из текста
  исходника). Основа контентного ключа.
- **Где живёт**: возвращается `ast.CanonicalTriggerCondition(spec TriggerSpec) string`
  (`internal/ast/canon.go`). Не хранится, не сериализуется — промежуточное значение при инициализации
  демона.
- **Правила вывода (`CanonicalTriggerCondition`)** — type-switch по `TriggerSpec`:

  | Вид триггера | Каноническая строка |
  |--------------|---------------------|
  | `*MetricTrigger{Metric, Op, Threshold}` | `"metric|" + Metric.Name + "|" + Op.String() + "|" + canonExpr(Threshold)` |
  | `*ScheduleTrigger` → `*EverySchedule{Every}` | `"every|" + canonDuration(Every)` |
  | `*ScheduleTrigger` → `*AtSchedule{At}` | `"at|" + At.Value` |
  | `*EventTrigger` | `""` (нет durable-ключа) |
  | `*DeadlineTrigger` | `""` (нет durable-ключа) |

  где `canonDuration(d *DurationLit) = d.Amount + "|" + d.Unit`.

- **Инвариант различимости пустой строки**: ключевые виды (метрика/`каждые`/`в`) **всегда непусты**
  (префикс `metric|`/`every|`/`at|`). `""` означает «не-ключевой вид» (событие/дедлайн) — отличимо,
  слот ключа для него не читается.
- **Каноническое равенство** (SC-002): канонически-равные условия → равная строка → равный ключ.
  Косметика (разделители разрядов, пробелы) исчезает на уровне разобранного AST.

## Сущность 2: Сериализатор выражений `canonExpr` (рантайм, чистая функция)

- **Что**: тотальный рекурсивный `canonExpr(e ast.Expression) string` — покрывает **ВСЕ 19** видов
  выражений языка; `default` → `panic` (инвариант «не должно случиться», Конституция III).
- **Полная таблица форм** (порядок — как в карте кода):

  | Тип (`internal/ast`) | Каноническая форма |
  |----------------------|--------------------|
  | `IntLit{Value int64}` | `strconv.FormatInt(Value, 10)` |
  | `FloatLit{Value float64}` | `strconv.FormatFloat(Value, 'g', -1, 64)` (кратчайший round-trip — канон) |
  | `StringLit{Value string}` | `strconv.Quote(Value)` (экранированная закавыченная) |
  | `BoolLit{Value bool}` | `strconv.FormatBool(Value)` |
  | `NoneLit{}` | `"пусто"` |
  | `DurationLit{Amount, Unit string}` | `"длит(" + Amount + "|" + Unit + ")"` |
  | `WindowPeriodLit{Amount, Unit string}` | `"окно(" + Amount + "|" + Unit + ")"` |
  | `LastCompletedPeriodLit{Noun string}` | `"прошлый(" + Noun + ")"` |
  | `ListLit{Elements []Expression}` | `"[" + join(canonExpr(el), ",") + "]"` |
  | `Ident{Name string}` | `Name` |
  | `BinaryExpr{Op, Left, Right}` | `"(" + canonExpr(Left) + " " + Op.String() + " " + canonExpr(Right) + ")"` |
  | `UnaryExpr{Op, Operand}` | `"(" + Op.String() + canonExpr(Operand) + ")"` |
  | `CallExpr{Callee, Args}` | `canonExpr(Callee) + "(" + join(args, ",") + ")"` |
  | `IndexExpr{Target, Index}` | `canonExpr(Target) + "[" + canonExpr(Index) + "]"` |
  | `FieldExpr{Target, Field}` | `canonExpr(Target) + "." + Field.Name` |
  | `RunProcessExpr{Process, Args}` | `"запустить(" + Process.Name + "|" + join(args, ",") + ")"` |
  | `CallExternalExpr{Target, Args}` | `"вызвать(" + Target.Name + "|" + join(args, ",") + ")"` |
  | `ValueExpr{}` | `"значение"` |
  | `EventExpr{}` | `"событие"` |
  | (любой иной) | `panic(fmt.Sprintf("canonExpr: незнакомый тип выражения %T", e))` |

- **Жёсткие инварианты** (всё остальное — деталь формы, может уточняться):
  1. **Детерминизм**: один и тот же AST → одна и та же строка (стабильно между прогонами/версиями
     при фиксированной версии Go).
  2. **Отсутствие тихого схлопывания**: разные выражения → разные строки; неизвестная форма —
     громкий сбой, не молчаливое совпадение.
  3. **Нормализация чисел**: разделители разрядов игнорируются (`10_000_000 ≡ 10000000`); дробное —
     фиксированная (round-trip) форма.
- **Исчерпываемость** — защищена тестом T1 (stub-тип `Expression` → `default`-panic).

## Сущность 3: Durable-ключ триггера (формат значения меняется, тип — нет)

- **Что**: строковый идентификатор строки durable-состояния (поле `TriggerState.TriggerID`,
  PRIMARY KEY).
- **До фичи**: позиционный — `triggerID(idx) = "trg-<N>"`, N — индекс в `interp.Triggers()`.
- **После фичи**: контентный —
  `"trg-" + fmt.Sprintf("%016x", FNV-1a-64(canonical + "#" + strconv.Itoa(ord)))`.
  - `canonical` — строка Сущности 1 (непуста для ключевых видов).
  - `ord` — 0-based порядковый номер **внутри группы** триггеров с одинаковой `canonical` (FR-004).
- **Тип поля и контракт хранилища НЕ меняются** (FR-007): поле уже строковое, принимает
  произвольную строку. `LoadTriggerState`/`SaveTriggerState`/`ErrTriggerStateNotFound`,
  `TriggerState{TriggerID, Kind, LastBool, LastFire, LastFiredDate}` — байт-целы.
- **Стабильность**: уникальное условие → `ord=0` → ключ **не зависит от позиции/соседей** (SC-001).

## Сущность 4: Массив ключей демона (`triggerKeys []string`, поле экземпляра)

- **Что**: срез контентных ключей, **выровненный по индексам** `interp.Triggers()`.
- **Где живёт**: новое поле `triggerKeys []string` структуры `Daemon` (`daemon.go:25-33`).
  Конституция V — состояние экземпляра, **не** пакетный `var`.
- **Заполнение**: один раз в конструкторе `New` (`daemon.go:37-49`) через
  `buildTriggerKeys(interp.Triggers())` (FR-005). Per-tick — только чтение `d.triggerKeys[idx]`.
- **Алгоритм `buildTriggerKeys(trig []*ast.TriggerDecl) []string`**:
  ```
  keys := make([]string, len(trig))
  ordinals := map[string]int{}
  for idx, td := range trig {
      c := ast.CanonicalTriggerCondition(td.Spec)
      if c == "" { continue }              // событие/дедлайн — слот пуст, не читается
      ord := ordinals[c]; ordinals[c]++
      h := fnv.New64a(); h.Write([]byte(c + "#" + strconv.Itoa(ord)))
      keys[idx] = "trg-" + fmt.Sprintf("%016x", h.Sum64())
  }
  return keys
  ```
- **Чтение**: `metrics.go:38` `id := d.triggerKeys[idx]`; `schedule.go:47` `id := d.triggerKeys[idx]`.
  Слоты событие-/дедлайн-триггеров (`""`) **не читаются** (events.go/checkdeadlines.go не зовут ключ).

## Сущность 5 (durable, без изменений): `trigger_state`

- **Поля** (`store/types.go:64-70`): `TriggerID, Kind, LastBool, LastFire, LastFiredDate` — **не
  меняются**.
- **Kind-консты** (`daemon`): `metricKind="metric"` (`metrics.go:12`),
  `everyKind="schedule_every"`, `atKind="schedule_at"` (`schedule.go:14-17`) — **не меняются**.
- **DDL** (`sqlite.go:52-58`): `trigger_state(trigger_id TEXT PRIMARY KEY, …)` — **не меняется**;
  входит в `baselineVersion=1`.
- **Parity Memory**: `memory.go:143/153`; SQLite Load `:501-534` / Save `:536-548` — **не меняются**.

## Сущность 6: Ступень миграции схемы 2→3

- **Что**: новый (3-й) элемент `schemaMigrations` (`sqlite.go:106-122`) =
  `"DELETE FROM trigger_state;"`; `currentSchemaVersion` 2→3 (`sqlite.go:82-84`).
- **Инвариант INV-R1** (`init()` `:91-97`):
  `currentSchemaVersion (3) == baselineVersion (1) + len(schemaMigrations) (2)`. **`1 + 2 = 3`** —
  двойной compile/runtime-замок (паника при рассогласовании).
- **Применение** (`migrate()` `:374-412`): `PRAGMA user_version`; для `v < 3`:
  `schemaMigrations[v-baseline]` + `PRAGMA user_version = v+1` в одной транзакции (атомарно).
  Forward-only. Вызов из `NewSQLiteStore:150`.
- **Семантика перехода**: сброс состояния + ленивый ре-прайминг (миграция видит только базу, не
  AST → старые `trg-<N>` непереиздаваемы). Поведенческая нейтральность первого тика обеспечивается
  прайм-семантикой (вкл. приведённый под неё schedule_at, FR-010).

## Переход состояния: первый тик после апгрейда (поведенческая нейтральность)

| Вид триггера | Состояние после `DELETE` | Поведение на первом тике (промах) | Источник |
|--------------|--------------------------|-----------------------------------|----------|
| Метрика | строки нет | прайм: `Save{LastBool:cur}` + continue, **НЕ срабатывает** даже при `cur==true` | `metrics.go:59-67` (без изменений) |
| Расписание `каждые` | строки нет | прайм: `Save{LastFire:now}` + return, **НЕ срабатывает** | `schedule.go:66-74` (без изменений) |
| Расписание `в "ЧЧ:ММ"`, `сейчас >= цель` | строки нет | **БЫЛО**: catch-up → fire; **СТАНЕТ** (FR-010): прайм `Save{LastFiredDate:today}` + return, **НЕ срабатывает** | `checkAt` `schedule.go:105-133` (правка FR-010) |
| Расписание `в "ЧЧ:ММ"`, `сейчас < цель` | строки нет | без изменений: `now.Before(target)`→return; штатно сработает в target | `checkAt :121` (не трогается) |
| Событие / дедлайн | (нет durable-состояния) | не участвуют в edge-сопоставлении; ключа нет | — |
