# Feature Specification: Unified clock across all CLI paths (C4)

**Feature Branch**: `021-unified-cli-clock`

**Created**: 2026-06-18

**Status**: Draft

**Input**: User description: "C4 — единые часы во всех путях CLI (развилка §8 «двойные часы»). Сегодня serve/emit корректно инъектируют engine.Clock через evalClockFromEngine, но прочие CLI-подкоманды (run, start, complete, tasks, metric) строят собственные/реальные часы — недетерминируемо и неконсистентно во времени внутри одного вызова. C4 унифицирует: ОДИН engine.Clock протягивается во все пути."

## Overview

Today the `serve` and `emit` subcommands of the `ladix` CLI run from a **single, injectable clock**: every "now" they consult — both the date used to evaluate metric expressions and the timestamps that stamp lifecycle records — derives from one source of time. Every other subcommand (`run`, `start`, `complete`, `tasks`, `metric`) instead reaches for **independent real (wall-clock) time** in two or three separate places. This means two parts of the same single command invocation can disagree about what "now" is (e.g. a metric evaluated against one instant while a task is stamped with another), and it makes those commands impossible to exercise deterministically in tests.

C4 unifies time across the CLI: **one injectable clock per command invocation** flows into every place that needs the current moment, so that within a single command all consumers of "now" agree, and so that injecting a fixed clock yields a fully deterministic, time-stable result. `serve` and `emit` already behave this way and MUST remain byte-for-byte unchanged.

The authoritative source for this feature is `docs/reliability-model.md` §C-4 (problem §C-4.1, recipe §C-4.2, test-locks §C-4.3, boundary §C-4.4); see also §C-0, §C-6, §C-7.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - One injectable clock in every CLI path (Priority: P1)

A maintainer (or an automated test) runs any of the time-sensitive subcommands — `run`, `start`, `complete`, `tasks`, `metric` — and wants every notion of "now" inside that single invocation to come from one consistent, injectable source of time. With a fixed clock injected, the command's time-dependent output (metric evaluation dates, lifecycle timestamps, task-line "now", run summaries) is fully deterministic and identical on every run.

**Why this priority**: This is the entire feature. Without it, the affected commands remain non-deterministic in time and internally inconsistent (the "double clocks" fork from §8). It also unblocks deterministic regression testing of those paths. There are no lower-priority slices — the single story is the MVP.

**Independent Test**: Inject a fixed clock (a known calendar instant) into each rewired path in turn and assert that the time-dependent output is deterministic and matches the injected instant; flip the clock to a different instant and assert the output tracks it. Verify by running each subcommand's test under the fixed clock and observing stable, instant-aligned output, while `serve`/`emit` behavior is unchanged.

**Acceptance Scenarios**:

1. **Given** the `run` path with a fixed clock set to a known instant, **When** the command evaluates triggers and prints its summary, **Then** the metric-evaluation date and the summary's "now" both derive from that single instant and the output is deterministic.
2. **Given** the `start` path with a fixed clock, **When** a process instance is started and persisted, **Then** the lifecycle timestamps and any metric-evaluation date both follow that single instant.
3. **Given** the `complete` path with a fixed clock, **When** a task is completed, **Then** the task-completion timestamp and updated-at stamp are deterministic and aligned to the injected instant.
4. **Given** the `tasks` path with a fixed clock, **When** the task list is rendered, **Then** the "now" used to format each task line derives from the injected instant and is deterministic.
5. **Given** the `metric` path with a fixed clock, **When** the metric is evaluated, **Then** both the interpreter's date and the engine's notion of "now" derive from that single injected instant (today only the interpreter date is injectable).
6. **Given** any rewired path is changed to reach for independent real/wall-clock time again, **When** the deterministic test runs, **Then** the test fails (turns red), proving the single-clock guarantee is enforced.

---

### Edge Cases

- **Two consumers of "now" within one command must agree.** Previously `run`/`start`/`complete` consulted time in two or three independent places; under C4 a single injected clock feeds all of them, so they can never disagree within one invocation.
- **`metric` completeness.** The `metric` path already injects time into the interpreter's metric-evaluation date but lets the engine default to real time; the engine's notion of "now" is latent (no current observable effect) but is unified anyway so the path cannot drift if the engine later stamps something.
- **`serve` and `emit` must not change.** These already use one injected clock; the feature MUST NOT alter their behavior or their byte-level output. The existing `serve` golden regression must stay green.
- **Monotonic clocks are explicitly out of scope.** Wall-clock unification only; monotonic time is incompatible with durable restart (the persisted timestamp format drops the monotonic component) and is deferred to backlog (§C-9 / §C-4.4).
- **No behavioral language change.** No new keywords, error codes, evaluation codes, builtins, or dependencies; existing programs remain valid and produce the same results.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Each affected CLI invocation (`run`, `start`, `complete`, `tasks`, `metric`) MUST derive every notion of the current moment from a **single clock value** for the duration of that invocation, so that all time-dependent consumers within one command agree.
- **FR-002**: That single clock MUST be **injectable** — production uses the standard real-time source, while tests can substitute a fixed clock so the command's time-dependent output is fully deterministic.
- **FR-003**: The clock-to-interpreter adapter currently local to `serve` MUST be relocated to a shared location within the CLI command package so that all rewired paths consume the **same adapter type**, with **no behavioral change** to how it is built or used.
- **FR-004**: The `run` path MUST use the single injected clock for both its trigger/metric evaluation and any "now" it reports in its summary, replacing the multiple independent real-time lookups it uses today.
- **FR-005**: The `start` path MUST use the single injected clock for both metric evaluation and the engine's lifecycle timestamps, replacing its two independent real-time lookups.
- **FR-006**: The `complete` path MUST use the single injected clock for the engine's lifecycle stamps (task-completion time and updated-at), replacing its independent real-time lookups, so completion timestamps are deterministic under a fixed clock.
- **FR-007**: The `tasks` path MUST use the single injected clock for the "now" it uses to format each task line, replacing its raw real-time lookup.
- **FR-008**: The `metric` path MUST additionally feed the single injected clock to the engine (completing the unification), even though the engine's current use of "now" on that path has no observable effect today.
- **FR-009**: The `serve` and `emit` paths MUST remain **unchanged** — same behavior, same byte-level output; their existing clock injection is the reference design and MUST NOT be modified.
- **FR-010**: The feature MUST NOT change the signatures or behavior of the existing clock abstraction or the engine option used to inject it; it reuses them as-is.
- **FR-011**: The feature MUST NOT add any new keyword, syntax/error code, evaluation code, builtin, or external dependency; it MUST preserve determinism.
- **FR-012**: The change MUST be confined to the CLI command surface; the evaluation, engine, store, and daemon internals MUST have an empty diff. The store contract count stays at 18 and the process-runtime method count stays at 8.
- **FR-013**: Monotonic clocks MUST NOT be introduced; wall-clock unification only (monotonic deferred to backlog).

### Key Entities *(include if feature involves data)*

- **Clock (single source of "now")**: The one injectable notion of the current instant for a command invocation. Production binds it to real time; tests bind it to a fixed instant. Every time-dependent consumer in the command reads from this one value.
- **Clock-to-date adapter**: The shared helper that exposes the single clock to the metric-evaluation layer (truncating the instant to a calendar date in local time, exactly as today). Relocated from the `serve` path to a shared location; behaviorally identical.
- **Rewired CLI paths**: `run`, `start`, `complete`, `tasks`, `metric` — the five command paths that today consult independent real time and that C4 converts to the single injected clock. (`serve`, `emit` are already unified and excluded.)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Injecting a fixed clock into each rewired path (`run`, `start`, `complete`, `tasks`, `metric`) produces deterministic, time-stable output that is identical across repeated runs and aligned to the injected instant.
- **SC-002**: For every rewired path, an inversion check exists: if that path is reverted to reach for independent real/wall-clock time, its deterministic test turns red. (5 paths covered.)
- **SC-003**: The `serve` clock path is provably unchanged — the existing `serve` golden regression stays green and its fixed-clock test fake still compiles and passes.
- **SC-004**: Within any single rewired command invocation, there is exactly one source of "now": no path consults independent real time more than once, and all time-dependent consumers agree.
- **SC-005**: The diff is confined to the CLI command package; the evaluation, engine, store, and daemon packages have an empty diff, no new keyword/error/eval code/builtin/dependency is introduced, the store contract count remains 18, and the process-runtime method count remains 8.

## Assumptions

- The existing injectable clock abstraction and the engine option that accepts it are sufficient and are reused without signature or behavior changes (no new time abstraction is created).
- "Production" time is the standard real-time system clock; the feature only changes *where the clock is constructed and how it flows*, not the production source of time.
- The `serve`/`emit` design (single clock + adapter) is the canonical reference; the rewired paths adopt the same shape.
- The unification of the engine clock on the `metric` path has no observable output today; it is done for completeness/future-proofing, and that latency is acceptable and intended.
- Monotonic-clock semantics are out of scope and deferred; durable-restart timestamp behavior is unaffected.
- Existing example programs and goldens (other than any that already pin time via the documented `serve` path) are unaffected because production time behavior is preserved.
