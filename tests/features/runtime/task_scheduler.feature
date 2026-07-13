# Source contracts:
#   - docs/architecture/adr/0008-recursive-task-decomposition-and-dag-scheduler.md
#     (Phase 3; amended 2026-07-07: typed DAG nodes — Kind decides dispatch, not an oracle)
#   - docs/architecture/behavior/adr/0008_task_scheduler.feature.md
#   - internal/runtime/scheduler (Scheduler: state machine, slot budget, parent-as-join)
#
# Behavior: the scheduler walks a live task.Graph, driving each node through one state
# machine, dispatching on the node's declared Kind. A Task leaf executes; a Step node
# decomposes (a branch); a decomposed Step joins on its children. Concurrency is bounded by
# a slot budget; dispatch is breadth-first deterministic. The decomposer/executor are
# injected stubs — no LLM, and atomicity is never inferred (it's declared on the record).

@runtime @task-scheduler @arch:adr-0008
Feature: the scheduler drives a task DAG to completion
  As the runtime's execution brain
  I want a lazy, interleaved scheduler over the task DAG
  So that a plan runs its task leaves and decomposes its step nodes, in parallel where it can

  # use-case: UC-RTSCHED-001
  @unit
  Scenario: Independent task leaves are all dispatched to the executor
    Given a plan of task leaves "a", "b", "c" with no dependencies
    And the executor reports every leaf executed
    When the scheduler runs
    Then "a", "b", "c" each reach status "done"
    And the decomposer is never invoked

  # use-case: UC-RTSCHED-002
  @unit
  Scenario: A node runs only after its dependencies are done
    Given task leaves "a" and "b" with no dependencies
    And a task leaf "c" depending on "a" and "b"
    And the executor reports every leaf executed
    When the scheduler runs
    Then "a" and "b" are dispatched before "c"
    And "c" reaches status "done"

  # use-case: UC-RTSCHED-003
  @unit
  Scenario: A step node is decomposed, not executed
    Given a step node "goal"
    And the decomposer expands "goal" into task leaves "g1" and "g2"
    And the executor reports every leaf executed
    When the scheduler runs
    Then the executor is never invoked for "goal"
    And "g1" and "g2" each reach status "done"

  # use-case: UC-RTSCHED-004
  @unit
  Scenario: A decomposed step joins on its children
    Given a step node "goal"
    And the decomposer expands "goal" into task leaves "g1" and "g2"
    And the executor reports every leaf executed
    When the scheduler runs
    Then "goal" is dispatched before "g1" and "g2"
    And "goal" reaches status "done"

  # use-case: UC-RTSCHED-005
  @unit
  Scenario: Concurrent dispatch is bounded by the slot budget
    Given a slot budget of 2
    And a plan of 4 task leaves with no dependencies
    And the executor reports every leaf executed
    When the scheduler runs
    Then at most 2 workers were in flight at once
    And all 4 leaves reach status "done"

  # use-case: UC-RTSCHED-006
  # Interleaving proven structurally: a task leaf's execution and a sibling step's
  # decomposition are in flight at the same time (peak >= 2), no blocking stub or timer.
  @unit
  Scenario: A task leaf and a decomposing step are in flight together
    Given a task leaf "fast" with no dependencies
    And a step node "slow" with no dependencies
    And the decomposer expands "slow" into task leaves "s1" and "s2"
    And the executor reports every leaf executed
    When the scheduler runs
    Then "fast" and "slow" were in flight together
    And "fast" reaches status "done"
    And "slow" reaches status "done"

  # use-case: UC-RTSCHED-007
  @unit
  Scenario: A failed leaf blocks its dependents and the plan surfaces the failure
    Given task leaves "a" and "b" with no dependencies
    And a task leaf "c" depending on "a" and "b"
    And the executor reports "a" failed
    When the scheduler runs
    Then "a" reaches status "failed"
    And "c" does not reach status "done"
    And the scheduler terminates

  # use-case: UC-RTSCHED-007b
  # TOOL-7: a denied leaf (policy blocked it, or the user declined approval) is a
  # deliberate decision, not a bug — it must not report as "failed" (RCA:
  # nimble-pebble-2 — task-565-1's denied git_status call was indistinguishable
  # from a crash in the plan's terminal report). Its sibling keeps running and
  # completing normally: only the denied node itself is affected.
  @unit
  Scenario: A denied leaf is distinguished from a genuine failure
    Given task leaves "a" and "b" with no dependencies
    And the executor reports "a" denied
    When the scheduler runs
    Then "a" reaches status "denied"
    And "a" does not reach status "failed"
    And "b" reaches status "done"

  # use-case: UC-RTSCHED-008
  @unit
  Scenario: A step node at max depth is marked for clarification, not executed
    Given a max task depth of 0
    And a step node "goal"
    When the scheduler runs
    Then "goal" is not decomposed
    And "goal" reaches status "abstained"
    And the executor is never invoked for "goal"

  # use-case: UC-RTSCHED-009
  @unit
  Scenario: The scheduler terminates when every node is terminal
    Given a plan of task leaves "a", "b", "c" with no dependencies
    And the executor reports every leaf executed
    When the scheduler runs
    Then the run completes
    And no node is left runnable

  # use-case: UC-RTSCHED-010
  @unit
  Scenario: Dispatch order is deterministic and breadth-first
    Given a slot budget of 1
    And a plan of task leaves "a", "b", "c" with no dependencies
    And the executor reports every leaf executed
    When the scheduler runs
    Then the dispatch order is exactly "a, b, c"
