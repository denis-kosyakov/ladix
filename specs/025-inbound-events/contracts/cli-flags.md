# Контракт: CLI-флаги приёмника (`serve --listen/--token`)

Источник: якорь §IE-3, §IE-5. Парсинг — зеркало `--interval` (serve.go:82), формы `--x v` и `--x=v`.

## Синопсис
```
ladix serve [--db путь] [--interval D] [--max-depth N] [--webhook URL]
            [--listen host:port] [--token СЕКРЕТ] <файл>
```

## `--listen host:port` (opt-in)
- Без флага → `ln == nil` → сетевой сервер НЕ стартует; serve байт-идентичен текущему (**FR-IE-1**).
- `--listen` без `--db` → **exit 2**, stderr `ladix: --listen требует --db`. Проверка в `serveMain` ДО `net.Listen` (**FR-IE-4**, R8).
- `net.Listen("tcp", listen)` — вне guard, рядом с открытием SQLite (serve.go:146-153). Bind-ошибка → **exit 2**, stderr `ladix: не удалось открыть сокет '<host:port>': <err>` (**FR-IE-5**, R5).
- Открытый `ln` передаётся в `startEventListener`/`srv.Serve(ln)`.

## `--token СЕКРЕТ` (опц.)
- Если `--token` не задан → fallback env `LADIX_LISTEN_TOKEN` (флаг бьёт env).
- Пусто → auth выключен. Непусто → хендлер сверяет `X-Ladix-Token` (constant-time).

## Дефолт-граница loopback (R7)
- host из `--listen` не loopback (`127.0.0.1`/`::1`/`localhost`; пустой host = все интерфейсы) ∧ токен пуст → stderr-предупреждение `ladix: ВНИМАНИЕ: --listen на не-loopback адресе без --token — эндпоинт запускает процессы без аутентификации`. НЕ блокирует.

## Жизненный цикл (FR-IE-6/8, D-IE-10)
- `stop := startEventListener(ln, st, clock, token)`; `defer stop()` ВНУТРИ guard-замыкания.
- LIFO: guard-замыкание возвращается → `stop()` (Shutdown+wg.Wait) → `guard()` возвращает → `defer sq.Close()` (serve.go:152). ⇒ Shutdown+join строго ДО Close.
- Часы `clock` — те же, что у демона (прод `engine.SystemClock{}`).

## Замки-барьеры (НЕ ломать)
- `daemon_test.go:15-47`, `serve_golden_test.go:310-330/361-371` (NumGoroutine after≤before) — без `--listen` остаются зелёными нетронутыми.
- `grep require src/go.mod` → только `modernc.org/sqlite` (0 новых зависимостей).
