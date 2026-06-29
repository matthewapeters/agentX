# Source contracts:
#   - docs/implementation/04_llm_prompt_tooling_runtime.md (Instructions and
#     Bootstrap Prompts → Story: bootstrap prompt at startup)
#   - docs/implementation/03_configuration_and_storage.md (User prompt files)
#
# Behavior: a bootstrap prompt (bootstrap-prompt.md) is submitted automatically
# at startup with instructions prefixed; the response opens the session and the
# bootstrap prompt itself is not recorded as a user entry.

@functional @arch:bootstrap-prompt
Feature: Bootstrap prompt at startup
  As a user of agentx
  I want a configured bootstrap prompt run automatically at startup
  So that the session opens with an agent response

  # use-case: UC-BOOTSTRAP-SUBMIT
  Scenario: Bootstrap prompt is submitted with instructions and opens with a response
    Given a started orchestrator with instructions "Sys rules." and bootstrap prompt "Summarize the rules." and a capturing model that replies "ok"
    When the orchestrator runs its bootstrap prompt
    And the bootstrap session is flushed to disk
    Then the model received a system message "Sys rules."
    And the model received a user message "Summarize the rules."
    And an agent response is recorded
    And no user prompt is recorded

  # use-case: UC-BOOTSTRAP-SUBMIT
  # variant: absent
  Scenario: No bootstrap prompt is a no-op
    Given a started orchestrator with no bootstrap prompt and a capturing model that replies "ok"
    When the orchestrator runs its bootstrap prompt
    And the bootstrap session is flushed to disk
    Then no agent response is recorded
