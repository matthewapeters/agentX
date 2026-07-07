# Source contracts:
#   - docs/architecture/adr/0008-recursive-task-decomposition-and-dag-scheduler.md
#     (Phase 4c; amended 2026-07-07: typed DAG nodes — the planner declares Kind, so there
#     is no separate atomicity oracle to adapt here)
#   - docs/architecture/behavior/adr/0008_task_decomposition_integration.feature.md
#   - internal/runtime/decompose (Decomposer adapter)
#
# Behavior: the scheduler's injected Decomposer seam becomes concrete. It plans a Step's
# goal in an isolated branch that inherits the parent's working-memory context, returning
# child records + synthesis. Its own judgment call is the echo check (a child that just
# restates the parent's goal) — not structurally catchable by the planner's JSON-schema
# constraint, since it's semantic. A violation gets one retry with the problem named, then
# gives up (scheduler.ErrNoProgress). Stubbed planner — the live LLM wiring is separate.

@runtime @task-decompose @arch:adr-0008
Feature: the decomposer adapter bridges the scheduler to the branch and planner
  As the Decompose route's integration
  I want decomposition to run on a plan-only branch and enforce the planner's Kind contract
  So that a Step's goal plans into a well-formed child DAG, retrying once on a violation

  # use-case: UC-DEC-DECOMPOSER
  @unit
  Scenario: The decomposer plans in a branch and returns children plus synthesis
    Given a stub planner returning sub-goals "a" and "b" with synthesis "investigate then propose"
    And the parent has an enabled fact "project" = "agentX"
    When the goal "review the project" is decomposed
    Then the result carries 2 child records
    And the decomposition synthesis is "investigate then propose"
    And the planner saw the parent fact "project" in its context

  # use-case: UC-DEC-RETRY
  @unit
  Scenario: A planner that echoes the parent goal on the first attempt but not the retry succeeds
    Given a stub planner that echoes the parent goal on the first attempt only
    When the goal "review the project" is decomposed
    Then the decomposition succeeds
    And the retry context named the violation

  # use-case: UC-DEC-GIVE-UP
  @unit
  Scenario: A planner that always echoes the parent goal exhausts its retry and gives up
    Given a stub planner that always echoes the parent goal
    When the goal "review the project" is decomposed
    Then the decomposition reports no progress
