---
description: "Task list — B4 эскалация дедлайна (016): B4a фронтенд ПЕРЕД B4b бэкенд"
---

# Tasks: B4 — эскалация дедлайна (B4a фронтенд + B4b бэкенд, durable)

**Input**: `/specs/016-deadline-escalation/` (spec.md, plan.md, research.md, data-model.md,
contracts/, quickstart.md). Якорь — `docs/automation-model.md` §AU-6 / §AU-2 / §AU-10 / §AU-12 /
D-AU-4/5/6.

**Tests**: ВКЛЮЧЕНЫ и tests-first (Принцип VI). Каждый несущий инвариант (INV-1..5) имеет
замок + мутпробу (инверсия): «мутация → красный». Особо: durable (INV-2, снять `if t.Escalated`),
4-я фаза не ломает первые три (INV-1), SE-TRIGGER-KIND golden, v1 задача-IDENT (INV-3), 4 точки
кодека (пропуск любой → escalated теряется).

**Organization (ДВА ПРОХОДА A→B):** **B4a-ФРОНТЕНД** (Phase 2–4) реализуется ПЕРВЫМ и ПОЛНОСТЬЮ
ДО **B4b-БЭКЕНД** (Phase 5–7). B4b топологически зависит от B4a (тело эскалации = AST-узел из
B4a). `[P]` — разные файлы, без зависимостей.

## Path Conventions

- B4a прод: `src/internal/ast/trigger.go`, `src/internal/parser/{parse_decl.go,errors.go}`,
  `src/internal/eval/{analyze.go,trigger_run.go}`.
- B4b прод: `src/internal/store/{types.go,sqlite.go,memory.go}`,
  `src/internal/daemon/{tick.go,checkdeadlines.go (новый)}`. Читается `src/internal/engine/format.go`.
- Тесты: `*_test.go` рядом с прод-файлами; golden — `src/cmd/ladix/golden_test.go`,
  `parser/parse_decl_test.go`, `parser/inventory_test.go`, `daemon/*_test.go`, `store/*_test.go`.
- Все пути — от корня репо `/Users/denis/dev/ladix`. Строки якоря @a92ad50 (импл сверяет — могут сдвинуться).

---

## Phase 1: Setup (общий)

- [ ] **T001** Сверить базу @a92ad50: `parseTriggerDecl` диспетчер (`parse_decl.go:406-417`),
  `msgTriggerKind` (`errors.go:29`)/коммент `:26`, `checkTrigger` (`analyze.go:319`)+расписание-кейс
  `checkTriggerBody(…,false,false)` (`:368`), run-заглушки (`trigger_run.go:49/51`), `tick()` 3 фазы
  (`tick.go:10-16`), `Task` struct (`types.go:48-57`), кодек (`sqlite.go:33/161/165/179/186/296/310`),
  `fireBody`/`NewTriggerBodyEnv` (`fire.go:22-27`), `RunRestartScan` (`restart.go:28`), `engine.Overdue`
  (`format.go:35`), clean[] (`examples_test.go:12-35` — 24 файла). `ProcessRuntime`=8, Store=15.
- [ ] **T002** `go test ./...` + `go vet ./...` + `-race` на чистом дереве — зелёные (база регресса).
  Зафиксировать счётчики: lexer L=11, parser SE=14, errors_golden eval=28, §EN-7 пины.

---

## === B4a — ФРОНТЕНД (Phase 2–4) — РЕАЛИЗУЕТСЯ ПЕРВЫМ ===

## Phase 2: B4a Foundational — AST-узел (блокирует парсер/семпроход B4a)

- [ ] **T003** [Тест ДО, P] `TestDeadlineTriggerNode` (`ast/trigger_test.go`): `NewDeadlineTrigger(pos,
  proc, step)` → поля `Process`/`Step`, `Pos()==pos`, реализует `TriggerSpec` (`var _ ast.TriggerSpec
  = (*ast.DeadlineTrigger)(nil)`). Сейчас RED (узла нет).
- [ ] **T004** Добавить `ast.DeadlineTrigger{specBase; Process, Step Ident}` + `NewDeadlineTrigger`
  + `func (*DeadlineTrigger) triggerSpec() {}` (`ast/trigger.go`, рядом с Metric/Event/Schedule).
  T003 → GREEN. **Инверсия: убрать `triggerSpec()` → compile-fail (не реализует TriggerSpec).**

## Phase 3: B4a Парсер — parseDeadlineTrigger + expectLexeme + SE-каскад

- [ ] **T005** [Тест ДО] `TestParseDeadlineTrigger` (`parser/parse_decl_test.go`): `когда задача
  просрочена в P.S:\n    печать(1)` → `*ast.DeadlineTrigger{Process:"P",Step:"S"}`, `Pos()`=токен
  `задача` (line/col). RED.
- [ ] **T006** [Тест ДО] `TestDeadlineTriggerMalformed` (`parser/parse_decl_test.go`): 5 негативов
  SE-EXPECTED (exact+pos) — `задача X`→`ожидалось 'просрочена', получено 'X'`; нет `в`→`ожидалось
  'в', получено 'P'`; нет процесса (`в .S`)→`ожидалось 'имя процесса', получено '.'`; нет `.`→`ожидалось
  '.', получено 'S'`; нет шага (`в P.`)→`ожидалось 'имя шага', получено '<лексема>'`. RED.
- [ ] **T007** Реализовать `expectLexeme(want string)` (`parser/parse_decl.go`, по образцу `expectCompOp`
  `:441`): `tok.Type==IDENT && tok.Lexeme==want` → advance; иначе `p.error(tok.Pos, msgExpected(want, tok))`.
- [ ] **T008** Реализовать `parseDeadlineTrigger` (`parser/parse_decl.go`): advance `задача`;
  `expectLexeme("просрочена")`; `expect(KW_IN,"в")`; `expect(IDENT,"имя процесса")`; `expect(DOT,".")`;
  `expect(IDENT,"имя шага")` → `NewDeadlineTrigger`. Ветка в `parseTriggerDecl` (`:406`): IDENT-лексема
  `задача` → `parseDeadlineTrigger`, иначе default → SE-TRIGGER-KIND. T005/T006 → GREEN.
- [ ] **T009** **Инверсия парсера**: временно убрать `expectLexeme("просрочена")` → `когда задача в P.S:`
  ложно парсится → T006 красный. Восстановить. Доказывает, что хелпер несёт проверку.
- [ ] **T010** [SE-TRIGGER-KIND каскад, Тест ДО→ИСТОЧНИК] Расширить `msgTriggerKind` (`errors.go:29`)
  `"метрика, событие или расписание"` → `"метрика, событие, расписание или задача"` + коммент `:26`.
  Со-обновить ВСЕ зеркала: `inventory_test.go:34`; ТРИ exact-match golden `parse_decl_test.go:1549`
  (`TestTriggerSyntaxDiagnostics`), `:1622` (`TestTriggerNegativesExactPos`), `:1666`
  (`TestGoldenTriggerSyntaxDiagnostics` двухстрочная). `когда мусор:` → новый текст.
- [ ] **T011** **Инверсия каскада**: оставить старый текст в `errors.go:29`, но обновить только
  тесты → зеркало `inventory_test.go:34`/golden `:1549` красное (текст разошёлся). Доказывает, что
  правят ИСТОЧНИК. Восстановить. Счётный замок `inventory_test.go:38` `wantCodes=14` НЕ менялся.
- [ ] **T012** **v1-замок (INV-3)** [P] `TestTaskIdentNotKeyword` (`parser/parse_decl_test.go` или
  `examples_test`): `пусть задача = 10`, `задача()` вне позиции триггера → parse-clean (IDENT, не
  триггер). **Инверсия: если `задача` стала бы глобальным KW — `пусть задача = 10` падает → этот тест
  ловит. Подтверждает D-AU-4 (лексер L=11 не тронут).**

## Phase 4: B4a Семпроход + run-заглушка

- [ ] **T013** [Тест ДО] `TestDeadlineTriggerSemantics` (`eval/analyze_trigger_test.go`): позитив
  (процесс+шаг объявлены, тело со свободным `факт`) → analyze OK; негатив-а неизв. процесс →
  `процесс '<имя>' не объявлен` (exact+pos `Process`); негатив-б неизв. шаг → `шаг '<шаг>' не найден в
  процессе '<процесс>'`; негатив-в `значение`/`событие` в теле → наследуемый TR-VAL-CTX/TR-EVT-CTX. RED.
- [ ] **T014** Реализовать кейс `*ast.DeadlineTrigger` в `checkTrigger` (`eval/analyze.go:319`):
  (а) процесс объявлен (переиспользовать `процесс '%s' не объявлен` `:729`); (б) шаг существует в
  `pd.Steps`; (в) `checkTriggerBody(td.Body, false, false)` (как расписание, lenient-scope D-AU-6).
  T013 → GREEN.
- [ ] **T015** **eval=28 замок (INV-5)**: `errors_golden_test.go` len(seen)==28 НЕ изменилось
  (семантические триггерные коды — в `analyze_trigger_test.go`-семействе, не инкрементят 28).
  **Инверсия: если новый код попал в errors_golden → 28 ломается → ловит.**
- [ ] **T016** [Тест ДО] `TestDeadlineTriggerRunStub` (`eval/trigger_run_test.go` или
  `cmd/ladix/golden_test.go`): `ladix run` файла с эскалация-триггером → exit 0 + stdout `задача
  триггер 'P.S' требует serve (фича 007b)`, тело НЕ исполнено. RED.
- [ ] **T017** Реализовать run-заглушку (`eval/trigger_run.go`, кейс `*ast.DeadlineTrigger` в
  switch `RunTriggers` — экспортный метод `*Interpreter`, рядом с событие/расписание `:49/:51`):
  `fmt.Fprintf(w, "задача триггер '%s.%s' требует serve (фича 007b)\n", spec.Process.Name,
  spec.Step.Name)`. T016 → GREEN. **Инверсия: исполнять тело под run → тест ловит побочный вывод.**
- [ ] **T018** [P, опц.] Витрина B4a: новый чисто-парсящийся `examples/контроль_плана.ladix` (срез §2:
  процесс+человеческий шаг+`срок:`+эскалация-триггер) → добавить в `clean[]` (`examples_test.go`,
  ЖИВОЙ набор 24→25) + `examples/MANIFEST.md`. Негативный `examples/ошибка_эскалация.ladix`
  (малформенный) → golden-замок `cmd/ladix/golden_test.go`, НЕ в clean[]. **`ошибочная.ladix` НЕ
  перезаписывать.**

### B4a GATE
- [ ] **T019** B4a-гейт: `go test ./...` + `vet` + race зелёные; L=11, SE=14, eval=28 целы; §EN-7
  пины целы; все зеркала SE-TRIGGER-KIND зелёные; v1-замок зелёный. Фронтенд готов под B4b.

---

## === B4b — БЭКЕНД durable (Phase 5–7) — РЕАЛИЗУЕТСЯ ВТОРЫМ, ПОСЛЕ B4a-гейта ===

## Phase 5: B4b Store — Task.Escalated + 4 точки кодека

- [ ] **T020** [Тест ДО] `TestTaskEscalatedCodec` (`store/sqlite_test.go`): round-trip
  `SaveTask{Escalated:true}` → новый `SQLiteStore` на той же `--db` → `ListPendingTasks`/`LoadTask` →
  `Escalated==true`; UPSERT `false`→`true` (тот же ID) → перечитать → `true`. RED (поля/колонки нет).
- [ ] **T021** Добавить `Task.Escalated bool` (`store/types.go`, после `CompletedAt`). 4 точки
  SQLite-кодека (`sqlite.go`): (1) DDL `:33` +`escalated INTEGER NOT NULL DEFAULT 0`; (2) `SaveTask`
  INSERT-список `:161`; (3) `ON CONFLICT … DO UPDATE SET escalated=…` `:165`; (4) SELECT-читатели
  `buildTask`/`scanTask` `:296/310` (+`LoadTask :179`, `ListPendingTasks :186`). `memory.go` copyTask
  несёт bool тривиально. T020 → GREEN.
- [ ] **T022** **Инверсии 4 точек (мутпробы)** [P]: (точка 3) убрать `escalated` из ON CONFLICT →
  UPSERT-тест красный; (точка 4) убрать `escalated` из SELECT/scanTask → round-trip даёт `false` →
  красный. Восстановить. Доказывает целостность всех точек.
- [ ] **T023** **Store=15 замок (INV-4)**: интерфейс `Store` НЕ растёт (`ListTasksByInstance` — B6,
  НЕ здесь). Тест-компиляция `var _ store.Store = (*store.SQLiteStore)(nil)` + ручной счёт методов.

## Phase 6: B4b Daemon — 4-я фаза tick + checkDeadlines + fireDeadlineBody

- [ ] **T024** [Тест ДО, INV-1] `TestTickFourPhasesOrder` (`daemon/tick_test.go`): живой daemon-тест
  с метрика+расписание+эскалация в одном `tick()`; первые три фазы (`ResetRunState→drainEvents→
  evalMetrics→checkSchedules`) отрабатывают как до B4 (порядок/идемпотентность), `checkDeadlines` —
  в хвосте под тем же `d.mu`. RED (фазы нет).
- [ ] **T025** [Тест ДО] `TestCheckDeadlinesFire` (`daemon/checkdeadlines_test.go`): инстанс+задача с
  дедлайном, эскалация-триггер; Clock до срока → тишина; Clock за срок → `fireDeadlineBody` исполнено
  `[уведомление] руководитель: <факт>`, `Escalated=true`; нет эскалация-триггеров → ранний `return`
  без `ListPendingTasks`; инжект `факт` из `inst.Variables`. RED.
- [ ] **T026** Добавить 4-ю фазу `d.checkDeadlines()` в хвост `tick()` (`daemon/tick.go:10`) под тем
  же `d.mu`, ПОСЛЕ `checkSchedules`; первые три НЕ трогать. T024 → GREEN.
- [ ] **T027** Реализовать `checkDeadlines` + `fireDeadlineBody` (новый `daemon/checkdeadlines.go`)
  по контракту `tick-phase-checkdeadlines.md`: фильтр триггеров → ранний return → ОДИН
  `ListPendingTasks("")` → `if t.Escalated continue` → `if !engine.Overdue(t,now) continue` →
  `LoadInstance` → совпадение шаг/процесс → `safeFire(fireDeadlineBody(td.Body, inst.Variables))` →
  `t.Escalated=true; SaveTask(t)` → `break`. `fireDeadlineBody`: `NewTriggerBodyEnv` + цикл `Define`
  по ВСЕМ `vars` (D-AU-6) + `EvalBlockInTrigger`. T025 → GREEN.
- [ ] **T028** **Инверсия INV-1**: переставить `checkDeadlines` ПЕРЕД `checkSchedules` → если есть
  зависимость по кешам/порядку, живой daemon-тест T024 ловит сдвиг; либо порядок-замок красный.
  Восстановить хвостовую позицию.

## Phase 7: B4b Durable × рестарт (ОБЯЗАТЕЛЬНЫЙ golden + мутпроба)

- [ ] **T029** [Тест ДО, INV-2] `TestDeadlineDurableRestart` (`daemon/checkdeadlines_test.go`,
  Go-API §AU-12.B): SQLiteStore(demo.db) + `engine.Start("эскалация_плана",[2500000])`; FixedClock=
  created → tick → тишина; Clock+=3дн → tick → РОВНО одна эскалация (`[уведомление] руководитель:
  2500000`), `Escalated` персистнут; tick снова → нет повтора; РЕСТАРТ (новый SQLiteStore на той же
  `--db`) + `RunRestartScan` + tick → нет повтора; assert уведомление РОВНО один раз за все прогоны.
  Замки (а) единичность, (б) персист SQLite, (в) аддитивность 4-й фазы. RED.
- [ ] **T030** Прогнать T029 на реализации (T021+T027) → GREEN. Граничные: задача завершена до
  просрочки → нет эскалации; задача эскалирована→завершена → штатно.
- [ ] **T031** **МУТПРОБА durable (INV-2, несущая, замок г §AU-12.B)**: снять `if t.Escalated {
  continue }` в `checkDeadlines` → T029 КРАСНЕЕТ (двойная эскалация на шаге «tick снова»/«после
  рестарта»). Восстановить фильтр. Это главный замок одноразовости×рестарта.
- [ ] **T032** **Мутпробы кодек→durable (доп.)**: пропуск точки 3 (ON CONFLICT escalated) → на
  рестарте `Escalated=false` → T029 красный; пропуск точки 4 (SELECT escalated) → уже без рестарта
  `false` → красный. Подтверждает: durable-golden = интегральный замок 4 точек кодека.
- [ ] **T033** [P, опц.] Не-молчащее демо (§AU-12.A, INV-1/риск #4): живой daemon-тест с метрикой
  «выше порога → падает ниже во время демона» → до пересечения тишина, на пересечении РОВНО одно
  edge-fire (`LastBool != nil && !*LastBool && cur`), re-arm не повторяет. (Полный CLI end-to-end
  §AU-12.C — на M2-гейте после B5.)

### B4b GATE
- [ ] **T034** B4b-гейт: `go test ./...` + `vet` + `-race` зелёные; durable-golden зелёный +
  мутпробы кусают; ProcessRuntime=8, Store=15; §EN-7/007b golden целы; детерминизм (FixedClock).
  Зафиксировать осознанную последовательность: CLI §AU-12.B/§AU-12.C — на M2-гейте ПОСЛЕ B5/B6.

---

## Зависимости (топология)

- **Setup (T001-T002)** → всё.
- **B4a**: T003→T004 (AST); T004→T005-T012 (парсер/каскад); T004→T013-T017 (семпроход/run);
  T018 опц.; **T019 B4a-гейт закрывает ВСЮ B4a**.
- **B4b ЗАВИСИТ от T019**: T020→T021→T022/T023 (Store); T021→T024-T028 (daemon, тело эскалации
  использует AST из B4a); T021+T027→T029-T032 (durable); T033 опц.; **T034 B4b-гейт**.
- Внутри фаз `[P]` — разные файлы.

## Замки-инверсии (сводка несущих INV)

| INV | Замок | Мутпроба (инверсия) | Задачи |
|-----|-------|---------------------|--------|
| INV-1 (007b) | живой daemon-тест 4 фаз | переставить `checkDeadlines` перед `checkSchedules` → красный | T024,T026,T028 |
| INV-2 (durable) | `TestDeadlineDurableRestart` | снять `if t.Escalated` → красный | T029,T031 |
| INV-2 (кодек) | round-trip + durable | пропуск точки 3/4 → escalated теряется → красный | T020,T022,T032 |
| INV-3 (фронтенд) | v1 `пусть задача=10` parse-clean; SE-каскад | `задача` как KW → v1 красный; старый текст → зеркало красное | T010,T011,T012 |
| INV-4 (швы) | ProcessRuntime=8, Store=15 | новый метод Store → счёт красный | T023,T034 |
| INV-5 (счётчики) | L=11/SE=14/eval=28 | новый код в errors_golden → 28 красный | T015,T019,T034 |

## Дословные тексты (Принцип VIII — НЕ переформулировать)

- run-заглушка: `задача триггер '<процесс>.<шаг>' требует serve (фича 007b)`
- SE-TRIGGER-KIND: `ожидалось 'метрика, событие, расписание или задача', получено '<лексема>'`
- SE-EXPECTED: `ожидалось '<want>', получено '<лексема>'` (want ∈ просрочена/в/имя процесса/./имя шага)
- семантика: `процесс '<имя>' не объявлен` ; `шаг '<шаг>' не найден в процессе '<процесс>'`
- stdout эскалации: `[уведомление] руководитель: <значение>`
