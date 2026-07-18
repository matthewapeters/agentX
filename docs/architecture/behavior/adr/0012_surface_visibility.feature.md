# Behavior — Wavefront Surface Visibility: Nesting, Value/Error, Convergence, Pin (ADR 0012 amendment, Surface Visibility)

Status: **Implemented** (2026-07-17). Realizes ADR 0012's "Amendment (2026-07-17):
Surface Visibility — Chat Output, Context, Working Memory".

Built exactly as scoped below. Tests:
`internal/runtime/wavefront/scheduler_test.go` (`TestObserverSeesDecomposeAndConverge`),
`internal/surfaces/output/plan_test.go` (`TestWavefrontStepValueRenders`,
`TestWavefrontConvergenceAnnotates`, `TestSelectedPlanNodeNavigationAndPin`,
`TestWavefrontSourceTag`),
`internal/runtime/plan_tree_test.go` (extended `completed` call),
`internal/runtime/scheduler/observer_test.go` (extended `recObserver`),
`tests/features/transport/working_memory_api.feature` (three new scenarios: the
plan-node pin endpoint, refusal on an unresolved node, refusal to ever set a
plan-node pin live). All pre-existing tests in every touched package pass
unchanged; full `go build ./...` / `go vet ./...` / `go test ./...` clean.

## Problem

Reading `wavefront.Scheduler` (ADR 0012 Phase 7, already shipped) against the
plan widget it renders into (`internal/surfaces/output/plan.go`, ADR 0009 §9c)
surfaced a latent bug, not a missing enhancement: `wavefront.Scheduler` never
called `scheduler.Observer.NodeDecomposed` — only `NodeDispatched` and
`NodeCompleted`. The widget's nested-box recursion only ever descends into a
node's `children`, which is populated *exclusively* by the `"decomposed"` event
handler. Net effect: **a wavefront plan rendered as its root box and nothing
else**, regardless of how much work happened underneath it.

Three smaller, related gaps:

- `NodeCompleted(id, status)` didn't carry the `Value`/`Error` ADR 0012's
  "Graph-as-Blackboard" amendment had already added to `task.Record` — so even a
  node that *did* render had nothing to show for a Step's resolved fact (a Know
  has no tool call; `Value` is its only content).
- Convergence (a Need's edge folding onto an existing node) is a cross-branch
  edge the widget's tree-shaped rendering cannot express as a second nested box.
- `output.Model.SelectedToolEvent` unconditionally excluded any plan-tagged
  tool_result from Pin, and there was no selection concept for an individual node
  inside a plan widget at all — so no plan finding, from either engine, was ever
  pinnable, only the plan's own auto-generated rollup summary (ADR 0010 §4).

## Design

### 1. `wavefront.Scheduler.applyClassify` reports fresh children via the existing `NodeDecomposed`

```
GIVEN a Step's classify response spawns one or more brand-new child nodes
      (command-valued Needs always; open-value Needs with no existing match)
WHEN  the response is merged
THEN  the parent's observer receives exactly one NodeDecomposed(parent, freshChildren)
      call, in the same wire shape the continuous engine already uses
  AND no NodeDecomposed call fires for a node whose classify response resolved
      it directly (a self-match Know) or spawned nothing new.

GIVEN a Step's classify response's Need converges onto an already-existing node
      (registerOrConvergeNeed finds a match via findExistingNode)
WHEN  the response is merged
THEN  that child is NOT included in the NodeDecomposed children list
  AND the observer instead receives NodeConverged(parentID, existingNode), via
      the optional scheduler.ConvergenceObserver interface (type-asserted, so an
      Observer that doesn't implement it — e.g. a bare test stub — simply never
      receives the call).
```

### 2. `Observer.NodeCompleted` carries `value, errText`

```
GIVEN either engine's setStatus writes a terminal Status/Value/Error onto a node
WHEN  it notifies the observer
THEN  NodeCompleted receives the same value/errText that were just written —
      never a separate, potentially-stale copy.

GIVEN planObserver.NodeCompleted receives a non-empty value or errText
WHEN  it publishes the task_node "completed" event
THEN  the payload includes "value" and/or "error" keys (only when non-empty)
  AND planTrees.completed records them onto the durable PlanTreeNode.

GIVEN the output/context plan widget applies a "completed" delta carrying value
WHEN  the node is a Step (not a Task) and is later expanded
THEN  its resolved value renders in a "🧩 value" box, the same collapsible
      treatment a Task's result gets (drawTextBox, shared code) — hidden while
      collapsed, boxed excerpt while expanded, capped at maxResultLines.

GIVEN a Step's errText is non-empty instead
WHEN  it renders expanded
THEN  it shows "⚠ error" instead of "🧩 value" — error takes precedence over a
      stale value if somehow both are set.
```

### 3. Convergence renders as a reference annotation, never a duplicate box

```
GIVEN parent P's classify response converges a Need onto existing node E
WHEN  the plan widget renders P expanded
THEN  P's content includes a line "↳ converges onto: <E's goal>"
  AND E's own box, wherever its real (first) owner rendered it, is unaffected —
      E's content is never drawn a second time under P.

GIVEN the "converged" task_node event arrives before E's own first event
WHEN  the widget folds it
THEN  E is ensure()'d with the goal carried on the converged event, so the
      annotation has a real goal to show even if E hasn't been dispatched yet
      from the widget's point of view (an enrich-only ensure, same discipline
      every other ensure call site already follows).
```

### 4. Node-level pin cursor and dispatch

```
GIVEN a plan widget that is NOT the selected top-level widget
WHEN  SelectedPlanNode is called, or ActiveNodeNext/Prev is invoked
THEN  SelectedPlanNode reports ok=false, and ActiveNodeNext/Prev is a no-op —
      the cursor only exists, and only moves, "inside" a selected plan widget.

GIVEN a plan widget becomes selected for the first time
WHEN  its node cursor is read
THEN  it defaults to the plan's root node.

GIVEN the active node is a Task (or command-Need) whose tagged tool_result has
      arrived (its ordinal was captured into planNode.resultOrdinal)
WHEN  SelectedPlanNode is called
THEN  it returns HasOrdinal=true with that ordinal — Pin dispatches through the
      existing, unmodified PinToolEvent(ordinal, false) path.

GIVEN the active node is a Step (e.g. a wavefront Know) with a resolved Value
      and no tool result at all
WHEN  SelectedPlanNode is called
THEN  it returns HasValue=true with that value — Pin dispatches through the new
      PinPlanNode(root, nodeID) path instead.

GIVEN the active node has been dispatched but not yet resolved (no ordinal, no
      value)
WHEN  SelectedPlanNode is called
THEN  ok=false — nothing pinnable yet.

GIVEN the context surface's Key handler
WHEN  the user presses Tab or Shift+Tab
THEN  it calls ActiveNodeNext/ActiveNodePrev respectively (both previously-free
      keys in this surface's keymap).
```

### 5. `PinPlanNode` — server-side, authoritative, and never live-eligible

```
GIVEN a plan node with a non-empty Value in the durable plan-tree registry
WHEN  PinPlanNode(root, nodeID) is called
THEN  it creates a session.Fact{Owner: OwnerPin, Value: <the node's Value>,
      Source: nil, Enabled: true} and returns its key
  AND the fact's key is derived from the node's own goal text (human-readable)
      plus the node id (for uniqueness), not an opaque identifier alone.

GIVEN a plan/node id PinPlanNode cannot find, or a node whose Value is empty
      (dispatched but not yet resolved)
WHEN  PinPlanNode is called
THEN  it returns an error and creates no fact.

GIVEN a fact PinPlanNode created (Source == nil, by construction)
WHEN  SetFactLive(key, true) is called on it
THEN  it is refused ("fact is not pinned to a tool source") — the same existing
      refusal PinToolEvent's live=true path already enforces for a Source-less
      fact, not new gating logic.

GIVEN the working-memory surface's own "l" keybinding
WHEN  the selected fact has Source == nil (a plan-node pin)
THEN  pressing "l" is a client-side no-op — it never even reaches the server
      (PD-WM-AF-009's existing guard, unchanged).
```

## Tests

- `internal/runtime/wavefront/scheduler_test.go`:
  `TestObserverSeesDecomposeAndConverge` — a two-branch convergence scenario
  (mirrors the existing `TestCrossBranchConvergence`) asserting `NodeDecomposed`
  fires with the root's fresh children and exactly one `NodeConverged` call
  covers the converged branch, never a second `NodeDecomposed` entry for it.
- `internal/surfaces/output/plan_test.go`:
  - `TestWavefrontStepValueRenders` — a Step child nests under its root (the
    core bug fix) and its resolved value renders in a `🧩 value` box once the
    plan ends.
  - `TestWavefrontConvergenceAnnotates` — a converging parent shows the `↳`
    annotation; the converged-onto node's content is not duplicated.
  - `TestSelectedPlanNodeNavigationAndPin` — cursor defaults to root, navigates
    forward/backward through Task (ordinal) and Step (value) nodes correctly,
    reports `ok=false` before selection and on an unresolved node, and the `›`
    cursor renders only on the active node while the widget is selected.
- `internal/runtime/plan_tree_test.go` — extended to assert `completed` records
  `Value` onto the durable `PlanTreeNode`.
- `internal/runtime/scheduler/observer_test.go` — `recObserver.NodeCompleted`
  updated to the new signature.
- `tests/features/transport/working_memory_api.feature` /
  `tests/steps/transport/http_steps.go` — three new scenarios exercising the
  real HTTP round trip: `PinPlanNode` success, refusal on an unresolved node,
  and refusal to ever set a plan-node pin live.

## Addendum — the "🌊" provenance tag (ADR 0012, same-day addendum)

`task.Record.Provenance.Source` already distinguished wavefront-produced nodes
(`"wavefront"`) from the continuous engine's (`"planner"`, or empty for its own
root) — it just never reached the wire or the widget.

```
GIVEN a node whose Provenance.Source is "wavefront"
WHEN NodeDispatched/NodeDecomposed fires for it, or a task_plan snapshot
     includes it
THEN the published payload carries "source": "wavefront"
  AND the widget's node gains that source
  AND its title line renders a "🌊 " tag immediately before the status glyph

GIVEN a node whose Provenance.Source is "planner" or empty
WHEN it renders
THEN no "🌊" tag appears
```

Test: `internal/surfaces/output/plan_test.go`'s `TestWavefrontSourceTag` — a
wavefront root and wavefront child both show the tag; a `"planner"`-sourced
sibling in the same plan does not.

## Addendum — engine-selection routing, Origin, and the durable snapshot (ADR 0012, later same-day addendum)

Prompted by reviewing session `brave-lantern`: the engine actually selected
for a plan invocation had no test pinning it to `Settings.WavefrontEnabled`,
`Provenance.Source` never reached the durable `plans/<rootID>.json` snapshot
(only the live wire/widget), and `Source` alone can't distinguish a wavefront
Know from a wavefront Need, or the continuous engine's Step from its Task —
the signal needed to observe *how* a plan solved its problem (chain-of-thought
step/action chain vs. tree-of-thought know/need branch-and-converge), not just
which engine ran it.

```
GIVEN Settings.WavefrontEnabled and a live wavefront classifier
WHEN runPlanPhase decides which engine to run
THEN selectedEngine(true, classifier) returns "wavefront"
  AND selectedEngine(false, classifier) returns "planner"
  AND selectedEngine(true, nil) returns "planner" (no classifier built — the
      buildWavefront() gate never fired, so there is nothing to route to)

GIVEN a node with Provenance{Source: "wavefront", Origin: "need"} (or "know")
 WHEN it dispatches or decomposes
 THEN the TASK_NODE/TASK_PLAN payload carries "source" and "origin"
  AND the durable PlanTreeNode (plans/<rootID>.json) carries the same
      Source/Origin — not just the live event stream

GIVEN a continuous-engine node built by planner.go
 WHEN it is a Task record
 THEN Provenance.Origin is "action"
 WHEN it is a Step record
 THEN Provenance.Origin is "step"

GIVEN a wavefront node built by wavefront/merge.go
 WHEN it comes from registerOrConvergeKnow creating a fresh node
 THEN Provenance.Origin is "know"
 WHEN it comes from registerOrConvergeNeed (command- or open-valued)
 THEN Provenance.Origin is "need"
 WHEN a Know instead resolves an EXISTING node via self-match
 THEN no new record is created, so no Origin is stamped by this path
```

Tests: `TestSelectedEngineRouting` (`internal/runtime/plan_cycle_test.go`);
extended `TestPlanTreeRegistryDispatchDecomposeComplete`
(`internal/runtime/plan_tree_test.go`); extended
`TestCommandNeedExecutesAndUnblocksParent`,
`TestOpenNeedSpawnsChildClassifiedInTurn`, `TestSelfMatchResolvesDirectly`
(`internal/runtime/wavefront/scheduler_test.go`).
