<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan:
specs/009-v1-finalization/plan.md (фича 009-v1-finalization — ФИНАЛИЗАЦИЯ ПРОДУКТА v1: закрытие
разрывов «док↔реальность», «чеклист↔факт», «витрина↔скоуп» БЕЗ новой языковой функциональности.
Затрагиваются ТОЛЬКО доки (README/SPEC/ARCHITECTURE/docs/CHECKLIST), examples/*.ladix + MANIFEST.md,
и Go-тесты в src/internal/* + src/cmd/ladix (golden/smoke). 0 новых зависимостей, 0 строк прод-кода
языка. US1 воспроизводимость README из чистого клона: cd src перед build/vet/test, мост «собрать в
корень» (cd src && go build -o ../ladix ./cmd/ladix), запуск ./ladix из корня, тесты cd src && go
test ./...; go.mod НЕ переносить, go.work НЕ использовать. US2 CHECKLIST.md: 50× [x] с
доказательством файл:строка, текст рубрики НЕ менять, hello.lang со сноской .lang→.ladix. US3
витрина: 7 классов (событие/расписание-триггеры, мультиисточник/мультиметрика, полный цикл процесса
run --db→tasks→complete, 3 негатива синтаксис/тип/процесс) + записи MANIFEST + golden/негатив-замки;
детерминизм FixedClock + t.TempDir() (паттерн trigger_golden_test.go), пиннинг сегодня() ЗАПРЕЩЁН;
выручка.ladix — отдельный run-golden на фикс-Clock. US4 docs/quickstart.md (пользовательский,
русский, метрика+процесс за 5 мин) — создаёт implement. US5 выравнивание доков: «Go 1.22+»→go 1.25
(src/go.mod), снять «Найдено K ошибок» (тест-замок отсутствия НЕ трогать), тесты в src/internal/,
онбординг.ladix:13 под движок 006, тип(x) «зарезервировано v1» + сноска достижимости 34 БЕЗ
активации. US6 финальная приёмка против рубрики §1–§9 + чистое дерево. Constitution 9/9 PASS,
0 NEEDS CLARIFICATION. Связывающий якорь — docs/v1-finalization-model.md §VF-0…§VF-8 + «Предрешённые
развилки» (любой пробел закрывается ТАМ). Дата/время builtins 008 — в specs/008-datetime-builtins/
plan.md; Триггеры 007a — в specs/007a-trigger-frontend/plan.md; движок
процессов 006 — в specs/006-process-engine/plan.md; фронтенд процессов 005 — в
specs/005-process-frontend/plan.md; декларативный слой 004 — в specs/004-source-metric/plan.md;
интерпретатор 003 — в specs/003-interpreter-eval/plan.md; парсер+AST 002 — в
specs/002-parser-ast/plan.md; лексер 001 — в specs/001-lexer-tokens/plan.md).
<!-- SPECKIT END -->

## Контекст основного потока ≤20%

Держать заполнение основного контекста ≤20%. Тяжёлые чтение/поиск/анализ делегировать субагентам (Task); в основной поток возвращать только выводы, не сырьё. При росте к порогу — делегируй или сворачивай.
