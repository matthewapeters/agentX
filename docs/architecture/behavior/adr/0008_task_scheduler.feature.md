# Feature: ADR 0008 Phase 3 — Task DAG Scheduler

Status: **Implemented** (2026-07-06). Continuous concurrency model, parent-as-join
decomposition. Code: `internal/runtime/scheduler/scheduler.go`. Runnable contract:
`tests/features/runtime/task_scheduler.feature` (@unit, UC-RTSCHED-001…010). All six leans
accepted. **Integration seam not yet wired** (Phase 4): the real `Oracle` (classifier) and
`Decomposer` (branch-backed) — both are injected interfaces exercised here with stubs.

Realizes **Phase 3** of `docs/architecture/adr/0008-recursive-task-decomposition-and-dag-scheduler.md`:
*"The readiness state machine (BLOCKED / NEEDS-DECOMPOSE / RUNNABLE / DONE / FAILED), single
shared slot budget, cycle + depth guards, breadth-first v1."*

The scheduler is the load-bearing piece: it walks the live `task.Graph`, drives every node
through one state machine, and — treating "not yet atomic" as just another not-ready state —
makes lazy decomposition and interleaved execution fall out of a single loop. It is
**LLM-free itself**: the atomicity test and the decomposition are injected collaborators
(the classifier and the Phase-2 branch), so the scheduler's orchestration logic is unit-
testable with deterministic stubs. The real classifier/decomposer wiring is Phase 4.

Schema / source links:

- `internal/prompting/task/graph.go` (Phase 1 `Graph`: `Ready`, `Roots`, `Dependents`, `Add`,
  `Update`, statuses `Proposed/Ready/InProgress/Done/Failed/Cancelled/Abstained`)
- `internal/runtime/branch` (Phase 2 `Branch`, `Result`, `MergeResult`, depth bound)
- `internal/executor` (`Outcome`, `Status`; the `Execute(ctx, task.Record) Outcome` seam)

## Scope

**In scope (Phase 3 — the scheduling loop over injected collaborators):**

- **One state machine, derived each step.** For every non-terminal node the scheduler
  computes exactly one state:
  - **DONE** — status `Done`. Terminal; unblocks dependents.
  - **FAILED** — status `Failed`. Terminal; its dependents can never become ready (v1: the
    plan surfaces the failure; no auto-retry — see Open Point 2).
  - **BLOCKED** — a dependency is not yet `Done`. Wait.
  - **IN-FLIGHT** — a decompose or execute is running for it (scheduler-tracked). Skip.
  - **RUNNABLE** — deps all `Done`, the oracle calls it **atomic**, not in-flight → dispatch
    to the executor.
  - **NEEDS-DECOMPOSE** — deps all `Done`, **not atomic**, depth `< maxDepth`, not in-flight
    → dispatch to the decomposer (a Phase-2 branch); its `Result` merges children into the
    graph, expanding the frontier.
  - **NEEDS-CLARIFY (Ask)** — not atomic at `maxDepth`: recursion is bounded, so instead of
    decomposing forever the node is marked for a clarifying question (ADR's fail-to-Ask).
- **Lazy + interleaved.** A node is expanded only when it reaches the frontier (deps met);
  a ready atomic leaf executes while its siblings are still being decomposed.
- **Shared slot budget.** At most `slots` dispatches (decompose **or** execute) run at once,
  clamped to the Ollama fan-out plateau (`[[ollama-fanout-concurrency-curve]]`). The
  classifier's own fan-out draws from the same budget in production (Open Point 4).
- **Breadth-first priority (v1).** The ready set is dispatched in the graph's deterministic
  first-seen order; a cost-weighted critical-path order is v1.5 (Open Point 3).
- **Guards.** Cycle/dangling integrity is inherited from `Graph.Add` (Phase 1); depth is
  inherited from `branch.Fork` (Phase 2). The scheduler adds no new integrity rules.
- **Termination.** The loop ends when no node is runnable, needs-decompose, or in-flight —
  i.e. every node is terminal or permanently blocked by a failure.

**Out of scope (later):** the real atomicity oracle and decomposer (Phase 4, LLM);
re-decomposition on failure (ADR OQ2); cost-weighted critical-path ordering (v1.5);
persisting scheduler progress for mid-run resume.

## Contract

Injected collaborators (all stubbable, no LLM in the scheduler itself):

- `Oracle interface { Atomic(ctx, task.Record) (bool, error) }` — the atomicity test:
  type-resolves **and** one-step (the ADR §2 refinement). Backed by the classifier in prod.
- `Decomposer interface { Decompose(ctx, task.Record) (branch.Result, error) }` — runs a
  read-restricted branch and returns child records + synthesis; the scheduler `MergeResult`s
  them into the live graph.
- `Executor interface { Execute(ctx, task.Record) executor.Outcome }` — the existing seam.

`Scheduler{graph, oracle, decomposer, executor, slots}` with `Run(ctx) error`.

Invariants:

- **Atomic-before-undertake.** A node reaches the executor only via RUNNABLE, i.e. only
  after the oracle calls it atomic. A non-atomic node is never executed.
- **Deps-before-node.** A node is dispatched only when every dependency is `Done` (reuses
  `Graph.Ready`).
- **Bounded concurrency.** In-flight dispatches never exceed `slots`.
- **Frontier growth is monotonic and integrity-checked.** Decomposition only adds nodes and
  edges through `Graph.Add`, so the DAG stays acyclic and dangling-free throughout.
- **Deterministic dispatch order** for a fixed plan + fixed oracle verdicts (breadth-first).

Concurrency model (recommended): continuous — dispatches run as goroutines gated by a
`slots` semaphore; a completion channel drives graph updates (`Update` status / `MergeResult`)
and re-evaluation. This gives true interleaving (a leaf executes while a sibling decomposes),
per the ADR. Tests use deterministic stub collaborators and assert on final graph state, max
observed concurrency, and dispatch order — not on wall-clock timing.

## Behavior

```gherkin
@adr0008 @phase3 @scheduler @positive
Scenario: ADR-0008-P3-001 Independent atomic leaves are all dispatched to the executor
  Given a plan of atomic leaves "a", "b", "c" with no dependencies
  And the executor reports every leaf "executed"
  When the scheduler runs
  Then "a", "b", and "c" each reach status "done"
  And the decomposer is never invoked

@adr0008 @phase3 @scheduler @deps
Scenario: ADR-0008-P3-002 A node runs only after its dependencies are done
  Given a plan where "c" depends on atomic leaves "a" and "b"
  And the executor reports every leaf "executed"
  When the scheduler runs
  Then "a" and "b" are dispatched before "c"
  And "c" reaches status "done"

@adr0008 @phase3 @scheduler @decompose
Scenario: ADR-0008-P3-003 A non-atomic node is decomposed, not executed
  Given a plan with a single non-atomic node "goal"
  And the decomposer expands "goal" into atomic leaves "g1" and "g2"
  And the executor reports every leaf "executed"
  When the scheduler runs
  Then the executor is never invoked for "goal"
  And "g1" and "g2" reach status "done"

@adr0008 @phase3 @scheduler @decompose @frontier
Scenario: ADR-0008-P3-004 Decomposition expands the frontier and the parent joins on its children
  Given a plan with a single non-atomic node "goal"
  And the decomposer expands "goal" into atomic leaves "g1" and "g2"
  When the scheduler runs
  Then "g1" and "g2" are scheduled after "goal" is decomposed
  And "goal" reaches status "done" only after "g1" and "g2" are done

@adr0008 @phase3 @scheduler @budget
Scenario: ADR-0008-P3-005 Concurrent dispatch is bounded by the slot budget
  Given a slot budget of 2
  And a plan of 4 independent atomic leaves
  And an executor that blocks until released
  When the scheduler runs
  Then at most 2 leaves are in flight at once
  And all 4 leaves eventually reach status "done"

@adr0008 @phase3 @scheduler @interleave
Scenario: ADR-0008-P3-006 A ready leaf executes while a sibling is still decomposing
  Given a plan with an atomic leaf "fast" and a non-atomic node "slow" with no dependency between them
  And a decomposer that blocks until released
  When the scheduler runs
  Then "fast" reaches status "done" before "slow" is decomposed

@adr0008 @phase3 @scheduler @failure @negative
Scenario: ADR-0008-P3-007 A failed leaf blocks its dependents and the plan surfaces the failure
  Given a plan where "c" depends on atomic leaves "a" and "b"
  And the executor reports "a" failed
  When the scheduler runs
  Then "a" reaches status "failed"
  And "c" never reaches status "done"
  And the scheduler terminates

@adr0008 @phase3 @scheduler @depth @negative
Scenario: ADR-0008-P3-008 A non-atomic node at max depth fails to Ask instead of recursing
  Given a plan with a non-atomic node "goal" already at the max task depth
  When the scheduler runs
  Then "goal" is not decomposed further
  And "goal" is marked for clarification rather than executed

@adr0008 @phase3 @scheduler @termination
Scenario: ADR-0008-P3-009 The scheduler terminates when every node is terminal
  Given a plan of atomic leaves that all execute successfully
  When the scheduler runs
  Then the loop exits with no node runnable, needs-decompose, or in flight

@adr0008 @phase3 @scheduler @determinism
Scenario: ADR-0008-P3-010 Dispatch order is deterministic for a fixed plan and verdicts
  Given a plan of atomic leaves "a", "b", "c" with no dependencies
  And a slot budget of 1
  When the scheduler runs twice with the same oracle verdicts
  Then the dispatch order is identical both times
  And it follows the graph's first-seen order
```

## Open points to settle before building

1. **Decomposition-DAG semantics: parent-as-join vs. replacement.** Recommended
   **parent-as-join**: `Decompose(N)` adds children with edges `child → N`, so N becomes a
   join that is BLOCKED until its children are `Done`, then completes without execution (its
   work was the children). This needs no rewiring of N's existing dependents and reuses
   `Graph.Ready` verbatim. The scheduler must distinguish a *decomposed join* (mark `Done`
   when children done) from an *atomic leaf* (execute) — via a scheduler-internal
   `decomposed` set, or a new `task.Status` (e.g. `Joined`). Lean: scheduler-internal set for
   v1, promote to a durable status when mid-run resume is needed. Alternative (replacement:
   remove N, rewire dependents onto the children's sink) is messier and loses N's provenance.
2. **Failure propagation.** v1: a `Failed` node blocks its dependents; the plan surfaces the
   failure and the scheduler terminates cleanly (no auto-retry, no re-decompose — ADR OQ2
   defers that). Confirm this is acceptable for the first cut.
3. **Priority.** v1 breadth-first (graph first-seen order). v1.5 uses a planner-emitted cost
   to prioritize the longest dependency chain so the critical path is not starved. Confirm v1
   for now.
4. **Shared slot budget scope.** Does the scheduler own the one semaphore the classifier
   fan-out *also* draws from, or only bound its own execute/decompose dispatches? Cleanest
   long-term is a single process-wide budget object injected into both; v1 can bound just the
   scheduler's dispatches and unify later. Flagging so we don't accidentally build two pools.
5. **Concurrency model.** Continuous (goroutines + slot semaphore + completion channel — true
   interleaving, recommended) vs. wave-based (dispatch a wave, join, repeat — simpler and
   trivially deterministic, but a long decompose stalls the next wave, violating "interleave
   freely"). Lean continuous, with deterministic stubs keeping tests reproducible.
6. **Package.** `internal/runtime/scheduler` (peer to `internal/runtime/branch`), consuming
   `task.Graph` + `branch` + `executor`. Confirm the boundary.
