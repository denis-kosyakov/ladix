# Контракт: HTTP-эндпоинт приёма (`POST /events/{имя}`)

Источник: якорь §IE-2, §IE-4, §IE-6. Тексты тел — дословно RU, всегда с завершающим `\n`.

## Запрос
```
POST /events/{имя}
[X-Ladix-Token: СЕКРЕТ]   ← только если демон запущен с --token
Тело: сырой JSON-payload как есть (или пусто)
```
- `{имя}` — `strings.TrimPrefix(r.URL.Path, "/events/")`; `r.URL.Path` уже percent-декодирован `net/http`. Матч строго байтовый (без NFC).
- Тело — `io.ReadAll(r.Body)` → строка → `PayloadJSON` без парсинга.

## Ответы (код + тело, golden)

| Код | Условие | Тело (`\n` в конце) |
|-----|---------|----------------------|
| `202 Accepted` | enqueue успешен | `событие e-NNNNNN '<имя>' принято` |
| `400 Bad Request` | пустое имя (`POST /events/`) | `ladix: пустое имя события` |
| `401 Unauthorized` | задан токен, заголовок не совпал/отсутствует | `ladix: неверный токен` |
| `405 Method Not Allowed` | метод не POST | `ladix: метод не поддерживается, только POST` |
| `500 Internal Server Error` | сбой `Store` при enqueue | `ladix: сбой хранилища` |

## Порядок проверок (R2)
1. метод ≠ POST → 405
2. токен задан ∧ `subtle.ConstantTimeCompare([]byte(header), []byte(token)) != 1` → 401
3. имя пусто → 400
4. enqueue err → 500
5. ok → 202 + `Fprintf(w, "событие %s '%s' принято\n", id, name)`

## Инварианты
- **FR-IE-2**: сигнатура `eventsHandler(store.Store, engine.Clock, string)` — НИКАКОГО `*engine.Engine`/интерпретатора (статический замок изоляции).
- **FR-IE-6**: `202` строго ПОСЛЕ успешного `EnqueueEvent`; сбой → `500`, событие не теряется молча.
- **FR-IE-7**: битый JSON → `202` (не валидируем на приёме).
- **FR-IE-9**: дефолт (токен пуст) → auth пропущен; любой POST → 202.
- Тела `202` («принято») ≠ ack `emit` («поставлено в очередь») — намеренно (D-IE-8).
