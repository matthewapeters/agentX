# Source contracts:
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-C3)
#   - docs/architecture/runtime_contracts/event-envelope.schema.json
#   - docs/architecture/runtime_contracts/processing-state.schema.json
#   - docs/implementation/04_llm_prompt_tooling_runtime.md (The Prompt/Response Loop)
#
# Behavior: a prompt submission records the user prompt, streams the model's
# response as transient agent_delta chunks then one complete agent_response, and
# drives processing-state
# working/respond → completed (or → failed on model error), with deterministic
# event ordering recoverable from the persisted event log. No classification event
# fires — the native tool-calling loop has no classify step (see
# tests/features/runtime/prompt_loop.feature).

@functional @arch:runtime-bootstrap
Feature: Prompt cycle orchestration
  As the agentx runtime
  I want a submitted prompt to drive the model and event stream deterministically
  So that the chat surface renders a consistent exchange

  # use-case: UC-PROMPT-CYCLE
  Scenario: Streaming response completes
    Given a started orchestrator with a stub model that streams "Hel", "lo"
    When the prompt "hi" is submitted
    And the orchestrator is shut down
    Then the recorded content events are, in order:
      | content_type   | text  |
      | user_prompt    | hi    |
      | agent_response | Hello |
    And the final processing state is "completed"

  # use-case: UC-PROMPT-CYCLE
  # variant: model-failure
  Scenario: Model failure reports a failed cycle
    Given a started orchestrator with a stub model that fails with "ollama down"
    When the prompt "hi" is submitted
    And the orchestrator is shut down
    Then an error event with "ollama down" is recorded
    And the final processing state is "failed"
