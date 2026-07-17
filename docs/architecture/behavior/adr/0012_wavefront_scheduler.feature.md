# Behavior — `wavefront.Scheduler` (ADR 0012 Phase 7)

Status: **Implemented** (2026-07-17). Realizes ADR 0012's Phased Build Plan
(amendment) step 7, informed by the same-day addendum's node-lifecycle and
scope-reduction findings. Highest-risk phase in this effort — the priority contract
to pin, per ADR 0008's own precedent for its scheduler.

Built exactly as scoped below, in `scheduler.go` (the dispatch loop, `execute`,
`synthesize`, `setStatus`) and `merge.go` (`applyClassify` and everything
convergence-related). All 11 tests in `scheduler_test.go` passed on the first run
against the design as written — every subtlety worked out in the behavior doc
(unified self-match, cycle-guard reuse for both the immediate-self and
deeper-ancestor cases, cross-branch convergence determinism) held up unchanged in
code. Full suite and `-race` on the whole package clean.

**One known, deliberate gap:** `scheduler.Observer.NodeDecomposed` is never called
— `setStatus` (and therefore `NodeCompleted`) is wired for every terminal
transition, but a node spawning children via `applyClassify` does not currently
notify the observer of that (unlike the continuous engine's `applyDecompose`, which
calls `NodeDecomposed` after merging). Not required for this phase's own
correctness or its tests; left as a Phase 8 consideration if UI parity with the
continuous engine's live plan visibility (ADR 0009 §9c) is wanted for wavefront-run
plans.

## Scope

**In scope:** the scheduling loop over injected `Classifier`/`scheduler.Executor`
collaborators — continuous dispatch, the two-tier node lifecycle, merge-time
convergence for open-value Needs, command-Need execution, `Value`/`Error`/`Seq`
population. **Not in scope:** orchestrator wiring (Phase 8), the eval harness
(Phase 9), command-Need dedup (deferred per the addendum), a dedicated
always-on synthesis call (Open Question 1, still open).

## Design

### 1. Every wavefront node is either a further question or a resolved command — reusing `task.Kind` exactly

There is no wavefront-specific node-kind concept. A `Need` with a `Command` becomes
a `task.KindTask` child, dispatched through `execute` (mirrors
`scheduler.Scheduler.execute` almost verbatim, reusing `scheduler.Executor`) exactly
like the continuous engine's Task leaves. A `Need` with no `Command` becomes a
`task.KindStep` child, dispatched through `classify` again — recursively, the same
rules, same cap. The root is always `task.KindStep`. `Kind` dispatch in `work()` is
the same cheap switch `scheduler.Scheduler.work` already has; only the `KindStep`
branch's action differs (classify instead of decompose).

### 2. The two-tier lifecycle: `classified`/`awaitingResolution`, generalizing `scheduler.go`'s `dispatched`/`decomposed`

- `classified[id]` — this node's own classify (or execute) dispatch has happened,
  ever, at most once. Mirrors `dispatched`; a re-dispatch attempt against a node
  already in this map (and not `awaitingResolution`) is `scheduler.ErrStalled`
  (reused directly, same signal, same meaning).
- `awaitingResolution[id]` — this node's classify response spawned deps it's now
  waiting on (mirrors `decomposed`). Once such a node is `Ready()` again (all deps
  Done), the dispatch scan does **not** unconditionally mark it Done the way a
  continuous-engine join does — it runs one more check (§4): free (a self-match
  already exists in the graph) or one bounded synthesis call, itself gated by the
  same `inflight` map so it cannot be double-dispatched. No third bookkeeping map is
  needed — `inflight` alone prevents a re-scan from re-triggering the synthesis
  dispatch while it's outstanding.

### 3. Working-memory rendering happens on the main loop, never inside a worker goroutine

`task.Graph` has no internal lock — its safety depends entirely on the "graph
mutated only on the main loop goroutine" discipline `scheduler.go` already
established. Worker goroutines in wavefront must never call `graph.Nodes()`
directly; the WM string (Done facts + open-question names, "THINGS TO BE
CONSIDERED") is rendered on the main loop at dispatch time and passed into `work`/
`classify` as a plain, already-immutable string parameter — the same pattern
`scheduler.work` already uses by passing a `task.Record` copy rather than a graph
reference.

```
GIVEN two nodes are dispatched in the same pass (concurrent classify calls)
WHEN  their goroutines run
THEN  neither ever touches *task.Graph directly — race-detector clean by
      construction, not by locking.
```

### 4. Know registration is a single mechanism that also *is* the self-match check — no separate self-match code path

Every `Know` in a classify response, regardless of whether it happens to match the
dispatching node's own goal, goes through the same fold:

```
registerOrConvergeKnow(know):
    existingID, found := findExistingNode(know.Name)   // normalized-name scan, ALL
                                                          // nodes (Done or not) —
                                                          // a match on an
                                                          // already-Done node is a
                                                          // no-op; a match on an
                                                          // open node resolves it
                                                          // now with this value.
    if found:
        if that node isn't already Done: setStatus(existingID, Done, know.Value, "")
        return existingID
    create a new, already-Done, free-standing node for it (Provenance: wavefront)
    return its new id
```

Because the dispatching node N is itself already a graph node under its own exact
goal text, a `Know` whose name matches N's own goal (totAlX's documented, imperfect
"echo the question verbatim" pattern) is found by the *same* `findExistingNode` scan
that finds any other match — it resolves N via the identical code path used for
every other convergence, not a special case bolted on separately. `onClassifyComplete`
checks `resolvedID == N.ID` after folding every Know to know whether N resolved
itself this way; if so, N's Needs from the same response are discarded (a node that
just resolved has nothing left to wait on) and no `awaitingResolution` entry is made.

### 5. Need registration reuses `task.Graph`'s existing cycle guard — no new ancestor-walk needed

A `Need` with no command either converges onto an existing node (found via the same
`findExistingNode` scan as Knows) or spawns a brand-new `KindStep` child (`Deps: []`
at creation, so it can never itself close a cycle on `Add` — cycle risk exists only
on the *convergence* path, where the target node might already be N's own ancestor).
`task.Graph.validate`'s existing, already-tested cycle detection (built for the
continuous engine, ADR 0008 Phase 1) catches this for free: attempting
`graph.Update(N)` with a new dep pointing at one of N's own ancestors fails with
`task.ErrCycle`, because the ancestor is already transitively reachable back to N
through the existing parent→child edges. This is the exact case totAlX built a
bespoke ancestor-reachability check to guard against (a model "echoing its own
question back to itself as if it were a NEED it needs answered," silently stalling
the whole run) — wavefront gets the same protection from code that already existed
for an unrelated reason, by construction, not by porting totAlX's guard.

```
GIVEN a Need's registration would close a dependency cycle (converging onto one of
      the dispatching node's own ancestors)
WHEN  the edge is attempted
THEN  graph.Update returns task.ErrCycle; that one Need is skipped (not wired,
      never added to N's Deps) and processing continues with the rest of the
      response's Needs — a single bad Need degrades gracefully, it never fails the
      whole node.

GIVEN a command-valued Need
WHEN  it's registered
THEN  no convergence/existence-check runs at all — it always becomes a new
      task.KindTask child and executes unconditionally (deferred dedup, addendum
      finding #2).
```

### 6. A node's own resolution check runs once right after folding its Knows/Needs, and again whenever an `awaitingResolution` node becomes `Ready()`

Same predicate both times — `findExistingNode`, filtered to `Status == Done` — just
invoked from two call sites: immediately after `onClassifyComplete` folds a
response (catches the immediate self-match case, §4), and from the dispatch scan
when a previously-`awaitingResolution` node's deps are all satisfied (catches the
case where the answer was sitting in the graph via a converged/registered Need, or
appeared from an unrelated branch after N's own deps happened to also resolve — see
Open Point below for the accepted trade-off).

```
GIVEN N's dependencies are all Done (Ready() again) and N has no self-match
WHEN  the dispatch scan reaches it
THEN  it dispatches exactly one fallback synthesis call (wavefront's
      DefaultSynthesisPromptTemplate, schema-free, no Think — same posture as
      output summarization) — never a second classify call. Its result, success or
      failure, resolves N terminally (Done/Value or Failed/Error) — there is no
      third attempt.
```

**Accepted trade-off, not built here:** this only rechecks a node when *its own*
deps are satisfied, not on every graph change — a self-match that becomes available
from an unrelated branch while N's own deps are still pending is not noticed until
N's own deps also resolve. This is a performance cost (a redundant wait), never a
correctness one (N will still resolve correctly once its own deps are done, whether
or not the earlier match would have short-circuited it) — deliberately deferred,
same "accuracy first, then performance" posture as the command-Need dedup deferral.

### 7. Termination is exactly `scheduler.ErrStalled`, reused directly, plus the existing depth/children caps

No new mechanism. Depth cap × per-node children cap bounds total node count, hence
total dispatches; `classified[id]` guarantees each node's own classify-or-execute
call happens at most once, mirroring `dispatched`'s exact guarantee; a re-dispatch
attempt is `scheduler.ErrStalled` (imported and returned directly — same error, same
meaning, no wavefront-local duplicate). A `KindStep` node at max depth fails to
`doneAsk` (`task.Abstained`) exactly like the continuous engine, never recurses
past the bound.

### 8. `Value`/`Error`/`Seq` population reuses Phase 5's discipline exactly

`setStatus(id, status, value, errText string)` — identical signature and semantics
to `scheduler.Scheduler.setStatus`. Command-Need execution failures carry
`out.Reason` as `Error`, successes carry `out.Result.Preview` (summarized via
`wavefront.NewCondenser`/`TruncateFindings` when oversized — the same mechanics
Phase 7a relocated here) as `Value`. Synthesis failures carry the chat error's text;
synthesis successes carry the model's answer text directly (no "insufficient
information" special-casing — the synthesis prompt already instructs the model to
say so plainly, and that prose is a legitimate `Value`, not an error).

## Tests

`internal/runtime/wavefront/scheduler_test.go` (new), stub `Classifier`/
`scheduler.Executor` mirroring the continuous scheduler's own test conventions
(`guard_test.go`'s `okExec`, `node_test.go`'s function-typed stubs):

- Termination without a timer (mirrors `TestRunTerminatesWithoutTimer`); ctx
  cancellation (mirrors `TestCtxCancelStops`).
- A classify response with a self-matching Know resolves the root directly, no
  children spawned, discarding any co-present Needs.
- A command-valued Need executes and its result (or failure) sets Value/Error on
  the child, which then unblocks the parent's join.
- An open-value Need with no existing match spawns a new child, classified in turn.
- Convergence: two nodes (dispatched in the same pass or different passes)
  proposing the identical (normalized) open-value Need name end up depending on
  ONE child, not two — asserted via final node count, not timing.
- A Need that would close a cycle (converges onto its own ancestor) is skipped, not
  wired, and does not fail the dispatching node.
- A node with no self-match, all deps resolved, dispatches exactly one synthesis
  call and resolves from its result.
- A synthesis call that errors resolves the node Failed with the error text as
  Error, not a silent stall.
- Depth cap: a `KindStep` at max depth resolves `Abstained`, never recurses further.
- Race-detector clean end to end (`-race`), specifically covering concurrent
  classify dispatches to prove §3's "WM rendered on the main loop only" holds under
  real goroutine scheduling, not just by inspection.
- An all-convergence pass (every dispatched node's Needs converge onto already-open
  nodes, nothing new learned) does not spuriously terminate early or stall
  incorrectly — it's simply not "no progress" in the `ErrStalled` sense, since each
  node was still classified exactly once; genuine stalls are still caught by the
  existing dispatched-at-most-once guarantee.

## Open Points carried forward

- Open Question 1 (dedicated synthesis call vs. reuse `planContext`) is unaffected
  by this phase — `Scheduler.Run` returns the final graph; how its results become
  respond-path context is Phase 8's concern.
- Command-Need dedup (addendum finding #2) remains explicitly deferred; not built
  here.
