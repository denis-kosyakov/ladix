# Самопроверка против функционального контракта §1–§9 — Ladix v1 (фича 009-v1-finalization)

Финальная приёмочная самопроверка против функционального контракта §1–§9 (§1 Концепция … §9 Тесты/примеры/доки),
выполнена ПОСЛЕ финализации 009. Проверка честная — по реальному коду / тестам / докам / примерам.
Гейт зафиксирован: `cd src && go build ./... && go vet ./... && go test ./... -count=1` →
build/vet PASS, **10/10 пакетов `ok`, ноль «no test files»**, exit 0 (T038). Чистый клон по
README дословно из корня → build/run/test все exit 0, `hello` → `Привет, Уклад!` (T039).

| Раздел | Статус | Доказательство (файл:строка / пример / тест) |
|---|---|---|
| §1 Концепция | ✓ полное покрытие | `README.md:1–9` (название + идея: декларатив/императив/процессы/триггеры, не GP-язык) · `SPEC.md:9` §1 Концепция · `CHECKLIST.md:7–8` |
| §2 Лексика / синтаксис / грамматика | ✓ полное покрытие | Лексер `src/internal/lexer/lexer.go` + 12 тест-файлов; парсер `src/internal/parser/parser.go` + приоритет `parse_expr.go:11`; грамматика `SPEC.md:80` §3 + `docs/grammar.md`; компиляция витрины `src/internal/parser/examples_test.go` (`TestExamplesParseCleanSet`) зелёный · `CHECKLIST.md:12,20,21,34` |
| §3 Семантика / модель выполнения | ✓ полное покрытие | Интерпретатор `src/internal/eval/interpreter.go`; области/затенение `SPEC.md:243` §6; модель исполнения `docs/eval-model.md` + `docs/execution-model.md`; движок процессов `src/internal/engine/` + `docs/process-model.md`; пакеты `eval`(24 теста)/`engine`/`store`/`daemon` все `ok` · `CHECKLIST.md:23` |
| §4 Типы данных | ✓ полное покрытие | Типы-значения `src/internal/value/` (5 тест-файлов); описание `SPEC.md:148` §4; маппинг JSON→Ladix `SPEC.md:390` §9.3; `тип(x)` честно помечен «зарезервировано v1» (FR-018, `src/internal/eval/builtins.go:56`) · `CHECKLIST.md:13` |
| §5 Управляющие конструкции | ✓ полное покрытие | Условие `src/internal/eval/stmt.go:174`; цикл `:48`; блоки `:159`; прервать/продолжить/вернуть `SPEC.md:296` §7.3; примеры `examples/условие.ladix`, `examples/цикл.ladix`; тесты `src/internal/eval/stmt_test.go:68,103` · `CHECKLIST.md:35–37` |
| §6 Функции / области | ✓ полное покрытие | Функции `src/internal/eval/call.go:49`; параметры `:67`; возврат/локали `:49,66`; рекурсия `examples/факториал.ladix`; область-барьер `SPEC.md:243` §6; тесты `src/internal/eval/call_test.go:6` · `CHECKLIST.md:38–41` |
| §7 Ошибки / диагностика | ✓ полное покрытие | Синтаксис `src/internal/errors/parserror.go:27`; строка/колонка `position.go:11`; типы `typeerror.go:19`; неизвестная переменная `src/internal/eval/expr.go:113`; recover-барьер без stack trace `src/cmd/ladix/main.go:477` + `SPEC.md:553` §13; негатив-замки `src/cmd/ladix/golden_test.go` (3 класса ошибок); тесты `src/internal/errors/evalerrors_test.go:11` · `CHECKLIST.md:46–50` |
| §8 Реализация | ✓ полное покрытие | Полный собственный фронтенд lexer→parser→AST→eval; CLI `src/cmd/ladix/main.go` (`run`/`serve`/`emit`/`metric`); SQLite-стор `src/internal/store/` (чистый Go, без CGO); структура `docs/STRUCTURE.md`; гейт build/vet PASS, 10/10 `ok` (T038); README дословно воспроизводим из чистого клона (T039) · `CHECKLIST.md:20–25` |
| §9 Тесты / примеры / доки | ✓ полное покрытие | ~93 `_test.go` по 10 пакетам (`eval` 24, `lexer` 12, `cmd/ladix` 10 golden/smoke …); 16 примеров `examples/*.ladix` (7 классов витрины 009 + база + 3 негатива) под golden/негатив-замками + `examples/MANIFEST.md`; доки `README`/`SPEC`/`ARCHITECTURE` + `docs/*.md` (12 файлов) + `docs/quickstart.md`; CHECKLIST 50/50 `[x]` · `CHECKLIST.md:64–71` |
| **Итого §1–§9** | **✓ полное покрытие** | Все §1–§9 закрыты; каждый с доказательством файл:строка / зелёным замком |

## Дополнительные возможности — ОСОЗНАННО ВНЕ СКОУПА

Дополнительные возможности сверх функционального контракта в задаче 009 явно вне скоупа — отмечены как осознанно
не делавшиеся (0 строк новой языковой функциональности — природа фичи документно-приёмочная):

- Операторная арифметика дат/длительностей (`Дата±Длит`, `Дата−Дата`, `Длит*X`) — оставлена
  **deferred v2** (D-A=A, фича 008 §DB; `docs/datetime-builtins-model.md`). Не делалось осознанно.
- Активация `тип(x)` — **зарезервировано v1** (FR-018); функция зарегистрирована, но синтаксически
  недостижима (reserved-word); 34 из 35 builtins достижимы (`src/internal/eval/builtins.go:56`).
  Не активировалось осознанно.
- Внешний бэкенд исполнения процессов (Camunda/Kestra) — синтаксис/UX рассчитаны на подмену,
  но в v1 встроенный движок (`README.md:7`). Не делалось осознанно.

## Агрегация data-model (восемь артефактов) — ЗАКРЫТО

Сводка статусов приёмочных артефактов фичи 009 (`specs/009-v1-finalization/data-model.md`):
воспроизводимость README (US1, smoke-замок `reproducibility_smoke_test.go`), CHECKLIST 50/50
(US2, grep `^- \[ \]`=0 / `^- \[x\]`=50), витрина 7 классов (US3, golden/негатив-замки +
`examples_test.go` зелёный), quickstart (US4, `quickstart_smoke_test.go`), выравнивание доков
(US5, grep-инвариант A1–A4=0, `aggregate_test.go` зелёный, `тип(x)` reserved), финальный гейт
(US6 T038 10/10), чистый клон (US6 T039), чистое дерево (US6 T040) — **все ЗАКРЫТО**.
