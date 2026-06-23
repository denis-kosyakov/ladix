<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan:
specs/028-numeric-path-hardening/plan.md (фича 028-numeric-path-hardening — ХАРДЕНИНГ числовых путей
движка метрик БЕЗ изменения прод-поведения: тесты-онли запирание combineBinary/combineUnary + обоих
numberToValue ЧЕРЕЗ ДЕРИВАТИВ МЕТРИКИ + механический рейминг двух одноимённых numberToValue. Часть A
(тесты): A1 деление на ноль (evalDiv float / evalFloorDiv,evalMod на НЕПУСТОМ окне — D4-1 короткозамыкает
пустое в Пусто, потому знаменатель=разность агрегатов «количество(запись)-количество(запись)»=0); A2
переполнение (evalAdd «+9223372036854775807», evalSubMul «*MaxInt64», evalFloorDiv MinInt64//-1); A3 ЯДРО
combineUnary 0%-покрытия — кастом-фикстура поле=-9223372036854775808 (=MinInt64, в int64, грузится строгим
как Целое) → дериватив «-(мин(поле))»→переполнение, «-(среднее(сумма_заказа))»→-1000000.0; A4 NaN/±Inf —
кастом-фикстура поле=1e300 (конечно, строгий грузит) → «среднее*среднее»=+Inf, «(..*..)-(..*..)»=Inf-Inf=NaN,
ассерт math.IsInf/IsNaN НЕ ==; A5 None-операнд — метрика-с-пустым-окном даёт None, читается глобалью в
деривативе внешней непустой метрики → combineUnary default/combineBinary type-mismatch → ОшибкаТипа «к Пусто»;
A6 строгий sourceNumberToValue целое-вне-int64→§SM-9.B; A7 толерантный payloadNumberToValue 1e400→+Inf,-1e400→
-Inf,целое-вне-диап-без-точки→Дробное (НИКОГДА None). Часть B (рейминг): eval/source_loader.go метод
numberToValue→sourceNumberToValue (+вызов decodeValue:158); jsonval/decode.go функция numberToValue→
payloadNumberToValue (+вызов DecodeValue:81); +по 1 строке перекрёстной ссылки «двойник». ГРАНИЦЫ: дифф
СТРОГО в src/internal/eval (тесты+рейминг строгого) + src/internal/jsonval (тесты+рейминг толерантного) +
doc-sync §SM-9.B; ПУСТОЙ прод-функц.дифф combine*/arith/engine/store/daemon/cmd (FR-012/SC-005 байт-в-байт);
0 новых зависимостей (math stdlib уже в jsonval-тестах)/KW/builtins/eval-кодов/операторов; тексты ошибок
русские неизменны. Каждый замок краснеет при удалении гарда (мутстратегия в research.md/test-locks.md).
Constitution 9/9 PASS, Complexity Tracking ПУСТ (самая чистая фича: тесты+рейминг). plan/research/data-model/
contracts(test-locks)/quickstart СОЗДАНЫ, НЕ имплементировано — ждёт /speckit-tasks. Предыдущая фича 027 — в
specs/027-stable-trigger-keys/plan.md (фича 027-stable-trigger-keys — замена ПОЗИЦИОННОГО durable-ключа
метрика-/расписание-триггеров (triggerID(idx)="trg-<N>" по индексу в interp.Triggers()) на КОНТЕНТНЫЙ
ключ из условия триггера: "trg-"+hex16(FNV-1a-64(canonical(условие)+"#"+ord)). Устраняет тихую порчу
edge-baseline: вставка/перестановка/удаление несвязанного (в т.ч. событие-/дедлайн-) триггера больше не
сдвигает idx и не заставляет триггер унаследовать чужую строку trigger_state. (1) НОВЫЙ
src/internal/ast/canon.go (листовой пакет, импорт только fmt/strconv): CanonicalTriggerCondition(spec)
type-switch (*MetricTrigger→"metric|"+Metric.Name+"|"+Op.String()+"|"+canonExpr(Threshold);
*ScheduleTrigger→*EverySchedule "every|"+Amount+"|"+Unit / *AtSchedule "at|"+At.Value;
*EventTrigger/*DeadlineTrigger→"" не-ключевой) + ТОТАЛЬНЫЙ рекурсивный canonExpr по ВСЕМ 19 видам
Expression с ГРОМКИМ default-panic (инвариант «не должно случиться», Конституция III) — нет молчащего
схлопывания; числа нормализуются по разобранному значению (strconv.FormatInt/FormatFloat('g',-1,64);
10_000_000≡10000000), строки strconv.Quote. (2) НОВЫЙ ключ-билдер buildTriggerKeys([]*ast.TriggerDecl)
[]string в internal/daemon (keys.go/tick.go, импорт hash/fnv): группировка по канон.строке + 0-based ord
внутри группы дубликатов + FNV-1a-64 → массив выровнен по idx; ""-слоты событие/дедлайн не читаются.
НАХОДКА: конструктор демона New (daemon.go:37-49) УЖЕ принимает interp → triggerKeys []string новое поле
структуры Daemon (daemon.go:25-33), заполняется ВНУТРИ New из interp.Triggers() — сигнатура New НЕ
меняется, call-sites serve.go:326 + 4 теста НЕ трогаются, cmd/ladix ВНЕ диффа (рецепт допускал иначе —
опровергнуто кодом). metrics.go:38/schedule.go:47 triggerID(idx)→d.triggerKeys[idx]; старый triggerID
УДАЛЁН. (3) Миграция Store 2→3: schemaMigrations += "DELETE FROM trigger_state;" + currentSchemaVersion
2→3 (sqlite.go:82-84,106-122); INV-R1 init():91-97 1+2=3 двойной замок; DDL/тип TriggerState/контракт
Store ЦЕЛЫ (FR-007); переход = сброс+ленивый ре-прайминг (миграция видит только БД, не AST). (4) FR-010
(Complexity Tracking — СМЕНА поведения, санкц.спекой): checkAt (schedule.go:105-133) на первом промахе
miss=ErrTriggerStateNotFound && !now.Before(target) → Save{Kind:atKind,LastFiredDate:today}+return (прайм,
НЕ догонять тело); случай now<target НЕ трогается (штатно в target). Иначе сброс trigger_state дал бы
ложные catch-up-запуски. ГРАНИЦЫ: дифф СТРОГО в internal/ast(canon)+internal/daemon(билдер+2 call-site
+checkAt+поле/New+тесты)+internal/store(миграция+тест)+docs/SPEC; ПУСТОЙ функц.дифф eval/engine/cmd;
0 новых внешних зависимостей (hash/fnv/strconv/hash stdlib); 0 новых KW/builtins/eval-кодов. 9 ЗАМКОВ:
T1 исчерпываемость canonExpr (19 типов + stub→default-panic) / T2 канон.равенство parse→canon
(10_000_000≡10000000) / T3 различие / T4 дубликаты ord 0,1 / T5 🔁ЯДРО стабильность ключа метрики под
вставкой событие-триггера ПЕРЕД (инверсия: вернуть triggerID(idx)→краснеет; ревьюер проверит мутпробой)
/ T6 ре-прайминг при правке условия / T7 миграция v2-БД→user_version==3+trigger_state пуста (паттерн
store/migrate_test.go) / T8 🔁 нейтральность 1-го тика (метрика/каждые/в праймят НЕ срабатывают; инверсия
checkAt→краснеет) / T9 паритет Memory/SQLite eachStore. Маркер инверсии в докстрингах 🔁. Doc-sync
(implement-стадия, СИМВОЛЬНЫЕ ссылки на canonExpr/buildTriggerKeys, без захардкоженного trg-<N>): SPEC
FR-023+§C-9/§12 закрыт, engine-model §EM-17.2.1, trigger-/reliability-§C-9/automation-model. SC-008 греп
«нет triggerID/trg-%d в прод». Constitution 9/9 PASS (V — ключи поле демона НЕ пакетный var; III — громкий
default-panic; I — 0 новых зависимостей). plan/research/data-model/contracts(canon/trigger-keys/migration)/
quickstart СОЗДАНЫ, НЕ имплементировано — ждёт /speckit-tasks. Предыдущая фича 026 — в
specs/026-source-path-resolution/plan.md (фича 026-source-path-resolution — резолв относительных путей
файлов-источников от каталога .ladix-файла (file-relative) вместо cwd + CLI-флаг --source-base. Механизм
(eval): +поле Interpreter.sourceBase +сеттер SetSourceBase (зеркало SetProcessRuntime; NewInterpreter
сигнатура ЦЕЛА — 37 call-sites не тронуты, дефолт base="" ≡ cwd-резолв); метод resolveSourcePath
(filepath.IsAbs→как есть / filepath.Join(base,rel)) в 3 загрузчиках source_loader.go:68/239/319
(JSON/CSV/NDJSON); текст ошибки «источник '%s': файл «%s» не найден» показывает РЕЗОЛВЛЕННЫЙ путь
(FR-008, runtimeErr/ОшибкаВыполнения код/категория ЦЕЛЫ). CLI (cmd/ladix): флаг --source-base обе формы
(--flag val/--flag=val, дословное зеркало --db; пропуск значения → stderr «ladix: флаг --source-base
требует значение» exit 2) во ВСЕХ 5 подкомандах run/metric/complete (main.go) + start (start.go) + serve
(serve.go); база = sourceBaseDir(флаг ?? filepath.Dir(programPath)) → interp.SetSourceBase ПОСЛЕ
NewInterpreter; serve пробрасывает резолвленную базу serveFile→buildServeDaemon (+11 тест call-sites ""),
демон перечитывает источники по той же базе на тиках (ResetRunState НЕ сбрасывает sourceBase). git mv
data→examples/data (5 файлов; пути "data/..." в examples/*.ladix НЕ менялись — стали file-relative к
examples/data). ГРАНИЦЫ: дифф СТРОГО в src/internal/eval (interpreter.go+exports.go+source_loader.go) +
src/cmd/ladix (main/start/serve+тесты) + examples/ (git mv) + docs; ПУСТОЙ дифф internal/{store,engine,
daemon}; Store 18 цел; 0 новых зависимостей (path/filepath stdlib); 0 новых KW/builtins/eval-кодов.
Замки: TestResolveSourcePath (table-driven) / TestLoadSourceSalesJSON (examples/data+явный SetSourceBase) /
TestRunRevenueAbsolutePathFromTempDir (smoke t.TempDir абс.путь→exit0) / TestSourceBaseFlagOverride (обе
формы) / TestSourceBaseFlagMissingValue (exit2); withRepoRoot СНЯТ (14 call-sites → filepath.Abs(examplePath));
metric_engine_test.go salesPath()→examples/data; ассерт «не найден» под резолвленный путь; golden-байты
stdout НЕ изменены (строгая проверка мультимножества литералов); мутпроба (резолвер игнорит базу) краснит
TestResolveSourcePath/TestLoadSourceSalesJSON/TestSourceBaseFlagOverride. Doc-sync (источник истины
docs/source-metric-model.md §SM-8.1): SPEC §9.1/§9.7/§12, README «Примечание о путях»+флаг-таблица,
examples/MANIFEST, specs/004 помечен устаревшим; SC-004 (нет «cwd-relative»/«из корня репо»/«отложен в v2»).
go build/vet/test ./... зелёные. Constitution 9/9 PASS. Анализ-стадия поймала 2 MAJOR blast-radius git mv
(metric_engine_test salesPath + ассерт metric_test:185) — закрыты до implement. Коммиты: spec 892ffae →
plan 8a02746 → tasks f8c5f1c (+нит 21745ef) → feat e9827e7 → docs 5bed334, ветка 026-source-path-resolution
(НЕ смержена — ждёт ревью/мерж). Предыдущая фича C-IE 025 — в
specs/025-inbound-events/plan.md (фича 025-inbound-events — трек B «Входящие события»: HTTP-приём
событий по сети ВНУТРИ демона serve (входящая парность к исходящим эффектам M2/M3). Новый opt-in
флаг --listen host:port поднимает POST /events/{имя} → кладёт событие в durable-очередь events ОБЩИМ
хелпером минта (рефактор D-IE-8: вынести СВОБОДНУЮ enqueueEvent(st store.Store, name, payload string,
clock engine.Clock)(id,error) из emitEvent emit.go:58-85; ack-печать ВНЕ хелпера, тексты различны
НАМЕРЕННО: emit «поставлено в очередь» / HTTP «принято»). drainEvents/фронтенд/контракт Store НЕ
меняются — неразличимость источников ниже точки минта (FR-IE-3, имя кириллическое percent-кодир).
Хендлер eventsHandler(store.Store, engine.Clock, token) касается ТОЛЬКО Store+Clock, НИКОГДА Engine/
Interpreter (FR-IE-2, не потокобезопасны). Коды §IE-2 дословно: 202 «событие e-NNNNNN '<имя>' принято»
/ 400 пустое имя / 401 неверный токен (X-Ladix-Token, crypto/subtle, env LADIX_LISTEN_TOKEN) / 405
только POST / 500 сбой хранилища; битый JSON→202 (FR-IE-7). CLI §IE-3: парсинг --listen/--token
зеркало --interval serve.go:82; --listen без --db → exit 2 ДО net.Listen (FR-IE-4); net.Listen ВНЕ
guard рядом с SQLite serve.go:146-153 (bind-ошибка → exit 2, FR-IE-5); не-loopback без токена →
stderr-warn. Lifecycle §IE-5 stdlib-only (sync.WaitGroup+srv.Shutdown, БЕЗ errgroup): startEventListener
→go srv.Serve(ln); defer stop() ВНУТРИ guard-замыкания → LIFO Shutdown+wg.Wait СТРОГО ДО defer
sq.Close serve.go:152 (FR-IE-6 at-least-once, FR-IE-8 no-leak). ГРАНИЦЫ: дифф СТРОГО в src/cmd/ladix/
(emit.go рефактор + serve.go + НОВЫЙ events_http.go + тесты); ПУСТОЙ дифф internal/{store,engine,
daemon,eval}; Store 18 цел; 0 новых зависимостей (go.mod = только modernc.org/sqlite; net/http stdlib);
нулевой регресс serve без --listen — барьеры daemon_test.go:15-47 + serve_golden_test.go:361-371
(NumGoroutine after≤before) зелёные НЕТРОНУТЫ; детерминизм fixedClock (serve_golden_test.go:21-23).
Замки FR-IE-1..11 httptest симметрично webhook_cli_test.go. Якорь — docs/inbound-events-model.md
§IE-0..§IE-8 (D-IE-1..D-IE-10). doc-sync канонов (execution-model EM-17.10/SPEC §12/README §99/
automation-model/trigger-model) — спарринг-чат ПОСЛЕ мержа, НЕ в этой ветке. Constitution 9/9 PASS.
Предыдущая фича C5 022 — в
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
specs/002-parser-ast/plan.md; лексер 001 — в specs/001-lexer-tokens/plan.md))))).
<!-- SPECKIT END -->

## Контекст основного потока ≤20%

Держать заполнение основного контекста ≤20%. Тяжёлые чтение/поиск/анализ делегировать субагентам (Task); в основной поток возвращать только выводы, не сырьё. При росте к порогу — делегируй или сворачивай.
