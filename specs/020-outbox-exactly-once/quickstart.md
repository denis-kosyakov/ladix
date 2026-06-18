# Quickstart: C2b — Outbox exactly-once

Как проверить фичу руками и какие сценарии она закрывает. Сборка: `cd /Users/denis/dev/ladix/src && go build ./...`.

## Сценарий 1 — Durable exactly-once гейт (P1, главный)

Зеркало `driveServeToNoRepeat` (`cmd/ladix/m2_golden_test.go:234`) / `TestDeadlineDurableRestart`. Inline-const источник (изолирован от файловых golden).

1. Прогнать усиленный §2 end-to-end под `serve` на durable `--db`: CSV-источник → метрика → триггер → процесс `эскалация_плана` → человеческий шаг `связаться_с_клиентом` → `complete --данные '{"итог":"перезвонит"}'` → авто-шаг `зафиксировать_итог` (`присвоить итог = данные.итог`, durable) → авто-шаг `уведомить_crm` (`уведомить crm("итог звонка: " + итог)` — реальный POST).
2. На авто-шаге `уведомить_crm` эффект доставлен: **POST = 1**, `SaveOutbox(delivered=1)`.
3. Краш демона mid-advance (после SaveOutbox). Открыть НОВЫЙ Store на той же `--db`, `RunRestartScan` → реактивация → `advance(пустой payload)` → тело `уведомить_crm` переисполнено.
4. Pre-check `LoadOutbox(key)` видит `Delivered=true` → доставка ПРОПУЩЕНА.
5. **Ожидание: счётчик POST остался 1** (exactly-once).

Замок: `TestStepEffectExactlyOnceRestart`. Мутпроба: снять pre-check → POST×2 → красный.

## Сценарий 2 — Исполнимое усиление §2 (демо-пример)

Эволюция `examples/контроль_плана.ladix` (+2 авто-шага; текст уже в `docs/v2-charter.md` §2, не редактируем):

```ladix
процесс эскалация_плана(текущая_выручка):
    шаг связаться_с_клиентом:
        исполнитель: "менеджер"
        срок:        2дн
        присвоить факт = текущая_выручка
    шаг зафиксировать_итог после связаться_с_клиентом:   # авто-шаг: захват payload → durable
        присвоить итог = данные.итог
    шаг уведомить_crm после зафиксировать_итог:          # авто-шаг: реальный эффект
        уведомить crm("итог звонка: " + итог)
```

- Под `run` эскалация — заглушка (исполняет `serve`); тело авто-шагов исполняется под durable-прогоном.
- Обновить `examples/MANIFEST.md:151` (запись `контроль_плана.ladix`).
- Переснять golden: `cmd/ladix/main_test.go:137` (`TestCLIGoldenDeadlineEscalation`, +строка эффекта `crm`).
- Арность процесса не меняется (один параметр) → `start_golden_test.go:46` не затронут.

Почему split, а не одношаговая форма: payload `--данные` эфемерен (на рестарте пуст); `присвоить итог = данные.итог` durable-захват → эффект-шаг читает durable `итог` → арг-эвал на рестарте успешен → эффект переисполняется → outbox глушит. Одношаговая форма на рестарте: пустой payload → `Строка + Пусто` typeError → шаг провален → POST=0.

## Сценарий 3 — checkDeadlines устойчив к fault'ам (P2)

Инъекция fault-Store (`internal/daemon/checkdeadlines_fault_test.go`):

1. `ListPendingTasks` падает → лог `checkDeadlines: листинг задач: …` + ранний return, демон жив.
2. `LoadInstance` падает для одной задачи → задача пропущена (continue), остальные обработаны, нет паники.
3. `SaveTask`(Escalated) падает после fire → лог `checkDeadlines: персист Escalated задачи …`; known window fire-then-persist (комментарий), at-least-once.

## Проверки инвариантов (M3-гейт)

```sh
cd /Users/denis/dev/ladix/src
go build ./...                                   # двойной compile-замок Store зелёный
go vet ./...                                     # без замечаний
go test ./...                                    # все замки + мутпробы краснят при снятии гарантий
grep -rn 'ProcessRuntime' internal/eval/runtime.go   # ровно 8 методов
git diff --stat -- internal/eval                 # ПУСТО (eval не тронут)
```

| Инвариант | Ожидание |
|---|---|
| Store | 18 методов, двойной замок, базовые 16 байт-целы |
| ProcessRuntime | 8 (без изменений) |
| eval-дифф | пустой |
| Новые KW/SE/eval-коды/builtins/зависимости | 0 |
| Детерминизм | FixedClock во всех новых тестах |
| POST на durable-рестарте §2 | ровно 1 |
