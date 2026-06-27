# Source contracts:
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-C3, prompt cycle)
#   - docs/architecture/runtime_contracts/processing-state.schema.json
#
# Behavior: canceling the submit context interrupts the in-flight model call.
# The partial response is kept, no error event is recorded, and the cycle ends
# in the completed state (interruption is a user action, not a failure).

@functional @arch:runtime-bootstrap
Feature: Prompt interruption
  As a user of the chat surface
  I want to interrupt a response in progress
  So that I can stop a long or unwanted generation without it being an error

  # use-case: UC-PROMPT-INTERRUPT
  Scenario: Interrupting a streaming response ends the cycle cleanly
    Given a started orchestrator with a stub model that blocks until canceled
    When the prompt "hi" is submitted in the background
    And the in-flight prompt is interrupted
    And the session is flushed to disk
    Then no error event is recorded
    And the interrupted cycle ends "completed"
