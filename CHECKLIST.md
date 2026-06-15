# Чеклист готовности проекта

Используйте этот список перед сдачей.

## Документация

- [x] В `README.md` есть название языка. — `README.md:1`
- [x] В `README.md` есть описание идеи языка. — `README.md:5`
- [x] В `README.md` есть инструкция установки зависимостей. — `README.md:15`
- [x] В `README.md` есть инструкция запуска программы. — `README.md:35`
- [x] В `README.md` есть инструкция запуска тестов. — `README.md:59`
- [x] В `SPEC.md` описана грамматика языка. — `SPEC.md:80`
- [x] В `SPEC.md` описаны типы данных. — `SPEC.md:148`
- [x] В `SPEC.md` описаны области видимости. — `SPEC.md:243`
- [x] В `SPEC.md` описаны ошибки. — `SPEC.md:552`
- [x] В `SPEC.md` описаны ограничения языка. — `SPEC.md:538`

## Реализация

- [x] Есть лексер. — `src/internal/lexer/lexer.go`
- [x] Есть парсер. — `src/internal/parser/parser.go`
- [x] Есть AST или аналогичная внутренняя структура. — `src/internal/ast/node.go`
- [x] Есть интерпретатор или компилятор. — `src/internal/eval/interpreter.go`
- [x] Программу можно запустить из файла. — `src/cmd/ladix/main.go:168`
- [x] Код проекта структурирован и читаем. — `docs/STRUCTURE.md:1`

## Возможности языка

- [x] Есть переменные. — `src/internal/eval/stmt.go:14`
- [x] Есть присваивание. — `src/internal/eval/stmt.go:22`
- [x] Есть арифметические выражения. — `src/internal/eval/arith.go:42`
- [x] Есть логические выражения. — `src/internal/eval/arith.go:77`
- [x] Есть операции сравнения. — `src/internal/eval/arith.go:69`
- [x] Есть приоритет операторов. — `src/internal/parser/parse_expr.go:11`
- [x] Есть условный оператор. — `src/internal/eval/stmt.go:174`
- [x] Есть цикл. — `src/internal/eval/stmt.go:48`
- [x] Есть блоки кода. — `src/internal/eval/stmt.go:159`
- [x] Есть функции. — `src/internal/eval/call.go:49`
- [x] Есть параметры функций. — `src/internal/eval/call.go:67`
- [x] Есть возвращаемое значение. — `src/internal/eval/call.go:49` · `examples/функция.ladix:4`
- [x] Есть локальные переменные. — `src/internal/eval/call.go:66`
- [x] Есть вывод данных. — `src/internal/eval/builtins.go:60`

## Ошибки

- [x] Синтаксические ошибки показываются понятно. — `src/internal/errors/parserror.go:27`
- [x] Ошибки содержат номер строки. — `src/internal/errors/position.go:11` · `SPEC.md:557`
- [x] Ошибки типов показываются понятно. — `src/internal/errors/typeerror.go:19`
- [x] Ошибка неизвестной переменной показывается понятно. — `src/internal/eval/expr.go:113`
- [x] Обычная ошибка пользователя не приводит к внутреннему stack trace реализации. — `src/cmd/ladix/main.go:477` · `SPEC.md:581`

## Примеры

- [x] Есть `examples/hello.lang`. — `examples/hello.ladix` (факт. расширение .ladix)
- [x] Есть пример арифметики. — `examples/арифметика.ladix:2`
- [x] Есть пример условия. — `examples/условие.ladix:5`
- [x] Есть пример цикла. — `examples/цикл.ladix:3`
- [x] Есть пример функции. — `examples/функция.ladix:2`
- [x] Есть пример рекурсии. — `examples/факториал.ladix:2`
- [x] Есть пример ошибочной программы. — `examples/ошибка.ladix:5`

## Тесты

- [x] Есть тесты лексера. — `src/internal/lexer/lexer_test.go:8`
- [x] Есть тесты парсера. — `src/internal/parser/parse_stmt_test.go:39`
- [x] Есть тесты выражений. — `src/internal/eval/expr_test.go:12`
- [x] Есть тесты переменных. — `src/internal/eval/stmt_test.go:42`
- [x] Есть тесты условий. — `src/internal/eval/stmt_test.go:68`
- [x] Есть тесты циклов. — `src/internal/eval/stmt_test.go:103`
- [x] Есть тесты функций. — `src/internal/eval/call_test.go:6`
- [x] Есть тесты ошибок. — `src/internal/errors/evalerrors_test.go:11`
