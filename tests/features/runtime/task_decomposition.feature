# Source contracts:
#   - docs/architecture/adr/0008-recursive-task-decomposition-and-dag-scheduler.md
#   - docs/architecture/cascade_classifier.md (reconciliation fold)
#   - Fixture: the canonical project-review prompt (see memory + ADR 0008 § oracle)
#
# STATUS: contract-first. These scenarios spec ADR 0008 behavior that is NOT yet built.
# They deliberately carry NO run-tag (@unit/@integration/@functional/@e2e), so no suite
# runner selects them and `make all` stays green. Each scenario notes the tier + phase at
# which it earns its run-tag and its steps get implemented. Add the run-tag in the SAME
# commit that lands the steps — never a run-tag without a step definition (Strict mode
# fails undefined steps).
#
# The fixture prompt is canonical and reused across tiers:
#   "review the current project and identify one feature that is under-developed"
# Observed before-state trace (why this exists):
#   action=query@0.95 (resolved!)  response=abstained{produced:2 executed:1 neither:1}
#   → reconciled to ask "classification uncertain".  Desired: reconcile to DECOMPOSE.

@runtime @task-decomposition @arch:adr-0008
Feature: a compound goal is recognized, decomposed to atomic steps, and scheduled
  As the server's request-handling brain
  I want a non-atomic goal broken down until every leaf is executable
  So that "review the project and propose a feature" actually investigates the project
  And independent steps can run in parallel

  # ======================================================================
  # GOAL 1 — the classification methodology routes a compound goal correctly.
  # Type-resolution is not atomicity: a confident single-type verdict whose
  # response scatters toward "produced" is compound, not uncertain.
  # ======================================================================

  # tier: @integration — activate at Phase 4 (Decompose route)
  # UC-RTDECOMP-001
  Scenario: A confident-but-compound turn reconciles to decompose, not ask
    Given a classifier that scores the turn action "query" at 0.95
    And the response classifier abstains with spread produced:2 executed:1 neither:1
    When the classifier turn "review the current project and identify one feature that is under-developed" is submitted
    Then the reconciled route is "decompose"
    And the session timeline contains no task_result event with route "ask"

  # tier: @integration — activate at Phase 4
  # UC-RTDECOMP-002  (the sibling discriminator: genuine ambiguity still asks)
  Scenario: A genuinely ambiguous turn still reconciles to ask
    Given a classifier that scores the turn action "query" at 0.5
    And the response classifier abstains with spread none:2 produced:1 executed:1
    When the classifier turn "maybe look at some stuff" is submitted
    Then the reconciled route is "ask"

  # tier: @functional (real Ollama) — activate at Phase 4; tolerant eval, not a gate.
  # UC-RTDECOMP-003. Asserts SHAPE over runs, never an exact verdict.
  # NOTE: intentionally left as an eval contract; a single pass/fail on a stochastic
  # classifier is flaky. When wired, express as "in at least K of N runs".
  Scenario: The canonical review prompt is not routed to a single atomic executor step
    Given a live classifier against the running project
    When the fixture review prompt is submitted
    Then the turn is not reconciled to a single atomic action
    And the routing is one of "decompose" or "ask"

  # ======================================================================
  # GOAL 3 — decompose an ambiguous goal into atomic steps (branch context).
  # (Placed before Goal 2 because a leaf must exist before it can drain to a tool.)
  # ======================================================================

  # tier: @integration — activate at Phase 2 (branch context) + Phase 4
  # UC-RTDECOMP-010
  Scenario: A non-atomic goal is decomposed until every leaf is atomic
    Given a decomposer that expands the fixture review prompt
    When the goal is decomposed in a branch context
    Then every leaf task is atomic
    And an atomic leaf is one the action classifier resolves to a single type and the response classifier calls one-step
    And the decomposition halts at or before max_task_depth

  # tier: @integration — activate at Phase 2
  # UC-RTDECOMP-011  (the branch is plan-only: no writes, no tool runs during planning)
  Scenario: Decomposition in a branch context produces no side effects
    Given a decomposer that expands the fixture review prompt
    When the goal is decomposed in a branch context
    Then no executor tool is invoked during decomposition
    And no artifact is written during decomposition
    And only the child task records and a synthesis return to the parent context

  # tier: @unit — activate at Phase 4 (oracle in isolation)
  # UC-RTDECOMP-012  (regression anchor for the fixture's exact trace)
  Scenario: A resolved single type is not accepted as atomic without one-step satisfiability
    Given a task whose action classifies as "query" at 0.95
    And whose response classifier does not call it one-step
    When the atomicity oracle evaluates the task
    Then the task is judged not atomic
    And the task is enqueued for decomposition

  # ======================================================================
  # GOAL 2 — classification scores properly engage the tools infrastructure:
  # an atomic leaf actually drains through the reconcile→executor path.
  # ======================================================================

  # tier: @integration — activate at Phase 3 (scheduler) + existing executor machinery
  # UC-RTDECOMP-020
  Scenario: An atomic leaf from a decomposed plan drains through the executor
    Given a decomposed plan whose first atomic leaf classifies as "command"
    And the task executor reports "executed"
    When the scheduler dispatches the runnable leaf
    Then the session timeline contains a task_result event
    And the task_result event records status "executed"

  # tier: @integration — activate at Phase 3
  # UC-RTDECOMP-021  (atomic-before-undertake: a non-atomic node never reaches the executor)
  Scenario: A non-atomic node is never dispatched to the executor
    Given a plan node that is not yet atomic
    And the task executor reports "executed"
    When the scheduler evaluates the node
    Then the node is enqueued for decomposition
    And no task_result event is recorded for that node

  # ======================================================================
  # GOAL 4 — parallelization. Tested STRUCTURALLY (DAG shape + runnable set),
  # not by racing goroutines. Concurrency-in-fact is a separate scheduler unit test.
  # ======================================================================

  # tier: @integration — activate at Phase 3
  # UC-RTDECOMP-030
  Scenario: Independent investigation steps are schedulable in parallel
    Given a decomposed plan for the fixture review prompt
    Then at least two leaf tasks share no dependency path
    And the scheduler reports more than one runnable task in the first wave

  # tier: @unit — activate at Phase 3 (scheduler with a fake executor + bounded budget)
  # UC-RTDECOMP-031  (width is clamped to the shared slot budget, not unbounded)
  Scenario: Concurrent dispatch is bounded by the shared slot budget
    Given a scheduler with a slot budget of 2
    And a plan with 4 independent runnable leaves
    When the scheduler dispatches the first wave
    Then at most 2 leaves are dispatched concurrently
    And the remaining leaves stay runnable pending a freed slot
