# Source contracts:
#   - docs/ux/03_PANEL_DETAILS.md PD-02 (AF-001/002/003 + stop, re-authored for TUI)
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-B3)
#
# Behavior: the input panel edits a multi-line prompt, submits on Enter, inserts
# a newline on Shift+Enter, disables submit while streaming, and offers a stop
# action while streaming. Submitting echoes the prompt into the output.

@functional @arch:input-panel @ux:PD-02
Feature: Input panel controls
  As a user of the chat surface
  I want to type, submit, and interrupt prompts
  So that I can drive the conversation

  # use-case: UC-INPUT-TYPE
  Scenario: Typing builds the input value
    Given a focused input panel
    When the user types "hello"
    Then the input value is "hello"

  # use-case: UC-INPUT-SUBMIT
  Scenario: Enter submits non-empty input
    Given a focused input panel
    When the user types "hi"
    And the user presses enter
    Then the input reports a submit action

  # use-case: UC-INPUT-SUBMIT
  # variant: empty
  Scenario: Enter on empty input does nothing
    Given a focused input panel
    When the user presses enter
    Then the input reports no action

  # use-case: UC-INPUT-NEWLINE
  Scenario: Shift+Enter inserts a newline
    Given a focused input panel
    When the user types "a"
    And the user presses shift+enter
    And the user types "b"
    Then the input value is "a\nb"

  # use-case: UC-INPUT-STREAMING
  Scenario: Submit is disabled while streaming
    Given a focused input panel
    When the user types "x"
    And the input is set streaming
    And the user presses enter
    Then the input reports no action

  # use-case: UC-INPUT-STOP
  Scenario: Stop is available while streaming
    Given a focused input panel
    When the input is set streaming
    And the user presses esc
    Then the input reports a stop action

  # use-case: UC-CHAT-ECHO
  Scenario: Submitting a prompt echoes it into the output
    Given a new chat surface sized 40 by 16
    When the user types "hi there" and submits
    Then the chat output contains "hi there"
