<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan:
specs/012-mdx-diagnostics/plan.md (фича 012-mdx-diagnostics — веха M-DX «Диагностика и
восстановление парсера», фронтенд v2 после M1, БЕЗ новой языковой функциональности. Две независимые
US. US1 (P1) DX1 — подавление фантомного каскада: ведущее sync-lead ключевое слово в позиции
выражения на одной/смежной строке даёт 2–4 диагностики на одну сломку → цель РОВНО 1; контроль
`значение⏎{` = 2 (анти-over-suppress). Единственная правка — default-ветка parsePrimary
(parse_expr.go:209): потреблять токен ДО error(), зеркально parse_stmt.go:29; synchronize/suppress-
reset НЕ трогать; критерий смежности структурный (идентичность токена/владение блоком), НЕ Pos.Line;
новый хелпер assertDiagnostics (упорядоченный count-exact); decl-сайты M1 — по мутпробе. ЕДИНСТВЕННЫЙ
пункт v2, модифицирующий горячий инвариант panic-mode (FR-025@002:164). US2 (P2) DX2 — бизнес-
формулировки scope A (лексика L-1..L-11=11 + синтаксис SE-*=14; де-жаргон: «токен»→«элемент»,
«литерал»→«число/строка в кавычках»), коды внутренние, двухстрочный канон §13 + позиция сохранены;
полный инвентарь каталога с count-locks (L=11, SE=14; eval=28 НЕ трогать); подсказки «возможно, вы
имели в виду…» (Левенштейн KW+имена); витрина — НОВЫЕ файлы, ошибочная.ladix НЕ перезаписывать.
Границы: только parser/lexer-каталог/examples; ПУСТОЙ дифф eval/engine/store; ProcessRuntime 7 /
Store 15; 0 новых зависимостей; детерминизм. Канон новых текстов — docs/diagnostics-model.md
(разрешает Принцип VIII; запись в Complexity Tracking), большой SPEC §13.4 синкает архитектор на шве.
Ветка БЕЗ авто-мержа в master. Constitution 9/9 PASS. Финализация v1 009 — в
specs/009-v1-finalization/plan.md; Дата/время builtins 008 — в specs/008-datetime-builtins/
plan.md; Триггеры 007a — в specs/007a-trigger-frontend/plan.md; движок
процессов 006 — в specs/006-process-engine/plan.md; фронтенд процессов 005 — в
specs/005-process-frontend/plan.md; декларативный слой 004 — в specs/004-source-metric/plan.md;
интерпретатор 003 — в specs/003-interpreter-eval/plan.md; парсер+AST 002 — в
specs/002-parser-ast/plan.md; лексер 001 — в specs/001-lexer-tokens/plan.md).
<!-- SPECKIT END -->

## Контекст основного потока ≤20%

Держать заполнение основного контекста ≤20%. Тяжёлые чтение/поиск/анализ делегировать субагентам (Task); в основной поток возвращать только выводы, не сырьё. При росте к порогу — делегируй или сворачивай.
