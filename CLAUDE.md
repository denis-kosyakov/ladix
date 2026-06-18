<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan:
specs/022-human-explain-fire/plan.md (фича 022-human-explain-fire — M3 «Надёжность» пункт C5:
человеко-explain срабатывания (наблюдаемость «почему»), в дополнение к inspect («где»). При
срабатывании метрик-триггера ВСЕГДА (always-on, D-C-6) печатать одну человеко-читаемую строку §C-5.3:
run → i.out (fire-if-true, без ребра; протянуть i.out в runMetricTrigger, eval/trigger_run.go:78-92,
писатель тела НЕ w); serve → d.logf (с маркером ребра ложь→истина; ветка if fired daemon/metrics.go:82,
ДО тела). ЕДИНСТВЕННАЯ протяжка — расширить СВОБОДНУЮ функцию EvalMetricCondition (eval/
trigger_daemon.go:31) на +threshold value.Value во ВСЕХ ветках (не-success→None), один call-site
daemon/metrics.go:39; это НЕ метод ProcessRuntime → ProcessRuntime ОСТАЁТСЯ 8 байт-цел. Формат §C-5.3
дословно: <снимок>/<порог>=value.String (repr.go:20, БЕЗ подчёркиваний), <оп>=BinOp.String (op.go:35);
explain одностроч (не §13 двухстрочный — не ошибка; запись в Complexity Tracking). ОБЯЗАТЕЛЬНЫЙ
golden-churn co-land §C-5.5: обновить РОВНО 8 тестов в 4 файлах (tick_test.go ×2
PhaseOrderAllThreeFire/FourPhasesOrder; trigger_golden_test.go ×5 Fires/DBRepeatEphemeral/
MultiMetricOrder/MixedKindsOrder/BodyReadShadow; metric_window_golden_test.go ×1
TestWindowMetricTriggerFires — explain в out не w, stubs.Len()==0 цел) + GUARD «НЕ затронуты»
(count/contains metrics_test/schedule_test/daemon_test MFIRE/m2_endtoend; no-fire source_negatives/
WindowMetricTriggerSilent/events-FIFO A\nB\nC — НЕ ТРОГАТЬ). Новые замки TestRunTriggerExplain/
TestServeTriggerExplain (exact-match) + silence (тик уже-истина, нет новой строки) + inversion (не
протянуть порог→serve-строка неверна→краснит). ГРАНИЦЫ: дифф в eval(explain+EvalMetricCondition)+
daemon(call-site+serve logf)+перечисленные golden; inspect НЕ менять; ПУСТОЙ прод-дифф store/engine;
Store 18 / ProcessRuntime 8 целы; 0 новых KW/SE/eval-кодов/builtins/зависимостей; детерминизм. Якорь —
docs/reliability-model.md §C-5. Constitution 9/9 PASS. Предыдущая фича C4 021 — в
specs/021-unified-cli-clock/plan.md (фича 021-unified-cli-clock — M3 «Надёжность» пункт C4: единые
часы во всех путях CLI, развилка §8 «двойные часы». ОДИН engine.Clock протягивается в run/start/
complete/tasks/metric (сегодня — независимые реальные SystemClock в 2–3 точках на путь). Рецепт §C-4.2:
ВЫНЕСТИ evalClockFromEngine из serve.go:32-38 в общий cmd/ladix/clock_adapter.go (то же поведение,
serve не меняется); каждый builder принимает один clock engine.Clock (прод engine.SystemClock{});
interp := eval.NewInterpreter(out, depth, evalClockFromEngine{clock}); eng := engine.NewEngine(st,
interp, out, append([]engine.Option{engine.WithClock(clock)}, withExternalCallerOpt(caller)...)...);
сводки/строки-задач «сейчас» = clock.Now(). metric ДОБИТЬ engine.WithClock (латентный эффект — для
полноты). serve+emit НЕ ТРОГАТЬ (serve залочен serve_golden_test.go:216, fixedClock :21-23 компилится).
Монотонные часы НЕ делать (§C-9/§C-4.4). ГРАНИЦЫ: дифф СТРОГО в src/cmd/ladix/; ПУСТОЙ дифф
eval/engine/store/daemon (engine.Clock/WithClock сигнатуры байт-целы); Store 18 (ДВОЙНОЙ compile-замок)
/ ProcessRuntime 8 целы; 0 новых KW/SE/eval-кодов/builtins/зависимостей; детерминизм. Тест-замки §C-4.3:
FixedClock-инъекция в КАЖДЫЙ путь → детерминированный вывод; инверсия (возврат к реальным часам)
краснит; serve-unchanged регресс-гард. Якорь — docs/reliability-model.md §C-4. Constitution 9/9 PASS.
Предыдущая фича C2b 020 — в
specs/020-outbox-exactly-once/plan.md (фича 020-outbox-exactly-once — M3 «Надёжность» пункт C2b:
outbox-леджер идемпотентности + exactly-once доставка реального эффекта В ТЕЛЕ ШАГА процесса через
рестарт демона (POST ровно 1). Store 16→18 АДДИТИВНО (ДВОЙНОЙ compile-замок store.go:44-45):
+тип OutboxRecord (types.go), +sentinel ErrOutboxNotFound, +LoadOutbox/SaveOutbox в ОБЕИХ impl
(Memory map+глубокая копия Args/времён; SQLite SELECT/INSERT ON CONFLICT); таблица outbox уже создана
C2a. Кодек §C-2b.6: Args→encodeList(value.NewList(args)); Result→encodeValue; None→tagged-Пусто blob
НЕ SQL NULL; сериализация ВНУТРИ SQLiteStore (eval не импортирует store). Дедуп в effect-методах
движка (engine/runtime.go CallExternal/CallExternalResult/Notify, в КАЖДОМ из 3 независимо), активен
⟺ len(e.active)>0; ключ (inst.ID,CurrentStep,effectIndex); новое поле activeFrame.effectIndex (reset
в advance перед телом, инкремент на каждый эффект). Протокол D-C-9 deliver-then-record + pre-check:
LoadOutbox→если delivered вернуть сохранённый Result без доставки; иначе доставить, затем SaveOutbox;
зазор POST→SaveOutbox = осознанный at-least-once (§C-9). ProcessRuntime ОСТАЁТСЯ 8 (eval-дифф ПУСТ).
Гейт TestStepEffectExactlyOnceRestart (зеркало driveServeToNoRepeat, inline-const); 3 fault-теста
checkDeadlines (:38-41/:50-53/:63-65); codec round-trip; исполнимое усиление §2 = эволюция
examples/контроль_плана.ladix +2 авто-шага + MANIFEST + переснять main_test.go:137. ГРАНИЦЫ: дифф в
internal/store+engine+daemon(тесты)+examples+тесты; ПУСТОЙ дифф eval; cmd прод не трогать (serve
clock-путь цел); 0 новых KW/SE/eval-кодов/builtins/зависимостей; детерминизм FixedClock. Якорь —
docs/reliability-model.md §C-2b/§C-1. Constitution 9/9 PASS. Предыдущая фича C2a 019 — в
specs/019-store-schema-migrations/plan.md. Фича M-DX 012 — в
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
specs/002-parser-ast/plan.md; лексер 001 — в specs/001-lexer-tokens/plan.md)))).
<!-- SPECKIT END -->

## Контекст основного потока ≤20%

Держать заполнение основного контекста ≤20%. Тяжёлые чтение/поиск/анализ делегировать субагентам (Task); в основной поток возвращать только выводы, не сырьё. При росте к порогу — делегируй или сворачивай.
