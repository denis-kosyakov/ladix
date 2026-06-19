---
description: "Task list for feature 024-correctness-fixes"
---

# Tasks: Фиксы корректности value-пространства (6 багов)

**Input**: Design documents from `/specs/024-correctness-fixes/`

**Prerequisites**: plan.md (фазы Fix A–F + гейты), spec.md (US1–US6), research.md, data-model.md,
contracts/ (контракты 6 фиксов как exact-match)

**Якорь истины**: `ladix-sparring/handoff-024-correctness-fixes.md` — §3 (АНКЕР: точные спеки 6
фиксов A–F, авторитетно). При расхождении побеждает якорь. Plan.md дословно цитирует §3 — задачи
ниже сверены с ним.

**Tests**: Это фича **починки корректности без новой языковой функциональности**. Каждый фикс A–F —
точечная прод-правка, закреплённая **co-land** тест-замком в той же или соседней задаче (FR-016):
замок (особенно golden Фикса C) НЕ остаётся красным между задачами. Поэтому тест-задачи здесь — **сам
импл-слайс**, не опциональный TDD-оверлей.

**Organization**: задачи сгруппированы по user-story (US1=Fix C, US2=Fix A, US3=Fix B, US4=Fix D,
US5=Fix E, US6=Fix F) в порядке приоритета spec (P1→P3), затем финальный гейт + mutation-probe.
Нумерация фаз отражает приоритет анкера.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: параллелизуемо (другой файл, нет зависимостей от незавершённых задач)
- **[Story]**: к какому фиксу относится (US1=Fix C, US2=Fix A, US3=Fix B, US4=Fix D, US5=Fix E,
  US6=Fix F)
- Все пути — абсолютные от корня repo (`/Users/denis/dev/ladix`); в описаниях даны relative-от-root.

## Path Conventions

- Single project, Go-модуль в `src/`. Точечные правки в `src/internal/{eval,value,engine,jsonval}` +
  их `*_test.go`. Реестр ошибок — `specs/003-interpreter-eval/contracts/runtime-errors.md` (под
  `specs/`, не «большой док»).
- Прод-дифф строго в `internal/{eval,value,engine,jsonval}` + co-land тесты + `specs/024/**` +
  `specs/003-interpreter-eval/contracts/runtime-errors.md`. Несущие швы целы:
  `ProcessRuntime` = 8, `Store` = 18 (никаких новых методов/миграций).

---

## Phase 1: Fix C — `дробное()`/`число()` отвергают NaN/Inf/hex-float (User Story 1, Priority: P1) 🎯 MVP

**Goal**: `builtinDrobnoe`/`builtinChislo` громко отвергают NaN/Inf/hex-float через общий хелпер
`parseFiniteFloat`; +2 новых сообщения; **единственный фикс, меняющий golden** (`len(seen) 28 → 30`).

**Independent Test**: `дробное("nan")`/`дробное("+inf")`/`дробное("0x1p4")`/`число("inf")` → ошибка;
`дробное("3.5")`/`число("42")` — ок. Golden `TestErrorsRegistryExactMatch` зелёный при `len(seen)==30`.

**⚠️ КРИТИЧНО (FR-016)**: код (новые сообщения в `builtins_convert.go`) и **golden-замок**
(`errors_golden_test.go`: count `28→30` + 2 кейса в exact-match таблице + синк
`specs/003-interpreter-eval/contracts/runtime-errors.md`) обновляются в **одной задаче (T001)** —
golden не остаётся красным между задачами.

**Контракт**: `contracts/` (Фикс C exact-match, сообщения дословно). FR-005..008.

- [ ] T001 [US1] **CO-LAND Fix C (код + golden в одной задаче).**
      (а) В `src/internal/eval/builtins_convert.go` вынести общий хелпер
      `parseFiniteFloat(s string) (float64, bool)`: после `strconv.ParseFloat(strings.TrimSpace(s), 64)`
      отказывать (`false`), если `strings.ContainsAny(s, "xXpP")` (Go hex-float) ИЛИ
      `math.IsInf(f, 0) || math.IsNaN(f)`. Использовать в **обоих** — `builtinDrobnoe` (`:68`,
      `ParseFloat` на `:80`) и `builtinChislo` (`:90`, `ParseFloat` на `:101`); отказ →
      `runtimeErr(pos, <msg>)`, `pos` как у прочих ошибок этих builtins. `целое()` (ParseInt) **НЕ
      трогать**. Два новых сообщения **дословно** (`%s` = аргумент после TrimSpace):
      `дробное: «%s» не является конечным числом` · `число: «%s» не является конечным числом`.
      (б) В **той же задаче** обновить golden `src/internal/eval/errors_golden_test.go`: добавить 2 кейса
      (категория `ОшибкаВыполнения`/RT-*) в exact-match таблицу `TestErrorsRegistryExactMatch`;
      count-замок `if len(seen) != 28` → `!= 30` (`:205-207`, текст «хотим 28» → «хотим 30»).
      (в) В **той же задаче** синкнуть реестр `specs/003-interpreter-eval/contracts/runtime-errors.md`:
      +2 сообщения дословно.
      **Критерий приёмки** (FR-005/006/007): `go test ./internal/eval/ -run TestErrorsRegistryExactMatch`
      из `src/` зелёный при `len(seen)==30`; оба новых сообщения присутствуют в таблице и в реестре;
      `целое()` не затронут.
- [ ] T002 [US1] Co-land поведенческий замок: в `src/internal/eval/builtins_test.go` добавить
      **`TestConvertNonFinite`** — `дробное("nan")`/`дробное("+inf")`/`дробное("0x1p4")`/
      `число("inf")` → ошибка (сообщения дословно из T001, NaN/Inf через содержимое строки-аргумента);
      `дробное("3.5")` → `Дробное{3.5}`, `число("42")` → `Целое{42}` — ок (happy-path не сломан).
      **Критерий приёмки** (FR-008): замок зелёный; отказ нефинитных и hex-float наблюдаем,
      штатные конверсии целы.

**Checkpoint Fix C**: NaN/Inf/hex-float отвергаются громко; golden синхронен (28→30), реестр синкнут,
happy-path цел. Единственный пользовательски-видимый фикс закрыт.

---

## Phase 2: Fix A — идемпотентность создания задачи при рестарте serve (User Story 2, Priority: P1)

**Goal**: query-guard в `advance` не даёт минтить дубль человеческой задачи на (instance, step) при
реактивации инстанса (рестарт демона). Новых методов Store нет — `Store` = 18 цел.

**Independent Test**: реактивировать `StatusRunning`-инстанс с уже сохранённой `TaskPending` на
человеческом шаге → `len(st.ListPendingTasks("")) == 1` (не `2`), id неизменен.

**Dependencies**: независим от Fix C (другой пакет). [P] относительно остальных фиксов.

**Контракт**: `contracts/` (Фикс A). FR-001..002. Partial-unique-индекс ОТВЕРГНУТ (без миграции/бампа
схемы; Store = 18).

- [ ] T003 [P] [US2] В `src/internal/engine/engine.go` — `advance` (`:255`), блок минта задачи
      (`:310-332`, человеческий шаг `hasAssignee`): добавить query-guard перед `e.st.NextTaskID()` —
      искать существующую открытую задачу на текущем (instance, step) через существующий
      `e.st.ListPendingTasks(...)`, условие `t.Status == store.TaskPending && t.StepName ==
      step.Name.Name`. Найдена → **НЕ** минтить id, **НЕ** `SaveTask`, **НЕ** `printTaskCreated`;
      выставить `inst.Status = store.StatusWaiting`, `e.save(inst)`, вернуть `nil`. Не найдена → текущий
      путь (mint + SaveTask + печать + save). Новых методов Store **не добавлять** (`ListPendingTasks`
      существует). **Критерий приёмки** (FR-001): дубль не минтится при наличии Pending на (inst, step);
      `Store` = 18 цел.
- [ ] T004 [US2] Co-land замок: создать НОВЫЙ файл `src/internal/engine/reactivate_test.go` с
      **`TestReactivateInstanceIdempotentTask`** — предзалить `StatusRunning`-инстанс с уже сохранённой
      `TaskPending` на человеческом шаге; вызвать `ReactivateInstance`; ассертить
      `len(st.ListPendingTasks("")) == 1` (не `2`) + неизменность id задачи.
      **Критерий приёмки** (FR-002): замок зелёный; реактивация не дублирует задачу.

**Checkpoint Fix A**: рестарт serve не плодит дублей человеческих задач; query-guard читает
существующий store-метод, несущий шов Store байт-цел.

---

## Phase 3: Fix B — `Compare` трактует NaN как несравнимый (User Story 3, Priority: P2)

**Goal**: `value.Compare` объявляет NaN несравнимым (`(0, false)`); следствие — `сортировать`/`мин`/
`макс` со значением-NaN поднимают существующую typeErr «несравнимы». `cmpFloat` НЕ трогать.

**Independent Test**: `Compare(Дробное{NaN}, Дробное{5}) → (_, false)`; `сортировать`/`мин` списка с
NaN → ошибка «несравнимы».

**Dependencies**: независим (пакет `value`, листовой). [P].

**Контракт**: `contracts/` (Фикс B). FR-003..004.

- [ ] T005 [P] [US3] В `src/internal/value/equal.go` — `Compare` (`:91`, float-ветки `:98/:103/:105`):
      перед каждым вызовом `cmpFloat` на float-аргументах добавить guard
      `if math.IsNaN(af) || math.IsNaN(bf) { return 0, false }` (несравнимы). `cmpFloat` (`:159`) **НЕ
      трогать**; `Inf` **НЕ трогать**; `==`/`!=` (через `value.Equal`) **НЕ трогать**.
      **Критерий приёмки** (FR-003): `Compare` с NaN-аргументом возвращает `(0, false)` во всех
      float-ветках; не-NaN сравнения целы.
- [ ] T006 [US3] Co-land замки: в `src/internal/value/equal_test.go` добавить (в/рядом с `TestCompare`)
      **`TestCompareNaN`** — `Compare(Дробное{NaN}, Дробное{5}) → (_, false)` (NaN через `math.NaN()`);
      плюс eval-уровень — `src/internal/eval/builtins_test.go` **`TestSortMinNaNIncomparable`**:
      `сортировать`/`мин` списка с NaN → ошибка «несравнимы» (через `ok==false`). Eval-замок реализован
      **белым ящиком** (NaN сконструирован напрямую в Go-значениях, не из `.ladix`-исходника), потому что
      после фиксов C/D NaN **недостижим** из пользовательского кода (`дробное`/`число` и CSV-загрузчик
      отвергают `Inf`/`NaN`; нет `sqrt`/`log`/степени; деление-на-ноль — ошибка) — это согласуется с
      примечанием анкера «Радиус»: фикс B — defense-in-depth. **Критерий приёмки** (FR-004): замки
      зелёные; NaN несравним, агрегаты краснеют штатной typeErr.

**Checkpoint Fix B**: недетерминированный порядок при NaN устранён; defense-in-depth для последствий
float→NaN; существующие сравнения не регрессируют.

---

## Phase 4: Fix D — jsonval сохраняет число вне float64 как `Дробное{±Inf}` (User Story 4, Priority: P2)

**Goal**: `numberToValue` на ErrRange возвращает `Дробное{±Inf}` вместо `None` (толерантный контракт
payload: деградация лучше сбоя доставки). Store-codec round-trip `±Inf/NaN` уже держит.

**Independent Test**: `1e400` → `Дробное{+Inf}`, `-1e400` → `Дробное{-Inf}`.

**Dependencies**: независим (пакет `jsonval`, автономен). [P].

**Контракт**: `contracts/` (Фикс D). FR-009..010.

- [ ] T007 [P] [US4] В `src/internal/jsonval/decode.go` — `numberToValue` (`:135`), ветка `Float64()`
      (`:142`): на `Float64()`-ErrRange (`f` уже `±Inf`) вернуть `value.Дробное{V: f}`
      (`f, _ := n.Float64(); return value.Дробное{V: f}`) вместо текущего `value.None`. Для чисел ветка
      `None` становится недостижима. **Критерий приёмки** (FR-009): число вне float64 даёт
      `Дробное{±Inf}`, не `None`; без новых ошибок (толерантно).
- [ ] T008 [US4] Co-land замок: в `src/internal/jsonval/decode_test.go` добавить
      **`TestDecodeValueFloatOverflow`** — `1e400` → `Дробное{+Inf}`, `-1e400` → `Дробное{-Inf}` (проверка
      через `math.IsInf`). **Критерий приёмки** (FR-010): замок зелёный; ±Inf сохраняется, не теряется.

**Checkpoint Fix D**: out-of-range число payload больше не молча теряется как None; round-trip держит
±Inf.

---

## Phase 5: Fix E — `сумма()` промоутится в Дробное вместо ложного overflow (User Story 5, Priority: P2)

**Goal**: `builtinSumma` гейтит overflow-гард целого аккумулятора за наличием `Дробное` (`hasFloat`):
смешанный список с float суммируется во `fSum` без int-гарда → `Дробное`; all-int путь сохраняет
существующий overflow-гард (RT-OVERFLOW).

**Independent Test**: `сумма([MaxInt64, MaxInt64, 1.5])` → `Дробное` (≈1.8e19), не ошибка; регресс
`сумма([MaxInt64, 1])` (all-int) → по-прежнему «переполнение целого числа».

**Dependencies**: независим (пакет `eval`, файл `builtins_aggregate.go`). [P] относительно Fix C
(другой файл; но замок T010 — в `builtins_test.go`, см. ниже).

**Контракт**: `contracts/` (Фикс E). FR-011..012.

- [ ] T009 [P] [US5] В `src/internal/eval/builtins_aggregate.go` — `builtinSumma` (`:18`): предсканировать
      список на присутствие `Дробное` (`hasFloat`); при наличии — суммировать во `fSum` **без** int-гарда
      → `Дробное`; все `Целые` → текущий int-путь с overflow-гардом (RT-OVERFLOW). Чисто-целый путь
      сохраняет существующую ошибку переполнения. **Критерий приёмки** (FR-011): смешанный список с float
      не поднимает ложный overflow; all-int overflow цел.
- [ ] T010 [US5] Co-land замок: в `src/internal/eval/builtins_test.go` добавить
      **`TestSummaOverflowGatedByFloat`** — `сумма([9223372036854775807, 9223372036854775807, 1.5])`
      → `Дробное` (≈1.8e19), не ошибка; регресс `сумма([9223372036854775807, 1])` (all-int) →
      по-прежнему «переполнение целого числа». **Критерий приёмки** (FR-012): замок зелёный; promote и
      all-int-overflow оба наблюдаемы.

**Checkpoint Fix E**: ложное переполнение `сумма` на смешанных списках устранено; all-int overflow-гард
сохранён.

---

## Phase 6: Fix F — `MinInt64 // -1` поднимает overflow вместо wraparound (User Story 6, Priority: P3)

**Goal**: `floorDivInt64` меняет сигнатуру на `(int64, bool)` (bool=overflow), зеркально `mulInt64`:
`(0, true)` при `a == math.MinInt64 && b == -1`; `evalFloorDiv` на overflow → `runtimeErr(pos,
"переполнение целого числа")` (тот же текст/категория RT-OVERFLOW, что у `*`). Деление-на-ноль как есть.

**Independent Test**: выражение, дающее `MinInt64 // -1` → «переполнение целого числа»; `7 // 2 → 3`
не сломан; деление-на-ноль не тронуто.

**Dependencies**: независим (пакет `eval`, файл `arith.go`). [P] относительно Fix C/E (другой файл;
замок T012 — в `expr_test.go`).

**Контракт**: `contracts/` (Фикс F). FR-013..014.

- [ ] T011 [P] [US6] В `src/internal/eval/arith.go` — `floorDivInt64` (`:277`): сменить сигнатуру на
      `(int64, bool)`, зеркально `mulInt64` (`:259`); возвращать `(0, true)` при
      `a == math.MinInt64 && b == -1`. `evalFloorDiv` (`:171`, call-site `:180`) при overflow →
      `runtimeErr(pos, "переполнение целого числа")` (тот же текст/категория RT-OVERFLOW, что у `*`).
      Деление-на-ноль оставить как есть. Доп. call-site `metric_engine.go:256` использует тот же
      `evalFloorDiv` — правки не требует. **Критерий приёмки** (FR-013): `MinInt64 // -1` поднимает
      RT-OVERFLOW; happy-path/деление-на-ноль целы; новых кодов нет (RT-OVERFLOW переиспользован).
- [ ] T012 [US6] Co-land замок: в `src/internal/eval/expr_test.go` добавить
      **`TestFloorDivOverflowEdge`** — выражение, дающее `MinInt64 // -1` (`MinInt64` через
      `(0 - 9223372036854775807 - 1)`) → «переполнение целого числа»; happy-path `7 // 2 → 3` не ломать;
      деление-на-ноль не тронуто. **Критерий приёмки** (FR-014): замок зелёный; overflow на
      `MinInt64 // -1` наблюдаем, штатное деление цело.

**Checkpoint Fix F**: тихий wraparound `MinInt64 // -1` заменён громким RT-OVERFLOW; зеркало `mulInt64`,
без новых кодов.

---

## Phase 7: Финальный гейт + mutation-probe (Polish & Cross-Cutting)

**Purpose**: полный набор сборки/анализа/форматирования/тестов exit 0; доказать, что КАЖДЫЙ из 6
замков кусается при инверсии своего фикса (FR-015); подтвердить чистоту дерева и сохранность несущих
швов.

**Dependencies**: после Phase 1–6 (все 6 фиксов + co-land замки на месте).

- [ ] T013 **Финальный гейт (FR-019).** Из `src/`: `go build ./...`, `go vet ./...`, `gofmt -l .`
      (вывод пуст), `go test ./... -count=1` (все пакеты ok). **Критерий приёмки**: все exit 0;
      `gofmt -l` ничего не печатает; рабочее дерево чистое — сборочный бинарь (`src/ladix`) и файлы БД
      не закоммичены (`.gitignore`).
- [ ] T014 Верифицировать сохранность несущих швов: `ProcessRuntime` = 8 и `Store` = 18 целы
      (compile/codec-замки зелёные без правок; никаких новых методов Store/миграций — Fix A читает
      существующий `ListPendingTasks`). **Критерий приёмки** (FR-001, плановый инвариант): прод-дифф
      строго в `internal/{eval,value,engine,jsonval}` + co-land тесты + `specs/024/**` +
      `specs/003-interpreter-eval/contracts/runtime-errors.md`; 0 новых KW/кодов ошибок (кроме 2
      сообщений Fix C под существующим `ОшибкаВыполнения`)/builtins/зависимостей.
- [ ] T015 **Mutation-probe всех 6 замков (FR-015).** В одноразовом scratch-worktree по очереди
      откатить каждый фикс → соответствующий замок краснеет, затем вернуть:
      (1) Fix C → `TestConvertNonFinite` (+ golden `TestErrorsRegistryExactMatch`);
      (2) Fix A → `TestReactivateInstanceIdempotentTask`;
      (3) Fix B → `TestCompareNaN` + `TestSortMinNaNIncomparable`;
      (4) Fix D → `TestDecodeValueFloatOverflow`;
      (5) Fix E → `TestSummaOverflowGatedByFloat`;
      (6) Fix F → `TestFloorDivOverflowEdge`.
      **Критерий приёмки** (FR-015): каждый замок доказуемо краснеет при инверсии своего фикса; не
      краснеет → усилить замок и повторить.

**Checkpoint Phase 7**: гейты зелёные, швы целы, все 6 замков доказуемо кусаются → фича готова к
ревью.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Fix C)**: нет зависимостей — старт немедленно (MVP, единственный пользовательски-видимый
  фикс).
- **Phase 2 (Fix A)**: независим — другой пакет (`engine`). [P] относительно прочих.
- **Phase 3 (Fix B)**: независим — пакет `value` (листовой). [P].
- **Phase 4 (Fix D)**: независим — пакет `jsonval` (автономен). [P].
- **Phase 5 (Fix E)**: независим — `eval/builtins_aggregate.go`. [P] (замок в `builtins_test.go`).
- **Phase 6 (Fix F)**: независим — `eval/arith.go`. [P] (замок в `expr_test.go`).
- **Phase 7 (гейт + mutation-probe)**: depends on Phase 1–6.

### Внутри каждого фикса (co-land дисциплина FR-016)

- Fix C: T001 (код + golden + реестр **в одной задаче**, golden не красный) → T002 (поведенческий замок).
- Fix A: T003 (query-guard) → T004 (НОВЫЙ замок-файл).
- Fix B: T005 (NaN-guard) → T006 (замок).
- Fix D: T007 (Inf-сохранение) → T008 (замок).
- Fix E: T009 (hasFloat-гейт) → T010 (замок).
- Fix F: T011 (сигнатура + RT-OVERFLOW) → T012 (замок).

### Parallel Opportunities

- Шесть фиксов A–F затрагивают разные файлы/пакеты — фазы 2–6 параллелизуемы относительно фазы 1 и
  друг друга ([P] на головных задачах T003/T005/T007/T009/T011). Внутри фикса код→замок
  последовательны (co-land).
- Fix C (T001) — критический путь golden (затрагивает `errors_golden_test.go` + реестр), ведётся
  атомарно.

---

## Parallel Example: фиксы A/B/D после старта C

```bash
# Из src/ — независимые пакеты, головные правки параллельно:
# T003 (Fix A): internal/engine/engine.go
# T005 (Fix B): internal/value/equal.go
# T007 (Fix D): internal/jsonval/decode.go
go test ./internal/engine/ -run TestReactivateInstanceIdempotentTask
go test ./internal/value/ -run TestCompareNaN
go test ./internal/eval/ -run TestSortMinNaNIncomparable
go test ./internal/jsonval/ -run TestDecodeValueFloatOverflow
```

## Parallel Example: mutation-probe (Phase 7)

```bash
# В scratch-worktree по очереди откатить фикс → замок краснит → revert:
# Fix C → TestConvertNonFinite + TestErrorsRegistryExactMatch
# Fix A → TestReactivateInstanceIdempotentTask
# Fix B → TestCompareNaN + TestSortMinNaNIncomparable
# Fix D → TestDecodeValueFloatOverflow
# Fix E → TestSummaOverflowGatedByFloat
# Fix F → TestFloorDivOverflowEdge
```

---

## Implementation Strategy

### MVP First (Fix C / US1)

1. Phase 1 (Fix C): `parseFiniteFloat` + 2 сообщения + golden `28→30` + реестр — атомарно (T001), затем
   поведенческий замок (T002).
2. **STOP and VALIDATE**: NaN/Inf/hex-float отвергаются громко; golden синхронен. Единственный
   пользовательски-видимый фикс закрыт.

### Incremental Delivery

1. Fix C → громкий отказ нефинитных (MVP, golden 28→30).
2. Fix A → идемпотентность задачи при рестарте serve.
3. Fix B → NaN несравним (детерминизм порядка).
4. Fix D → out-of-range payload как ±Inf, не None.
5. Fix E → `сумма` промоут в Дробное, без ложного overflow.
6. Fix F → `MinInt64 // -1` → RT-OVERFLOW, без wraparound.
7. Финальный гейт + mutation-probe (все 6 замков кусаются).

---

## Notes / Границы (FR-015/016/017/018, Constitution 9/9)

- **Прод-дифф точечный**: строго в `internal/{eval,value,engine,jsonval}` + co-land тесты + `specs/024/**`
  + `specs/003-interpreter-eval/contracts/runtime-errors.md`. ПУСТОЙ прод-дифф в `internal/store`,
  `internal/daemon`, не-тестовом CLI-коде.
- **Несущие швы целы**: `ProcessRuntime` = 8, `Store` = 18 (T014). Никаких новых методов Store/миграций
  (partial-unique-индекс ОТВЕРГНУТ, FR-001).
- **0 новых** KW / кодов ошибок (SE/L/eval) / встроенных функций / зависимостей — **кроме 2 сообщений
  Fix C** (оба под существующим `ОшибкаВыполнения`, рост каталога 28→30, FR-017).
- **co-land golden (FR-016)**: golden `errors_golden_test.go` (count `28→30` + 2 кейса + синк реестра
  `runtime-errors.md`) обновляется **в той же задаче T001**, что и сами сообщения в
  `builtins_convert.go`. Ни одна задача не оставляет тест-замок (особенно golden) красным между задачами.
- **Граница доков (OUT, FR-018)**: **НЕ трогать** `SPEC.md`, `ARCHITECTURE.md`, `grammar.md`,
  `README.md`, `docs/eval-model.md`, `docs/engine-model.md`, `docs/automation-model.md`,
  `docs/stdlib.md` — синк делает архитектор отдельно после мержа. Под `specs/` синкается только реестр
  `specs/003-interpreter-eval/contracts/runtime-errors.md` (Fix C).
- **Не-A–F находки**: если найдётся «ещё баг» вне A–F — НЕ чинить, только записать в отчёт.
- **Детерминизм** обязателен: NaN/Inf через `math.NaN()`/`math.Inf()`; mutation-probe в одноразовом
  scratch-worktree.
- [P] = разные файлы/пакеты, нет зависимостей. [Story] = трассировка US1=Fix C … US6=Fix F.
