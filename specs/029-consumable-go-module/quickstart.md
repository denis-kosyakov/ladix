# Quickstart: Потребляемый Go-модуль LADIX (029)

Две аудитории: **потребитель библиотеки** (платформа «Уклад») и **мейнтейнер LADIX**
(standalone reference-implementation после переезда модуля в корень). В конце — чеклист
приёмки (SC из spec).

---

## Раздел 1. Потребитель библиотеки (платформа «Уклад», go 1.23)

Уклад исполняет бизнес-определения нативно (вариант B) и подключает LADIX только как
парсер/валидатор/нормализатор с пином версии через semver. SQLite, движок процессов и демон
в его сборку НЕ затягиваются.

### Подключение

```sh
go get github.com/denis-kosyakov/ladix@v0.1.0
```

`go.mod` лежит в корне репозитория, module-path — `github.com/denis-kosyakov/ladix`
(без сегмента `/src`), go-директива — `1.23`.

### Минимальный пример: компиляция исходника

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/denis-kosyakov/ladix"
	"github.com/denis-kosyakov/ladix/ir"
)

func main() {
	source := `метрика выручка = сумма(заказ.сумма)`

	program, diags, err := ladix.Compile(source)
	if err != nil {
		// Внутренний сбой (НЕ ошибка компиляции пользователя).
		fmt.Fprintln(os.Stderr, "ladix: внутренний сбой:", err)
		os.Exit(1)
	}

	if len(diags) > 0 {
		// Пользовательские диагностики: тексты дословно русские из SPEC §13.
		for _, d := range diags {
			fmt.Fprintf(os.Stderr, "%s [%d:%d] %s\n", d.Stage, d.Pos.Line, d.Pos.Col, d.Message)
		}
		// program == nil ⟺ среди diags есть элемент уровня error.
		if program == nil {
			os.Exit(2)
		}
	}

	// Валидная программа: работаем с IR.
	fmt.Println("schema_version:", program.SchemaVersion) // == 1
	fmt.Println("метрик:", len(program.Metrics))
	fmt.Println("процессов:", len(program.Processes))
	fmt.Println("триггеров:", len(program.Triggers))

	_ = ir.SchemaVersion // == 1, сверка версии контракта на стороне потребителя
}
```

Контракт фасада (FR-005):

- `program != nil` ⟺ среди `diags` нет элемента с `Severity == error`;
- `err != nil` **только** при внутреннем сбое (например, ошибка чтения файла), а НЕ при
  ошибке компиляции пользователя.

### Компиляция из файла

```go
program, diags, err := ladix.CompileFile("определения.ladix")
// Эквивалентно Compile над содержимым файла; при ошибке чтения файла err != nil.
```

### Сериализация IR в JSON

`*ir.Program` сериализуем напрямую; все теги — `snake_case`, версия схемы в `schema_version`.

```go
blob, _ := json.MarshalIndent(program, "", "  ")
fmt.Println(string(blob))
// {
//   "schema_version": 1,
//   "metrics": [ ... ],
//   "processes": [ ... ],
//   "triggers": [ ... ]
// }
```

В IR `SchemaVersion == 1` выражения представлены **каноническими строками** (как
`ast.CanonicalTriggerCondition`); структурное представление — будущий bump `SchemaVersion`.

### Проверка, что SQLite НЕ затянут (SC-003)

```sh
# Из проекта-потребителя, импортирующего только ladix и ir:
go list -deps github.com/denis-kosyakov/ladix | grep -c modernc.org/sqlite
# ОЖИДАЕМО: 0
```

Аналогично в замыкании нет пакетов `internal/{store,engine,daemon}` — граница удерживается
тестом-стражем `boundary_test.go` на стороне LADIX.

---

## Раздел 2. Мейнтейнер LADIX (модуль в КОРНЕ репозитория)

После рефактора `go.mod` лежит в корне; все команды выполняются **из корня** (раньше — из `src/`).

### Сборка бинарника

```sh
# Новый путь (модуль в корне):
go build -o ladix ./cmd/ladix

# Раньше было: cd src && go build -o ../ladix ./cmd/ladix
```

### Тесты и статический анализ

```sh
go test ./...
go vet ./...
gofmt -l .   # должно быть пусто
```

### Запуск примера

```sh
./ladix run examples/hello.ladix
```

### Новые проверки фичи

```sh
# Тест-страж границы фронтенд↔backend (SQLite/internal не протекли):
go test -run TestImportBoundary ./...

# Golden-тесты фасада Compile (дословность диагностик §13, SchemaVersion == 1):
go test -run TestCompile ./...

# Unit публичного пакета ir:
go test ./ir/...
```

### Публикация версии

```sh
git tag v0.1.0
git push <remote> v0.1.0
```

Политика semver (FR-016):

- версии языка **аддитивны** — новые возможности не ломают существующий синтаксис;
- `ir.SchemaVersion` bump'ится **только** при breaking-изменении формата IR (удаление/
  переименование поля, смена типа/семантики, переход выражений из канонических строк в
  структурное представление);
- первый релизный тег — `v0.1.0`.

---

## Раздел 3. Критерии приёмки (чеклист соответствия SC)

| # | Критерий | Проверка |
|---|---|---|
| SC-001 | `go get` работает с корня | `go.mod` в корне; `module github.com/denis-kosyakov/ladix` (без `/src`); module-path стабилен |
| SC-002 | Сборка/анализ/тесты зелёные из корня | `go build ./...`, `go vet ./...`, `go test ./...` зелёные; `gofmt -l .` пусто |
| SC-003 | Публичное замыкание чистое | `go list -deps` для `ladix`/`ir` НЕ содержит `modernc.org/sqlite` и `internal/{store,engine,daemon}`; `boundary_test.go` краснеет при протечке |
| SC-004 | Валидный исходник → IR | `Compile` валидной программы → `*ir.Program`, `SchemaVersion == 1`, `err == nil`, без diags уровня error |
| SC-005 | Невалидный исходник → диагностики | `Compile` невалидной программы → `program == nil`, `[]ir.Diagnostic` с `Message` **дословно** из SPEC §13 |
| SC-006 | go-директива == 1.23 | `go.mod` `go 1.23`; README и конституция согласованы (расхождение `1.25.0` устранено) |
| SC-007 | Первый релиз | тег `v0.1.0` по semver; определён `ir.SchemaVersion == 1` |
| SC-008 | Standalone не сломан | CLI/движок/демон/стор работают; все ~25k LOC тестов зелёные; ни один тест не удалён |
| SC-009 | 0 новых сущностей языка | нет новых KW/builtins/операторов/кодов eval/wire-ключей; тексты диагностик и числовая модель неизменны; фронтенд не добавил внешних зависимостей |
