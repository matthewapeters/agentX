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
  # Layout rows: output (flex) + status (1) + hint (1) + input (3).
  Scenario: Surface fills the terminal with output, status, hint, and input
    Given a new chat surface
    When the terminal size is 80 by 24
    Then the rendered view has 24 rows
    And the output region is 19 rows
    And the input region is 3 rows

  # use-case: UC-CHAT-FOCUS
  Scenario: Input is focused on start
    Given a new chat surface
    Then the input panel is focused

  # use-case: UC-CHAT-QUIT
  Scenario: Ctrl+C requests quit
    Given a new chat surface
    When the user presses ctrl+c
    Then the surface requests quit
