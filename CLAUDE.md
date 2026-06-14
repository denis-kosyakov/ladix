<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan:
specs/008-datetime-builtins/plan.md (фича 008-datetime-builtins — АКТИВАЦИЯ 7 ОТЛОЖЕННЫХ ДАТА/ВРЕМЯ
BUILTINS: `вчера`/`завтра`/`длительность`/`в_секундах`/`в_минутах`/`в_часах`/`в_днях` на внутренней
Go date-math. 0 новых пакетов/зависимостей — расширяется только internal/eval. Новый файл
src/internal/eval/builtins_duration.go (стиль builtins_date.go): вчера/завтра = i.now()±1 день через
window.go (dateToTime/AddDate/timeToDate, без операторной арифметики Ladix); длительность(n,u) —
конструктор value.Длительность{Amount,Unit} БЕЗ нормализации (валидация арность→тип→единица из 6
канонических); конвертеры в_* — totalSec через mulInt64, целочисленное частное с усечением к нулю
(зеркало целое()), мес→ошибка без даты-якоря. Реестр builtins.go: 7 явных add(Builtin{ArityFixed});
deferredNames→[]; цикл заглушек удалить; инвариант-коммент → «РОВНО 35 активных, 0 deferred = 35».
Ретайр SEM-DEFERRED-BUILTIN: ветки if b.Deferred (analyze.go:592/call.go:34) остаются ИНЕРТНЫМ
backstop'ом (поле Deferred НЕ удалять), golden-кейс удалить, счётчик errors_golden 29→28. 4 новые
строки ошибок (байт-в-байт, гильемы «», без точки): «длительность: ожидается Целое и Строка,
получено <тип> и <тип>» · «длительность: неизвестная единица «<единица>»» · «<имя>: ожидается
Длительность, получено <тип>» · «<имя>: месяцы не приводятся без даты-якоря»; арность/переполнение
— существующие строки. Тест-замки (4 файла: builtins_test/duration_builtins_test/analyze_test/
errors_golden_test) инвертируются СИНХРОННО. Операторная арифметика дат/длительностей (Дата±Длит,
Дата−Дата, Длит±Длит, Длит*X) ОСТАЁТСЯ deferred v2 (D-A=A). Связывающий якорь —
docs/datetime-builtins-model.md §DB-0…§DB-9 (синки shared-доков §DB-8 — работа архитектора при
посадке, реализатор НЕ трогает). Триггеры 007a — в specs/007a-trigger-frontend/plan.md; движок
процессов 006 — в specs/006-process-engine/plan.md; фронтенд процессов 005 — в
specs/005-process-frontend/plan.md; декларативный слой 004 — в specs/004-source-metric/plan.md;
интерпретатор 003 — в specs/003-interpreter-eval/plan.md; парсер+AST 002 — в
specs/002-parser-ast/plan.md; лексер 001 — в specs/001-lexer-tokens/plan.md).
<!-- SPECKIT END -->

## Контекст основного потока ≤20%

Держать заполнение основного контекста ≤20%. Тяжёлые чтение/поиск/анализ делегировать субагентам (Task); в основной поток возвращать только выводы, не сырьё. При росте к порогу — делегируй или сворачивай.
