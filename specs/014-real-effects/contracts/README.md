# Contracts: B2 «Реальные эффекты `вызвать` / `уведомить` через HTTP-вебхук»

**Feature**: 014-real-effects | **Источник**: `docs/automation-model.md` §AU-4 (D-AU-2)

Контракты B2 — четыре среза несущих границ, затронутых фичей. Все аддитивны; шов `ProcessRuntime` НЕ расширяется (B1 уже дал 8-й метод).

| Файл | Граница | Суть |
|---|---|---|
| [`external-caller.md`](./external-caller.md) | Драйвер + Option движка | интерфейс `ExternalCaller`, `WithExternalCaller`, дефолт `printCaller`, делегирование методов движка |
| [`webhook-wire.md`](./webhook-wire.md) | Провод HTTP (`webhookCaller` + `jsonval`) | тело запроса `{"цель","данные"}`, декод ответа, пустое тело→Пусто, нетегированный энкодер, httptest |
| [`cli-webhook.md`](./cli-webhook.md) | CLI-проводка | `--вебхук`/`LADIX_WEBHOOK` в run/serve/complete/start; serve=единый движок; ошибка неверного URL (дословно) |
| [`golden-en7.md`](./golden-en7.md) | Инвариант §EN-7 | дефолт-драйвер = печать-стаб байт-в-байт; ≥6 пинов целы; активация `runtimeErrWrap` |

**Несущие инварианты фичи (дрейф-аудит на M2-гейте)**:
1. **§EN-7 golden цел дефолтом** (ГЛАВНЫЙ): движок БЕЗ `WithExternalCaller` → печать-стаб байт-в-байт; пины `engine_test 108/167/176/185/194`, `main_test 117/200/235` зелёные без правки текста.
2. **Шов eval↔engine НЕ расширяется**: `ProcessRuntime` остаётся 8 методов (B1 дал `CallExternalResult`); `eval` НЕ импортирует `store`/`engine`/`jsonval`.
3. **0 новых зависимостей**; реальный HTTP только под `net/http/httptest`; детерминизм; пустой дифф `store`/`lexer`/`parser`/`ast`.

Тексты CLI-ошибки/форматов стаба — ДОСЛОВНО из §AU-10.C / §AU-4.2. Большие доки синкает архитектор на шве.
