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
  Scenario: A pure conversational turn emits no task events
    Given a started orchestrator whose classifier calls the turn "none"
    When the classifier turn "how are you" is submitted
    Then the session timeline contains no task_proposed event
    And the session timeline contains no task_result event

  # use-case: UC-RT-TASK-EXEC  (TC-RTTASK-003)
  @integration
  Scenario: An actionable turn is reconciled and drained through the executor
    Given the task executor reports "executed"
    And the model's response shows an action
    And a started orchestrator whose classifier calls the turn "artifact"
    When the classifier turn "write hello.txt with hi" is submitted
    Then the session timeline contains a task_result event
    And the task_result event records status "executed"
    And the task_result event records route "reify"

  # use-case: UC-RT-TASK-VOLUNTEERED  (TC-RTTASK-004)
  @integration
  Scenario: A volunteered action on a conversational turn needs approval and is not executed
    Given the task executor reports "executed"
    And the model's response shows an action
    And a started orchestrator whose classifier calls the turn "none"
    When the classifier turn "what does this file contain" is submitted
    Then the session timeline contains no task_proposed event
    And the task_result event records route "confirm"
    And the task_result event records status "needs_approval"

  # use-case: UC-RT-TASK-DIAG  (TC-RTTASK-005)
  # Observability: the three fan-group stage scores (triage/action/response) are
  # surfaced on every classified turn, so a routing decision is never opaque.
  @integration
  Scenario: A classified turn emits a task_diagnostic carrying the three stage scores
    Given a started orchestrator whose classifier calls the turn "artifact"
    When the classifier turn "write hello.txt with hi" is submitted
    Then the session timeline contains a task_diagnostic event
    And the task_diagnostic event records the triage, action, and response scores

  # use-case: UC-RT-TASK-DIAG-SKIP  (TC-RTTASK-006)
  # No turn is silently dropped: a turn that does not execute still leaves a reason.
  @integration
  Scenario: A non-executing turn still emits a task_diagnostic with a skip reason
    Given a started orchestrator whose classifier calls the turn "none"
    When the classifier turn "how are you" is submitted
    Then the session timeline contains a task_diagnostic event
    And the task_diagnostic event outcome is "skipped"
    And the task_diagnostic event reason is not empty
