# Quickstart: проверка M-DX

Команды из корня репозитория. Сборка/тесты — из `src/` (go.mod там).

## Сборка
```bash
cd src && go build -o ../ladix ./cmd/ladix && cd ..
```

## DX1 — каскад схлопнут до 1 (ручная проверка)
```bash
printf 'пусть x = если\n'                 > /tmp/a.ladix && ./ladix run /tmp/a.ladix; echo "exit=$?"   # ожидаем 1 диагностику, exit 1
printf 'если вернуть:\n    печать(1)\n'    > /tmp/b.ladix && ./ladix run /tmp/b.ladix; echo "exit=$?"   # ожидаем 1 диагностику
printf 'значение\n{\n'                     > /tmp/c.ladix && ./ladix run /tmp/c.ladix; echo "exit=$?"   # КОНТРОЛЬ: ожидаем 2 диагностики
```
Критерий: a→1, b→1, c→2; ни в одном выводе нет Go stack trace и строки «Найдено K ошибок».

## DX2 — нет жаргона наружу
```bash
./ladix run /tmp/a.ladix 2>&1 | grep -E 'токен|литерал|L-|SE-' && echo "ПРОВАЛ: жаргон/код наружу" || echo "OK: чисто"
```

## Тесты и гейты
```bash
cd src
gofmt -l .                       # пусто
go vet ./...                     # без замечаний
go test ./... -count=1           # все зелёные
go test ./internal/parser/ -run 'TestRecover|TestCascade|TestMultipleIndependent|Golden' -count=1 -v
go test ./internal/lexer/ -run 'Catalog|Inventory' -count=1
go build ./...                   # 0
cd ..
```

## Мутант-доказательство (DX1)
1. Временно откатить consume-before-error в `parsePrimary` default (`parse_expr.go`).
2. `cd src && go test ./internal/parser/ -run Cascade -count=1` → каскадные замки ДОЛЖНЫ упасть (2/4 вместо 1).
3. Контроль `значение⏎{` остаётся 2 при обоих состояниях.
4. Вернуть фикс — зелёные.

## Инвариант бэкенда (пустой дифф)
```bash
git diff --stat master -- src/internal/eval src/internal/engine src/internal/store   # пусто
grep -n 'type ProcessRuntime interface' src/internal/eval/runtime.go 2>/dev/null || true  # интерфейс ProcessRuntime (7 методов) объявлен в eval, реализован *Engine в engine
# ProcessRuntime = 7 методов, Store = 15 методов — не изменены (доказывается пустым дифф-стэтом выше)
```

## Витрина
```bash
ls examples/ | grep -E 'ошиб'   # ошибочная.ladix и спутники НЕ перезаписаны; новые кейсы — отдельными файлами
```

## Полнота каталога
```bash
cd src && go test ./internal/lexer/ -run Inventory -count=1   # len(seen)!=11 — зелёный
go test ./internal/parser/ -run Inventory -count=1            # len(seen)!=14 — зелёный
go test ./internal/eval/ -run Registry -count=1               # len(seen)!=28 — НЕ тронут, зелёный
```
