# Implementation Plan: Стабильные контентные ключи триггеров

**Branch**: `027-stable-trigger-keys` | **Date**: 2026-06-22 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/027-stable-trigger-keys/spec.md`

## Summary

Заменить позиционный durable-ключ метрика- и расписание-триггеров (`triggerID(idx) = "trg-<N>"`
по индексу в `interp.Triggers()`) на **контентный** ключ, выводимый детерминированно из условия
триггера: `"trg-" + hex16(FNV-1a-64(canonical(условие) + "#" + порядковый_номер))`. Это устраняет
тихую порчу edge-baseline: вставка/перестановка/удаление любого несвязанного триггера (включая
событие- и дедлайн-, у которых нет durable-состояния) больше не сдвигает индекс и не заставляет
триггер унаследовать чужую строку `trigger_state`.

Технический подход — три согласованные правки + миграция:

1. Новый **чистый AST→строка** канонизатор в листовом пакете `internal/ast`
   (`canon.go`): `CanonicalTriggerCondition(spec)` + тотальный рекурсивный `canonExpr` по всем
   19 видам выражений языка с **громким** `default`-panic (инвариант «не должно случиться»,
   Конституция III) — нет молчащего схлопывания разных форм.
2. Новый **ключ-билдер** в `internal/daemon` (группировка по канонической строке, 0-based
   порядковый номер внутри группы дубликатов, `hash/fnv`-хеш), вычисляющий стабильный массив
   ключей **один раз при инициализации демона** (поле `triggerKeys []string`); два call-site
   `triggerID(idx)` → `d.triggerKeys[idx]`; старый `triggerID` удалён.
3. Миграция схемы Store **2→3**: `DELETE FROM trigger_state;` + бамп `currentSchemaVersion`
   (INV-R1: baseline 1 + 2 ступени = 3). Переход = сброс состояния + ленивый ре-прайминг.
4. FR-010: `checkAt` (schedule_at) приводится к **прайм-без-срабатывания** на первом промахе при
   `сейчас >= цель` (узко-ограниченное отступление от «семантика edge-детекта не меняется», см.
   Complexity Tracking) — иначе сброс `trigger_state` вызвал бы ложные catch-up-запуски.

Никаких новых зависимостей (`hash/fnv`, `strconv`, `hash` — stdlib), новых ключевых слов,
builtins, eval-кодов. Тип `TriggerState` и DDL `trigger_state` **не меняются** — меняется только
формат значения ключа.

## Technical Context

**Language/Version**: Go 1.22+ (Конституция I), идиоматичный `gofmt`/`go vet ./...` без замечаний.

**Primary Dependencies**: только stdlib — `hash/fnv` (FNV-1a-64), `strconv` (нормализация
чисел/строк/булевых), `fmt` (hex-форматирование). Никаких новых внешних зависимостей; единственная
внешняя зависимость проекта — `modernc.org/sqlite` (не затрагивается).

**Storage**: SQLite (`modernc.org/sqlite`, чистый Go) + in-memory parity-impl. Таблица
`trigger_state` (DDL **не меняется**); миграция схемы `2→3` через существующий механизм
`schemaMigrations` (`sqlite.go`). `Memory` impl без версии схемы — миграция SQLite-специфична.

**Testing**: стандартный `testing` (table-driven, `t.Run`/`t.Helper`/`t.TempDir`), **без**
`testify`. Дом-хелперы: `eachStore` (`daemon/restart_test.go:35`, memory+sqlite сабтесты),
`TestTriggerStoreContract` (`store/trigger_store_test.go`), паттерн `store/migrate_test.go`.
Маркер инверсионного замка в докстрингах — 🔁.

**Target Platform**: один статический бинарник `ladix` (Linux/macOS/Windows), CGO запрещён.

**Project Type**: интерпретатор DSL (single project, Go-раскладка `cmd/` + `internal/`).

**Performance Goals**: ключи считаются один раз при инициализации демона (не per-tick); на тике —
O(1) чтение из массива по индексу. Канонизатор — линейный обход AST-условия (мелкие деревья).

**Constraints**: детерминизм (один и тот же AST → одна и та же строка → один и тот же ключ);
нулевой регресс контракта Store; пустой функц.дифф `internal/{eval,engine}`; FixedClock в тестах.

**Scale/Scope**: дифф в 4 пакетах (`internal/ast`, `internal/daemon`, `internal/store`, +`cmd/ladix`
только если конструктор демона передаёт `interp` — **проверено: НЕ передаёт, см. ниже**) + docs/SPEC.

### Находка: структура демона и его конструктор (единственный «unknown» рецепта)

- **Структура**: `type Daemon struct { … interp *eval.Interpreter … }` —
  `src/internal/daemon/daemon.go:25-33` (поле `interp` :28).
- **Конструктор**: `func New(st, eng, interp, clock, interval, out) *Daemon` —
  `src/internal/daemon/daemon.go:37-49` (литерал структуры :41-48).
- **Где живёт `interp.Triggers()`**: `eval/interpreter.go:141-143` → `[]*ast.TriggerDecl` в порядке
  объявления; читается в демоне `metrics.go:33` и `schedule.go:42`.
- **Call-sites `New`**: ровно 1 прод — `src/cmd/ladix/serve.go:326`
  (`daemon.New(st, eng, interp, clock, interval, stdout)`); 4 тест — `daemon_test.go:56`,
  `checkdeadlines_test.go:39`, `m2_endtoend_test.go:98`, `helpers_test.go:123`.

**Расхождение с рецептом / корректировка**: рецепт допускал, что конструктор демона может жить в
`cmd/ladix` и принимать `interp` извне — **это не так**. `interp` уже передаётся в `New` и хранится
полем. Поэтому `triggerKeys` заполняется **внутри `New`** из `interp.Triggers()` сразу после
литерала структуры (никакой правки сигнатуры `New`, никакого нового параметра, нулевой дифф в
сигнатуре прод call-site `serve.go:326` и 4 тест call-sites). **`cmd/ladix` НЕ в диффе фичи** (он
лишь передаёт уже имеющийся `interp`). Это сужает границы диффа против исходного рецепта.

**Прочие сверки рецепта с кодом (совпадают)**: `DurationLit.Unit` — `string`
(`ast/literal.go:45`), значит `canonDuration = d.Amount+"|"+d.Unit` корректно; `AtSchedule.At` —
**значение** `StringLit` (не указатель, `ast/trigger.go:118`), значит `at.At.Value` корректно;
`CompOp.String()`→`BinOp.String()` (`op.go:68`), `UnOp.String()` (`op.go:85`) существуют;
`checkAt` `:105-133` с ветвью `if alreadyToday || now.Before(target) { return }` (:121) — точка
вставки FR-010.

## Constitution Check

*GATE: пройти до Phase 0. Перепроверено после Phase 1 — без новых нарушений.*

| # | Принцип | Вердикт | Обоснование |
|---|---------|---------|-------------|
| I | Язык и сборка | ✅ PASS | Go 1.22+, `gofmt`/`go vet` чисто. **0 новых зависимостей** — `hash/fnv`, `strconv`, `hash` из stdlib; CGO не вводится. |
| II | Парсинг — ручной | ✅ PASS | Не затрагивает лексер/парсер. Канонизатор работает над **уже разобранным** AST, не парсит текст (regexp/генераторы не вводятся). |
| III | Ошибки — явные типы | ✅ PASS | Канонизатор — чистая функция; неизвестная форма выражения = **громкий `panic`** как инвариант «не должно случиться» (ДОПУСКАЕТСЯ Принципом III). Штатные пути не паникуют; CLI recover-барьер `guard()` в `serveMain` цел. Хелпер `safeFire` (tick.go:30) изолирует панику тела триггера — не затрагивается. |
| IV | Позиции — сквозные | ✅ PASS | Канонизатор читает поля значений узлов (Name/Value/Op/…), `Position` не трогает и не теряет. Узлы `ast` остаются листовыми (не импортируют `errors`). |
| V | Без глобального состояния | ✅ PASS | Ключи — **поле `triggerKeys []string` экземпляра `Daemon`**, заполняется в конструкторе `New` (DI). **Никакого пакетного `var`.** Канонизатор не держит состояния. Миграция — детерминированный DDL без глобалов. |
| VI | Тесты — вперёд | ✅ PASS | Не лексер/парсер, но фича сопровождается 9 замками (вкл. исчерпываемость канонизатора с громким default, инверсионные 🔁-локи на ядро бага, миграцию, FR-010). Тесты — часть каждой задачи. |
| VII | Раскладка проекта | ✅ PASS | `canon.go` — в листовом `internal/ast` (опираются на него, он ни на кого выше не опирается; импортирует только `fmt`/`strconv`). `internal/daemon` опирается на `ast` (ребро уже есть). Цикл не вводится. |
| VIII | Язык сообщений | ✅ PASS | Канонические строки — **внутренние** (durable-ключ, не пользовательский вывод; диагностикой не печатаются). Существующие русские строки демона/диагностики не меняются. Panic-текст канонизатора — внутренний инвариант (не пользовательское сообщение §13). |
| IX | Спека — источник истины | ✅ PASS | Doc-sync (SPEC §C-9/§12/FR-023, engine-model §EM-17.2.1, trigger-/reliability-/automation-model) запланирован символьными ссылками. Открытых вопросов нет (все NEEDS CLARIFICATION закрыты в research.md). |

**Итог Constitution Check: 9/9 PASS.** Единственное отступление — FR-010 (изменение поведения
schedule_at) — это **не** нарушение принципа, а узко-ограниченная смена поведения, явно
санкционированная спекой; зафиксирована в Complexity Tracking ниже.

## Project Structure

### Documentation (this feature)

```text
specs/027-stable-trigger-keys/
├── plan.md              # Этот файл (/speckit-plan)
├── research.md          # Phase 0 (/speckit-plan)
├── data-model.md        # Phase 1 (/speckit-plan)
├── quickstart.md        # Phase 1 (/speckit-plan) — тест-стратегия (9 замков)
├── contracts/
│   ├── canon.md         # Контракт CanonicalTriggerCondition / canonExpr
│   ├── trigger-keys.md  # Контракт ключ-билдера демона
│   └── migration.md     # Контракт миграции схемы 2→3
└── tasks.md             # Phase 2 (/speckit-tasks — НЕ создаётся этой командой)
```

### Source Code (repository root)

```text
src/
├── cmd/ladix/
│   └── serve.go                      # call-site daemon.New(:326) — НЕ в диффе (interp уже передаётся)
└── internal/
    ├── ast/
    │   ├── canon.go                  # НОВЫЙ: CanonicalTriggerCondition + canonExpr (чистый AST→string)
    │   ├── canon_test.go             # НОВЫЙ: T1 исчерпываемость 19 типов + громкий default
    │   ├── trigger.go                # READ-ONLY: TriggerSpec, MetricTrigger, ScheduleTrigger…
    │   ├── literal.go / expr.go      # READ-ONLY: 19 видов Expression
    │   └── op.go                     # READ-ONLY: CompOp/BinOp/UnOp .String()
    ├── daemon/
    │   ├── daemon.go                 # ПРАВКА: поле triggerKeys + заполнение в New
    │   ├── tick.go                   # ПРАВКА: удалить triggerID; (опц.) разместить keys-билдер рядом
    │   ├── keys.go                   # НОВЫЙ (вариант): buildTriggerKeys([]*ast.TriggerDecl) []string
    │   ├── metrics.go                # ПРАВКА: triggerID(idx) → d.triggerKeys[idx] (:38)
    │   ├── schedule.go               # ПРАВКА: triggerID(idx)→d.triggerKeys[idx] (:47) + FR-010 в checkAt
    │   └── *_test.go                 # НОВЫЕ замки T2..T6, T8, T9
    └── store/
        ├── sqlite.go                 # ПРАВКА: schemaMigrations += "DELETE FROM trigger_state;"; ver 2→3
        ├── sqlite_test.go / migrate_test.go  # НОВЫЙ замок T7 (миграция 2→3)
        └── types.go / store.go       # READ-ONLY: TriggerState, DDL, контракт Store (НЕ меняются)

docs/                                 # doc-sync (implement-стадия): engine-/trigger-/reliability-/automation-model
SPEC.md                               # doc-sync: FR-023, §C-9/§12 «стабильные ключи триггеров»
```

**Structure Decision**: single project, стандартная Go-раскладка `cmd/ladix` + `internal/*`
(Конституция VII). Новый код локализован в **листовом** `internal/ast` (канонизатор) и
`internal/daemon` (ключ-билдер + call-site + FR-010); миграция — в `internal/store`. `cmd/ladix`
**вне диффа**. Размещение ключ-билдера — `keys.go` (новый) либо рядом с удалённым `triggerID` в
`tick.go`; решение — деталь стадии implement (контракт идентичен).

## Complexity Tracking

> Заполняется ТОЛЬКО при отступлениях, требующих обоснования.

| Отступление | Зачем нужно | Почему более простой путь отвергнут |
|-------------|-------------|-------------------------------------|
| **FR-010: изменение поведения schedule_at** (на первом промахе при `сейчас >= цель` — прайм-без-срабатывания вместо catch-up) | Единообразие прайм-семантики между всеми тремя видами durable-триггеров обязательно для **поведенческой нейтральности первого тика после апгрейда** (FR-009/FR-010). После миграции `DELETE FROM trigger_state` все триггеры на первом тике видят «промах»; метрика и `каждые` уже праймят-без-срабатывания, а schedule_at сейчас **догоняет тело** (catch-up). Без приведения сброс баз вызвал бы ложные плановые запуски — прямое противоречие цели фичи. | «Оставить catch-up» отвергнут: напрямую воспроизводит дефект (тихие ложные запуски при апгрейде). «Не делать DELETE, мигрировать ключи на лету» невозможно: миграция видит только базу, не AST программы → старые `trg-<N>` непереиздаваемы в контентные. Изменение узко-ограничено (только ветка `miss && сейчас>=цель`; случай `сейчас<цель` не трогается, штатно сработает в target) и залочено тестом (T8) с инверсионным 🔁. |
| **Громкий `panic` в `default` канонизатора** | Конституция III ДОПУСКАЕТ панику как инвариант «не должно случиться». Тотальный switch по 19 типам без молчащего default — единственный способ гарантировать, что добавление нового вида выражения **не** схлопнется тихо в чужую каноническую строку (SC-007, FR-003). | «Молчащий default → пустая строка / fmt.Sprintf("%T")» отвергнут: тихо коллапсирует разные выражения в один ключ (порча baseline) либо даёт недетерминированную форму; нарушает FR-003. Исчерпываемость защищена тестом T1 (stub-тип Expression → recover-assert panic). |

> FR-010 — **смена поведения**, санкционированная спекой (§Complexity Tracking spec.md), не нарушение
> принципа. `panic`-default — явно разрешённое Принципом III исключение. Обе записи — для трассируемости.
