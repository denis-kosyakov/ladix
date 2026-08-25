# ARCHITECTURE.md — архитектура реализации Ladix

> **Разделение ответственности.** `SPEC.md` описывает **язык** (что значит каждая конструкция). Этот документ описывает **реализацию** (как код организован в пакеты, как данные текут по пайплайну, какие контракты между слоями). Здесь нет новой семантики — только структура Go-кода и связки контрактов, уже зафиксированных в `SPEC.md`, `docs/execution-model.md`, `README` (раздел CLI).
>
> **Ссылки.** `SPEC §13` — контракт ошибок и диагностики; `README, раздел CLI` — полный CLI-справочник; `docs/execution-model.md` — детальная модель исполнения процессов; `docs/engine-model.md` — связывающий якорь фичи 006 (контракт `Store` v006, пакет `engine`, экспорт `eval`/`ProcessRuntime`, CLI — канон при расхождении); `docs/grammar.md` — полный EBNF; `docs/stdlib.md` — полный реестр стандартной библиотеки.

---

## 1. Обзор

Ladix реализован как **tree-walking интерпретатор** со встроенным **движком процессов**. Никакой компиляции в байткод или нативный код: парсер строит AST, интерпретатор обходит дерево и вычисляет значения напрямую. Это самый простой для чтения и отладки подход, адекватный масштабу DSL.

Архитектурный принцип: **фронтенд языка — собственный навсегда, бэкенд исполнения — модульный**. Лексер, парсер, AST, семантика и интерпретатор выражений написаны вручную и не подлежат замене. Движок процессов работает поверх интерфейса `Store`, что оставляет путь отступления — заменить встроенный движок/хранилище на Camunda/Kestra или другой бэкенд, не трогая синтаксис и UX языка (см. §9).

**Конструкции верхнего уровня обрабатываются по-разному** (`SPEC §3`):
- **Определения** (`функция`, `источник`, `метрика`, `процесс`, `когда`) — регистрируются в глобальном пространстве, не исполняются в момент объявления.
- **Statements** (`пусть`, присваивание, `если`, `пока`, `для`, вызов, `печать`) — исполняются немедленно, сверху вниз.

Сборка — **один статический бинарник `ladix`** через `go build`, **без CGO** (`modernc.org/sqlite` — чистый Go), что даёт кросс-компиляцию под Windows/Mac/Linux одной командой.

---

## 2. Раскладка пакетов

Стандартная Go-раскладка. Go-модуль живёт **в корне репозитория** (`go.mod` в корне, module-path `github.com/denis-kosyakov/ladix` без сегмента `/src` — фича 029); пути ниже даны относительно корня.

> **Граница «публичный контракт ↔ внутренняя реализация» (фичи 029, 030).** Публичная поверхность модуля — **РОВНО три пакета вне `internal/`**: корневой `ladix` (узкий фасад `Compile`/`CompileFile`), `ir` (версионируемые типы результата со `SchemaVersion`) и `metrics` (публичный исполнитель metrics-подмножества IR над данными потребителя, фича 030, `docs/module-contract.md` §MC-8). Всё остальное, включая фронтенд `lexer`/`parser`/`ast`/`value`/`errors`, остаётся под `internal/` и частью semver-контракта НЕ является. Инвариант: публичному замыканию (`ladix`, `ir`, `metrics`) ЗАПРЕЩЕНО зависеть — прямо или транзитивно — от `internal/{store,engine,daemon}` и от `modernc.org/sqlite`; `internal/eval` явно исключён из запрета (`ladix` зовёт его `Analyze`, `metrics` — конвейер вычисления §SM-8). Инвариант закреплён тест-стражем `boundary_test.go` через `go list -deps`. Расширение поверхности — только аддитивно.

> **Десять внутренних пакетов + три публичных.** Технические рамки перечисляют семь пакетов; этот документ добавляет листовой пакет `internal/value/` (вариант A, §2.1) — `Value` делят три пакета (`eval`, `store`, `engine`), и без нейтрального места его пришлось бы объявлять в `eval`, втягивая `store`/`engine` в зависимость от интерпретатора (вариант B, отвергнут в §2.1). Фичи серии 007/v2 добавили ещё два: `internal/daemon/` (ядро демона `serve`) и `internal/jsonval/` (нейтральный кодек JSON↔`Value`). Итого десять внутренних пакетов + `main` + три публичных (`ladix`, `ir`, `metrics`).

| Пакет | Путь | Ответственность |
|---|---|---|
| `ladix` | `./` (корень) | **ПУБЛИЧНЫЙ.** Узкий фасад библиотеки: `Compile(source)` / `CompileFile(path)` → `*ir.Program` + `[]ir.Diagnostic`. Три стадии статической валидации (лексика → синтаксис → `eval.Analyze`) БЕЗ исполнения; понижение AST → IR каноническими строками (`lower.go`); конвертер ошибок фронтенда в диагностики (`diagnostics.go`); recover-барьер. Состояния между вызовами не держит. |
| `ir` | `ir/` | **ПУБЛИЧНЫЙ, листовой.** Версионируемый контракт вывода: `SchemaVersion`, `Program`/`Metric`/`Process`/`Step`/`Trigger`, `Diagnostic`, собственная `Position`. JSON-теги `snake_case` без `omitempty`. Не импортирует ни `ast`, ни `errors`, ни `value` (§4.1). |
| `metrics` | `metrics/` | **ПУБЛИЧНЫЙ (фича 030).** Исполнитель metrics-подмножества IR: `Evaluate(ir.Metric, []map[string]any, Options) (Result, []ir.Diagnostic, error)` — разбирает канонические строки `Where`/`Aggregate`/`Period`/`ByDate` синтетической декларацией метрики через `internal/lexer`/`internal/parser`, прогоняет `internal/eval.Analyze`, затем конвейер §SM-8 над записями потребителя. **Вправе** зависеть от `internal/{lexer,parser,ast,eval,value,errors}`; **НЕ вправе** — от `internal/{store,engine,daemon}` и `modernc.org/sqlite` (страж T3, `boundary_test.go`). |
| `main` | `cmd/ladix/` | Точка входа; ручной диспетчер подкоманд (`realMain`: ветвление по `args[0]`, разбор флагов — ручной цикл, без stdlib `flag`); recover-барьер; коды возврата 0/1/2 (`README, раздел CLI`). |
| `errors` | `internal/errors/` | Категории ошибок Ladix как Go error-типы с полями `Line`, `Col`, `Msg`; сентинелы; форматирование пользовательского сообщения (`SPEC §13`). Листовой пакет — от него зависят все. |
| `value` | `internal/value/` | Интерфейс `Value` и 10 конкретных типов-значений Ladix (§5). Листовой пакет (зависит только от stdlib), общий для `eval`/`store`/`engine` — устраняет цикл `eval ↔ store`. |
| `lexer` | `internal/lexer/` | Токенизация; стек уровней отступа → виртуальные токены `Indent`/`Dedent`/`Newline`/`EOF`; подавление `Newline` внутри парных скобок. |
| `ast` | `internal/ast/` | Узлы AST (§4); каждый узел несёт позицию `(Line, Col)`. Чистые типы данных, без поведения интерпретатора. Листовой (`Position` — локальная структура, §4.1). |
| `parser` | `internal/parser/` | Ручной recursive descent: токены → AST; panic-mode error recovery (синхронизация по `Newline`/`Dedent`/top-level-ключевым словам; лимит ≈20 сообщений). |
| `eval` | `internal/eval/` | Семантический анализ (резолв имён, арность, scope-гарды) + интерпретатор выражений/statements; таблица символов и scope; стдлиб-функции. |
| `engine` | `internal/engine/` | Движок процессов как библиотека над `Store`: `Start`/`advance`/`Complete`; реализует `eval.ProcessRuntime` (D-1, §2.1); engine-Clock (D-2). Webhook-вызовы `вызвать`/`уведомить` (`caller.go`) кодируют тело через `jsonval`. Не сервис, без глобалов; планировщик `serve` живёт в `daemon`, не здесь. |
| `daemon` | `internal/daemon/` | Ядро демона `serve` (фича 007): одна горутина-тикер (`time.NewTicker`) под `context.Context`; 4-фазный детерминированный тик `drainEvents → evalMetrics → checkSchedules → checkDeadlines` (§7.5). Над `Store`/`engine`/`eval`-интерпретатором; durable `trigger_state`, изоляция сбоев на триггер. |
| `jsonval` | `internal/jsonval/` | Нейтральный кодек «JSON ↔ `value.Value`» (`decode.go`/`encode.go`); plain-JSON (нетегированный, в отличие от type-tagged кодека `store`). Используется `engine` (webhook-payload) и `daemon` (payload событий → `Запись`). |
| `store` | `internal/store/` | Интерфейс `Store` (§6) + сентинелы; `memory.go` (`MemoryStore`); `sqlite.go` (`SQLiteStore`, `modernc.org/sqlite`, WAL, без CGO); кодек type-tagged JSON (внутреннее дело `SQLiteStore`). |

### 2.1. Граф зависимостей (без циклов)

Три листовых пакета внизу (`errors`, `value`, `jsonval`) плюс почти-листовой `ast` и публичный лист `ir` (все зависят только от stdlib); `main` — корень CLI, импортирует всё; публичный `ladix` — корень библиотеки. Рёбра (читать «X → Y» как «X импортирует Y»):

- `errors` → ∅ (листовой; стандартная Go-идиома «базовый пакет ошибок внизу»).
- `value` → ∅ (листовой; зависит только от stdlib).
- `jsonval` → `value` (кодек оперирует `Value`; больше ничего внутреннего не тянет).
- `ast` → ∅ (`Position` — локальная структура в `ast`, см. §4.1; пакет полностью листовой).
- `lexer` → `errors` (выдаёт `LexError`).
- `parser` → `lexer`, `ast`, `errors`.
- `eval` → `ast`, `value`, `errors` (интерпретатор читает AST, оперирует `Value`). **eval НЕ импортирует ни `store`, ни `engine`** — процессные builtins и действия шагов ходят через интерфейс `ProcessRuntime`, объявленный в самом `eval` (разрыв цикла, см. ниже).
- `engine` → `eval`, `store`, `ast`, `value`, `jsonval`, `errors` (движок реализует `eval.ProcessRuntime`, исполняет тела шагов через экспортируемую поверхность `eval`, резолвит `ProcessDecl`/`StepDecl` из `ast`, оперирует `Value`, кодирует webhook-тело через `jsonval`, персистит через `store`).
- `store` → `value` (контракт оперирует `Value`; сентинелы `ErrInstanceNotFound`/`ErrTaskNotFound`/`ErrTaskAlreadyCompleted` объявлены в самом `store` через stdlib `errors.New` — `docs/engine-model.md §EN-2`; ребра `store → internal/errors` нет).
- `daemon` → `engine`, `eval`, `store`, `ast`, `value`, `jsonval` (демон `serve`: крутит тик над движком/интерпретатором, резолвит триггеры из `ast`, читает/пишет `Store`, декодирует payload событий через `jsonval`). Планировщик/тикер `serve` живёт здесь, не в `engine`.
- `ir` → ∅ (публичный лист; собственная `Position`, см. §4.1).
- `ladix` → `lexer`, `parser`, `ast`, `errors`, `eval`, `ir` (публичный фасад). **`ladix` НЕ импортирует ни `store`, ни `engine`, ни `daemon`** — граница, машинно проверяемая `boundary_test.go`: потребитель библиотеки не тянет SQLite.
- `metrics` → `lexer`, `parser`, `ast`, `errors`, `eval`, `value`, `ir` (публичный исполнитель, фича 030). Тот же набор зависимостей, что у `ladix`, плюс `value` (типизация записей потребителя, Д-8 дельты `metrics-evaluator`). **`metrics` НЕ импортирует ни `store`, ни `engine`, ни `daemon`** — тот же страж `boundary_test.go` (T3).
- `main` → всё.

Текстовая схема уровней (стрелка вверх = «зависит от»):

```
      ladix (ПУБЛИЧНЫЙ фасад)          main (CLI)
        │  → ir, parser, eval           │  (импортирует всё)
        │     ✗ store/engine/daemon     │
   ┌────┴───┬─────────┬──────────┬──────┴───┐
 parser    eval     engine     daemon      store
   │        │          │          │          │
 lexer   ast,value  eval,store engine,eval value
   │     errors      ast,value  store,value
 errors             jsonval     jsonval
                    errors
   └──── errors / value / jsonval / ast / ir (листовые) ────┘
```

**Развязка `Value`.** `eval`, `store`, `engine` оперируют `Value` (§5). `store` не должен импортировать `eval` (хранилищу не нужен интерпретатор), а `eval` не импортирует `store` (D-1) — поэтому `Value` живёт в отдельном листовом пакете `internal/value/` (**вариант A — принят**). Альтернатива (вариант B: `Value` в `eval`, `store`/`engine` импортируют `eval`) отвергнута: делает `eval` «толстым» хабом и втягивает интерпретатор в зависимости хранилища. Дополнительный пакет — цена за граф без двусмысленностей.

**Разрыв цикла `eval ↔ engine` (D-1, фича 006).** Ребра `eval → engine` в графе **нет**: интерфейс `ProcessRuntime` объявлен в `eval` и инжектируется сеттером `Interpreter.SetProcessRuntime`; `engine` его реализует. Через него идут `запустить процесс` (`StartProcess`), персист «присвоить» (`AssignProcessVar`), стабы `вызвать`/`уведомить` (`CallExternal`/`Notify`) и три process-builtins (`InstanceStatus`/`InstanceVariables`/`UserTasks`, D-15). Дословный контракт интерфейса и экспортируемой поверхности `eval` — `docs/engine-model.md §EN-4`. Ранний эскиз ребра `eval → store` (доменные стдлиб-функции ходят в `store` напрямую) **снят**: к `Store` имеет доступ только `engine`.

---

## 3. Пайплайн выполнения

Команда `ladix run file.ladix` — каноничный путь; остальные команды переиспользуют его стадии:

```
исходник (.ladix)
   │
   ▼  [0] чтение файла          (ошибка открытия = CLI Error → exit 2)
байты UTF-8
   │
   ▼  [1] lexer                 (ЛексическаяОшибка)
поток токенов (+ Indent/Dedent/Newline/EOF)
   │
   ▼  [2] parser                (СинтаксическаяОшибка; panic-mode → несколько ошибок)
AST (*ast.Program)
   │
   ▼  [3] семантика (eval)      (СемантическаяОшибка: неизвестное имя, арность, scope)
проверенный AST + таблица символов
   │
   ▼  [4] интерпретатор (eval)  (ОшибкаТипа, ОшибкаВыполнения)
       + движок (engine)        (ошибки движка — те же категории; «ОшибкаПроцесса» зарезервирована, в 006 не вводится — D-14)
   │
   ▼  вывод (stdout), состояние процессов (Store)
exit 0
```

- Стадии 1–2 — фронтенд (токены, дерево). Стадия 3 — однопроходный семантический обход (резолв имён, проверка арности функций, scope-гарды `вернуть`/`прервать`/`продолжить`/`StepAction`, валидация атрибутов `метрика`/`шаг`). Стадия 4 — исполнение.
- **Что НЕ резолвится на стадии 3 (важно для имплементатора).** Голые имена внутри выражений `где`/`агрегат` метрики — это **поля записи источника**, а не глобальные переменные/функции (`SPEC §3`). Их валидность проверяется против **схемы источника** (объединение ключей по загруженным записям), что требует **загрузки данных источника** — то есть это происходит при **вычислении метрики** (стадия 4 / момент чтения метрики), а не статически на стадии 3. Стадия 3 для тел `где`/`агрегат` проверяет только синтаксис/арность стдлиб-функций и не падает на «неизвестном имени» (оно может оказаться полем). См. §5 (модель `Запись`: отсутствующее поле → `пусто`).
- **Накопление ошибок (panic-mode, `SPEC §13`).** Lexer и parser собирают **несколько** ошибок за прогон (синхронизация по точкам восстановления), печатают все в формате `SPEC §13`, в конце — «Найдено K ошибок». Exit-код всё равно `1` (количество диагностик кода не меняет). **Семантика (стадия 3) — fail-fast**: падает на первой ошибке, без единого сквозного коллектора диагностик через все стадии. Panic-mode/накопление — только в лексере и парсере (там реализовано «восстановление после ошибок»); тянуть резолв имён после ошибки в v1 не стоит риска.
- **Граница run vs serve.** `run` гонит весь пайплайн синхронно от старта до конца программы: без `--db` — на `MemoryStore` (эфемерно), с `--db` — на `SQLiteStore` (мост в персист, Q2 фичи 006). `complete <file.ladix> <task-id>` входит в пайплайн до стадии 4 (компиляция файла обязана пройти чисто, Q3) и дальше работает над `SQLiteStore` и движком; `tasks` файла не принимает — только `SQLiteStore`. `serve`/`emit` (фича 007) работают над `SQLiteStore` (демон — пакет `daemon`). `metric`/`repl`, как и `run` без `--db`, остаются на `MemoryStore` (`README, раздел CLI`; канон команд 006 — `docs/engine-model.md §EN-6`).

---

## 4. Эскиз AST-узлов

Контракт: **каждый узел несёт позицию `(Line, Col)`** — требование «колонка везде». Позиция протаскивается из токенов в узлы и далее в рантайм (`eval`/`engine` берут `Line, Col` для сообщений об ошибках).

### 4.1. Идиома позиции и интерфейсы узлов

> **Position дублируется, не разделяется.** В репозитории ТРИ структурно одинаковых
> `Position{Line, Col}`: в `internal/errors` (её несут ошибки и токены), в `internal/ast`
> (её несут узлы AST) и в публичном `ir` (её несут элементы IR и диагностики). Дублирование
> намеренно: оно сохраняет листовыми и `ast`, и `ir` — ни тот, ни другой не импортирует
> `errors`. Перенос между слоями — покомпонентный (`lowerPos`), тип общим не делается.

```go
package ast

// Position — позиция в исходнике; встраивается во все узлы.
// Локальна для пакета ast (не импортируется из errors) — это делает ast листовым.
type Position struct {
    Line int // 1-based
    Col  int // 1-based, КОЛОНКА ВЕЗДЕ
}

// Node — корневой интерфейс. Pos() даёт позицию для диагностик.
type Node interface {
    Pos() Position
}

// Встраиваемая база: даёт Pos() всем узлам через embedding.
type base struct{ position Position }
func (b base) Pos() Position { return b.position }

// Подынтерфейсы — для type-switch в parser/eval/engine.
type Statement    interface { Node; stmtNode() }
type Expression   interface { Node; exprNode() }
type Decl         interface { Node; declNode() }      // FunctionDecl/SourceDecl/MetricDecl/ProcessDecl/TriggerDecl
type TopLevelItem interface { Node }                  // Statement | Decl
```

> Идиома: пустые маркер-методы (`stmtNode()`/`exprNode()`) — стандартный Go-приём «sum type через интерфейс»; компилятор гарантирует, что `IntLit` не пролезет туда, где ждут `Statement`. `base` через embedding избавляет от копипасты `Pos()` в каждом узле.
>
> `Position` дублируется как локальный тип `ast`, а не тащится из `errors`: иначе `ast → errors` лишает `ast` листовости. `errors` держит собственные поля `Line, Col int` (не импортирует `ast`); связка — на месте создания ошибки в `eval`/`parser`, где обе стороны видны.
>
> **Конвенция `Pos()` узла** (load-bearing для диагностик `eval`): `BinaryExpr.Pos`/`UnaryExpr.Pos` = токен **оператора** (не левого операнда); `CallExpr.Pos` = позиция `Callee`; `IndexExpr.Pos`/`FieldExpr.Pos` = позиция `Target`; литералы и `Ident` — свой токен. Так runtime-диагностика указывает на оператор — деление на ноль рапортуется на колонке `/` (см. `examples/ошибка.ladix`, стр. 5 кол. 14).
>
> **Statement-узлы:** инструкции с ведущим ключевым словом (`LetStmt`/`IfStmt`/`WhileStmt`/`ForStmt`/`ReturnStmt`/`BreakStmt`/`ContinueStmt`) и декларации (`FunctionDecl`, а также `RunProcessExpr` по ведущему `запустить`, `StepAction` по `присвоить`/`вызвать`/`уведомить`) берут `Pos` = свой ведущий ключевой токен. Два statement без ключевого слова — явные исключения: `AssignStmt.Pos` = позиция lvalue (токен `Name`/`Ident`), `ExpressionStmt.Pos` = `Expr.Pos()`.

### 4.2. Верхний уровень (`SPEC §3`)

```go
type Program struct {
    base
    Items  []TopLevelItem
    EOFPos Position
}
```

### 4.3. Statements (`SPEC §3`) — реализуют `Statement`

| Узел | Поля |
|---|---|
| `LetStmt` | `Name Ident; Value Expression` |
| `AssignStmt` | `Name Ident; Value Expression` (lvalue только `Ident`; поля/индексы запрещены v1) |
| `ExpressionStmt` | `Expr Expression` |
| `IfStmt` | `Cond Expression; Then *Block; Else *ElseClause` |
| `WhileStmt` | `Cond Expression; Body *Block` |
| `ForStmt` | `Var Ident; Iterable Expression; Body *Block` |
| `ReturnStmt` | `Value Expression` (nil → голый `вернуть` → `пусто`) |
| `BreakStmt` | — |
| `ContinueStmt` | — |
| `StepAction` | три варианта (см. 4.6); входит в `Statement`; семгард «только в шаге» (eval) |

`ElseClause` — union: финальный `иначе` (`Body *Block`) либо `иначе если` (`Cond Expression; Then *Block; Else *ElseClause`).

### 4.4. Declarations (`SPEC §3`) — реализуют `Decl`

| Узел | Поля |
|---|---|
| `FunctionDecl` | `Name Ident; Params []Ident; Body *Block` (позиционные параметры; вложенных функций нет) |
| `SourceDecl` | `Name Ident; File string` (v1: единственный атрибут `файл`) |
| `MetricDecl` | `Name Ident; Source Ident; Where/Aggregate/Period/ByDate Expression; Attrs MetricAttrPos` (плоский — фактический код 004; канон форм — `docs/source-metric-model.md §SM-2`) |
| `MetricAttrPos` | `SourcePos/WherePos/AggregatePos/PeriodPos/ByDatePos Position` (позиции присутствующих атрибутов; вспом., не `Node`) |
| `ProcessDecl` | `Name Ident; Params []Ident (опц.); Steps []*StepDecl` (канон форм процесса/шага — `docs/process-model.md §PM-2`) |
| `StepDecl` | `Name Ident; After []Ident; Assignee Expression; Deadline Expression; Attrs StepAttrPos; Body []Statement` (плоский, как `MetricDecl`; **не** top-level — реализует только `Pos()`) |
| `StepAttrPos` | `AssigneePos/DeadlinePos Position` (позиции присутствующих атрибутов шага; вспом., не `Node`) |
| `TriggerDecl` | `Spec TriggerSpec; Body *Block` (`когда <Spec>: <Body>`, фича 007; формы — `docs/grammar.md`/SPEC §11.6) |

**Узлов `StepLine`/`StepAttr` нет** (D-1, `docs/process-model.md §PM-2`): тело шага плоско —
атрибуты в `Assignee`/`Deadline`+`StepAttrPos`, операторы в `Body []Statement`; парсер разводит
attr/statement при разборе блока (`§PM-3`). Аналогично `MetricDecl` (плоский, без узлов `MetricAttr`).

Семантика валидируется в `eval` (стадия 3), не в типах: `метрика` требует `источник`+`агрегат`, `период`⟺`по_дате`, каждый атрибут ≤1 раза; `срок` без `исполнитель` → ошибка; `StepAfter` ссылается на шаги того же процесса. (Голые имена в `где`/`агрегат` — поля источника, резолв отложен до вычисления метрики, см. §3.)

### 4.5. Trigger specs (`SPEC §3`) — реализуют `TriggerSpec`

| Узел | Поля |
|---|---|
| `MetricTrigger` | `Metric Ident; Op CompOp; Threshold Expression` (срабатывает на переходе ложь→истина) |
| `EventTrigger` | `Event Ident` (в теле доступна предопределённая `событие: Запись`) |
| `ScheduleTrigger` | `Spec ScheduleSpec` |
| `ScheduleSpec` | union: `EverySpec{Dur DurationLit}` (`каждые`) или `AtSpec{Time string}` (`в "ЧЧ:ММ"`) |

### 4.6. Step actions (`SPEC §3`) — варианты `StepAction`

| Узел | Поля | Семантика |
|---|---|---|
| `AssignAction` | `Name Ident; Value Expression` | `присвоить` → мутирует переменные процесса (персист, `SPEC §6`) |
| `CallAction` | `Name Ident; Args []Expression` | `вызвать` → внешняя система; fire-and-forget, без захвата; сбой → инстанс `провален` |
| `NotifyAction` | `Name Ident; Args []Expression` | `уведомить` → стаб-лог |

### 4.7. Expressions (`SPEC §3`, `SPEC §5`) — реализуют `Expression`

Каскад приоритетов (низший→высший): `LogicOr → LogicAnd → LogicNot → Comparison → Additive → Multiplicative → Unary → Postfix → Primary`. Все бинарные операторы **лево-ассоциативны**.

| Узел | Поля |
|---|---|
| `BinaryExpr` | `Op BinOp; Left, Right Expression` (`или`/`и`/`+`/`-`/`*`/`/`/`//`/`%`/`CompOp`; короткозамкнутые `и`/`или`; цепочечные сравнения запрещены) |
| `UnaryExpr` | `Op UnOp; Operand Expression` (`не` / унарный `-`) |
| `CallExpr` | `Callee Expression; Args []Expression` |
| `IndexExpr` | `Target Expression; Index Expression` (срезы не поддерживаются v1) |
| `FieldExpr` | `Target Expression; Field Ident` (`.поле`) |
| `RunProcessExpr` | `Process Ident; Args []Expression` (опц. скобки; возвращает Строку-id, `SPEC §11`) |

Literals / Primary:

| Узел | Терминал |
|---|---|
| `IntLit` | `IntLiteral` → int64 |
| `FloatLit` | `FloatLiteral` → float64 |
| `StringLit` | `StringLiteral` |
| `BoolLit` | `истина`/`ложь` |
| `NoneLit` | `пусто` |
| `DurationLit` | значение + единица (Сек/Мин/Час/Дн/Нед/Мес) |
| `ListLit` | `Elements []Expression` (висящая запятая ок; гетерогенный; `[]`) |
| `Ident` | имя (переменная/функция/поле записи/период-константа — резолв в eval) |
| `GroupExpr` | `(...)` — обычно сворачивается в обёрнутое выражение |

`BinOp` — единый enum всех бинарных операторов: `или` `и` `+` `-` `*` `/` `//` `%` `==` `!=` `<` `<=` `>` `>=`. `CompOp` — именованное подмножество `BinOp` из шести сравнений (`==` `!=` `<` `<=` `>` `>=`), на которое ссылается `MetricTrigger.Op` (§4.5). Один источник истины операторов: `CompOp` не дублирует константы, а отбирает их из `BinOp`.

### 4.8. Структурные узлы (`SPEC §3`)

```go
type Block struct {
    base
    Stmts []Statement // минимум 1; пустые блоки запрещены
}
```

> **Блок-формы определений — отдельные узлы, не `Block`.** `Block` (≥1 statement) — это тело `если`/`пока`/`для`/функции/триггера. Тела `шаг`/`источник`/`метрика`/`процесс` имеют свои наполнители (шаг: `Body []Statement` + плоские `Assignee`/`Deadline`; источник: `File StringLit`; метрика: плоские `Source`/`Where`/`Aggregate`/`Period`/`ByDate`; процесс: `[]*StepDecl`) и **не** являются `Block`. Это уже отражено в полях `StepDecl`/`MetricDecl`/`ProcessDecl`/`SourceDecl` выше — не вводить общий `Block` для них.

> Полный указатель нетерминалов и терминалов лексера — `docs/grammar.md`.

---

## 5. Модель значений `Value` (пакет value)

`Value` — Go-интерфейс, представляющий любое из **10 типов** Ladix (`SPEC §4`). Живёт в листовом пакете `internal/value/` (§2.1, вариант A). Движок и `Store` оперируют `Value`, не зная формата хранения. Имена числовых типов фиксированы каноном: «Целое»/«Дробное» (не «Число»).

```go
package value

type Value interface {
    TypeName() string // "Целое"/"Дробное"/... — основа для тип(x) и сообщений об ошибках типа
}
```

| Тип Ladix | Go-представление | Примечание |
|---|---|---|
| `Целое` | `int64` | переполнение → `ОшибкаВыполнения` |
| `Дробное` | `float64` | IEEE 754 |
| `Строка` | `string` (UTF-8) | неизменяемая; индексация в рунах |
| `Булево` | `bool` | |
| `Пусто` | синглтон | единственное значение `пусто` |
| `Длительность` | пара `(value int64, unit)` | unit ∈ {Сек,Мин,Час,Дн,Нед,Мес}; `1мес` не сводится к секундам |
| `Период` | enum (5 констант) | ежедневно/еженедельно/ежемесячно/ежеквартально/ежегодно |
| `Дата` | год-месяц-день (без времени) | литералов нет; `дата(...)`/`сегодня()` |
| `Список` | **ссылочный**, изменяемый (`*[]Value` или slice-обёртка) | гетерогенный |
| `Запись` | `map[string]Value` (открытая) | литералов нет (v1); отсутствующее поле → `пусто` |

- **Тэг-типа + значение.** Каждый Ladix-тип — отдельная конкретная Go-структура в пакете `value`, реализующая `Value`; type-switch в интерпретаторе различает их. `Список`/`Запись` — ссылочные (мутация через одну переменную видна через другую, `SPEC §4`).
- **Доменные сущности** (`Задача`/`Процесс`/`Событие`/`Роль`) отдельными типами **не** выделены — представлены через `Запись` и `Строка` (`SPEC §4`). `тип(x)` возвращает Строку-имя.
- **Граница сериализации.** Type-tagged JSON (`docs/execution-model.md`) — **исключительно внутреннее дело `SQLiteStore`**. Интерфейс `Store` принимает/отдаёт Go-структуры с `map[string]value.Value`; `MemoryStore` хранит `Value` нативно. Движок про JSON/SQL не знает — это и держит путь к Camunda/Kestra открытым (§9).

---

## 6. Интерфейс `Store` (engine ↔ store)

Каноничный контракт v006 — `docs/engine-model.md §EN-2` (этот раздел синхронизирован с ним; база механики — `docs/execution-model.md`). Устаревший эскиз `SaveProcess`/`LoadProcess` из ранних заметок удалён — не воспроизводить.

Контракт 006 — **нарезанный** (D-3): 8 методов и 3 сентинела над `ProcessInstance`/`Task`. Серия 007/v2 расширила его аддитивно (см. ниже); актуальный интерфейс несёт 18 методов.

```go
type Store interface {
    SaveInstance(inst *ProcessInstance) error            // upsert: создание и обновление
    LoadInstance(id string) (*ProcessInstance, error)    // не найден → ErrInstanceNotFound

    SaveTask(t *Task) error
    LoadTask(id string) (*Task, error)                   // не найдена → ErrTaskNotFound
    ListPendingTasks(assignee string) ([]*Task, error)   // assignee=="" → все открытые; порядок — по возрастанию ID (D-15)
    MarkTaskCompleted(id string, completedAt time.Time) error // атомарно открыта→завершена (D-12); повтор → ErrTaskAlreadyCompleted

    NextInstanceID() (string, error)                     // mint "p-NNNNNN" (D-10)
    NextTaskID() (string, error)                         // mint "t-NNNNNN"
}

var (
    ErrInstanceNotFound     = errors.New("process instance not found")
    ErrTaskNotFound         = errors.New("task not found")
    ErrTaskAlreadyCompleted = errors.New("task already completed")
)
```

В 006 эти 8 методов составляли весь контракт; серия 007/v2 добавила аддитивно (`docs/engine-model.md §EN-2`, `docs/execution-model.md` EM-5):
- **Триггерные** (фича 007): `LoadTriggerState`/`SaveTriggerState`/`NextEventID`/`EnqueueEvent`/`ListUnprocessedEvents`/`MarkEventProcessed` + сентинел `ErrTriggerStateNotFound` (планировщик — §7.5).
- **Листинг инстансов** `ListInstancesByStatus` (рестарт-скан демона `serve`; в 006 его не было — D-4).
- **Outbox-леджер** `LoadOutbox`/`SaveOutbox` + сентинел `ErrOutboxNotFound` (M3, exactly-once эффектов; SQLite — таблица `outbox`, миграция 1→2).

Контрактные инварианты:
- **Сентинелы под `errors.Is`** (идиома Go; не сравнивать строки). `состояние_процесса`/`статус_процесса` так отличают «не найдено» → Ladix-ошибка `процесс '<id>' не найден` от прочих сбоев.
- **`SaveInstance` — upsert.** Движок не различает insert/update; реализация решает (`INSERT … ON CONFLICT` в SQLite / перезапись в карте в Memory).
- **Завершение задачи атомарно (D-12), повтор — гард-догон (D-4).** `MarkTaskCompleted` атомарно переводит `открыта`→`завершена` (SQLite: условный `UPDATE … WHERE status='открыта'` + проверка rows affected; Memory: под mutex); уже завершена → `ErrTaskAlreadyCompleted`. Повторный `complete` уже-завершённой задачи — **не** безусловная ошибка: движок проверяет инстанс, и если тот в `ожидает` И `CurrentStep == task.StepName` (хвост сбоя «задача завершена, advance не успел»), выполняет **идемпотентное до-продвижение** (CLI: exit 0 с пометкой в выводе); иначе — ошибка `задача '<id>' уже завершена` (CLI: exit 2). Прежний канон «повторный complete всегда exit 2» **отменён** D-4. Числовые коды и тексты — решение CLI (`docs/engine-model.md §EN-6/§EN-8.B`), см. §7.2/§8.3.
- **Транзакционного комбо-метода «завершить задачу + продвинуть инстанс» в интерфейсе нет намеренно**: корректность — идемпотентный догон D-4 (§7.4); `SQLiteStore` волен внутри обернуть связанные записи в транзакцию.
- **Граница сериализации** — §5 выше: формат хранения внутри `SQLiteStore`.
- **Две реализации:** `MemoryStore` (`run` без `--db`, тесты lifecycle — карты + счётчики id под одним `sync.Mutex`, без JSON) / `SQLiteStore` (`run --db`/`complete`/`tasks`/`serve`/`emit`; WAL + `busy_timeout=5000` + `foreign_keys=ON`).

### 6.1. Структуры данных движка (`docs/execution-model.md`)

```go
type Status string

const (
    StatusCreated   Status = "создан"      // персистирован, первый шаг ещё не активирован (транзиентно)
    StatusRunning   Status = "выполняется" // активный шаг исполняет тело
    StatusWaiting   Status = "ожидает"     // активный шаг создал Task, инстанс спит
    StatusDone      Status = "выполнен"    // все шаги готовы (терминал)
    StatusFailed    Status = "провален"    // runtime-ошибка шага/атрибута (терминал)
    StatusCancelled Status = "отменён"     // зарезервирован; в v1 недостижим (SPEC §12)
)

type ProcessInstance struct {
    ID          string                 // "p-NNNNNN" (D-10)
    ProcessName string                 // имя ProcessDecl
    Status      Status
    CurrentStep string                 // имя активного шага; при терминале — последний обработанный
    Variables   map[string]value.Value // переменные процесса; пусть-локали шага сюда НЕ попадают
    CreatedAt   time.Time              // engine-Clock (D-2)
    UpdatedAt   time.Time              // выставляет движок перед КАЖДЫМ SaveInstance
}

type TaskStatus string

const (
    TaskPending   TaskStatus = "открыта"
    TaskCompleted TaskStatus = "завершена"
)

type Task struct {
    ID          string     // "t-NNNNNN" (D-10)
    InstanceID  string     // → ProcessInstance.ID
    StepName    string     // шаг, породивший задачу
    Assignee    string     // значение «исполнитель» (Строка, D-18)
    Deadline    *time.Time // CreatedAt + «срок» (D-19); nil, если «срок» не задан
    Status      TaskStatus
    CreatedAt   time.Time
    CompletedAt *time.Time // nil, пока открыта; выставляет MarkTaskCompleted (D-12)
    Escalated   bool       // durable-флаг эскалации просроченной задачи (фича 016 B4b, D-AU-5; одноразово, см. §7.5 checkDeadlines)
}
```

Структуры триггеров/событий (фича 007, отгружены в `store`):

```go
type TriggerState struct {
    TriggerID     string     // "trg-<N>", N = 0-based индекс TriggerDecl в исходнике
    Kind          string     // "metric" | "schedule_every" | "schedule_at"
    LastBool      *bool      // metric: базовая линия; nil = не праймлен
    LastFire      *time.Time // schedule_every: момент последнего срабатывания
    LastFiredDate *string    // schedule_at: дата "YYYY-MM-DD" последнего срабатывания
}

type Event struct {
    ID          string    // "e-…"
    Name        string
    PayloadJSON string    // сырой JSON → Запись при обработке (маппинг как у источников §9)
    CreatedAt   time.Time
    Processed   bool
}
```

- **`*time.Time` для nullable** (`Deadline`, `CompletedAt`, `LastFire`): `nil` = «не задано», идиоматичнее нулевого времени.
- **Просрочка считается движком через engine-Clock** (`engine.Overdue`: `now.After(*Deadline)`, при `nil`-дедлайне — `false`; `now` берётся из `Clock`, D-2 — прямой `time.Now()` есть только в `SystemClock`); наружу — производное `просрочена: Булево` в `Запись` (`docs/engine-model.md §EN-3`, см. §7.7). Сырой `Deadline` Ladix-коду не виден (нет типа `ДатаВремя`).

### 6.2. Идентификаторы (D-10, `docs/engine-model.md §EN-2`)

Mint в `Store`, непрозрачная строка с префиксом типа: `p-` (инстанс), `t-` (задача), `e-` (событие, `NextEventID`, фича 007). Схема D-10: префикс + **персистентный монотонный счётчик**, нуль-паддинг 6 (`fmt.Sprintf("p-%06d", n)` / `"t-%06d"`). SQLite: таблица `counters(name TEXT PRIMARY KEY, value INTEGER NOT NULL)`, инкремент и выдача в одной транзакции; Memory: счётчик под mutex. Случайный `crypto/rand`-суффикс из раннего эскиза (`p-000123-a3f7`) **отменён**: уникальность гарантирует счётчик (`SaveInstance` остаётся upsert — коллизии исключены конструктивно), id маскируемы в golden-тестах. Уникальность — в рамках Store. Пользователь формат не парсит (`SPEC §11`).

---

## 7. Движок процессов (engine)

### 7.1. Движок как библиотека

`engine` — обычный Go-пакет над `Store`, не сетевой сервис и не глобал. Методы `Start`/`advance`/`Complete` переиспользуются и в CLI-командах, и в демоне `serve` (пакет `daemon`). «Продвинуть процесс» — синхронный вызов в том процессе ОС, который инициировал событие (CLI-команда или тик демона). Сетевого слоя в v1 нет.

**Конструктор, Clock, опции** (сигнатуры — канон `docs/engine-model.md §EN-3`):

```go
// Clock — время движка (D-2). НЕ путать с eval.Clock (дневной, value.Дата).
type Clock interface {
    Now() time.Time
}

// SystemClock — продовый Clock; ЕДИНСТВЕННОЕ легальное time.Now() движка.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

type Option func(*Engine)

// WithClock подменяет часы (тесты/golden: фиксированный момент).
func WithClock(c Clock) Option

// NewEngine строит движок над Store и интерпретатором. out — канал системных
// строк stdout (§EN-7); в CLI совпадает с out интерпретатора (печать программы
// и движка перемешиваются в порядке исполнения — всё синхронно).
func NewEngine(st store.Store, interp *eval.Interpreter, out io.Writer, opts ...Option) *Engine
```

Определения процессов движок берёт из интерпретатора (`interp.Process(name)`, `docs/engine-model.md §EN-4`) — для `lookup(ProcessName, CurrentStep)` и «следующего шага по исходнику»; ранний эскиз с картой `procs map[string]*ast.ProcessDecl` в сигнатуре **отменён**. Опции (`Option`): в 006 единственная — `WithClock` (D-2: подмена часов в тестах/golden; дефолт — `SystemClock{}`). Триггеры и интервал тика — у демона `serve` (пакет `daemon`, фича 007, см. §7.5). Engine-Clock используется для `CreatedAt`/`UpdatedAt`/`CompletedAt`, абсолютизации дедлайна (D-19) и `просрочена`; `eval.Clock` (дневной, golden 2026-05-31) не трогается.

### 7.2. Жизненный цикл и точки персиста

Движок резолвит `lookup(ProcessName, CurrentStep)` и «следующий шаг по исходнику» через `interp.Process(name)` (§7.1; `docs/engine-model.md §EN-4`). `CurrentStep` — единственный указатель: параллельности нет, «завершённые шаги» = все объявленные до текущего; `после` — только валидатор (стадия 3), не рантайм-DAG.

Псевдокод (сводка; полный ревизованный — `docs/engine-model.md §EN-3`), ▼ = `SaveInstance` (перед каждым ▼ движок выставляет `UpdatedAt = clock.Now()`):

```
Start(P, args):  id ← NextInstanceID; inst{создан, vars=bind(params,args), current=первый}; ▼; advance(inst); return id
advance(inst):   loop { шаг ← lookup; статус=выполняется ▼; атрибуты шага (D-9) → тело (каждое «присвоить» ▼)
                        if ошибка атрибута или тела: провален ▼ return
                        if исполнитель задан: SaveTask; ожидает ▼ return   # засыпание
                        next ← следующий; if next==∅: выполнен ▼ return; inst.current=next }
Complete(taskID): t←LoadTask; inst←LoadInstance; дрейф-гарды Q3 → гард-догон D-4 → гарды D-8
                        MarkTaskCompleted; next←следующий; if ∅: выполнен ▼ else inst.current=next; advance(inst)
```

> Повторный `complete` уже-завершённой задачи — **гард-догон D-4** (см. §6): если инстанс в `ожидает` И `CurrentStep == task.StepName` (хвост сбоя) — идемпотентное до-продвижение, CLI `exit 0` с пометкой; иначе — ошибка `задача '<id>' уже завершена`, CLI `exit 2`. Числовой код принадлежит CLI-слою (см. §6, §8.3). Фаза атрибутов шага — **до тела** (D-9); гарды D-8: инстанс в `ожидает` И `CurrentStep == task.StepName`, иначе exit 2 без мутаций.

### 7.3. Засыпание и пробуждение

- **Засыпание** = `return` из `advance` в статусе `ожидает` с персистированным инстансом. Никаких висящих горутин/таймеров на время ожидания человека — всё состояние в `Store`; инстанс переживает рестарт демона.
- **Пробуждение** = `ladix complete <file.ladix> <task-id>` в отдельном процессе ОС: строит Engine из исходника (Q3 — истина в файле, БД хранит только состояние), грузит инстанс, помечает задачу (`MarkTaskCompleted`, D-12), синхронно зовёт `advance` со следующего шага, персистит, выходит. Состояние — в SQLite; демону (007) сообщать ничего не нужно.

### 7.4. Восстановление после сбоя (at-least-once)

- `присвоить` коммитит переменную процесса сразу; локальные `пусть` шага не персистируются (теряется только незакоммиченный кадр).
- На рестарте: инстансы в `ожидает` корректны; застрявшие в `выполняется`/`создан` — переактивируются (`advance` прогоняет шаг заново). **Носитель рестарт-скана — демон `serve`** через `ListInstancesByStatus` (добавлен в `Store` фичей 007; в 006 листинга инстансов не было — D-3/D-4). Сбойное окно `complete` «задача завершена, advance не успел» закрывает гард-догон `complete` (D-4, §6).
- **At-least-once на уровне шага:** повтор шага может повторить эффекты. Exactly-once доставки `вызвать`/`уведомить` обеспечивает **outbox-леджер** (`LoadOutbox`/`SaveOutbox`, M3, §6): dedup по ключу `(instance_id, step_name, effect_index)`. Транзакционного комбо-метода в `Store` нет — корректность держит идемпотентный догон D-4 плюс outbox-дедуп.

### 7.5. Планировщик `serve` (пакет `daemon`, `docs/execution-model.md`)

Демон `serve` (пакет `internal/daemon`) поднимает одну горутину-тикер под `context.Context`; на каждом тике — последовательный детерминированный проход из **четырёх фаз**:

```
tick(): drainEvents() → evalMetrics() → checkSchedules() → checkDeadlines()
```

- **`drainEvents`** — `ListUnprocessedEvents()` FIFO; для каждого находит `EventTrigger.Name == E`, парсит `PayloadJSON` → `Запись`, привязывает к предопределённой `событие`, исполняет тела, `MarkEventProcessed`.
- **`evalMetrics`** — переоценка метрика-триггеров; детект перехода ложь→истина через `TriggerState.LastBool`; прайм без срабатывания при `ErrTriggerStateNotFound`; заморозка базы при невычислимом условии.
- **`checkSchedules`** — `каждые D` (якорь `LastFire`, дрейф не копим) и `в "ЧЧ:ММ"` (раз в сутки по `LastFiredDate`).
- **`checkDeadlines`** (4-я фаза, фича 016) — эскалация просроченных задач: проход по открытым задачам с истёкшим `Deadline`, выставление durable-флага `Task.Escalated`. Аддитивна **в хвост** под тем же mutex; порядок и идемпотентность первых трёх фаз не меняет.
- **Изоляция сбоев:** каждый триггер обёрнут `recover` (локальный, не главный барьер `main`) — сбой одного не роняет демон и не зацикливает (базовая линия обновлена до тела).
- **Грациозная остановка:** SIGINT/SIGTERM → `cancel()` → `ctx.Done()` → тикер выходит без утечки горутины.
- **Режим `run`:** один оценочный проход метрика-триггеров (fire-if-true от базы `ложь`); расписание/событие — no-op с логом «требует serve»; `TriggerState` эфемерно в памяти.

### 7.6. Конкурентность

- `ladix run` — один процесс, `MemoryStore`, всё синхронно, гонок нет.
- `serve` + `ladix complete`/`tasks`/`emit` — разные процессы ОС над одним SQLite-файлом; сериализация записей — WAL + `busy_timeout` (не блокировки в коде Ladix).
- Внутри демона тик последовательный; прогон движка под `sync.Mutex`, чтобы два тика не лезли в один инстанс. `trigger_state` трогает только демон.

### 7.7. Отображение `Store` → стандартная библиотека (`docs/execution-model.md`, D-15)

Process-builtins реализованы в `eval` через инжектированный `ProcessRuntime` (`InstanceStatus`/`InstanceVariables`/`UserTasks`, §2.1); `engine` транслирует вызовы к `Store` — сам `eval` к `Store` не ходит:

| Функция Ladix | Через `Store` (трансляция engine) |
|---|---|
| `статус_процесса(id) → Строка` | `LoadInstance(id).Status`; `ErrInstanceNotFound` → `процесс 'id' не найден` |
| `состояние_процесса(id) → Запись` | `LoadInstance(id).Variables` → `Запись` (имя→значение); не найдено → та же ошибка |
| `задачи_пользователя(исполнитель) → Список` | `ListPendingTasks(исполнитель)` → `[]*Task` → `Список` из `Запись` (точные поля ниже) |

**`Task` → `Запись`** (поверхность, видимая Ladix-коду; дословно `docs/execution-model.md`):

| Поле `Запись` | Тип | Источник | Примечание |
|---|---|---|---|
| `ид` | Строка | `Task.ID` | непрозрачный id задачи (`t-…`) |
| `процесс` | Строка | **`Task.InstanceID`** | id **инстанса** (`p-…`), **не** `ProcessName` — ловушка для имплементатора |
| `шаг` | Строка | `Task.StepName` | имя активного шага |
| `исполнитель` | Строка | `Task.Assignee` | значение атрибута `исполнитель` |
| `статус` | Строка | `Task.Status` | `"открыта"` / `"завершена"` |
| `просрочена` | Булево | движок | `engine.Overdue`: `now.After(*Deadline)`, `now` из engine-Clock (D-2); при `nil`-дедлайне — `ложь` |

> Сырой `Deadline` в `Запись` **не** входит (нет `ДатаВремя`, `SPEC §12`); точный дедлайн показывает только CLI-поверхность (строки задач `tasks` и сводки `run`, единый формат `FormatTaskLine` — D-22, `§EN-7` строка 6) — в язык Ladix дедлайн не течёт. `просрочена` — единственный наблюдаемый Ladix-коду признак просрочки.

---

## 8. CLI и обработка ошибок (cmd/ladix, internal/errors)

### 8.1. Диспетчер подкоманд (`README, раздел CLI`; канон 006 — `docs/engine-model.md §EN-6`)

Ручной диспетчер подкоманд (`realMain`: ветвление по `args[0]`; **без** stdlib `flag` и без cobra/внешних зависимостей — факт кода `main.go`). Модель git-подобная: `ladix <команда> [флаги]`. Usage-строка 006:

```
использование: ladix run [--max-depth N] [--db путь] <файл> | ladix metric [--max-depth N] <файл> <имя> | ladix complete [--db путь] [--max-depth N] <файл> <task-id> | ladix tasks [--db путь] [исполнитель]
```

| Команда | Store | Назначение |
|---|---|---|
| `run <file>` | Memory; с `--db` — SQLite | исполнить программу синхронно от старта до конца (`--max-depth`; `--db` — мост в персист, Q2: повторный `run --db` создаёт новые инстансы); в конце — сводка открытых задач |
| `metric <file> <имя>` | Memory | вычислить и напечатать одну метрику (`--max-depth`) |
| `complete <file.ladix> <task-id>` | SQLite | завершить задачу, продвинуть инстанс (`--db`, `--max-depth`); файл обязателен — истина в исходнике (Q3), компиляция должна пройти чисто |
| `tasks [исполнитель]` | SQLite | список открытых задач (`--db`); файла **не** принимает — всё из БД |
| `serve <file>` | SQLite | демон-планировщик (`--db`, `--interval`, `--max-depth`); пакет `daemon` (фича 007) |
| `emit <событие> [json]` | SQLite | поставить внешнее событие в очередь (`--db`); фича 007 |

> Команда `repl` (интерактивный режим) — **вне 006** (дополнительная возможность, фича не назначена); в этой таблице перечислены Store-режимы пакетного запуска, полный список и семантику команд см. `README, раздел CLI`.

`ladix start` **отсутствует** — запуск процесса идёт через `запустить процесс` в программе под `run` (мост в персист — `run --db`, Q2) или через триггер под `serve`; внешний канал данных — у `emit`.

Разбор флагов каждой команды — ручной цикл по аргументам (стиль `runMain`: формы `--флаг значение` и `--флаг=значение`; неизвестный флаг → `ladix: неизвестный флаг <а>`, exit 2) — ошибка разбора возвращает код, не зовёт `os.Exit` мимо барьера. Неизвестная команда → stderr + exit `2`. Флаг хранилища — `--db`: у `complete`/`tasks` дефолт `ladix.db` (текущая директория, cwd); у `run` дефолта нет — без флага `run` работает на `MemoryStore` (Q2).

### 8.2. Контракт ошибок (`SPEC §13`)

- **Категории как Go error-типы** в `internal/errors`, каждый несёт `Line, Col, Msg`, реализует `error`, дружелюбен к `errors.Is`/`errors.As`: `ЛексическаяОшибка`, `СинтаксическаяОшибка`, `СемантическаяОшибка`, `ОшибкаТипа`, `ОшибкаВыполнения`. Категория `ОшибкаПроцесса` зарезервирована прозой SPEC §13.1, **в 006 не вводится** и в коде отсутствует (D-14: ошибки движка — `ОшибкаВыполнения` либо CLI-ошибки).
- **Канонический формат** (русский):
  ```
  Ошибка в строке N, колонка M:
  <описание>
  ```
  опционально ниже — фрагмент строки + caret (`^`). При panic-mode — каждая в этом формате, в конце «Найдено K ошибок».
- **Все сообщения пользователя — на русском** (конвенция «репо на английском» относится к коду/комментариям, не к тексту ошибок). Сообщения хранятся в `internal/errors` (формат) и в местах их порождения (текст `Msg`).

### 8.3. Recover-барьер и коды возврата (`SPEC §13`)

```go
func main() { os.Exit(run(os.Args[1:])) } // вся логика в run, чтобы defer/recover отработали до Exit

func run(args []string) (code int) {
    defer func() {
        if r := recover(); r != nil {
            // дженерик «внутренняя ошибка интерпретатора» в stderr (БЕЗ Go stack trace;
            // трейс только при LADIX_DEBUG)
            code = 1 // recover-путь всегда 1
        }
    }()
    return dispatch(args) // печатает ошибки Ladix в формате SPEC §13; возвращает 0/1/2
}
```

- Любая штатная ошибка Ladix → форматированное сообщение `SPEC §13` в stderr + ненулевой код, **возвращаемый из `dispatch`** (не паника).
- Непредвиденная Go-паника (баг интерпретатора) → `recover` → дженерик-сообщение + exit `1` (без Go stack trace наружу).
- **Коды:** `0` — успех (включая `run` с висящими задачами и гард-догон D-4); `1` — ошибка программы Ladix (lex/parse/semantic/type/runtime, провал инстанса D-14), а также recover-путь; `2` — ошибка использования CLI (плохие аргументы, файл/БД не открыт, неизвестный task-id, гарды `complete` D-8 и дрейф исходника Q3, невалидный JSON у `emit` — 007). Код `2` идёт **только** из `dispatch`; `panic` никогда не выходит с `2`.
- **Повторный `complete` уже-завершённой задачи — гард-догон D-4**, не безусловный `exit 2`: хвост сбоя (инстанс `ожидает`, `CurrentStep == task.StepName`) → идемпотентное до-продвижение, exit `0` с пометкой в выводе; иначе — `задача '<id>' уже завершена`, exit `2`. Тексты и коды — решение CLI (`docs/engine-model.md §EN-6/§EN-8.B`); движок порождает типизированную ошибку (§7.2), CLI-слой присваивает код.

---

## 9. Точки расширения (путь отступления)

Каждая фиксирует, как поменять реализацию, **не ломая синтаксис/UX языка**:

| Что меняем (v2+) | Граница, которую держим в v1 | Где |
|---|---|---|
| Встроенный движок → Camunda/Kestra/другой | Движок зависит только от интерфейса `Store` и AST процессов; не от SQL/JSON. Заменяется реализация `engine`/`Store`, фронтенд не трогается. | §2, §6, §7.1 |
| JSON-источники → реальные коннекторы (БД/API) | `SourceDecl` оставляет блок атрибутов под расширение (`тип`/`разделитель`/`кодировка`); v1 — только `файл`. | `SPEC §9` |
| Стабы `вызвать`/`уведомить` → реальные интеграции | Семантика fire-and-forget и «сбой → провален» уже зафиксированы; меняется только реализация действия. | §4.6, §7.4 |
| Exactly-once эффектов | At-least-once честно задокументирован; outbox/журнал — аддитивны. | §7.4 |
| Стабильный ключ триггера при правке исходника | `TriggerID = "trg-<N>"` (индекс) сейчас; хеш условия — v2, меняет только `engine`/схему `trigger_state`. | §6.1, `docs/execution-model.md` |
| Сетевой приём событий (HTTP) | Источник событий v1 — только `ladix emit` через общий SQLite; добавление транспорта аддитивно к `events`. | §7.5 |
| `ladix start <процесс>` из CLI | Симметрично `emit` (короткоживущий процесс пишет «команду старта» в Store); синтаксис команд не ломается. | §8.1 |

---

## 10. Заметки для имплементации (сверка контрактов)

- **Каноничный `Store`** — `docs/engine-model.md §EN-2` (`SaveInstance`/`LoadInstance`/`MarkTaskCompleted`/…; 8 методов 006 — D-3; §6 синхронизирован с ним); расширен аддитивно до 18 методов (триггерные + `ListInstancesByStatus` + outbox, фичи 007/v2; `docs/execution-model.md` EM-5). Устаревший эскиз `SaveProcess`/`LoadProcess` не воспроизводить.
- **Числовые типы — только «Целое»/«Дробное»** (`SPEC §4`). «Число» в пользовательских сообщениях об ошибке — дрифт; сообщение вида «условие должно быть Булево» при нечисловом операнде печатает фактическое имя типа (`Целое`/`Дробное`/…), не «Число».
- **Накопление ошибок** (§3): fail-fast в семантике (стадия 3 падает на первой ошибке). Panic-mode/«K ошибок» — только лексер+парсер. Единого сквозного коллектора диагностик через все три стадии в v1 нет.
- **Резолв полей `где`/`агрегат` отложен до вычисления метрики** (§3, §4.4) — это не статическая стадия 3, а проверка против схемы источника при загрузке данных (`SPEC §3`). Имплементатору: не падать на «неизвестном имени» внутри `где`/`агрегат` на стадии 3.
- **Golden-фикстура `ошибка.ladix`.** Рантайм-диагностики завязаны на точное протаскивание `(Line, Col)` (канон «колонка везде»). Эталон: колонка `14` для оператора `/` в `печать(всего / делитель)` (счёт с 1: `печать(` = 1–7, `всего` = 8–12, пробел = 13, `/` = 14) и канонический текст `деление на ноль` (деление `/`, `//`, `%` на нулевой делитель — runtime-ошибка, `SPEC §13`).
