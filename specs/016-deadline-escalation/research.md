# Research: B4 эскалация дедлайна — прецеденты и эмпирика (@a92ad50)

Источник истины §AU-6/§AU-2/§AU-10/§AU-12. Ниже — прецеденты существующего кода, на которые
B4 опирается зеркально, с эмпирическими строками (сверены в `src/` на master @a92ad50).

## B4a — фронтенд

### Прецедент: три существующих вида триггера

`parseTriggerDecl` (`internal/parser/parse_decl.go:402-417`) — диспетчер после `KW_WHEN`:
- `case lexer.KW_METRIC` → `parseMetricTrigger` (`:410`/`:430`)
- `case lexer.KW_EVENT` → парс события (`:412`)
- `case lexer.KW_SCHEDULE` → `parseScheduleTrigger` (`:414`/`:460`)
- `default` → `p.error(p.peek().Pos, msgExpected(msgTriggerKind, p.peek()))` (`:417`) → **SE-TRIGGER-KIND**.

**Решение B4a:** добавить ветку ПЕРЕД default: если `p.peek().Type == lexer.IDENT &&
p.peek().Lexeme == "задача"` → `parseDeadlineTrigger()`. Это сохраняет лексер контекстно-
независимым (D-AU-4): `задача` — обычный IDENT, контекст применяет парсер.

### Прецедент: хелперы expect/error

- `expect(t lexer.TokenType, expected string)` (`parser.go:79`) — общий; даёт SE-EXPECTED через
  `msgExpected` при несовпадении ТИПА токена. Годится для `в` (KW_IN), `.` (DOT), `имя процесса`/
  `имя шага` (IDENT).
- `error(pos, msg)` (`parser.go:98`); `errorLocal` (`parser.go:111`).
- `expectCompOp()` (`parse_decl.go:441`) — **прецедент локального expect-хелпера в parse_decl.go**;
  по его образцу пишется НОВЫЙ `expectLexeme(want string)` (сверяет `tok.Type==IDENT &&
  tok.Lexeme==want`, иначе `p.error(pos, msgExpected(want, tok))`). Нужен потому, что `просрочена`
  — IDENT-лексема, а `expect` сверяет только ТИП (IDENT матчит любую идентификатор-лексему).
- `msgExpected(expected string, got lexer.Token)` (`errors.go:59`) — шаблон `ожидалось '…',
  получено '<лексема>'`.

### Прецедент: msgTriggerKind и его зеркала

`const msgTriggerKind = "метрика, событие или расписание"` (`errors.go:29`); коммент `errors.go:26`
цитирует текст. **РАСШИРЯЕТСЯ** до `"метрика, событие, расписание или задача"` (§AU-10.A). Тот же
код SE-TRIGGER-KIND (счёт SE=14 НЕ меняется). Зеркала старого текста (со-обновить, иначе `go
test` красный):
- `parser/inventory_test.go:34` — fragment-match `"метрика, событие или расписание"`.
- `parser/parse_decl_test.go:1549` `TestTriggerSyntaxDiagnostics` — exact `ожидалось 'метрика,
  событие или расписание', получено 'x'`.
- `parser/parse_decl_test.go:1622` `TestTriggerNegativesExactPos` — exact `… получено 'X'`, line/col.
- `parser/parse_decl_test.go:1666` `TestGoldenTriggerSyntaxDiagnostics` — двухстрочная golden
  `Ошибка в строке 1, колонка 7:\nожидалось 'метрика, событие или расписание', получено 'X'`.

### Прецедент: семпроход checkTrigger

`checkTrigger(td *ast.TriggerDecl)` (`eval/analyze.go:319`) — switch по `td.Spec.(type)`:
- `*ast.MetricTrigger` (`:321`): резолв метрики + `checkTriggerBody(td.Body, true, false)` (`:342`).
- `*ast.EventTrigger` (`:343`): `checkTriggerBody(td.Body, false, true)` (`:344`).
- `*ast.ScheduleTrigger` (`:345`): проверки формата + `checkTriggerBody(td.Body, false, false)` (`:368`).

**Решение B4a:** кейс `*ast.DeadlineTrigger` зеркалит РАСПИСАНИЕ — `checkTriggerBody(td.Body,
false, false)` (lenient-scope, D-AU-6; `значение`/`событие` в теле запрещены наследуемым
TR-VAL-CTX/TR-EVT-CTX). Перед телом: проверка процесса/шага.

`checkTriggerBody(body, inMetricTrigger, inEventTrigger)` (`analyze.go:408`) сам не проверяет
объявленность плоских Ident (lenient) → свободный `факт` статической ошибки не даёт.

### Прецедент: формулировки семошибок процесс/шаг

- `процесс '%s' не объявлен` — `analyze.go:729` (в `checkRunProcess`, `semErr(r.Pos(), …)`).
  **Переиспользуется** для «процесс эскалации не объявлен» (§AU-6.1.3/§AU-10.B).
- Шаги процесса: `pd.Steps` (`ast/process.go`), итерация `for _, step := range pd.Steps`
  (`analyze.go:277/285/297/303`). «шаг не найден в процессе» — новый exact-match
  `шаг '<шаг>' не найден в процессе '<процесс>'` (§AU-10.B). Прецедент текста с обоими именами:
  `шаг '%s' после '%s', но шаг '%s' не объявлен` (`analyze.go:289`).

### Прецедент: run-заглушка

`internal/eval/trigger_run.go` — `RunTriggers` (экспортный метод `*Interpreter`; §AU-6.1.4 якорь
пишет `runTriggers` строчными, реальный символ — `RunTriggers`), диспетчер `switch td.Spec.(type)`,
печатает заглушки в порядке объявления:
- событие: `fmt.Fprintf(w, "событие триггер '%s' требует serve (фича 007b)\n", spec.Event.Name)` (`:49`)
- расписание: `fmt.Fprintf(w, "расписание триггер '%s' требует serve (фича 007b)\n", scheduleName(spec.Spec))` (`:51`)

**Решение B4a:** ветка для `*ast.DeadlineTrigger`:
`fmt.Fprintf(w, "задача триггер '%s.%s' требует serve (фича 007b)\n", spec.Process.Name, spec.Step.Name)`.

## B4b — бэкенд

### Прецедент: Task struct + кодек

`Task` (`store/types.go:48-57`): поля `ID/InstanceID/StepName/Assignee/Deadline/Status/
CreatedAt/CompletedAt`. **Нет `Escalated`** — добавляется (после `CompletedAt`). `TaskPending =
"открыта"` (`:43`), `TaskCompleted = "завершена"` (`:44`).

SQLite-кодек `Task` (4 точки, §AU-2):
1. DDL `CREATE TABLE IF NOT EXISTS tasks (…)` (`sqlite.go:33`) — +`escalated INTEGER NOT NULL DEFAULT 0`.
2. `SaveTask` INSERT-список колонок (`sqlite.go:161`).
3. `ON CONFLICT(id) DO UPDATE SET …` (`sqlite.go:165`) — UPSERT.
4. SELECT-читатели через `scanTask(id, row)` (`sqlite.go:296`) + `buildTask(id, instanceID,
   stepName, assignee, deadline, status, createdAt, completedAt)` (`sqlite.go:310`); вызываются
   из `LoadTask` (`:179`) и `ListPendingTasks(assignee)` (`:186` — ГЛАВНЫЙ читатель скана).

`MemoryStore.copyTask` несёт bool тривиально (`cp := *t`). Миграция = сброс схемы (D-AU-9).
Store-контракт остаётся **15 методов** (колонка, не метод; `ListTasksByInstance` — B6).

### Прецедент: tick-фазы

`tick()` (`daemon/tick.go:10-16`): `d.mu.Lock(); defer d.mu.Unlock(); d.interp.ResetRunState();
d.drainEvents(); d.evalMetrics(); d.checkSchedules()`. **Решение B4b:** добавить
`d.checkDeadlines()` ПОСЛЕ `checkSchedules`, под тем же `d.mu` — аддитивно в хвост, не меняя
первые три.

### Прецедент: fireBody / инжект / барьер

`fireBody(body, inj injection)` (`fire.go:22`): `env := d.interp.NewTriggerBodyEnv()` (`:23`,
= `NewEnvironment(global)` + `markBoundary`, `trigger_daemon.go:60`); `env.Define(inj.name,
inj.val)`; `return d.interp.EvalBlockInTrigger(env, body)` (`:27`). `injection` struct (`fire.go:10`)
— ОДИН name/val.

**Решение B4b:** `fireDeadlineBody(body, vars map[string]value.Value)` — НОВАЯ функция, инжектит
ВСЕ переменные циклом `for k,v := range vars { env.Define(k, v) }`, минуя `injection`-конверт
(struct не расширяется). `vars` = `inst.Variables`, уже загружено в `checkDeadlines` (без
round-trip `InstanceVariables`). Read-only барьер (TR-BODY-RO) наследуется от `NewTriggerBodyEnv`.

### Прецедент: скан, просрочка, рестарт, часы

- `engine.Overdue(t *store.Task, now time.Time)` (`engine/format.go:35`): `t.Deadline != nil &&
  now.After(*t.Deadline)`. Единый источник просрочки (нет off-by-one на `now==Deadline`).
- `RunRestartScan()` (`daemon/restart.go:28`) реактивирует через `ListInstancesByStatus(status)`
  (`restart.go:32`) по `[Running, Created]`. Эскалация-состояние НЕ в TriggerState — в
  `Task.Escalated`, перечитывается из SQLite через `ListPendingTasks`.
- `safeFire(fn)` (007b) — изоляция тела триггера (паника/ошибка → лог, демон не падает).
- Часы: `d.clock.Now()` (`engine.Clock`, инжектируется в демон `daemon.go:25`). Тот же Clock в
  движке/интерпретаторе (007b «двойные часы через адаптер `evalClockFromEngine`» сведены к
  одному источнику в serve). Golden продвигает FixedClock за дедлайн.

## Расхождение якоря (зафиксировано, без догадок)

§AU-11.1 говорит «clean[] на момент M2 — 22 файла», но эмпирически `examples_test.go:12-35`
содержит **24 файла** (M1 коннекторы/окна уже влиты). Якорь устарел на 2; B4 добавляет новые
чисто-парсящиеся примеры в ЖИВОЙ clean[] (не в гипотетический 22-файловый). Негативные
примеры → golden-замки `cmd/ladix/golden_test.go`, не в clean[].

## Эмпирическая сводка строк (@a92ad50)

| Якорь | Файл:строка |
|---|---|
| `msgTriggerKind` / коммент | `parser/errors.go:29` / `:26` |
| `parseTriggerDecl` диспетчер | `parser/parse_decl.go:406-417` |
| `expect` / `error` / `errorLocal` | `parser/parser.go:79 / :98 / :111` |
| `expectCompOp` (прецедент локального хелпера) | `parser/parse_decl.go:441` |
| `msgExpected` | `parser/errors.go:59` |
| `checkTrigger` / расписание-кейс `checkTriggerBody(…,false,false)` | `eval/analyze.go:319 / :368` |
| `процесс '%s' не объявлен` | `eval/analyze.go:729` |
| run-заглушки событие/расписание | `eval/trigger_run.go:49 / :51` |
| `tick()` 3 фазы | `daemon/tick.go:10-16` |
| `Task` struct | `store/types.go:48-57` |
| DDL/SaveTask/ONCONFLICT/LoadTask/ListPending/scanTask/buildTask | `store/sqlite.go:33/161/165/179/186/296/310` |
| `fireBody`/`injection`/`NewTriggerBodyEnv`/`EvalBlockInTrigger` | `daemon/fire.go:10/22-27` |
| `RunRestartScan`/`ListInstancesByStatus` | `daemon/restart.go:28/32` |
| `engine.Overdue` | `engine/format.go:35` |
| clean[] (24 файла) | `parser/examples_test.go:12-35` |
