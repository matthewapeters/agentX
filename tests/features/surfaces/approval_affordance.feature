# Source contracts:
#   - docs/implementation/05_security_approvals_and_command_policy.md (approval round-trip)
#   - docs/build-plan/04_tool_runtime_backlog.md (TOOL-3)
#
# Behavior: while the runtime awaits an interactive decision (tool approval,
# verb-continuation approval, or any future kind), the chat surface swaps a
# navigable-list widget into the input panel — titled "AgentX Needs Your
# Input" — showing the prompt and options it was handed. Up/down (or j/k)
# moves a highlighted-row cursor; Enter confirms the highlighted option and
# echoes back its Decision string. One interaction model regardless of what's
# being decided.

@functional @arch:chat-surface @ux:PD-01
Feature: Interactive-decision approval affordance
  As a user of the chat surface
  I want a navigable option list with visible feedback while a decision awaits me
  So that I always know which option I'm about to choose, regardless of what's being asked

  # use-case: UC-CHAT-APPROVAL
  Scenario: The widget shows the attention border title and options while awaiting input
    Given a new chat surface sized 40 by 10
    When the processing state becomes awaiting input
    Then the chat hint shows "AgentX Needs Your Input"
    And the chat hint shows "Approve for this session"
    And the chat hint shows "Deny"

  # use-case: UC-CHAT-APPROVAL
  Scenario: The hint advertises navigate/confirm regardless of decision kind
    Given a new chat surface sized 40 by 10
    When the processing state becomes awaiting input
    Then the chat hint shows "navigate"
    And the chat hint shows "confirm"

  # use-case: UC-CHAT-APPROVAL
  # variant: confirm the first (default-highlighted) option
  Scenario: Pressing enter on the default selection approves for the session
    Given a chat surface that records approvals sized 40 by 10
    When the processing state becomes awaiting input
    And the "enter" key is pressed
    Then the approval decision is "session"

  # use-case: UC-CHAT-APPROVAL
  # variant: navigate down once
  Scenario: Navigating down once and confirming approves globally
    Given a chat surface that records approvals sized 40 by 10
    When the processing state becomes awaiting input
    And the "down" key is pressed
    And the "enter" key is pressed
    Then the approval decision is "global"

  # use-case: UC-CHAT-APPROVAL
  # variant: navigate down twice
  Scenario: Navigating down twice and confirming denies
    Given a chat surface that records approvals sized 40 by 10
    When the processing state becomes awaiting input
    And the "down" key is pressed
    And the "down" key is pressed
    And the "enter" key is pressed
    Then the approval decision is "deny"

  # use-case: UC-CHAT-APPROVAL
  # variant: navigate past the end and back up
  Scenario: The cursor clamps at the last option and can navigate back up
    Given a chat surface that records approvals sized 40 by 10
    When the processing state becomes awaiting input
    And the "down" key is pressed
    And the "down" key is pressed
    And the "down" key is pressed
    And the "up" key is pressed
    And the "enter" key is pressed
    Then the approval decision is "global"
