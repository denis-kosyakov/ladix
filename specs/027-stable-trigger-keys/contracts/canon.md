# Контракт: канонизатор условия триггера (`internal/ast/canon.go`)

Новый файл в **листовом** пакете `internal/ast`. Чистые функции AST → строка. Импортирует только
`fmt`, `strconv` (stdlib; листовость `ast` сохранена — `errors` не импортируется).

## `CanonicalTriggerCondition(spec ast.TriggerSpec) string`

**Назначение**: каноническая строка durable-условия триггера (FR-002). Type-switch по `TriggerSpec`.

**Контракт**:

| Вход (`spec`) | Выход |
|---------------|-------|
| `*MetricTrigger{Metric, Op, Threshold}` | `"metric|" + Metric.Name + "|" + Op.String() + "|" + canonExpr(Threshold)` |
| `*ScheduleTrigger` с `Spec == *EverySchedule{Every}` | `"every|" + canonDuration(Every)` |
| `*ScheduleTrigger` с `Spec == *AtSchedule{At}` | `"at|" + At.Value` |
| `*EventTrigger` | `""` |
| `*DeadlineTrigger` | `""` |

- `canonDuration(d *DurationLit) string = d.Amount + "|" + d.Unit`.
- **Инвариант**: для ключевых видов результат **всегда непуст** (гарантировано префиксом). `""` ⟺
  не-ключевой вид (нет durable-состояния). Слот `""` не читается на тике.
- **Детерминизм**: один `spec` → одна строка.

## `canonExpr(e ast.Expression) string` (приватный, тотальный, рекурсивный)

**Назначение**: сериализовать **любое** выражение языка в детерминированную строку (FR-003).
`switch e := e.(type)` по всем 19 конкретным типам; `default` → `panic`.

**Контракт по типам** (формы — деталь; жёсткие инварианты — ниже таблицы):

| Тип | Форма |
|-----|-------|
| `*IntLit` | `strconv.FormatInt(e.Value, 10)` |
| `*FloatLit` | `strconv.FormatFloat(e.Value, 'g', -1, 64)` |
| `*StringLit` | `strconv.Quote(e.Value)` |
| `*BoolLit` | `strconv.FormatBool(e.Value)` |
| `*NoneLit` | `"пусто"` |
| `*DurationLit` | `"длит(" + e.Amount + "|" + e.Unit + ")"` |
| `*WindowPeriodLit` | `"окно(" + e.Amount + "|" + e.Unit + ")"` |
| `*LastCompletedPeriodLit` | `"прошлый(" + e.Noun + ")"` |
| `*ListLit` | `"[" + strings.Join(map(canonExpr, e.Elements), ",") + "]"` |
| `*Ident` | `e.Name` |
| `*BinaryExpr` | `"(" + canonExpr(e.Left) + " " + e.Op.String() + " " + canonExpr(e.Right) + ")"` |
| `*UnaryExpr` | `"(" + e.Op.String() + canonExpr(e.Operand) + ")"` |
| `*CallExpr` | `canonExpr(e.Callee) + "(" + join(args) + ")"` |
| `*IndexExpr` | `canonExpr(e.Target) + "[" + canonExpr(e.Index) + "]"` |
| `*FieldExpr` | `canonExpr(e.Target) + "." + e.Field.Name` |
| `*RunProcessExpr` | `"запустить(" + e.Process.Name + "|" + join(args) + ")"` |
| `*CallExternalExpr` | `"вызвать(" + e.Target.Name + "|" + join(args) + ")"` |
| `*ValueExpr` | `"значение"` |
| `*EventExpr` | `"событие"` |
| **иное** | `panic(fmt.Sprintf("canonExpr: незнакомый тип выражения %T", e))` |

**Жёсткие инварианты (нормативны; формы выше — деталь)**:

1. **Тотальность**: каждый из 19 типов имеет ветвь; неизвестная форма — `panic` (Конституция III,
   «не должно случиться»). Нет молчащего `default`, схлопывающего разные выражения.
2. **Детерминизм**: один AST → одна строка (стабильно между прогонами при фикс. версии Go).
3. **Различимость**: разные выражения → разные строки. В частности — нормализация чисел:
   `10_000_000 ≡ 10000000` (равны после разбора), но `< 10000000 ≠ < 9999999`.
4. **Строки**: экранированная закавыченная форма (`strconv.Quote`) — служебные символы не
   коллизируют с разделителями.

## Замки (детально — в quickstart.md)

- **T1** (`ast/canon_test.go`): исчерпываемость — table из РОВНО 19 типов с ожидаемыми строками +
  локальный stub-тип, реализующий `Expression` (`exprNode()`), → `canonExpr(stub)` паникует
  (recover+assert). 🔁: убрать ветку из switch → стаб/тип в `default`-panic → краснеет.
- **T2/T3/T4** (parse→canon): равенство (формат числа/пробелы → один ключ), различие (имя/оп/порог
  → разные), дубликаты (ord 0/1 → разные).
