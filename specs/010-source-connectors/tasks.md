# Tasks: Коннекторы источника (тип: + схема поля:) (010-A1)

**Input**: Design documents from `/specs/010-source-connectors/`

**Prerequisites**: plan.md (есть), spec.md (3 US — P1/P2/P3, FR-001..FR-018, SC-001..SC-009), research.md, data-model.md (5 структур + матрица коэрсии), якорь `docs/source-connectors-model.md` §SC-0..§SC-13 + предрешённые развилки A1-1…A1-10.

**Природа фичи**: чистый фронтенд + загрузчик. 0 новых пакетов, 0 новых зависимостей (CSV/JSON — stdlib), 0 NEEDS CLARIFICATION. Аддитивно расширяются лексер (+`KW_TYPE`), AST (`SourceDecl`+`FieldDef`), парсер (`тип:`/`поля:`), семантика (4 проверки §SC-4-sem), загрузчик (`loadCSV`/`loadNDJSON`/`applySchema`). Пакет `value` и контракт `Store`/`ProcessRuntime`/движок/триггеры — НЕ трогаются. Все программы v1 остаются валидными (источник без `тип:`/`поля:` ≡ v1 JSON schemaless).

## Формат: `[ID] [P?] [Story?] Описание`

- **[P]**: можно параллелить (разные файлы, нет зависимостей).
- **[Story]**: к какой US относится (US1/US2/US3). Phase A–B (Foundational) и Phase G (Polish) — без [Story]; Phase C–F несут [Story] там, где задача однозначно служит одной истории.
- Каждая задача содержит ТОЧНЫЙ путь файла, действие в одну строку и удовлетворяемые FR/SC/§SC.
- **TESTS-FIRST (конституция VI)**: для лексера/парсера/семантики тест-задача пишется и КРАСНЕЕТ до прод-задачи того же правила. Тест-замки помечены `🔒 ТЕСТ-ЗАМОК`; инверсия 009-замка — `🔁 ИНВЕРСИЯ`.

**Структура — по ФАЗАМ РЕАЛИЗАЦИИ (каждая — барьер-гейт A→B→C→D→E→F→G).** Внутри фазы — `[P]`. Барьер: фаза N+1 не стартует, пока фаза N не зелёная (`cd src && go test ./...`).

**Пути (факт репозитория, верифицировано)**: корень `…/ladix/`; модуль в `src/` (`src/go.mod`, `go 1.25.0`); лексер `src/internal/lexer/{token.go,keywords.go}` (`reservedWords` — 12 слов, строки 29–31 содержат `"тип": true`; `keywords` — 34 слова, `KW_FILE` уже в token.go:43); AST `src/internal/ast/decl.go` (`SourceDecl` со строки 21, конструктор `NewSourceDecl`); парсер `src/internal/parser/parse_decl.go` (`parseSourceDecl`:41, `metricAttrName`:95, `openAttrBlock`:372, `msgUnknownAttr`/`msgDuplicateAttr`/`msgEmptyBlock`); загрузчик `src/internal/eval/source_loader.go` (`loadSource`:24, `i.recordCache[name]`:76, JSON-путь `decodeValue`/`decodeObject`); дата-парсер `src/internal/eval/builtins_date.go:61` (`parseISODate`); запись `src/internal/value/record.go` (`Keys()`/`Get()`/`NewRecord` — Set-метода нет, `fields` приватны); builtins-сноска `src/internal/eval/builtins.go:53-62,75`; 009-замок `src/cmd/ladix/docs_alignment_test.go:80` (`TestDocsAlignmentA4TipReserved`); golden-инфраструктура `src/cmd/ladix/{golden_test.go,trigger_golden_test.go,metric_test.go}` (`examplePath`/`realMain`/`writeProgFixture`/`maskIDs`/`fixedClock2026`/`withRepoRoot`); парс-сет `src/internal/parser/examples_test.go` (`TestExamplesParseCleanSet`); витрина `examples/*.ladix` + `examples/MANIFEST.md`.

---

## Phase A — Лексер: `KW_TYPE` + разрезервирование `тип` (Foundational, §SC-3)

**Назначение**: ввести ключевое слово `KW_TYPE`, убрать `тип` из `reservedWords`. **§SC-D-RESERVE: критично — `тип` делается keyword, НЕ обычным IDENT** (иначе `тип(5)` распарсится как вызов builtin → случайно активирует `тип(x)`, что хартия §3/§9 запрещает). `поля` ОБЯЗАН остаться `IDENT` (v1-программа может использовать `поля` как переменную — keyword-изация сломала бы обр. совместимость). `KW_TYPE` НЕ добавляется в `startsExpression`/`parsePrimary` → `тип(5)` = парс-ошибка → builtin `тип` остаётся dormant, достижимость 34 неизменна.

**⚠️ КРИТИЧНО (барьер)**: вся фаза C (парсер) матчит атрибут по `attrTok.Lexeme == "тип"` — лексер обязан эмитить токен для `тип` ДО неё.

- [ ] T001 [P] 🔒 ТЕСТ-ЗАМОК (Phase A) Табличные тесты лексера в `src/internal/lexer/lexer_test.go` (или соседний `keywords_test.go`): (а) `тип` лексится как `KW_TYPE` (не IDENT, не lex-ошибка); (б) `поля` остаётся `IDENT`; (в) разрезервирование — `тип` БОЛЬШЕ не даёт L-11 «зарезервированное слово»; (г) аннотации `Целое`/`Дробное`/`Строка`/`Логическое`/`Дата` и значения `json`/`csv`/`ndjson` — `IDENT`. Тесты КРАСНЕЮТ до T002/T003. Покрывает FR-001/§SC-3/§SC-D-RESERVE/§SC-D-LEX-MIN (конституция VI).
- [ ] T002 [P] (Phase A) Добавить `KW_TYPE` (новый `TokenType`) в `src/internal/lexer/token.go` рядом с `KW_RUN` (строка ~63), завести запись в `tokenNames`; обновить счётчик-комментарий эмитируемых видов (67→68). НЕ добавлять `KW_TYPE` в `startsExpression`/`parsePrimary`. Удовлетворяет §SC-D-LEX-MIN/FR-016.
- [ ] T003 (Phase A) В `src/internal/lexer/keywords.go`: удалить `"тип": true` из `reservedWords` (счётчик 12→11, коммент строки 27/29–31), добавить `"тип": KW_TYPE` в `keywords` (счётчик 34→35, коммент строки 3). `поля` НЕ трогать (остаётся незарезервированным IDENT). Зависит от T002 (тип `KW_TYPE` существует). Удовлетворяет FR-001/§SC-D-RESERVE/§SC-D-LEX-MIN.

**Checkpoint A**: `тип`→`KW_TYPE`, `поля`→`IDENT`; счётчики keyword/reserved/токенов выровнены; T001 зелёный. `cd src && go test ./internal/lexer/...` ok.

---

## Phase B — AST: `SourceDecl` +`Type`/`Fields`/позиции, узел `FieldDef` (§SC-2)

**Назначение**: аддитивно расширить `SourceDecl`; добавить листовой узел `FieldDef`. **§SC-D-1**: presence `тип:`/`поля:` через `Pos.Line != 0` (как `MetricAttrPos`), не nil. **§SC-D-2**: `FieldDef` несёт локальную `ast.Position`, НЕ импортирует `errors` (`ast` остаётся листовым, конституция IV/VII). **§SC-D-3**: конструктор `NewSourceDecl` сохраняет сигнатуру; `Type`/`Fields` заполняются сеттер-присваиванием парсером.

- [ ] T004 [P] 🔒 ТЕСТ-ЗАМОК (Phase B) Тест конструкции AST в `src/internal/ast/decl_test.go` (или соседний): `SourceDecl` с заданными `Type`/`TypePos`/`Fields`/`FieldsPos` и `FieldDef{Name,TypeName,Pos}` — поля доступны, presence `Line!=0` отличает заданный атрибут от нулевого; v1-форма даёт нулевые `Type`/`Fields`. КРАСНЕЕТ до T005. Покрывает §SC-2/§SC-D-1 (конституция VI).
- [ ] T005 (Phase B) В `src/internal/ast/decl.go` (рядом с `SourceDecl`:21): добавить поля `Type Ident`, `TypePos Position`, `Fields []FieldDef`, `FieldsPos Position` (дословно §SC-2); добавить новый листовой узел `FieldDef{Name Ident; TypeName Ident; Pos Position}`. `NewSourceDecl` — сигнатуру НЕ менять (§SC-D-3). Зависит от T004. Удовлетворяет FR-001/FR-003/FR-004/§SC-2/§SC-D-1/2/3.

**Checkpoint B**: AST расширен аддитивно, `ast` листовой (без `errors`); T004 зелёный.

---

## Phase C — Парсер: `тип:` голый IDENT + `поля:` вложенный блок (§SC-4)

**Назначение**: `parseSourceDecl` (`parse_decl.go:41`) — цикл атрибутов на `switch attrTok.Lexeme` с валидатором `sourceAttrName(lexeme) ∈ {файл,тип,поля}` (зеркало `metricAttrName`:95). **§SC-D-PARSE-2**: `тип:` → голый IDENT (`json|csv|ndjson` — валидация семантическая, парсер принимает любой IDENT). **§SC-D-PARSE-3**: `поля:` → вложенный блок через `openAttrBlock()`, внутренний цикл `for !p.check(DEDENT) && !p.check(EOF)` с собственным `seen map` на имена полей, восстановление через `continue` (стиль `parseStepDecl`), пустой блок → `msgEmptyBlock`. Дубль атрибута/неизвестный атрибут — переиспользуют `msgDuplicateAttr`/`msgUnknownAttr` (§SM-9). `KW_TYPE` в выражении отвергается `parsePrimary` → `тип(5)` = парс-ошибка.

**⚠️ Барьер**: фаза D (семантика) читает `sd.Type`/`sd.Fields`, заполняемые здесь.

- [ ] T006 [P] 🔒 ТЕСТ-ЗАМОК (Phase C) Табличные ПОЗИТИВНЫЕ тесты парсера в `src/internal/parser/parse_decl_test.go`: (а) `тип: csv` → `sd.Type.Name=="csv"`, `TypePos.Line!=0`; (б) `поля:`-блок из ≥2 строк `имя: Тип` → срез `FieldDef` с именами/типами/позициями; (в) v1-форма (только `файл:`) → нулевые `Type`/`Fields`. КРАСНЕЕТ до T009/T010. Покрывает FR-001/FR-003/§SC-D-PARSE-2/3 (конституция VI).
- [ ] T007 [P] 🔒 ТЕСТ-ЗАМОК (Phase C) Табличные НЕГАТИВНЫЕ тесты парсера в `src/internal/parser/parse_decl_test.go`: (а) дубль `тип:` → `msgDuplicateAttr("тип")`; (б) дубль имени поля в `поля:` → `msgDuplicateAttr`/доменная; (в) пустой `поля:` (INDENT сразу DEDENT) → `msgEmptyBlock`; (г) неизвестный атрибут источника → `msgUnknownAttr`. КРАСНЕЕТ до T009/T010. Покрывает edge-cases §SC-D-PARSE-3/spec Edge Cases (конституция VI).
- [ ] T008 [P] 🔒 ТЕСТ-ЗАМОК + 🔁 ИНВЕРСИЯ (Phase C) Тест в `src/internal/parser/parse_expr_test.go` (или parse_decl_test): `тип(5)` в выражении → парс-ошибка (`KW_TYPE` не начинает выражение), НЕ успешный разбор вызова. Это нижний слой инварианта «`тип(x)` недостижим» (полный CLI-замок — T028). КРАСНЕЕТ, если `KW_TYPE` попадёт в `parsePrimary`. Покрывает FR-016/§SC-D-RESERVE/§SC-10 #7.
- [ ] T009 (Phase C) В `src/internal/parser/parse_decl.go`: ввести `sourceAttrName(lexeme) bool` (зеркало `metricAttrName`:95) для `{файл,тип,поля}`; переписать цикл `parseSourceDecl` на `switch attrTok.Lexeme` с `seen map` (дубль → `msgDuplicateAttr`, неизвестный → `msgUnknownAttr`). Ветка `тип:` — `KW_TYPE` `:` `IDENT` `NEWLINE` → `sd.Type=Ident`, `sd.TypePos=attrTok.Pos` (§SC-D-PARSE-2). Зависит от T006/T007/T005. Удовлетворяет FR-001/FR-002/§SC-D-PARSE-1/2.
- [ ] T010 (Phase C) В `src/internal/parser/parse_decl.go`: ветка `поля:` — `openAttrBlock()` + внутренний цикл `for !p.check(DEDENT) && !p.check(EOF)` собирает `FieldDef{Name,TypeName,Pos}` (строки `IDENT(имя) : IDENT(тип) NEWLINE`), собственный `seen map` на имена (дубль → `msgDuplicateAttr`, `continue`-восстановление), пустой блок → `msgEmptyBlock`; `expect(DEDENT)` (§SC-D-PARSE-3). Зависит от T009. Удовлетворяет FR-003/FR-004/§SC-D-PARSE-3.

**Checkpoint C**: `тип:`/`поля:` парсятся, негативы дают канон §SM-9, `тип(5)` = парс-ошибка; T006/T007/T008 зелёные.

---

## Phase D — Семантика: 4 проверки источника (§SC-4-sem, analyze.go)

**Назначение**: в `Analyze` (после регистрации источников) — статические проверки (без чтения файла), позиции из `TypePos`/`FieldDef.Pos`. Тексты — дословный канон §SC-9.A. **§SC-4-sem п.4**: дубли имён полей ловит парсер (T007/T010), семантика НЕ дублирует.

- [ ] T011 [P] 🔒 ТЕСТ-ЗАМОК (Phase D) Табличные тесты семантики в `src/internal/eval/analyze_decl_test.go`: (а) `тип: xml` → `источник '<имя>': неизвестный тип источника 'xml' (допустимо: json, csv, ndjson)`, позиция `TypePos`; (б) `тип: csv` без `поля:` → `тип 'csv' требует объявления полей (поля:)`; (в) поле с типом `Деньги` → `неизвестный тип поля 'Деньги' (допустимо: Целое, Дробное, Строка, Логическое, Дата)`, позиция `FieldDef.Pos`; (г) `json` без `поля:` — валиден (без ошибки). КРАСНЕЕТ до T012. Тексты byte-exact §SC-9.A. Покрывает FR-004/FR-017/SC-005 (конституция VI/VIII).
- [ ] T012 (Phase D) В `src/internal/eval/analyze.go`: 4 проверки источника §SC-4-sem — (1) `Type.Name ∈ {json,csv,ndjson}` (пусто=json ок) иначе `semErr` (позиция `TypePos`); (2) `Type ∈ {csv,ndjson}` && `len(Fields)==0` → `semErr` (позиция `TypePos`/decl); (3) каждый `FieldDef.TypeName.Name ∈ {Целое,Дробное,Строка,Логическое,Дата}` иначе `semErr` (позиция `FieldDef.Pos`); (4) дубли полей — НЕ дублировать (парсер). Тексты §SC-9.A byte-identical. Зависит от T011. Удовлетворяет FR-004/FR-017/§SC-4-sem/§SC-9.A.

**Checkpoint D**: 3 семантические ошибки + позиции; `json` без схемы валиден; T011 зелёный.

---

## Phase E — Загрузчик + коэрсия: `loadCSV`/`loadNDJSON`/`applySchema` (§SC-6, source_loader.go)

**Назначение**: `loadSource` (`source_loader.go:24`) — диспетчер по `decl.Type.Name` (`""`→json — §SC-6/A1-2). **§SC-D-CSV**: `loadCSV` через `encoding/csv` (stdlib, 0 deps — данные, не грамматика; конституция I/II), 1-я строка заголовок, `,`+UTF-8, заголовок обязан содержать все объявленные поля (иначе load-ошибка), лишние столбцы игнорируются (A1-6). **§SC-D-NDJSON**: `loadNDJSON` построчно, пустые строки пропускаются (A1-9), не-объект → load-ошибка. **§SC-D-RECORD**: `applySchema` НЕ мутирует — пересобирает через `NewRecord(keys,fields)`/`Keys()`/`Get()` (пакет `value` НЕ трогается, конституция VII). **§SC-D-COERCE**: матрица коэрсии (CSV-всё-Строка / JSON-NDJSON-типизировано), единственный промоушен `Целое→Дробное` (§SC-D-COERCE-PROMO), CSV-Целое без `.`/`e` (§SC-D-COERCE-INT). **§SC-D-DATE**: `Дата` через тот же `parseISODate` (`builtins_date.go:61`, НЕ дублировать). Тексты ошибок — дословный канон §SC-9.B.

**⚠️ Барьер**: фаза F (golden/примеры) гоняет эти загрузчики.

- [ ] T013 [P] 🔒 ТЕСТ-ЗАМОК (Phase E) Фикстуры данных в `src/internal/eval/testdata/`: эквивалентные наборы записей в формах `*.json` (со схемой) / `*.csv` / `*.ndjson` (одинаковые скаляры) + кейсы ошибок: отсутствие поля, несовпадение типа, невалидная дата, CSV без заголовкового поля, не-объект NDJSON, null/пустая ячейка. Покрывает §SC-10 #2/§SC-D-RECORD edge. (Фикстуры — общий вход для T014–T018.)
- [ ] T014 [P] 🔒 ТЕСТ-ЗАМОК (Phase E) Тесты `loadCSV`/`loadNDJSON` в `src/internal/eval/source_loader_test.go`: (а) CSV с заголовком грузится, лишние столбцы игнорируются; (б) CSV без объявленного столбца → `в заголовке CSV «<путь>» отсутствует поле '<поле>'`; (в) NDJSON с пустыми строками — сквозная нумерация записей с 1; (г) NDJSON не-объект → `запись N: некорректный JSON`/«не является объектом». КРАСНЕЮТ до T019/T020. Покрывает FR-008/FR-009/§SC-D-CSV/NDJSON/§SC-9.B (конституция VI).
- [ ] T015 [P] 🔒 ТЕСТ-ЗАМОК (Phase E) Тесты коэрсии `applySchema` в `src/internal/eval/source_loader_test.go`: по строке матрицы §SC-D-COERCE — CSV-строка→Целое/Дробное/Логическое/Дата; JSON `Целое`→`Дробное` (промоушен ок); `Дробное`→`Целое` (ошибка `ожидался Целое, получено Дробное`); CSV-`Целое` с `«12.5»` → «не является целым»; CSV-`Логическое` вне `истина`/`ложь` → «не является логическим (ожидалось истина/ложь)»; отсутствие поля → «отсутствует объявленное поле»; лишнее поле — игнор без ошибки; null/`""` → A1-10/A1-7 (edge §SC-D-RECORD). КРАСНЕЮТ до T021. Все тексты byte-exact §SC-9.B. Покрывает FR-005/FR-006/FR-010/FR-013/SC-005/SC-006 (конституция VI/VIII).
- [ ] T016 [P] 🔒 ТЕСТ-ЗАМОК (Phase E) Тест распознавания дат в `src/internal/eval/source_loader_test.go`: поле `Дата` со строкой `«2026-05-31»` → `value.Дата` (без явного `дата(...)`); `«2026-13-40»`/`«31.05.2026»` → `«<знач>» не является датой (ожидался формат ГГГГ-ММ-ДД)` с именем источника/поля/N. КРАСНЕЕТ до T021. Покрывает FR-007/SC-004/§SC-D-DATE (конституция VI).
- [ ] T017 (Phase E) В `src/internal/eval/source_loader.go`: превратить `loadSource` в диспетчер `switch decl.Type.Name { case "","json": loadJSON; case "csv": loadCSV; case "ndjson": loadNDJSON }`; существующий JSON-путь вынести в `loadJSON` без изменения поведения; после загрузки — `if len(decl.Fields) > 0 { recs = applySchema(decl, recs) }`; кеш `i.recordCache[name]` (§SC-D-CACHE). Зависит от Phase B/C (поля `Type`/`Fields`). Удовлетворяет FR-002/FR-011/§SC-6.
- [ ] T018 (Phase E) В `src/internal/eval/source_loader.go`: реализовать `loadCSV(decl,path)` через `encoding/csv` (заголовок = 1-я строка, `,`+UTF-8, проверка наличия объявленных столбцов → load-ошибка `в заголовке CSV …`, лишние игнор) и `loadNDJSON(decl,path)` (построчно, пустые строки skip, каждая непустая → объект через JSON-декодер, не-объект → load-ошибка). 0 новых зависимостей. Зависит от T017, T014. Удовлетворяет FR-008/FR-009/FR-014/§SC-D-CSV/NDJSON.
- [ ] T019 (Phase E) В `src/internal/eval/source_loader.go`: реализовать `applySchema(decl,recs)` пересборкой через `NewRecord`/`Keys()`/`Get()` (§SC-D-RECORD, `value` НЕ трогать) — присутствие = членство `name` в `Keys()` (отсутствие → load-ошибка), коэрсия по матрице §SC-D-COERCE (CSV-всё-Строка / JSON-NDJSON типизировано), промоушен только `Целое→Дробное`, `Дата`→`parseISODate` (переиспользовать `builtins_date.go:61`, НЕ дублировать); лишние поля сохраняются, порядок ключей цел. Все тексты §SC-9.B byte-identical. Зависит от T017, T015, T016. Удовлетворяет FR-005/FR-006/FR-007/FR-010/FR-012/FR-013/§SC-D-COERCE/RECORD/DATE.

**Checkpoint E**: json/csv/ndjson грузятся, коэрсия/даты/ошибки данных работают, `value` не тронут, 0 deps; T013–T016 зелёные. `cd src && go test ./internal/eval/...` ok.

---

## Phase F — Тест-замки CLI, эквивалентность форматов, витрина, инверсия 009 (§SC-10)

**Назначение**: golden-приёмка эквивалентности форм + ошибок данных через CLI; витрина `examples/*.ladix` + фикстуры + MANIFEST; инверсия 009-замка `тип`; регресс-замок обратной совместимости. Детерминизм — `FixedClock{2026,5,31}` + `t.TempDir()` с абсолютным путём в `файл:` (паттерн `trigger_golden_test.go`/`fixedClock2026`); ожидания — inline-строки. Негатив-замок — `assertNegativeExample` (exit 1, stderr byte-exact §13, без `.go:`/goroutine).

**⚠️ Порядок внутри пары пример↔замок**: пример (а) создаётся ДО/ВМЕСТЕ со своим замком (б). Позитивы добавляются в `TestExamplesParseCleanSet`; негативы НЕ добавляются (они должны падать).

- [ ] T020 [P] 🔒 ТЕСТ-ЗАМОК (Phase F, US3) Golden эквивалентности форматов в `src/cmd/ladix/` (новый `source_format_golden_test.go` или блок `metric_test.go`): один набор записей в json(со схемой)/csv/ndjson из `testdata/`, одна метрика, `fixedClock2026`, абс. путь — три прогона дают БАЙТ-ТОЧНО равный скаляр. 🔁 если форматы разойдутся — красный. Зависит от Phase E, T013. Покрывает SC-001/SC-002/§SC-10 #2.
- [ ] T021 [P] 🔒 ТЕСТ-ЗАМОК (Phase F, US1) Golden диспетчера «`тип:` опущен → json» в `src/cmd/ladix/`: источник без `тип:`/`поля:` грузится как v1 JSON schemaless, скаляр не меняется. 🔁 регресс диспетчера. Зависит от Phase E. Покрывает FR-002/FR-011/SC-007/§SC-10 #3.
- [ ] T022 [P] 🔒 ТЕСТ-ЗАМОК (Phase F, US2) Negative-замки ошибок данных в `src/cmd/ladix/` (новый `source_negatives_test.go`): через `realMain`/`assertNegativeExample` — отсутствие поля (A1-5), несовпадение типа `Дробное`→`Целое` (A1-10), невалидная дата (A1-7), CSV без заголовкового поля, семантика (неизвестный `тип:`, csv без `поля:`, неизвестный тип поля) — каждый exit 1 + stderr byte-exact §SC-9 + нет `.go:`/goroutine. 🔁 если перестал падать ИМЕННО этой ошибкой — красный. Зависит от Phase D/E, T013. Покрывает SC-004/SC-005/§SC-10 #5/§SC-9.
- [ ] T023 [P] (Phase F, US1) Создать `examples/` CSV-демо: `examples/<csv-демо>.ladix` (`файл: "data/orders.csv"`, `тип: csv` + схема `поля:` `дата_заказа: Дата`/`сумма_заказа: Дробное`/`статус: Строка`, фильтр `где статус == "оплачен"` + `сумма(сумма_заказа)`) + фикстура **в корне репо** `data/orders.csv` (рядом с `data/sales.json`; заголовок + строки). **Путь — `data/orders.csv` (резолв через `withRepoRoot` от корня, как `data/sales.json`), НЕ `examples/data/`.** Parse-clean. Покрывает SC-003/US1.
- [ ] T024 [P] (Phase F, US3) Создать `examples/` NDJSON-демо: `examples/<ndjson-демо>.ladix` (`файл: "data/orders.ndjson"`, `тип: ndjson` + схема) + фикстура **в корне** `data/orders.ndjson` (объекты + пустые строки). Parse-clean. Покрывает FR-009/US3.
- [ ] T025 [P] (Phase F, US2) Создать `examples/` JSON-со-схемой демо: `examples/<json-схема>.ladix` (`файл: "data/orders.json"`, `тип: json` + `поля:` поверх JSON-массива) + фикстура **в корне** `data/orders.json`. Parse-clean. Покрывает FR-003/US2.
- [ ] T026 [P] (Phase F, US2) Создать `examples/ошибочная.ladix` (готовит M-DX, §SC-10 #6): кейс с ошибкой схемы/типа источника — exit 1, §13-канон. При создании зафиксировать ФАКТИЧЕСКИЙ байт-точный stderr и передать в golden T029. Покрывает §SC-10 #6.
- [ ] T027 (Phase F, US1/US2/US3) Добавить позитивные демо (T023/T024/T025) в `TestExamplesParseCleanSet` (`src/internal/parser/examples_test.go`); `ошибочная.ladix` (T026) — НЕ добавлять (должен падать). Зависит от T023–T026.
- [ ] T028 🔒 ТЕСТ-ЗАМОК (Phase F) Golden CSV-демо в `src/cmd/ladix/`: прогон `examples/<csv-демо>.ladix` (через `withRepoRoot` для резолва `data/orders.csv`, `fixedClock2026` если дата-зависимо) → байт-точный ожидаемый скаляр, exit 0 (SC-003 DoD-срез M1). 🔁 если CSV→метрика разошлась — красный. Зависит от T023. Покрывает SC-003.
- [ ] T029 🔒 ТЕСТ-ЗАМОК (Phase F, US2) Negative-замок `examples/ошибочная.ladix` в `src/cmd/ladix/` через `assertNegativeExample`: exit 1 + stderr byte-exact (пин из T026) + нет `.go:`. 🔁. Зависит от T026. Покрывает §SC-10 #6.
- [ ] T030 🔁 ИНВЕРСИЯ 009-ЗАМКА (Phase F) Обновить `TestDocsAlignmentA4TipReserved` (`src/cmd/ladix/docs_alignment_test.go:80`) под §SC-10 #7: СОХРАНИТЬ (a) exit 1 и (c) stdout НЕ содержит «Целое» (ядро инварианта — `тип(x)` недостижим, builtin dormant); (b) заменить assert «зарезервированное слово» (строки 94–95) на новый текст ПАРС-диагностики `parsePrimary` для `KW_TYPE` (определить ЭМПИРИЧЕСКИ в impl-прогоне `печать(тип(5))`). Замок ОБЯЗАН кусаться: если `тип(x)` станет достижим → stdout=«Целое» → красный (мутпроба Ф8/G). Зависит от Phase A/C. Покрывает FR-016/SC-009/§SC-10 #7.
- [ ] T031 (Phase F) Обновить коммент-факты в `src/internal/eval/builtins.go:53-62` (механизм: `тип` = keyword `KW_TYPE`, НЕ reserved; достижимость 34 неизменна, 35 зарегистрировано) и соответствующий коммент в `src/internal/eval/builtins_test.go` (если есть `:63`). НЕ удалять регистрацию/реализацию `builtinTip` (`builtins.go:75`). Зависит от Phase A. Покрывает FR-016/§SC-D-RESERVE/§SC-10 #7.
- [ ] T032 🔒 ТЕСТ-ЗАМОК (Phase F) Регресс обратной совместимости: подтвердить, что все существующие `examples/*.ladix` и golden v1 (`метрики.ladix`/`выручка.ladix` и пр.) — БЕЗ изменения поведения (источник без `тип:`/`поля:` ≡ v1 JSON schemaless); `cd src && go test ./...` — все ранее зелёные замки зелёные. Зависит от Phase E. Покрывает FR-011/FR-015/FR-018/SC-007/SC-008/§SC-10 #1.
- [ ] T033 (Phase F) Обновить `examples/MANIFEST.md`: записи новых A1-демо (CSV/NDJSON/JSON-схема/ошибочная) — файл / назначение / golden-stdout либо пометка дата-зависимости + ссылка на run-golden. Зависит от T023–T029.

**Checkpoint F**: эквивалентность форм + ошибки данных под CLI-замками; витрина 4 демо + фикстуры + MANIFEST; инверсия 009 сохраняет (a)+(c); v1-регресс зелёный.

---

## Phase G — Self-check: gofmt/vet/build/test + мутпробы + чистое дерево

**Назначение**: финальный гейт фичи; каждый новый замок кусается при инверсии прод-логики.

- [ ] T034 (Phase G) Финальный гейт из `src/`: `cd src && gofmt -l . && go vet ./... && go build ./... && go test ./... -count=1` — gofmt пуст, vet/build PASS, все пакеты `ok`, exit 0. Покрывает SC-008/конституция I.
- [ ] T035 (Phase G) Мутпробы (Ф8): подтвердить, что каждый новый замок ЛОМАЕТСЯ при инверсии прод-логики — диспетчер (`csv`↔`json`), коэрсия (убрать промоушен / снять CSV-int-строгость), валидатор (расширить множество типов), дата-парс (ослабить формат), `KW_TYPE`-в-выражении (добавить в `parsePrimary` → T030/T008 краснеют, stdout=«Целое»). НЕ коммитить мутации — только проверить «кусается». Покрывает §SC-10 #8/SC-009.
- [ ] T036 (Phase G) Чистота дерева: `git status --short` пуст (собранный `ladix`/`src/ladix` игнорируется, на ФС не лежит); 0 новых зависимостей (`git diff src/go.mod src/go.sum` пуст); пакет `value`/`Store`/движок/триггеры не тронуты. Покрывает FR-014/FR-015/SC-008/§SC-11.

**Checkpoint G**: гейт зелёный, мутпробы кусают, дерево чистое, 0 deps, изоляция фронтенда цела. Конвейер 010 готов к мержу.

---

## Dependencies / Phase gates

### Барьеры фаз (A→B→C→D→E→F→G)

- **Phase A (Лексер)** — без зависимостей; БЛОКИРУЕТ всё (парсер матчит токен `тип`). Внутри: T001 [P] (тест) → T002 [P] → T003.
- **Phase B (AST)** — после A; БЛОКИРУЕТ парсер/семантику (поля `Type`/`Fields`). Внутри: T004 [P] → T005.
- **Phase C (Парсер)** — после B; БЛОКИРУЕТ семантику/загрузчик (заполняет `sd.Type`/`sd.Fields`). Внутри: T006/T007/T008 [P] (тесты) → T009 → T010.
- **Phase D (Семантика)** — после C; статические проверки до load. Внутри: T011 [P] → T012.
- **Phase E (Загрузчик)** — после C (D желательно до E, чтобы невалидные деклы не доходили; T013 фикстуры [P] вперёд). Внутри: T013–T016 [P] (тесты+фикстуры) → T017 → T018/T019.
- **Phase F (Замки+витрина)** — после E (и D для семантических негативов). Пары пример↔замок: T023→T028, T026→T029; T020/T021/T022 после E; T030/T031 после A/C; T027 после T023–T026; T033 после T023–T029.
- **Phase G (Self-check)** — после всего; T034→T035→T036.

### Внутри фаз ([P] = разные файлы, нет зависимостей)

- A: T001 ∥ T002 (тест и token.go); T003 после T002 (нужен тип `KW_TYPE`).
- B: T004 (тест) ∥ старт T005-плана; T005 после T004 (tests-first).
- C: T006 ∥ T007 ∥ T008 (тесты, один файл `parse_decl_test.go`/`parse_expr_test.go` — при worktree-изоляции один владелец файла, секвенировать запись в общий тест-файл); T009 после тестов; T010 после T009.
- D: T011 (тест) → T012.
- E: T013 ∥ T014 ∥ T015 ∥ T016 (фикстуры + тесты, разные testdata-файлы; `source_loader_test.go` — один владелец, секвенировать запись); T017 → T018 ∥ T019.
- F: T020 ∥ T021 ∥ T022 (golden, разные тест-файлы); T023 ∥ T024 ∥ T025 ∥ T026 (разные `examples/*.ladix`+фикстуры); затем T027/T028/T029/T030/T031/T032/T033.
- G: T034 → T035 → T036 (последовательно).

### Один-владелец-файла (worktree-изоляция implement)

- `src/internal/parser/parse_decl_test.go` — T006/T007 (+возможно T008): один агент, последовательно.
- `src/internal/eval/source_loader_test.go` — T014/T015/T016: один агент, последовательно.
- `src/internal/parser/parse_decl.go` — T009→T010 (тот же файл, секвенированы).
- `src/internal/eval/source_loader.go` — T017→T018→T019 (тот же файл, секвенированы).
- `examples/MANIFEST.md` — только T033.

---

## Notes

- [P] = разные файлы / содержательно независимо.
- TESTS-FIRST (конституция VI): лексер/парсер/семантика — тест-задача КРАСНЕЕТ до прод-задачи того же правила (T001<T003, T004<T005, T006/T007<T009/T010, T011<T012, T014/T015/T016<T018/T019).
- ИНВЕРСИЯ обязательна: каждый golden краснеет, если форма/коэрсия/дата разошлись; каждый негатив краснеет, если перестал падать своей ошибкой; T030/T008 краснеют, если `тип(x)` стал достижим (stdout=«Целое»).
- `value`/`Store`/`ProcessRuntime`/движок/триггеры/фронтенд `где`/`агрегат`/`период`/`по_дате` — НЕ трогать (§SC-8/§SC-11).
- `parseISODate` (`builtins_date.go:61`) — переиспользовать, НЕ дублировать (§SC-D-DATE).
- Тексты диагностик — byte-identical §SC-9 (конституция VIII; без точки в конце, `'…'` идентификаторы, `«…»` литералы, N с 1).
- 0 новых пакетов, 0 новых зависимостей (CSV/JSON — stdlib), `тип(x)` остаётся недостижим (builtin dormant), все программы v1 валидны.
- Доковые синки больших доков (SPEC §9/§12/§13, `source-metric-model.md`, README) — ПРЕДЛАГАЮТСЯ на M1-гейте (§SC-13), НЕ правятся в импл-чате без гейта.
- Коммитить после каждой задачи или логической группы; на любом checkpoint можно валидировать фазу барьером `cd src && go test ./...`.
