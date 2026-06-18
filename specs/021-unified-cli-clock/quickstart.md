# Quickstart: Unified clock across all CLI paths (C4)

## What this feature does

Makes every `ladix` CLI subcommand run from **one injectable clock**, so that within a single command all notions of "now" agree, and so a fixed clock yields fully deterministic, time-stable output. `serve`/`emit` already did this; C4 brings `run`, `start`, `complete`, `tasks`, `metric` in line.

## Build & test

```sh
cd /Users/denis/dev/ladix/src
go build ./...      # must succeed
go test ./...       # all green, incl. new cmd/ladix clock test-locks
go vet ./...        # clean
```

## How to verify the change is correctly scoped

The diff MUST be confined to `src/cmd/ladix/`:

```sh
cd /Users/denis/dev/ladix
git diff --stat                                  # only src/cmd/ladix/* entries
git diff -- src/internal/eval src/internal/engine \
            src/internal/store src/internal/daemon   # MUST be empty
```

## How the single clock flows (per command)

```
clock := engine.SystemClock{}                          // prod (tests pass a fixed-instant fake)
interp := eval.NewInterpreter(out, depth, evalClockFromEngine{clock})
eng    := engine.NewEngine(st, interp, out,
            append([]engine.Option{engine.WithClock(clock)}, withExternalCallerOpt(caller)...)...)
// any summary / task-line "now" = clock.Now()
```

The adapter `evalClockFromEngine` now lives in `cmd/ladix/clock_adapter.go` (moved from `serve.go`); `serve` keeps using it unchanged.

## How to demonstrate determinism (test-lock shape)

For each rewired path, a test injects a fixed clock and asserts the time-dependent output is stable and aligned to the injected instant:

```
T := time.Date(2026, 6, 18, 12, 0, 0, 0, time.Local)
clock := fixedClock{at: T}        // engine.Clock fake (same shape as serve_golden_test.go:21-23)
// build the path's interp+engine with `clock`, run it, assert:
//   run     → metric date & summary "now" == T
//   start   → lifecycle stamps & metric date == T
//   complete→ MarkTaskCompleted / UpdatedAt == T
//   tasks   → each task line's "now" == T
//   metric  → interpreter metric date == T
// re-run with same T → byte-identical; switch to T' → output tracks T'
```

**Inversion**: temporarily revert a path to an independent `engine.SystemClock{}` → its deterministic test must turn red (proves the single-clock guarantee is enforced).

**Serve guard**: `go test ./cmd/ladix -run TestServeMetricDateFollowsSchedulerClock` stays green; the `fixedClock` fake still compiles after the adapter moves.

## Out of scope

- Monotonic clocks (deferred — §C-9 / §C-4.4; incompatible with durable RFC3339 restart).
- Any change to `serve`/`emit`, to `engine.Clock`/`engine.WithClock` signatures, or to `eval`/`engine`/`store`/`daemon`.

## Authority

`docs/reliability-model.md` §C-4 (recipe §C-4.2, test-locks §C-4.3, boundary §C-4.4) is the single source of truth.
