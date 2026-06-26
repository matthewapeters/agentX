# Source contracts:
#   - docs/architecture/channel_registry.md (Processing State Contract)
#   - docs/architecture/runtime_contracts/processing-state.schema.json
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-B4)
#
# Behavior: the chat surface shows a processing-state indicator (idle/working +
# phase) that tracks the session processing-state feed.

@functional @arch:processing-indicator
Feature: Chat processing-state indicator
  As a user of the chat surface
  I want a working indicator
  So that I can see when the agent is busy and in which phase

  # use-case: UC-CHAT-STATUS
  Scenario: Status starts idle
    Given a new chat surface sized 40 by 10
    Then the chat status shows "idle"

  # use-case: UC-CHAT-STATUS
  # variant: working
  Scenario: Status reflects the working phase
    Given a new chat surface sized 40 by 10
    When the processing state becomes working in phase "respond"
    Then the chat status shows "working"
    And the chat status shows "respond"

  # use-case: UC-CHAT-STATUS
  # variant: spinner
  Scenario: Spinner animates while awaiting the response
    Given a new chat surface sized 40 by 10
    When the processing state becomes working in phase "respond"
    Then a spinner tick advances the indicator
