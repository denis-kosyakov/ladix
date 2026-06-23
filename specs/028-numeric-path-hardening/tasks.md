---
description: "Task list — Харденинг числовых путей движка метрик (028)"
---

# Tasks: Харденинг числовых путей движка метрик

**Input**: Design documents from `/specs/028-numeric-path-hardening/`

**Prerequisites**: plan.md ✓, spec.md ✓, research.md ✓, data-model.md ✓, contracts/test-locks.md ✓, quickstart.md ✓

**Tests**: ВКЛЮЧЕНЫ и ОБЯЗАТЕЛЬНЫ. Фича целиком — характеризационные замки eval-поведения
(Конституция VI; spec FR-001..FR-008/FR-006; plan «вся про тесты»). Каждый числовой тест
ОБЯЗАН проверять **категорию (`isRuntime`/`isType`) + сообщение (вербатим) + тип результата
(+ позицию у A1)**, а не только «не паника» (FR-006/SC-004). Прогон из `src/`.

**Organization**: Задачи сгруппированы по user story для независимой реализации/проверки.
Часть A (US1+US2) — только тесты, прод-код НЕ трогается. Часть B (US3) — два механических
рейминга + перекрёстные комментарии.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: параллелизуемо (разные файлы, нет зависимостей на незавершённые задачи)
- **[Story]**: US1 / US2 / US3 (Setup/Foundational/Polish — без метки)
- Точный путь файла — в каждой задаче
- **🔁**: задача мутационной проверки (замок ОБЯЗАН покраснеть при удалении гарда). Выполняется
  в **ИЗОЛЯЦИИ** (`git worktree`/копия, НЕ в общем дереве параллельно с другими `go test` —
  параллельный мутатор в общем чекауте портит конкурентные прогоны; см. memory
  review-mutation-shared-worktree).

---

## Карта историй (US → приоритет → суть → замки)

| US | Приоритет | Суть | Замки (research.md / test-locks.md) | Файл(ы) |
|----|-----------|------|-------------------------------------|---------|
| **US1** | P1 🎯 MVP | Числовые краевые ветки `combineBinary`/`combineUnary`/`arith` заперты **через дериватив метрики** (combineUnary был 0% покрытия) | A1 div-zero (`/`,`//`,`mod`) · A2 overflow (add/mul/floorDiv MinInt64//-1) · A3 unary neg (-MinInt64 / -Дробное) · A4 NaN/±Inf пропагация · A5 None-операнд | `src/internal/eval/metric_engine_test.go` |
| **US2** | P2 | Оба контракта `numberToValue` заперты до рейминга | A6 строгий (целое вне int64 → ошибка §SM-9.B) · A7 толерантный (±Inf, число НИКОГДА не None) | `src/internal/eval/source_loader_test.go` (A6) · `src/internal/jsonval/decode_test.go` (A7) |
| **US3** | P3 | Развести опасный одноимённый `numberToValue` (противоположные контракты) | B1 рейминг строгого `→sourceNumberToValue` · B2 рейминг толерантного `→payloadNumberToValue` · B3 перекрёстные комментарии-двойники · B-mut компиляционный замок | `src/internal/eval/source_loader.go` (B1) · `src/internal/jsonval/decode.go` (B2) |

Прод-логика `combineBinary`/`combineUnary`/`arith.go` — **байт-в-байт неизменна** (FR-012/SC-005).
Единственный прод-дифф — рейминг двух `numberToValue` + 4 строки комментариев (B3). 0 новых
зависимостей/KW/builtins/eval-кодов/операторов (SC-006).

---

## Phase 1: Setup (зелёный baseline до правок)

**Purpose**: зафиксировать исходное зелёное состояние, чтобы любой регресс был атрибутируем фиче.

- [ ] T001 Подтвердить зелёный baseline ДО любых правок: из `src/` прогнать `gofmt -l .` (пусто), `go build ./...`, `go vet ./...`, `go test -race ./...` — всё зелёное. Зафиксировать вывод как исходный эталон. Если baseline красный — остановиться и сообщить (фича не должна стартовать на красном дереве).

**Checkpoint**: дерево зелёное → можно добавлять замки.

---

## Phase 2: Foundational (общие тест-фикстуры — кастомный JSON-источник)

**Purpose**: единый helper кастомного источника для деривативных замков, которым НЕ годится
стандартная `examples/data/sales.json`: A3 (поле `MinInt64=-9223372036854775808`), A4 (поле
`1e300` для конечного большого float, перемножение которого даёт `±Inf`/`NaN`). Helper
переиспользует существующие `writeJSON`/`makeSourceDecl` (source_loader_test.go) и
билдер-стиль `buildMetricInterp` с подстановкой `sd.File.Value` на временный путь
(research.md «Фикстуры» / «Кастомная»; test-locks.md «Заметки к конструкциям»).

**⚠️ BLOCKS US1-задачи A3/A4** (T009/T010). A1/A2-add/A2-mul/A5 идут на стандартной фикстуре и
этого helper'а не требуют.

- [ ] T002 [P] В `src/internal/eval/metric_engine_test.go` добавить тест-helper `buildMetricInterpCustomSource(t, jsonRecords string, aggregate string) (value.Value, error)` (или эквивалент): записать временный JSON через `writeJSON(t, t.TempDir(), "f.json", jsonRecords)`, спарсить источник+метрику в стиле `buildMetricInterp`, переписать `sd.File.Value` на временный путь (НЕ `salesPath()`), задать `агрегат:` из параметра, окно — `где: статус == "оплачен"` + `период: ежемесячно` + `по_дате: дата(...)` для непустого окна (или единичная запись), вызвать `evalMetric` и вернуть `(value.Value, error)`. Фикстура для A3 = `[{"поле": -9223372036854775808, "дата_заказа": "...", "статус": "оплачен"}]`; для A4 = `[{"огромное": 1e300, ...}]`. ВАЖНО (research A3): `-9223372036854775808` записывается в JSON одним токеном json.Number, грузится строгим путём как `Целое{MinInt64}` без переполнения. ВАЖНО (research A4): источник отвергает `1e400` строгим путём (Float64 err) — поэтому в фикстуре `1e300` (конечное), а `±Inf` строится перемножением в деривативе. ПЕРЕД написанием прочитать существующие `buildMetricInterp`/`writeJSON`/`makeSourceDecl`, чтобы переиспользовать их и не дублировать парсинг.

**Checkpoint**: helper готов → A3/A4 могут строить кастомные окна.

---

## Phase 3: User Story 1 — Числовые краевые ветки через дериватив метрики (Priority: P1) 🎯 MVP

**Goal**: запереть характеризационными тестами через **дериватив метрики** все краевые числовые
ветки `combineBinary`/`combineUnary`/`arith.go`: div-zero, overflow, унарный минус (ЯДРО —
`combineUnary` был 0% покрытия), пропагацию NaN/±Inf, операнд None. Тесты проверяют
категорию+сообщение+тип(+позицию A1), не «не паника».

**Independent Test**: `cd src && go test -race ./internal/eval/ -run 'TestMetricDivByZero|TestMetricIntOverflow|TestMetricUnaryNeg|TestMetricFloatSpecials|TestMetricNoneOperand' -v` — зелёное; покрытие `combineUnary` > 0% (SC-002).

> Все A1–A5 — НОВЫЕ тест-функции в `src/internal/eval/metric_engine_test.go` (один файл → между
> собой НЕ параллелятся; помечены [P] только относительно US2-файлов). Непустое окно обязательно
> (research «КРИТИЧНО: D4-1»): на пустом окне корневой дериватив короткозамыкается в `Пусто` ДО
> `combine*` (metric_engine.go:79-81) — стандартная фикстура `paid+ежемесячно` даёт 2 выживших.

### Тесты US1

- [ ] T003 [US1] A1 деление на ноль — `TestMetricDivByZero` (табличный) в `src/internal/eval/metric_engine_test.go`. Три строки таблицы через дериватив на стандартной непустой фикстуре (research A1 / test-locks A1-div/A1-floordiv/A1-mod):
  - `/` (дробная, `evalDiv` arith.go:164): `агрегат: сумма(сумма_заказа) / (количество(запись) - количество(запись))` → знаменатель Целое 0, промоушен к float → `rf==0`;
  - `//` (целочисл., `evalFloorDiv` arith.go:178): `агрегат: сумма(сумма_заказа) // (количество(запись) - количество(запись))`;
  - `mod` (целочисл., `evalMod` arith.go:193): `агрегат: сумма(сумма_заказа) % (количество(запись) - количество(запись))`.
  Ассерт (через `evalMetricErr`+`evalErr`→`(line,col,msg)`): `isRuntime(err)` И `msg == "деление на ноль"` (вербатим) И **`line>=1 && col>=1`** (позиция оператора, Конституция IV/FR-001) И гарантированно НЕ паника. Пинит гарды `evalDiv`/`evalFloorDiv`/`evalMod` `if …==0`.

- [ ] T004 [US1] A2 переполнение целого (стандартная фикстура) — `TestMetricIntOverflow` (табличный) в `src/internal/eval/metric_engine_test.go`. Две строки на стандартной непустой фикстуре (сумма=2000000>0; research A2 / test-locks A2-add/A2-mul):
  - add (`addInt64` arith.go:247): `агрегат: сумма(сумма_заказа) + 9223372036854775807` (литерал = MaxInt64, выразим);
  - mul (`mulInt64` arith.go:263): `агрегат: сумма(сумма_заказа) * 9223372036854775807`.
  Ассерт: `isRuntime(err)` И `msg == "переполнение целого числа"` (вербатим). Пинит overflow-ловушки `addInt64`/`mulInt64`.

- [ ] T005 [US1] A2 переполнение `floorDivInt64 MinInt64 // -1` (кастом-фикстура) — отдельная строка/подтест `TestMetricIntOverflowFloorDiv` (или строка в `TestMetricIntOverflow` через `buildMetricInterpCustomSource`) в `src/internal/eval/metric_engine_test.go`. Зависит от T002. Конструкция (research A2 / test-locks A2-floordiv): фикстура поле=`-9223372036854775808`, `агрегат: мин(поле) // (количество(запись) - количество(запись) - 1)` → числитель MinInt64, знаменатель `2-2-1=-1` (НЕ 0, иначе сработал бы div-zero) → ловушка `floorDivInt64` `a==MinInt64 && b==-1` (arith.go:283-292). Ассерт: `isRuntime(err)` И `msg == "переполнение целого числа"`.

- [ ] T006 [US1] A3 унарный минус `-Дробное` → корректно (стандартная фикстура, ЯДРО combineUnary) — `TestMetricUnaryNegFloat` в `src/internal/eval/metric_engine_test.go`. Конструкция (research A3 / test-locks A3-neg-float): `агрегат: -(среднее(сумма_заказа))`, окно paid+ежемесячно → `среднее`=`Дробное{1000000.0}` → `combineUnary(neg, Дробное)` ветка metric_engine.go:293 → `Дробное{-1000000.0}`. Ассерт (через `evalGolden`/значение): результат успешен (без ошибки) И `value.String(v) == "-1000000.0"` И **реально прогоняет `combineUnary`** (SC-002). Пинит Дробное-ветку `combineUnary`.

- [ ] T007 [US1] A3 унарный минус `-MinInt64` → переполнение (кастом-фикстура, ЯДРО combineUnary) — `TestMetricUnaryNegOverflow` в `src/internal/eval/metric_engine_test.go`. Зависит от T002. Конструкция (research A3 / test-locks A3-neg-min): фикстура поле=`-9223372036854775808` (грузится строгим путём как `Целое{MinInt64}`, единичное окно), `агрегат: -(мин(поле))` → `мин`=`Целое{MinInt64}` → `combineUnary(neg, Целое)` ветка `v.V == math.MinInt64` (metric_engine.go:289) → переполнение. Ассерт: `isRuntime(err)` И `msg == "переполнение целого числа"` И **реально прогоняет `combineUnary`** (закрывает 0% покрытия, SC-002). Пинит MinInt64-ловушку `combineUnary`.

- [ ] T008 [US1] A4 пропагация NaN/±Inf через combineBinary (кастом-фикстура) — `TestMetricFloatSpecials` (табличный) в `src/internal/eval/metric_engine_test.go`. Зависит от T002. Три строки на кастом-фикстуре поле `огромное`=`1e300` (конечное; research A4 / test-locks A4-pinf/A4-ninf/A4-nan):
  - +Inf: `агрегат: среднее(огромное) * среднее(огромное)` → `Дробное{1e300}²` через float-ветку `evalSubMul` (arith.go:149-150) → `Дробное{+Inf}`; ассерт `v.(value.Дробное)` ok И `math.IsInf(v.V, +1)`;
  - -Inf: `агрегат: -(среднее(огромное) * среднее(огромное))` → combineUnary над `+Inf`; ассерт `math.IsInf(v.V, -1)`;
  - NaN: `агрегат: (среднее(огромное)*среднее(огромное)) - (среднее(огромное)*среднее(огромное))` → `+Inf - +Inf` → NaN; ассерт `v.(value.Дробное)` ok И **`math.IsNaN(v.V)`** (НЕ `==`, NaN != NaN; Edge Case спеки/FR-004). Во всех строках: результат — `value.Дробное` (НЕ паника/None), значение по IEEE-754. (`math` уже импортирован в jsonval/decode_test.go; eval-тест импортирует его аналогично.) Пинит float-ветки `evalSubMul` (отсутствие перехвата Inf/NaN).

- [ ] T009 [US1] A5 операнд None/Пусто в combineUnary и combineBinary — `TestMetricNoneOperand` (табличный) в `src/internal/eval/metric_engine_test.go`. Конструкция (research A5 / test-locks A5-unary-none/A5-binary-none): объявить метрику `пусто_м` с **пустым** окном (напр. `среднее(сумма_заказа)` + `период: ежедневно` → `value.None` по D4-1), затем внешняя метрика на **непустом** окне читает её как глобаль (паттерн `TestMetricAsValueReentrant`):
  - unary (`combineUnary` default metric_engine.go:295): `агрегат: -(пусто_м)` → typeErr; ассерт `isType(err)` И `msg == "унарный '-' нельзя применить к Пусто"`;
  - binary (`evalAdd` type-mismatch arith.go): `агрегат: сумма(сумма_заказа) + пусто_м` → typeErr; ассерт `isType(err)` И `msg == "'+' нельзя применить к Целое и Пусто"`.
  Сначала эмпирически подтвердить `TypeName()` у None (ожидается «Пусто») для exact-match; если хрупко — `isType`+`strings.Contains(msg,"нельзя применить")`, но предпочесть exact. НЕ путать None-операнд с пустым окном D4-1. Пинит default `combineUnary` и type-mismatch `evalAdd`.

### Мутпробы US1 (🔁, в ИЗОЛЯЦИИ)

- [ ] T010 [US1] 🔁 Мутпроба A1 div-zero (изолированный worktree/копия). Временно закомментировать гард `if rf == 0` в `evalDiv` (`src/internal/eval/arith.go:163-164`) → прогнать `go test ./internal/eval/ -run TestMetricDivByZero` → строка `/` ОБЯЗАНА покраснеть (получит `Дробное{+Inf}` вместо ошибки). Затем убрать `if ri.V == 0` в `evalFloorDiv` (arith.go:177-178) и `evalMod` (arith.go:192-193) → строки `//`/`mod` ОБЯЗАНЫ покраснеть (паника `integer divide by zero` вместо типизированной ошибки). Откатить мутацию (НЕ коммитить). Подтвердить запись результата.

- [ ] T011 [US1] 🔁 Мутпроба A2 overflow (изолированный worktree/копия). Временно заставить `addInt64` (`src/internal/eval/arith.go:247-253`) возвращать `s, false` всегда → `go test ./internal/eval/ -run TestMetricIntOverflow` строка add ОБЯЗАНА покраснеть (wrapped Целое вместо ошибки). Аналогично убрать MinInt64-ловушку в `floorDivInt64` (arith.go:283-292) → строка floorDiv (T005) ОБЯЗАНА покраснеть. Откатить.

- [ ] T012 [US1] 🔁 Мутпроба A3 `-MinInt64` combineUnary (изолированный worktree/копия). Временно убрать гард `if v.V == math.MinInt64` в `combineUnary` (`src/internal/eval/metric_engine.go:289`) → `go test ./internal/eval/ -run TestMetricUnaryNegOverflow` ОБЯЗАН покраснеть (вернёт обёрнутое `Целое{MinInt64}` вместо ошибки «переполнение»). Откатить. Это ядровой замок 0%-покрытия combineUnary (SC-002).

**Checkpoint**: US1 полностью функциональна и независимо проверяема; combineUnary покрыт (SC-002).

---

## Phase 4: User Story 2 — Оба контракта numberToValue заперты (Priority: P2)

**Goal**: запереть до рейминга оба де-факто контракта `numberToValue`: строгий (источник) даёт
ошибку §SM-9.B на целом-вне-int64; толерантный (payload) даёт `±Inf` и число НИКОГДА не
деградирует в None.

**Independent Test**: `cd src && go test -race ./internal/eval/ -run TestSourceIntOutOfRange ./internal/jsonval/ -run TestPayloadNumberInfinity -v` — зелёное.

> A6 и A7 — РАЗНЫЕ файлы (`eval/source_loader_test.go` vs `jsonval/decode_test.go`) → между собой
> [P]; и обе [P] относительно US1-файла `metric_engine_test.go`.

### Тесты US2

- [ ] T013 [P] [US2] A6 строгий контракт — `TestSourceIntOutOfRange` в `src/internal/eval/source_loader_test.go`. Конструкция (research A6 / test-locks A6-strict): через `writeJSON`+`makeSourceDecl`+`assertLoadErr` фикстура `[{"поле": 99999999999999999999}]` (целое вне int64, БЕЗ точки/e/E) → `n.Int64()` падает → ветка `§SM-9.B` (source_loader.go:221-224). Ассерт (`assertLoadErr` уже проверяет `isRuntime`): `msg == "источник 'продажи': запись 0, поле 'поле': целое число вне диапазона"` (вербатим; индекс записи 0-based). Пинит строгий гард `n.Int64() err → §SM-9.B`.

- [ ] T014 [P] [US2] A7 толерантный контракт (золотой замок) — `TestPayloadNumberInfinity` (табличный) в `src/internal/jsonval/decode_test.go`. Конструкция (research A7 / test-locks A7-tolerant) через `PayloadToRecord`+`rec.Get("x")`:
  - `{"x": 1e400}` → `value.Дробное` И `math.IsInf(.V, +1)` (Float64 overflow → +Inf, err проигнорирован, decode.go:135-147);
  - `{"x": -1e400}` → `value.Дробное` И `math.IsInf(.V, -1)`;
  - `{"x": 99999999999999999999}` (целое вне int64, без точки) → ЭМПИРИЧЕСКИ Float64 даёт **конечный ~1e20, НЕ ±Inf** → ассертить `v, ok := rec.Get("x").(value.Дробное); ok` (число стало **Дробным**, НЕ None) — БЕЗ `IsInf`. Во всех строках: **число НИКОГДА не None** (FR-008). Использовать `math.IsInf`/`math.IsNaN`, не `==`. Пинит Float64-fallback толерантного пути.

### Мутпробы US2 (🔁, в ИЗОЛЯЦИИ)

- [ ] T015 [US2] 🔁 Мутпроба A6 строгий (изолированный worktree/копия). Временно в `src/internal/eval/source_loader.go` (метод `numberToValue`, ветка ~:221) заставить на `n.Int64()` err возвращать `Дробное` (как толерантный путь) вместо ошибки §SM-9.B → `go test ./internal/eval/ -run TestSourceIntOutOfRange` ОБЯЗАН покраснеть (получит Дробное, нет ошибки). Откатить.

- [ ] T016 [US2] 🔁 Мутпроба A7 толерантный (изолированный worktree/копия). Временно в `src/internal/jsonval/decode.go` (функция `numberToValue`, Float64-fallback ~:145) заставить на overflow возвращать `value.None` вместо `Дробное` → `go test ./internal/jsonval/ -run TestPayloadNumberInfinity` ОБЯЗАН покраснеть (None != Дробное; нарушен «НИКОГДА не None»). Откатить.

**Checkpoint**: оба контракта numberToValue заперты (SC-003) — рейминг US3 теперь страхуется.

---

## Phase 5: User Story 3 — Развести одноимённый нейминг numberToValue (Priority: P3)

**Goal**: переименовать две одноимённые `numberToValue` с противоположными контрактами в
`sourceNumberToValue` (строгий, метод) и `payloadNumberToValue` (толерантный, свободная функция),
обновить по одному call-site каждой, добавить по одной перекрёстной строке-комментарию. Поведение
байт-в-байт прежнее (страхуется A6/A7 + существующими тестами).

**Independent Test**: `cd src && go build ./... && go vet ./... && go test -race ./internal/eval/ ./internal/jsonval/` — зелёное; греп новых имён находит символ+вызов, греп `func numberToValue`/`numberToValue(` даёт 0 совпадений.

> B1 (eval/source_loader.go) и B2 (jsonval/decode.go) — РАЗНЫЕ файлы → [P] между собой. B3 правит
> те же два файла (комментарии) — выполнять В РАМКАХ B1/B2 или сразу после.

- [ ] T017 [P] [US3] B1 рейминг строгого (метод) в `src/internal/eval/source_loader.go`: метод `numberToValue` (объявление :211) → `sourceNumberToValue`; обновить его единственный call-site в `decodeValue` (:158, `return i.numberToValue(...)` → `return i.sourceNumberToValue(...)`). Подпись/тело/тексты ошибок НЕ менять (только имя символа). После — `go build ./...` зелёный; `TestSourceIntOutOfRange` (A6) + существующий §SM-9 edge-набор проходят (поведение прежнее).

- [ ] T018 [P] [US3] B2 рейминг толерантного (свободная функция) в `src/internal/jsonval/decode.go`: функция `numberToValue` (объявление :135) → `payloadNumberToValue`; обновить её единственный call-site в `DecodeValue` (:81, `return numberToValue(t), nil` → `return payloadNumberToValue(t), nil`). Подпись/тело НЕ менять (только имя). После — `go build ./...` зелёный; `TestPayloadNumberInfinity` (A7) + существующий `TestPayloadToRecordValueTypes` проходят.

- [ ] T019 [US3] B3 перекрёстные комментарии-двойники (правит оба файла из T017/T018). В `src/internal/eval/source_loader.go` к `sourceNumberToValue` добавить ОДНУ строку перекрёстной ссылки на толерантный близнец (напр. «толерантный двойник: jsonval.payloadNumberToValue (вне диапазона → ±Inf, никогда не None)»); существующий поясняющий комментарий контракта (:209-210) СОХРАНИТЬ. В `src/internal/jsonval/decode.go` к `payloadNumberToValue` добавить ОДНУ строку на строгий близнец (напр. «строгий двойник: eval.sourceNumberToValue (вне int64 → ошибка §SM-9.B)»); существующий комментарий (:132-134) и шапку-асимметрию (:11-14) СОХРАНИТЬ. Никаких других переименований (FR-015): греп `func numberToValue`/`\bnumberToValue(` по `src/` → 0 совпадений; греп `двойник` → 2 совпадения.

- [ ] T020 [US3] 🔁 B-mut компиляционный замок (изолированный worktree/копия). Временно НЕ обновить один call-site после рейминга (напр. оставить `i.numberToValue(...)` в decodeValue при переименованном объявлении) → `go build ./...` ОБЯЗАН упасть («undefined: …»/символ не найден). Подтверждает, что рейминг страхуется компилятором. Откатить к корректному состоянию (call-site обновлён).

**Checkpoint**: все три истории независимо функциональны; асимметрия numberToValue читаема в именах.

---

## Phase 6: Polish & Cross-Cutting (финальный гейт + границы + doc-sync)

**Purpose**: полный гейт, проверка границ диффа, doc-sync §SM-9.B по необходимости.

- [ ] T021 Полный гейт SC-001: из `src/` прогнать `gofmt -l .` (пусто), `go build ./...`, `go vet ./...`, `go test -race ./...` — всё зелёное. Существующие `TestGoldenSM10`/`TestPayloadToRecordValueTypes`/§SM-9 edge-набор НЕ сломаны (прод-поведение прежнее, FR-012/SC-005).

- [ ] T022 Проверка покрытия combineUnary (SC-002): `cd src && go test ./internal/eval/ -run 'TestMetricUnaryNeg' -coverprofile=/tmp/cov.out && go tool cover -func=/tmp/cov.out | grep -i combineUnary` → combineUnary > 0% (ветки OpNeg: Целое-MinInt64 + Дробное прогнаны). Был 0%.

- [ ] T023 Проверка границ диффа (SC-005/FR-012): `cd src && git diff --stat internal/eval/metric_engine.go internal/eval/arith.go` → **ПУСТО** (combineBinary/combineUnary/arith байт-в-байт прежние). Единственный прод-дифф — `internal/eval/source_loader.go` + `internal/jsonval/decode.go` (рейминг + перекрёстные комментарии, B1/B2/B3). Подтвердить ПУСТОЙ дифф в `engine`/`store`/`daemon`/`cmd` и 0 новых зависимостей (`go.mod`/`go.sum` не тронуты, SC-006).

- [ ] T024 Doc-sync §SM-9.B (по необходимости): в `docs/source-metric-model.md` §SM-9.B — если упоминается имя `numberToValue`, обновить ссылку на новое `sourceNumberToValue`. Новой языковой функциональности нет → крупные каноны НЕ правятся (FR-014). Если упоминаний имени нет — задача no-op, отметить как проверенную.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1, T001)**: без зависимостей — стартует немедленно. БЛОКИРУЕТ всё (baseline должен быть зелёным).
- **Foundational (Phase 2, T002)**: после T001. БЛОКИРУЕТ только A3/A4 (T005, T007, T008).
- **US1 (Phase 3)**: после T001; A3/A4-задачи (T005/T007/T008) дополнительно после T002.
- **US2 (Phase 4)**: после T001 — НЕЗАВИСИМА от US1 (другие файлы), может идти параллельно US1.
- **US3 (Phase 5)**: после US2 (рейминг страхуется запертыми контрактами A6/A7) — рекомендуется P2→P3. Технически компилируется и без US2, но spec требует запереть контракты до рейминга.
- **Polish (Phase 6)**: после US1+US2+US3.

### User Story Dependencies

- **US1 (P1)**: после Foundational (для A3/A4); A1/A2-add/A2-mul/A5 — сразу после T001. Независима от US2/US3.
- **US2 (P2)**: после T001; независима от US1. Запирает контракты ДО рейминга US3.
- **US3 (P3)**: после US2 (страховка). Рейминг + комментарии.

### Within Each User Story

- Тесты пишутся и проходят до мутпроб (мутпроба проверяет, что замок краснеет на сломанном гарде).
- Мутпробы (🔁) — ПОСЛЕ соответствующих тестов, в ИЗОЛЯЦИИ.

### Parallel Opportunities

- **Между US1 и US2**: `metric_engine_test.go` (US1) ∥ `source_loader_test.go` (A6) ∥ `decode_test.go` (A7) — три разных файла, нет общих зависимостей.
- **Внутри US2**: T013 (A6) [P] ∥ T014 (A7) [P] — разные файлы/пакеты.
- **Внутри US3**: T017 (B1, eval) [P] ∥ T018 (B2, jsonval) [P] — разные файлы.
- **Внутри US1**: все A1–A5 в одном файле `metric_engine_test.go` → последовательно (НЕ [P] между собой).
- **Мутпробы (🔁)**: НЕ параллелить в общем дереве — каждая в изолированном worktree/копии.

---

## Parallel Example: US1 ∥ US2

```bash
# После T001 (+T002 для A3/A4) три файла можно вести параллельно:
Задача T003–T009: A1–A5 в src/internal/eval/metric_engine_test.go      (US1)
Задача T013:      A6 в src/internal/eval/source_loader_test.go         (US2, [P])
Задача T014:      A7 в src/internal/jsonval/decode_test.go             (US2, [P])

# Рейминг US3 — два файла параллельно:
Задача T017: sourceNumberToValue в src/internal/eval/source_loader.go  (US3, [P])
Задача T018: payloadNumberToValue в src/internal/jsonval/decode.go     (US3, [P])
```

---

## Implementation Strategy

### MVP First (US1 — P1)

1. T001 (baseline зелёный) → T002 (helper для A3/A4).
2. T003–T009 (A1–A5 замки) → T010–T012 (мутпробы 🔁).
3. **STOP & VALIDATE**: `go test -race ./internal/eval/` зелёное; combineUnary > 0% (SC-002).

### Incremental Delivery

1. Setup + Foundational → baseline + helper.
2. US1 → числовые ветки заперты, combineUnary покрыт (MVP — ядро ценности).
3. US2 → оба контракта numberToValue заперты (страховка для рейминга).
4. US3 → рейминг + перекрёстные комментарии (безопасность сопровождения).
5. Polish → полный гейт + границы + doc-sync.

### Параллельная стратегия (если несколько исполнителей)

- T001 → затем US1 (исполнитель A, metric_engine_test.go) ∥ US2 (исполнитель B, source_loader_test.go + decode_test.go).
- US3 — после US2; B1/B2 параллельно (разные файлы).
- Мутпробы 🔁 — у каждого свой изолированный worktree.

---

## Notes

- [P] = разные файлы, нет зависимостей. 🔁 = мутпроба (краснеет при сломанном гарде, в ИЗОЛЯЦИИ).
- Прод-логика `combineBinary`/`combineUnary`/`arith.go` — **байт-в-байт неизменна** (FR-012/SC-005); T023 это проверяет.
- Единственный прод-дифф — рейминг двух `numberToValue` + 4 строки комментариев (B3). 0 новых KW/builtins/eval-кодов/операторов/зависимостей (SC-006).
- Эмпирические уточнения зафиксированы: A4 фикстура `1e300` (источник отвергает `1e400`); A7 `99999999999999999999`→**Дробное** (конечное ~1e20, НЕ ±Inf), `±Inf` только `1e400`/`-1e400`; A5 None-операнд через ссылку на **пустую метрику-глобал** в деривативе непустой метрики.
- NaN/±Inf — только `math.IsNaN`/`math.IsInf`, никогда `==` (NaN != NaN).
- Все тексты ошибок — русские, дословно прежние, exact-match (Конституция VIII).
- Прогон проверок — из `src/`.
- Git: НЕ делать add/commit в рамках этой стадии (по указанию). Мутпробы откатывать без коммита.
