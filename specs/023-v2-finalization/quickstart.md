# Quickstart (артефакт SpecKit): проверка фичи 023-v2-finalization

> **Это артефакт SpecKit** (`specs/023-v2-finalization/quickstart.md`) — как прогнать и проверить
> замки этой фичи. Это **НЕ** корневой `docs/quickstart.md` (тот вне границы фичи, правит архитектор).

**Якорь приёмки**: `docs/v2-finalization-model.md §F-1` (критерий приёмки) + §F-7 (финальный
прогон-чеклист). Все команды — из каталога `src/` (Go-модуль) либо из корня репо (для примеров/данных).

## 0. Предусловия
- Go 1.22+, ветка `023-v2-finalization`.
- `data/orders.csv` на месте (общая фикстура; не менять).

## 1. Пример парсится чисто (W1)
```bash
cd src && go test ./internal/parser/ -run TestExamplesParseCleanSet -count=1
```
Ожидание: PASS — `examples/контроль_плана.ladix` входит в чистый набор. Также вручную убедиться, что в
файле НЕТ `2500000` и `печать` (литеральный старт удалён):
```bash
grep -nE '2500000|печать' examples/контроль_плана.ladix   # из корня репо; ожидание: пусто
```

## 2. T-GOLD-METRIC — детерминированный golden аналитической половины (W2)
```bash
cd src && go test ./cmd/ladix/ -run T-?GoldMetric -count=1   # точное имя — impl-time
```
Ожидание: PASS. Пинит 3 фасета под `FixedClock{2026,6,15}` из repo-root:
- (i) скаляр `выручка_30д = 300000.0` (`runMetric`);
- (ii) строка explain `триггер 'выручка_30д < 3000000' сработал: выручка_30д = 300000.0 (снимок) <
  3000000 (порог) → истина` (`runFile`, RUN-путь);
- (iii) метрика-driven старт инстанса `p-000001` (RUN-путь).

Проверить, что старый `TestCLIGoldenDeadlineEscalation` удалён:
```bash
cd src && grep -rn 'TestCLIGoldenDeadlineEscalation' cmd/ladix/   # ожидание: пусто
```

## 3. T-LIFECYCLE — герметичный жизненный цикл (W3)
```bash
cd src && go test ./cmd/ladix/ -run T-?Lifecycle -count=1   # точное имя — impl-time
```
Ожидание: PASS. `start` → задача `t-000001` → `complete --данные '{"итог":"перезвонит"}'` → строка
эффекта `[уведомление] crm: итог звонка: перезвонит` → `инстанс p-000001: выполнен`; дат в выводе
эффекта нет (дедлайн маскируется `<DT>`).

## 4. Мутпробы — замки кусаются при инверсии (FR-010, W2/W3)
- Сорвать снимок (`300000.0`→`0.0`) или строку explain → T-GOLD-METRIC краснит.
- Сменить ключ payload (`итог`) или текст эффекта → T-LIFECYCLE краснит.
- После проверки откатить мутацию (дерево чистое).

## 5. Регресс-инварианты зелёные без правок (W4)
```bash
cd src && go test ./internal/daemon/ -run TestM2GoldenEndToEnd -count=1
cd src && go test ./... -run TestStepEffectExactlyOnceRestart -count=1
cd src && go test ./cmd/ladix/ -run 'Start|Inspect|Clock' -count=1
cd src && go test ./cmd/ladix/ -run TestCompleteClockInjected -count=1
```
Ожидание: все PASS. Терминальные гейты НЕ переписаны; `TestCompleteClockInjected` (гоняет догон-авто-
шаги) согласован с именами шагов/ключом payload.

## 6. Финальные гейты (W5, FR-013)
```bash
cd src && go build ./... && go vet ./... && gofmt -l . && go test ./... -count=1
```
Ожидание: build/vet чисто, `gofmt -l .` — пусто, `go test ./...` — exit 0 (10 пакетов). Рабочее дерево
чистое: сборочный бинарь (`src/ladix`) и `*.db`/`*.sqlite` не закоммичены (`.gitignore`).

## 7. Проверка границ (SC-008)
```bash
git status --porcelain   # из корня репо
```
Ожидание: изменены ТОЛЬКО `examples/контроль_плана.ladix` + Go-тесты в `src/cmd/ladix/` (+ артефакты
`specs/023-v2-finalization/`). НЕ тронуты: `SPEC.md`, `README.md`, `CHECKLIST.md`,
`examples/MANIFEST.md`, `docs/quickstart.md`, `docs/v2-charter.md`, любые `docs/*-model.md`. Прод-логика
(`internal/eval|engine|store|daemon`, не-тестовый `cmd/ladix/`) — ПУСТОЙ дифф; швы `ProcessRuntime`=8 /
`Store`=18 целы.
