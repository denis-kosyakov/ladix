# Implementation Plan: Харденинг числовых путей движка метрик

**Branch**: `028-numeric-path-hardening` | **Date**: 2026-06-23 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/028-numeric-path-hardening/spec.md`

## Summary

Две независимые части, обе **без изменения прод-поведения**:

- **Часть A (US1+US2 — тесты-онли)**: запереть характеризационными тестами существующую
  числовую семантику движка метрик через **дериватив метрики** (путь, по которому формула
  реально исполняется в движке: `evalAggExpr` → `combineBinary` / `combineUnary` →
  `evalAdd`/`evalSubMul`/`evalDiv`/`evalFloorDiv`/`evalMod` / унарный `combineUnary`), а
  также оба контракта `numberToValue`. Покрываются краевые ветки: деление на ноль (дробная
  `/` и целочисленные `//`/`mod`), переполнение целого (`evalAdd`/`evalSubMul`/`evalFloorDiv`
  `MinInt64 // -1`), унарный минус (`-MinInt64` → переполнение, `-Дробное` → корректно —
  **ядро: `combineUnary` сегодня 0% покрытия**), пропагация `NaN`/`±Inf` по IEEE-754 через
  `combineBinary`, операнд `None`/`Пусто` → типовая ошибка. Тесты проверяют **категорию +
  сообщение + тип + значение**, а не только «не паника», и краснеют при удалении гарда
  (мутационная чувствительность).
- **Часть B (US3 — чисто механический рефактор)**: развести две одноимённые функции
  `numberToValue` с противоположными контрактами. Строгий метод (путь загрузки источника,
  `eval/source_loader.go`, `§SM-9.B`: вне `int64` → ошибка) → `sourceNumberToValue`.
  Толерантная свободная функция (путь декодирования payload, `jsonval/decode.go`: вне
  диапазона → `±Inf`) → `payloadNumberToValue`. У каждой обновляется единственный вызов и
  добавляется одна строка перекрёстной ссылки на близнеца. Имена НАМЕРЕННО разные. Поведение
  байт-в-байт прежнее.

**Технический подход**: всё, что есть для тестов (билдер интерпретатора метрики, золотые
ассерты, помощник «ошибка → строка/координата», запись JSON-источника, декодер payload,
`FixedClock`), уже существует и переиспользуется. Часть A не добавляет прод-кода вообще; часть
B — два переименования + четыре строки комментариев. `combineBinary` / `combineUnary` /
`arith.go` остаются **байт-в-байт неизменными** (FR-012/SC-005).

## Technical Context

**Language/Version**: Go 1.22+ (`gofmt` обязателен, `go vet ./...` без замечаний — Конституция I).

**Primary Dependencies**: только stdlib `testing` + `math` (`math.IsNaN`/`math.IsInf` уже
используются в `internal/jsonval/decode_test.go`); прод-зависимость хранилища
`modernc.org/sqlite` фичей не затрагивается. **0 новых внешних зависимостей** (SC-006).

**Storage**: N/A — durable-хранилище и фронтенд не затрагиваются (границы спеки).

**Testing**: `go test -race ./...`, прогон из каталога `src/`. Детерминизм через
`testClock = FixedClock{2026-05-31}` (фикстура `examples/data/sales.json`). Переиспользуемые
хелперы: `eval/metric_engine_test.go` (`buildMetricInterp`/`goldenMetric`/`evalGolden`/
`evalMetricErr`/`evalMetricByName`/`salesPath`), `eval/helpers_test.go`
(`evalErr`→`(line,col,msg)`/`isRuntime`/`isType`), `eval/source_loader_test.go`
(`newTestInterp`/`writeJSON`/`makeSourceDecl`/`assertLoadErr`/`loadSource`), `jsonval/decode_test.go`
(`PayloadToRecord`/`DecodeValue`/`NewDecoder`, ассерт через `math.IsInf`).

**Target Platform**: один статический бинарник `ladix`, кросс-платформенный (Конституция I).

**Project Type**: интерпретатор DSL (single project, Go-раскладка `cmd/ladix` + `internal/*`).

**Performance Goals**: N/A (тесты-онли + рейминг; никакого hot-path-кода не добавляется).

**Constraints**: **ПУСТОЙ прод-функциональный дифф** в `combine*` / `arith` / `engine` /
`store` / `daemon` / `cmd`; дифф строго в `src/internal/eval` (тесты + рейминг строгого
преобразователя) и `src/internal/jsonval` (тесты + рейминг толерантного) + при необходимости
doc-sync `§SM-9.B`. 0 новых ключевых слов / встроенных функций / кодов eval / операторов;
тексты ошибок и wire-ключи неизменны (FR-014/FR-016/SC-006).

**Scale/Scope**: ~7 новых тест-блоков (A1–A7) + 2 переименования прод-символов с обновлением
по 1 call-site каждое.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| # | Принцип | Вердикт | Обоснование |
|---|---------|---------|-------------|
| I | Язык и сборка | **PASS** | Go 1.22+; `gofmt`/`go vet ./...` чисто (SC-001). **0 новых внешних зависимостей** — нужен только stdlib `math` (`IsNaN`/`IsInf`), который уже импортирован в `jsonval/decode_test.go`. CGO не вводится; артефакт сборки не меняется (тесты + рейминг). |
| II | Парсинг — ручной | **PASS** | Лексер/парсер не трогаются; ни генераторов, ни regex. Лексерный путь `FLOAT`→`L-8` НЕ трогается, остаётся отложенным (FR-016, Out of Scope). |
| III | Ошибки — явные типы | **PASS** | Фича **подтверждает** принцип «паника только как инвариант»: каждый тест A1–A5 проверяет, что краевая ветка даёт **ошибку с типом** (`errors.ОшибкаВыполнения`/`errors.ОшибкаТипа` через `errors.As`/хелперы `isRuntime`/`isType`), а НЕ панику. Деление на ноль / переполнение / `-MinInt64` → типизированная ошибка, не паника. Внутренние «не должно случиться» (`combineUnary`/`combineBinary` default) остаются громким инвариантом. |
| IV | Позиции — сквозные | **PASS** | Тест A1 явно проверяет, что ошибка деления на ноль несёт `Pos{Line,Col}` (1-based, в рунах) — `evalErr` возвращает `(line,col,msg)` и тест ассертит координату, а не только сообщение (FR-001/FR-006). Прод-код уже протаскивает `b.Pos()`/`u.Pos()` в каждую ошибку; рейминг этого не меняет. |
| V | Без глобального состояния | **PASS** | Рейминг не вводит ни одного пакетного изменяемого `var`: строгий `sourceNumberToValue` остаётся **методом** на `*Interpreter`, толерантный `payloadNumberToValue` — **свободной чистой функцией** без состояния. Тесты создают интерпретатор явно (`buildMetricInterp`/`newTestInterp`), `Store` инжектируется. |
| VI | Тесты — вперёд | **PASS** | Фича **вся про тесты** — характеризационные замки существующего поведения eval, табличные где уместно (A1 — таблица веток `/`,`//`,`mod`; A7 — таблица `1e400`/`-1e400`/целое-вне-диапазона). Негативные кейсы (ошибочные ветки) — первоклассные. Рейминг страхуется этими замками. |
| VII | Раскладка проекта | **PASS** | Дифф строго в `internal/eval` и `internal/jsonval` — оба существующих пакета. Граф зависимостей не меняется: `jsonval` остаётся листовым (импорт только `value`+stdlib), `eval` не приобретает новых импортов. Никаких новых пакетов/циклов. |
| VIII | Язык сообщений | **PASS** | Тексты ошибок — русские, **дословно прежние** («деление на ноль» / «переполнение целого числа» / «целое число вне диапазона …» / «унарный '-' нельзя применить к …»); тесты ассертят их **вербатим** (exact-match), переформулировки нет. Двухстрочный канон §13 при печати CLI не затрагивается (тесты бьют по eval-уровню). |
| IX | Спека — источник истины | **PASS** | Запираемое поведение соответствует размещённым докам: `§SM-9.B` (строгий контракт числа источника), `§SM-10`/`§SM-8` шаг 5 (D4-1 пустое окно → Пусто), `§3.3`/`§3.2` (арифметика/унарные), `§AU-5.2` (толерантный payload). Doc-sync `§SM-9.B` ссылается на новое имя `sourceNumberToValue`. Пробелов/догадок нет — все ветки прочитаны в коде. |

**Итог: 9/9 PASS. Complexity Tracking ПУСТ** — это самая чистая фича конвейера: тесты +
механический рейминг, прод-логика `combine*`/`arith` байт-в-байт цела (FR-012/SC-005), ни
одной санкционированной девиации.

## Project Structure

### Documentation (this feature)

```text
specs/028-numeric-path-hardening/
├── plan.md              # Этот файл (/speckit-plan)
├── research.md          # Phase 0: рецепты A1–A7 через дериватив + асимметрия numberToValue + мутстратегия
├── data-model.md        # Phase 1: числовое значение + два преобразователя
├── quickstart.md        # Phase 1: как прогнать тесты + проверить мутчувствительность
├── contracts/
│   └── test-locks.md    # Phase 1: таблица замков A1–A7 + рейминг-замок
└── tasks.md             # Phase 2 (/speckit-tasks — НЕ создаётся этим планом)
```

### Source Code (repository root)

```text
src/
├── internal/
│   ├── eval/
│   │   ├── metric_engine.go          # ЦЕЛ (combineBinary :247-280 / combineUnary :284-305 — диспетчеры деривативного вычислителя; НЕ трогать)
│   │   ├── arith.go                  # ЦЕЛ (evalAdd/evalSubMul/evalDiv/evalFloorDiv/evalMod — числовые гарды; НЕ трогать, FR-012)
│   │   ├── source_loader.go          # B1: метод numberToValue → sourceNumberToValue (:211) + call-site в decodeValue (:158); +1 строка перекрёстной ссылки (B3)
│   │   ├── metric_engine_test.go     # A1–A5: НОВЫЕ тесты через дериватив (goldenMetric/evalGolden/evalMetricErr; кастом-источник через writeJSON+makeSourceDecl для A3/A4)
│   │   ├── source_loader_test.go     # A6: НОВЫЙ строгий тест целое-вне-int64 → §SM-9.B (assertLoadErr/writeJSON)
│   │   └── helpers_test.go           # ЦЕЛ (переиспользуем evalErr/isRuntime/isType)
│   └── jsonval/
│       ├── decode.go                 # B2: функция numberToValue → payloadNumberToValue (:135) + call-site в DecodeValue (:81); +1 строка перекрёстной ссылки (B3)
│       └── decode_test.go            # A7: НОВЫЙ золотой замок 1e400/-1e400/целое-вне-диапазона → Дробное ±Inf (math.IsInf)
└── ... (engine/store/daemon/cmd — ПУСТОЙ дифф)

docs/
└── source-metric-model.md           # doc-sync §SM-9.B (по необходимости): ссылка на новое имя sourceNumberToValue
```

**Structure Decision**: Single project, стандартная Go-раскладка (Конституция VII). Фича
строго аддитивна в тест-плоскости двух существующих пакетов (`internal/eval`,
`internal/jsonval`) + два механических переименования прод-символов в тех же пакетах. Никаких
новых пакетов, файлов прод-кода или зависимостей. `combine*` / `arith.go` /
`engine`/`store`/`daemon`/`cmd` — вне диффа (ПУСТОЙ функциональный дифф, граница спеки).

## Complexity Tracking

> Нарушений Конституции нет — таблица намеренно пуста.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| _(нет)_ | _(нет)_ | _(нет)_ |
