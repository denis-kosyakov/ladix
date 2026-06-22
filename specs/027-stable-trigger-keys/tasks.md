---
description: "Task list — Стабильные контентные ключи триггеров"
---

# Tasks: Стабильные контентные ключи триггеров

**Input**: Документы дизайна из `/specs/027-stable-trigger-keys/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/{canon,trigger-keys,migration}.md, quickstart.md
**Ветка**: `027-stable-trigger-keys` | **Корень исходников**: `src/`

**Tests**: Включены и обязательны — Конституция VI (фича парсер-смежная: канонизатор живёт в
`internal/ast`; тесты идут рядом/раньше прод-кода). 9 замков T1–T9 (quickstart.md), маркер
инверсионной мутпробы — 🔁.

## Формат: `[ID] [P?] [Story?] Описание`

- **[P]** — можно параллелить (разные файлы, нет зависимостей от незавершённых задач).
- **[Story]** — к какой US относится (US1–US4); фазы Setup/Foundational/Polish без метки.
- Каждая задача указывает точный путь файла.

## Карта историй

| Story | Приоритет | Суть | Замки |
|-------|-----------|------|-------|
| US1 | P1 🎯 MVP | Редактирование/перестановка файла не ломает baseline (ядро бага) | T5 🔁, T6 |
| US2 | P2 | Каноническое равенство и различие условий | T2, T3 |
| US3 | P3 | Дубликаты условий различимы (ordinal) | T4 |
| US4 | P2 | Чистый переход существующих БД (миграция 2→3 + нейтральность 1-го тика) | T7, T8 🔁, T9 |

---

## Phase 1: Setup (общая инфраструктура)

**Назначение**: подтвердить базовую готовность; новых каркасов фича не вводит (0 новых зависимостей,
0 новых файлов вне `internal/ast` и `internal/daemon`).

- [ ] T001 Подтвердить зелёный baseline до правок: из `src/` прогнать `go build ./...`, `go vet ./...`,
      `go test ./...` (фиксируем исходное зелёное состояние перед изменениями).

---

## Phase 2: Foundational (блокирующие предпосылки) ⚠️

**Назначение**: контентный ключ-механизм, на который опираются ВСЕ истории. Цепочка зависимостей:
`canon.go` ⟶ `buildTriggerKeys` ⟶ поле `triggerKeys` в `Daemon` ⟶ замена call-sites чтения ключа.

**⚠️ CRITICAL**: ни одна история (US1–US4) не может стартовать, пока эта фаза не завершена.

### Канонизатор AST (листовой `internal/ast`) — тест ВПЕРЁД (Конституция VI)

- [ ] T002 [P] Замок T1 — исчерпываемость `canonExpr` в `src/internal/ast/canon_test.go` (НОВЫЙ):
      table-driven кейсы по **РОВНО 19** конкретным типам выражений с ожидаемыми каноническими
      строками (`*IntLit`/`*FloatLit`/`*StringLit`/`*BoolLit`/`*NoneLit`/`*DurationLit`/
      `*WindowPeriodLit`/`*LastCompletedPeriodLit`/`*ListLit`/`*Ident`/`*BinaryExpr`/`*UnaryExpr`/
      `*CallExpr`/`*IndexExpr`/`*FieldExpr`/`*RunProcessExpr`/`*CallExternalExpr`/`*ValueExpr`/
      `*EventExpr`) + локальный stub-тип, реализующий `Expression` (`exprNode()`), → `canonExpr(stub)`
      **паникует** (`recover` + assert факта/текста паники «незнакомый тип выражения %T»). 🔁: убрать
      любую ветку switch → её тип уходит в `default`-panic → кейс/stub краснеет. (FR-003, SC-007)

- [ ] T003 Создать `src/internal/ast/canon.go` (НОВЫЙ; импорт `fmt`, `strconv`, `strings`):
      экспортный `CanonicalTriggerCondition(spec ast.TriggerSpec) string` (type-switch:
      `*MetricTrigger`→`"metric|"+Metric.Name+"|"+Op.String()+"|"+canonExpr(Threshold)`;
      `*ScheduleTrigger`/`*EverySchedule`→`"every|"+canonDuration(Every)`;
      `*ScheduleTrigger`/`*AtSchedule`→`"at|"+At.Value`; `*EventTrigger`/`*DeadlineTrigger`→`""`) +
      приватный тотальный рекурсивный `canonExpr(e ast.Expression) string` (switch по всем 19 типам
      из contracts/canon.md, `default`→`panic(fmt.Sprintf("canonExpr: незнакомый тип выражения %T", e))`)
      + хелпер `canonDuration(d *DurationLit) string = d.Amount+"|"+d.Unit`. Сделать T002 зелёным.
      (FR-002/003) [блокирует T004 и все истории]

### Ключ-билдер демона + интеграция (`internal/daemon`)

- [ ] T004 Добавить `buildTriggerKeys(trig []*ast.TriggerDecl) []string` в `src/internal/daemon/keys.go`
      (НОВЫЙ файл; импорт `hash/fnv`, `strconv`, `fmt`) по алгоритму contracts/trigger-keys.md:
      `c := ast.CanonicalTriggerCondition(td.Spec)`; `c==""`→пустой слот (continue); группировка
      ordinal-map `ordinals[c]++`; `h := fnv.New64a(); h.Write([]byte(c+"#"+strconv.Itoa(ord)))`;
      `keys[idx] = "trg-"+fmt.Sprintf("%016x", h.Sum64())`; `len(keys)==len(trig)`, выравнивание по
      индексам. (FR-001/004/005) [зависит от T003]

- [ ] T005 Добавить поле `triggerKeys []string` в `type Daemon struct` (`src/internal/daemon/daemon.go:25-33`)
      и заполнить его в конструкторе `New` (`:37-49`) после литерала структуры:
      `d.triggerKeys = buildTriggerKeys(interp.Triggers())`. Сигнатура `New` НЕ меняется, прод call-site
      `serve.go:326` и 4 тест call-site НЕ трогаются. (FR-001) [зависит от T004]

- [ ] T006 Заменить call-sites чтения ключа на массив + удалить позиционный `triggerID`:
      `src/internal/daemon/metrics.go:38` `id := triggerID(idx)` → `id := d.triggerKeys[idx]`;
      `src/internal/daemon/schedule.go:47` то же; **удалить** функцию `triggerID(idx)`
      (`src/internal/daemon/tick.go:43-45`) полностью + убрать/обновить её докстринг (`tick.go:41-42`).
      **ОБЯЗАТЕЛЬНО** убрать осиротевший `import "fmt"` (`tick.go:3`) — это его единственный потребитель,
      иначе `go build` краснеет на неиспользуемом импорте (MAJOR-2 из analyze). Также обновить устаревшие
      докстринг-комментарии с упоминанием `trg-<N>` под символьную/контентную формулировку (SC-008):
      `src/internal/daemon/metrics.go:22`, `src/internal/eval/interpreter.go:137`,
      `src/internal/store/types.go:65` (тип/код НЕ менять — только текст комментария).
      `events.go`/`checkdeadlines.go` ключ не зовут — не трогать. (FR-005, SC-008) [зависит от T005]

**Checkpoint**: контентный ключ минтится один раз при инициализации демона и читается на тиках;
старый позиционный путь удалён. Истории US1–US4 могут стартовать.

---

## Phase 3: User Story 1 — редактирование/перестановка файла не ломает baseline (P1) 🎯 MVP

**Goal**: вставка/перестановка/удаление несвязанного триггера (в т.ч. событие-/дедлайн-) не сдвигает
durable-ключ метрика-/расписание-триггера → его baseline не наследует чужую строку. Это ядро
устраняемого дефекта.

**Independent Test**: `buildTriggerKeys([метрика])` и `buildTriggerKeys([событие, та же метрика])`
дают для метрики ИДЕНТИЧНЫЙ ключ; правка условия даёт НОВЫЙ ключ без ложного фронта.

- [ ] T007 [P] [US1] Замок T5 🔁 (ЯДРО) в `src/internal/daemon/keys_test.go` (НОВЫЙ): построить ключи
      для среза `[метрика-триггер]` и для `[событие-триггер, тот же метрика-триггер]` (несвязанный
      вставлен ПЕРЕД метрикой) → ключ метрика-триггера ИДЕНТИЧЕН в обоих случаях. 🔁: возврат к
      позиционному `triggerID(idx)` → idx метрики 0→1 → ключи разные → краснеет. (FR-001/004/005, SC-001)

- [ ] T008 [P] [US1] Замок T6 в `src/internal/daemon/keys_test.go`: правка условия триггера (напр.
      порога) → `buildTriggerKeys` даёт НОВЫЙ ключ → при загрузке читается чистый baseline (ленивый
      прайм), старая строка под старым ключом не подхватывается → нет ложного фронта. 🔁: сделать ключ
      нечувствительным к порогу → старый baseline подхватился → краснеет. (FR-002/008, SC-003)

**Checkpoint**: US1 (MVP) самостоятельно тестируема — позиционный дрейф устранён.

---

## Phase 4: User Story 2 — каноническое равенство и различие условий (P2)

**Goal**: семантически эквивалентные условия (разный формат числа/пробелы) дают один ключ; различие
по имени/оператору/порогу — разные ключи. Доказывается через РЕАЛЬНЫЙ парсер (независимость от текста).

**Independent Test**: parse→canon трёх форм одного порога совпадает; parse→canon различающихся условий
расходится.

- [ ] T009 [P] [US2] Замок T2 в `src/internal/ast/canon_test.go`: распарсить ЧЕРЕЗ parser три текста
      триггера, отличающихся только форматированием числа/пробелами
      (`выручка_30д < 10_000_000` ≡ `< 10000000` ≡ `<  10000000`) → `CanonicalTriggerCondition`
      (и, где доступно, ключ `buildTriggerKeys`) СОВПАДАЮТ. 🔁: канон по тексту токена вместо
      `FormatInt` → `10_000_000 ≠ 10000000` → краснеет. (FR-002/003, SC-002)

- [ ] T010 [P] [US2] Замок T3 в `src/internal/ast/canon_test.go`: разные имя метрики / оператор
      сравнения / значение порога → РАЗНЫЕ канонические строки/ключи. 🔁: схлопнуть оператор/имя в
      канон (потерять компонент) → совпадение → краснеет. (FR-002, SC-003)

**Checkpoint**: US2 самостоятельно тестируема — каноническое равенство/различие подтверждены парсером.

---

## Phase 5: User Story 3 — дубликаты условий различимы (P3)

**Goal**: два триггера с идентичным условием получают разные ключи (ordinal 0/1).

**Independent Test**: `buildTriggerKeys` для двух идентичных условий даёт два разных ключа.

- [ ] T011 [P] [US3] Замок T4 в `src/internal/daemon/keys_test.go`: два триггера с ИДЕНТИЧНЫМ условием
      → `buildTriggerKeys` даёт РАЗНЫЕ ключи (`ord` 0 и 1). 🔁: убрать `#ord` из хешируемой строки →
      один ключ на оба → краснеет. (FR-004, SC-004)

**Checkpoint**: US3 самостоятельно тестируема — дизамбигуация дубликатов работает.

---

## Phase 6: User Story 4 — чистый переход существующих БД (P2)

**Goal**: апгрейд существующей v2-БД сбрасывает позиционное `trigger_state` (миграция 2→3) и первый
тик после апгрейда поведенчески нейтрален (все три вида durable-триггеров праймят, НЕ срабатывают);
durable-замки держатся в обеих impl Store.

**Independent Test**: открытие v2-БД → `user_version==3` + `trigger_state` пуста; первый тик не
порождает ложных срабатываний (вкл. schedule_at FR-010); Memory/SQLite паритет.

**Примечание зависимостей**: миграция (T013) и FR-010 (T014) — независимы между собой и от US1–US3
(разные файлы: `store/sqlite.go` vs `daemon/schedule.go`), помечены [P]. T012 (тест миграции) пишется
вперёд T013.

- [ ] T012 [P] [US4] Замок T7 в `src/internal/store/migrate_test.go` (паттерн существующего
      `migrate_test.go`): собрать v2-БД со строками `trigger_state` (`trg-0`, `trg-1`, …) и
      `PRAGMA user_version=2` → `NewSQLiteStore(path)` → `trigger_state` ПУСТА И `PRAGMA user_version==3`;
      Close+reopen сохраняет новые (контентные) ключи и версию 3. 🔁: не забампить версию / убрать
      `DELETE`-ступень → INV-R1 паника ИЛИ строки остаются → краснеет. (FR-009, SC-005; только SQLite)

- [ ] T013 [P] [US4] Миграция схемы 2→3 в `src/internal/store/sqlite.go`: добавить 3-й элемент в
      `schemaMigrations` — строку `"DELETE FROM trigger_state;"` с комментарием
      «2 → 3: ре-кей триггеров на контентные ключи (§FR-009), сброс позиционного состояния»; забампить
      `currentSchemaVersion` 2→3. INV-R1: `3 == 1 + 2`. DDL `trigger_state`, тип `TriggerState`,
      `baselineVersion=1`, контракт Store — НЕ трогать. Сделать T012 зелёным. (FR-009, SC-005)

- [ ] T014 [P] [US4] FR-010 — прайм-без-срабатывания `checkAt` в `src/internal/daemon/schedule.go`
      (`~:105-133`): ДО существующего fire вставить miss-ветку —
      `miss := stderrors.Is(loadErr, store.ErrTriggerStateNotFound); if miss && !now.Before(target) {
      Save(&store.TriggerState{TriggerID:id, Kind:atKind, LastFiredDate:&today}); return }` (прайм,
      тело НЕ исполняется; ветка `сейчас < цель` не трогается, штатно сработает в target).
      `checkEvery`/метрика уже праймят — не трогать. (FR-008/010, SC-006)

- [ ] T015 [US4] Замок T8 🔁 (нейтральность первого тика) в `src/internal/daemon/firsttick_test.go`
      (НОВЫЙ; FixedClock): после апгрейда (сброшенное состояние) первый тик —
      метрика-истинная-на-апгрейде → miss → прайм, НЕ срабатывает; `schedule_every` наступившее →
      прайм, НЕ срабатывает; `schedule_at` с `сейчас>=цель` → прайм (`LastFiredDate=today`), НЕ
      срабатывает; следующий день/тик — срабатывает штатно. 🔁: не привести `checkAt` к прайму (T014)
      → schedule_at срабатывает на первом промахе → краснеет. (FR-008/010, SC-006)
      [зависит от T013, T014]

- [ ] T016 [US4] Замок T9 — паритет durable Memory/SQLite через `eachStore`
      (`src/internal/daemon/restart_test.go:35`): durable-замки (детерминированный минт ключа +
      одинаковое состояние Load/Save) прогоняются в ОБЕИХ impl там, где применимо (миграционный T7 —
      только SQLite). 🔁: расхождение Load/Save между impl → один сабтест краснеет. (FR-007, SC-001/004)
      [зависит от T013, T014]

**Checkpoint**: US4 самостоятельно тестируема — апгрейд чист, первый тик нейтрален, паритет держится.

---

## Phase 7: Polish & Cross-Cutting (doc-sync + финальный гейт)

**Назначение**: синхронизация канонов (источник истины — символьные ссылки на
`CanonicalTriggerCondition`/`canonExpr`/`buildTriggerKeys`, БЕЗ захардкоженного позиционного `trg-<N>`)
и финальный зелёный гейт. Выполняется ПОСЛЕ прохождения всех тестов T1–T9.

- [ ] T017 [P] Doc-sync `SPEC.md`: требование вывода durable-ключа триггера (**FR-023**); пункт
      **§C-9 / §12** «стабильные ключи триггеров» пометить ЗАКРЫТЫМ. Символьно, без `trg-<N>`.

- [ ] T018 [P] Doc-sync `docs/engine-model.md` **§EM-17.2.1**: минт ключа триггера — был позиционный
      `triggerID(idx)`, стал контентный `buildTriggerKeys`.

- [ ] T019 [P] Doc-sync `docs/trigger-model.md`: durable-идентичность триггера привязана к смыслу
      (каноническое условие), не к позиции.

- [ ] T020 [P] Doc-sync `docs/reliability-model.md` **§C-9**: стабильные контентные ключи триггеров.

- [ ] T021 [P] Doc-sync `docs/automation-model.md`: упоминание durable-ключа в модели автоматизации
      (символьно).

- [ ] T022 Финальный гейт (SC-008): из `src/` прогнать `gofmt -l` (пусто), `go build ./...`,
      `go vet ./...`, `go test ./...` (всё зелёное) + grep-проверка отсутствия позиционного формата в
      проде: `grep -rnE 'func triggerID|"trg-%d"' internal/ cmd/ --include='*.go' | grep -v '_test.go'`
      ДОЛЖНО быть пусто. (NB: широкий `triggerID` НЕЛЬЗЯ — он матчит неприкосновенный параметр
      контракта Store `LoadTriggerState(triggerID string)`, 6 ложных хитов → false-FAIL гейта,
      MAJOR-1 из analyze. Литерал `"trg-%d"` допустим только в тестовой v2-фикстуре T7.)
      [зависит от всех предыдущих]

---

## Dependencies & Execution Order

### Зависимости фаз

- **Setup (Phase 1)**: без зависимостей.
- **Foundational (Phase 2)**: зависит от Setup; БЛОКИРУЕТ все истории. Внутри —
  строгая цепочка T002→T003→T004→T005→T006 (T002 тест вперёд T003).
- **US1 / US2 / US3 / US4 (Phase 3–6)**: зависят ТОЛЬКО от завершения Phase 2; между собой
  независимы (могут идти параллельно при наличии рук).
- **Polish (Phase 7)**: doc-sync — после прохождения всех тестов; финальный гейт T022 — самый последний.

### Зависимости внутри историй

- US1: T007, T008 — параллельны ([P], тест-онли поверх готовой Phase 2).
- US2: T009, T010 — параллельны ([P]).
- US3: T011 — одиночная.
- US4: T012 (тест) вперёд T013 (миграция); T013 и T014 (FR-010) независимы [P]; T015, T016 зависят от
  T013+T014.

### Параллельные возможности

- В Phase 2 параллелится только T002 (тест канонизатора) — остальное строго последовательно (общая
  цепочка `ast`→`daemon`).
- Истории US1–US4 целиком параллельны между собой после Phase 2.
- US4: T013 (store) и T014 (daemon) — разные файлы, [P].
- Phase 7: T017–T021 (5 doc-sync, разные файлы) — все [P]; T022 — последним.

---

## Parallel Example: US4 (миграция + FR-010 независимы)

```text
# После Phase 2 — два независимых файла можно вести параллельно:
Задача T013: миграция 2→3 в src/internal/store/sqlite.go
Задача T014: FR-010 прайм-без-срабатывания в src/internal/daemon/schedule.go
# Затем свести интеграционными замками T015 (первый тик) и T016 (паритет).
```

```text
# Phase 7 — пять doc-sync параллельно:
Задача T017: SPEC.md (FR-023, §C-9/§12)
Задача T018: docs/engine-model.md §EM-17.2.1
Задача T019: docs/trigger-model.md
Задача T020: docs/reliability-model.md §C-9
Задача T021: docs/automation-model.md
```

---

## Implementation Strategy

### MVP First (US1)

1. Phase 1 (Setup, T001).
2. Phase 2 (Foundational, T002–T006) — КРИТИЧНО, блокирует всё.
3. Phase 3 (US1, T007–T008).
4. **STOP & VALIDATE**: ядро бага (позиционный дрейф) устранено и залочено T5 🔁.

### Incremental Delivery

1. Setup + Foundational → контентный ключ-механизм готов.
2. US1 → ядро устранено (MVP).
3. US2 → каноническое равенство/различие.
4. US3 → дубликаты различимы.
5. US4 → чистый апгрейд существующих БД + нейтральный первый тик.
6. Polish → doc-sync + зелёный гейт.

---

## Notes

- [P] — РАЗНЫЕ файлы, нет зависимостей от незавершённых задач.
- Все 9 замков quickstart присутствуют: T1=T002, T2=T009, T3=T010, T4=T011, T5=T007, T6=T008, T7=T012,
  T8=T015, T9=T016.
- Тесты-инверсии (🔁): T002, T007 (ЯДРО), T008, T009, T010, T011, T012, T015 — ревьюер проверяет на мутпробе.
- ПУСТОЙ функц.дифф: `internal/eval`, `internal/engine`, `cmd/ladix`; контракт Store (тип
  `TriggerState`, DDL `trigger_state`) цел; 0 новых внешних зависимостей / KW / builtins / eval-кодов.
- Старый `triggerID(idx)` удалён (T006); SC-008 проверяется grep'ом в T022.
- Детерминизм: FixedClock в путях демона/расписаний (T015, T016).
