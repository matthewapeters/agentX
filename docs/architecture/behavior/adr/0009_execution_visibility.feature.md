# Behavior — ADR 0009 §9a: Streamed Plan Events + Spiral Guard

Slice: **9a + spiral guard** (built together — visibility is the safety net, the guard is
the cure; tidy-cove RCA 2026-07-07). Status: **DONE**.

## Spiral guard

### decompose.stripResultPlumbing / SimilarGoals (guard.go)
- GIVEN a goal "Run `ls -la` on X and capture its output"
  WHEN plumbing is stripped THEN the action "Run `ls -la` on X" remains — the executor
  returns results automatically, so plumbing clauses are not a second step.
- GIVEN two tidy-cove chain rungs ("Run … and save output to $OUTPUT" vs "Execute … and
  write stdout to $OUTPUT") WHEN compared THEN they are similar (stopwords dropped, verb
  synonyms folded, ≥0.8 containment of the smaller token set).
- GIVEN a legitimate child ("Read README to understand features") of a review-project
  parent WHEN compared THEN not similar (real decomposition is never blocked).

### decompose.Decomposer.Decompose (non-progress guard)
- GIVEN the planner returns a child whose goal echoes the parent's
  WHEN decomposing THEN it returns an error wrapping `scheduler.ErrNoProgress` and no
  children — refusing to fund a recursion that cannot advance.

### scheduler.work (ErrNoProgress fallback)
- GIVEN the decomposer returns ErrNoProgress for a node the oracle judged non-atomic
  WHEN the worker handles it THEN the node is executed as an atomic leaf (Done/Failed by
  outcome), dispatched exactly once — never recursed, never marked Failed for the refusal.

### decompose.HeuristicOneStep (hardened)
- GIVEN "Run `ls -la` on <dir> and capture its output" WHEN judged THEN one-step (plumbing
  stripped first).
- GIVEN "enumerate all files and directories" WHEN judged THEN one-step (noun "and" is not
  clause chaining; only " and <action-verb>", " then ", ";" chain).
- GIVEN "review the project and identify a feature" WHEN judged THEN not one-step.

### decompose.DefaultMaxDepth
- GIVEN a plan recursion reaching depth 3 WHEN a node at the bound is non-atomic THEN it
  resolves to Ask — a spiral costs at most 3 levels, not 10.

### planner.PromptTemplate (generator-side rules)
- Steps are one verb+object action; result plumbing is explicitly forbidden with a
  WRONG/RIGHT example ("the tool returns results automatically"); no shell syntax; the
  goal itself is never restated as a step.

## §9a streamed events

### scheduler.Observer (WithObserver option; callbacks on the main loop, never concurrent)
- GIVEN an observer WHEN a node is handed to a worker THEN NodeDispatched(rec, depth)
  fires before the oracle call.
- GIVEN a decomposition lands WHEN the parent becomes a join THEN NodeDecomposed(parent,
  children) fires with the admitted children.
- GIVEN any node reaches a terminal status THEN NodeCompleted(id, status) fires (all
  terminal transitions flow through setStatus).
- GIVEN a nil observer THEN the scheduler is silent and behavior is unchanged.

### Orchestrator.runPlanPhase / planObserver / publishPlan (plan_cycle.go)
- GIVEN the plan cycle starts THEN processing shows the `planning` phase and an initial
  `task_plan` snapshot (phase "started", the root node) is published before any work.
- GIVEN the scheduler drains WHEN each transition happens THEN a `task_node` delta event
  (dispatched / decomposed / completed) is published immediately — batch-emit at
  completion is retired (the documented ADR 0009 invariant).
- GIVEN the drain ends THEN a final `task_plan` snapshot (phase "ended", executed count)
  is published; a plan that executed nothing carries a loud "plan blocked" error, never
  silence.
- GIVEN the background Decompose route (runDecomposition) THEN it streams through the same
  observer and uses the same depth bound.

### state
- `task_node` is a valid content type; `planning` is a valid processing phase.

## Tests
`scheduler/observer_test.go` (lifecycle stream order + no-progress fallback),
`decompose/guard_test.go` (tidy-cove chain similarity, legit-decomposition negative,
echo-planner refusal), `decompose/live_test.go` (hardened heuristic cases),
`decompose/drain_test.go` (observer-threaded signature). Orchestrator-level event
assertions remain deferred (fixed-verdict harness limitation, per Phase 4d note).
