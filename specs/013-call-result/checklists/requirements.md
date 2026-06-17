# Specification Quality Checklist: B1 «Захват результата `вызвать` как выражения»

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-06-17
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

- Спека ссылается на внутренние артефакты (`CallExpr`/`RunProcessExpr` в `ast/expr.go`, `ProcessRuntime`, `runtimeErrWrap`, `startsExpression`) как на КОНТЕКСТ/ИСТОЧНИК ИСТИНЫ (`docs/automation-model.md` §AU-3, D-AU-1 — решение залочено владельцем), а не как на самостоятельное предписание реализации — это требование Принципа IX конституции (спека-источник истины опирается на размещённые доки). Точные имена (`CallExternalExpr`, `CallExternalResult`) перенесены из §AU-3 дословно, т.к. они структурно несущие (имя `CallExpr` занято → redeclared без точного имени).
- Все 15 FR имеют измеримые критерии приёмки (SC-001..007) и привязаны к acceptance-сценариям US1; счётчик шва (7→8) и `value.None`-возврат под стабом — эмпирически сверены с master @95f61e7 (`eval/runtime.go` 7 методов, `value.None` синглтон `scalar.go:33`).
- Краевые кейсы (имя `CallExpr` занято; развязка statement↔выражение без неоднозначности; `уведомить` только statement; located-ошибка недостижима под стабом; golden §EN-7) идентифицированы и закрыты решениями §AU-3, не оставлены открытыми.
- Все пункты пройдены с первой итерации; [NEEDS CLARIFICATION] не введены — развилка §8 закрыта владельцем (D-AU-1 = вариант «б»), отражена в Assumptions/Контексте.
