# Source contracts:
#   - docs/implementation/03_configuration_and_storage.md (Persistence Behavior)
#   - docs/architecture/runtime_contracts/event-envelope.schema.json
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-A4)
#
# Behavior: session events are persisted append-only under events/ and are
# recoverable from disk in epoch order.

@integration @arch:event-persistence
Feature: Append-only session event persistence
  As the agentx runtime
  I want every session event durably recorded
  So that a session timeline is recoverable and auditable

  # use-case: UC-EVENT-PERSIST
  Scenario: Recorded events are recoverable in epoch order
    Given a session with a recorder
    When a "user_prompt" event with epoch 1 is recorded
    And a "tool_result" event with epoch 3 is recorded
    And a "thinking" event with epoch 2 is recorded
    Then 3 event files exist under the session events directory
    And loading events returns epochs in order 1, 2, 3

  # use-case: UC-EVENT-PERSIST
  # variant: append-only
  Scenario: Persistence is append-only
    Given a session with a recorder
    When a "user_prompt" event with epoch 1 is recorded
    And a "user_prompt" event with epoch 1 is recorded
    Then 2 event files exist under the session events directory

  # use-case: UC-EVENT-PERSIST
  # variant: via-bus
  Scenario: Events published to the bus are persisted
    Given a session with a recorder draining the event bus
    When 2 events are published to the bus for the session
    Then loading events returns 2 events
