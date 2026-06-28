# Source contracts:
#   - docs/implementation/04_llm_prompt_tooling_runtime.md (Classification Cycle:
#     output contract, retry and fallback)
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-D3)
#
# Behavior: the classifier extracts a tolerant strict-JSON verdict, validates the
# route enum, retries on failure, and falls back to respond_directly.

@functional @arch:prompt-classification
Feature: Prompt classifier
  As the agentx runtime
  I want a reliable route from a possibly-noisy model verdict
  So that prompts are dispatched deterministically and never stall

  # use-case: UC-CLASSIFY
  Scenario: A clean verdict yields its route
    Given a classifier with retries 1
    When the classifier model returns:
      """
      {"route": "single_tool", "confidence": 0.8, "rationale": "edit a file"}
      """
    Then the classified route is "single_tool"

  # use-case: UC-CLASSIFY
  # variant: tolerant-extraction
  Scenario: JSON wrapped in prose and fences is still parsed
    Given a classifier with retries 1
    When the classifier model returns:
      """
      Sure, here is my classification:
      ```json
      {"route": "respond_directly"}
      ```
      """
    Then the classified route is "respond_directly"

  # use-case: UC-CLASSIFY
  # variant: malformed-falls-back
  Scenario: Unparseable output retries then falls back
    Given a classifier with retries 1
    When the classifier model returns:
      """
      I cannot answer in JSON.
      """
    Then the classified route is "respond_directly"

  # use-case: UC-CLASSIFY
  # variant: unknown-route-falls-back
  Scenario: An unknown route falls back
    Given a classifier with retries 0
    When the classifier model returns:
      """
      {"route": "frobnicate"}
      """
    Then the classified route is "respond_directly"
