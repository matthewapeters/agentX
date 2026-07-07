# Source contracts:
#   - docs/architecture/adr/0008-recursive-task-decomposition-and-dag-scheduler.md (Phase 4c)
#   - docs/architecture/behavior/adr/0008_task_decomposition_integration.feature.md
#   - internal/runtime/decompose (Oracle, Decomposer adapters)
#
# Behavior: the scheduler's injected seams become concrete. The Oracle calls a node atomic
# only when the action classifier resolves its goal AND a one-step check passes (the ADR §2
# refinement). The Decomposer plans a non-atomic goal in an isolated branch that inherits
# the parent's working-memory context, returning only child records + synthesis. Stubbed
# classifier/planner — the live LLM wiring is Phase 4d/4e.

@runtime @task-decompose @arch:adr-0008
Feature: the oracle and decomposer adapters bridge the scheduler to the classifier and branch
  As Phase 4 integration
  I want the scheduler's atomicity test and decomposition backed by the real collaborators
  So that decomposition runs on the live classifier and a plan-only branch

  # use-case: UC-DEC-ORACLE-RESOLVE  (ADR-0008 P4-005a)
  @unit
  Scenario: A resolved goal that is not one-step is judged not atomic
    Given the action classifier resolves the goal to "query"
    And the one-step check reports not one-step
    When the oracle evaluates the goal
    Then the goal is judged not atomic

  # use-case: UC-DEC-ORACLE-ATOMIC  (ADR-0008 P4-005b)
  @unit
  Scenario: A resolved one-step goal is judged atomic
    Given the action classifier resolves the goal to "command"
    And the one-step check reports one-step
    When the oracle evaluates the goal
    Then the goal is judged atomic

  # use-case: UC-DEC-ORACLE-ABSTAIN  (ADR-0008 P4-005c)
  @unit
  Scenario: A goal the action classifier abstains on is not atomic
    Given the action classifier abstains on the goal
    When the oracle evaluates the goal
    Then the goal is judged not atomic

  # use-case: UC-DEC-DECOMPOSER  (ADR-0008 P4-006)
  @unit
  Scenario: The decomposer plans in a branch and returns children plus synthesis
    Given a stub planner returning sub-goals "a" and "b" with synthesis "investigate then propose"
    And the parent has an enabled fact "project" = "agentX"
    When the goal "review the project" is decomposed
    Then the result carries 2 child records
    And the decomposition synthesis is "investigate then propose"
    And the planner saw the parent fact "project" in its context
