# Source contracts:
#   - docs/architecture/adr/0008-recursive-task-decomposition-and-dag-scheduler.md (Phase 4b)
#   - docs/architecture/behavior/adr/0008_task_decomposition_integration.feature.md
#   - internal/prompting/planner (Parse: plan JSON -> child task records)
#
# Behavior: the planner parses the model's plan JSON into child task records under a parent,
# rewriting local step ids to session-unique child ids and remapping deps so the sub-DAG is
# well-formed. A malformed plan (no steps, missing id/goal, dangling dep) is rejected. This
# is the deterministic, LLM-free core of decomposition planning (the prompt is tuned later).

@prompting @planner @arch:adr-0008
Feature: the planner parses a plan into child task records
  As the decomposition step
  I want a compound goal's plan JSON turned into DAG-shaped child records
  So that the scheduler can drive the sub-plan

  # use-case: UC-PLAN-001  (ADR-0008 P4-007)
  @unit
  Scenario: A plan parses into child records with remapped dependencies
    Given a parent task id "task-5"
    And a planner response with steps "g1" and "g2" where "g2" depends on "g1"
    When the plan is parsed
    Then it yields 2 child records
    And a child goal for "g1" exists
    And the child for "g2" depends on the child for "g1"
    And every child id is namespaced under "task-5"

  # use-case: UC-PLAN-002
  @unit
  Scenario: A plan with no steps is rejected
    Given a parent task id "task-5"
    And a planner response with no steps
    When the plan is parsed
    Then parsing fails

  # use-case: UC-PLAN-003
  @unit
  Scenario: A step depending on an unknown step is rejected
    Given a parent task id "task-5"
    And a planner response whose step depends on an undefined step
    When the plan is parsed
    Then parsing fails
