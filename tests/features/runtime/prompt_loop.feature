# Source contracts:
#   - docs/implementation/04_llm_prompt_tooling_runtime.md (The Prompt/Response
#     Loop; Native tool-calling wire format; The plan_task tool; Hooks framework)
#
# Behavior: the main loop is submit -> LLM (advertising native tool schemas) ->
# detect tool calls vs a chat response -> execute/plan_task -> hook points ->
# loop. There is no classify step (see prompt_cycle.feature/tool_cycle.feature
# for the ordering that replaces classify_respond_cycle.feature, now retired).
# Thinking applies uniformly (Settings.ThinkingEnabled), not per classified
# route. Hooks run at two points every iteration but are a no-op registry in
# this build (no hooks are registered yet).

@functional @arch:runtime-bootstrap
Feature: Native tool-calling prompt loop
  As the agentx runtime
  I want one flat loop driven by the model's own tool-calling
  So that acting on a turn is a model decision, not a pre-classifier's

  # use-case: UC-PROMPT-LOOP
  Scenario: A plain conversational turn produces no tool calls
    Given a started orchestrator with a stub model that replies "hello there"
    When the prompt "hi" runs the loop
    Then the loop's content events are, in order:
      | content_type   |
      | user_prompt    |
      | agent_response |
    And the loop's final state is "completed"

  # use-case: UC-PROMPT-LOOP
  # variant: thinking-applies-uniformly
  Scenario: Thinking streams before the response when enabled
    Given a started orchestrator with thinking enabled whose model thinks "let me check" then replies "the answer"
    When the prompt "hi" runs the loop
    Then the loop's content events are, in order:
      | content_type   |
      | user_prompt    |
      | thinking       |
      | agent_response |
    And the loop's final state is "completed"

  # use-case: UC-PROMPT-LOOP
  # variant: thinking-budget-falls-back
  Scenario: Thinking that exceeds the budget falls back to a direct answer
    Given a started orchestrator with thinking enabled whose thinking stalls past the budget then replies "fallback answer"
    When the prompt "hi" runs the loop
    Then the loop answer is "fallback answer"
    And the loop's final state is "completed"

  # use-case: UC-PROMPT-LOOP
  # variant: plan-task-tool
  Scenario: A turn that calls plan_task completes the investigation and answers
    Given a started orchestrator with plan_task wired whose model calls plan_task then replies "done investigating"
    When the prompt "review this project" runs the loop
    Then the loop's timeline contains a task_plan event
    And the loop answer is "done investigating"
    And the loop's final state is "completed"
