# Source contracts:
#   - docs/ux/03_PANEL_DETAILS.md PD-01/PD-02 (re-authored for the TUI)
#   - docs/architecture/00_ARCHITECTURE_RECONCILIATION.md (2-panel chat surface)
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-B1)
#
# Behavior: the chat surface is a two-panel Bubble Tea program (output on top,
# input on the bottom) that fills the terminal, focuses input on start, and
# quits on Ctrl+C.

@functional @arch:chat-surface @ux:PD-01 @ux:PD-02
Feature: Chat surface two-panel layout
  As a user of the agentx chat surface
  I want a stable output/input layout
  So that I can read responses and type prompts in one view

  # use-case: UC-CHAT-LAYOUT
  # The input height is dynamic: an empty input is one row and the output takes the
  # rest. Chrome: output frame (flex+2) + status (1) + hint (1) + input frame (1+2).
  Scenario: Surface fills the terminal with output, status, hint, and input
    Given a new chat surface
    When the terminal size is 80 by 24
    Then the rendered view has 24 rows
    And the output region is 17 rows
    And the input region is 1 rows

  # use-case: UC-CHAT-LAYOUT
  # the input grows with content (up to its max) and the output shrinks to match
  Scenario: The input grows as the prompt spans more lines
    Given a new chat surface
    When the terminal size is 80 by 24
    And the user types the prompt "one"
    And the user inserts a prompt newline
    And the user types the prompt "two"
    Then the input region is 2 rows
    And the output region is 16 rows

  # use-case: UC-CHAT-FOCUS
  Scenario: Input is focused on start
    Given a new chat surface
    Then the input panel is focused

  # use-case: UC-CHAT-QUIT
  Scenario: Ctrl+C requests quit
    Given a new chat surface
    When the user presses ctrl+c
    Then the surface requests quit
