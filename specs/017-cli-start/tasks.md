---
description: "Task list — B5 ladix start (017): CLI-запуск инстанса, типизир. литералы argv, сверка арности"
---

# Tasks: B5 — `ladix start <процесс> [аргументы]` (CLI)

**Input**: `/specs/017-cli-start/` (spec.md, plan.md, research.md, data-model.md, contracts/, quickstart.md).
Якорь — `docs/automation-model.md` §AU-7 / §AU-9 / §AU-10 / §AU-4.5 / D-AU-7 / D-AU-10.

**Tests**: ВКЛЮЧЕНЫ и tests-first (Принцип VI). Каждый несущий инвариант (INV-1..4) + критичные
требования имеют замок + инверсию (мутпроба «мутация → красный»). Особо УСИЛЕНЫ инверсии:
- **(a) арность-замок**: N≠M → exit 2 дословно → красный при сдвиге текста/логики;
- **(b) каждый тип литерала** парсится верно + плохой литерал дословно → красный;
- **(c) неизв. процесс** дословно (НЕ текст движка) → красный;
- **(d) stdout канон** exact-match (golden маскирует `<DT>`) → красный при сдвиге;
- **(e) единая `--db`/openStore** без регресса complete/tasks/emit (замок).

**Organization**: CLI-only. Прод-код — только `cmd/ladix/`. eval/engine/store/value — ПУСТОЙ дифф
(INV-1). `[P]` — разные файлы/без зависимостей.

## Path Conventions

- Прод: `src/cmd/ladix/start.go` (новый: `startMain` + `parseArgLiteral`), `src/cmd/ladix/main.go`
  (+ветка switch `start`, +`usage`-строка, +хелпер `openStore`).
- Тесты: `src/cmd/ladix/start_golden_test.go` (новый), `src/cmd/ladix/start_literal_test.go` (новый,
  табличный парсер литералов). Регресс-замки — существующие `cmd/ladix/{golden_test,payload_cli_test,
  emit_golden_test,main_test}.go`.
- Фикстура: `src/examples/эскалация_плана.ladix` (новая ИЛИ переиспользовать существующую 016-фикстуру);
  `src/examples/MANIFEST.md` (регистрация при добавлении).
- Все пути — от корня `/Users/denis/dev/ladix`. Строки якоря @a1ad856 (импл сверяет — могут сдвинуться).

---

## Phase 1: Setup (база регресса)

- [ ] **T001** Сверить базу @a1ad856: switch подкоманд (`main.go` — run/metric/complete/tasks/serve/emit),
  `usage`-строка (`main.go:71`), `engine.Start(name,args)(string,error)` (`engine.go:65`) + текст
  `процесс '%s' не найден в определении` (`:69`), `printTaskCreated` (`engine.go:461`, `deadlineLayout`
  `format.go:11`), `interp.Process` (`exports.go:19`), `ProcessDecl.Params` (`ast/process.go:10`),
  `parseWebhookCaller`/`withExternalCallerOpt` (`main.go:39/60`), инлайн-Store (`main.go:235-244` и 4
  зеркала), `value.{Целое,Дробное,Строка,Булево,None,Дата}`, `idMaskRE` (`trigger_golden_test.go:70`).
  Подтвердить `ProcessRuntime`=8, Store=16, L=11/SE=14/eval=28.
- [ ] **T002** `go test ./...` + `go vet ./...` + `-race` (где применимо) на чистом дереве — зелёные.
  Зафиксировать счётчики L=11/SE=14/eval=28 + §EN-7 пины + complete/tasks/emit golden как базу регресса.

---

## Phase 2: Foundational — парсер argv-литералов (блокирует startMain)

- [ ] **T003** [Тест ДО, P] **(инверсия b)** `TestParseArgLiteralTypes` (`start_literal_test.go`):
  табличный — `2500000`→`Целое{2500000}`, `-42`→`Целое{-42}`, `0`→`Целое{0}`, `3.14`→`Дробное{3.14}`,
  `-0.5`→`Дробное{-0.5}`, `1e3`→`Дробное{1000}`, `истина`→`Булево{true}`, `ложь`→`Булево{false}`,
  `пусто`→`None`, `2026-01-01`→`Дата{2026,1,1}`, `перезвонит`→`Строка`, `2 500 000`→`Строка`. RED (нет функции).
- [ ] **T004** [Тест ДО, P] **(инверсия b)** `TestParseArgLiteralErrors` (`start_literal_test.go`):
  `99999999999999999999`→ошибка `целое вне диапазона типа Целое`; `2026-13-45`→ошибка парса даты. RED.
- [ ] **T005** Реализовать `parseArgLiteral(argv string) (value.Value, error)` (`start.go`) — порядок
  data-model §1.2 (целое→дробное→булево/пусто→дата(ISO)→строка), ручные проверки + `strconv.ParseInt/
  ParseFloat` + `time.Parse("2006-01-02")` с re-format проверкой. T003/T004 → GREEN.
  **Инверсия: перенести fallback-Строку ПЕРЕД числом → T003 (`2500000`→Строка) красный.**
- [ ] **T006** **(инверсия b)** Замок диапазона: убрать проверку `strconv.ParseInt` overflow (вернуть
  Строку молча на `99999999999999999999`) → T004 ДОЛЖЕН покраснеть. Подтвердить мутпробой, восстановить.

---

## Phase 3: openStore + каркас startMain (US1, US4)

- [ ] **T007** [Тест ДО] **(инверсия e)** `TestOpenStoreSelectsBackend` (`start_golden_test.go` или
  отдельный): `openStore("")` → MemoryStore + no-op close; `openStore(tmp.db)` → SQLiteStore + Close,
  файл создан. RED (нет хелпера).
- [ ] **T008** Реализовать `openStore(dbPath) (store.Store, func() error, error)` (`main.go` или `start.go`)
  — узкий снимок `main.go:235-244` (contract open-store.md). T007 → GREEN.
- [ ] **T009** **(инверсия e, регресс)** Прогнать существующие golden `complete`/`tasks`/`emit`
  (`payload_cli_test.go`, `main_test.go`, `emit_golden_test.go`) — ДОЛЖНЫ остаться зелёными после ввода
  `openStore` (и опц. рефактора под него). Если рефактор красит golden — откатить рефактор, `start`
  всё равно использует хелпер (минимальная правка). Замок: complete/tasks/emit поведение не изменилось.
- [ ] **T010** Каркас `startMain(rest []string, stdout, stderr io.Writer) int` (`start.go`): разбор
  флагов (`--db` дефолт `ladix.db`, `--вебхук`, `--max-depth`) + позиционные (файл, процесс, аргументы);
  usage/неизв.флаг/флаг-без-значения exit 2 (паритет emit). Ветка switch `start` в `main.go` +
  расширить `usage`-строку записью start. Компиляция файла (lex→parse→Analyze), ошибка → exit 1.

---

## Phase 4: Сверка арности + неизв. процесс (US3) — ДО engine.Start

- [ ] **T011** [Тест ДО] **(инверсия a)** `TestStartArityMismatch` (`start_golden_test.go`): процесс
  `эскалация_плана(факт)` (1 параметр); `start f эскалация_плана` (0 арг) → stderr
  `ladix: процесс 'эскалация_плана' ожидает 1 аргументов, получено 0` exit 2; `start f эскалация_плана 100 200`
  (2 арг) → `… получено 2` exit 2. RED.
- [ ] **T012** [Тест ДО] **(инверсия c)** `TestStartUnknownProcess` (`start_golden_test.go`):
  `start f неизвестный 5` → stderr `ladix: процесс 'неизвестный' не объявлен` exit 2 (НЕ текст движка
  `не найден в определении`). RED.
- [ ] **T013** Реализовать в `startMain`: `pd, ok := interp.Process(name)`; `!ok` → FR-015 exit 2;
  `len(pd.Params) != len(posArgs)` → FR-014 exit 2. ОБЕ проверки ДО `engine.Start`. T011/T012 → GREEN.
  **Инверсия (a): убрать арность-проверку (положиться на engine.Start) → T011 красный (нет exit 2 / др. текст).**
  **Инверсия (c): убрать `!ok`-проверку (положиться на engine.Start текст) → T012 красный (текст движка).**
- [ ] **T014** [Тест ДО→GREEN] `TestStartZeroArity` (`start_golden_test.go`): процесс без параметров,
  `start f P` (0 арг) → старт проходит exit 0 (арность 0=0).

---

## Phase 5: engine.Start + stdout канон + вебхук (US1, US2 интеграция)

- [ ] **T015** [Тест ДО] **(инверсия d)** `TestStartGoldenCanon` (`start_golden_test.go`): фикстура с
  человеческим первым шагом + `срок:`; `start f эскалация_плана 2500000 --db tmp.db` → stdout (маска `<DT>`):
  ```
  [задача] t-000001 → менеджер, шаг 'связаться_с_клиентом', срок до <DT>
  запущен инстанс p-000001
  ```
  exit 0. RED (нет вызова Start + строки `запущен инстанс`).
- [ ] **T016** Реализовать в `startMain`: `parseWebhookCaller` → `withExternalCallerOpt` → `NewEngine` →
  `SetProcessRuntime` → `id, err := eng.Start(name, posArgs)` → при успехе `fmt.Fprintf(stdout,
  "запущен инстанс %s\n", id)`. T015 → GREEN. Связать парсинг argv (Phase 2) в `posArgs`.
  **Инверсия (d): изменить строку `запущен инстанс` (напр. `запущен %s`) → T015 красный.**
- [ ] **T017** [Тест ДО→GREEN] `TestStartTerminalNoTasks` (`start_golden_test.go`): терминальный процесс
  без человеческих задач → stdout = ровно `запущен инстанс p-000001` (движок тих, нет `[задача]`).
- [ ] **T018** [Тест ДО] **(инверсия b конец-в-конец)** `TestStartLiteralBinding` (`start_golden_test.go`):
  `start f P 2500000` связывает `факт` с `Целое{2500000}` (проверка через печать значения в первом шаге
  ИЛИ через inspect-независимый канал — печать `[задача]`/значения переменной). RED→GREEN.
- [ ] **T019** [Тест ДО] `TestStartBadLiteralCLI` (`start_golden_test.go`): `start f P 99999999999999999999`
  → stderr `ladix: не удалось разобрать аргумент '99999999999999999999': целое вне диапазона типа Целое`
  exit 2 (CLI-уровень, до engine.Start). RED→GREEN (через Phase 2 + проводку ошибки в startMain).
- [ ] **T020** [Тест ДО] **(вебхук)** `TestStartBadWebhookURL` (`start_golden_test.go`):
  `start f P 1 --вебхук "://плохо"` → stderr `ladix: неверный URL вебхука '://плохо'` exit 2.
  `start f P 1 --db tmp.db` без вебхука → дефолт-стаб, exit 0. RED→GREEN (переиспользовать
  `parseWebhookCaller`). **Инверсия: не звать parseWebhookCaller → невалидный URL не отлавливается (нет exit 2).**

---

## Phase 6: Дефолт `--db` + витрина + регресс (US1-4, INV-2/3)

- [ ] **T021** [Тест ДО→GREEN] **(инверсия e)** `TestStartDefaultDB` (`start_golden_test.go`): `start f P 1`
  без `--db` → пишет в SQLite `ladix.db` (НЕ Memory): инстанс читается из `ladix.db` после команды
  (через `tasks`/прямой Store-read в тесте). **Инверсия: дефолт `""` (Memory) → инстанс не персистнут → красный.**
- [ ] **T022** [P] Фикстура `examples/эскалация_плана.ladix` (если не переиспользуем 016): процесс с
  параметром `факт`, человеческим первым шагом `связаться_с_клиентом` (`исполнитель: менеджер`, `срок:`).
  Чисто парсится. Добавить в `clean[]` (`examples_test.go`) ЕСЛИ примером языка + зарегистрировать в
  `examples/MANIFEST.md` (§AU-11.1). (Если только golden-фикстура start — по правилу §AU-11.1;
  негатив — не в clean[].)
- [ ] **T023** **(регресс, INV-3)** `go test ./...` + `go vet ./...` (+`-race` где есть) — ВЕСЬ v1/M1/M-DX/
  007b/durable golden зелёный; §EN-7 пины целы; счётчики L=11/SE=14/eval=28 НЕ изменены (CLI-ошибки —
  stderr cmd, не каталог). Подтвердить `ProcessRuntime`=8, Store=16 не тронуты.

---

## Phase 7: Полировка

- [ ] **T024** [P] Комментарии-якоря в `start.go`: ссылки §AU-7/§AU-9/§AU-10/§AU-4.5, заметка «арность
  проверяется ДО engine.Start (движок даёт др. текст engine.go:69)», «вебхук в start — паритет complete
  §AU-4.5». `gofmt`.
- [ ] **T025** [P] Сверка дословности всех текстов с §AU-10.C/§AU-10.D (4 CLI-ошибки + 2-строчный stdout
  канон) — буква-в-букву. Проверить, что `usage`-строка обновлена и связна.
- [ ] **T026** Финальный прогон `go build ./...` (один бинарник, CGO нет) + `go test ./cmd/ladix/...`
  + полный `go test ./...`. 0 новых зависимостей (`go.mod` дифф пуст). Детерминизм golden (свежая БД →
  p-000001/t-000001; `<DT>`-маска).

---

## Dependencies & Order

- Phase 1 (T001-T002) → всё.
- Phase 2 (T003-T006, парсер литералов) — Foundational, блокирует T016/T018/T019.
- Phase 3 (T007-T010) — openStore + каркас; T010 блокирует Phase 4/5.
- Phase 4 (T011-T014, арность/неизв.процесс) ДО Phase 5 (engine.Start).
- Phase 5 (T015-T020) — ядро запуска + канон + вебхук.
- Phase 6 (T021-T023) — дефолт БД + витрина + регресс.
- Phase 7 (T024-T026) — полировка.
- `[P]`: T003∥T004 (один файл — по содержимому последовательно, но независимы по логике); T022/T024/T025
  — разные файлы.

## Замки ↔ инварианты (карта)

| Инвариант / требование | Замок | Инверсия |
|------------------------|-------|----------|
| (a) арность N≠M → exit 2 дословно | T011 | T013-инв (убрать проверку → красный) |
| (b) каждый тип литерала + плохой | T003/T004/T018/T019 | T005-инв/T006 (порядок/overflow → красный) |
| (c) неизв.процесс дословно (не движок) | T012 | T013-инв (`!ok` убрать → текст движка → красный) |
| (d) stdout канон exact-match | T015/T017 | T016-инв (сдвиг строки → красный) |
| (e) единая `--db`/openStore без регресса | T007/T009/T021 | T021-инв (дефолт Memory → не персист → красный) |
| INV-1 CLI-only (ProcessRuntime=8, Store=16) | T001/T023 | — (компиляция/счётчики) |
| INV-3 каталог L=11/SE=14/eval=28 цел | T002/T023 | — |
| вебхук в start (§AU-4.5) | T020 | T020-инв (не звать parseWebhookCaller → красный) |
