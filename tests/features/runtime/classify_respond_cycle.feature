# Source contracts:
#   - docs/implementation/04_llm_prompt_tooling_runtime.md (Classification Cycle:
#     events and ordering)
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-D4)
#
# Behavior: a submitted prompt is classified before it is answered. The recorded
# turn is user_prompt → classification → agent_response, and processing moves
# idle → classify → respond → completed.

@functional @arch:runtime-bootstrap
Feature: Classify-respond prompt cycle
  As the agentx runtime
  I want every prompt classified before the response
  So that intent routing is part of the chat/response cycle

  # use-case: UC-CLASSIFY-CYCLE
  Scenario: A prompt is classified then answered
    Given a started orchestrator that classifies as "respond_directly" and replies "hello there"
    When the prompt "hi" runs the full cycle
    Then the cycle's content events are, in order:
      | content_type   |
      | user_prompt    |
      | classification |
      | agent_response |
    And the recorded classification route is "respond_directly"
    And the cycle's final state is "completed"

  # use-case: UC-CLASSIFY-CYCLE
  # variant: reserved-route-still-responds
  Scenario: A reserved route still produces a response
    Given a started orchestrator that classifies as "single_tool" and replies "done"
    When the prompt "edit the file" runs the full cycle
    Then the recorded classification route is "single_tool"
    And the cycle's final state is "completed"

  # use-case: UC-CLASSIFY-CYCLE
  # variant: thinking-streams-before-response
  Scenario: Thinking streams before the response when enabled
    Given a started orchestrator that thinks "let me check" then replies "the answer"
    When the prompt "hi" runs the full cycle
    Then the cycle's content events are, in order:
      | content_type   |
      | user_prompt    |
      | classification |
      | thinking       |
      | agent_response |
    And the cycle's final state is "completed"
