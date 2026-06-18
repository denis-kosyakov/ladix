---
description: "Task list for feature 021-unified-cli-clock (C4 — unified clock across all CLI paths)"
---

# Tasks: Unified clock across all CLI paths (C4)

**Input**: Design documents from `/specs/021-unified-cli-clock/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/clock-injection.md, quickstart.md

**Authority**: `docs/reliability-model.md` §C-4 (recipe §C-4.2, test-locks §C-4.3, boundary §C-4.4) — single source of truth.

**Tests**: REQUIRED. Per constitution principle VI and §C-4.3, the per-path fixed-clock injection + inversion test-locks and the serve-unchanged regression guard are mandatory and authored together with each rewire.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files / independent, no dependency on incomplete work)
- **[Story]**: US1 = the single P1 user story ("one injectable clock in every CLI path")
- All paths are under `/Users/denis/dev/ladix/` (Go module rooted at `src/`).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish the baseline before touching any path. No code beyond confirming the green starting point.

- [ ] T001 Confirm clean starting point on branch `021-unified-cli-clock`: from `/Users/denis/dev/ladix/src` run `go build ./...`, `go vet ./...`, `go test ./...` — all green BEFORE any change (records the pre-change baseline for the empty-diff/boundary check in Phase 5).
- [ ] T002 Re-read the live clock seams to pin exact line anchors before editing: `src/cmd/ladix/serve.go` (adapter `evalClockFromEngine` struct + `Now()`, ~:32-38; serve assembly `buildServeDaemon` ~:201-223), `src/internal/engine/clock.go` (`Clock` iface, `SystemClock`, `WithClock`), `src/cmd/ladix/serve_golden_test.go` (`fixedClock` fake ~:21-23, `TestServeMetricDateFollowsSchedulerClock` ~:216). No edits — confirm anchors only.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Extract the shared adapter so every path can consume one type. MUST complete before any rewire.

**⚠️ BLOCKS all of Phase 3 (rewires) and Phase 4 (per-path tests).**

- [ ] T003 Create NEW file `src/cmd/ladix/clock_adapter.go` (package `main`) and MOVE the `evalClockFromEngine` adapter (struct `evalClockFromEngine struct{ c engine.Clock }` + method `func (a evalClockFromEngine) Now() value.Дата` that truncates `a.c.Now()` to Y/M/D in Local) VERBATIM out of `src/cmd/ladix/serve.go:32-38`. Behavior-frozen — byte-identical logic; type stays unexported; required imports (`engine`, `value`, `time`) move with it.
- [ ] T004 In `src/cmd/ladix/serve.go` REMOVE only the now-duplicated local `evalClockFromEngine` type declaration (lines ~32-38); leave `buildServeDaemon` and every other serve line UNTOUCHED — serve keeps referencing the shared `evalClockFromEngine`. Verify `cd src && go build ./...` compiles (serve still resolves the type from `clock_adapter.go`).

**Checkpoint**: Adapter is shared; serve unchanged behaviorally; build green. Rewires can begin.

---

## Phase 3: User Story 1 — One injectable clock in every CLI path (Priority: P1)

**Goal**: Every rewired CLI invocation derives all "now" from a single injectable `engine.Clock` (prod `engine.SystemClock{}`), threaded into the interpreter (via `evalClockFromEngine{clock}`) and the engine (via `engine.WithClock(clock)`), with any summary/task-line "now" read from `clock.Now()`. Reference shape = `buildServeDaemon` (serve.go:201-223). `serve` + `emit` NOT touched.

**Independent Test**: Inject a fixed clock into each rewired path; assert all time-dependent output aligns to the injected instant and is byte-stable across reruns (covered by Phase 4 tests).

> NOTE on ordering (constitution VI): each rewire task in Phase 3 is paired with its Phase 4 test task (injection + inversion). Author the test alongside the rewire — Phase 3/Phase 4 pairs are listed separately for clarity but land together per path.

### Rewires

- [ ] T005 [US1] Rewire `run` in `src/cmd/ladix/main.go` (~:251 interp clock, ~:255 NewEngine, ~:277 raw `engine.SystemClock{}.Now()` summary): collapse the three independent clocks into ONE `clock engine.Clock` (prod `engine.SystemClock{}`); build `interp := eval.NewInterpreter(out, depth, evalClockFromEngine{clock})`; pass `engine.WithClock(clock)` into the `engine.NewEngine` options (`append([]engine.Option{engine.WithClock(clock)}, withExternalCallerOpt(caller)...)...`); replace the summary `engine.SystemClock{}.Now()` with `clock.Now()`. No second clock constructed.
- [ ] T006 [US1] Rewire `start` in `src/cmd/ladix/start.go` (~:133 `eval.SystemClock` interp, ~:138 NewEngine): collapse the two independent clocks into ONE `clock engine.Clock`; `interp := eval.NewInterpreter(out, depth, evalClockFromEngine{clock})`; add `engine.WithClock(clock)` to the engine options. Replace `eval.SystemClock`/engine-default usage on this path.
- [ ] T007 [US1] Rewire `complete` in `src/cmd/ladix/main.go` (~:446/:451): thread ONE `clock engine.Clock` into the engine via `engine.WithClock(clock)` so `MarkTaskCompleted`/`UpdatedAt` lifecycle stamps follow it; build the interpreter with `evalClockFromEngine{clock}` for consistency. Remove the independent real-time engine clock.
- [ ] T008 [US1] Rewire `tasks` in `src/cmd/ladix/main.go` (~:559 raw `engine.SystemClock{}.Now()` feeding `FormatTaskLine`): inject an `engine.Clock` parameter into `listTasks` and format each task line's "now" from `clock.Now()` instead of constructing a raw `SystemClock{}`. Update the `listTasks` call-site to pass the single clock.
- [ ] T009 [US1] Rewire `metric` in `src/cmd/ladix/main.go` (~:296 sig / ~:309 interp / ~:319 NewEngine): add `engine.WithClock(clock)` to the engine options using the SAME clock already feeding the interpreter's eval-clock (eval-clock path already injectable — leave it; do not construct a second clock). Per research §R5 this engine clock is latent (no observable effect today) — included for completeness/future-proofing.

**Checkpoint (rewires)**: `cd src && go build ./...` green; `serve`/`emit` lines untouched; every rewired path constructs exactly one clock.

---

## Phase 4: User Story 1 — Test-locks (§C-4.3, authored with the rewires)

**Goal**: Lock determinism per path (fixed-clock injection), enforce the single-clock guarantee (inversion reddens on real-clock fallback), and guard serve.

**File**: new `src/cmd/ladix/clock_unify_test.go` (use an `engine.Clock` fixed-instant fake mirroring `serve_golden_test.go:21-23` `fixedClock`).

### Per-path fixed-clock injection (deterministic, time-aligned output)

- [ ] T010 [P] [US1] In `src/cmd/ladix/clock_unify_test.go` add `TestRunClockInjected`: drive the `run` path with a fixed clock at a known instant `T`; assert the metric-evaluation date AND the summary "now" both derive from `T`; assert byte-identical output on rerun with same `T` and that switching to `T'` moves the output to `T'`.
- [ ] T011 [P] [US1] Add `TestStartClockInjected`: drive `start` with fixed clock `T`; assert lifecycle stamps and any metric date follow `T`; deterministic across reruns.
- [ ] T012 [P] [US1] Add `TestCompleteClockInjected`: drive `complete` with fixed clock `T`; assert `MarkTaskCompleted` time and `UpdatedAt` stamp equal `T` and are deterministic.
- [ ] T013 [P] [US1] Add `TestTasksClockInjected`: drive `tasks` (`listTasks`) with fixed clock `T`; assert each rendered task line's "now" derives from `T` and is byte-stable.
- [ ] T014 [P] [US1] Add `TestMetricClockInjected`: drive `metric` with fixed clock `T`; assert the interpreter metric date equals `T` (engine clock also `T`, latent); deterministic across reruns.

### Per-path inversion (reddens on real/wall-clock fallback)

- [ ] T015 [US1] Add inversion assertions for all five paths in `src/cmd/ladix/clock_unify_test.go` (mutation-probe style, documented in a comment): each `Test*ClockInjected` MUST be constructed so that if the path reverts to an independent `engine.SystemClock{}` (real wall clock), the assertion diverges from `T` and the test turns RED. Document the intent: "if a path falls back to a real/wall clock, this test must fail." Verify by temporarily reintroducing `engine.SystemClock{}.Now()` in each path (locally, then revert) and confirming the corresponding test reddens.

### Serve-unchanged regression guard

- [ ] T016 [US1] Verify the serve clock path is byte-intact: `cd src && go test ./cmd/ladix -run TestServeMetricDateFollowsSchedulerClock` stays GREEN (serve_golden_test.go:216) and the `fixedClock` fake (serve_golden_test.go:21-23) still compiles after the adapter move (T003/T004). No edits to `serve_golden_test.go` or `serve.go` clock assembly.

**Checkpoint**: All five paths deterministic under a fixed clock; inversions proven to redden; serve golden green. US1 complete and independently testable.

---

## Phase 5: Polish & Cross-Cutting — Boundary / drift-audit (§C-7)

**Purpose**: Prove the empty-diff guarantee and the frozen surfaces.

- [ ] T017 Boundary check — diff confinement: from `/Users/denis/dev/ladix` run `git diff --stat` and confirm EVERY changed path is under `src/cmd/ladix/`; then confirm `git diff -- src/internal/eval src/internal/engine src/internal/store src/internal/daemon` is EMPTY. Any non-empty internal diff is a failure (FR-012 / contract C-4).
- [ ] T018 [P] Frozen-surface check: confirm `engine.Clock`, `engine.SystemClock`, `engine.WithClock`, `engine.NewEngine`, `eval.NewInterpreter`, `eval.Clock`, `eval.SystemClock` signatures are byte-unchanged; Store contract still 18 methods with the DOUBLE compile-lock at `src/internal/store/store.go:42-45` intact; ProcessRuntime still 8 methods in `src/internal/eval/runtime.go`; no new keyword / SE-code / eval-code / builtin / external dependency was added (grep / inspect; no `go.mod` change).
- [ ] T019 Full gate: from `/Users/denis/dev/ladix/src` run `go build ./...`, `go vet ./...`, `go test ./...` — all green (incl. new `clock_unify_test.go` and the serve golden). `gofmt` clean on changed files.
- [ ] T020 [P] Docs/anchor cross-check: confirm the implemented recipe matches `docs/reliability-model.md` §C-4.2 (shared adapter + per-command assembly), §C-4.3 (test-locks present), §C-4.4 (no monotonic clocks introduced). Report any anchor↔code line drift in the PR notes (do not silently "fix" the anchor).

---

## Dependencies & Execution Order

- **Phase 1 (T001–T002)**: baseline + anchors. No dependencies.
- **Phase 2 (T003–T004)**: adapter extraction. Depends on Phase 1. **BLOCKS Phase 3 & 4.**
- **Phase 3 (T005–T009)**: rewires. Each depends on Phase 2 (shared adapter). T005/T007/T008/T009 all touch `main.go` (different functions) → keep sequential to avoid edit conflicts; T006 (`start.go`) is independent.
- **Phase 4 (T010–T016)**: tests. Each path's test depends on that path's rewire (T010←T005, T011←T006, T012←T007, T013←T008, T014←T009). T010–T014 are `[P]` once their rewires land (all in one new test file — coordinate file writes). T015 (inversion) depends on T010–T014. T016 (serve guard) depends only on Phase 2.
- **Phase 5 (T017–T020)**: boundary/gate. Depends on all of Phase 3 & 4.

## Parallel Execution Examples

- After Phase 2: T006 (`start.go`) can proceed in parallel with the first `main.go` rewire; the remaining `main.go` rewires (T005/T007/T008/T009) serialize on that file.
- After rewires land: T010–T014 injection tests are logically parallel (same new file — serialize writes, parallel design).
- In Phase 5: T018 and T020 are `[P]` (read-only checks) and can run alongside T017/T019.

## Implementation Strategy (MVP)

This feature is a single P1 story; the MVP IS US1. Land Phase 2 (shared adapter) → Phase 3 rewires path-by-path, each immediately paired with its Phase 4 injection + inversion test (constitution VI tests-first) → Phase 4 serve guard → Phase 5 boundary/gate. Deliver as one cohesive change confined to `src/cmd/ladix/`.
