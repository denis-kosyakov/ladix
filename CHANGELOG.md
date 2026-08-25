# Changelog

Формат — [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/), версии — [semver](https://semver.org/lang/ru/)
(ось модуля, `vX.Y.Z`; независимая ось схемы IR — `ir.SchemaVersion`, см.
`docs/module-contract.md` §MC-5).

## [Не выпущено]

### v0.2.0 — публичный исполнитель метрик

#### Added
- Третий публичный пакет `github.com/denis-kosyakov/ladix/metrics` — исполнитель
  metrics-подмножества IR над данными потребителя: `Evaluate(ir.Metric,
  []map[string]any, Options) (Result, []ir.Diagnostic, error)`. Вычисляет метрику
  (`где:`/`агрегат:`/`период:`/`по_дате:`) по каноническим строкам `ir.Metric`,
  байт-в-байт совпадающе с `ladix metric`, без файла источника и без SQLite.
  Контракт — `docs/module-contract.md` §MC-8; дельта спеки —
  `openspec/specs/metrics-evaluator/spec.md`.
- Новое значение `stage:"runtime"` в словаре `ir.Diagnostic.Stage` — стадия
  исполнения метрики публичным исполнителем. Аддитивно, `ir.SchemaVersion` не
  меняется (`docs/module-contract.md` §MC-3/§MC-5).
- Четыре новых дословных текста диагностик: два потолка сложности (глубина,
  число узлов), недопустимое каноническое выражение и неподдерживаемый тип
  значения в записи потребителя — `docs/diagnostics-model.md` §MDX-2.

#### Changed
- Ничего ломающего. `ladix` и `ir` этим релизом НЕ затронуты — их дифф пуст.
  `ir.SchemaVersion` остаётся `1`.

## [v0.1.0] — первый релиз потребляемого Go-модуля

### Added
- Публичная поверхность: пакет `ladix` (`Compile`/`CompileFile`) и пакет `ir`
  (`SchemaVersion == 1`). `go.mod` перенесён в корень репозитория,
  module-path `github.com/denis-kosyakov/ladix`.
- Инвариант «потребитель библиотеки не тянет `modernc.org/sqlite` и
  `internal/{store,engine,daemon}`» закреплён стражем `boundary_test.go`.
- Политика версионирования модуля и схемы IR — `README.md` «Версионирование и
  совместимость», `docs/module-contract.md` §MC-5.

---

Более ранние версии (до `v0.1.0`, язык и CLI Ladix) — см. историю git
(`git log`); отдельного тега для них нет.
