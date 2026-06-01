# Полная грамматика Ladix

Полная грамматика Ladix. Ядро и навигация — SPEC §3.

Этот документ содержит полный EBNF всех конструкций языка (то, что не вошло в ядро SPEC §3), пошаговый алгоритм лексера INDENT/DEDENT и алфавитный индекс всех нетерминалов и терминалов.

Опирается на категории токенов из §2: `Ident`, `IntLiteral`, `FloatLiteral`, `StringLiteral`, `BoolLiteral`, `NoneLiteral`, `DurationLiteral`, `Newline`, `Indent`, `Dedent`, `EOF`.

## 1. Нотация

Используется W3C EBNF (как в спецификациях XML, JSON, URL):

| Конструкция | Значение |
|---|---|
| `::=` | определение |
| `\|` | альтернатива |
| `*` | ноль или более повторений |
| `+` | одно или более повторений |
| `?` | опционально (ноль или одно) |
| `(...)` | группировка |
| `"..."` | литеральный токен (ключевое слово, символ оператора) |
| Имя с заглавной буквы | нетерминал или класс токенов (например, `Ident`, `IntLiteral`) |

## 2. Верхний уровень

Ladix-файл — скриптовый: смесь определений и императивных операторов, исполняется сверху вниз.

```ebnf
Program      ::= TopLevelItem* EOF
TopLevelItem ::= Statement
              | FunctionDecl
              | SourceDecl
              | MetricDecl
              | ProcessDecl
              | TriggerDecl
```

**Семантика:**
- `FunctionDecl`, `SourceDecl`, `MetricDecl`, `ProcessDecl`, `TriggerDecl` — определения: регистрируются в глобальном пространстве, не выполняются в момент объявления.
- `Statement` (`пусть`, присваивание, `если`, `пока`, `для`, вызов, `печать`, ...) — исполняется немедленно.

## 3. Императивные операторы

```ebnf
Statement       ::= LetStmt
                  | AssignStmt
                  | IfStmt
                  | WhileStmt
                  | ForStmt
                  | ReturnStmt
                  | BreakStmt
                  | ContinueStmt
                  | StepAction
                  | ExpressionStmt

LetStmt         ::= "пусть" Ident "=" Expression Newline
AssignStmt      ::= Ident "=" Expression Newline
ExpressionStmt  ::= Expression Newline

IfStmt          ::= "если" Expression ":" Block ElseClause?
ElseClause      ::= "иначе" ":" Block
                  | "иначе" "если" Expression ":" Block ElseClause?

WhileStmt       ::= "пока" Expression ":" Block
ForStmt         ::= "для" Ident "в" Expression ":" Block

ReturnStmt      ::= "вернуть" Expression? Newline
BreakStmt       ::= "прервать" Newline
ContinueStmt    ::= "продолжить" Newline
```

**Тонкости:**
- `LetStmt` объявляет переменную в текущей области видимости. `AssignStmt` без `пусть` — переприсваивание; имя должно быть уже объявлено (проверка на этапе семантики).
- Lvalue в `AssignStmt` — только `Ident`. Присваивания в поля и индексы (`x.поле = ...`, `x[i] = ...`) **в v1 не допускаются**.
- `ReturnStmt` без выражения возвращает `пусто`. Валиден только внутри тела функции; вне функции (top-level, шаг процесса, тело триггера) — семантическая ошибка (см. §7).
- `BreakStmt`/`ContinueStmt` валидны только внутри `WhileStmt`/`ForStmt`; «ближайший охватывающий цикл» в смысле лексической вложенности (проверка — семантика, см. §7).
- На одной строке разрешён **только один statement**. `;` как разделитель не вводится.
- **Запуск процесса** (`запустить процесс P(…)`) — выражение, не отдельный statement (см. раздел 9, `RunProcessExpr`). Standalone-использование оформляется как `ExpressionStmt`.
- **`StepAction`** (`присвоить`/`вызвать`/`уведомить`, раздел 7) входит в `Statement`, поэтому допустим внутри `IfStmt`/`WhileStmt`/`ForStmt`. Семантически валиден **только в теле шага процесса** (включая вложенные блоки внутри шага); вне шага (top-level, тело функции, тело триггера) — семантическая ошибка `действие '<имя>' допустимо только в шаге процесса` (§11).

## 4. Функции

```ebnf
FunctionDecl    ::= "функция" Ident "(" ParamList? ")" ":" Block
ParamList       ::= Ident ("," Ident)* ","?
```

В v1 — только позиционные параметры, без значений по умолчанию, без `*args`/`**kwargs`. Именованные аргументы при вызове не поддерживаются.

**Вложенные функции не поддерживаются.** `FunctionDecl` входит только в `TopLevelItem` (раздел 2) и **не входит** в `Statement` (раздел 3). Объявление `функция` внутри тела другой функции, шага процесса или триггера — синтаксическая ошибка. Замыканий в языке нет. См. §8 и §12.

## 5. Источник

```ebnf
SourceDecl      ::= "источник" Ident ":" SourceBlock
SourceBlock     ::= Newline Indent SourceAttr+ Dedent
SourceAttr      ::= "файл" ":" StringLiteral Newline
```

В v1 единственный атрибут — `файл`. Блок остаётся, чтобы в v2 расширяться (`тип`, `разделитель`, `кодировка`) без поломки совместимости.

```ladix
источник заказы:
    файл: "data/orders.json"
```

## 6. Метрика

```ebnf
MetricDecl      ::= "метрика" Ident ":" MetricBlock
MetricBlock     ::= Newline Indent MetricAttr+ Dedent
MetricAttr      ::= ("источник" | "где" | "агрегат" | "период" | "по_дате") ":" Expression Newline
```

**Семантические правила** (проверяются после парсинга):
- Атрибуты `источник`, `агрегат` — обязательны.
- Атрибуты `где`, `период`, `по_дате` — опциональны.
- `период` требует `по_дате` (и наоборот: `по_дате` без `период` не имеет эффекта) — семантика §10.
- Каждый атрибут указывается не более одного раза.
- Порядок атрибутов произвольный.
- `по_дате` задаёт ось времени для нарезки по `период`: выражение, вычисляемое на каждой записи (scope полей §6), даёт `Дата` (или `пусто` → запись вне окна). Семантика — §10.

**Поля записи как идентификаторы.** Внутри выражений `где` и `агрегат` неопределённые имена (не глобальные переменные, не функции) трактуются как поля текущей записи источника. Это семантика, грамматика просто видит `Expression`.

```ladix
метрика месячная_выручка:
    источник: заказы
    где:      статус == "оплачен"
    агрегат:  сумма(сумма_заказа)
    период:   ежемесячно
    по_дате:  дата(дата_заказа)
```

Слова `ежедневно`, `еженедельно`, `ежемесячно`, `ежеквартально`, `ежегодно` — **предопределённые идентификаторы** периодов (значения типа `Период` в стандартной библиотеке, не ключевые слова; см. §4).

## 7. Процесс и шаг

```ebnf
ProcessDecl     ::= "процесс" Ident ("(" ParamList? ")")? ":" ProcessBlock
ProcessBlock    ::= Newline Indent StepDecl+ Dedent

StepDecl        ::= "шаг" Ident StepAfter? ":" StepBlock
StepAfter       ::= "после" Ident ("," Ident)*

StepBlock       ::= Newline Indent StepLine+ Dedent
StepLine        ::= StepAttr | Statement

StepAttr        ::= ("исполнитель" | "срок") ":" Expression Newline

StepAction      ::= AssignAction | CallAction | NotifyAction
AssignAction    ::= "присвоить" Ident "=" Expression Newline
CallAction      ::= "вызвать" Ident "(" ArgList? ")" Newline
NotifyAction    ::= "уведомить" Ident "(" ArgList? ")" Newline
```

**Семантические правила:**
- Атрибуты шага (`исполнитель`, `срок`) указываются не более одного раза каждый.
- `StepAfter` ссылается на имена шагов, объявленных в том же процессе (проверка — семантика).
- `StepAction` входит в `Statement` (раздел 3), поэтому `StepLine` сводится к `StepAttr | Statement`, а действия допустимы и **внутри** `если`/`пока`/`для` шага. Семантический гард: `присвоить`/`вызвать`/`уведомить` вне тела шага процесса — ошибка (§11). Различие действий по семантике:
  - `присвоить` модифицирует переменные процесса (персистируются, §6);
  - `вызвать` обращается к внешней системе (в v1 — стаб-лог; сбой → процесс `провален`; без захвата результата — §11);
  - `уведомить` отправляет сообщение исполнителю/роли (в v1 — стаб-лог, §11).
- **Параметры процесса** (`ParamList?` в `ProcessDecl`) — позиционные, без значений по умолчанию (синтаксис идентичен `FunctionDecl`, раздел 4). Связываются с переданными аргументами при создании `ProcessInstance` и становятся **начальными переменными процесса** (§6): видны во всех шагах, мутируются через `присвоить`. Скобки опциональны: `процесс P:` без параметров — корректно. Запуск процесса с аргументами — выражение `RunProcessExpr` (раздел 9, §7).

```ladix
процесс разбор_падения(текущая_выручка):
    шаг подготовка_отчёта:
        исполнитель: "аналитик"
        срок:        2дн
        присвоить просадка = текущая_выручка
        вызвать сформировать_отчёт(текущая_выручка)
    шаг встреча_с_руководителем после подготовка_отчёта:
        исполнитель: "директор"
        срок:        1дн
        уведомить руководитель("отчёт готов, выручка: " + строка(просадка))
```

## 8. Триггер

```ebnf
TriggerDecl     ::= "когда" TriggerSpec ":" Block

TriggerSpec     ::= MetricTrigger
                  | EventTrigger
                  | ScheduleTrigger

MetricTrigger   ::= "метрика" Ident CompOp Expression
EventTrigger    ::= "событие" Ident
ScheduleTrigger ::= "расписание" ScheduleSpec
ScheduleSpec    ::= "каждые" DurationLiteral
                  | "в" StringLiteral
```

**Семантика:**
- `MetricTrigger` срабатывает на переходе условия из ложного в истинное (не на каждом обновлении метрики).
- `EventTrigger` срабатывает при поступлении внешнего события с указанным именем.
- `ScheduleTrigger`:
  - `каждые <DurationLiteral>` — периодически (например, `каждые 1дн`);
  - `в <строка>` — в указанное время дня (формат `"ЧЧ:ММ"`, парсится семантикой).
- В теле `EventTrigger` доступна предопределённая переменная `событие` с данными события.

## 9. Выражения

Каскад приоритетов (от низшего к высшему):

```ebnf
Expression      ::= LogicOr
LogicOr         ::= LogicAnd ("или" LogicAnd)*
LogicAnd        ::= LogicNot ("и" LogicNot)*
LogicNot        ::= "не" LogicNot | Comparison
Comparison      ::= Additive (CompOp Additive)?
CompOp          ::= "==" | "!=" | "<" | "<=" | ">" | ">="
Additive        ::= Multiplicative (("+" | "-") Multiplicative)*
Multiplicative  ::= Unary (("*" | "/" | "//" | "%") Unary)*
Unary           ::= "-" Unary | Postfix
Postfix         ::= Primary PostfixOp*
PostfixOp       ::= "(" ArgList? ")"
                  | "[" Expression "]"
                  | "." Ident
ArgList         ::= Expression ("," Expression)* ","?

Primary         ::= IntLiteral
                  | FloatLiteral
                  | StringLiteral
                  | BoolLiteral
                  | NoneLiteral
                  | DurationLiteral
                  | ListLiteral
                  | RunProcessExpr
                  | Ident
                  | "(" Expression ")"
ListLiteral     ::= "[" (Expression ("," Expression)* ","?)? "]"
RunProcessExpr  ::= "запустить" "процесс" Ident ("(" ArgList? ")")?
```

**Правила:**
- **Цепочечные сравнения запрещены.** `1 < x < 10` — ошибка парсера: `сравнения нельзя сцеплять, используйте 'и': 1 < x и x < 10`.
- **Деление:** `/` всегда возвращает `Дробное` с плавающей точкой; `//` — целочисленное деление (оба операнда целые → целое, иначе runtime-ошибка типа); `%` — остаток. Деление на ноль (`/`, `//`, `%`) — runtime-ошибка (§4, §13).
- **Литерал списка** с висящей запятой допускается: `[1, 2, 3,]`. Гетерогенный: `[1, "две", истина]`.
- **`RunProcessExpr`** — запуск процесса как выражение, возвращает строковый идентификатор инстанса (см. §7). Скобки `(...)` — часть самой `RunProcessExpr`, не Postfix-вызов по результату. Парсер при виде `запустить` сразу выбирает эту альтернативу, неоднозначности с Postfix нет. Standalone-использование оформляется как `ExpressionStmt`.
- **Возведение в степень** в v1 не поддерживается.
- **Тернарный оператор** в v1 не поддерживается (используется блочный `если/иначе`).

## 10. Блок и структурные токены

```ebnf
Block           ::= Newline Indent Statement Statement* Dedent
```

**Виртуальные токены** (выдаются лексером, не присутствуют в исходном тексте):
- `Newline` — конец логической строки. Внутри парных скобок (`(...)`, `[...]`, `{...}`) лексер не выдаёт `Newline`.
- `Indent` — увеличение уровня отступа на одну ступень (+4 пробела).
- `Dedent` — уменьшение уровня отступа на одну ступень. Выдаётся по одному на каждый снятый уровень.
- `EOF` — конец файла; перед ним лексер закрывает все открытые блоки последовательностью `Dedent`.

**Пустые блоки запрещены** — `Block` требует минимум один `Statement`.

### 10.1. Алгоритм лексера для INDENT/DEDENT

1. Стек уровней начинается с `[0]`.
2. В начале каждой логической строки считаем ведущие пробелы → `cur`.
3. `cur > top(stack)` → `push(cur)`, выдать `Indent`.
4. `cur < top(stack)` → `pop()`, выдавать `Dedent` пока `top(stack) == cur`. Если не сошлось — ошибка:

   ```
   Ошибка в строке N, колонка M:
   отступ не соответствует ни одному внешнему уровню
   ```

5. `cur == top(stack)` → ничего не делать.

Дополнительные лексические правила отступов (табы запрещены, отступ кратен 4) и полный реестр диагностик — в §13.

## 11. Алфавитный индекс нетерминалов и терминалов

Алфавитный указатель всех нетерминалов:

`Additive`, `ArgList`, `AssignAction`, `AssignStmt`, `Block`, `BreakStmt`, `CallAction`, `CompOp`, `Comparison`, `ContinueStmt`, `ElseClause`, `EventTrigger`, `Expression`, `ExpressionStmt`, `ForStmt`, `FunctionDecl`, `IfStmt`, `LetStmt`, `ListLiteral`, `LogicAnd`, `LogicNot`, `LogicOr`, `MetricAttr`, `MetricBlock`, `MetricDecl`, `MetricTrigger`, `Multiplicative`, `NotifyAction`, `ParamList`, `Postfix`, `PostfixOp`, `Primary`, `ProcessBlock`, `ProcessDecl`, `Program`, `ReturnStmt`, `RunProcessExpr`, `ScheduleSpec`, `ScheduleTrigger`, `SourceAttr`, `SourceBlock`, `SourceDecl`, `Statement`, `StepAction`, `StepAfter`, `StepAttr`, `StepBlock`, `StepDecl`, `StepLine`, `TopLevelItem`, `TriggerDecl`, `TriggerSpec`, `Unary`, `WhileStmt`.

**Терминалы из лексера:** `Ident`, `IntLiteral`, `FloatLiteral`, `StringLiteral`, `BoolLiteral`, `NoneLiteral`, `DurationLiteral`, `Newline`, `Indent`, `Dedent`, `EOF`, плюс ключевые слова и операторы/разделители из §2.
