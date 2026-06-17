# Feature Specification: B4 — эскалация дедлайна (фронтенд B4a + бэкенд B4b, durable)

**Feature Branch**: `016-deadline-escalation`

**Created**: 2026-06-17

**Status**: Draft

**Input**: Якорь — `docs/automation-model.md` §AU-6 (B4 усиленный = B4a фронтенд + B4b
бэкенд), §AU-2 (поле `Task.Escalated` + 4 точки SQLite-кодека), §AU-10 (диагностики/stdout
дословно), §AU-12 (golden A/B/C, особ. §AU-12.B durable), решения §AU-1 D-AU-4/D-AU-5/D-AU-6.
Веха M2 автоматизации. БЕЗ новой языковой функциональности сверх нового вида триггера.

## Контекст и граница

B4 закрывает золотой сценарий §2 «эскалация просроченной задачи»: новый вид
триггера-декларации `когда задача просрочена в <Процесс>.<Шаг>: <тело>` (B4a, фронтенд),
а под `serve` демон сканирует просроченные человеческие задачи и одноразово исполняет
тело эскалации, переживая рестарт без повтора (B4b, бэкенд, durable). Эскалация
ровно раз на задачу, флаг персистнут в SQLite (`Task.Escalated`).

Фича разрезана на ДВЕ фаза-группы, реализуемые ДВУМЯ ПРОХОДАМИ:
- **B4a — ФРОНТЕНД** (parser/AST/семпроход/run-заглушка): новый вид триггера компилируется
  (lex/parse/analyze) и под `run` печатает заглушку. ИДЁТ ПЕРВЫМ.
- **B4b — БЭКЕНД** (daemon + store-поле): durable-эскалация под `serve`. Топологически
  ЗАВИСИТ от B4a (тело эскалации = AST-узел из B4a). ИДЁТ ВТОРЫМ.

### Слои затронуты

**B4a:** `internal/ast/trigger.go` (узел `DeadlineTrigger`), `internal/parser/parse_decl.go`
(`parseDeadlineTrigger` + ветка в `parseTriggerDecl` + новый хелпер `expectLexeme`),
`internal/parser/errors.go` (расширение `msgTriggerKind`), `internal/eval/analyze.go`
(кейс `*ast.DeadlineTrigger` в `checkTrigger`), `internal/eval/trigger_run.go` (run-заглушка).

**B4b:** `internal/store/types.go` (поле `Task.Escalated`), `internal/store/sqlite.go`
(4 точки кодека), `internal/store/memory.go` (copyTask тривиально несёт bool),
`internal/daemon/tick.go` (4-я фаза), `internal/daemon/checkdeadlines.go` (НОВЫЙ —
`checkDeadlines` + `fireDeadlineBody`).

### НЕ затронуты (пустой дифф — drift-watch)

- `internal/eval` engine/store-импорты, `internal/value`, `internal/engine` (кроме чтения
  `engine.Overdue`, существующий хелпер). `ProcessRuntime` остаётся **8 методов** (НЕ
  расширяется — `checkDeadlines` зовёт `EvalBlockInTrigger`/`NewTriggerBodyEnv`, уже в шве).
- **Store остаётся 15 методов** — B4 добавляет КОЛОНКУ `escalated` к существующим
  методам, НЕ новый метод. (`ListTasksByInstance` — это B6, НЕ B4.)
- Счётные замки: лексер L=11 цел (D-AU-4: `задача`/`просрочена` остаются IDENT, лексер НЕ
  трогается); парсер SE=14 цел (новых SE-кодов нет; SE-TRIGGER-KIND расширяет ТЕКСТ при том
  же коде); `errors_golden_test` eval=28 цел (семантические триггерные коды живут в
  `analyze_trigger_test.go`-семействе, §AU-10.B, не инкрементят 28).

### КРИТИЧНО — порядок с B5 (durable-golden §AU-12.B использует `ladix start`)

§AU-12.B durable-golden в каноне стартует инстанс командой `ladix start эскалация_плана
2500000 --db demo.db`, но **`ladix start` — это B5, идёт ПОСЛЕ B4 в поезде**. Поэтому:

- **На этапе B4b durable-поведение тестируется на уровне Go-API**: создать инстанс+задачу с
  дедлайном напрямую через `StartProcess`/Store-API в SQLite, прогнать `tick()` демона с
  FixedClock, продвинуть часы за дедлайн, проверить РОВНО одну эскалацию; перезапустить
  Store на той же `--db`, прогнать tick снова → отсутствие повтора. Мутпроба: снять фильтр
  `if t.Escalated` → durable-тест КРАСНЕЕТ.
- **Полная CLI-сборка §AU-12.B (через `ladix start`) и end-to-end §AU-12.C** — собираются
  на **M2-гейте ПОСЛЕ B5**. Это осознанная последовательность поезда, НЕ дефект B4
  (зафиксировано в plan.md, раздел границ/последовательности).

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Синтаксис и компиляция эскалация-триггера (Priority: P1, B4a)

Автор `.ladix` объявляет триггер `когда задача просрочена в эскалация_плана.связаться_с_клиентом:`
с телом `уведомить руководитель(факт)`. Файл лексируется (`задача`/`просрочена` — IDENT),
парсится в `ast.DeadlineTrigger{Process, Step}`, проходит семпроход (процесс объявлен, шаг
существует), и под `run` печатает заглушку, НЕ исполняя тело.

**Why this priority**: Фундамент B4 — без распознавания синтаксиса бэкенд B4b нечего
исполнять (тело эскалации = AST-узел из B4a). MVP фронтенда = именно это.

**Independent Test**: `.ladix` с объявленным процессом, человеческим шагом и эскалация-триггером
к этому шагу; `ladix run f` → exit 0 + строка-заглушка `задача триггер 'P.S' требует serve
(фича 007b)`, тело НЕ исполнено. Полностью независимо от B4b.

**Acceptance Scenarios**:

1. **Given** объявлен процесс `эскалация_плана` с шагом `связаться_с_клиентом`, **When**
   парсится `когда задача просрочена в эскалация_плана.связаться_с_клиентом:\n    печать(1)`,
   **Then** `TriggerDecl.Spec` = `*ast.DeadlineTrigger{Process:"эскалация_плана", Step:"связаться_с_клиентом"}`,
   `Pos()` = токен `задача`.
2. **Given** тот же файл, **When** `ladix run f`, **Then** exit 0, stdout содержит
   `задача триггер 'эскалация_плана.связаться_с_клиентом' требует serve (фича 007b)`, тело
   `печать(1)` НЕ выполнено.
3. **Given** `пусть задача = 10` (v1-код вне позиции триггера), **When** `ladix run`,
   **Then** exit 0 — `задача` остаётся обычным IDENT (D-AU-4, v1 НЕ ломается).

---

### User Story 2 — Семантика и диагностики эскалация-триггера (Priority: P1, B4a)

Семпроход проверяет: процесс объявлен, шаг существует в процессе; тело трактуется как у
расписание-триггера (lenient-scope, оба контекст-флага false). Малформенный триггер даёт
точную `SE-EXPECTED`; нераспознанный вид после `когда` даёт расширенный `SE-TRIGGER-KIND`.

**Why this priority**: Без точных диагностик автор не отличит опечатку от структурной ошибки;
дословность текстов (§AU-10) — конституционный принцип VIII. Равноприоритетно синтаксису.

**Independent Test**: набор негативных `.ladix` (неизв. процесс, неизв. шаг, пропуск
`просрочена`/`в`/`.`/шага, нераспознанный вид после `когда`) → exact-match диагностики;
позитивные тела с `значение`/`событие` → наследуемый контекст-гард. Независимо от B4b.

**Acceptance Scenarios**:

1. **Given** `когда задача просрочена в неизвестный.шаг:`, процесс `неизвестный` НЕ объявлен,
   **When** analyze, **Then** `СемантическаяОшибка` `процесс 'неизвестный' не объявлен`.
2. **Given** процесс `P` объявлен, но без шага `нетшага`, `когда задача просрочена в P.нетшага:`,
   **When** analyze, **Then** `СемантическаяОшибка` `шаг 'нетшага' не найден в процессе 'P'`.
3. **Given** `когда задача X` (вместо `просрочена`), **When** parse, **Then** `SE-EXPECTED`
   `ожидалось 'просрочена', получено 'X'`.
4. **Given** `когда задача просрочена P.S` (пропуск `в`), **When** parse, **Then** `SE-EXPECTED`
   `ожидалось 'в', получено 'P'`.
5. **Given** `когда задача просрочена в P S` (пропуск `.`), **When** parse, **Then** `SE-EXPECTED`
   `ожидалось '.', получено 'S'`.
6. **Given** `когда мусор:` (нераспознанный вид), **When** parse, **Then** `SE-TRIGGER-KIND`
   `ожидалось 'метрика, событие, расписание или задача', получено 'мусор'` (РАСШИРЕННЫЙ текст).
7. **Given** тело эскалации содержит `значение` или `событие`, **When** analyze, **Then**
   наследуемый контекст-гард TR-VAL-CTX / TR-EVT-CTX (оба флага false, как расписание-триггер).
8. **Given** свободный `факт` в теле эскалации (не объявлен статически), **When** analyze,
   **Then** НЕТ статической ошибки (lenient-scope; резолв в рантайме против инжекта B4b).

---

### User Story 3 — Durable-эскалация под serve, переживает рестарт (Priority: P1, B4b)

Под `serve` 4-я фаза `tick()` сканирует просроченные открытые задачи. Если задача просрочена
(дедлайн < часы демона), НЕ эскалирована, её шаг/процесс совпали с эскалация-триггером →
тело эскалации исполняется с инжектом ВСЕХ `InstanceVariables` инстанса в read-only env →
`Task.Escalated=true` → `SaveTask`. Одноразово на задачу. После рестарта демона (та же `--db`)
`Escalated==true` персистнут в SQLite → скан видит флаг → `continue` → повтора НЕТ.

**Why this priority**: Ядро B4b — durable-гарантия «эскалация ровно раз, переживает рестарт»
(хартия §2, риск критиков #1). MVP бэкенда = именно это.

**Independent Test (Go-API, до B5)**: через `StartProcess`/Store-API создать инстанс с
человеческим шагом и задачей с дедлайном в SQLite; FixedClock = created → tick → тишина (НЕ
просрочена); Clock += за дедлайн → tick → РОВНО одна эскалация (тело напечатало уведомление),
`Escalated` персистнут; перезапустить Store на той же `--db`, `RunRestartScan` + tick → нет
повтора. **Мутпроба**: снять `if t.Escalated continue` → второй tick после рестарта эскалирует
повторно → durable-тест КРАСНЕЕТ.

**Acceptance Scenarios**:

1. **Given** инстанс `p1` ожидает на задаче `t1` (дедлайн = created+2дн), Clock=created,
   эскалация-триггер к шагу `t1`, **When** `tick()`, **Then** тишина (`t1` НЕ просрочена,
   `Escalated` не тронут).
2. **Given** то же, Clock продвинут на created+3дн, **When** `tick()`, **Then** тело
   эскалации исполнено РОВНО раз (`[уведомление] руководитель: <значение факт>`),
   `t1.Escalated==true`, `SaveTask` вызван.
3. **Given** `t1` уже эскалирована (`Escalated==true`), **When** следующий `tick()`, **Then**
   `continue` — повтора НЕТ.
4. **Given** `t1.Escalated==true` персистнут в SQLite, **When** Store перезапущен на той же
   `--db` + `RunRestartScan` + `tick()`, **Then** скан читает `Escalated==true` → повтора НЕТ
   (durable × рестарт).
5. **Given** задача завершена `complete` ДО просрочки, **When** `tick()` после дедлайна,
   **Then** задачи нет в `ListPendingTasks` → эскалации нет (корректно).
6. **Given** тело эскалации `уведомить руководитель(факт)`, инстанс имеет переменную
   `факт=2500000`, **When** эскалация срабатывает, **Then** `факт` резолвится из инжекта
   `InstanceVariables` (D-AU-6), уведомление содержит `2500000`.

---

### User Story 4 — 4-я фаза tick аддитивна, метрики-демо не молчит (Priority: P2, B4b)

`checkDeadlines` добавляется В ХВОСТ `tick()` под ТЕМ ЖЕ `d.mu`, ПОСЛЕ первых трёх фаз
(`ResetRunState→drainEvents→evalMetrics→checkSchedules`), НЕ меняя их порядок/идемпотентность.
Метрика-демо §2 «всегда ниже порога» молчит без edge-пересечения (§AU-12.A) — golden ОБЯЗАН
внести пересечение во время демона (вариант i: данные падают НИЖЕ порога на тике; вариант ii:
прямой `ladix start`, чище — на M2-гейте).

**Why this priority**: Инвариант 007b (риск критиков #3) и не-молчащее демо (риск #4) —
несущие, но проверяемы поверх ядра US3. Живой daemon-тест.

**Independent Test**: живой daemon-тест с метрикой + расписанием + эскалацией в одном
`tick()`; проверить, что первые три фазы отрабатывают как до B4 (порядок/идемпотентность),
а четвёртая — в хвосте, под тем же `d.mu`. Edge-демо: метрика падает НИЖЕ порога во время
демона → ровно одно `LastBool: false→true` срабатывание (re-arm не повторяет).

**Acceptance Scenarios**:

1. **Given** демон с метрика-, расписание- и эскалация-триггерами, **When** `tick()`, **Then**
   фазы исполнены в порядке `ResetRunState→drainEvents→evalMetrics→checkSchedules→checkDeadlines`,
   все под одним `d.mu.Lock`.
2. **Given** метрика «всегда выше порога», затем данные падают НИЖЕ во время демона, **When**
   серия тиков, **Then** до пересечения тишина; на пересечении РОВНО одно edge-fire
   (`LastBool != nil && !*LastBool && cur`); re-arm не повторяет.

---

### Edge Cases

- `пусть задача = 10`, `задача()` вне позиции триггера → `задача` обычный IDENT, exit 0 (D-AU-4).
- `когда задача просрочена в P.` (пропуск имени шага) → `SE-EXPECTED` `ожидалось 'имя шага', получено '<лексема>'`.
- `когда задача просрочена в .S` (пропуск имени процесса) → `SE-EXPECTED` `ожидалось 'имя процесса', получено '.'`.
- Тело эскалации с действием-шага (`исполнитель:`/`срок:`) → наследуемый запрет §TR-7.C.
- `--db` отсутствует (MemoryStore) → durable-гарантия НЕ держится (эфемерно); демо ОБЯЗАНО с `--db` (§AU-9).
- Задача эскалирована, затем завершена → `Escalated` уже стоит; завершение штатно.
- Нет эскалация-триггеров в программе → `checkDeadlines` ранний `return` (нет работы), без листинга задач.

## Requirements *(mandatory)*

### B4a — фронтенд

- **FR-001**: Парсер ОБЯЗАН распознавать `когда задача просрочена в <Процесс>.<Шаг>:` как
  `ast.DeadlineTrigger`. Лексемы `задача`/`просрочена` — IDENT (НЕ ключевые слова); контекст
  применяет ПАРСЕР после `когда` (D-AU-4). Лексер НЕ трогается (L=11 цел).
- **FR-002**: AST-узел `ast.DeadlineTrigger{specBase; Process Ident; Step Ident}` +
  `NewDeadlineTrigger(pos, process, step)` + маркер `func (*DeadlineTrigger) triggerSpec() {}`.
  Четвёртый конкретный тип `TriggerSpec`. `Pos()` = токен `задача`.
- **FR-003**: `parseTriggerDecl` (`parse_decl.go:406`): после `KW_WHEN`, если
  `p.peek().Type == lexer.IDENT && p.peek().Lexeme == "задача"` → `parseDeadlineTrigger()`;
  иначе прежний `default` → `SE-TRIGGER-KIND`.
- **FR-004**: `parseDeadlineTrigger`: (1) `advance()` потребляет IDENT `задача`; (2) НОВЫЙ
  хелпер `expectLexeme("просрочена")` (сверяет `tok.Type==IDENT && tok.Lexeme==want`, иначе
  `p.error(pos, msgExpected(want, tok))`); (3) `expect(KW_IN, "в")`; (4) `expect(IDENT, "имя
  процесса")`; (5) `expect(DOT, ".")`; (6) `expect(IDENT, "имя шага")`. Все ошибки —
  `SE-EXPECTED` через `msgExpected`. Новых SE-кодов НЕТ (SE=14 цел).
- **FR-005**: `msgTriggerKind` (`errors.go:29`) РАСШИРЯЕТСЯ:
  `"метрика, событие или расписание"` → `"метрика, событие, расписание или задача"`. Тот же код
  SE-TRIGGER-KIND (счёт 14 НЕ меняется). Со-обновить комментарий `errors.go:26` если цитирует.
- **FR-006**: Семпроход `checkTrigger` (`analyze.go:319`) — кейс `*ast.DeadlineTrigger`:
  (а) `Process.Name` объявлен, иначе `СемантическаяОшибка` `процесс '<имя>' не объявлен`
  (переиспользовать `analyze.go:729`); (б) `Step.Name` существует в процессе, иначе
  `шаг '<шаг>' не найден в процессе '<процесс>'`; (в) тело —
  `checkTriggerBody(td.Body, false, false)` (как расписание-триггер, lenient-scope, D-AU-6).
- **FR-007**: Run-заглушка (`trigger_run.go`, ветка в `RunTriggers` — экспортный метод
  `*Interpreter`, диспетчер по `td.Spec.(type)`, рядом с кейсами событие/расписание `:49/:51`): ДОСЛОВНО
  `fmt.Fprintf(w, "задача триггер '%s.%s' требует serve (фича 007b)\n", spec.Process.Name, spec.Step.Name)`.
  Тело НЕ исполняется (зеркало событие/расписание под `run`).
- **FR-008**: v1 НЕ ломается: `пусть задача = 10`, `задача()` вне позиции триггера → exit 0,
  `задача` остаётся обычным IDENT. `DeadlineTrigger` аддитивен.

### B4b — бэкенд (durable)

- **FR-009**: Поле `Task.Escalated bool` (`store/types.go`, после `CompletedAt`) — durable,
  одноразово. `MemoryStore.copyTask` несёт его тривиально (`cp := *t`).
- **FR-010**: SQLite-кодек `Task` — ВСЕ 4 точки (иначе `Escalated` молча теряется):
  (1) DDL `tasks` (`sqlite.go:33`) +`escalated INTEGER NOT NULL DEFAULT 0`;
  (2) `SaveTask` INSERT-список колонок (`sqlite.go:161`);
  (3) `ON CONFLICT(id) DO UPDATE SET … escalated=…` (UPSERT, `sqlite.go:165`);
  (4) ВСЕ SELECT-читатели через `buildTask`/`scanTask` (`sqlite.go:296/310`): `LoadTask`
  (`:179`), `ListPendingTasks` (`:186` — главный читатель скана) — добавить `escalated` в
  SELECT и в сигнатуру `buildTask`/`scanTask`. (`ListTasksByInstance` — B6, НЕ сейчас.)
- **FR-011**: 4-я фаза `tick()` (`tick.go:10`): `d.checkDeadlines()` добавляется В ХВОСТ под
  ТЕМ ЖЕ `d.mu`, ПОСЛЕ `ResetRunState→drainEvents→evalMetrics→checkSchedules`, НЕ меняя
  порядок/идемпотентность первых трёх. Верифицировать живым daemon-тестом.
- **FR-012**: `checkDeadlines`: ранний `return` если нет эскалация-триггеров; ОДИН
  `ListPendingTasks("")` до циклов; для каждой задачи: `if t.Escalated continue` (durable-фильтр);
  `if !engine.Overdue(t, now) continue` (существующий хелпер, `format.go:35`); `LoadInstance`;
  для каждого эскалация-триггера: совпадение `t.StepName==spec.Step.Name && inst.ProcessName==spec.Process.Name`
  → `safeFire(fireDeadlineBody)` → `t.Escalated=true; SaveTask(t)` → `break` (одна на задачу).
  Ошибка листинга → лог + выход из фазы (изоляция как первые три).
- **FR-013**: `fireDeadlineBody(body, vars map[string]value.Value)` — НОВАЯ функция рядом с
  `fireBody` (`fire.go:22`): `env := NewTriggerBodyEnv()`; цикл `for k,v := range vars {
  env.Define(k, v) }` (инжект ВСЕХ `InstanceVariables`, D-AU-6, минуя struct `injection`);
  `return EvalBlockInTrigger(env, body)`. `vars` = `inst.Variables` (уже загружено, без round-trip).
  Read-only барьер тела (TR-BODY-RO) наследуется от `NewTriggerBodyEnv`+`markBoundary`.
- **FR-014**: Durable × рестарт: `Escalated` персистнут в SQLite; `RunRestartScan` (`restart.go:28`)
  реактивирует инстансы, `checkDeadlines` читает `ListPendingTasks` из той же `--db`, видит
  `Escalated==true` → `continue` → повтор невозможен.
- **FR-015**: Часы — существующий `d.clock.Now()` (`engine.Clock`, инжектируемый, `daemon.go:25`);
  тот же Clock в движке/интерпретаторе (007b «двойные часы через адаптер» уже сведены к одному
  источнику). `checkDeadlines` сравнивает дедлайн с часами демона.

### Несущие инварианты (нацеливают tasks)

- **INV-1 (007b)**: 4-я фаза `checkDeadlines` аддитивна в хвост под тем же `d.mu`; первые 3
  фазы (порядок/идемпотентность) НЕ меняются. Живой daemon-тест.
- **INV-2 (durable)**: эскалация ровно раз на задачу, переживает рестарт; `Escalated`
  персистнут в SQLite (4 точки кодека целостны). **Мутпроба-замок: снять `if t.Escalated`
  → durable-golden КРАСНЫЙ.**
- **INV-3 (фронтенд)**: `задача`/`просрочена` остаются IDENT, v1 `пусть задача = 10` → exit 0;
  SE-TRIGGER-KIND каскад полон (все зеркала-замки обновлены, счёт SE=14 цел).
- **INV-4 (швы)**: `ProcessRuntime`=8 (НЕ расширяется); eval без store/engine; Store=15 методов
  (B4 = новая колонка, НЕ новый метод).
- **INV-5 (счётчики)**: §EN-7/007b golden целы (кроме осознанного сдвига SE-TRIGGER-KIND
  строки); lexer L=11 цел; SE=14 цел; errors_golden eval=28 цел (семантические триггерные — в
  `analyze_trigger_test.go`, не инкрементят 28).

### Key Entities

- **`ast.DeadlineTrigger`** — четвёртый `TriggerSpec`: `Process Ident`, `Step Ident`. Pos = `задача`.
- **`Task.Escalated bool`** — durable-флаг одноразовости эскалации; колонка `escalated INTEGER
  NOT NULL DEFAULT 0`.
- **`checkDeadlines` / `fireDeadlineBody`** — 4-я фаза tick (скан) + исполнитель тела с
  мульти-инжектом `InstanceVariables`.

## Дословные тексты (exact-match golden — канон §AU-10)

- **Run-заглушка**: `задача триггер '<процесс>.<шаг>' требует serve (фича 007b)`.
- **SE-TRIGGER-KIND (новая строка)**: `ожидалось 'метрика, событие, расписание или задача', получено '<лексема>'`.
- **SE-EXPECTED (малформенный)**: `ожидалось '<want>', получено '<лексема>'`, где want ∈
  {`просрочена`, `в`, `имя процесса`, `.`, `имя шага`}.
- **Семантика — процесс не объявлен**: `процесс '<имя>' не объявлен`.
- **Семантика — шаг не найден**: `шаг '<шаг>' не найден в процессе '<процесс>'`.
- **stdout эскалации (golden §AU-12.B)**: `[уведомление] руководитель: <значение>` (через тело).

## Доковые зеркала SE-TRIGGER-KIND (синкает архитектор на M2-гейте — НЕ моя зона)

Расширение текста `msgTriggerKind` имеет зеркала в БОЛЬШИХ доках, которые B4 НЕ трогает, но
ПЕРЕЧИСЛЯЕТ для синка архитектором:
- `docs/trigger-model.md:432`, `:433`, `:1070`, `:1087`.
- `SPEC §13` / `docs/diagnostics-model.md` — любые цитаты старого текста.

ТВОИ (B4a) зеркала-замки старого текста, ОБЯЗАТЕЛЬНЫЕ к со-обновлению (иначе гейт `go test` не
зелёный): `parser/errors.go:29` (+ коммент `:26`), `parser/inventory_test.go:34` (fragment-match),
`parser/parse_decl_test.go:1549` (`TestTriggerSyntaxDiagnostics`), `:1622`
(`TestTriggerNegativesExactPos`), `:1666` (`TestGoldenTriggerSyntaxDiagnostics` двухстрочная).

## Эмпирические якоря (сверены в коде @a92ad50)

- `msgTriggerKind` = `"метрика, событие или расписание"` (`errors.go:29`), коммент `errors.go:26`.
- `parseTriggerDecl` диспетчер `KW_METRIC`/`KW_EVENT`/`KW_SCHEDULE`+`default→SE-TRIGGER-KIND`
  (`parse_decl.go:406-417`); `expect`/`error` (`parser.go:79/98`); `expectCompOp` (`parse_decl.go:441`,
  прецедент локального хелпера); `msgExpected` (`errors.go:59`).
- `checkTrigger` (`analyze.go:319`): `MetricTrigger`/`EventTrigger`/`ScheduleTrigger`; расписание
  зовёт `checkTriggerBody(td.Body, false, false)` (`analyze.go:368`) — ТОЧНЫЙ шаблон для эскалации;
  `процесс '%s' не объявлен` (`analyze.go:729`, `checkRunProcess`).
- `trigger_run.go`: заглушки событие/расписание `:49/:51` (`требует serve (фича 007b)`).
- `tick()` (`tick.go:10-16`): `ResetRunState→drainEvents→evalMetrics→checkSchedules` под `d.mu`.
- `Task` struct (`store/types.go:48-57`) без `Escalated`; `TaskPending="открыта"`.
- SQLite кодек: DDL tasks `sqlite.go:33`, SaveTask `:161`, ON CONFLICT `:165`, LoadTask `:179`,
  ListPendingTasks `:186`, scanTask `:296`, buildTask `:310`.
- `fireBody`/`injection`/`NewTriggerBodyEnv`/`EvalBlockInTrigger` (`fire.go:10/22-27`).
- `RunRestartScan` (`restart.go:28`), `ListInstancesByStatus` (`restart.go:32`).
- `engine.Overdue` (`format.go:35`).
- Замок `examples_test.go` `TestExamplesParseCleanSet` clean[] — **эмпирически 24 файла**
  (`:12-35`), НЕ 22 как в §AU-11.1 (якорь устарел; новые чисто-парсящиеся примеры B4
  добавлять в ЖИВОЙ clean[]). Негативные примеры → golden-замки, не в clean[].

## Success Criteria *(mandatory)*

- **SC-001**: `когда задача просрочена в P.S:` → `DeadlineTrigger{P,S}`; `пусть задача = 10` → exit 0.
- **SC-002**: Run-заглушка дословна; тело НЕ исполнено под `run`.
- **SC-003**: Малформенный триггер → точная `SE-EXPECTED` (5 want-вариантов); нераспознанный
  вид → расширенный `SE-TRIGGER-KIND`; все зеркала-замки зелёные; SE=14, L=11 целы.
- **SC-004**: Неизв. процесс/шаг → дословные `СемантическаяОшибка`; eval=28 цел.
- **SC-005**: Durable-эскалация ровно раз на задачу, переживает рестарт (Go-API тест); мутпроба
  снятия `if t.Escalated` → красный.
- **SC-006**: 4-я фаза аддитивна, первые три не сломаны (живой daemon-тест); метрика-демо не молчит.
- **SC-007**: `ProcessRuntime`=8, Store=15 методов; 0 новых зависимостей; детерминизм (FixedClock).
- **SC-008**: Порядок с B5 учтён: durable на Go-API сейчас, CLI §AU-12.B/C на M2-гейте после B5.
