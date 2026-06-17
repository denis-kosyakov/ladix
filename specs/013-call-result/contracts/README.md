# Contracts: B1 «Захват результата `вызвать` как выражения»

**Feature**: 013-call-result | **Источник**: `docs/automation-model.md` §AU-3 (D-AU-1)

Контракты B1 — два среза несущих границ, затронутых фичей. Оба аддитивны.

| Файл | Граница | Суть |
|---|---|---|
| [`process-runtime.md`](./process-runtime.md) | Шов eval↔engine (`ProcessRuntime`) | +1 метод `CallExternalResult` (7→8); `CallExternal` делегирует; `eval` без импорта `store`/`engine`; golden §EN-7 цел |
| [`parser-call-expr.md`](./parser-call-expr.md) | Грамматика и AST (фронтенд) | новый узел `CallExternalExpr`; ветка `parsePrimary case KW_CALL`; `startsExpression += KW_CALL`; развязка statement↔выражение; постфиксы |

**Несущие инварианты фичи (дрейф-аудит на M2-гейте)**:
1. `ProcessRuntime` = РОВНО 8 методов (+`CallExternalResult`), 7 старых сигнатур не трогаются; `eval` НЕ импортирует `store`/`engine`.
2. Аддитивность фронтенда (§3): `CallExternalExpr` не ломает v1 (существующие выражения и постфикс-вызов `f(x)`).
3. golden §EN-7 печать-стаба `вызвать`/`уведомить` байт-в-байт цел (B1 меняет грамматику, не драйвер).
