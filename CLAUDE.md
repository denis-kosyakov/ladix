<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan:
specs/005-process-frontend/plan.md (фича 005-process-frontend — ФРОНТЕНД ПРОЦЕССОВ v1,
ЧИСТЫЙ ФРОНТЕНД (исполнение тела шага/действий и движок процессов deferred до 006): новые
плоские AST-узлы ProcessDecl/StepDecl/StepAttrPos (internal/ast); переход парсера —
parseProcessDecl/parseStepDecl/parseAfterList + снятие cut только с KW_PROCESS (internal/parser);
расширение семпрохода Analyze — i.processes, регистрация в шаге 1, checkProcessDecl (уникальность
шагов, резолв 'после', срок-без-исполнителя, analyzeStep с засевом параметрами), контекст-гард
действий через прокидку inStep, checkRunProcess (арность против i.processes); сдвиг СЕМАНТИЧЕСКОЙ
стороны границы deferred (§PM-5) — eval-исполнение остаётся рантайм-deferred. Единственная
наблюдаемая рантайм-граница — top-level 'запустить процесс'. Связывающий якорь —
docs/process-model.md §PM-0…§PM-8 (решения D-1…D-13 закрыты). Декларативный слой 004 — в
specs/004-source-metric/plan.md; интерпретатор 003 — в specs/003-interpreter-eval/plan.md;
парсер+AST 002 — в specs/002-parser-ast/plan.md; лексер 001 — в specs/001-lexer-tokens/plan.md).
<!-- SPECKIT END -->
