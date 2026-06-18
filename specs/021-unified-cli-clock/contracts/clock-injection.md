# Contract: CLI clock injection (C4)

**Type**: Internal package contract for `cmd/ladix` (no external/public API surface changes; CLI flags and stdout/stderr output are byte-identical in production). This documents the shape every rewired command builder MUST satisfy.

## C-1 — Shared adapter (relocated, behavior-frozen)

```
// package main (cmd/ladix), file clock_adapter.go
type evalClockFromEngine struct { c engine.Clock }
func (a evalClockFromEngine) Now() value.Дата   // truncate a.c.Now() to Y/M/D in Local
```

- MUST be moved verbatim from `serve.go:32-38`; behavior identical.
- MUST remain unexported and in package `main`.
- `serve` (`buildServeDaemon`) MUST keep using this exact type with no edit other than the type no longer being declared in `serve.go`.

## C-2 — Canonical per-command assembly

Every rewired builder MUST accept a single `clock engine.Clock` and build:

```
interp := eval.NewInterpreter(out, depth, evalClockFromEngine{clock})
eng    := engine.NewEngine(st, interp, out,
            append([]engine.Option{engine.WithClock(clock)}, withExternalCallerOpt(caller)...)...)
// summary / task-line "now"  ==  clock.Now()
```

- Production caller passes `engine.SystemClock{}`.
- Test caller passes a fixed-instant `engine.Clock` fake.
- The SAME `clock` value feeds the interpreter adapter AND `engine.WithClock` AND any summary "now". No second clock may be constructed.

## C-3 — Per-path obligations

| Path | MUST | MUST NOT |
|---|---|---|
| `run` | thread one clock to interp + engine + summary `clock.Now()` | construct any independent `engine.SystemClock{}` after the one clock is chosen |
| `start` | thread one clock to interp + engine | use `eval.SystemClock` / engine-default independently |
| `complete` | thread one clock to engine (stamps `MarkTaskCompleted`/`UpdatedAt`) | leave engine on default real time |
| `tasks` | take `engine.Clock` param into `listTasks`; format task lines with `clock.Now()` | call raw `engine.SystemClock{}.Now()` |
| `metric` | add `engine.WithClock(sameClock)` (eval-clock already injected) | construct a second clock for the engine |

## C-4 — Frozen surfaces (empty diff outside cmd/ladix)

The implementation MUST NOT change any of:
- `engine.Clock`, `engine.SystemClock`, `engine.WithClock`, `engine.NewEngine` signatures/behavior.
- `eval.NewInterpreter`, `eval.Clock`, `eval.SystemClock`.
- `store` (18 methods, double compile-lock), `daemon`.
- No new keyword / SE-code / eval-code / builtin / external dependency.

**Verification**: `git diff` restricted to `internal/eval`, `internal/engine`, `internal/store`, `internal/daemon` MUST be empty; `git diff --stat` MUST show changes only under `src/cmd/ladix/`.

## C-5 — Serve/emit untouched

- `serve` clock path MUST stay byte-intact: `serve_golden_test.go:216` (`TestServeMetricDateFollowsSchedulerClock`) green; `fixedClock` fake (`serve_golden_test.go:21-23`) still compiles.
- `emit` MUST NOT change.

## C-6 — Determinism (acceptance contract)

For each rewired path, injecting a fixed clock at instant `T` MUST yield output where every time-dependent value derives from `T`:
- `run`: metric-evaluation date and summary "now" both at `T`.
- `start`: lifecycle stamps and metric date at `T`.
- `complete`: completion + updated-at stamps at `T`.
- `tasks`: each task line's "now" at `T`.
- `metric`: interpreter metric date at `T` (engine clock also `T`, latent).

Re-running with the same `T` MUST produce byte-identical output. Changing to instant `T'` MUST move the output to `T'`.
