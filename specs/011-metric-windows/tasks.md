# Tasks: Окна метрик (период: последние N <ед> + прошлый <период>) (011-A2)

**Input**: Design documents from `/specs/011-metric-windows/`

**Prerequisites**: plan.md (есть), spec.md (3 US — P1/P2/P3, FR-001..FR-019, SC-001..SC-010), research.md, data-model.md (Период 3 формы / Окно / Метрика-скаляр + §MW-D-VALUE), якорь `docs/metric-windows-model.md` §MW-0..§MW-13 + предрешённые развилки A2-1…A2-7.

**Природа фичи**: чистый фронтенд + вычислитель окна. 0 новых пакетов, 0 новых зависимостей (`time.AddDate` — stdlib), 0 NEEDS CLARIFICATION. **Лексер НЕ трогается** (`последние`/`прошлый`/`прошлая`/nouns остаются `IDENT`, `30дн` — существующий `DURATION`). Аддитивно расширяются: `value.Период` (+`Amount`/`Unit`/`Offset`, ровно 3 файла), AST (`WindowPeriodLit`+`LastCompletedPeriodLit`), парсер (`parsePeriodValue` спец-ветка `период:`), семантика (3 проверки §MW-SEM-1/2/3), eval (+2 case + `nounToAdverb`), вычислитель окна (`window.go` +скользящее +сдвиг `Offset`). **`store/codec.go` НЕ трогается** (§MW-3: скользящие/завершённые `Период` недостижимы как first-class значения, `Name`-only round-trip корректен). **`Store`/`ProcessRuntime`/экспорт `eval`/условие и тело триггера — НЕ трогаются (A2-7)**. A2-4 (`по_дате`) покрыта существующей проверкой `analyze.go:168` — НОВОЙ проверки нет. Все программы v1 валидны (5 календарных периодов — те же границы). Ряд значений ОТЛОЖЕН (A2-1).

## Формат: `[ID] [P?] [Story?] Описание`

- **[P]**: можно параллелить (разные файлы, нет зависимостей).
- **[Story]**: к какой US относится (US1/US2/US3). Phase A–B (Foundational) и Phase G (Polish) — без [Story]; Phase C–F несут [Story] там, где задача однозначно служит одной истории.
- Каждая задача содержит ТОЧНЫЙ путь файла, действие в одну строку и удовлетворяемые FR/SC/§MW.
- **TESTS-FIRST (конституция VI)**: для парсера/семантики/вычислителя окна тест-задача пишется и КРАСНЕЕТ до прод-задачи того же правила. Тест-замки помечены `🔒 ТЕСТ-ЗАМОК`; инверсия golden/негатива — `🔁 ИНВЕРСИЯ`.

**Структура — по ФАЗАМ РЕАЛИЗАЦИИ (каждая — барьер-гейт A→B→C→D→E→F→G).** Внутри фазы — `[P]`. Барьер: фаза N+1 не стартует, пока фаза N не зелёная (`cd src && go test ./...`).

**Пути (факт репозитория, верифицировано)**: корень `…/ladix/`; модуль в `src/` (`src/go.mod`, `go 1.25.0`); **лексер НЕ трогается** (`последние`/`прошлый`/nouns — `IDENT` через `scan_ident.go`; `30дн` — `DURATION` с `DurationValue{Amount,Unit}` из `scan_number.go`, единицы в `keywords.go durationUnits`); `value.Период{Name string}` — `src/internal/value/deferred.go:28`; равенство `src/internal/value/equal.go:71-72` (`case Период: ok && x.Name == y.Name`); репрезентация `src/internal/value/repr.go:49` (`case Период: return x.Name`); кодек `src/internal/value/store/codec.go` (НЕ трогать); AST-литералы `src/internal/ast/literal.go` (паттерн `DurationLit` со строки 61, `exprBase`/`base{pos}`, конструктор `NewDurationLit`); парсер `src/internal/parser/parse_decl.go` (`parseMetricDecl`:185, ветка `case "период":` строка 225 зовёт `parseExpression()`; `metricAttrName`:175 `{источник,где,агрегат,период,по_дате}`); eval `src/internal/eval/expr.go` (`evalExpr`:17 switch по `*ast.*`); окно `src/internal/eval/window.go` (`periodWindow`:17 switch `p.Name`, INCLUSIVE `[начало,конец]`, помощники `dateToTime`/`timeToDate`/`lastDayOfMonth`); метрика `src/internal/eval/metric_engine.go` (`evalMetric`:26, тип-чек `pv.(value.Период)`, `periodWindow(per, i.now())`:59, `emptyWindowResult`:93; `i.now()` заморожен `interpreter.go:155-161`); фильтр `recordSurvives` (INCLUSIVE — НЕ трогать); семантика `src/internal/eval/analyze.go` (`Analyze`:19, связка период↔по_дате:165-171 `hasPeriod := m.Attrs.PeriodPos.Line != 0`, `semErr`); golden-инфраструктура eval `src/internal/eval/{window_test.go,metric_engine_test.go,analyze_decl_test.go}` (`periodWindow(value.Период{Name:c.period}, c.d)`:42, `buildMetricInterp` с `FixedClock{2026,5,31}`, `goldenMetric(where,aggregate,period,byDate)`:48, `dt(y,m,d)`); CLI-harness `src/cmd/ladix/{golden_test.go,metric_test.go}` (`realMain`/`examplePath`/`assertNegativeExample`:115/`withRepoRoot`:31/`runMetric(...,clock,...)`/`fixedClock2026`:27 `{2026,5,31}`/`metricFixture`); парс-сет `src/internal/parser/examples_test.go` (`TestExamplesParseCleanSet`); витрина `examples/*.ladix` + `examples/MANIFEST.md`; данные в корне `data/` (`orders.csv`/`orders.json`/`orders.ndjson`/`sales.json` от A1).

---

## Phase A — value.Период: аддитивное расширение `Amount`/`Unit`/`Offset` (Foundational, §MW-3/§MW-D-VALUE)

**Назначение**: расширить `value.Период` АДДИТИВНО (+`Amount int64`/`Unit string`/`Offset int`), синхронизировать равенство (все поля) и репрезентацию (скользящая/завершённая формы), добавить `nounFromAdverb`. **§MW-D-VALUE: пакет `value` остаётся листовым** (без новых импортов; `Период` буквально моделирует период). 5 календарных констант сохраняют нулевые новые поля → регистрация `interpreter.go:77-79` НЕ меняется, структурное равенство существующих периодов цело. **`store/codec.go` НЕ трогать** (§MW-3 обосновано: скользящие/завершённые `Период` недостижимы как first-class значения — кодек `Name`-only round-trip корректен).

**⚠️ КРИТИЧНО (барьер)**: eval (фаза D) строит `value.Период{Amount,Unit,Offset}` — поля обязаны существовать ДО неё; `repr.go` зависит от `nounFromAdverb`.

- [ ] T001 [P] 🔒 ТЕСТ-ЗАМОК (Phase A) Табличные тесты равенства/репрезентации в `src/internal/value/value_test.go` (или соседний `equal_test.go`/`repr_test.go`): (а) `Период{Name:"последние",Amount:30,Unit:"дн"}` ≠ `Период{Name:"последние",Amount:7,Unit:"дн"}` (равенство учитывает ВСЕ поля); (б) два `Период{Name:"ежемесячно"}` с нулевыми новыми полями — равны (обр. совместимость); (в) `String(Период{Name:"последние",Amount:30,Unit:"дн"})=="последние 30дн"`; (г) `String(Период{Name:"ежемесячно",Offset:-1})=="прошлый месяц"` (через `nounFromAdverb`); (д) `String(Период{Name:"ежемесячно"})=="ежемесячно"` (v1). КРАСНЕЕТ до T002/T003/T004. Покрывает FR-017/§MW-3/§MW-D-VALUE-EQ (конституция VI/VII).
- [ ] T002 [P] (Phase A) В `src/internal/value/deferred.go` (структура `Период`:28): добавить поля `Amount int64`, `Unit string`, `Offset int` (дословно §MW-3); без новых импортов (листовость). Удовлетворяет FR-017/§MW-3.
- [ ] T003 (Phase A) В `src/internal/value/equal.go` (`case Период`:71-72): сравнивать ВСЕ поля — `ok && x.Name==y.Name && x.Amount==y.Amount && x.Unit==y.Unit && x.Offset==y.Offset`. Зависит от T002. Удовлетворяет FR-017/§MW-D-VALUE-EQ.
- [ ] T004 (Phase A) В `src/internal/value/repr.go` (`case Период`:49): `Name=="последние"`→`fmt.Sprintf("последние %d%s", Amount, Unit)`; `Offset!=0`→`"прошлый "+nounFromAdverb(Name)`; иначе `Name` (v1). Добавить чистый хелпер `nounFromAdverb(adverb string) string` (ежедневно→день…ежегодно→год) в `repr.go` (или `deferred.go`). Зависит от T002. Удовлетворяет FR-017/§MW-3/§MW-D-VALUE-EQ.

**Checkpoint A**: `Период` расширен аддитивно, `value` листовой (без `errors`/новых импортов); равенство/репрезентация по всем полям; 5 констант нулевые; `window_test.go:42` (`Период{Name:...}` keyed) не сломан; T001 зелёный; `store/codec.go` не тронут. `cd src && go test ./internal/value/...` ok.

---

## Phase B — AST: `WindowPeriodLit` + `LastCompletedPeriodLit` (§MW-D-PARSE-3)

**Назначение**: добавить два листовых узла-выражения (паттерн `DurationLit`/`exprBase`/`base{pos}`). **§MW-D-PARSE-3**: оба несут локальную `ast.Position`, НЕ импортируют `errors` (`ast` остаётся листовым, конституция IV/VII). `WindowPeriodLit{Amount string, Unit string}` (Amount — строка из `DurationValue`, в int парсится в семантике/eval); `LastCompletedPeriodLit{Noun string}` (Noun ∈ {день…год}). Оба — `Expression`, `Pos()` = свой токен.

- [ ] T005 [P] 🔒 ТЕСТ-ЗАМОК (Phase B) Тест конструкции AST в `src/internal/ast/literal_test.go` (или соседний): `NewWindowPeriodLit(pos,"30","дн")` → поля `Amount`/`Unit` доступны, `Pos()` возвращает заданную позицию; `NewLastCompletedPeriodLit(pos,"месяц")` → `Noun` доступен, `Pos()` корректен; оба удовлетворяют интерфейсу `Expression`. КРАСНЕЕТ до T006. Покрывает §MW-D-PARSE-3 (конституция VI).
- [ ] T006 (Phase B) В `src/internal/ast/literal.go` (рядом с `DurationLit`:61): добавить `WindowPeriodLit{exprBase; Amount string; Unit string}` + `NewWindowPeriodLit(pos, amount, unit string)` и `LastCompletedPeriodLit{exprBase; Noun string}` + `NewLastCompletedPeriodLit(pos, noun string)` (паттерн `NewDurationLit`: `exprBase{base{pos}}`). Локальная `Position`, без `errors`. Зависит от T005. Удовлетворяет FR-001/FR-006/§MW-D-PARSE-3.

**Checkpoint B**: 2 новых узла-выражения, `ast` листовой (без `errors`); T005 зелёный.

---

## Phase C — Парсер: `parsePeriodValue` спец-парс ветки `период:` (§MW-4)

**Назначение**: в `parseMetricDecl` (`parse_decl.go:185`) ветку `case "период":` (строка 225) заменить `period = p.parseExpression()` на `period = p.parsePeriodValue()`. **§MW-D-PARSE-1**: матч `peek().Lexeme == "последние"`/`"прошлый"`/`"прошлая"` контекстно (как A1 матчит `тип`/`поля`); слова остаются IDENT (НЕ keyword — v1-переменная безопасна). **§MW-D-PARSE-2 (контигуальная форма)**: `последние`→`advance`→`expect(DURATION, …)`→`WindowPeriodLit{Amount:durTok.Value.Amount, Unit:durTok.Value.Unit, Pos}`; спейсовая `30 дн` (`INT(30)` `IDENT(дн)`) → `expect(DURATION)` найдёт INT → парс-ошибка «ожидался период вида N<ед>, например 30дн». `прошлый/прошлая`→`advance`→`expect(IDENT, …)`→`LastCompletedPeriodLit{Noun:nounTok.Lexeme, Pos}`. **§MW-D-PARSE-4**: иначе → `parseExpression()` (адверб-константа v1 БЕЗ изменений).

**⚠️ Барьер**: фаза D (семантика) обходит `m.Period` и различает `WindowPeriodLit`/`LastCompletedPeriodLit`/`Ident`, заполняемые здесь.

- [ ] T007 [P] 🔒 ТЕСТ-ЗАМОК (Phase C) Табличные ПОЗИТИВНЫЕ тесты парсера в `src/internal/parser/parse_decl_test.go`: (а) `период: последние 30дн` → `m.Period` = `*ast.WindowPeriodLit` с `Amount=="30"`, `Unit=="дн"`, `Pos.Line!=0`; (б) `период: прошлый месяц` → `*ast.LastCompletedPeriodLit{Noun:"месяц"}`; (в) `прошлая неделя` → `Noun=="неделя"`; (г) `период: ежемесячно` → `*ast.Ident` (путь v1 неизменен). КРАСНЕЕТ до T010/T011. Покрывает FR-001/FR-006/FR-009/§MW-D-PARSE-2/4 (конституция VI).
- [ ] T008 [P] 🔒 ТЕСТ-ЗАМОК (Phase C) Табличные НЕГАТИВНЫЕ тесты парсера в `src/internal/parser/parse_decl_test.go`: (а) `последние` без `DURATION` (конец строки / `последние нед`) → парс-ошибка «ожидался период вида N<ед>, например 30дн»; (б) спейсовая `последние 30 дн` (`INT` после `последние`) → та же парс-ошибка (§MW-D-PARSE-2); (в) `прошлый` без noun (конец строки) → парс-ошибка «ожидался период: день/неделя/месяц/квартал/год». КРАСНЕЕТ до T010/T011. Тексты §MW-8 byte-exact. Покрывает FR-014/edge §MW-12 (конституция VI/VIII).
- [ ] T009 [P] 🔒 ТЕСТ-ЗАМОК (Phase C) Регресс-тест в `src/internal/parser/parse_decl_test.go`: `последние`/`прошлый`/`прошлая` КАК ПЕРЕМЕННАЯ вне ветки `период:` (напр. `присвоить последние = 5` / выражение `x = последние + 1`) — остаются обычными `IDENT`, спец-парс НЕ срабатывает (контекстный матч), парсятся как `Ident`. КРАСНЕЕТ, если слова станут keyword. **Доп. (защита codec-skip §MW-3):** `последние`/`прошлый` НЕ предрегистрированы как глобалы → при чтении как переменной дают рантайм «не объявлено …» (скользящий/завершённый `Период` НЕ может стать first-class значением → в `Store`/codec не попадает). Покрывает §MW-D-PARSE-1/edge §Edge Cases/§MW-3 (конституция VI).
- [ ] T010 (Phase C) В `src/internal/parser/parse_decl.go`: ввести метод `parsePeriodValue() ast.Expression` — `peek().Lexeme=="последние"` → `advance` + `expect(DURATION, "период вида N<ед>, например 30дн")` → `ast.NewWindowPeriodLit(pos, durTok.Value.Amount, durTok.Value.Unit)` (§MW-D-PARSE-2). Зависит от T007/T008/T006. Удовлетворяет FR-001/FR-009/§MW-4/§MW-D-PARSE-1/2.
- [ ] T011 (Phase C) В `src/internal/parser/parse_decl.go`: в `parsePeriodValue()` ветви `peek().Lexeme=="прошлый"||"прошлая"` → `advance` + `expect(IDENT, "период: день/неделя/месяц/квартал/год")` → `ast.NewLastCompletedPeriodLit(pos, nounTok.Lexeme)`; `else` → `parseExpression()` (v1); в `case "период":` (`parseMetricDecl`:225) заменить `parseExpression()` на `parsePeriodValue()`. Зависит от T010. Удовлетворяет FR-006/FR-009/§MW-4/§MW-D-PARSE-2/4.

**Checkpoint C**: 3 формы `период:` парсятся; негативы дают канон §MW-8; `последние`/`прошлый`-как-переменная остаются IDENT; адверб-путь v1 неизменен; T007/T008/T009 зелёные. `cd src && go test ./internal/parser/...` ok.

---

## Phase D — eval + вычислитель окна: 2 case + скользящее окно + сдвиг Offset (§MW-5/§MW-6)

**Назначение**: (1) `evalExpr` (`expr.go:17`) +2 case: `WindowPeriodLit`→`value.Период{Name:"последние", Amount:atoi(lit.Amount), Unit:lit.Unit, Offset:0}`; `LastCompletedPeriodLit`→`value.Период{Name:nounToAdverb(lit.Noun), Amount:0, Unit:"", Offset:-1}`. (2) `nounToAdverb` (день→ежедневно…год→ежегодно) в `metric_engine.go`. (3) `periodWindow` (`window.go:17`) +ветка `Name=="последние"` (§MW-D-WINDOW-SLIDE: `конец=d`; `нижняя_искл=d−N ед` через `AddDate`; `начало=нижняя_искл.AddDate(0,0,+1)` — инклюзивный эквивалент полуинтервала `(d−N, d]`; `дн`→`AddDate(0,0,-N)`, `нед`→`AddDate(0,0,-7*N)`, `мес`→`AddDate(0,-N,0)`) + ветка `Offset!=0` (§MW-D-WINDOW-COMPLETED: сдвиг якоря `d` назад на `|Offset|` периодов базовой единицы, затем существующая календарная логика 5 ветвей). **`recordSurvives` (INCLUSIVE) НЕ трогать** — окно делается инклюзивно-эквивалентным. **Детерминизм**: всё от `d=i.now()` (заморожен), НИКОГДА `time.Now()` (конституция V).

**⚠️ Барьер**: фаза F (golden/витрина) гоняет вычислитель окна.

- [ ] T012 [P] 🔒 ТЕСТ-ЗАМОК (Phase D) Golden-таблица границ окна в `src/internal/eval/window_test.go` на `dt(2026,6,15)` (§MW-10): `последние 30дн`→`[2026-05-17, 2026-06-15]` (нижняя_искл=`2026-05-16` ИСКЛ, `2026-05-17` вкл, `2026-06-15` вкл); `последние 2нед`→`[2026-06-02, 2026-06-15]`; `последние 1мес`→`[2026-05-16, 2026-06-15]`; `прошлый месяц` (`Offset:-1, Name:ежемесячно`)→`[2026-05-01, 2026-05-31]`; **эквивалентность** `последние 7дн`≡`последние 1нед` (одинаковые границы). КРАСНЕЕТ до T015. 🔁 полуинтервал/N/Offset. Покрывает SC-002/SC-003/§MW-10 #1/#2/#3 (конституция VI).
- [ ] T013 [P] 🔒 ТЕСТ-ЗАМОК (Phase D) Golden конца месяца в `src/internal/eval/window_test.go` на `dt(2026,5,31)` (§MW-D-WINDOW-EDGE): `последние 1мес` и `прошлый месяц` → ЗАФИКСИРОВАННЫЙ НОРМАЛИЗОВАННЫЙ результат Go `AddDate` (честный golden, не «красивый»; значение определить ЭМПИРИЧЕСКИ в impl-прогоне и запиннить). КРАСНЕЕТ до T015. 🔁 если арифметику месяцев изменить — красный. Покрывает SC-008/§MW-9 #4/§MW-D-WINDOW-EDGE (конституция VI).
- [ ] T014 [P] 🔒 ТЕСТ-ЗАМОК (Phase D) Golden завершённых периодов в `src/internal/eval/window_test.go` на `dt(2026,6,15)`: `прошлая неделя`/`прошлый квартал`/`прошлый год`/`прошлый день` — каждый даёт полный предыдущий КАЛЕНДАРНЫЙ период (базовый адверб + `Offset:-1`). КРАСНЕЕТ до T015. 🔁 Offset 0↔−1. Покрывает SC-007/§MW-9 #3 (конституция VI).
- [ ] T015 (Phase D) В `src/internal/eval/window.go` (`periodWindow`:17): добавить `case "последние"` (§MW-D-WINDOW-SLIDE: `конец=d`, `нижняя_искл=d−N ед` по `Unit`, `начало=нижняя_искл.AddDate(0,0,+1)`, через `dateToTime`/`timeToDate`) и обработку `p.Offset != 0` для календарных `Name` (§MW-D-WINDOW-COMPLETED: сдвиг `d` на `Offset` периодов базовой единицы — `ежедневно`→`AddDate(0,0,Offset)`/`еженедельно`→`AddDate(0,0,7*Offset)`/`ежемесячно`→`AddDate(0,Offset,0)`/`ежеквартально`→`AddDate(0,3*Offset,0)`/`ежегодно`→`AddDate(Offset,0,0)`, затем существующие 5 ветвей сдвинутой даты). `recordSurvives` НЕ трогать. Зависит от T012/T013/T014, Phase A. Удовлетворяет FR-002/FR-007/§MW-6/§MW-D-WINDOW.
- [ ] T016 (Phase D) В `src/internal/eval/expr.go` (`evalExpr`:17): +2 case — `*ast.WindowPeriodLit`→`value.Период{Name:"последние", Amount:<atoi lit.Amount>, Unit:lit.Unit}`; `*ast.LastCompletedPeriodLit`→`value.Период{Name:nounToAdverb(lit.Noun), Offset:-1}`. Добавить чистую функцию `nounToAdverb(noun string) string` (день→ежедневно…год→ежегодно) в `src/internal/eval/metric_engine.go`. Зависит от Phase A/B, T015. Удовлетворяет FR-001/FR-006/FR-007/§MW-5.

**Checkpoint D**: скользящее окно (полуинтервал) + завершённый период (сдвиг) считаются от `i.now()`; `7дн≡1нед`; edge конца месяца честный; `recordSurvives`/`i.now()` не тронуты; T012/T013/T014 зелёные. `cd src && go test ./internal/eval/...` ok.

---

## Phase E — Семантика: 3 статические проверки оконных форм (§MW-SEM-1/2/3, analyze.go)

**Назначение**: в `Analyze` (`analyze.go:19`, после связки период↔по_дате) — статический обход `m.Period` AST, различая `WindowPeriodLit`/`LastCompletedPeriodLit`. Позиция — `m.Attrs.PeriodPos` (presence `Line!=0`). **§MW-SEM-4 (A2-4, `по_дате`)**: покрыта существующей проверкой `analyze.go:168` («период требует по_дате») — НОВОЙ проверки НЕ добавлять. Тексты — дословный канон §MW-8.A.

- [ ] T017 [P] 🔒 ТЕСТ-ЗАМОК (Phase E) Табличные тесты семантики в `src/internal/eval/analyze_decl_test.go`: (а) `последние 5час` → `метрика '<имя>': единица 'час' недопустима для окна (допустимо: дн, нед, мес)`, позиция `PeriodPos`; (б) `последние 0дн` → `метрика '<имя>': размер окна должен быть положительным`; (в) `прошлый век` → `метрика '<имя>': неизвестный период 'век' (допустимо: день, неделя, месяц, квартал, год)`; (г) `последние 30дн` без `по_дате` → СУЩЕСТВУЮЩЕЕ `метрика '<имя>': 'период' требует 'по_дате'` (НЕ новое сообщение — A2-4). КРАСНЕЕТ до T018. Тексты byte-exact §MW-8.A. Покрывает FR-003/FR-004/FR-005/FR-008/FR-018/SC-006 (конституция VI/VIII).
- [ ] T018 (Phase E) В `src/internal/eval/analyze.go`: после связки период↔по_дате — обход `m.Period`: при `*ast.WindowPeriodLit` — (§MW-SEM-1) `Unit ∈ {дн,нед,мес}` иначе `semErr(PeriodPos, "…единица '<ед>' недопустима для окна (допустимо: дн, нед, мес)")`; (§MW-SEM-2) `atoi(Amount) ≥ 1` иначе `semErr(PeriodPos, "…размер окна должен быть положительным")`; при `*ast.LastCompletedPeriodLit` — (§MW-SEM-3) `Noun ∈ {день,неделя,месяц,квартал,год}` иначе `semErr(PeriodPos, "…неизвестный период '<noun>' (допустимо: день, неделя, месяц, квартал, год)")`. A2-4 — НЕ дублировать (`analyze.go:168` покрывает). Тексты §MW-8.A byte-identical. Зависит от T017, Phase B/C. Удовлетворяет FR-003/FR-004/FR-008/FR-018/§MW-SEM-1/2/3/§MW-8.A.

**Checkpoint E**: 3 семантические ошибки + позиция `PeriodPos`; A2-4 даёт существующее сообщение (не дублировано); T017 зелёный. `cd src && go test ./internal/eval/...` ok.

---

## Phase F — Golden CLI, триггер A2-7, DoD-срез, обр. совместимость, витрина (§MW-9)

**Назначение**: golden-приёмка под CLI на инъектированном `FixedClock` (через `runMetric(...,clock,...)`/`withRepoRoot`, паттерн `metric_test.go`); триггер по оконной метрике (A2-7, без изменения кода триггера); DoD-срез `выручка_30д` над CSV-источником (A1); регресс обр. совместимости 5 календарных периодов; витрина `examples/*.ladix` + MANIFEST; негатив-замки §13 (`assertNegativeExample`). **Детерминизм — `FixedClock` (инъекция), НЕ пиннинг `сегодня()`** (конституция V). Дата-зависимый DoD пиннится на `FixedClock` (для `последние 30дн` нужен `{2026,6,15}` — НЕ конец месяца, §MW-D-WINDOW-EDGE; существующий `fixedClock2026:27` — `{2026,5,31}`, для DoD-среза завести отдельный фикс-Clock). Негатив-замок — exit 1, stderr byte-exact §13, без `.go:`/goroutine.

**⚠️ Порядок внутри пары пример↔замок**: пример (а) создаётся ДО/ВМЕСТЕ со своим замком (б). Позитивы добавляются в `TestExamplesParseCleanSet`; негативы НЕ добавляются (они должны падать).

- [ ] T019 [P] 🔒 ТЕСТ-ЗАМОК (Phase F, US1) Golden-скаляр оконной метрики в `src/internal/eval/metric_engine_test.go` (или `src/cmd/ladix/`): на `FixedClock{2026,6,15}` метрика `последние 30дн`+`по_дате` над **ВЫДЕЛЕННОЙ фикстурой границ окна** (записи ровно на `2026-05-16` искл / `2026-05-17` вкл / `2026-06-15` вкл) → агрегат ровно по записям окна, скаляр запиннен; запись на нижней границе НЕ учтена (полуинтервал виден в выводе). **Фикстура — отдельная (`testdata/window_bounds.json` или inline), НЕ `data/orders.csv`** (его НЕ модифицировать — общий с A1). 🔁 если границу/окно сдвинуть — красный. Зависит от Phase D/E. Покрывает SC-003/§MW-9 #1.
- [ ] T020 [P] 🔒 ТЕСТ-ЗАМОК (Phase F, US3) Регресс обр. совместимости в `src/cmd/ladix/` + `src/internal/eval/`: на `fixedClock2026` `период: ежемесячно`+`по_дате` (`метрики.ladix`/`выручка.ladix`/`metricFixture`) → скаляр и границы `[2026-05-01,2026-05-31]` БЕЗ изменения вывода (5 календарных периодов — нулевые новые поля, тот же путь). 🔁 регресс v1. Зависит от Phase A/D. Покрывает FR-013/SC-005/§MW-9 #8.
- [ ] T021 [P] 🔒 ТЕСТ-ЗАМОК (Phase F, US1) Триггер по оконной метрике A2-7 в `src/cmd/ladix/` (или `src/internal/eval/`): `когда метрика <оконная последние 30дн> < N` на `FixedClock{2026,6,15}` → срабатывает на скаляр окна, exit 0, БЕЗ изменения кода условия/тела триггера (`trigger_run.go:64` не тронут). 🔁 замок на регресс границы eval↔движок. Зависит от Phase D/E. Покрывает FR-011/SC-009/§MW-9 #7.
- [ ] T022 [P] (Phase F, US1) Создать `examples/окно_скользящее.ladix`: CSV-источник (A1, `файл: "data/orders.csv"`, `тип: csv`+схема) + метрика `период: последние 30дн` + `по_дате` + `агрегат: сумма(сумма_заказа)` (зеркало `examples/источник_csv.ladix`). Parse-clean. Покрывает SC-004/US1.
- [ ] T023 [P] (Phase F, US2) Создать `examples/окно_завершённое.ladix`: источник + метрика `период: прошлый месяц` + `по_дате`. Parse-clean. Покрывает SC-002/US2.
- [ ] T024 [P] (Phase F, US1) Создать `examples/выручка_30д.ladix` (DoD-срез M1): CSV-источник (A1, `data/orders.csv`) + метрика `выручка_30д` `период: последние 30дн` + `по_дате` + `агрегат: сумма(сумма_заказа)`. **Путь `data/orders.csv` (резолв `withRepoRoot` от корня), НЕ `examples/data/`.** Parse-clean. Покрывает SC-004/FR-019/US1.
- [ ] T025 [P] (Phase F, US3) Создать негатив-примеры витрины: `examples/окно_единица.ladix` (`последние 5час`), `examples/окно_размер.ladix` (`последние 0дн`), `examples/окно_noun.ladix` (`прошлый век`) — каждый exit 1, §13-канон. При создании зафиксировать ФАКТИЧЕСКИЙ байт-точный stderr и передать в golden T028. Покрывает §MW-9 #5.
- [ ] T026 🔒 ТЕСТ-ЗАМОК (Phase F, US1) Golden DoD-среза `выручка_30д` в `src/cmd/ladix/`: прогон `examples/выручка_30д.ladix` через `runMetric`/`withRepoRoot` для резолва `data/orders.csv` на **`FixedClock{2026,6,15}`** (отдельный фикс-Clock, НЕ `fixedClock2026:{2026,5,31}` — `последние 30дн` нужен не-конец-месяца) → байт-точный ожидаемый скаляр, exit 0 (SC-004 DoD M1). **`data/orders.csv` НЕ модифицировать** (A1-golden `TestCLIGoldenSourceCSV` пинит сумму-без-окна `2300000.5`). Окно `(2026-05-16, 2026-06-15]` исключает `2026-05-04`(1.2M)/`2026-05-12`(0.8M), оставляет `2026-05-27`(оплачен, в окне) → ожидаемый DoD-скаляр **`300000.0`** (демонстрирует фильтрацию окном vs полная сумма 2.3M; `2026-05-19` — отменён, отфильтрован `где`). 🔁 если CSV→окно→метрика разошлась — красный. Зависит от T024. Покрывает SC-004/FR-019/§MW-9 #9.
- [ ] T027 (Phase F, US1/US2) Добавить позитивные демо (T022/T023/T024) в `TestExamplesParseCleanSet` (`src/internal/parser/examples_test.go`); негативы T025 (`окно_единица`/`окно_размер`/`окно_noun`) — НЕ добавлять (должны падать). Зависит от T022–T025.
- [ ] T028 🔒 ТЕСТ-ЗАМОК (Phase F, US3) Negative-замки витрины в `src/cmd/ladix/` через `assertNegativeExample`: `окно_единица.ladix` → «…единица 'час' недопустима для окна…»; `окно_размер.ladix` → «…размер окна должен быть положительным»; `окно_noun.ladix` → «…неизвестный период 'век'…» — каждый exit 1 + stderr byte-exact (пин из T025) + нет `.go:`/goroutine. 🔁 если перестал падать ИМЕННО этой ошибкой — красный. Зависит от T025, Phase E. Покрывает SC-006/§MW-9 #5/§MW-8.
- [ ] T029 (Phase F) Обновить `examples/MANIFEST.md`: записи новых A2-демо (`окно_скользящее`/`окно_завершённое`/`выручка_30д`/`окно_единица`/`окно_размер`/`окно_noun`) — файл / назначение / golden-stdout либо пометка дата-зависимости + ссылка на run-golden (DoD на `FixedClock{2026,6,15}`). Зависит от T022–T028.

**Checkpoint F**: скользящее/завершённое окно под CLI-замками; триггер A2-7 без изменения кода; DoD `выручка_30д` зелёный на `{2026,6,15}`; v1-регресс (5 периодов) зелёный; витрина 4 позитива + 3 негатива + MANIFEST. `cd src && go test ./...` ok.

---

## Phase G — Self-check: gofmt/vet/build/test + мутпробы + чистое дерево

**Назначение**: финальный гейт фичи; каждый новый замок кусается при инверсии прод-логики.

- [ ] T030 (Phase G) Финальный гейт из `src/`: `cd src && gofmt -l . && go vet ./... && go build ./... && go test ./... -count=1` — gofmt пуст, vet/build PASS, все пакеты `ok`, exit 0. Покрывает SC-010/конституция I.
- [ ] T031 (Phase G) Мутпробы (Ф8, §MW-9 #10): подтвердить, что каждый новый замок ЛОМАЕТСЯ при инверсии прод-логики — граница (полуинтервал `(d−N,d]`→замкнутый `[d−N,d]` / `начало` без `+1day` → T012/T019 краснеют), `N`→`N±1` (T012), `Offset` `0↔−1` (T014), `Name=="последние"`-ветка снята (T012), unit-guard §MW-SEM-1 снят / множество расширено (T017/T028 — `5час` проходит), `N≥1`-guard снят (T017 — `0дн` проходит). НЕ коммитить мутации — только проверить «кусается». Покрывает SC-006/§MW-9 #10.
- [ ] T032 (Phase G) Чистота дерева + изоляция: `git status --short` пуст (собранный `ladix`/`src/ladix` игнорируется); 0 новых зависимостей (`git diff src/go.mod src/go.sum` пуст); **`git diff` подтверждает: `store/codec.go`, `recordSurvives`, `trigger_run.go`, `Store`/`ProcessRuntime`/экспорт `eval`, лексер — НЕ тронуты** (A2-7, §MW-3, §MW-7, §MW-11). Покрывает FR-015/FR-016/SC-010/§MW-11.

**Checkpoint G**: гейт зелёный, мутпробы кусают, дерево чистое, 0 deps, `store/codec.go`/триггер/лексер не тронуты, изоляция фронтенда цела. Конвейер 011 готов к мержу.

---

## Dependencies / Phase gates

### Барьеры фаз (A→B→C→D→E→F→G)

- **Phase A (value.Период)** — без зависимостей; БЛОКИРУЕТ eval (строит `Период{Amount,Unit,Offset}`). Внутри: T001 [P] (тест) → T002 [P] → T003 ∥ T004 (после T002).
- **Phase B (AST)** — после A не требуется (независим), но tests-first внутри; БЛОКИРУЕТ парсер/семантику/eval (узлы `WindowPeriodLit`/`LastCompletedPeriodLit`). Внутри: T005 [P] → T006.
- **Phase C (Парсер)** — после B; БЛОКИРУЕТ семантику/eval (заполняет `m.Period` узлами). Внутри: T007/T008/T009 [P] (тесты) → T010 → T011.
- **Phase D (eval+окно)** — после A (поля `Период`) и B/C (узлы). Внутри: T012/T013/T014 [P] (golden) → T015 → T016.
- **Phase E (Семантика)** — после B/C (узлы в `m.Period`); статика до eval. Внутри: T017 [P] → T018.
- **Phase F (Замки+витрина)** — после D/E. Пары пример↔замок: T024→T026, T025→T028; T019/T020/T021 после D/E; T022/T023/T024/T025 [P] (разные `examples/*.ladix`); T027 после T022–T025; T029 после T022–T028.
- **Phase G (Self-check)** — после всего; T030→T031→T032.

### Внутри фаз ([P] = разные файлы, нет зависимостей)

- A: T001 (тест, `value_test.go`) ∥ T002 (`deferred.go`); T003 (`equal.go`) ∥ T004 (`repr.go`) после T002.
- B: T005 (тест) → T006 (tests-first).
- C: T007 ∥ T008 ∥ T009 (тесты, один файл `parse_decl_test.go` — при worktree-изоляции один владелец, секвенировать запись); T010 после тестов; T011 после T010.
- D: T012 ∥ T013 ∥ T014 (golden, один файл `window_test.go` — один владелец, секвенировать); T015 после golden; T016 после T015.
- E: T017 (тест) → T018.
- F: T019 ∥ T020 ∥ T021 (golden, разные тест-блоки); T022 ∥ T023 ∥ T024 ∥ T025 (разные `examples/*.ladix`); затем T026/T027/T028/T029.
- G: T030 → T031 → T032 (последовательно).

### Один-владелец-файла (worktree-изоляция implement)

- `src/internal/parser/parse_decl_test.go` — T007/T008/T009: один агент, последовательно.
- `src/internal/eval/window_test.go` — T012/T013/T014: один агент, последовательно.
- `src/internal/value/deferred.go` — T002 (T004 `nounFromAdverb` может уйти сюда или в `repr.go`).
- `src/internal/parser/parse_decl.go` — T010→T011 (тот же файл, секвенированы).
- `src/internal/eval/window.go` — только T015; `src/internal/eval/expr.go` — только T016; `src/internal/eval/metric_engine.go` — `nounToAdverb` (T016).
- `src/internal/eval/analyze.go` — только T018.
- `examples/MANIFEST.md` — только T029.

---

## Notes

- [P] = разные файлы / содержательно независимо.
- TESTS-FIRST (конституция VI): value/парсер/семантика/окно — тест-задача КРАСНЕЕТ до прод-задачи того же правила (T001<T002/T003/T004, T005<T006, T007/T008/T009<T010/T011, T012/T013/T014<T015, T017<T018).
- ИНВЕРСИЯ обязательна: каждый golden краснеет, если граница/N/Offset/единица разошлись; каждый негатив краснеет, если перестал падать своей ошибкой; T020 краснеет, если 5 календарных периодов изменили поведение.
- **`store/codec.go` НЕ трогать** (§MW-3: скользящие/завершённые `Период` недостижимы как first-class значения, `Name`-only round-trip корректен). **`recordSurvives` (INCLUSIVE) НЕ трогать** (§MW-2) — окно делается инклюзивно-эквивалентным. **`trigger_run.go`/`Store`/`ProcessRuntime`/экспорт `eval`/условие и тело триггера — НЕ трогать** (A2-7, §MW-7). **Лексер НЕ трогать** (§MW-2). **`i.now()` (заморожен) — никогда `time.Now()`** (конституция V).
- A2-4 (`по_дате`) — НЕ дублировать: существующая проверка `analyze.go:168` («период требует по_дате») покрывает (§MW-SEM-4); добавить лишь golden (T017 п.г).
- Тексты диагностик — byte-identical §MW-8 (конституция VIII; без точки в конце, `'…'` идентификаторы/единицы/nouns, `«…»` значения; позиция = `PeriodPos`).
- 0 новых пакетов, 0 новых зависимостей (`time.AddDate` — stdlib), `value` остаётся листовым, ряд значений ОТЛОЖЕН (A2-1), все программы v1 валидны.
- §MW-D-WINDOW-EDGE: нормализация месяцев Go `AddDate` принята; DoD-golden на `FixedClock{2026,6,15}` (НЕ конец месяца) + явный edge `{2026,5,31}` (T013) с честным нормализованным значением.
- Доковые синки больших доков (SPEC §10/§12/§13, `source-metric-model.md`, README) — ПРЕДЛАГАЮТСЯ на M1-гейте (§MW-13), НЕ правятся в импл-чате без гейта.
- Коммитить после каждой задачи или логической группы; на любом checkpoint можно валидировать фазу барьером `cd src && go test ./...`.
</content>
</invoke>
