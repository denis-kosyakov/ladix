# Phase 1 Data Model: Unified clock across all CLI paths (C4)

This feature is a **wiring refactor**, not a data feature. It introduces **no new persisted entities, no Store changes, no schema changes**. The "entities" below are code/runtime constructs (clock flow), captured for design completeness.

## Entities (runtime / code constructs)

### Clock (single source of "now")

- **What it is**: One `engine.Clock` value per CLI invocation. Interface (existing, unchanged): `Now() time.Time` (`engine/clock.go:7-8`).
- **Production binding**: `engine.SystemClock{}` (real time, `engine/clock.go:13/16` — the sole `time.Now()` in `engine`).
- **Test binding**: a fixed-instant fake implementing `engine.Clock` (e.g. the existing `fixedClock` shape, `serve_golden_test.go:21-23`).
- **Invariant**: within one invocation, exactly one such value is constructed and threaded to all consumers. No path constructs more than one clock; no path reads real time independently of it.
- **Lifecycle**: created at the top of each rewired command builder; passed by parameter (no package-level state — constitution V).

### Clock-to-date adapter (`evalClockFromEngine`)

- **What it is**: shared unexported adapter bridging `engine.Clock` → `eval.Clock`. Struct holds one `engine.Clock`; `Now() value.Дата` truncates the instant to Y/M/D in Local (behavior unchanged from `serve.go:35`).
- **Relationship**: wraps the single Clock; produced once per invocation as `evalClockFromEngine{clock}` and handed to `eval.NewInterpreter`.
- **Relocation**: moved verbatim from `cmd/ladix/serve.go:32-38` to new `cmd/ladix/clock_adapter.go`, same package. No signature/behavior change.
- **Consumers**: `serve` (unchanged) + the five rewired paths.

### Rewired CLI paths

The five command builders that change from independent real clocks to the single injected clock:

| Path | Consumers of "now" today | After C4 |
|---|---|---|
| `run` | interp clock + engine default + raw `SystemClock{}.Now()` summary (3) | one clock → adapter + `WithClock` + `clock.Now()` summary |
| `start` | interp `eval.SystemClock` + engine default (2) | one clock → adapter + `WithClock` |
| `complete` | engine default stamping `MarkTaskCompleted`/`UpdatedAt` (2) | one clock → adapter + `WithClock` (deterministic stamps) |
| `tasks` | raw `SystemClock{}.Now()` → `FormatTaskLine` | one `engine.Clock` param into `listTasks` → `clock.Now()` |
| `metric` | eval-clock injectable; engine default | add `WithClock(sameClock)`; eval-clock unchanged |

`serve` and `emit` are **not** in this table — already unified, excluded from the diff.

## Non-entities (explicitly unchanged — empty diff)

- **Store contract**: 18 methods, double compile-lock (`store/store.go:42-45`) — **no change**. No `OutboxRecord`/new method/new sentinel here (that was C2b).
- **ProcessRuntime**: 8 methods (`eval/runtime.go`) — **no change**, no signature touched.
- **DB schema**: version unchanged by this feature.
- **`engine.Clock` / `engine.WithClock` / `engine.NewEngine` / `eval.NewInterpreter` signatures**: **byte-intact**.
- **Keywords / SE-codes / eval-codes / builtins / dependencies**: none added.

## State transitions

None. No stateful entity is introduced or modified. The only "state" is the per-invocation clock value, which is immutable for the life of the invocation.
