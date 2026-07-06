# Source contracts:
#   - docs/architecture/task_record.md (record shape, status lifecycle, provenance)
#   - docs/architecture/cascade_classifier.md (§ task record: externalize the task)
#   - internal/prompting/task (FromAction: action decision -> durable record)
#
# Behavior: a recognized action becomes a durable task record decoupled from the turn
# (the vivid-willow fix). A pure-conversation turn emits nothing; an abstained
# classification emits an "abstained" record so the uncertainty survives.

@prompting @task @arch:task-record
Feature: the classifier emits a durable task record
  As the executor pass
  I want recognized actions externalized as durable, DAG-shaped records
  So that an action the model commits to survives outside the conversation

  # use-case: UC-TASK-PROPOSE  (TC-TASK-001)
  @unit
  Scenario: An actionable verdict becomes a proposed task
    Given an action decision "artifact" at confidence 0.9
    When a task is derived with id "t1" goal "write hello.txt"
    Then a task is emitted
    And the task type is "artifact"
    And the task status is "proposed"
    And the task deps are empty

  # use-case: UC-TASK-NONE  (TC-TASK-002)
  @unit
  Scenario: A conversational verdict emits no task
    Given an action decision "none" at confidence 0.9
    When a task is derived with id "t2" goal "chat"
    Then no task is emitted

  # use-case: UC-TASK-ABSTAIN  (TC-TASK-003)
  @unit
  Scenario: An abstained classification emits an abstained task
    Given an abstained action decision
    When a task is derived with id "t3" goal "maybe write a file"
    Then a task is emitted
    And the task status is "abstained"

  # use-case: UC-TASK-PROVENANCE  (TC-TASK-004)
  @unit
  Scenario: Provenance carries the classification confidence and escalation
    Given an action decision "command" at confidence 0.8
    And the decision was escalated
    When a task is derived with id "t4" goal "run the build"
    Then a task is emitted
    And the task provenance confidence is 0.8
    And the task provenance is escalated
