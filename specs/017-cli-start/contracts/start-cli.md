# Contract — `ladix start` CLI (B5)

Источник: §AU-7, §AU-9, §AU-10.C/§AU-10.D, §AU-4.5, D-AU-7, D-AU-10. Все тексты — дословно.

## Грамматика вызова

```
ladix start <файл> <процесс> [аргументы...] [--db путь] [--вебхук URL] [--max-depth N]
```
- `<файл>` — первый позиционный: исходник `.ladix` с определениями (как run/complete).
- `<процесс>` — второй позиционный: имя объявленного процесса.
- `[аргументы...]` — остальные позиционные: значения параметров (типизируются, см. data-model §1).
- `--db путь` — путь к SQLite (дефолт `ladix.db`, D-AU-10; SQLite, НЕ Memory).
- `--вебхук URL` / env `LADIX_WEBHOOK` — HTTP-драйвер внешних эффектов (§AU-4.5).
- `--max-depth N` — глубина рекурсии (паритет run/complete).

## Диспетчеризация

`cmd/ladix/main.go` switch: `case "start": return startMain(args[1:], stdout, stderr)`.
`usage`-строка (main.go:71) РАСШИРЯЕТСЯ записью start:
`ladix start [--db путь] [--вебхук URL] [--max-depth N] <файл> <процесс> [аргументы...]`.

## Поведение (happy path)

1. Разбор флагов + позиционных. Нет `<файл>`/`<процесс>` → usage exit 2.
2. Компиляция `<файл>` (lex→parse→Analyze). Ошибка компиляции → диагностика exit 1 (как complete).
3. `parseArgLiteral` на каждый позиционный аргумент → `[]value.Value` (data-model §1). Ошибка → exit 2.
4. `openStore(dbPath)` → SQLite `ladix.db` по умолчанию (defer close).
5. `parseWebhookCaller(webhook)` → caller (или nil = стаб). Невалидный URL → exit 2 (ERR-WEBHOOK).
6. Сборка стека: `NewInterpreter` → `Analyze(prog)` → `NewEngine(st, interp, stdout, withExternalCallerOpt(caller)...)`
   → `SetProcessRuntime(eng)`.
7. `interp.Process(name)`: `!ok` → ERR-UNDECL exit 2. Иначе `len(pd.Params)` vs `len(args)`: mismatch →
   ERR-ARITY exit 2. (ОБЕ проверки ДО `eng.Start`.)
8. `id, err := eng.Start(name, args)`. Движок печатает `[задача]`-строки сам. err → CLI-маппинг exit 2/1.
9. `startMain` печатает `запущен инстанс <id>`. exit 0.

## stdout канон (§AU-10.D, exact-match; golden маскирует `<время>`→`<DT>`)

Процесс с человеческим первым шагом + дедлайн:
```
[задача] t-000001 → менеджер, шаг 'связаться_с_клиентом', срок до <время>
запущен инстанс p-000001
```
Терминальный процесс без задач:
```
запущен инстанс p-000001
```

## CLI-ошибки (§AU-10.C — stderr, exit 2, ДОСЛОВНО)

| Триггер | Текст |
|---------|-------|
| неизвестный процесс | `ladix: процесс '<имя>' не объявлен` |
| арность mismatch | `ladix: процесс '<имя>' ожидает <N> аргументов, получено <M>` |
| целое вне диапазона | `ladix: не удалось разобрать аргумент '<argv>': целое вне диапазона типа Целое` |
| невалидный URL вебхука | `ladix: неверный URL вебхука '<URL>'` |
| флаг без значения | `ladix: флаг --<имя> требует значение` (паритет существующих команд) |
| неизвестный флаг | `ladix: неизвестный флаг <флаг>` (паритет) |

## Exit-коды

| Код | Условие |
|-----|---------|
| 0 | успешный старт инстанса |
| 1 | ошибка компиляции файла (диагностика двухстрочного канона §13) |
| 2 | CLI-ошибка (арность/неизв.процесс/плохой литерал/URL/флаг/usage) |

## Инварианты контракта

- `ProcessRuntime` НЕ меняется (=8); Store НЕ получает метод (start потребляет `engine.Start`). INV-1.
- Каталог диагностик L=11/SE=14/eval=28 НЕ меняется (CLI-ошибки — stderr cmd). INV-3.
- Дефолт Store = SQLite `ladix.db` (D-AU-10), НЕ Memory. INV-2.
- 0 новых зависимостей; детерминизм id/времени для golden. SC-006/007.
