# Source contracts:
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-D5, ESC chord)
#   - docs/ux/06_OUTPUT_WIDGET.md (Focus & keymap)
#
# Behavior: ESC is a leader/chord key. ESC,q quits; ESC,↑ focuses the output;
# ESC,↓ focuses the input; ESC,ESC interrupts an in-flight response. A pending
# chord is advertised in the hint strip and canceled by an unrecognized key.

@functional @arch:chat-surface @ux:PD-02
Feature: ESC chord command surface
  As a user of the agentx chat surface
  I want an ESC-driven chord and a discoverable hint strip
  So that I can quit cleanly and interrupt a response without guessing keys

  # use-case: UC-CHAT-COMMAND
  Scenario: ESC opens the chord menu
    Given a new chat surface sized 40 by 10
    Then the chat hint shows "esc → options"
    When ESC is pressed
    Then the chat hint shows "q quit"

  # use-case: UC-CHAT-COMMAND
  # variant: dismiss
  Scenario: ESC again cancels the chord
    Given a new chat surface sized 40 by 10
    When ESC is pressed
    And ESC is pressed
    Then the chat hint shows "esc → options"

  # use-case: UC-CHAT-COMMAND
  # variant: quit
  Scenario: ESC then q requests exit
    Given a new chat surface sized 40 by 10
    When ESC is pressed
    And the "q" key is pressed
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
