# Contract: checkDeadlines — 3 реальных fault-ветки

Источник: §C-2b.8. Точка: `internal/daemon/checkdeadlines.go:22` (функция корректна; 3 fault-ветки не покрыты). Метод — реальные тесты надёжности через инъекцию fault-Store. Нет файла `*fault*` в `internal/daemon` сегодня → создаётся `checkdeadlines_fault_test.go`.

## Ветка 1 — `ListPendingTasks` error (`:38-41`)

| Аспект | Контракт |
|---|---|
| Триггер | `ListPendingTasks` возвращает ошибку |
| Поведение | лог `"checkDeadlines: листинг задач: %s"` + ранний `return`; демон жив |
| Тест | fault-Store с падающим `ListPendingTasks` → фаза НЕ паникует, лог-строка присутствует, тик продолжается (следующий тик идёт) |

## Ветка 2 — `LoadInstance` error (`:50-53`)

| Аспект | Контракт |
|---|---|
| Триггер | `LoadInstance` падает для одной задачи |
| Поведение | `continue` — задача пропущена; прочие обрабатываются |
| Тест | fault-Store, `LoadInstance` падает для одной задачи → нет эскалации этой задачи, нет паники, остальные задачи обработаны |

## Ветка 3 — `SaveTask`(Escalated) error (`:63-65`)

| Аспект | Контракт |
|---|---|
| Триггер | `SaveTask` (персист `Escalated`) падает ПОСЛЕ срабатывания тела эскалации (POST уже отправлен) |
| Поведение | лог `"checkDeadlines: персист Escalated задачи %s: %s"`; следующий тик/рестарт ПЕРЕШЛЁТ = at-least-once |
| Тест | fault-Store, `SaveTask` падает после fire → лог-строка присутствует; **в комментарии теста зафиксировать**, что это известное окно fire-then-persist (пара к §C-2b.5 dispatch-зазору / §C-9 бэклог), НЕ дефект |

## Общие инварианты тестов

- Детерминизм: `FixedClock`/`fixedClock`.
- Нет паники ни в одной ветке (Принцип III — recover-границы не нужны, штатные пути не паникуют).
- Лог-строки на русском, дословно из §C-2b.8 (Принцип VIII).
- fault-Store — ручная обёртка над существующим Store (или `MemoryStore`) с инъекцией ошибки в один метод; 0 новых зависимостей (без mock-библиотек).

## Контрактные тесты (замки)

- `TestCheckDeadlinesListError` (ветка 1).
- `TestCheckDeadlinesLoadInstanceError` (ветка 2).
- `TestCheckDeadlinesSaveTaskError` (ветка 3, с комментарием known-window).
