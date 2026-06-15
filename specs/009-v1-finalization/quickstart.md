# Quickstart (плановый): как ПРОВЕРИТЬ фичу 009

> **Это плановый quickstart приёмки фичи** — как убедиться, что финализация v1
> закрыта. НЕ путать с пользовательским `docs/quickstart.md` («первая метрика и
> первый процесс за 5 минут»), который СОЗДАЁТ стадия implement (US4).

Источник критериев: `specs/009-v1-finalization/spec.md` (SC-001..008) +
`docs/v1-finalization-model.md` §VF-8. Все проверки прогоняются из репозитория
после реализации US1–US6.

## Шаг 0 — гейты из `src/` (база)

```sh
cd src
go build ./...        # exit 0
go vet ./...          # exit 0, без замечаний
go test ./... -count=1 # 10/10 пакетов ok, ноль «no test files»
```
✅ SC-002/SC-008: build/vet/test PASS, 10/10 пакетов покрыты.

## Шаг 1 — воспроизводимость README из чистого клона (US1)

Из **корня** свежего клона дословно:
```sh
cd src && go build -o ../ladix ./cmd/ladix   # → ./ladix в корне, exit 0
cd ..   # (если smoke не вернул сам)
./ladix run examples/hello.ladix             # → "Привет, Уклад!", exit 0
cd src && go test ./...                       # → 10/10 ok
```
✅ SC-001: блоки README дают exit 0; `hello` печатает `Привет, Уклад!`.
Проверить smoke-замок (контракт `reproducibility.md`).

## Шаг 2 — CHECKLIST.md 50/50 (US2)

```sh
grep -c '^- \[ \]' CHECKLIST.md   # = 0
grep -c '^- \[x\]' CHECKLIST.md   # = 50
```
Выборочно открыть 3–5 доказательств файл:строка и убедиться, что строка существует и
содержит заявленный факт; убедиться, что тексты пунктов рубрики не изменены (diff
против исходной рубрики — только чекбоксы и сноски).
✅ SC-003.

## Шаг 3 — витрина против скоупа (US3)

```sh
ls examples/   # событие / расписание / метрики / процесс / ошибка_синтаксис / ошибка_тип / ошибка_процесс
cd src && go test ./internal/parser/ -run Examples   # компиляция витрины зелёная
cd src && go test ./cmd/ladix/ -count=1               # golden/негатив/run-golden зелёные
```
Проверить, что каждый новый пример есть в `examples/MANIFEST.md`; run-golden
метрика-триггера зелёный в любой день (фикс-`Clock`).
✅ SC-004/SC-005.

## Шаг 4 — пользовательский quickstart (US4)

```sh
test -f docs/quickstart.md && echo OK
```
Пройти путь `docs/quickstart.md` дословно из чистого клона (установка → `hello` →
метрика → процесс) — каждая команда exit 0, без обращения к `*-model.md`.
✅ SC-006.

## Шаг 5 — выравнивание доков (US5, контракт `docs-alignment.md`)

```sh
grep -rn '1\.22' README.md ARCHITECTURE.md            # 0 в командах сборки
grep -rn 'Найдено K ошибок' SPEC.md README.md          # 0
grep -n 'deferred до 006' examples/онбординг.ladix      # 0
cd src && go test ./internal/errors/ -run Aggregate     # замок «Найдено» зелёный
```
Прогнать `печать(тип(5))` → остаётся reserved-word ошибкой (поведение не изменено).
✅ SC-007.

## Шаг 6 — финальная приёмка (US6, §VF-8)

```sh
git status   # чисто; ./ladix и src/ladix не закоммичены (в .gitignore)
```
Свести: каждый из 9 пунктов рубрики §1–§9 закрыт доказательством файл:строка или
зелёным прогоном; финальный smoke (README + quickstart из корня чистого клона) +
`cd src && go test ./...` — exit 0; дерево чистое.
✅ SC-008.

## Сводная таблица приёмки

| Шаг | US | Проверка | SC |
|-----|-----|----------|-----|
| 0 | — | build/vet/test из `src/` | SC-002 |
| 1 | US1 | блоки README из корня → exit 0 + smoke | SC-001 |
| 2 | US2 | grep `[ ]`=0 / `[x]`=50 + доказательства | SC-003 |
| 3 | US3 | 7 классов + MANIFEST + замки + run-golden | SC-004/SC-005 |
| 4 | US4 | `docs/quickstart.md` проходится за 5 мин | SC-006 |
| 5 | US5 | grep снятых утверждений = 0; `тип(5)` reserved | SC-007 |
| 6 | US6 | рубрика §1–§9 закрыта; дерево чистое | SC-008 |
