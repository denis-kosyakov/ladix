# Implementation Plan: Фронтенд процессов v1 — процесс, шаг, действия, запуск

**Branch**: `005-process-frontend` | **Date**: 2026-06-10 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/005-process-frontend/spec.md`

> **Связывающий якорь — источник истины.** Поведение, формы AST, переходы парсера, семантические
> правила и **байт-точные** тексты закреплены в `docs/process-model.md` (§PM-0…§PM-8); решения
> архитектора D-1…D-13 (§PM-1) приняты и **не переоткрываются**. При любом расхождении плана и
> якоря побеждает якорь. Этот план — **HOW**: перекладывает 28 FR спеки + §PM-2/§PM-3/§PM-4/§PM-5/
> §PM-6 в конкретную карту правок по файлам с **проверенными координатами кода** (сверены с
> фактическим деревом `src/` 10.06.2026, а не с номерами строк в якоре — где якорь дрейфует от
> кода, план следует коду).

## Summary

Пятый слой Ladix — **фронтенд процессов v1**: язык учится **объявлять** бизнес-процесс
(`процесс Имя(параметры):`) как последовательность **шагов** (`шаг Имя после …:`) с атрибутами
`исполнитель:`/`срок:`, действиями в теле (`присвоить`/`вызвать`/`уведомить`) и **запускать**
его из императивного кода (`запустить процесс Имя(...)`). Фича — **ЧИСТЫЙ ФРОНТЕНД**: парсер + AST
+ семантический проход; **ни строки исполнения** (рантайм-deferred остаётся до движка 006, §PM-0).

Фича **расширяет** готовый стек 001→004 (лексер, парсер, AST, интерпретатор ядра B, декларативный
слой), а не строит новый, в точности повторяя приём 004 на процессах:

1. **AST** (`internal/ast`) — два новых плоских узла `ProcessDecl`/`StepDecl` + вспомогательная
   `StepAttrPos` (§PM-2, D-1), по точному образцу `MetricDecl`+`MetricAttrPos`. Узлы действий
   (`AssignAction`/`CallAction`/`NotifyAction`, `ast/step.go`) и `RunProcessExpr` (`ast/expr.go`)
   **уже построены** в 003/004 — не вводятся заново (D-2/D-10, FR-009).
2. **Парсер** (`internal/parser`) — снять из `isUnexpectedTopLevel` **только `KW_PROCESS`**
   (`KW_WHEN`/`KW_VALUE`/`LBRACE`/`RBRACE` остаются отвергаемыми, D-6/FR-003); ветвь
   `parseProcessDecl` в `parseTopLevelItem`; новые `parseProcessDecl`/`parseStepDecl`/`parseAfterList`
   — зеркало заголовка `parseFunctionDecl` + блока `parseMetricDecl` с backstop. Дубль атрибута шага
   — `p.error+break` (D-8, **байт-идентично** `parseMetricDecl`).
3. **Семпроход** (`internal/eval/analyze.go`) — реестр `i.processes`; регистрация имени процесса
   (общий глобальный namespace, D-5); новый Шаг 1c `checkProcessDecl` (уникальность шагов, резолв
   `после`-валидатор D-4, `срок`-без-`исполнитель`, анализ тел шагов с засевом параметров D-12);
   контекст-гард действий (вне шага → ошибка, в шаге → ok, D-11); арность `запустить процесс`
   (`checkRunProcess`, резолв **только** против процессов, D-10).
4. **Граница deferred** (§PM-5, **критично**) — eval-**исполнение** не трогается
   (`stmt.go:63-64`/`expr.go:48-51` остаются рантайм-deferred). Действия в шаге и `запустить процесс`
   перестают быть **семантической** ошибкой; их «не поддерживается» переезжает в **рантайм**.
   **Единственная наблюдаемая рантайм-граница 005 = top-level `запустить процесс`**; тело шага в
   рантайме **не исполняется** (`ProcessDecl` — `Decl`, `Run()` его пропускает).
5. **Демо + синк** — подрезать `examples/онбординг.ladix` (убрать `статус_процесса`, D-9/§PM-7);
   синк `examples/MANIFEST.md`, doc-комментарий `ast/node.go:24` (union из 4 деклараций),
   `SPEC §13.4/§7.3/§7.4`, `ARCHITECTURE §4.4/§4.8` — зона якоря, §PM-0.6.

**Технический подход**: только stdlib (`fmt` — единственное в правках), конституция I; новых
пакетов нет — `ProcessDecl`/`StepDecl` добавляются в `internal/ast`, разбор — в `internal/parser`,
валидация — в `internal/eval/analyze.go` (конституция VII; `internal/engine`/`internal/store` **не
заводятся**, D-7/§PM-8). Парсер — ручной recursive descent в стиле 002/004 (конституция II).
**Новых типов ошибок нет**: все диагностики процесса/шага — `СемантическаяОшибка` (`semErr`, D-3/
FR-019); `errors.ОшибкаПроцесса` — это категория **исполнения** (006), её в коде нет и фронтенд её
не порождает. Все тексты — байт-точно из §PM-6 (конституция VIII).

Поставка по приоритетам User Stories: **все три истории — P1** (как в spec): US1 (объявить +
статически проверить) → US2 (действия и зависимости в теле шага) → US3 (запустить из императивного
кода). Фундамент-фаза A (AST) впереди; фазы B (парсер) и C (семпроход) реализуют истории; фаза D —
демо + синк-доки.

## Technical Context

**Language/Version**: Go **1.22** (модуль `github.com/denis-kosyakov/ladix`, корень — `src/`),
без CGO. Подтверждено `src/go.mod`: `go 1.22`.

**Primary Dependencies**: только стандартная библиотека Go. **Новых зависимостей 005 не вводит** —
весь новый код опирается на `fmt` (форматирование текстов §PM-6) и существующие пакеты
`internal/{lexer,ast,parser,eval,value,errors}`. `modernc.org/sqlite` **НЕ используется** — store
deferred до 006 (§PM-8). `testify/require` — точечно в тестах (конституция VI).

**Storage**: N/A. `internal/store`/`internal/engine` в 005 **не заводятся** (D-7/§PM-8, конституция
VII). Фронтенд процессов ничего не хранит и не исполняет — `ProcessDecl`/`StepDecl` живут в AST,
реестр `i.processes` — поле `Interpreter` (как `i.funcs`/`i.sources`/`i.metrics`).

**Testing**: `go test` — три стратегии (как 002/004):
(1) **табличные** (table-driven) тесты по слоям: AST (`ProcessDecl`/`StepDecl`/`StepAttrPos` — поля,
`Pos()`, реализация маркеров — по образцу `ast/decl_test.go`/`ast/step_test.go`); parse-тесты
(параметры опц.; `после` 0/1/N имён; чередование attr/statement; дубль атрибута; пустой блок;
не-шаг в блоке процесса) по образцу `parser/parse_decl_test.go`; семпроход
(`eval/analyze_decl_test.go`-образец: уник шагов, резолв `после`, `срок`-без-`исполнитель`,
контекст-гард действий, `вернуть` в шаге, арность `запустить процесс`);
(2) **exact-match текстов ошибок** — все тексты §PM-6.A/B/C сверяются **байт-в-байт** (SC-001/002/
003; переформулировать запрещено, конституция VIII);
(3) **позитив + регресс-кейс** `онбординг.ladix` (после подрезки): парс чисто + семантика чисто
(SC-004/006); CLI `ladix run онбординг.ladix` → код 1 на рантайм-границе `запустить процесс`.
Co-located `*_test.go`; вывод — через инжектированный `out` (`bytes.Buffer`); `go test -race`
зелёный; **все** регресс-тесты 001/002/003/004 остаются зелёными (FR-027, обновляются 2 кейса
`examples_test.go`).

**Target Platform**: один статический бинарник `ladix` (`go build`), кросс-компиляция
darwin/linux/windows (конституция I).

**Project Type**: компилятор/интерпретатор (tree-walking) — CLI `ladix run` + `ladix metric`. Новых
CLI-подкоманд **нет** (`ladix check` НЕ вводится, D-7/FR-026).

**Performance Goals**: учебно-прикладной масштаб; жёстких целей нет. Семпроход процесса — лёгкий
fail-fast обход AST (как `checkMetricDecl`), не зависит от размера данных.

**Constraints**:

- **Чистый фронтенд (§PM-0/§PM-5, критично):** исполнение тела шага/действий/запуска **не
  реализуется**. eval-строки `stmt.go:63-64` (`AssignAction`/`CallAction`/`NotifyAction` →
  `deferredConstruct`) и `expr.go:48-51` (`RunProcessExpr`/`DurationLit` → `deferredConstruct`)
  **уже корректны для 005 — не трогать** (FR-025). Тело шага в рантайме **недостижимо**
  (`ProcessDecl` — `Decl`; `Run()` `interpreter.go:84-87` пропускает не-`Statement`) → **не писать
  недостижимый рантайм-тест** «действие в шаге → deferred».
- **Снятие cut — минимальное (D-6):** из `isUnexpectedTopLevel` убирается **только `KW_PROCESS`**.
  `KW_WHEN`/`KW_VALUE` (триггеры — 007), `LBRACE`/`RBRACE` остаются отвергаемыми. Грамматика/
  приоритеты выражений и существующие AST-узлы не меняются — расширение строго аддитивно.
- **Без новой категории ошибок (D-3):** `errors.ОшибкаПроцесса` НЕ вводить; все диагностики
  процесса/шага — `СемантическаяОшибка` (`semErr`). Парсер-диагностики — `ОшибкаРазбора`
  (через `p.error`).
- **Тексты — байт-точно из §PM-6** (конституция VIII): payload без завершающей точки; одинарные
  `'…'` для идентификаторов/ключевых слов; `N` нумеруется с 1; позиция = строка/колонка в рунах.
- **Механизм deferred-builtin не меняется (D-9):** `статус_процесса`/`состояние_процесса`/
  `задачи_пользователя` остаются deferred-builtins (семантическая ошибка `функция '<имя>' не
  поддерживается в этой версии`); регресс-тесты `TestBuiltinDeferredAll` не трогаются.
- **`DurationLit` не трогать (D-11):** остаётся семантический deferred в обходимой позиции; в
  `срок:` он не обходится (атрибуты шага не валидируются семпроходом) и проходит.
- Без изменяемого состояния уровня пакета (конституция V): реестр `i.processes` — поле
  `Interpreter`, инициализируется в `NewInterpreter`.
- Граф пакетов **ацикличен**; `internal/ast`, `internal/value`, `internal/errors` — **листовые**
  (новые узлы `ast` опираются только на `ast`-локальную `Position`).

**Scale/Scope**: фронтенд процессов v1 — `процесс` (параметры опц. + ≥1 шаг) + `шаг` (`после`-список
+ `исполнитель`/`срок` + тело-операторы). **2 новых AST-узла** (`ProcessDecl`/`StepDecl`) +
1 вспомогательная (`StepAttrPos`); **1 новое поле** интерпретатора (`i.processes`); **1 новый шаг**
семпрохода (`checkProcessDecl` + `analyzeStep` + `checkRunProcess`); **новый параметр `inStep`**
протаскивается через `checkStmt`/`checkStmts`/`checkElse`; **0 новых встроенных**, **0 новых типов
ошибок**, **0 новых CLI-команд**, **0 новых пакетов**. Реестр текстов: §PM-6.A (3 парсера,
переиспользуются) + §PM-6.B (9 семпроход) + §PM-6.C (4 запуск) = 16 байт-точных строк. Синк
2 doc-файлов (`MANIFEST.md`, `ast/node.go:24`) + зона якоря (`SPEC`/`ARCHITECTURE`).

## Constitution Check

*GATE: проверяется до Phase 0 и повторно после Phase 1. Нарушение допускается только с записью в
Complexity Tracking.*

| Принцип | Требование | Как план соблюдает | Статус |
|---|---|---|---|
| I. Язык и сборка | Go 1.22+, `gofmt`/`go vet` чисто, без CGO, один бинарник, зависимости ограничены | Только stdlib (`fmt`); новых зависимостей нет; `modernc.org/sqlite` НЕ вводится (store — 006); гейты `go build`/`go vet`/`gofmt` (SC-007) | ✅ PASS |
| II. Парсинг — ручной | Лексер/парсер вручную, без генераторов/regex | `parseProcessDecl`/`parseStepDecl`/`parseAfterList` — ручной recursive descent по образцу `parseFunctionDecl` (заголовок) + `parseMetricDecl` (блок); токены `KW_PROCESS`/`KW_STEP`/`KW_AFTER`/`KW_ASSIGNEE`/`KW_DEADLINE` уже в лексере (`keywords.go:20-21`) | ✅ PASS |
| III. Ошибки — явные типы | Типы в `internal/errors` с `Position`, без panic в штатных путях, recover-барьер на CLI | Переиспользуются готовые типы: `ОшибкаРазбора` (парсер) и `СемантическаяОшибка` (семпроход, `semErr`); **новых типов не добавляется** (D-3 — `errors.ОшибкаПроцесса` это 006); control-flow без panic; `guard` на CLI без изменений | ✅ PASS |
| IV. Позиции — сквозные | `Position{Line,Col}` (с 1, в рунах) на каждом узле, печатается строка И колонка | `StepAttrPos{AssigneePos, DeadlinePos}` несёт позицию ключевого слова присутствующего атрибута (как `MetricAttrPos`) для точной диагностики `срок`-без-`исполнитель`; каждый `Ident` в `After` несёт свою `Pos()` для диагностики `после`; `ProcessDecl.Pos()`=токен `процесс`, `StepDecl.Pos()`=токен `шаг` | ✅ PASS |
| V. Без глобального состояния | Нет mutable package-level state; зависимости инжектируются | `i.processes map[string]*ast.ProcessDecl` — поле `Interpreter`, init в `NewInterpreter` (как `i.funcs`/`i.sources`/`i.metrics`); package-var не вводится | ✅ PASS |
| VI. Тесты — вперёд | Табличные тесты (вкл. негативные) — часть каждой задачи | Tests-first по слоям (AST/parse/семпроход) + exact-match (16 текстов §PM-6) + позитив/регресс `онбординг.ladix`; `-race`; вывод через `bytes.Buffer` | ✅ PASS |
| VII. Раскладка проекта | `cmd/ladix` + `internal/{...}`; граф без циклов; `value`/`errors`/`ast` листовые | Новых пакетов нет; `internal/engine`/`internal/store` НЕ заводятся (D-7); `ast` правится аддитивно и остаётся листовым (`ProcessDecl`/`StepDecl` — только `ast`-локальная `Position`); цикла не возникает | ✅ PASS |
| VIII. Язык сообщений | Все сообщения по-русски, двухстрочный канон, тексты дословно | Новые тексты — ДОСЛОВНО из §PM-6.A/B/C; переиспользуются существующие (`msgDuplicateAttr`/`msgEmptyBlock`/`msgUnexpected` из 004; `'<имя>' уже объявлено…`/`checkReservedDeclName` из 004; deferred-тексты из `eval-model §8.3`); deferred-строки eval не трогаются | ✅ PASS |
| IX. Спека — источник истины | Поведение из размещённых документов; пробел → остановиться и спросить | Единственный связывающий источник — `docs/process-model.md §PM-0…§PM-8`; фон — `SPEC §3/§7.3/§7.4/§11/§13`, `grammar §7-9`, `eval-model §8.3/§9`; решения D-1…D-13 зафиксированы и не переоткрываются | ✅ PASS |

**Вывод gate**: нарушений нет — **Complexity Tracking пуст**. Осознанные доопределения
(не нарушения): (1) **новый параметр `inStep bool`** протаскивается через `checkStmt`/`checkStmts`/
`checkElse` (расширение существующей сигнатуры с `inFunction`/`loopDepth` — тот же приём, что 003/004
для контекста), `checkExpr` **не трогается**; (2) **снятие deferred** у действий-в-шаге и
`запустить процесс` — сдвиг **семантической** стороны двусторонней deferred-метки на контекст-/
резолв-проверку, рантайм-сторона остаётся (§PM-5). Оба — аддитивны, листовость и ацикличность графа
сохранены.

## Guardrails — контракт реализации (нормативно для tasks/implement)

Фиксирует инварианты как контракт, чтобы код не дрейфовал от спеки и `docs/process-model.md`.
`/speckit-tasks` и `/speckit-implement` ОБЯЗАНЫ соблюдать каждый пункт. **Координаты — проверенные**
(сверены с фактическим `src/` 10.06.2026).

1. **Аддитивность 004.** Снять из `isUnexpectedTopLevel` (`parser/parse_stmt.go:37-45`) **только**
   `KW_PROCESS` — оставить `KW_WHEN`, `KW_VALUE`, `LBRACE`, `RBRACE`; обновить doc-комментарий
   (`процесс` теперь парсится). Добавить в `parseTopLevelItem` (`parse_stmt.go:12-31`) ветвь
   `if p.check(lexer.KW_PROCESS) { return p.parseProcessDecl() }` **перед** проверкой
   `isUnexpectedTopLevel` (рядом с `KW_FUNC`/`KW_SOURCE`/`KW_METRIC`, строки 13-21). Грамматика/
   приоритеты выражений и существующие узлы не меняются. *(D-6, FR-003/004/005, §PM-3)*
2. **AST-узлы (D-1/§PM-2).** Файл — новый `ast/process.go` (или дополнение `ast/decl.go`/`ast/step.go`
   — на усмотрение, формы фиксированы). `ProcessDecl{Name Ident; Params []Ident; Steps []*StepDecl}`
   встраивает `declBase` первым полем → автоматически `Decl`/`TopLevelItem` (`Pos()`=токен `процесс`),
   пополняет union верхнеуровневых деклараций. `StepDecl{Name Ident; After []Ident; Assignee
   Expression; Deadline Expression; Attrs StepAttrPos; Body []Statement}` встраивает **`base`**
   (НЕ `declBase`/`stmtBase` — шаг не top-level и не оператор) → реализует **только** `Pos()`=токен
   `шаг`. `StepAttrPos{AssigneePos Position; DeadlinePos Position}` — вспомогательная плоская структура
   (как `MetricAttrPos`, **НЕ** `Node`): нулевая `Position{}` ⟺ атрибут отсутствует. Конструкторы
   `NewProcessDecl(pos, name, params, steps)` / `NewStepDecl(pos, name, after, assignee, deadline,
   attrs, body)`. **Никаких** узлов `StepLine`/`StepAttr{Kind}` (D-1). *(§PM-2)*
3. **Синк union (моя зона).** `ast/node.go:24` — обновить doc-комментарий `Decl`: «В подмножестве B
   единственная — FunctionDecl» (устарел ещё на 004) → «union: FunctionDecl | SourceDecl | MetricDecl
   | ProcessDecl». *(§PM-2, FR-028)*
4. **Действия и запуск — переиспользуются (D-2/D-10).** `AssignAction`/`CallAction`/`NotifyAction`
   (`ast/step.go:8-42`) и `RunProcessExpr` (`ast/expr.go:69-78`) **уже построены** (003/004) — формы
   НЕ трогать. `parseStepAction` (`parse_stmt.go:86-107`) и `parseRunProcess` (`parse_expr.go:209-221`,
   диспетч из `parsePrimary:196-197`) **уже работают** — ничего нового. *(FR-009, §PM-2/§PM-3)*
5. **`parseProcessDecl` (новая, `parser/parse_decl.go`).** Зеркало `parseFunctionDecl`
   (`parse_decl.go:10-20`, заголовок) + `parseMetricDecl` (`parse_decl.go:107-161`, блок с backstop):
   `advance()` (`процесс`, `Pos`); `expect(IDENT, "имя процесса")`; **опц. параметры** — если
   `check(LPAREN)` → `advance`; `parseParamList()` (**переиспользовать**, `parse_decl.go:23-36`);
   `expect(RPAREN, ")")`; иначе `Params=nil`; `expect(COLON, ":")`; открыть блок через
   `openAttrBlock()` (`parse_decl.go:165-173` — `false`→`msgEmptyBlock`, вернуть `ProcessDecl` с
   пустыми `Steps`); цикл `!DEDENT && !EOF`: `check(KW_STEP)` → `parseStepDecl()` в `steps`, иначе →
   `p.error(peek().Pos, msgUnexpected(peek())); break` + backstop (`before:=p.pos`; не сдвинулись →
   `advance`); `expect(DEDENT, "конец блока")`. *(FR-001/005/007, §PM-3, §PM-6.A)*
6. **`parseStepDecl` (новая).** `advance()` (`шаг`, `Pos`); `expect(IDENT, "имя шага")`; **`после`** —
   если `check(KW_AFTER)` → `advance`; `parseAfterList()` в `After`; `expect(COLON, ":")`; открыть
   блок (`openAttrBlock`); цикл `!DEDENT && !EOF` диспетчер по ведущему токену строки:
   - `KW_ASSIGNEE`/`KW_DEADLINE` → **StepAttr**: `attrTok:=peek()`; **если `seen[lexeme]` →
     `p.error(attrTok.Pos, msgDuplicateAttr(lexeme)); break`** (D-8 — **БАЙТ-идентично**
     `parseMetricDecl:129-132`, **`break`, НЕ `continue`**); иначе `advance` (ключевое слово),
     `expect(COLON, ":")`, `parseExpression()` → в `Assignee`/`Deadline`,
     `Attrs.AssigneePos`/`DeadlinePos = toASTPos(attrTok.Pos)`, `expect(NEWLINE)`, `seen[lexeme]=true`.
   - иначе → `parseStatement()` (`parse_stmt.go:50-73`, сам диспетчеризует `присвоить`/`вызвать`/
     `уведомить` → `parseStepAction`, и `пусть`/`если`/`пока`/`для`/выражение-оператор) → добавить
     в `Body` если `!=nil`.
   - backstop прогресса (`before:=p.pos`; не сдвинулись → `advance`).
   `expect(DEDENT, "конец блока")`; вернуть `NewStepDecl(...)`. **«Неизвестного атрибута» в шаге
   НЕТ** (в отличие от `metric`): любая не-`исполнитель`/`срок` строка — `Statement`. *(FR-002/006,
   §PM-3, §PM-6.A)*
7. **`parseAfterList` (новый хелпер).** `StepAfter ::= "после" Ident ("," Ident)*` **без скобок**
   (отличие от `parseParamList`): цикл `expect(IDENT, "имя шага")` → если `!ok` `break`, добавить в
   `after`; `match(COMMA)` → продолжить, иначе стоп. `после` сразу с `:`/NEWLINE → первый
   `expect(IDENT)` даёт SE-EXPECTED, `After` пуст, восстановление штатное (негатив §PM-7). Логика —
   как `parseParamList`, но терминатор — не `RPAREN`, а отсутствие `COMMA`. *(FR-002, §PM-3)*
8. **Переиспользование текстов парсера (§PM-6.A).** `msgDuplicateAttr` (`parser/errors.go:29-31` —
   `«атрибут '<имя>' уже задан»`); `msgEmptyBlock` (`errors.go:17`); `msgUnexpected`
   (`errors.go:45-47`). **Ничего нового** в `parser/errors.go` не добавляется. *(FR-006/007, §PM-6.A)*
9. **Реестр процессов (§PM-4).** В `Interpreter` (`eval/interpreter.go:17-35`) добавить поле
   `processes map[string]*ast.ProcessDecl` (рядом с `sources`/`metrics`, ~строка 31); в
   `NewInterpreter` (`interpreter.go:53-73`) инициализировать `processes: make(map[string]*ast.ProcessDecl)`
   (~строка 67). *(§PM-4)*
10. **Регистрация процесса в Шаге 1 (§PM-4, D-5).** В `Analyze` (`analyze.go:16-96`) — **оба**
    type-switch'а Шага 1 (`analyze.go:29-38` и `analyze.go:49-62`): добавить `case *ast.ProcessDecl`.
    В первом — `name, pos = d.Name.Name, d.Name.Pos()` (НЕ `isFunc`; общий глобальный namespace,
    повтор → общий `'<имя>' уже объявлено в строке N`, как источник/метрика). Во втором —
    `i.processes[name]=d` + `i.checkReservedDeclName(name, d.Pos())` (`analyze.go:141-151` —
    переиспользуется; запрет столкновения со встроенной/периодом). *(FR-010, §PM-6.B,
    §PM-4)*
11. **Шаг 1c — `checkProcessDecl` (новый, после Шага 1b `checkMetricDecl` `analyze.go:68-76`).**
    Цикл по `prog.Items`: `pd, ok := item.(*ast.ProcessDecl)` → `i.checkProcessDecl(pd)`. Порядок
    внутри (fail-fast): **(1)** уникальность шагов — собрать `имя → строка`; повтор →
    `semErr(step.Name.Pos(), "шаг '<имя>' уже объявлен в строке N")`. **(2)** резолв `после` (D-4,
    валидатор): для шага `S` на индексе `i`, для каждого `X ∈ S.After`: `X` не среди шагов →
    `semErr(Xident.Pos(), "шаг '<S>' после '<X>', но шаг '<X>' не объявлен")`; `X` на индексе `j>=i`
    (позже/сам) → `semErr(Xident.Pos(), "шаг '<S>' после '<X>', но '<X>' объявлен позже")`. **(3)**
    `срок`-без-`исполнитель`: `Attrs.DeadlinePos.Line != 0 && Attrs.AssigneePos.Line == 0` →
    `semErr(step.Attrs.DeadlinePos, "шаг '<имя>': срок без исполнитель не имеет эффекта")`. **(4)**
    анализ тел шагов — `analyzeStep(step, pd.Params)`. *(FR-011/012/013, §PM-4, §PM-6.B)*
12. **`analyzeStep` (новый, D-12).** Зеркало `analyzeArea` (`analyze.go:156-167`), отличия: `vars`
    **засевается параметрами** (`pd.Params` → `vars[p.Name]=true`), но `letLine` параметрами **НЕ**
    засевается (`пусть x` с именем параметра в шаге разрешён, теняет, §6.4 — отличие от тела функции);
    `collectVars(step.Body, letLine={}, vars)` (`analyze.go:173-205` — переиспользуется, ловит дубль
    шаг-локальных `пусть`/`для`); `checkStmts(step.Body, vars, inFunction=false, inStep=true,
    loopDepth=0)`. *(FR-016, §PM-4)*
13. **Прокинуть `inStep bool` (§PM-4).** Добавить параметр `inStep` в сигнатуры `checkStmts`
    (`analyze.go:222`), `checkStmt` (`analyze.go:231`), `checkElse` (`analyze.go:281`) — все
    рекурсивные вызовы передают полученный `inStep` **без изменения**. Вызовы из `analyzeArea`
    (глобаль/функции, `analyze.go:166`) передают `inStep=false`; из `analyzeStep` — `inStep=true`.
    **`checkExpr` НЕ трогать** (он `inStep` не принимает — действия/`вернуть` ловятся на уровне
    `checkStmt`, а `запустить процесс` резолвится `checkRunProcess` независимо от контекста, см. п.16).
    *(§PM-4, риск (в))*
14. **Контекст-гард действий (D-11, заменить `analyze.go:275-276`).** `case *ast.AssignAction,
    *ast.CallAction, *ast.NotifyAction:` — заменить безусловный `return i.deferredConstruct(st)` на:
    `if !inStep { return semErr(st.Pos(), fmt.Sprintf("действие '%s' допустимо только в шаге процесса",
    constructName(st))) }; return nil`. `constructName` (`interpreter.go:150-164`) **уже** даёт
    `присвоить`/`вызвать`/`уведомить` — не трогать. В шаге payload (`Args`/`Value`) **НЕ** обходится
    (`return nil`; резолв/арность/deferred — рантайму, риск (д)); рантайм-deferred `stmt.go:64` —
    недостижим в 005. *(FR-014/022, §PM-4/§PM-5, §PM-6.B)*
15. **`вернуть` в шаге (§7.3, обновить `analyze.go:239-242`).** `case *ast.ReturnStmt:` — при
    `!inFunction` к базовому тексту добавить суффикс для шага:
    `msg := "'вернуть' допустимо только внутри функции"; if inStep { msg += "; в шаге процесса
    используйте 'присвоить'" }; return semErr(st.Pos(), msg)`. `прервать`/`продолжить` в шаге —
    работают как везде (по `loopDepth`; внутри `пока`/`для` шага валидны), не трогать. *(FR-015,
    §PM-6.B; финальный текст = `'вернуть' допустимо только внутри функции; в шаге процесса используйте
    'присвоить'`)*
16. **Арность `запустить процесс` (D-10, заменить `analyze.go:330-331`).** `case *ast.RunProcessExpr:`
    в `checkExpr` — заменить `return i.deferredConstruct(ex)` на `return i.checkRunProcess(ex, vars)`.
    Новый метод `checkRunProcess(r *ast.RunProcessExpr, vars map[string]bool) error`: сначала
    `checkExpr` каждого `r.Args` (args-first, fail-fast); `name := r.Process.Name`; резолв **ТОЛЬКО**
    против `i.processes` (НЕ `vars`/`funcs`/`builtins` для имени-как-вызова, риск (г)): если
    `pd, ok := i.processes[name]` → арность `len(r.Args) != len(pd.Params)` →
    `semErr(r.Pos(), "'<P>' принимает N аргументов, передано M")`, иначе `nil`; иначе если
    `i.funcs[name]` → `semErr(r.Pos(), "'<P>' — функция, не процесс")`; иначе если
    `i.metrics[name]`/`i.sources[name]` → `semErr(r.Pos(), "'<P>' — не процесс")`; иначе →
    `semErr(r.Pos(), "процесс '<P>' не объявлен")`. **`i.builtins` НЕ проверяется** (имя встроенной
    после `запустить процесс` падает в общий `процесс '<P>' не объявлен` — осознанно, §PM-4).
    `checkRunProcess` **не** принимает `inStep` (работает в любой области — реестр готов с Шага 1).
    `DurationLit` (`analyze.go:332-333`) **НЕ трогать**. *(FR-018, §PM-4, §PM-6.C)*
17. **Граница deferred — eval НЕ трогать (§PM-5, критично).** `stmt.go:63-64`
    (`AssignAction`/`CallAction`/`NotifyAction` → `deferredConstruct`) и `expr.go:48-51`
    (`RunProcessExpr`/`DurationLit` → `deferredConstruct`) — **уже корректны для 005, НЕ трогать**.
    `deferredConstruct`/`constructName` (`interpreter.go:146-164`) — механизм без изменений
    (`constructName` уже покрывает все 5 узлов). Наблюдаемая рантайм-граница 005 = **только**
    top-level `запустить процесс`. **Недостижимый рантайм-тест «действие в шаге → deferred» НЕ
    писать** (риск (б)). *(FR-021/022/023/025, §PM-5)*
18. **Демо `онбординг.ladix` (D-9/§PM-7).** Убрать последнюю строку
    `печать("статус:", статус_процесса(id))` (`examples/онбординг.ladix:16`) — `статус_процесса`
    остаётся deferred-builtin (D-9), её вызов сделал бы семантику не чистой. Целевой файл — §PM-7
    (3 шага, top-level `пусть id = запустить процесс онбординг("Петров")` + `печать`). Демо: парс
    чисто + семантика чисто, `ladix run` → код 1 в рантайме на `запустить процесс`. *(FR-024, §PM-7,
    SC-004)*
19. **Регрессы парсера (FR-027, §PM-3).** `parser/examples_test.go::TestDeclarativeExamplesUnexpected`
    (строки 35-54): кейс `выручка.ladix` — обновить ожидание `«неожиданный токен 'процесс'»` →
    **`«неожиданный токен 'когда'»`** (процесс теперь парсится, падение позже на триггере 007). Кейс
    `онбординг.ladix` — **снять** из этого набора (он парсится целиком) и перенести в позитивные
    parse-тесты (парс с **нулём** ошибок) — например, в `TestExamplesParseCleanSet` (строки 12-33)
    или отдельным позитивным кейсом. *(FR-027, §PM-3, SC-004/005)*
20. **Тексты ошибок — байт-точно (§PM-6).** Новый слой — ДОСЛОВНО из §PM-6.A (парсер, переиспользуется),
    §PM-6.B (семпроход процесса/шага), §PM-6.C (запуск). Соглашения: payload без завершающей точки;
    `'…'` для идентификаторов/ключевых слов; `N`/`M` с 1. Exact-match тесты сверяют байт-в-байт
    (SC-001/002/003). *(FR-019, §PM-6, конституция VIII)*
21. **Синк доков (моя зона, §PM-0.6/FR-028).** `examples/MANIFEST.md` — **расщепить** совмещённую
    строку (`MANIFEST.md:54-79`, особенно :58-60 и :18-19): (а) `онбординг.ladix` теперь **парсится и
    проходит семантику чисто**, `ladix run` → код 1 в **рантайме** на `запустить процесс` (исполнение
    — 006); (б) `выручка.ladix` **остаётся** парс-ошибкой код 1, токен сдвигается
    `источник`/`процесс` → **`когда`** (триггеры — 007). `ast/node.go:24` — doc-комментарий union
    (п.3). `SPEC §13.4` (+ тексты §PM-6.B/C), `§7.3`/`§7.4` (тексты), `§11.4`/`§12` (термин),
    `ARCHITECTURE §4.4/§4.8` (формы — по якорю **уже выполнено**) — синк под §PM-6. *(FR-028, §PM-7)*

## Project Structure

### Documentation (this feature)

```text
specs/005-process-frontend/
├── plan.md              # Этот файл (/speckit-plan)
├── research.md          # Phase 0 — решения D-1..D-13 + переход парсера/семпрохода (/speckit-plan)
├── data-model.md        # Phase 1 — сущности ProcessDecl/StepDecl/StepAttrPos/Действие/Запуск/Параметр (/speckit-plan)
├── quickstart.md        # Phase 1 — как реализовывать и проверять (/speckit-plan)
├── contracts/           # Phase 1 — контракты (/speckit-plan)
│   ├── ast-process.md       # формы ProcessDecl/StepDecl/StepAttrPos + конструкторы (§PM-2)
│   ├── analyze-process.md   # Шаг 1c checkProcessDecl + analyzeStep + checkRunProcess + inStep (§PM-4)
│   ├── diagnostics-process.md # байт-точный реестр текстов §PM-6.A/B/C + граница deferred (§PM-5/§PM-6)
│   └── cli-process.md       # приёмка ladix run / регрессы парсера / поведение демо (§PM-3/§PM-5/§PM-7)
├── checklists/          # уже существует (фаза specify)
├── spec.md              # Спецификация (готова)
└── tasks.md             # Phase 2 (/speckit-tasks — НЕ создаётся этим планом)
```

### Source Code (repository root)

Корень Go-модуля — `src/`. Все `go`-команды от `src/`. Существующие пакеты правятся **аддитивно**;
новых пакетов нет (`internal/engine`/`internal/store` НЕ заводятся, D-7/§PM-8).

```text
src/
├── go.mod                          # github.com/denis-kosyakov/ladix, go 1.22 (без изменений)
├── cmd/
│   └── ladix/
│       └── main.go                 # БЕЗ изменений (нет ladix check, D-7; run/metric как есть, FR-026)
├── internal/
│   ├── lexer/                      # БЕЗ изменений — KW_PROCESS/KW_STEP/KW_AFTER/KW_ASSIGNEE/
│   │                               #   KW_DEADLINE уже в keywords.go:20-21 + token.go:49-56
│   ├── ast/                        # РАСШИРЯЕТСЯ (аддитивно, остаётся листовым)
│   │   ├── process.go              #   НОВЫЙ — ProcessDecl/StepDecl/StepAttrPos + конструкторы (§PM-2)
│   │   │                           #   (либо дополнение decl.go/step.go — формы фиксированы)
│   │   ├── process_test.go         #   НОВЫЙ — TestProcessDeclPos/TestStepDeclPos (var _ Decl/Node)
│   │   ├── step.go                 #   БЕЗ изменений (AssignAction/CallAction/NotifyAction готовы, D-2)
│   │   ├── expr.go                 #   БЕЗ изменений (RunProcessExpr готов, D-10)
│   │   └── node.go                 #   ИЗМЕНЯЕТСЯ — doc-комментарий Decl (union из 4, строка 24)
│   ├── parser/                     # РАСШИРЯЕТСЯ (ручной recursive descent, конституция II)
│   │   ├── parse_stmt.go           #   isUnexpectedTopLevel: убрать ТОЛЬКО KW_PROCESS (37-45);
│   │   │                           #   parseTopLevelItem: ветвь parseProcessDecl (12-31);
│   │   │                           #   parseStepAction (86-107) БЕЗ изменений
│   │   ├── parse_decl.go           #   +parseProcessDecl/parseStepDecl/parseAfterList
│   │   ├── parse_expr.go           #   БЕЗ изменений (parseRunProcess готов, 209-221)
│   │   ├── errors.go               #   БЕЗ изменений (msgDuplicateAttr/msgEmptyBlock/msgUnexpected готовы)
│   │   ├── parse_decl_test.go      #   +позитивные/негативные parse-тесты ProcessDecl/StepDecl
│   │   └── examples_test.go        #   РЕГРЕСС: выручка→'когда'; онбординг→позитив (FR-027)
│   ├── errors/                     # БЕЗ изменений (D-3 — errors.ОшибкаПроцесса это 006)
│   ├── value/                      # БЕЗ изменений (фронтенд ничего не вычисляет)
│   └── eval/                       # РАСШИРЯЕТСЯ (→ ast; НЕ engine/store)
│       ├── interpreter.go          #   +поле processes + init в NewInterpreter (17-35, 53-73);
│       │                           #   deferredConstruct/constructName (146-164) БЕЗ изменений
│       ├── analyze.go              #   Шаг1: case *ast.ProcessDecl (оба switch'а); +Шаг1c
│       │                           #   checkProcessDecl/analyzeStep/checkRunProcess; inStep через
│       │                           #   checkStmt/checkStmts/checkElse; контекст-гард действий;
│       │                           #   вернуть-в-шаге; арность запустить (заменить deferred)
│       ├── analyze_decl_test.go    #   +exact-match §PM-6.B/C (уник шагов, после, срок-без-исп,
│       │                           #   действие-вне-шага, вернуть-в-шаге, арность запустить)
│       ├── stmt.go                 #   БЕЗ изменений (63-64 рантайм-deferred остаётся, §PM-5)
│       └── expr.go                 #   БЕЗ изменений (48-51 рантайм-deferred остаётся, §PM-5)
└── (конец дерева src/)
```

`examples/` — на **корне репозитория**, СИБЛИНГ `src/` (а не его потомок); тесты читают их через
`filepath.Join("..","..","..","examples",name)` — три уровня вверх от `src/internal/parser` до корня:

```text
examples/                            # корень репозитория (рядом с src/, не внутри)
├── онбординг.ladix                 #   ИЗМЕНЯЕТСЯ — убрать строку статус_процесса (D-9/§PM-7)
├── выручка.ladix                   #   БЕЗ изменений (целиком не парсится — падает на 'когда', §PM-3)
└── MANIFEST.md                     #   ИЗМЕНЯЕТСЯ — расщепить демо-строки онбординг/выручка (FR-028)
```

**Structure Decision**:

- **Без новых пакетов** (D-7/§PM-8): фронтенд процессов целиком ложится в `ast` (узлы), `parser`
  (разбор), `eval/analyze.go` (валидация); `internal/engine`/`internal/store`/lifecycle/CLI
  `start`/`complete`/… НЕ заводятся — это движок 006. Граф минимален, мёртвой абстракции нет.
- **Аддитивное расширение `ast`/`parser`/`eval`**: новые узлы/функции добавляются рядом с
  существующими по «домашнему» паттерну (`declBase`-встраивание для `ProcessDecl`, `base` для
  `StepDecl`; `parseFunctionDecl`-заголовок + `parseMetricDecl`-блок; `checkMetricDecl`-образец для
  `checkProcessDecl`; `analyzeArea`-образец для `analyzeStep`; `checkCall`-образец для
  `checkRunProcess`).
- **`ast` остаётся листовым**: `ProcessDecl`/`StepDecl`/`StepAttrPos` используют только
  `ast`-локальную `Position` и существующие `ast`-типы (`Ident`/`Expression`/`Statement`) — `ast`
  не узнаёт об `errors`/`value`/`eval`.
- **`errors`/`value`/`lexer`/`cmd` не меняются**: новых типов ошибок нет (D-3); фронтенд ничего не
  вычисляет; токены процесса/шага уже в лексере; CLI без новых команд (D-7).
- **Тесты co-located** (`*_test.go`) — конвенция 001/002/003/004.

## Phasing — порядок поставки по приоритетам User Stories

Порядок следует приоритетам spec (все 3 истории — **P1**) с фундаментом-фазой A (AST) впереди.
Каждая фаза завершается зелёными гейтами (`go build`/`go vet`/`gofmt`/`go test -race`).

- **Фаза A (фундамент: AST + синк union).** 🎯 `ProcessDecl`/`StepDecl`/`StepAttrPos` + конструкторы
  (`ast/process.go`, §PM-2/Guardrail 2); `ProcessDecl` встраивает `declBase` (→ `Decl`/`TopLevelItem`),
  `StepDecl` — `base` (→ только `Pos()`); doc-комментарий `ast/node.go:24` (union из 4, Guardrail 3);
  AST-тесты по образцу `decl_test.go`/`step_test.go` (`var _ Decl = pd`, `var _ Node = sd`, поля,
  `Pos()`). **Гейт:** AST-тесты зелёные; `go build`/`go vet`/`gofmt` чисто; узлы реализуют нужные
  маркеры. *(блокирует B и C; FR-008/009)*
- **Фаза B — US1/US2 часть 1 (парсер + регресс).** 🎯 снять из `isUnexpectedTopLevel` **только**
  `KW_PROCESS` + ветвь `parseProcessDecl` в `parseTopLevelItem` (Guardrail 1); `parseProcessDecl`/
  `parseStepDecl`/`parseAfterList` (Guardrail 5-7); дубль атрибута = `p.error+break`
  (БАЙТ-идентично `parseMetricDecl`, D-8); переиспользовать `parseParamList`/`openAttrBlock`/
  `msgDuplicateAttr`/`msgEmptyBlock`/`msgUnexpected`; регресс `examples_test.go` (выручка→'когда',
  онбординг→позитив, Guardrail 19). **Гейт:** `онбординг.ladix` парсится с нулём ошибок (SC-004
  частично); дубль атрибута/пустой блок/не-шаг в блоке → exact-match §PM-6.A (SC-001); табличные
  кейсы `ProcessDecl`/`StepDecl`/`StepAttrPos` (параметры опц.; `после` 0/1/N; чередование
  attr/statement) дают канонические формы (SC-006); все parse-тесты 002/004 зелёные. *(US1-P1/US2-P1)*
- **Фаза C — US1/US2/US3 часть 2 (семпроход + граница deferred).** 🎯 поле `i.processes` + init
  (Guardrail 9); Шаг 1 регистрация процесса в обоих switch'ах (Guardrail 10); Шаг 1c
  `checkProcessDecl` (уник шагов → резолв `после` → `срок`-без-`исполнитель` → `analyzeStep`,
  Guardrail 11-12); прокинуть `inStep` через `checkStmt`/`checkStmts`/`checkElse` (`checkExpr` не
  трогать, Guardrail 13); контекст-гард действий (заменить deferred, Guardrail 14); `вернуть` в шаге
  (Guardrail 15); `checkRunProcess` арность (заменить deferred, резолв только против процессов,
  Guardrail 16); eval **не трогать** (`stmt.go`/`expr.go` рантайм-deferred остаётся, Guardrail 17).
  **Гейт:** уник шагов / `после` вперёд/неизвестный / `срок`-без-`исполнитель` / действие-вне-шага /
  `вернуть`-в-шаге / арность `запустить процесс` (все 4 текста) → exact-match §PM-6.B/C (SC-002/003);
  `онбординг.ladix` проходит семантику чисто (SC-004); действие в шаге + чтение параметра в шаге +
  `после A` назад → семантика чиста (US2 acceptance); `запустить процесс P("…")` арность 1==1 →
  чисто (US3 acceptance). *(US1-P1/US2-P1/US3-P1)*
- **Фаза D — демо + синк-доки + сквозной CLI.** Подрезать `examples/онбординг.ladix`
  (убрать `статус_процесса`, Guardrail 18); расщепить `examples/MANIFEST.md` (онбординг→рантайм/
  выручка→'когда', Guardrail 21); синк зоны якоря (`SPEC §13.4/§7.3/§7.4/§11.4/§12`,
  `ARCHITECTURE §4.4/§4.8` — по якорю уже выполнено). **Гейт:** `ladix run онбординг.ladix` → код 1,
  рантайм-текст `конструкция запустить процесс не поддерживается в этой версии` (SC-004);
  `ladix run выручка.ladix` → код 1, парс-ошибка `неожиданный токен 'когда'` (SC-005); все
  регресс-тесты 001/002/003/004 зелёные; `go vet`/`gofmt`/`-race` чисто (SC-007). *(US3-P1 CLI-граница)*

**Зависимости фаз**: A блокирует всё (узлы для парсера и семпрохода). B зависит от A (узлы для
разбора). C зависит от A+B (узлы + разобранный AST для валидации). D зависит от C (семантика чиста
→ CLI-граница на рантайме). Внутри фаз — tests-first (конституция VI): тест-кейс с exact-match
текстом §PM-6 пишется до/вместе с кодом.

## Complexity Tracking

> Нарушений конституции нет — таблица пуста.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |

Осознанные доопределения (НЕ нарушения, раскрыты в [research.md](./research.md)): (1) **параметр
`inStep bool`** через `checkStmt`/`checkStmts`/`checkElse` — расширение существующей контекст-сигнатуры
(`inFunction`/`loopDepth`), тот же приём 003/004; `checkExpr` не трогается; (2) **снятие
семантической стороны deferred** у действий-в-шаге (`analyze.go:275-276`) и `запустить процесс`
(`analyze.go:330-331`) — рантайм-сторона (`stmt.go:64`/`expr.go:49`) остаётся (§PM-5). Оба —
аддитивны, листовость и ацикличность графа сохранены.

## Риски и точки внимания

- **Дубль атрибута = `p.error+break`, НЕ `continue` (D-8, риск (а)).** Плоский AST (одно
  `Assignee`/`Deadline`) вынуждает ловить дубль при разборе. Восстановление — **строго как**
  `parseMetricDecl:129-132`: `p.error(attrTok.Pos, msgDuplicateAttr(lexeme)); break`. `p.error`
  синхронизируется до конца строки и ставит `suppress`; ручной `continue`+«дочитать `: Expr NEWLINE`»
  рассинхронил бы парсер. Остаток блока шага восстанавливается штатным synchronize.
- **Тело шага в рантайме НЕ исполняется (риск (б)).** `ProcessDecl` — `Decl`, не `Statement`; `Run()`
  (`interpreter.go:84-90`) пропускает не-`Statement`. Ветка `stmt.go:64` (действия → deferred)
  существует, но **недостижима** до движка 006. **НЕ писать** рантайм-тест «действие в шаге →
  deferred» — он недостижим. Единственная наблюдаемая рантайм-граница = top-level `запустить процесс`.
- **`inStep` через checkStmt/checkStmts/checkElse, `checkExpr` не трогать (риск (в)).** Действия и
  `вернуть` ловятся на уровне `checkStmt` (где `inStep`/`inFunction` доступны). `checkRunProcess`
  **не** принимает `inStep` — резолв процесса/арности не зависит от контекста (реестр готов с Шага 1).
  Если по ошибке протащить `inStep` в `checkExpr` — сломается арность вложенных вызовов вне шага.
- **`checkRunProcess` резолвит против `i.processes`, не `vars`/`builtins` (риск (г)).** В отличие от
  `checkCall` (`analyze.go:341-366`, проверяет `vars`/`funcs`/`builtins`), `checkRunProcess` резолвит
  имя **только** против реестра процессов (синтаксис `запустить процесс Ident`, D-10). Имя встроенной
  после `запустить процесс` падает в общий `процесс '<P>' не объявлен` (ветку `builtins` не
  добавлять). Аргументы (`r.Args`) обходятся `checkExpr` (args-first, fail-fast — как `checkCall`).
- **Action-payload не обходится семпроходом (риск (д)).** В контекст-гарде действия в шаге →
  `return nil` (payload `Args`/`Value` НЕ валидируется): резолв/арность/deferred аргументов — рантайму,
  как declaredness (D-11). Это **отличие** от `checkCall`, который обходит аргументы. `срок: 2дн`
  проходит, т.к. атрибуты шага семпроходом не обходятся (D-11) — `DurationLit` в `срок:` не достигает
  deferred-проверки (§PM-5, известная граница).
- **Регресс `examples_test.go` — два кейса (FR-027).** `выручка.ladix`: ожидание сдвигается
  `'процесс'` → `'когда'` (процесс теперь парсится). `онбординг.ladix`: **снять** из «unexpected»-
  набора (`TestDeclarativeExamplesUnexpected:41`) и перенести в позитивный
  (`TestExamplesParseCleanSet:13-18` или отдельный). Пропуск любого → красный тест (страховка).
- **`онбординг.ladix` должен пройти семантику ЧИСТО (D-9/§PM-7).** Строка
  `печать("статус:", статус_процесса(id))` (`онбординг.ladix:16`) **обязана** быть убрана — иначе
  deferred-builtin `статус_процесса` (D-9) даёт семантическую ошибку, и SC-004 («семантика чисто»)
  не выполнится. Запуск (`запустить процесс`) остаётся — он и даёт рантайм-границу.
- **`срок`-без-`исполнитель` — позиция на `срок:`, не на начале шага.** `StepAttrPos.DeadlinePos`
  несёт позицию ключевого слова `срок` (как `MetricAttrPos`); диагностика указывает на строку
  `срок:` (§PM-6.B, SC-002). Если `StepAttrPos` не заполнен парсером — позиция будет нулевой
  (страховка: парсер выставляет `Line!=0` ровно для присутствующих атрибутов, как `parseMetricDecl`).

## Открытые вопросы и отложенные расхождения

- **Приоритет текстов**: новый слой процессов — §PM-6.A/B/C (exact-match канон); deferred-тексты eval
  (`конструкция <X> …`, `функция '<имя>' …`) — `eval-model §8.3` (уже там); при расхождении побеждает
  якорь `docs/process-model.md` (§PM преамбула).
- **DRIFT якорь vs код (план следует коду):** §PM-3 называет «убрать KW_PROCESS из множества
  отвергаемых» — факт: `parse_stmt.go:39-40` всё ещё содержит `KW_PROCESS` рядом с
  `KW_WHEN`/`KW_VALUE`/`LBRACE`/`RBRACE` (убрать только его); `parseProcessDecl`/`parseStepDecl`/
  `ProcessDecl`/`StepDecl`/`StepAttrPos`/`i.processes`/`checkProcessDecl`/`inStep` — **отсутствуют**
  (создаются 005); `ast/node.go:24` doc всё ещё «единственная — FunctionDecl» (pre-004, обновить).
  Координаты в Guardrails — фактические.
- **`выручка.ladix`** в 005 целиком НЕ парсится (содержит `процесс` **и** `когда`) — это **не** баг
  005: процесс теперь парсится, падение сдвигается на `когда` (триггеры — 007, §PM-3). Не оцениваемый
  чеклист (демо, не golden).
- **Триггеры (`когда`/`значение`/`событие`/`расписание`), `internal/engine`/`store`, lifecycle,
  процессные builtins, рантайм-scope процесс-переменных, `Длительность`-конструктор, CLI `check`,
  топосорт `после`, вложенные процессы, дубль параметра процесса (D-13)** — вне scope 005
  (§PM-0/§PM-8); их ведущие токены/встроенные остаются отвергаемыми/deferred/непроверяемыми.

_Открытых `[NEEDS CLARIFICATION]` нет: все поведенческие решения зафиксированы в
`docs/process-model.md §PM-0…§PM-8` (единственный связывающий источник), решения D-1…D-13 приняты и
не переоткрываются. Любое доопределение фиксируется явно (конституция IX)._

---

**Следующий шаг — `/speckit-tasks`**: разложить фазы A–D на детальные, dependency-ordered задачи
(tests-first, с привязкой к Guardrail N, FR-NNN и user stories) в `specs/005-process-frontend/tasks.md`.
