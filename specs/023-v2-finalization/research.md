# Research: Финализация v2 — золотой сквозной пример §2

**Фича**: `023-v2-finalization` | **Якорь**: `docs/v2-finalization-model.md §F-1` (design-note
«дата-зависимость и тест-стратегия») + «Предрешённые развилки».

Кларификаций (NEEDS CLARIFICATION) нет — все развилки предрешены владельцем (Q1=B, Q2=A, Q3=A,
D-1=a, метрика=оконная, данные=`orders.csv`). Этот документ фиксирует тех-решения, унаследованные из
анкора и подтверждённые живой разведкой кодовой базы.

---

## R1. Дата-зависимость окна метрики и ловушка голого `run`

**Decision**: дата-зависимые golden'ы ОБЯЗАНЫ идти под `FixedClock{2026,6,15}`, НЕ под реальными
системными часами (`SystemClock`).

**Rationale**: метрика §2 — скользящее окно `последние 30дн` (faithful charter §2, показывает M1).
Снимок дата-зависим: окно `(d−30, d]`. На `FixedClock{2026,6,15}` окно `(2026-05-16, 2026-06-15]`
оставляет единственный оплаченный заказ `2026-05-27` (300000) → скаляр `300000.0`. **Ловушка
(подтверждено анкором §F-1):** голый `run` *сегодня* (2026-06-18) тоже даёт ~`300000.0` — это
сиюминутное совпадение; начиная с `2026-06-26` окно исключит `2026-05-27` → значение поедет к `0.0`,
и наивный golden станет ложно-красным по календарю. Поэтому FixedClock обязателен.

**Alternatives considered**:
- *Голый `run`/SystemClock* — отвергнут: ложно-зелёный сейчас, ложно-красный после 2026-06-26.
- *Периодless-метрика (дата-независимая)* — отвергнута развилкой владельца: не показывает окно →
  нарушает §2 (faithful charter).

---

## R2. Два пути исполнения: `runMetric` (скаляр) vs `runFile` RUN-путь (explain + старт)

**Decision**: T-GOLD-METRIC пинит 3 фасета **двумя разными путями**:
- (i) **скаляр** `выручка_30д = 300000.0` → `runMetric(path, "выручка_30д", depth, FixedClock, …)`.
- (ii) **строка explain** + (iii) **метрика-driven старт `p-000001`** → RUN-путь `runFile(path, "",
  depth, nil, clock, &out, &err)`.

**Rationale** (подтверждено сигнатурами с живого кода):
- `runMetric` (`src/cmd/ladix/main.go:300`, `func runMetric(path, metricName string, maxDepth int,
  clock eval.Clock, stdout, stderr io.Writer) int`) вычисляет **только скаляр** метрики — НЕ зовёт
  `interp.Run`/`interp.RunTriggers`, поэтому explain и старт инстанса НЕ эмитит.
- `runFile` (`src/cmd/ladix/main.go:227`, `func runFile(path, dbPath string, maxDepth int, caller
  engine.ExternalCaller, clock engine.Clock, stdout, stderr io.Writer) int`) исполняет top-level →
  `interp.Run` → `interp.RunTriggers` → `ExplainFire`: эмитит explain-строку (fire-if-true, без ребра)
  и метрика-driven старт процесса (`p-000001`).
- Следовательно один `runMetric`-паттерн все три фасета НЕ закрывает (FR-006). Очевидный кандидат
  `TestRevenueExampleFixedClockGolden` (`trigger_golden_test.go:507`) для (ii)/(iii) НЕ годится — он
  тоже зовёт `runMetric`, пинит лишь скаляр.

**Alternatives considered**: единый `runMetric`-замок — отвергнут (не наблюдает explain/старт).

---

## R3. Инъекция FixedClock — два типа Clock

**Decision**: использовать существующие типы без введения новых.
- **RUN-путь** (`runFile` принимает `engine.Clock`): тип `fixedClock{ t time.Time }`
  (`src/cmd/ladix/serve_golden_test.go:21`); значение `fixedClock{ time.Date(2026, 6, 15, 12, 0, 0, 0,
  time.Local) }`.
- **METRIC-путь** (`runMetric` принимает `eval.Clock`): `eval.FixedClock{D: value.Дата{Year: 2026,
  Month: 6, Day: 15}}` (как `fixedClock20260615`, `src/cmd/ladix/metric_window_golden_test.go:16`).

**Rationale**: оба типа уже существуют и используются соседними замками; инъекция в каждый путь даёт
детерминированный вывод. Живой референс run-инъекции — `clock_unify_test.go:52` (`runFile(prog, "",
depth, nil, clock, &out, &err)`). Соответствие конституции V (явная инъекция, без глобалов).

**Alternatives considered**: монотонные/новые Clock-типы — не нужны, FR-015 запрещает новую
функциональность.

---

## R4. Резолв пути к данным `data/orders.csv`

**Decision**: дата-зависимые/CSV-замки прогонять из **корня репозитория** через `withRepoRoot(t,
func(){…})` (chdir в repo-root), как делает `metric_window_golden_test.go`.

**Rationale**: источник `заказы` ссылается на относительный `data/orders.csv`; из cwd теста
`src/cmd/ladix` он не резолвится. Это вторая причина, по которой старый `TestCLIGoldenDeadlineEscalation`
ломается от добавления источника (первая — дата-зависимость, R1). Для T-LIFECYCLE — прогон из repo-root
с temp `--db` (`t.TempDir()`).

**Alternatives considered**: абсолютный csv-путь во `t.TempDir()` (паттерн `m2GoldenSrc`) — допустим,
но `withRepoRoot` проще и уже используется соседями.

---

## R5. Замена `TestCLIGoldenDeadlineEscalation`

**Decision**: **удалить** `TestCLIGoldenDeadlineEscalation` (`src/cmd/ladix/main_test.go:139`) и
заменить его детерминированной FixedClock-формой T-GOLD-METRIC.

**Rationale**: после впайки источника в `контроль_плана.ladix` старый замок ломается дважды —
(1) `data/orders.csv` не резолвится из cwd теста; (2) снимок метрики дата-зависим. Анкор §F-1 прямо
предписывает замену (Предрешённые развилки: «`TestCLIGoldenDeadlineEscalation` **заменяется**
FixedClock-golden'ом T-GOLD-METRIC»). FR-008/SC-006.

**Alternatives considered**: чинить старый замок in-place — отвергнуто (дата-наивная форма не
детерминирована; анкор требует замены, не патча).

---

## R6. Соседние golden'ы и co-land замок догона

**Decision**: `start_golden_test.go`/`inspect_golden_test.go`/`clock_unify_test.go` — ожидаемо зелёны
**без правок**; `TestCompleteClockInjected` (`clock_unify_test.go:167`) — перепроверить, держать
зелёным.

**Rationale**: эти замки инстанцируют процесс **по имени** (`complete`/`start`/`inspect` не зовут
`interp.Run` верхнего уровня → новый источник/метрика не вычисляются), поэтому добавление аналитики их
не задевает. **Нюанс (анкор §F-1):** `TestCompleteClockInjected` — единственный, кто реально ПРОГОНЯЕТ
новые догон-авто-шаги (несёт payload `{"итог":"перезвонит"}`, исполняет `присвоить итог = данные.итог`
+ `уведомить crm`); подтверждено живым кодом (`clock_unify_test.go:179`). Он остаётся зелёным как
побочный co-land-замок догона, но при смене **имён шагов** или **ключа payload** покраснеет (exit 1) →
перепроверить согласованность (FR-012).

**Alternatives considered**: переписывать соседей — не требуется и запрещено границей.

---

## R7. Терминальные гейты — не переписывать

**Decision**: `TestM2GoldenEndToEnd` (`src/internal/daemon/m2_endtoend_test.go`, эскалация + webhook
POST) и `TestStepEffectExactlyOnceRestart` (реальный эффект доставлен ровно 1 раз через рестарт демона)
— оставить зелёными, НЕ переписывать.

**Rationale**: предрешённая развилка владельца + FR-011/SC-006. Они уже пинят полную цепочку на
in-test источниках; репойнт на shipped-пример допустим, но НЕ требуется (анкор §F-1).

---

## R8. Точные байты выводов — снимаются с живого бинаря impl-time

**Decision**: точные строки explain и эффекта брать **с живого бинаря** на этапе реализации; в
contracts/ зафиксированы канонические формы из якорей.

**Rationale** (конституция VIII — тексты дословно, не переформулировать):
- **explain (run, без ребра)**, канон `reliability-model.md §C-5.3` (`:558`, `:579`):
  `триггер '<имя> <оп> <порог>' сработал: <имя> = <снимок> (снимок) <оп> <порог> (порог) → истина`.
  Числа через `value.String` — **без** подчёркиваний (`3000000`, снимок `300000.0`); оператор —
  `BinOp.String` (`<`). Для нашего триггера: `триггер 'выручка_30д < 3000000' сработал: выручка_30д =
  300000.0 (снимок) < 3000000 (порог) → истина`.
- **эффект**: `[уведомление] crm: итог звонка: перезвонит`.
- **скаляр** `runMetric`: `300000.0\n` (подтверждено `metric_window_golden_test.go:41`).
- **старт под run** (D-13, тихий старт процесса): печатается строка задачи `[задача] t-000001 →
  менеджер, шаг 'связаться_с_клиентом', срок до <DT>`; эскалация-триггер `когда задача просрочена` под
  `run` печатает заглушку `задача триггер 'эскалация_плана.связаться_с_клиентом' требует serve (фича
  007b)`.

**Alternatives considered**: хардкод байт «по памяти» — отвергнут, точные байты только с бинаря
(дедлайн маскируется `<DT>` для дата-независимости).
