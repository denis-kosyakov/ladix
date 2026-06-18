# Implementation Plan: Unified clock across all CLI paths (C4)

**Branch**: `021-unified-cli-clock` | **Date**: 2026-06-18 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/021-unified-cli-clock/spec.md`

**Authority**: `docs/reliability-model.md` §C-4 (problem §C-4.1, recipe §C-4.2, test-locks §C-4.3, boundary §C-4.4) + §C-0/§C-6/§C-7. This anchor carries the concrete code recipe; this plan transcribes it into design artifacts only (no Go is written here).

## Summary

The `serve`/`emit` CLI paths already run from a **single injectable `engine.Clock`**: the adapter `evalClockFromEngine` (in `cmd/ladix/serve.go:32-38`) feeds the same clock to the interpreter's metric-date layer while `engine.WithClock` feeds it to the engine's lifecycle timestamps. Every other path — `run`, `start`, `complete`, `tasks`, `metric` — instead constructs **independent real (`engine.SystemClock{}`) clocks** in two or three separate spots, so "now" can disagree within one invocation and the paths cannot be tested deterministically.

C4 unifies time: (1) **extract** `evalClockFromEngine` from `serve.go` into a shared file in package `cmd/ladix` so all paths share one adapter type (behaviorally identical; `serve` keeps using it unchanged); (2) **rewire** `run`/`start`/`complete`/`tasks`/`metric` to accept and thread **one** `engine.Clock` (prod = `engine.SystemClock{}`) into both the interpreter (via `evalClockFromEngine{clock}`) and the engine (via `engine.WithClock(clock)`), with any summary "now" read from `clock.Now()`. The diff is **strictly confined to `src/cmd/ladix/`**; `eval`/`engine`/`store`/`daemon` keep an **empty diff** because the existing `engine.Clock` interface and `engine.WithClock` option are reused unchanged. `serve`/`emit` are not touched (`serve` is locked by `serve_golden_test.go:216`). Monotonic clocks are deferred (§C-9/§C-4.4).

## Technical Context

**Language/Version**: Go 1.22+ (CGO disabled; single static `ladix` binary). Per constitution principle I.

**Primary Dependencies**: Standard library only for this feature. Reuses existing in-tree packages: `internal/engine` (`engine.Clock`, `engine.SystemClock`, `engine.WithClock`, `engine.NewEngine`), `internal/eval` (`eval.NewInterpreter`, `eval.Clock`), `internal/value` (`value.Дата`). **Zero new dependencies** (the lone storage dependency `modernc.org/sqlite` is untouched).

**Storage**: N/A for this feature — `internal/store` has an **empty diff**. Store contract stays at 18 methods with the double compile-lock intact.

**Testing**: `go test ./...` (table-driven; `testify/require` allowed pointwise). New tests live in `src/cmd/ladix/*_test.go`. Build/verify: `cd src && go build ./...` then `cd src && go test ./...`.

**Target Platform**: Cross-platform CLI (Linux/macOS/Windows), single static binary.

**Project Type**: Single-project Go CLI + interpreter (`cmd/ladix/` entrypoint, `internal/` packages). Standard layout per constitution principle VII.

**Performance Goals**: No performance dimension — this is a refactor-to-determinism. Goal is byte-identical production behavior with full time-determinism under an injected fixed clock.

**Constraints**: Diff strictly inside `src/cmd/ladix/`; **empty diff** in `eval`/`engine`/`store`/`daemon`; no signature change to `engine.Clock` / `engine.WithClock`; ProcessRuntime stays 8 methods; Store stays 18 methods; 0 new keywords / SE-codes / eval-codes / builtins / dependencies; full determinism; `serve` clock-path byte-intact (golden `serve_golden_test.go:216` green; fake `fixedClock` `serve_golden_test.go:21-23` keeps compiling). Wall-clock only — monotonic clocks NOT introduced.

**Scale/Scope**: 5 rewired CLI paths (`run`, `start`, `complete`, `tasks`, `metric`) + 1 adapter extraction. Touched files: `cmd/ladix/serve.go` (remove adapter only), `cmd/ladix/main.go` (run/complete/tasks/metric), `cmd/ladix/start.go` (start), one new `cmd/ladix/clock_adapter.go`, and new `cmd/ladix/*_test.go` test-locks. No example/golden churn expected (production time behavior preserved).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| # | Principle | Verdict | Notes |
|---|-----------|---------|-------|
| I | Язык и сборка (Go 1.22+, no CGO, no new deps) | **PASS** | Stdlib + existing in-tree packages only. 0 new dependencies. `gofmt`/`go vet` clean expected. |
| II | Парсинг — ручной | **PASS** | No lexer/parser change; feature is CLI wiring only. |
| III | Ошибки — явные типы; recover-барьер на CLI-границе | **PASS** | No error-model change; existing per-subcommand recover barriers untouched. Clock threading adds no panics. |
| IV | Позиции — сквозные | **PASS** | No diagnostics/positions affected. |
| V | Без глобального состояния | **PASS** | **Strengthens** the principle: clock is injected as a parameter, replacing inline `engine.SystemClock{}` literals (a form of hidden ambient state) with explicit dependency injection — exactly the constitution's "dependencies MUST be injected" mandate. |
| VI | Тесты — вперёд (лексер и парсер) | **PASS** | Lexer/parser unaffected. Per-path fixed-clock test-locks + inversions + serve-unchanged guard authored with the change (§C-4.3); tests are part of each task. |
| VII | Раскладка проекта (стандартная, ацикличный граф) | **PASS** | New `clock_adapter.go` stays in `cmd/ladix/`; import direction unchanged (`cmd/ladix` already imports `engine`/`eval`). No cycles; leaf packages untouched. |
| VIII | Язык сообщений (русский, дословный §13) | **PASS** | No user-facing message text changes. |
| IX | Спека — источник истины | **PASS** | Behavior fully specified by `docs/reliability-model.md` §C-4; no undocumented decisions — everything resolved from the anchor. |

**Result: 9/9 PASS.** No violations → **Complexity Tracking is empty**. The one nuance (metric-path `engine.WithClock` has a latent/no-observable-effect today, done for completeness) is a benign over-inclusion, not a constitution deviation; it is documented in research.md, not as a complexity entry.

## Project Structure

### Documentation (this feature)

```text
specs/021-unified-cli-clock/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (CLI clock-injection contract)
│   └── clock-injection.md
├── checklists/
│   └── requirements.md  # Spec quality checklist (from /speckit-specify)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
src/
├── cmd/ladix/                  # ◀ ONLY package with a diff
│   ├── serve.go                # REMOVE local evalClockFromEngine (lines ~32-38); serve still uses shared one
│   ├── main.go                 # run (~251/255/277), complete (~446/451), tasks (~559), metric (~296/319) rewired to one clock
│   ├── start.go                # start (~133/138) rewired to one clock
│   ├── clock_adapter.go        # NEW: shared evalClockFromEngine{engine.Clock} adapter (moved verbatim)
│   ├── serve_golden_test.go    # UNCHANGED — lock at :216; fixedClock fake :21-23 must keep compiling
│   └── clock_unify_test.go     # NEW: per-path fixed-clock injection + inversion test-locks (§C-4.3)
│
└── internal/                   # EMPTY DIFF (reused unchanged)
    ├── engine/                 # engine.Clock, SystemClock, WithClock, NewEngine — signatures intact
    ├── eval/                   # eval.NewInterpreter, eval.Clock — intact
    ├── store/                  # contract 18 methods, double compile-lock — intact
    └── daemon/                 # intact
```

**Structure Decision**: Single Go project, standard layout (constitution VII). The entire diff is confined to the `src/cmd/ladix/` package — adapter extraction (`clock_adapter.go`), five path rewirings (`serve.go` adapter removal, `main.go`, `start.go`), and new test-locks. No `internal/*` package is modified; the existing `engine.Clock` abstraction and `engine.WithClock` option are reused with byte-intact signatures, which is what keeps the `eval`/`engine`/`store`/`daemon` diff empty.

## Complexity Tracking

> No Constitution Check violations — this table is intentionally empty.

The feature reduces complexity (removes duplicated ad-hoc `engine.SystemClock{}` construction; centralizes one adapter). The sole over-inclusion — threading `engine.WithClock` on the `metric` path where the engine clock has no observable effect today — is a deliberate completeness measure (prevents future drift if the engine later stamps something on that path) and is documented as a decision in research.md. It introduces no new abstraction and changes no signature, so it does not rise to a constitution deviation requiring justification here.
