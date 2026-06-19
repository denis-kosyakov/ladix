# Implementation Plan: Фиксы корректности value-пространства (6 багов)

**Branch**: `024-correctness-fixes` | **Date**: 2026-06-19 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/024-correctness-fixes/spec.md`

**Якорь истины**: `ladix-sparring/handoff-024-correctness-fixes.md` — **§2** (граница: что трогаем / что
НЕ трогаем), **§3** (АНКЕР: точные спеки 6 фиксов A–F, авторитетно), **§4** (скоуп). При любом
расхождении побеждает якорь. Эта фича реализует **только** 6 фиксов A–F и их co-land тест-замки; синк
больших доков (`SPEC.md`, `docs/*-model.md`, `stdlib.md` и пр.) делает архитектор отдельно (см.
«Границы» и FR-018).

## Summary

Починка корректности **без новой языковой функциональности**: шесть предсуществующих логических багов
value-пространства (молчаливая потеря/искажение значений, недетерминированный порядок при NaN, дубль
задачи при рестарте демона, ложное переполнение, тихий wraparound) устраняются точечными правками по
анкеру, каждая закреплена co-land тест-замком, кусающимся при инверсии (mutation-probe, FR-015). Только
**Фикс C** добавляет наблюдаемое поведение: две новые ошибки + golden-дельта `28 → 30` (FR-006/FR-007);
остальные пять переиспользуют существующие сообщения / не вводят ошибок и golden-счётчик не меняют.

Технический подход (6 точечных правок, каждая с co-land замком; нумерация по приоритету spec):
- **Фикс C** (P1, `eval/builtins_convert.go`) — `builtinDrobnoe`/`builtinChislo` отвергают
  NaN/Inf/hex-float через общий хелпер `parseFiniteFloat`; +2 сообщения; golden `28 → 30`.
- **Фикс A** (P1, `engine/engine.go`) — идемпотентность минта задачи в `advance` через query-guard по
  `TaskPending` на (instance, step).
- **Фикс B** (P2, `value/equal.go`) — `Compare` объявляет NaN несравнимым (`(0, false)`).
- **Фикс D** (P2, `jsonval/decode.go`) — `numberToValue` сохраняет число вне float64 как `Дробное{±Inf}`,
  не `None`.
- **Фикс E** (P2, `eval/builtins_aggregate.go`) — `сумма` гейтит overflow-гард за наличием `Дробное`.
- **Фикс F** (P3, `eval/arith.go`) — `floorDivInt64`/`evalFloorDiv` поднимают RT-OVERFLOW на
  `MinInt64 // -1`.

Прод-дифф строго в `internal/{eval,value,engine,jsonval}` + co-land тесты + `specs/024/**` +
`specs/003-interpreter-eval/contracts/runtime-errors.md`. Несущие швы (`ProcessRuntime`, `Store`-методы)
целы. Большие доки НЕ трогаются (FR-018).

## Technical Context

**Language/Version**: Go 1.25, без CGO (конституция I); go-модуль в `src/`.

**Primary Dependencies**: только `modernc.org/sqlite` (чистый Go). **0 новых зависимостей** (FR-017).

**Storage**: SQLite через `internal/store` (Memory + SQLite impl). Фикс A читает `ListPendingTasks`
(существующий метод) и `SaveTask`/`NextTaskID` — **новых методов Store нет**, несущий шов `Store`
байт-цел. Фикс D полагается на уже работающий store-codec round-trip `±Inf/NaN` (ничего не добавляет).

**Testing**: `go test` (стандартный); golden/exact-match реестр ошибок —
`src/internal/eval/errors_golden_test.go` (`TestErrorsRegistryExactMatch`, count-замок `len(seen)`,
`:205`). Co-land замки в пакетах правок. Детерминизм: NaN/Inf через `math.NaN()`/`math.Inf()`; mutation-
probe в одноразовом scratch-worktree (FR-015).

**Target Platform**: один статический бинарник `ladix` (конституция I), кросс-платформенный CLI.

**Project Type**: компилятор/интерпретатор DSL — single project, библиотечные пакеты `internal/*`.

**Performance Goals**: N/A — точечные правки горячих путей без изменения сложности; цель —
детерминированные замки.

**Constraints**:
- **Прод-дифф точечный** — строго в `internal/eval`, `internal/value`, `internal/engine`,
  `internal/jsonval` (по анкеру) + co-land тесты + `specs/024/**` + `specs/003 contracts/runtime-errors.md`.
- **Несущие швы целы**: `ProcessRuntime` = **8** методов; `Store` = **18** (никаких новых методов/
  миграций — partial-unique-индекс ОТВЕРГНУТ, FR-001).
- **0 новых** ключевых слов / кодов ошибок (SE/L/eval-кодов) / встроенных функций / зависимостей —
  **кроме двух сообщений Фикса C** (FR-017).
- **Детерминизм** обязателен; все замки воспроизводимы.
- **Граница доков (OUT, FR-018)**: НЕ трогать `SPEC.md`, `ARCHITECTURE.md`, `grammar.md`, `README.md`,
  `docs/eval-model.md`, `docs/engine-model.md`, `docs/automation-model.md`, `docs/stdlib.md`.

**Scale/Scope**: 6 точечных прод-правок в 6 файлах + 6 co-land тест-замков; 1 golden-обновление (Фикс C);
1 синк реестра под `specs/`.

## Constitution Check

*GATE: проверено до Phase 0 и переоценено после Phase 1.*

| # | Принцип | Вердикт | Обоснование |
|---|---------|---------|-------------|
| I | Язык и сборка | ✅ PASS | Go 1.25, без CGO, один бинарь; 0 новых зависимостей (FR-017); `gofmt -l .`/`go vet` зелёные (FR-019). |
| II | Парсинг — ручной | ✅ PASS | Парсер/лексер не трогаются; все 6 фиксов — в eval/value/engine/jsonval, ниже фронтенда. |
| III | Ошибки — явные типы | ✅ PASS | Новых **типов** ошибок нет: Фикс C поднимает существующий `ОшибкаВыполнения` (`runtimeErr`); B/E/F переиспользуют существующие typeErr/RT-OVERFLOW; D толерантен (без ошибок). |
| IV | Позиции — сквозные | ✅ PASS | Новые сообщения Фикса C несут `pos` как прочие ошибки этих builtins; Фикс F — `pos` из `evalFloorDiv`; позиционирование не регрессирует. |
| V | Без глобального состояния | ✅ PASS | Все правки локальны функциям; глобалов/синглтонов не вводим. Фикс A читает store явно через `e.st`. |
| VI | Тесты — вперёд | ✅ PASS | Каждый фикс co-land с тест-замком, доказуемо кусающимся при инверсии (FR-015 mutation-probe); golden `28 → 30` co-land в той же правке (FR-016). |
| VII | Раскладка проекта | ✅ PASS | Файлы строго в `src/internal/{eval,value,engine,jsonval}` + их `*_test.go`; граф зависимостей не меняется (eval не импортирует store; jsonval автономен). |
| VIII | Язык сообщений §13 | ✅ PASS | Два новых сообщения Фикса C — на русском, бизнес-формой «`дробное: «%s» не является конечным числом`» (FR-006), дословно; деление-на-ноль/overflow тексты не переформулируются. |
| IX | Спека — источник истины | ✅ PASS | Якорь §3 (фиксы A–F) → spec.md → этот plan; реестр `specs/003 contracts/runtime-errors.md` синкается с 2 новыми сообщениями в той же правке (FR-007); 0 открытых кларификаций. |

**Итог: 9/9 PASS.** Отклонений нет → Complexity Tracking ниже не несёт нарушений.

**Примечание о каталоге ошибок (не отклонение):** Фикс C добавляет **ровно 2** сообщения об ошибке
(`дробное: …`, `число: …`), что растит count-замок реестра `28 → 30`. Это санкционированный рост (он —
наблюдаемый контракт US1, единственный пользовательски-видимый фикс), а НЕ нарушение «без новых кодов»:
новых **категорий/кодов** ошибок нет, оба сообщения — категории `ОшибкаВыполнения` (существующий тип).
explain-однострочников эта фича НЕ вводит (это контекст фич 022/023, здесь неприменим). Запись внесена
сюда для прозрачности дельты каталога.

## Project Structure

### Documentation (this feature)

```text
specs/024-correctness-fixes/
├── plan.md              # Этот файл (/speckit-plan)
├── research.md          # Phase 0: подтверждение анкер-локаций; ParseFloat-поведение NaN/Inf/hex; контракт-сплит C↔D
├── data-model.md        # Phase 1: затронутые сущности (реестр ошибок; дробное/число/сумма; Compare/cmpFloat; numberToValue; advance/Task; floorDiv)
├── quickstart.md        # Phase 1: как прогнать 6 замков + mutation-probe в scratch-worktree
├── contracts/           # Phase 1: контракты 6 фиксов как exact-match (вход→выход, сообщения C дословно)
└── tasks.md             # Phase 2 (/speckit-tasks — НЕ создаётся этой командой)
```

### Source Code (repository root)

Затрагиваемые файлы строго по анкеру §3 (всё внутри `src/`-модуля):

```text
src/internal/eval/
├── builtins_convert.go     # Фикс C: parseFiniteFloat + builtinDrobnoe(:68)/builtinChislo(:90); отказ hex-float/Inf/NaN
├── builtins_aggregate.go   # Фикс E: builtinSumma(:18) — гейт overflow-гарда за hasFloat
├── arith.go                # Фикс F: floorDivInt64(:277)→(int64,bool); evalFloorDiv(:171) поднимает RT-OVERFLOW
├── errors_golden_test.go   # Фикс C co-land: +2 кейса в exact-match; count-замок 28→30 (:205)
├── builtins_test.go        # Фикс C/E замки: дробное/число отказ + сумма-promote
└── expr_test.go            # Фикс F замок: MinInt64 // -1 → переполнение

src/internal/value/
├── equal.go                # Фикс B: Compare(:91) перед cmpFloat — NaN-guard → (0,false); cmpFloat(:159) НЕ трогать
└── equal_test.go           # Фикс B co-land: TestCompare NaN→(_,false) + eval-уровень сортировать/мин

src/internal/jsonval/
├── decode.go               # Фикс D: numberToValue(:135) — ErrRange → Дробное{±Inf}, не None (:142 ветка)
└── decode_test.go          # Фикс D co-land: 1e400→Дробное{+Inf}, -1e400→Дробное{-Inf}

src/internal/engine/
├── engine.go               # Фикс A: advance(:255) — query-guard по TaskPending до NextTaskID/SaveTask(:310-329)
└── reactivate_test.go      # Фикс A co-land: НОВЫЙ файл — реактивация не дублирует задачу (len==1)

specs/003-interpreter-eval/
└── contracts/runtime-errors.md  # Фикс C синк реестра: +2 сообщения (под specs/, не «большой док»)
```

**Несущие швы (compile-замки должны остаться зелёными без правок):** `ProcessRuntime` = 8;
`Store` = 18 (Фикс A новых методов не добавляет — query-guard читает существующий `ListPendingTasks`).

**Structure Decision**: single project, Go-модуль в `src/`. Шесть независимых точек правки в четырёх
библиотечных пакетах `internal/*`; каждая правка локальна одной-двум функциям. Граф импортов неизменен:
`eval` не импортирует `store`, `jsonval` автономен, `value` — листовой пакет.

## Phases (рабочий план реализации — для /speckit-tasks)

Порядок фаз — по приоритету spec (P1→P3). Каждая фаза = прод-правка + co-land замок в одной задаче
(FR-016 — golden никогда не красный между шагами).

### Fix C — `дробное()`/`число()` отвергают NaN/Inf/hex-float (US1, P1; FR-005..008)
- **Файлы:** `eval/builtins_convert.go` — `builtinDrobnoe` (`:68`, `ParseFloat` на `:80`),
  `builtinChislo` (`:90`, `ParseFloat` на `:101`).
- **Подход:** вынести общий хелпер `parseFiniteFloat(s string) (float64, bool)`: после
  `strconv.ParseFloat(strings.TrimSpace(s), 64)` отказывать, если `strings.ContainsAny(s, "xXpP")`
  (Go hex-float) ИЛИ `math.IsInf(f, 0) || math.IsNaN(f)`. Использовать в **обоих** builtins; отказ →
  `runtimeErr(pos, <msg>)`, `pos` — как у прочих ошибок этих builtins. `целое()` (ParseInt) НЕ трогать.
- **2 новых сообщения дословно** (`%s` = аргумент после TrimSpace):
  `дробное: «%s» не является конечным числом` · `число: «%s» не является конечным числом`.
- **Влияние на golden:** `errors_golden_test.go` — +2 кейса (категория `ОшибкаВыполнения`/RT-*) в
  exact-match таблицу; count-замок `len(seen) != 28` → `len(seen) != 30` (`:205-207`). Синк
  `specs/003-interpreter-eval/contracts/runtime-errors.md` в той же правке. **Единственный фикс,
  меняющий golden (28 → 30).**
- **Тест-замок:** `eval/builtins_test.go` — **`TestConvertNonFinite`**: `дробное("nan")`/
  `дробное("+inf")`/`дробное("0x1p4")`/`число("inf")` → ошибка; `дробное("3.5")`/`число("42")` — ок.

### Fix A — идемпотентность создания задачи при рестарте serve (US2, P1; FR-001..002)
- **Файлы:** `engine/engine.go` — `advance` (`:255`), блок минта задачи `:310-332` (человеческий шаг
  `hasAssignee`).
- **Подход:** query-guard перед `e.st.NextTaskID()`: искать существующую открытую задачу на текущем
  (instance, step) — `t.Status == store.TaskPending && t.StepName == step.Name.Name` (через
  существующий `e.st.ListPendingTasks(...)`). Найдена → НЕ минтить id, НЕ `SaveTask`, НЕ
  `printTaskCreated`; выставить `inst.Status = store.StatusWaiting`, `e.save(inst)`, вернуть `nil`. Не
  найдена → текущий путь (mint + SaveTask + печать + save). Partial-unique-индекс ОТВЕРГНУТ (без
  миграции/бампа схемы; Store = 18 цел). Линейность v1-процессов → `Pending` на (inst, CurrentStep)
  однозначна (edge case spec).
- **Влияние на golden:** нет (новых сообщений/кодов не вводит; счётчик не трогает).
- **Тест-замок:** `engine/reactivate_test.go` (НОВЫЙ) — **`TestReactivateInstanceIdempotentTask`**: предзалить
  `StatusRunning`-инстанс с уже сохранённой `TaskPending` на человеческом шаге; вызвать
  `ReactivateInstance`; ассертить `len(st.ListPendingTasks("")) == 1` (не `2`) + неизменность id.

### Fix B — `Compare` трактует NaN как несравнимый (US3, P2; FR-003..004)
- **Файлы:** `value/equal.go` — `Compare` (`:91`, float-ветки `:98/:103/:105`); `cmpFloat` (`:159`)
  НЕ трогать.
- **Подход:** перед каждым вызовом `cmpFloat` на float-аргументах добавить guard
  `if math.IsNaN(af) || math.IsNaN(bf) { return 0, false }` (несравнимы). `Inf` НЕ трогать. Следствие
  (намеренное): `сортировать`/`мин`/`макс` со значением-NaN поднимают существующую typeErr «несравнимы»
  через `ok==false`. `==`/`!=` (через `value.Equal`) НЕ трогать.
- **Влияние на golden:** нет.
- **Тест-замки:** `value/equal_test.go` — **`TestCompareNaN`** (в/рядом с `TestCompare`):
  `Compare(Дробное{NaN}, Дробное{5}) → (_, false)` (NaN через `math.NaN()`); плюс eval-уровень —
  `eval/builtins_test.go` — **`TestSortMinNaNIncomparable`**: `сортировать`/`мин` списка с NaN →
  ошибка «несравнимы». Eval-замок реализован **белым ящиком** (NaN сконструирован напрямую в
  Go-значениях, не из `.ladix`-исходника), потому что после фиксов C/D NaN **недостижим** из
  пользовательского кода (`дробное`/`число` и CSV-загрузчик отвергают `Inf`/`NaN`; нет
  `sqrt`/`log`/степени; деление-на-ноль — ошибка). Это согласуется с примечанием анкера «Радиус»:
  фикс B — defense-in-depth.

### Fix D — jsonval сохраняет число вне float64 как `Дробное{±Inf}` (US4, P2; FR-009..010)
- **Файлы:** `jsonval/decode.go` — `numberToValue` (`:135`), ветка `Float64()` (`:142`).
- **Подход:** на `Float64()`-ErrRange `f` уже = `±Inf` → вернуть `value.Дробное{V: f}`
  (`f, _ := n.Float64(); return value.Дробное{V: f}`) вместо текущего `value.None`. Толерантный контракт
  payload (деградация лучше сбоя доставки); store-codec round-trip `±Inf/NaN` уже держит. Для чисел
  ветка `None` становится недостижима.
- **Влияние на golden:** нет (толерантно, без ошибок).
- **Тест-замок:** `jsonval/decode_test.go` — **`TestDecodeValueFloatOverflow`**: `1e400` →
  `Дробное{+Inf}`, `-1e400` → `Дробное{-Inf}` (через `math.IsInf`).

### Fix E — `сумма()` промоутится в Дробное вместо ложного overflow (US5, P2; FR-011..012)
- **Файлы:** `eval/builtins_aggregate.go` — `builtinSumma` (`:18`).
- **Подход:** гейтить overflow-гард целого аккумулятора за наличием `Дробное` (`hasFloat`):
  предсканировать список на присутствие `Дробное`; при наличии — суммировать во `fSum` БЕЗ int-гарда →
  `Дробное`; все Целые → текущий int-путь с overflow-гардом (RT-OVERFLOW). Чисто-целый путь сохраняет
  существующую ошибку (edge case spec).
- **Влияние на golden:** нет (overflow только на all-int пути).
- **Тест-замок:** `eval/builtins_test.go` — **`TestSummaOverflowGatedByFloat`**:
  `сумма([9223372036854775807, 9223372036854775807, 1.5])` → `Дробное` (≈1.8e19), не ошибка; регресс
  `сумма([9223372036854775807, 1])` (all-int) → по-прежнему «переполнение целого числа».

### Fix F — `MinInt64 // -1` поднимает overflow вместо wraparound (US6, P3; FR-013..014)
- **Файлы:** `eval/arith.go` — `floorDivInt64` (`:277`), `evalFloorDiv` (`:171`); зеркало `mulInt64`
  (`:259`).
- **Подход:** изменить сигнатуру `floorDivInt64` на `(int64, bool)` (bool=overflow), зеркально
  `mulInt64`: возвращать `(0, true)` при `a == math.MinInt64 && b == -1`. `evalFloorDiv` (`:180`
  call-site) при overflow → `runtimeErr(pos, "переполнение целого числа")` — ровно тот же текст/категория
  RT-OVERFLOW, что у `*`. Деление-на-ноль оставить как есть. (Доп. call-site `metric_engine.go:256`
  использует тот же `evalFloorDiv` — правки не требует.)
- **Влияние на golden:** нет (RT-OVERFLOW переиспользован).
- **Тест-замок:** `eval/expr_test.go` — **`TestFloorDivOverflowEdge`**: выражение, дающее
  `MinInt64 // -1` (`MinInt64` через `(0 - 9223372036854775807 - 1)`) → «переполнение целого числа»;
  happy-path `7 // 2 → 3` не ломать; деление-на-ноль не тронуто.

### Финальные гейты (FR-019)
Из `src/`: `go build ./...`, `go vet ./...`, `gofmt -l .` (пусто), `go test ./... -count=1` (11
пакетов ok) — exit 0. Рабочее дерево чистое: сборочный бинарь и файлы БД не закоммичены.

### Mutation-probe (FR-015)
КАЖДЫЙ из 6 замков обязан кусаться: в одноразовом scratch-worktree откатить соответствующий фикс →
замок краснеет. Не краснеет → усилить замок и повторить.

## Граница §2 — что НЕ в этом плане

Явно вне скоупа (бэклог / синк архитектора), по анкеру §4/§5 и spec «Out of Scope»:

- **Большие доки** — `SPEC.md`, `ARCHITECTURE.md`, `grammar.md`, `README.md`, `docs/eval-model.md`,
  `docs/engine-model.md`, `docs/automation-model.md`, `docs/stdlib.md`: синк делает архитектор отдельно
  после мержа (FR-018; не блокирует фичу). Под `specs/` синкается только реестр
  `specs/003-interpreter-eval/contracts/runtime-errors.md` (Фикс C).
- **Не-A–F находки** — если в ходе работы найдётся «ещё баг» вне A–F, его НЕ чинить, только записать в
  отчёт.
- **Парсер-восстановление** (`recover.go:71`, `parse_decl.go:303/444`) — бэклог.
- **`cmd/ladix/start.go` отрицательные литералы** — бэклог.
- **`inspect.go` порядок** — бэклог.
- **`engine.go:86` empty-Steps паника** (недостижима) — бэклог.
- **`daemon` schedule-overflow** (документированный край v1) — бэклог.
- **Float-арифметика → Inf** (отдельный backlog `float-overflow-spec-gap`) — НЕ здесь; Фикс B лишь
  страхует от её последствий (defense-in-depth).

## Complexity Tracking

> Constitution Check 9/9 PASS, нарушений нет. Заполнено справочно по дельте каталога ошибок (не
> нарушение — санкционированный наблюдаемый контракт US1).

| Элемент | Зачем нужно | Почему проще нельзя |
|---------|-------------|---------------------|
| +2 сообщения Фикса C (каталог 28→30) | Единственный пользовательски-видимый фикс: громкий отказ NaN/Inf/hex-float вместо тихой порчи value-пространства (FR-006) | Тихий пропуск = текущий баг; новых **кодов/категорий** не вводим — оба под существующим `ОшибкаВыполнения` |
