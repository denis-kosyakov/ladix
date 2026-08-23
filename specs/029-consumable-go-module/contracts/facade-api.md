# Contract: Публичный фасад `ladix` (facade-api)

Контракт для стадий **plan → implement → verify**. Описывает узкую точку входа
потребителя (платформа «Уклад», вариант B): корневой пакет `ladix` с двумя функциями
компиляции исходника в версионируемый `ir.Program`. Это **semver-поверхность** библиотеки
`github.com/denis-kosyakov/ladix`. Реализует FR-005 / FR-007; опирается на `ir` (см.
`ir-schema.md`) и страж границы (см. `import-boundary.md`).

## Сигнатуры (semver-поверхность)

```go
package ladix

// Compile разбирает и валидирует исходник LADIX, понижая его в стабильный IR.
func Compile(source string) (program *ir.Program, diags []ir.Diagnostic, err error)

// CompileFile читает файл .ladix и компилирует его содержимое (эквивалент Compile).
func CompileFile(path string) (program *ir.Program, diags []ir.Diagnostic, err error)
```

Любое изменение этих сигнатур (имена, порядок, типы, число параметров/возвратов) — это
**MAJOR** библиотеки. Добавление новых функций/типов рядом — MINOR (аддитивно).

## Контракт поведения (вход → `program` / `diags` / `err`)

| # | Вход | `program` | `diags` | `err` |
|---|------|-----------|---------|-------|
| **C1** | Валидный `source` | `!= nil`, `SchemaVersion == 1` | пуст ИЛИ без записей `Severity == "error"` | `nil` |
| **C2** | `source` с пользовательскими ошибками (лексика / синтаксис / семантика) | `== nil` | `≥1` запись `Severity == "error"`, `Message` **дословно** SPEC §13, `Pos` заполнена | `nil` |
| **C3** | `CompileFile`: ошибка чтения/IO файла | `== nil` | `nil` (или пуст) | `!= nil` (внутренний сбой) |
| **C4** | Внутренняя паника фронтенда (recover-барьер) | `== nil` | `nil` (или пуст) | `!= nil` (внутренний сбой) |

Ключевое различение (FR-005): **пользовательские ошибки исходника никогда не маскируются
под `err`** — они всегда едут в `diags` уровня `error` при `err == nil`. `err != nil` —
исключительно внутренний сбой (IO в `CompileFile`, перехваченная паника).

## Инвариант фасада

> `program != nil` **⟺** среди `diags` нет записи с `Severity == "error"`.

Двусторонний: наличие хотя бы одной `error`-диагностики ⟹ `program == nil`; и наоборот,
непустой `program` ⟹ в `diags` нет `error`. Записи иных severity в v1 не предусмотрены
(`severity ∈ {"error"}`, см. `ir-schema.md`), но инвариант формулируется устойчиво к их
будущему добавлению (warning/info не обнуляют `program`).

## Внутренняя реализация (для имплементатора, НЕ часть semver-контракта)

`Compile(source)` собирается из существующего фронтенда без изменения семантики
(FR-015), без глобального состояния (Принцип V):

```go
toks := lexer.New(source).Tokenize()
prog, parseErrs := parser.New(toks, errs).Parse()
interp := eval.NewInterpreter(io.Discard, defaultDepth, systemClock)
analysisErrs := interp.Analyze(prog)          // семантическая валидация, вывод в io.Discard
program := lowerProgram(prog)                  // понижение ast.Program → *ir.Program
// сбор lex/parse/semantic-ошибок → []ir.Diagnostic (Stage + дословный Message)
```

- **`eval` остаётся `internal`.** Фасад вызывает `eval.NewInterpreter(...).Analyze(...)`
  только ради семантической валидации; результат исполнения не нужен (вывод глушится в
  `io.Discard`). Это допустимо для границы: `eval` sqlite-free (см. `import-boundary.md`).
- **Без глобального состояния.** Каждый вызов `Compile` создаёт собственные lexer/parser/
  interpreter; никаких пакетных `var` (Принцип V).
- **`CompileFile(path)`** = `os.ReadFile(path)` → при IO-ошибке вернуть `err != nil` ДО
  компиляции; иначе `Compile(string(bytes))`.
- **recover-барьер** оборачивает тело `Compile`: перехваченная паника превращается в
  `err != nil` (C4), а не в краш потребителя.

## Стабильность

- Сигнатуры `Compile` / `CompileFile` и форма `ir.*` — публичная поверхность semver.
- Изменение сигнатур фасада = **MAJOR**.
- Изменение формы IR регулируется `ir.SchemaVersion` (см. `ir-schema.md`).
- Первый релиз — `v0.1.0` (FR-016).
