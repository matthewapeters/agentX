# Source contracts:
#   - docs/architecture/cascade_classifier.md (externalize the task; task record)
#   - docs/architecture/task_record.md
#   - internal/runtime (buildTaskClassifier presence-gate + maybeEmitTask hook)
#
# Behavior: when a prompt corpus is configured, the orchestrator runs the classifier
# pipeline after the response and emits a durable task_proposed event for an
# actionable turn. A pure-conversation turn emits none. With no corpus the classifier
# is off and the prompt cycle is unchanged (covered by the existing prompt-cycle
# feature — no task_proposed appears there).

@runtime @task-classifier @arch:task-record
Feature: the orchestrator emits a task record when the classifier is wired
  As the server's request-handling brain
  I want an actionable turn externalized as a durable task_proposed event
  So that an action the model commits to survives outside the conversation

  # use-case: UC-RT-TASK  (TC-RTTASK-001)
  @integration
  Scenario: An actionable turn emits a task_proposed event
    Given a started orchestrator whose classifier calls the turn "artifact"
    When the classifier turn "write hello.txt with hi" is submitted
    Then the session timeline contains a task_proposed event
    And the task_proposed event records type "artifact"

  # use-case: UC-RT-TASK-NONE  (TC-RTTASK-002)
  @integration
  Scenario: A conversational turn emits no task_proposed event
    Given a started orchestrator whose classifier calls the turn "none"
    When the classifier turn "how are you" is submitted
    Then the session timeline contains no task_proposed event

  # use-case: UC-RT-TASK-EXEC  (TC-RTTASK-003)
  @integration
  Scenario: An actionable turn is reconciled and drained through the executor
    Given the task executor reports "executed"
    And a started orchestrator whose classifier calls the turn "artifact"
    When the classifier turn "write hello.txt with hi" is submitted
    Then the session timeline contains a task_result event
    And the task_result event records status "executed"
    And the task_result event records route "reify"
