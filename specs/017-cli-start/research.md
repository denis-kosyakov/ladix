# Research — B5 `ladix start` (Phase 0)

Эмпирика прецедентов из `src/` (HEAD master @a1ad856). Все цитаты выверены grep'ом.

## R-1. `engine.Start` — сигнатура и поведение

`internal/engine/engine.go:65`:
```go
func (e *Engine) Start(name string, args []value.Value) (string, error) {
    pd, ok := e.interp.Process(name)
    if !ok {
        return "", fmt.Errorf("процесс '%s' не найден в определении", name)   // :69 — ДРУГОЙ текст!
    }
    id, err := e.st.NextInstanceID()      // p-000001, p-000002, … (store contract_test:148)
    ...
}
```
**Вывод:** `Start` возвращает `(instanceID, error)`; на неизвестном процессе даёт текст
`процесс '%s' не найден в определении` — это НЕ §AU-10-текст. Поэтому `startMain` обязан проверить
`interp.Process` и арность САМ ДО `Start` (plan Р-2, FR-010/011).

`Start→advance` печатает `[задача]`-строки через `printTaskCreated` (engine.go:319, :461):
```go
fmt.Fprintf(e.out, "[задача] %s → %s, шаг '%s', срок до %s\n",
    t.ID, t.Assignee, t.StepName, t.Deadline.Format(deadlineLayout))  // deadlineLayout = "2006-01-02 15:04" (format.go:11)
```
Без дедлайна — короткая форма `[задача] %s → %s, шаг '%s'`. `Start` НЕ печатает строку статуса инстанса
(её печатает `advanceAfterComplete`, не `Start`). Значит `запущен инстанс <id>` печатает САМ `startMain`
(§AU-10.D, FR-012/018).

## R-2. `interp.Process` — резолв процесса и арность

`internal/eval/exports.go:19`:
```go
func (i *Interpreter) Process(name string) (*ast.ProcessDecl, bool)
```
`ast/process.go:10`: `ProcessDecl.Params []Ident` → арность = `len(pd.Params)`.
**Вывод:** `pd, ok := interp.Process(name)`; `!ok` → FR-015; `len(pd.Params) != len(argv)` → FR-014.

## R-3. M1-парс даты — образец для argv-Дата

`internal/eval/source_loader.go:393,512,523`: Дата строится `parseISODate`; не-Строка в Дата-поле → A1-10
mismatch. `value.Дата{Year, Month, Day}` (`value/deferred.go:10`).
**Вывод:** argv формы `^\d{4}-\d{2}-\d{2}$` → `value.Дата` тем же подходом (валидировать календарь как M1:
`2026-13-45` → ошибка парса литерала, не Строка). impl переиспользует/зеркалит `parseISODate`-логику или
`time.Parse("2006-01-02", …)` с проверкой; точная функция — решение impl, единообразно M1.

## R-4. BOOL/NONE литералы лексера — образец для argv

`internal/lexer/scan_ident.go:26-33`:
```go
case "истина": l.emit(BOOL, word, pos, true)
case "ложь":   l.emit(BOOL, word, pos, false)
case "пусто":  l.emit(NONE, word, pos, nil)
```
**Вывод:** argv `истина`/`ложь` → `value.Булево{true/false}`; `пусто` → `value.None` (§AU-7.2, FR-008).
Те же три слова — единообразие CLI с языком.

## R-5. CLI-флаги и проводка вебхука (прецедент B2/complete)

- `usage`-строка (main.go:71) перечисляет все подкоманды с флагами — `start` нужно ДОБАВИТЬ туда.
- switch подкоманд (main.go) — `run/metric/complete/tasks/serve/emit`; +`case "start"`.
- `--db` парсинг: `a == "--db"` / `strings.HasPrefix(a, "--db=")` (emit.go:21-26); дефолт
  `defaultDBPath="ladix.db"` (main.go:101) для семьи complete/tasks/emit.
- `--max-depth` парсинг: main.go:183-200.
- `--вебхук`: `parseWebhookCaller(raw)` (main.go:39-54) уже формирует `(caller, error)`; пустой URL →
  `(nil, nil)` (дефолт-стаб); невалидный → `fmt.Errorf("неверный URL вебхука '%s'", raw)` (main.go:52);
  `withExternalCallerOpt(caller)` (main.go:60) → `[]engine.Option`.
**Вывод:** `start` переиспользует `parseWebhookCaller` + `withExternalCallerOpt` БЕЗ дублирования (Р-1).
Env `LADIX_WEBHOOK` читается внутри `parseWebhookCaller` (main.go:46) — паритет.

## R-6. Конструкция Store (5 инлайн-дублей → openStore)

Инлайн-конструкция `dbPath != "" → SQLite defer Close, иначе Memory`:
- runFile `main.go:235-244`,
- serve `serve.go` (§AU-9),
- complete `main.go:435`,
- tasks `main.go:539`,
- emit `emit.go:59`.
**Вывод:** выделить `openStore(dbPath) (store.Store, func() error, error)` (Р-3 плана), переиспользовать
в `start`. Рефактор остальных под хелпер — опционален, ТОЛЬКО при зелёном golden (регресс-замок US4-3).

## R-7. Golden-маскирование (прецедент)

`cmd/ladix/trigger_golden_test.go:70`: `idMaskRE = regexp.MustCompile([pt]-\d{6})` → `<ID>`;
дедлайн в golden маскируется как `<DT>` (main_test.go:249 `срок до <DT>`).
**Вывод:** start-golden маскирует время дедлайна `<DT>`; id детерминированы при свежей БД
(p-000001/t-000001) — можно сверять буквально либо маскировать `<ID>`. Решение: для канона §AU-10.D
оставить id буквальными (p-000001) на свежей БД + маскировать только `<DT>` (как main_test 4).

## R-8. Подтверждение инвариантов (счётчики)

- `ProcessRuntime` = 8 методов (после B1) — B5 НЕ трогает (CLI потребляет `engine.Start`).
- `Store` = 16 (после B6) — B5 НЕ добавляет метод.
- Каталог: L=11 (`lexer/lexerrors_test.go`), SE=14 (`parser/inventory_test.go`), eval=28
  (`errors_golden_test.go`) — B5 НЕ инкрементит (CLI-ошибки — stderr cmd, §EN-8.B).
- 0 новых зависимостей.

## R-9. Открытые вопросы → закрыты якорем

| Вопрос | Решение | Источник |
|--------|---------|----------|
| Вебхук в start? | ДА (паритет complete) | §AU-4.5, plan Р-1 |
| Кто проверяет арность? | startMain ДО engine.Start | §AU-7.3, plan Р-2 |
| Дефолт Store start? | SQLite ladix.db (семья complete) | D-AU-10/§AU-9 |
| Невалидная ISO-дата argv? | ошибка парса литерала (как M1) | §AU-7.2/M1, R-3 |
| stdout канон? | §AU-10.D дословно | §AU-10.D |
