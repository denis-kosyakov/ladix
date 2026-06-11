<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan:
specs/006-process-engine/plan.md (фича 006-process-engine — ДВИЖОК ИСПОЛНЕНИЯ ПРОЦЕССОВ +
ЧЕЛОВЕК-В-ЦИКЛЕ: новый пакет internal/store (контракт Store из 8 методов, две реализации —
MemoryStore память / SQLiteStore SQLite-файл; кодек type-tagged JSON; персистентный mint id);
новый пакет internal/engine (lifecycle Start/advance/Complete, engine-Clock, фаза атрибутов до
тела, гард-догон, реализация eval.ProcessRuntime); граница eval↔engine — интерфейс ProcessRuntime
в eval + сеттер-инъекция (D-1, разрыв цикла); активация deferred-веток 005 (действия шага, литерал
длительности, сравнения Длительности, 3 процессные встроенные — реестр 28+7); CLI run --db /
complete <file.ladix> <task-id> / tasks [исполнитель]; байт-точные stdout (§EN-7, 11 форматов) и
диагностики (§EN-8.A 9 / §EN-8.B 10). Первая внешняя зависимость проекта — modernc.org/sqlite
(чистый Go без CGO, разрешена конституцией I; go.mod+go.sum отдельным коммитом). Связывающий якорь —
docs/engine-model.md §EN-0…§EN-10 (решения Q1–Q3, D-1…D-22 закрыты). Фронтенд процессов 005 — в
specs/005-process-frontend/plan.md; декларативный слой 004 — в specs/004-source-metric/plan.md;
интерпретатор 003 — в specs/003-interpreter-eval/plan.md; парсер+AST 002 — в
specs/002-parser-ast/plan.md; лексер 001 — в specs/001-lexer-tokens/plan.md).
<!-- SPECKIT END -->
