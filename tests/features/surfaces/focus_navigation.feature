# Source contracts:
#   - docs/ux/06_OUTPUT_WIDGET.md (Focus & keymap, border hierarchy)
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-D5)
#
# Behavior: the chat surface tracks which panel has focus. ESC,↑ moves focus to
# the output; ESC,↓ returns it to the input; PgUp/PgDn auto-focus the output.
# The focused panel renders a bold active-color border, the other an inactive
# (dark gray) border.

@functional @arch:chat-surface @ux:PD-01 @ux:PD-02
Feature: Panel focus and themed borders
  As a user of the chat surface
  I want a clear focus toggle between the input and output panels
  So that I can scroll responses and return to typing without ambiguity

  # use-case: UC-CHAT-FOCUS
  Scenario: ESC then up focuses the output panel
    Given a new chat surface sized 40 by 10
    Then the input panel is focused
    When ESC is pressed
    And the "up" key is pressed
    Then the output panel has focus

  # use-case: UC-CHAT-FOCUS
  # variant: return
  Scenario: ESC then down returns focus to the input panel
    Given a new chat surface sized 40 by 10
    When ESC is pressed
    And the "up" key is pressed
    And ESC is pressed
    And the "down" key is pressed
    Then the input panel is focused

  # use-case: UC-CHAT-FOCUS
  # variant: pgup-autofocus
  Scenario: PgUp auto-focuses the output panel
    Given a new chat surface sized 40 by 10
    When the "pgup" key is pressed
    Then the output panel has focus

  # use-case: UC-CHAT-FOCUS-BORDER
  Scenario: Panels render themed focus borders
    Given a new chat surface sized 40 by 10
    Then the chat view shows the active border color
    And the chat view shows the inactive border color
