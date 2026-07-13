# Source contracts:
#   - docs/architecture/behavior/adr/0010_oversized_tool_output_recovery.feature.md
#   - docs/build-plan/04_tool_runtime_backlog.md (TOOL-6, Phase A)
#
# Behavior: when a tool result comes back Truncated (the output_max_bytes capture
# safety net triggered), the orchestrator offers a decision instead of silently
# proceeding on the partial result — reusing the same interactive decision gate
# tool-approval and verb-continuation already use. An "always" choice is
# remembered per tool id so a later truncation from the same tool resolves
# without asking again, its applied choice still stated in the result text. The
# "capture more" choice is always clamped to a configured absolute ceiling,
# whatever was requested or remembered.

@integration @arch:output-size-recovery
Feature: Oversized tool output recovery
  As the AgentX tool runtime
  I want a truncated tool result to trigger a decision, not silent acceptance
  So that the agent (or the user) can choose to accept it, get more, or abort

  # use-case: UC-OUTSIZE-001
  Scenario: Using the truncated result once does not persist a preference
    Given a started orchestrator with a small output cap and a large absolute cap
    And a command that emits 200 numbered lines is run and truncated
    When an output-size decision is requested
    And the user chooses "use_truncated_once"
    Then the decision is accepted
    And the resulting result is truncated
    And a second output-size decision for the same command is requested
    And the user chooses "abort"
    And the decision is declined

  # use-case: UC-OUTSIZE-002
  # variant: always
  Scenario: Always using truncated results is remembered without re-prompting
    Given a started orchestrator with a small output cap and a large absolute cap
    And a command that emits 200 numbered lines is run and truncated
    When an output-size decision is requested
    And the user chooses "use_truncated_always"
    Then the decision is accepted
    And the resulting result is truncated
    And a second output-size decision for the same command resolves without prompting
    And the decision is accepted
    And the resulting preview mentions "remembered preference"

  # use-case: UC-OUTSIZE-003
  Scenario: Capturing more once returns a larger, untruncated result
    Given a started orchestrator with a small output cap and a large absolute cap
    And a command that emits 200 numbered lines is run and truncated
    When an output-size decision is requested
    And the user chooses "expand_once"
    Then the decision is accepted
    And the resulting result is not truncated
    And the resulting result reports 200 lines

  # use-case: UC-OUTSIZE-004
  # variant: always
  Scenario: Always capturing more is remembered and auto-expands next time
    Given a started orchestrator with a small output cap and a large absolute cap
    And a command that emits 200 numbered lines is run and truncated
    When an output-size decision is requested
    And the user chooses "expand_always"
    Then the decision is accepted
    And the resulting result is not truncated
    And a second output-size decision for the same command resolves without prompting
    And the decision is accepted
    And the resulting result is not truncated
    And the resulting preview mentions "remembered preference"

  # use-case: UC-OUTSIZE-005
  Scenario: Aborting declines the result
    Given a started orchestrator with a small output cap and a large absolute cap
    And a command that emits 200 numbered lines is run and truncated
    When an output-size decision is requested
    And the user chooses "abort"
    Then the decision is declined

  # use-case: UC-OUTSIZE-006
  Scenario: Interrupting while awaiting fails cleanly
    Given a started orchestrator with a small output cap and a large absolute cap
    And a command that emits 200 numbered lines is run and truncated
    When an output-size decision is requested
    And the awaiting output-size request is interrupted
    Then the output-size request fails

  # use-case: UC-OUTSIZE-007
  # variant: ceiling-clamp
  Scenario: A remembered cap can never exceed the configured absolute ceiling
    Given a persisted expand override for the command with an oversized cap
    And a started orchestrator with a small output cap and a tiny absolute cap
    And a command that emits 2000 numbered lines is run and truncated
    When an output-size decision is requested and resolves without prompting
    Then the decision is accepted
    And the resulting result is truncated
    And the resulting preview mentions "remembered preference"
