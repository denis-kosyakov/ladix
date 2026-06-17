# Contract: Task.Escalated + 4 точки SQLite-кодека (B4b)

Источник §AU-2. Группа: **B4b-бэкенд**. Store-контракт остаётся 15 методов (колонка, не метод).

## Поле

`internal/store/types.go` — `Task` +поле (после `CompletedAt`):
```go
Escalated bool   // НОВОЕ: durable, одноразово (D-AU-5)
```

## 4 точки кодека (ВСЕ обязательны — пропуск любой = молчаливая потеря на рестарте)

| # | Точка | sqlite.go | Правка |
|---|-------|-----------|--------|
| 1 | DDL `tasks` | `:33` | +`escalated INTEGER NOT NULL DEFAULT 0` |
| 2 | `SaveTask` INSERT-список | `:161` | +колонка `escalated` + значение `boolToInt(t.Escalated)` |
| 3 | `ON CONFLICT(id) DO UPDATE SET …` | `:165` | +`escalated = excluded.escalated` (UPSERT персистит флаг) |
| 4 | SELECT-читатели `buildTask`/`scanTask` | `:296` / `:310` | +`escalated` в SELECT (`LoadTask :179`, `ListPendingTasks :186`) + параметр в сигнатуру `buildTask`/`scanTask` + парс `int→bool` |

`MemoryStore.copyTask` (`memory.go`) — `cp := *t` несёт bool тривиально. `UserTasks`
(`engine/runtime.go`) делегирует `ListPendingTasks` → наследует `Escalated`.

Миграция = сброс схемы (D-AU-9, БД тестовые) — пересоздать, НЕ ALTER.

## Что ломает пропуск каждой точки (мутпробы)

| Пропуск | Симптом |
|---------|---------|
| Точка 1 | INSERT с колонкой `escalated` → SQL-ошибка «no such column» |
| Точка 2 | новые задачи без `escalated` в INSERT → DEFAULT 0, флаг не пишется при создании (ок), но при upsert новой строки потеряется |
| Точка 3 | `Escalated=true; SaveTask` существующей задачи → UPSERT не обновляет `escalated` → на рестарте `false` → повтор |
| Точка 4 | `escalated` пишется, но НЕ читается → `ListPendingTasks` всегда `Escalated=false` → скан после рестарта эскалирует повторно |

## Тесты (B4b-группа, tests-first)

- Round-trip: `SaveTask{Escalated:true}` → новый `SQLiteStore` на той же `--db` →
  `ListPendingTasks`/`LoadTask` → `Escalated==true`.
- UPSERT: `SaveTask{Escalated:false}`, затем `SaveTask{Escalated:true}` (тот же ID) →
  перечитать → `Escalated==true` (точка 3 живёт).
- `MemoryStore`: `copyTask` несёт `Escalated`.
- Замок Store=15 методов: интерфейс `Store` не растёт (`ListTasksByInstance` — НЕ здесь, B6).
- Инверсия (точка 3): убрать `escalated` из ON CONFLICT → UPSERT-тест красный.
- Инверсия (точка 4): убрать `escalated` из SELECT/scanTask → round-trip даёт `false` → красный.
- Эти точки сводятся в durable-golden (`durable-restart.md`): пропуск любой → durable красный.
