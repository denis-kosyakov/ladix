# Specification Quality Checklist: Бэкенд триггеров — демон `serve`, события и edge-детект (007b)

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-06-13
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

- Контекст проекта: это спецификация ФИЧИ ИНФРАСТРУКТУРЫ ЯЗЫКА (демон-планировщик DSL Ladix). «Пользователь» = оператор CLI (`serve`/`emit`, сигнал остановки) / автор `.ladix`-файла; «система» = тулчейн + демон над общим хранилищем. Поэтому наблюдаемая поверхность (команды CLI и их флаги, наличие новой семош формата времени, exit-коды, формат id `e-NNNNNN`/`trg-<N>`, гарантии доставки at-most-once/at-least-once, грациозная остановка по сигналу ОС, durability под `--db`) — это WHAT-контракт и правомерно присутствует в спеке; внутренние механизмы (имена Store-методов, имена фаз `drainEvents`/`evalMetrics`/`checkSchedules`, структуры полей, файлы/строки) — HOW и в спеку выносятся лишь как опорные ориентиры якоря, не как требования к реализации.
- Источник истины — `docs/trigger-model.md §TR-11` (9 аддитивных швов), `docs/execution-model.md` EM-17/EM-11, обещание `Store` в `docs/engine-model.md`. Спека выведена из якорей и не вводит требований сверх них; инвариант границы §TR-11 (аддитивность, неизменность синтаксиса/AST/реестра диагностик 007a) зафиксирован отдельным FR-026.
- **Открытые решения §TR-11 закрыты на уровне требований без [NEEDS CLARIFICATION]**: (#2) стратегия пересчёта метрик между тиками — FR-005 + Assumptions, дефолт «явный метод сброса состояния интерпретатора», альтернатива «свежий интерпретатор per-tick» эквивалентна по контракту; (#6) семантика рестарт-скана и поведение при дрейфе `CurrentStep` — FR-019/FR-020 + Assumptions, дрейф → лог + залипание без срыва старта. Оба помечены «Решение (007b)».
- **Осознанное отступление от обещания «+6 методов Store»**: рестарт-скан добавляет седьмой метод `ListInstancesByStatus` — зафиксировано как deviation в FR-022 (со ссылкой `engine-model.md:829`) и в Assumptions, не как пробел.
- **SC-010** намеренно ссылается на инструментарий Go (`go build`/`go vet`/`gofmt`/`go test`): фактический приёмочный гейт проекта (как в прецедент-спеках 001–007a), а не имплементационная утечка. Сохранён осознанно.
- **Технические термины на поверхности (`time.Ticker`/`context.Context`/SIGINT/SIGTERM/WAL/SQLite/Go-duration)** упоминаются лишь там, где они образуют наблюдаемый операторский контракт (флаг `--interval` = Go-длительность; остановка = сигнал ОС; durability = SQLite под `--db`; кросс-процессная сериализация = WAL) — зеркало того, как 007a упоминал `p-NNNNNN`/`--db`. Способ реализации (горутины, мьютекс, имена методов) оставлен стадии plan.
- Граница 007a/007b строго проведена: единственная фронтенд-добавка 007b — новая семош валидации формата `"ЧЧ:ММ"` (FR-014), которой в реестре 007a НЕТ; всё прочее в 007a остаётся неизменным (FR-026, SC-010).
