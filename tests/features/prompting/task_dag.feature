# Source contracts:
#   - docs/architecture/adr/0008-recursive-task-decomposition-and-dag-scheduler.md (Phase 1)
#   - docs/architecture/behavior/adr/0008_task_dag_substrate.feature.md
#   - internal/prompting/task/graph.go (Graph: fold, integrity, queries)
#
# Behavior: task.Graph is an I/O-free projection of the append-only task event stream.
# Given records with deps it stores, reloads, queries, and guards a well-formed DAG.
# The event log is the source of truth; folding the same events twice is deterministic.
# No LLM, no scheduler, no branch context — pure substrate (Phase 1 of ADR 0008).

@prompting @task @arch:adr-0008
Feature: the task DAG substrate stores, reloads, queries, and guards edges
  As the Phase-3 scheduler and the replay/audit path
  I want task records with dependency edges projected into a well-formed DAG
  So that a plan survives persistence and its structure is trustworthy

  # use-case: UC-RTDAG-001
  @unit
  Scenario: A standalone task round-trips with empty deps
    Given a proposed task "t1" with no dependencies
    When the task events are persisted and reloaded
    Then the reconstructed DAG has 1 node
    And node "t1" has empty deps
    And the reload is byte-identical to the original

  # use-case: UC-RTDAG-002
  @unit
  Scenario: A record set with edges reconstructs the same DAG
    Given a proposed task "a" with no dependencies
    And a proposed task "b" with no dependencies
    And a proposed task "c" depending on "a" and "b"
    When the task events are persisted and reloaded
    Then the reconstructed DAG has 3 nodes
    And the edge set is exactly "a->c, b->c"
    And the roots are exactly "a, b"

  # use-case: UC-RTDAG-003
  @unit
  Scenario: The DAG is reconstructed from the event log, deterministically
    Given a proposed task "a" with no dependencies
    And a proposed task "b" with no dependencies
    And a proposed task "c" depending on "a" and "b"
    When the DAG is rebuilt from the event log twice
    Then both reconstructions are identical

  # use-case: UC-RTDAG-004
  @unit
  Scenario: Roots are the nodes with no dependencies
    Given a proposed task "a" with no dependencies
    And a proposed task "b" with no dependencies
    And a proposed task "c" depending on "a" and "b"
    When the roots are queried
    Then the roots are exactly "a, b"

  # use-case: UC-RTDAG-005
  @unit
  Scenario: A node is ready only when every dependency is done
    Given a proposed task "a" with no dependencies
    And a proposed task "b" with no dependencies
    And a proposed task "c" depending on "a" and "b"
    And node "a" transitions to "done"
    When the ready set is queried
    Then "c" is not in the ready set
    And node "b" transitions to "done"
    When the ready set is queried
    Then "c" is in the ready set

  # use-case: UC-RTDAG-006
  @unit
  Scenario: An edge to an unknown node is refused
    Given a proposed task "x" depending on "ghost"
    When "x" is admitted to the DAG
    Then admission fails with a dangling-dependency error
    And the DAG has 0 nodes

  # use-case: UC-RTDAG-007
  @unit
  Scenario: A dependency cycle is refused
    Given a proposed task "q" with no dependencies
    And a proposed task "p" depending on "q"
    When "q" is updated to depend on "p"
    Then admission fails with a cycle error
    And the DAG remains acyclic

  # use-case: UC-RTDAG-008
  @unit
  Scenario: Node ids are unique within a session
    Given a proposed task "dup" with no dependencies
    When a second task claims id "dup"
    Then admission fails with a duplicate-id error

  # use-case: UC-RTDAG-009
  @unit
  Scenario: A status update supersedes append-only, latest wins
    Given a proposed task "t1" with no dependencies
    When node "t1" is updated to status "done"
    Then node "t1" has status "done"
    And the prior "proposed" event is retained in the log
