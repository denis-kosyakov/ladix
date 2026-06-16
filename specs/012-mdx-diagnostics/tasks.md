---
description: "Task list — 012-mdx-diagnostics (веха M-DX)"
---

# Tasks: M-DX «Диагностика и восстановление парсера»

**Input**: Design documents from `/specs/012-mdx-diagnostics/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/ (parser-recovery.md, diagnostics-catalog.md)

**Tests**: ВКЛЮЧЕНЫ (Принцип VI tests-first; DX1 — red→green замки ДО правки; мутант-доказательство).

**Organization**: по user-story. US1 = DX1 (восстановление). US2 = DX2 (каталог/тон/подсказки/витрина).

**Команды**: сборка/тесты из `src/` (`cd src && go build -o ../ladix ./cmd/ladix`; `cd src && go test ./... -count=1`).

## Format: `[ID] [P?] [Story] Description`
- **[P]**: разные файлы, нет зависимостей от незавершённых задач.
- Каждая задача — путь файла + проверяемый критерий.

---

## Phase 1: Setup

- [ ] T001 Подтвердить стартовую базу на ветке `012-mdx-diagnostics`: `cd src && gofmt -l . && go vet ./... && go test ./... -count=1 && go build ./...` — всё зелёное (фиксируем «зелёный до правок»). Зафиксировать вывод в леджере.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: общей блокирующей инфры сверх существующей нет — базовый panic-mode (`recover.go`), `ErrorList` (`internal/errors/aggregate.go`), golden-инфра уже на месте; US1 и US2 независимы. Новый накопитель НЕ вводится (FR-006).

- [ ] T002 [P] Зафиксировать в леджере карту неприкосновенного (НЕ трогать ни в одной фазе): `src/internal/eval`, `src/internal/engine`, `src/internal/store`; `eval/errors_golden_test.go:205` (`len(seen)!=28`); `cmd/ladix/source_negatives_test.go`; `engine/*_test.go`; `engine_test.go:344` (§EN-7); `examples/ошибочная.ladix` + спутники.

**Checkpoint**: можно начинать US1 и US2.

---

## Phase 3: User Story 1 — Восстановление парсера от каскада (Priority: P1) 🎯 MVP

**Goal**: ровно 1 диагностика на сломанную конструкцию с sync-lead KW в позиции выражения; контроль независимых ошибок не регрессирует.

**Independent Test**: `пусть x = если`→1, `если вернуть:⏎ печать(1)`→1, `значение⏎{`→2 (контроль).

### Tests for US1 (tests-first — ДОЛЖНЫ упасть до фикса) ⚠️

- [ ] T003 [US1] Добавить хелпер `assertDiagnostics(t *testing.T, el *errors.ErrorList, want ...string)` в `src/internal/parser/recover_test.go` (или общий тест-хелпер парсера): `t.Helper()`; `el.Len()==len(want)` с `t.Fatalf`, дампящим `el.Error()` при несовпадении количества; затем по индексам `el.Errors()[i].Error()` байт-в-байт `== want[i]` (двухстрочный канон). Критерий: компилируется, существующий `TestMultipleIndependentErrors` можно выразить через него без смены ожиданий.
- [ ] T004 [P] [US1] Red→green C-REC-1 (same-line) в `src/internal/parser/recover_test.go`: `TestCascadeSameLineSingleDiagnostic` через `assertDiagnostics` на `пусть x = если`, `пусть y = пока`, `печать(для)` — ожидание РОВНО 1 диагностика каждый. Критерий: СЕЙЧАС падает (фактически 2), помечен как red→green.
- [ ] T005 [P] [US1] Red→green C-REC-2 (multiline bleed) в `src/internal/parser/recover_test.go`: `TestCascadeBlockBleedSingleDiagnostic` на `если вернуть:⏎    печать(1)` (+ `пока`/`для` аналоги) — ожидание РОВНО 1. Критерий: СЕЙЧАС падает (фактически 4), red→green.
- [ ] T006 [P] [US1] Контроль C-REC-3 (анти-over-suppress) в `src/internal/parser/recover_test.go`: `TestIndependentErrorsNotCollapsed` — `значение⏎{`→ровно 2 (строки 1,2) + N независимых сломок→N. Критерий: зелёный СЕЙЧАС и остаётся зелёным после фикса (без правки ожиданий). Подтвердить, что `TestMultipleIndependentErrors`/`TestErrorOnOneLineDoesNotCascadeNextValidLine`/`TestErrorBudgetShared`/`TestNoGoStackTrace`/`TestSynchronize*` зелёные без правки. **Краевой кейс орфан-отступа** (`пусть x = если⏎    печать(1)`: не-блок-владеющая сломка + сиротский INDENT): `assertDiagnostics` фиксирует ФАКТИЧЕСКОЕ число после фикса (характеризация); если >1 — комментарий «долг: сиротский INDENT/конец блока, не блок-владеющий, вне scope DX1» (честно, не тихо). Не требуется = 1.

### Implementation for US1

- [ ] T007 [US1] **Фикс** в `src/internal/parser/parse_expr.go` (default-ветка `parsePrimary`, ~:206-209): заменить `p.error(t.Pos, msgUnexpected(t))` на `bad := p.advance(); p.error(bad.Pos, msgUnexpected(bad))` — потребление ошибочного токена ДО `error()`, зеркально `parse_stmt.go:29`. `synchronize`/suppress-reset НЕ трогать. Критерий: T004/T005 зеленеют; T006 остаётся 2; вся `go test ./internal/parser/` зелёная.
- [ ] T008 [US1] Синк комментариев (омоним FR-025 устранён, ложное «2–4 диагностики» снято): `src/internal/parser/parser.go:88-96` (`error()` doc), `src/internal/parser/recover.go:14-18`, `src/internal/parser/parse_expr.go:206`. Текст: каскад same-line/смежной строки подавлен (DX1, фича 012); ссылка на `specs/012-mdx-diagnostics`. Критерий: `go vet` чисто; комментарии не противоречат поведению.
- [ ] T009 [US1] Мутант-доказательство (C-REC-5): временно откатить T007 → `cd src && go test ./internal/parser/ -run Cascade -count=1` ДОЛЖЕН упасть (2/4), контроль `значение⏎{` остаётся 2; вернуть фикс. Зафиксировать результат в леджере (замок реально кусается).
- [ ] T010 [US1] decl-мутпроба M1 (C-REC-6): прогнать `последние если`, `поля: если`, не-шаг в блоке процесса через бинарник/тест. Если каскад >1 — применить тот же приём к аналогичному decl-сайту (`parse_decl.go`) + red→green замок; если нет — добавить контроль-тест «ровно 1/ровно 2» в `parse_decl_test.go` с комментарием «долга нет». Критерий: результат каждого кейса зафиксирован замком, не оставлен открытым.
- [ ] T011 [US1] Инвариант DX1: `git diff --stat master -- src/internal/lexer src/internal/eval src/internal/engine src/internal/store` пуст по коммитам US1 (DX1 — только `src/internal/parser/`). Критерий: пустой дифф.

**Checkpoint**: DX1 функционален и независимо тестируем (MVP).

---

## Phase 4: User Story 2 — Качество и полнота каталога (Priority: P2)

**Goal**: бизнес-формулировки scope A, полный инвентарь с count-locks, подсказки, расширенная витрина.

**Independent Test**: golden scope A без жаргона/кодов; инвентарь-тесты (L=11, SE=14) зелёные; ссылочные (eval=28) не тронуты.

### DX2-канон (Phase E)

- [ ] T012 [US2] Создать `docs/diagnostics-model.md` (§MDX-0..N): реестр scope A с НОВЫМИ текстами (SE-UNEXPECTED/SE-INT-RANGE/L-5/L-6/L-8, опц. L-7/L-9), список ссылочных категорий (§8.3/§SM-9/engine — байт-в-байт), формат подсказки, явный отложенный пункт «подсказки по объявленным именам — eval заморожен, deferred», и пометка «перенос в SPEC §13.4 — архитектором на шве». Критерий: каждый переформулируемый текст имеет строку «было→стало»; документ — единый источник истины формулировок.

### DX2-переформулировка + согласованный golden-проход (Phase F — один коммит)

- [ ] T013 [US2] `src/internal/parser/errors.go`: SE-UNEXPECTED (`:61`) → «неожиданный элемент '%s'»; SE-INT-RANGE (`:50`) → «целое число '%s' вне диапазона типа Целое»; обновить комментарий `:9-11` на новый канон (`docs/diagnostics-model.md`, перенос в SPEC §13.4 архитектором). Критерий: тексты дословно совпадают с model-доком.
- [ ] T014 [US2] `src/internal/lexer/messages.go`: L-8 (`:25`) → «неверная запись числа '%s'»; L-5 (`:13`)/L-6 (`:14`) → «незакрытая строка в кавычках[…]»; опц. L-7/L-9 по решению (если меняются — синхронно с model-доком); обновить комментарий-канон. Критерий: дословно совпадает с model-доком.
- [ ] T015 [US2] Согласованный golden-проход (вместе с T013/T014, один коммит): обновить ожидания в `src/internal/lexer/lexerrors_test.go`, `src/internal/parser/errors_golden_test.go`, `src/internal/parser/errors_test.go`, `src/internal/parser/recover_test.go`, `src/internal/parser/parse_expr_test.go`, `src/internal/parser/parse_stmt_test.go`, `src/internal/parser/parse_decl_test.go`, `src/cmd/ladix/docs_alignment_test.go` (A4: «неожиданный токен 'тип'»→«неожиданный элемент 'тип'», смысл A4 сохранён), `src/cmd/ladix/main_test.go`. НЕ трогать `errors/*_test.go` (образцы форматирования), eval/source/engine golden. Критерий: `cd src && go test ./... -count=1` зелёный.
- [ ] T016 [P] [US2] Замок «нет жаргона/кодов наружу в scope A» в `src/internal/parser/` (или `cmd/ladix`): тест, прогоняющий набор лексических/синтаксических ошибок и проверяющий, что stderr/`Error()` НЕ содержит «токен», «литерал», `L-`, `SE-`. Критерий: зелёный после T013-T015; кусается, если жаргон вернётся.

### DX2-инвентарь/полнота (Phase G)

- [ ] T017 [US2] Count-lock лексики в `src/internal/lexer/lexerrors_test.go`: сводный инвентарь-тест `TestLexCatalogInventory`, покрывающий L-1..L-11, с `if len(seen)!=11 { t.Errorf(...) }`. Критерий: зелёный; сверен с реестром `messages.go`.
- [ ] T018 [US2] Count-lock синтаксиса в `src/internal/parser/errors_golden_test.go` (или новый `inventory_test.go`): инвентарь SE distinct=14 (7 ядровых + SE-SOURCE-NAME/UNKNOWN-ATTR/DUP-ATTR/DUP-FIELD/TRIGGER-KIND/EXPECT-COMPOP/SCHEDULE-SPEC), `if len(seen)!=14`. Категория «Процесса» исключена (комментарий-обоснование). Критерий: зелёный; сверен с `errors.go`.
- [ ] T019 [P] [US2] Мутант-проба инвентаря: удаление кода из реестра ломает соответствующий count-lock (зафиксировать в леджере). eval `len(seen)!=28` — НЕ трогать, остаётся зелёным.

### DX2-подсказки (Phase H — парсер-side, без eval)

- [ ] T020 [P] [US2] Чистый хелпер расстояния Левенштейна в `src/internal/parser/suggest.go` (новый файл): `func closestWord(got string, vocab []string, maxDist int) (string, bool)` — детерминированный (стабильный выбор при равенстве — порядок vocab). Без состояния, без `time.Now()`. Критерий: юнит-тесты `suggest_test.go` (кандидат в пределах порога / нет кандидата / равенство → первый по порядку).
- [ ] T021 [US2] Интеграция подсказки на ограниченных словарях парсера: к SE-UNKNOWN-ATTR (`parse_decl.go` источник/метрика — словарь валидных атрибутов известен парсеру) добавлять суффикс «; возможно, вы имели в виду '<attr>'?» при близком кандидате; опц. опечатка ведущего KW. Текст — из model-дока. Подсказки по объявленным ИМЕНАМ — НЕ здесь (eval заморожен), отложено (T012). Критерий: golden с подсказкой/без; детерминизм; пустой дифф eval.

### DX2-витрина (Phase I)

- [ ] T022 [P] [US2] Новые ошибочные примеры в `examples/`: отдельные файлы, демонстрирующие DX1 (каскад→1) и DX2 (бизнес-текст + подсказка). НЕ трогать `examples/ошибочная.ladix`, `ошибка.ladix`, `ошибка_синтаксис.ladix`, `ошибка_тип.ladix`, `ошибка_процесс.ladix`. Критерий: новые файлы созданы.
- [ ] T023 [US2] Golden-замки новых примеров в `src/cmd/ladix/` (через `assertNegativeExample`/инлайн): exit 1, stderr байт-в-байт §13-канон, нет Go stack trace. Запись в `examples/MANIFEST.md`. Критерий: зелёный; забайт-пиннутые старые примеры не тронуты.

**Checkpoint**: US1 и US2 работают независимо.

---

## Phase 5: Polish & Cross-Cutting (Phase J)

- [ ] T024 [P] `cd src && gofmt -l .` (пусто) + `go vet ./...` (без замечаний) по всем пакетам.
- [ ] T025 [P] `cd src && go test ./... -count=1` зелёный; `go test ./internal/daemon/ -race -count=1` (и др. конкурентные) зелёный.
- [ ] T026 [P] `cd src && go build ./...` = 0; `go.mod`/`go.sum` без новых зависимостей (`git diff master -- src/go.mod src/go.sum` пуст).
- [ ] T027 Инвариант-аудит: `git diff --stat master -- src/internal/eval src/internal/engine src/internal/store` пуст (вся веха); `ProcessRuntime` = 7 методов, `Store` = 15 методов (grep сигнатур); нет `time.Now()` в новом коде (`git diff master | grep 'time.Now()'` пуст в добавленных строках).
- [ ] T028 Прогнать `specs/012-mdx-diagnostics/quickstart.md` целиком; зафиксировать a→1/b→1/c→2, отсутствие жаргона, пустой дифф бэкенда.
- [ ] T029 Дерево чистое; ветка `012-mdx-diagnostics` БЕЗ мержа в master (мерж `--no-ff` делает архитектор на шве по отмашке владельца). Подготовить отчёт-возврат + леджер.

---

## Dependencies & Execution Order

### Phase Dependencies
- Setup (Ph1) → Foundational (Ph2) → US1 (Ph3) и US2 (Ph4) независимы → Polish (Ph5) после обеих.
- US1 — MVP; может быть смержен/продемонстрирован до US2.

### Within US1 (строгий порядок tests-first)
- T003 (хелпер) → T004/T005/T006 (замки, падают/контроль) → T007 (фикс, зеленеет) → T008 (комментарии) → T009 (мутант) → T010 (decl-мутпроба) → T011 (инвариант).

### Within US2
- T012 (канон) → T013/T014/T015 (переформулировка+golden, ОДИН коммит) → T016 (замок жаргона).
- T017/T018/T019 (инвентарь) — после T013-T015 (тексты стабилизированы).
- T020 → T021 (подсказки).
- T022 → T023 (витрина).

### Parallel Opportunities
- T004/T005/T006 [P] — разные тест-функции, пишутся параллельно (до T007).
- T016/T019 [P] — независимые замки.
- T020/T022 [P] — Левенштейн-хелпер и файлы-примеры независимы.
- T024/T025/T026 [P] — гейты.

---

## Implementation Strategy

### MVP (US1 only)
1. Ph1 Setup → 2. Ph3 US1 (T003-T011) → 3. STOP & VALIDATE: a→1, b→1, c→2, мутант кусается, пустой дифф бэкенда. DX1 — самостоятельный смерджабельный инкремент.

### Incremental
- US1 (DX1) → независимая проверка → US2 (DX2) → независимая проверка → Polish.
- Каждый стори не ломает предыдущий.

---

## Notes
- [P] = разные файлы, нет зависимостей.
- DX1 trифт только `src/internal/parser/`; DX2 — `parser/errors.go` + `lexer/messages.go` + согласованные golden + `docs/diagnostics-model.md` + `examples/`.
- НЕ трогать (байт-в-байт/пустой дифф): eval/engine/store, §8.3=28, §SM-9, §EN-7, ошибочная.ladix + спутники.
- Канон формулировок — `docs/diagnostics-model.md`; SPEC §13.4/README — зона архитектора на шве.
- Ветка БЕЗ мержа в master; коммиты русские; мерж `--no-ff` — архитектор.
