# Behavior — Orchestrator Wiring for Wavefront (ADR 0012 Phase 8)

Status: **Proposed**, implementation in progress. Realizes ADR 0012's Phased Build
Plan (amendment) step 8. Lower-risk than Phase 7 — mostly plumbing already-built
pieces (`wavefront.Classifier`, `wavefront.Scheduler`, `wavefront.CondenseFunc`)
into the orchestrator the same way `buildDecomposition` already does for the
continuous engine.

## Design

### 1. `Settings.WavefrontEnabled`, default off

New `[agentx.wavefront] enabled` TOML key, `Config.WavefrontEnabled()` (default
`false` via `boolOr` — an experimental second engine should not activate silently
for existing users), `Settings.WavefrontEnabled bool` wired through `app.go` the
same way `ToolsEnabled` is.

### 2. `buildWavefront` mirrors `buildDecomposition`, and needs only one `Chat` closure, not two

```
GIVEN buildWavefront runs (called from Start, right after buildDecomposition)
WHEN  WavefrontEnabled is false, or taskExec isn't built, or it was already built
THEN  it is a no-op — same guard shape as buildDecomposition's own early return.
```

The same `client.Complete`-backed closure serves both the classifier (always
passing `ClassifySchema()` as `format`) and the scheduler's synthesis path (always
passing `nil`) — `Complete`'s own `if len(req.Format) > 0` guard already makes the
presence of a schema optional per call, so there is no need for two separate
closures differing only in whether they set `Format`.

### 3. `runWavefrontPhase` returns the exact same `(string, bool, error)` contract `runPlanPhase` already does

`runPlanPhase` gains a `WavefrontEnabled` branch at its top, mirroring the ADR's
own framing ("the WavefrontEnabled branch in runPlanPhase") — callers
(`continuation.go`, `orchestrator.go`'s route dispatch) never need to know which
engine actually ran; both branches return the same shape.

```
GIVEN Settings.WavefrontEnabled is true and the engine was built successfully
WHEN  runPlanPhase is called
THEN  it drains through wavefront.Scheduler instead of decompose.DrainPlan,
      publishes the same TASK_PLAN/TASK_NODE events (reusing planObserver
      unchanged — it already satisfies scheduler.Observer, the same interface
      wavefront.Scheduler accepts, so no new Observer type is needed), and returns
      plan context text or handled=false exactly like the continuous branch does.
```

### 4. `wavefrontPlanContext` includes resolved `KindStep` nodes, not only `KindTask` executions

The continuous engine's `planContext` only ever sees `capturedStep`s, which
`capturingExec` only produces for Task-leaf executions — a Step's own resolution
was never captured as a "finding" there, because the continuous engine's Steps
don't carry a synthesized value of their own (yet — ADR 0010's still-pending judge
phases). Wavefront's Step nodes, by contrast, routinely carry the actual meaningful
synthesized answer (via self-match or the fallback synthesis call) — excluding
them would discard most of what a wavefront-run plan actually produced. This is a
deliberate difference from `planContext`, not an oversight: every non-root,
Done-or-Failed node (any Kind) is included.

```
GIVEN a wavefront plan drains with a mix of resolved KindTask and KindStep nodes
WHEN  wavefrontPlanContext renders it
THEN  every non-root Done/Failed node's Goal/Value/Error appears, KindStep
      included; the root itself is excluded (its resolution is the answer the
      respond call is building toward, not a finding feeding into it).
```

### 5. `publishPlan`'s `decompose.PlanOutcome` wrapper is bypassed, not reused awkwardly

`publishPlan(root, out decompose.PlanOutcome, executed, derr)` only ever reads
`out.Nodes` — `planSummary` (the function that actually builds the payload) already
takes `[]task.Record` directly. `runWavefrontPhase` calls `planSummary` directly
with its own `[]task.Record`, rather than constructing a synthetic
`decompose.PlanOutcome` just to satisfy a wrapper that adds nothing here.

## Tests

Given this phase is orchestrator-level plumbing over already-tested engine
internals (Phase 7's scheduler tests already cover the engine's own correctness),
tests here focus on the wiring seams specifically:

- `internal/runtime/wavefront_cycle_test.go` (new) — `wavefrontPlanContext`: root
  excluded, KindStep and KindTask both included, Failed nodes show their Error,
  a plan with nothing resolved returns `""` (matching `planContext`'s same
  "nothing investigated → answer directly" contract).
- `internal/runtime/classifier_pipeline_test.go` (extended, if needed) — the one
  `Chat` closure genuinely passes `format` through unmodified for both constrained
  and unconstrained calls (a regression guard against accidentally hard-coding
  `ClassifySchema()` into the shared closure).
