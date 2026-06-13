<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan:
specs/007a-trigger-frontend/plan.md (фича 007a-trigger-frontend — ФРОНТЕНД ТРИГГЕРОВ `когда …:`
+ МЕТРИКА-FIRE-IF-TRUE В `run`: разрез B — чистый фронтенд трёх форм триггера (метрика/событие/
расписание) плюс ОДНА точка исполнения. 0 новых пакетов: расширяются ast/parser/eval + 1 врезка в
cmd/ladix. Новые узлы AST — TriggerDecl (declBase) + маркер-интерфейсы TriggerSpec (Metric/Event/
Schedule) и ScheduleSpec (Every/At) + первичные ValueExpr/EventExpr (доступ событие.поле через
FieldExpr). Два независимых шва парсера (D-TR-1): шов A — top-level диспетчер `когда` (минус KW_WHEN
из isUnexpectedTopLevel, parseTriggerDecl); шов B — Primary `значение`/`событие`. Семпроход:
регистрация триггеров, резолв метрики, флаги inMetricTrigger/inEventTrigger (зеркало inStep),
контекст-гарды значение/событие; переиспользование гардов действий-шага §PM-6.B и checkRunProcess
§PM-6.C. fire-if-true — экспортный метод интерпретатора (RunTriggers), врезка после interp.Run, до
сводки задач: вычислить метрику → порог → сравнить → инжектировать read-only `значение` → исполнить
тело штатным путём движка 006. База ЛОЖЬ эфемерно (trigger_state не читается/не пишется даже под
--db). Событие/расписание — строка-заглушка (no-op). 7 новых диагностик (4 сем: TR-VAL-CTX/TR-EVT-CTX/
TR-MET-UNDECL/TR-MET-NOTMETRIC; 3 синт: SE-TRIGGER-KIND/SE-EXPECT-COMPOP/SE-SCHEDULE-SPEC). Связывающий
якорь — docs/trigger-model.md §TR-0…§TR-11 (D-TR-1, D-25…D-27 закрыты; нет/мес валидны без ошибки,
формат "ЧЧ:ММ" и демон serve/события/edge+trigger_state/исполнение расписания — граница 007b). Движок
процессов 006 — в specs/006-process-engine/plan.md; фронтенд процессов 005 — в
specs/005-process-frontend/plan.md; декларативный слой 004 — в specs/004-source-metric/plan.md;
интерпретатор 003 — в specs/003-interpreter-eval/plan.md; парсер+AST 002 — в
specs/002-parser-ast/plan.md; лексер 001 — в specs/001-lexer-tokens/plan.md).
<!-- SPECKIT END -->

## Контекст основного потока ≤20%

Держать заполнение основного контекста ≤20%. Тяжёлые чтение/поиск/анализ делегировать субагентам (Task); в основной поток возвращать только выводы, не сырьё. При росте к порогу — делегируй или сворачивай.
