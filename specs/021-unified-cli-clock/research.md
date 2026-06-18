# Phase 0 Research: Unified clock across all CLI paths (C4)

**Authority**: `docs/reliability-model.md` §C-4 + on-disk seam digest. No open NEEDS CLARIFICATION — all decisions resolved from the anchor.

## R1 — Reference design: the `serve` single-clock pattern

**Decision**: Adopt the exact shape `serve` already uses. One `engine.Clock` value flows into two consumers: the interpreter's metric-date layer (via the `evalClockFromEngine` adapter) and the engine's lifecycle timestamps (via `engine.WithClock`).

**Evidence (live code, seam digest §C4)**:
- `engine.Clock` interface = `engine/clock.go:7-8` (`Now() time.Time`). `SystemClock` :13, `Now` :16 (the sole `time.Now()` in `engine`, D-2). `WithClock` Option :19-23.
- Adapter `type evalClockFromEngine struct{ c engine.Clock }` = `serve.go:32`; `func (a evalClockFromEngine) Now() value.Дата` = `serve.go:35` (truncates the instant to Y/M/D in Local, exposing it as `eval.Clock`).
- Injected-clock assembly = `buildServeDaemon` `serve.go:201-223`: single clock → `engine.WithClock` + `evalClockFromEngine` adapter + `daemon.New` (FR-024 dual-clock unity).
- Locked by `serve_golden_test.go:216` (`TestServeMetricDateFollowsSchedulerClock`) with fake `fixedClock` (engine.Clock) at `serve_golden_test.go:21-23`.

**Rationale**: The pattern is already proven and golden-locked. Reusing its exact shape guarantees the rewired paths behave identically to the reference and keeps `engine`/`eval` signatures untouched.

**Alternatives rejected**:
- A brand-new clock abstraction → rejected: violates "0 new abstractions / empty diff outside cmd/ladix"; `engine.Clock` already suffices.
- Passing `time.Time` instants directly → rejected: loses injectability and the adapter that bridges `engine.Clock` → `eval.Clock`.

## R2 — Extracting `evalClockFromEngine` to a shared file

**Decision**: Move `evalClockFromEngine` (struct + `Now()`) **verbatim** out of `serve.go` into a new `cmd/ladix/clock_adapter.go` in the **same package** `main` (package `cmd/ladix`). `serve` keeps referencing the same unexported type — no behavioral change.

**Rationale**: All five rewired paths need the same adapter type. Same-package relocation means zero import churn and zero behavior change; the type stays unexported. The seam digest confirms the adapter is purely `serve.go`-local today (only used at the serve assembly site), so moving it is safe.

**Alternatives rejected**:
- Duplicating the adapter per call-site → rejected: copy-paste, drift risk.
- Exporting it / moving to `internal/eval` → rejected: would dirty the `eval` diff and widen surface; the anchor explicitly says "shared file in package cmd/ladix".

**File-name note**: anchor suggests `clock.go` or `clock_adapter.go`; `clock_adapter.go` chosen to avoid colliding with the conceptual notion of `engine/clock.go` / `eval/clock.go` and to read clearly.

## R3 — Canonical per-command recipe

**Decision**: Each rewired builder accepts one `clock engine.Clock` (prod `engine.SystemClock{}`) and assembles:
```
interp := eval.NewInterpreter(out, depth, evalClockFromEngine{clock})
eng    := engine.NewEngine(st, interp, out,
            append([]engine.Option{engine.WithClock(clock)}, withExternalCallerOpt(caller)...)...)
// any "now" for summaries = clock.Now()
```
This mirrors `buildServeDaemon` (serve.go:201-223).

**Rationale**: Single source of "now" per invocation (FR-001/FR-004..FR-008). Production keeps real time (`engine.SystemClock{}`), tests substitute a fixed clock.

## R4 — Per-path rewiring map (live line anchors, seam digest §C4)

| Path | Today | Action |
|---|---|---|
| **run** | `main.go:251` interp clock + `:255` NewEngine default + `:277` raw `engine.SystemClock{}.Now()` for summary — **three independent clocks** | Thread one `engine.Clock`: adapter into interp, `WithClock` into engine, `clock.Now()` for summary |
| **start** | `start.go:133` `eval.SystemClock` interp + `:138` NewEngine default — **two independent** | One clock: adapter + `WithClock` |
| **complete** | `main.go:446/451` — two independent; engine clock stamps `MarkTaskCompleted`/`UpdatedAt` | One clock: adapter + `WithClock` → completion stamps deterministic |
| **tasks** | `main.go:559` raw `engine.SystemClock{}.Now()` feeds `FormatTaskLine` | Inject `engine.Clock` param into `listTasks`; use `clock.Now()` |
| **metric** | `main.go:296` sig / `:309` interp (eval-clock already injectable) / `:319` NewEngine default | Add `engine.WithClock(sameClock)`; eval-clock unchanged |
| serve | already unified (`buildServeDaemon`) | **NOT touched** (locked) |
| emit | already injectable (`emit.go:49/58/75`) | **NOT touched** |

**Note**: anchor §C-4.1 corrects that `run.go`/`tasks.go`/`metric.go` do **not** exist — `run`, `complete`, `tasks`, `metric` all live in `cmd/ladix/main.go`; `start` in `start.go`; `serve` in `serve.go`.

## R5 — `metric` engine clock: latent effect, included for completeness

**Decision**: Thread `engine.WithClock(clock)` on the `metric` path even though the engine's notion of "now" has **no observable effect** on that path today (the eval-clock already drives the metric date; the engine does no lifecycle stamping there currently).

**Rationale**: Completeness + future-proofing — if the engine later stamps something on the metric path, it must follow the same single clock, not silently drift back to real time. Unifying now closes the door on a latent re-introduction of "double clocks". This is a benign over-inclusion, not a constitution deviation; recorded here per the anchor's note.

**Alternatives rejected**: leaving the metric engine on default real time → rejected: re-opens the §8 double-clocks fork the moment the engine touches "now" there; the cost of unifying now is zero.

## R6 — What stays byte-intact (empty-diff guarantee)

**Decision**: Do **not** modify `engine.Clock`, `engine.SystemClock`, `engine.WithClock`, `engine.NewEngine`, `eval.NewInterpreter`, `eval.Clock`, `eval.SystemClock`, or anything in `store`/`daemon`. The feature only changes *where the clock is built and how it flows* inside `cmd/ladix`.

**Evidence**: All consumed symbols already exist with the needed signatures (seam digest §C4). Store stays 18 + double compile-lock (`store.go:42-45`); ProcessRuntime stays 8 (`eval/runtime.go`). No new KW/SE/eval-code/builtin/dependency.

**Verification gate (§C-7)**: `git diff --stat` after implementation must show changes only under `src/cmd/ladix/`; `git diff` of `internal/eval`, `internal/engine`, `internal/store`, `internal/daemon` must be empty.

## R7 — Monotonic clocks: out of scope (§C-4.4)

**Decision**: Do NOT introduce monotonic clocks. Wall-clock unification only.

**Rationale**: Monotonic time is incompatible with durable restart — the RFC3339 persistence format drops the monotonic component of `time.Time`. Deferred to backlog §C-9.

## R8 — Test strategy (anchor §C-4.3)

**Decision**: New `cmd/ladix/clock_unify_test.go` with, per rewired path (run/start/complete/tasks/metric):
1. **Fixed-clock injection** — inject a `fixedClock{2026-…}` (engine.Clock fake) and assert the time-dependent output (metric date, lifecycle stamps, task-line "now", run summary) is deterministic and aligned to the injected instant. Mirrors `serve_golden_test.go:216`.
2. **Inversion** — a check that reddens if the path reverts to an independent `engine.SystemClock{}` (date/stamp would diverge from the injected instant).

Plus a **serve-unchanged regression guard**: confirm `serve_golden_test.go:216` stays green and the `fixedClock` fake still compiles after the adapter moves.

**Rationale**: Directly encodes the §C-4.3 test-locks. The `fixedClock` fake (`serve_golden_test.go:21-23`) is the existing template for an injectable `engine.Clock`; new tests reuse the same shape.

## Open questions

None. All resolved from the anchor.
