# Source contracts:
#   - docs/architecture/channel_registry.md (event channels + processing-state)
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-B5)
#
# Behavior: a submitted prompt is handed to the runtime, and the resulting event
# stream and processing-state transitions render back into the panels. Verified
# against a stub runtime (the real Ollama prompt cycle lands in Phase C).

@e2e @arch:chat-round-trip
Feature: Chat prompt round trip
  As a user of the chat surface
  I want my prompt to reach the runtime and the response to stream back
  So that I can hold a conversation

  # use-case: UC-CHAT-SUBMIT
  Scenario: Submitting hands the prompt to the runtime
    Given a chat surface wired to a stub runtime
    When the user submits "ping"
    Then the runtime received prompt "ping"

  # use-case: UC-CHAT-ROUNDTRIP
  Scenario: A submitted prompt streams a response into the output
    Given a chat surface wired to a stub runtime
    When the user submits "ping"
    Then the output eventually contains "ping"
    And the output eventually contains "pong"
    And the chat status eventually shows "completed"
