# Source contracts:
#   - docs/ux/03_PANEL_DETAILS.md PD-CTX (enable/disable an element)
#   - docs/implementation/03_configuration_and_storage.md (Enabled Semantics)
#
# Behavior: toggling a conversation element's enabled state (the context surface's
# management path) controls whether it folds into the agent's upcoming assembled
# context. The orchestrator applies it in memory (effective next turn) and persists
# it in the element's event file.

@integration @arch:context-surface @ux:PD-CTX
Feature: Enable/disable a conversation element
  As a user managing my agent's context
  I want to disable a prior turn
  So that it stops consuming budget and influencing upcoming answers

  # use-case: UC-CTX-TOGGLE
  Scenario: A disabled response is withheld from the next context
    Given an orchestrator that captures context and answers "alpha"
    When the prompt "first question" is submitted for toggling
    And the last agent response is disabled
    And the prompt "second question" is submitted for toggling
    Then the captured context omits "alpha"
    And the captured context includes "first question"
    And the recorded agent response is persisted disabled

  # use-case: UC-CTX-TOGGLE
  # Re-enabling folds the element back into context.
  Scenario: Re-enabling a response restores it to context
    Given an orchestrator that captures context and answers "beta"
    When the prompt "first question" is submitted for toggling
    And the last agent response is disabled
    And the last agent response is re-enabled
    And the prompt "second question" is submitted for toggling
    Then the captured context includes "beta"
