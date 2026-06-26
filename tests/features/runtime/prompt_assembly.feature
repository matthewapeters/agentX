# Source contracts:
#   - docs/implementation/04_llm_prompt_tooling_runtime.md (Prompt Stack Model)
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-C2)
#
# Behavior: the assembler composes a system prompt (when present) plus the user
# message, deterministically.

@unit @arch:prompt-assembly
Feature: Prompt assembly
  As the agentx runtime
  I want a deterministic message sequence
  So that the model receives a consistent prompt

  # use-case: UC-PROMPT-ASSEMBLE
  Scenario: System and user messages
    Given a prompt assembler with system prompt "You are AgentX."
    When a prompt is assembled for "hello"
    Then there are 2 messages
    And message 1 is a "system" message with "You are AgentX."
    And message 2 is a "user" message with "hello"

  # use-case: UC-PROMPT-ASSEMBLE
  # variant: no-system
  Scenario: System message omitted when empty
    Given a prompt assembler with no system prompt
    When a prompt is assembled for "hi"
    Then there is 1 message
    And message 1 is a "user" message with "hi"
