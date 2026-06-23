# Quickstart: Харденинг числовых путей движка метрик (028)

Все команды — из каталога `src/` (раскладка проекта; фикстура `examples/data/sales.json`
резолвится тестами через `salesPath()`/`SetSourceBase`).

## Прогон проверок

```sh
cd src

# Форматирование (Конституция I) — должно быть пусто
gofmt -l .

# Сборка + статанализ — без ошибок (страхует рейминг B1/B2: символ найден)
go build ./...
go vet ./...

# Целевые пакеты фичи (быстрый цикл при разработке тестов)
go test -race ./internal/eval/ ./internal/jsonval/

# Полный гейт (SC-001)
go test -race ./...
```

Ожидаемо: всё зелёное. Новые тесты A1–A7 проходят; существующие (`TestGoldenSM10`,
`TestPayloadToRecordValueTypes`, `§SM-9` edge-набор) НЕ ломаются (прод-поведение прежнее,
FR-012/SC-005).

## Запуск отдельных замков

```sh
cd src

# A1–A5 (числовые ветки через дериватив метрики)
go test -race ./internal/eval/ -run 'TestMetricDivByZero|TestMetricIntOverflow|TestMetricUnaryNeg|TestMetricFloatSpecials|TestMetricNoneOperand' -v

# A6 (строгий sourceNumberToValue — целое вне int64)
go test -race ./internal/eval/ -run TestSourceIntOutOfRange -v

# A7 (толерантный payloadNumberToValue — ±Inf, без деградации в None)
go test -race ./internal/jsonval/ -run TestPayloadNumberInfinity -v
```

## Проверка покрытия combineUnary (SC-002)

`combineUnary` был 0% — A3 должен его реально прогнать:

```sh
cd src
go test ./internal/eval/ -run 'TestMetricUnaryNeg' -coverprofile=/tmp/cov.out
go tool cover -func=/tmp/cov.out | grep -i 'combineUnary'
# combineUnary должен показать > 0% (ветки OpNeg: Целое-MinInt64 и Дробное прогнаны)
```

## Проверка мутационной чувствительности (SC-004)

Каждый замок обязан **краснеть при удалении соответствующего гарда**. Контрольные мутации
(вносить во временную копию/worktree, прогнать тест, убедиться, что он КРАСНЕЕТ, затем
откатить — НЕ коммитить мутацию):

| Замок | Мутация в прод-коде | Ожидаемо |
|---|---|---|
| A1-div | `arith.go:163` `evalDiv`: убрать `if rf == 0 {…}` | `TestMetricDivByZero` КРАСНЕЕТ (получит `+Inf` вместо ошибки) |
| A1-floordiv/mod | `arith.go:177`/`:192`: убрать `if ri.V == 0 {…}` | соответствующая строка таблицы КРАСНЕЕТ (паника вместо ошибки) |
| A2-add | `arith.go:249` `addInt64`: вернуть `s, false` всегда | `TestMetricIntOverflow` КРАСНЕЕТ (wrapped число) |
| A3-neg-min | `metric_engine.go:289`: убрать `if v.V == math.MinInt64 {…}` | `TestMetricUnaryNegOverflow` КРАСНЕЕТ (вернёт MinInt64) |
| A4 | `arith.go:149` `evalSubMul`: перехватить `±Inf`→ошибка | `TestMetricFloatSpecials` КРАСНЕЕТ (`IsInf`/`IsNaN` не выполнится) |
| A5 | `metric_engine.go:295` default: заменить на возврат `value.None` | `TestMetricNoneOperand` КРАСНЕЕТ (нет typeErr) |
| A6 | `source_loader.go:221`: вернуть `Дробное`, как толерантный | `TestSourceIntOutOfRange` КРАСНЕЕТ (нет ошибки `§SM-9.B`) |
| A7 | `decode.go:145`: вернуть `value.None` на overflow | `TestPayloadNumberInfinity` КРАСНЕЕТ (None != Дробное) |
| Рейминг | НЕ обновить call-site после переименования | `go build ./...` КРАСНЕЕТ (символ не найден) |

> Изолировать мутацию `git worktree`/копией: параллельный мутатор в общем чекауте портит
> конкурентные `go test` прогоны (см. memory: review-mutation-shared-worktree).

## Что НЕ должно измениться (SC-005)

```sh
cd src
# combine*/arith прод-логика — байт-в-байт прежняя:
git diff --stat internal/eval/metric_engine.go internal/eval/arith.go
# ОЖИДАЕМО: пусто (combineBinary/combineUnary/arith не тронуты).
# Единственный прод-дифф source_loader.go/decode.go — рейминг + перекрёстные комментарии.
```
