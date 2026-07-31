# RETIRED (2026-07-31): the classifier this feature exercises is no longer
# wired into the main loop — see docs/implementation/04_llm_prompt_tooling_runtime.md
# ("The Prompt/Response Loop" / "Legacy: classify / continuation / task-classifier
# pipeline (unwired)") and docs/implementation/90_open_questions.md (D.5). Tagged
# @pending-hook-reintegration instead of @functional so it is excluded from the
# active suites (tests/suites/runtime_godog_test.go only runs @unit/@integration/
# @functional/@e2e) — kept in the corpus, not deleted, since the underlying
# classify package is unwired, not removed, and may return as a hook or a tool.
# The behavior it still covers that remains real (thinking streams before the
# response; thinking budget fallback) is now covered, without the classifier
# dependency, by tests/features/runtime/prompt_loop.feature.
#
# Source contracts (as originally written):
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-D4)
#
# Original behavior: a submitted prompt is classified before it is answered. The
# recorded turn is user_prompt → classification → agent_response, and processing
# moves idle → classify → respond → completed.

@pending-hook-reintegration @arch:runtime-bootstrap
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

  # use-case: UC-CLASSIFY-CYCLE
  # variant: thinking-budget-falls-back-to-direct
  Scenario: Thinking that exceeds the budget falls back to a direct answer
    Given a started orchestrator whose thinking stalls past the budget then replies "fallback answer"
    When the prompt "hi" runs the full cycle
    Then the recorded answer is "fallback answer"
    And the cycle's final state is "completed"
