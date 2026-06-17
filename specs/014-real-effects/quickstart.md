# Quickstart: проверка B2 (реальные эффекты `вызвать` / `уведомить`)

Команды из корня репозитория. Сборка/тесты — из `src/` (go.mod там). Сборка — для L1-Реализации; на фазе спецификации бинарника ещё нет.

## Сборка
```bash
cd src && go build -o ../ladix ./cmd/ladix && cd ..
```

## B2 — дефолт-стаб без `--вебхук` (§EN-7 golden цел)

БЕЗ флага/env эффекты печатаются стабом байт-в-байт как в v1; сеть не трогается.

```bash
cat > /tmp/b2.ladix <<'EOF'
процесс демо():
    шаг проверка:
        уведомить ИТ: создать учётку
        присвоить ответ = вызвать crm("клиент")
        печать(ответ)
EOF
./ladix run /tmp/b2.ladix; echo "exit=$?"
# ожидаем: «[уведомление] ИТ: создать учётку», «[вызов] crm(клиент)», печать «Пусто»; exit 0
```

## B2 — реальная доставка через `--вебхук` (httptest или локальный приёмник)

С флагом эффекты идут `POST` на URL; результат `вызвать` = декодированный ответ.

```bash
# приёмник, печатающий тело и возвращающий JSON-объект (пример на любом стеке):
#   POST / → читает {"цель":"crm","данные":["клиент"]}, отвечает {"статус":"ок"}
./ladix run /tmp/b2.ladix --вебхук http://localhost:8080/hook; echo "exit=$?"
# ожидаем: приёмник получил POST {"цель":"ИТ","данные":["создать учётку"]} и
#          {"цель":"crm","данные":["клиент"]}; печать «ответ» = Запись {статус: ок}
# стаб НЕ печатается (вывод идёт на вебхук)
```

Через env (без флага):
```bash
LADIX_WEBHOOK=http://localhost:8080/hook ./ladix run /tmp/b2.ladix; echo "exit=$?"
# эффект идёт на URL из env
```

## B2 — невалидный URL вебхука (CLI-ошибка, exit 2)

```bash
./ladix run /tmp/b2.ladix --вебхук '://мусор'; echo "exit=$?"
# stderr ровно: ladix: неверный URL вебхука '://мусор'
# exit=2, stdout пуст, движок не запускается
```

## B2 — реальная доставка под `serve` (эскалация дедлайна на вебхук)

```bash
./ladix start /tmp/эскалация.ladix --db /tmp/b2.db
./ladix serve /tmp/эскалация.ladix --db /tmp/b2.db --вебхук http://localhost:8080/hook --interval 1s
# при наступлении дедлайна тело эскалации (уведомить/вызвать) идёт POST на вебхук,
# а НЕ печатается стабом (тот же экземпляр движка, §AU-4.5 / §AU-12.C)
```

## Контрольный httptest-сниппет (форма теста для L1-Реализации)

```go
// caller_test.go / webhook_cli_test.go — реальный POST под net/http/httptest (stdlib)
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    body, _ := io.ReadAll(r.Body)
    // require: r.Method == POST; r.Header.Get("Content-Type") == "application/json"
    // require: string(body) == `{"цель":"crm","данные":["клиент"]}`
    w.Write([]byte(`{"статус":"ок"}`))   // ответ → Запись
}))
defer srv.Close()
caller := webhookCaller{baseURL: srv.URL, httpClient: srv.Client()}
got, err := caller.Call("crm", []value.Value{value.Строка("клиент")})
// require: err == nil; got == Запись{статус: "ок"}
```

## Тесты
```bash
cd src && go test ./internal/engine/... ./internal/jsonval/... ./internal/eval/... ./cmd/ladix/... -count=1
# зелёные: §EN-7 golden (дефолт-стаб), httptest POST/декод/пустое тело/сбой, CLI-ошибка URL,
#          контракт ExternalCaller/Option, кодек типов; шов ProcessRuntime = 8 (без изменений)
```

## Что проверяет каждый пункт

| Пункт | US | FR | SC |
|---|---|---|---|
| дефолт-стаб без флага | US1 | FR-004/005 | SC-001 |
| `--вебхук` POST + декод ответа | US2 | FR-006/007/008 | SC-002/003 |
| env `LADIX_WEBHOOK` | US4 | FR-015 | SC-005 |
| невалидный URL → ошибка | US4 | FR-016 | SC-006 |
| `serve` на вебхук | US4 | FR-017 | SC-005 |
| сбой → ОшибкаВыполнения | US3 | FR-012/013 | SC-004 |
