# Source contracts:
#   - docs/implementation/04_llm_prompt_tooling_runtime.md (Instructions and
#     Bootstrap Prompts → Story: instructions prefix every context)
#   - docs/implementation/03_configuration_and_storage.md (User prompt files)
#
# Behavior: standing user instructions (agentx-instructions.md) are prefixed to
# every LLM context, with the user's prompt finishing the context.

@functional @arch:prompt-instructions
Feature: User instructions prefix every context
  As a user of agentx
  I want my standing instructions sent ahead of every prompt
  So that the agent always follows them

  # use-case: UC-INSTRUCTIONS-PREFIX
  Scenario: Instructions lead the context and the prompt finishes it
    Given a started orchestrator with instructions "Always be terse." and a capturing model
    When the user submits the prompt "hello"
    Then the model received a system message "Always be terse."
    And the model received a user message "hello"
    And the system message precedes the user message
