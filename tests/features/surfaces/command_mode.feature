# Source contracts:
#   - docs/build-plan/03_chat_surface_backlog.md (chat surface, command mode)
#   - docs/ux/03_PANEL_DETAILS.md PD-02 (re-authored for the TUI)
#
# Behavior: a vi-style command mode triggered by ESC. ESC from insert opens a
# ":" command line (with a hint strip of commands); ESC again dismisses it.
# Quit verbs exit. While a response is working, ESC arms then confirms an
# interrupt, and the hint strip advertises this.

@functional @arch:chat-surface @ux:PD-02
Feature: Vi-style command mode
  As a user of the agentx chat surface
  I want ESC-driven commands and a discoverable hint strip
  So that I can quit cleanly and interrupt a response without guessing keys

  # use-case: UC-CHAT-COMMAND
  Scenario: ESC opens the command line
    Given a new chat surface sized 40 by 10
    Then the chat hint shows "esc → command options"
    When ESC is pressed
    Then the chat hint shows ":q quit"

  # use-case: UC-CHAT-COMMAND
  # variant: dismiss
  Scenario: ESC again dismisses the command line
    Given a new chat surface sized 40 by 10
    When ESC is pressed
    And ESC is pressed
    Then the chat hint shows "esc → command options"

  # use-case: UC-CHAT-COMMAND
  # variant: quit
  Scenario: A quit command requests exit
    Given a new chat surface sized 40 by 10
    When the user enters the command "q"
    Then the surface requests quit

  # use-case: UC-CHAT-INTERRUPT
  Scenario: Hint advertises interrupt while working, with confirm
    Given a new chat surface sized 40 by 10
    When the processing state becomes working in phase "respond"
    Then the chat hint shows "esc → interrupt"
    When ESC is pressed
    Then the chat hint shows "esc again to confirm interrupt"

  # use-case: UC-CHAT-INTERRUPT
  # variant: confirm-stops-runtime
  Scenario: Confirming the interrupt stops the runtime
    Given a chat surface that records interrupts sized 40 by 10
    When the processing state becomes working in phase "respond"
    And ESC is pressed
    And ESC is pressed
    And the interrupt is confirmed
    Then the runtime is interrupted
