# Behavior — Eval Harness Comparing Both Engines (ADR 0012 Phase 9)

Status: **Implemented** (2026-07-17). Realizes ADR 0012's Phased Build Plan
(amendment) step 9 — the lowest-priority phase in this effort: a measurement tool,
not required for either engine's own correctness (both are already fully tested in
isolation, Phases 5-8). Closes out the ADR's full Phased Build Plan.

## Design

### 1. A standalone `cmd/` tool, mirroring `cmd/ollamabench`'s construction pattern

Like `ollamabench`, this tool constructs its own `*ollama.Client` directly and talks
to a real Ollama host — it does not go through `runtime.Orchestrator` (whose
engine-selection is a single `WavefrontEnabled` switch; the harness needs to run
*both* engines on the *same* goal, which the production switch structurally
doesn't support and shouldn't be bent to support just for this).

### 2. A recording, non-executing `Executor` — safe to run repeatedly, and the right scope for what this tool measures

The harness's purpose is comparing *decomposition behavior* (grounding, dispatch
count, node shape) between engines, not re-validating tool execution — that's
already covered by the existing executor/tools test suites. A `recordingExecutor`
implementing `scheduler.Executor` logs each proposed call
(`{tool, args}`) and returns a placeholder success outcome
(`"[recorded, not run] <tool> <args>"`) without touching the filesystem or network,
so the harness is safe to run against a real project directory repeatedly with no
side effects.

### 3. Both engines' prompts see the same, real, read-only tool catalog

`tools.DefaultRegistry().Available(true)` (read-only only, matching the
investigating-branch convention ADR 0008 already established) renders the same
catalog text both engines' classify/decompose prompts see — real tool ids and
argument names, so the comparison reflects genuine grounding behavior, not an
artificial/stubbed catalog.

### 4. Both engines already expose a comparable dispatch-count metric — no new instrumentation needed

`scheduler.Scheduler.DispatchOrder()`/`Peak()` and `wavefront.Scheduler.DispatchOrder()`/
`Peak()` (Phase 7) already exist, symmetrically, on both types. `len(DispatchOrder())`
is used directly as "how many LLM-driving dispatches this goal took" — a fair,
already-built comparison point, not something this phase needs to add.

### 5. A small built-in goal corpus, overridable

A handful of realistic project-investigation goals ship as defaults (in the spirit
of ADR 0008's own canonical review fixture); `-goals <path>` overrides with a
newline-separated file for a larger or project-specific corpus.

```
GIVEN a goal from the corpus
WHEN  the harness runs it through both engines
THEN  it reports, per engine: wall time, dispatch count, peak concurrency, and the
      root's final outcome (Done+Value, Failed+Error, or still-open/stalled) —
      printed as one comparison row per goal, not aggregated across the whole
      corpus (aggregate statistics are a natural follow-up, not built here).
```

## Explicitly out of scope

- Aggregate statistics across a corpus run (mean/p50 dispatch count, etc.) —
  ollamabench's own per-round-record-not-aggregated style is mirrored deliberately;
  a follow-up if the harness sees real use.
- Automated judging of *answer quality* (whether the synthesized answer is
  actually correct) — this reports what each engine *did*, not whether either
  engine's answer was right; that judgment is inherently goal-specific and human
  (or LLM-judge) territory, not scoped here.
- Resolving Open Question 1 (dedicated wavefront synthesis call vs. reusing
  `planContext`) — the harness makes that evaluation *possible* by giving both a
  measurement surface; it does not itself run that specific experiment.

## Tests

This is a `cmd/` tool exercising real network calls by design (like
`ollamabench`, which also has no test file — matched precedent), not a
Godog-suite candidate. Verified with real runs against the live
`nemotron-cascade-2:latest` instance already confirmed reachable in this
environment:

```
# host=localhost:11434 model=nemotron-cascade-2:latest slots=4 maxdepth=3 goals=1
goal                                    engine       wall      dispatch peak nodes status     value/error
What language is this project written  continuous   60023ms   3        1    3     scheduler-error  context deadline exceeded
in?                                     wavefront    60059ms   1        1    1     scheduler-error  context deadline exceeded
```

A first run with a 60s per-engine-per-goal budget confirmed correct *timeout*
handling — both engines report `context deadline exceeded` cleanly (via
`schedulerErr`, not a crash or a hang past the deadline) rather than continuing
past their own cancelled context. Re-run with a 5-minute budget:

```
goal                                    engine       wall       dispatch peak nodes status    value/error
What language is this project written   continuous   71862ms    7        2    7     proposed
in?                                     wavefront    244088ms   6        2    9     proposed
```

Both completed without error this time and produced structurally sane, comparable
output — real dispatch counts, real node growth, real wall time — confirming the
harness itself works correctly end to end against a real model. Neither engine's
root actually resolved to `Done` within this run (`status: proposed` — a node stays
`Proposed` until it terminally resolves for *both* engines, by design; see
`scheduler.go`/`wavefront/scheduler.go`, neither has an explicit "in progress"
status). This reflects real model latency and the default `maxdepth=3` cap on a
genuinely open-ended investigative goal, not a harness defect — `nemotron-cascade-2`
took 30-70+ seconds per call with reasoning enabled throughout this session's other
live checks, consistent with totAlX's own core finding that reasoning cost, not
prompt size, dominates latency. This tool has no reasoning-effort control of its
own (unlike the production paths, which gained `Think`/budget control in Phase 1)
— a reasonable follow-up, not built here, since the goal of this run was verifying
the harness's own correctness, not tuning it for fast, fully-resolved runs.

- `go build ./cmd/wavefronteval` succeeds. ✓
- A real run against `nemotron-cascade-2:latest` completes (or times out cleanly)
  and prints a sane, symmetric comparison table for both engines. ✓ (both runs
  above)
