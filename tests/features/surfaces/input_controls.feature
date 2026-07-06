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

  # use-case: UC-INPUT-NEWLINE
  # Alt+Enter is the terminal-agnostic soft-newline (works without key
  # disambiguation, unlike Shift+Enter).
  Scenario: Alt+Enter inserts a newline
    Given a focused input panel
    When the user types "a"
    And the user presses alt+enter
    And the user types "b"
    Then the input value is "a\nb"

  # use-case: UC-INPUT-NEWLINE
  Scenario: Ctrl+J inserts a newline
    Given a focused input panel
    When the user types "a"
    And the user presses ctrl+j
    And the user types "b"
    Then the input value is "a\nb"

  # use-case: UC-INPUT-NEWLINE-HINT
  # The empty input shows a dim soft-newline hint that vanishes once the user types.
  Scenario: The soft-newline hint shows on an empty input and clears on typing
    Given a focused input panel sized 40 wide with max 8 rows
    Then the rendered input hints "alt+enter for newline"
    When the user types "x"
    Then the rendered input shows no hint

  # use-case: UC-INPUT-NEWLINE-HINT
  # On a terminal that disambiguates modified keys, the hint advertises Shift+Enter.
  Scenario: The hint upgrades to Shift+Enter when the terminal supports it
    Given a focused input panel sized 40 wide with max 8 rows
    And the terminal supports key disambiguation
    Then the rendered input hints "shift+enter for newline"

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

  # use-case: UC-INPUT-HISTORY  (PD-02-AF-013)
  Scenario: Up seeds the previous submitted prompt
    Given a focused input panel
    And the input has submitted prompt "first"
    And the input has submitted prompt "second"
    When the user presses up
    Then the input value is "second"
    When the user presses up
    Then the input value is "first"

  # use-case: UC-INPUT-HISTORY  (PD-02-AF-013 / AF-014)
  # variant: draft stashed on step back, restored on return
  Scenario: Navigating history preserves and restores the in-progress draft
    Given a focused input panel
    And the input has submitted prompt "first"
    And the user types "draft"
    When the user presses up
    Then the input value is "first"
    When the user presses down
    Then the input value is "draft"

  # use-case: UC-INPUT-HISTORY  (PD-02-AF-015)
  # variant: boundary at the oldest prompt
  Scenario: Up past the oldest prompt flashes and holds the value
    Given a focused input panel
    And the input has submitted prompt "first"
    When the user presses up
    Then the input value is "first"
    When the user presses up
    Then the input value is "first"
    And the input reports a history boundary

  # use-case: UC-INPUT-HISTORY  (PD-02-AF-015)
  # variant: boundary at the present draft
  Scenario: Down at the present line flashes and holds the value
    Given a focused input panel
    And the input has submitted prompt "first"
    When the user presses down
    Then the input value is ""
    And the input reports a history boundary

  # use-case: UC-INPUT-HISTORY  (PD-02-AF-013)
  # variant: an edited seed is appended to history on submit
  Scenario: Submitting an edited seed adds the new text to history
    Given a focused input panel
    And the input has submitted prompt "first"
    When the user presses up
    And the user types " revised"
    And the user presses enter
    And the user presses up
    Then the input value is "first revised"

  # use-case: UC-INPUT-HISTORY  (PD-02-AF-016)
  # Esc,Esc clears an active seed back to an empty prompt (chord owned by chat)
  Scenario: Esc,Esc clears a history seed
    Given a new chat surface sized 40 by 16
    When the user types "first" and submits
    And the "up" key is pressed
    And ESC is pressed
    And ESC is pressed
    Then the chat input value is ""

  # use-case: UC-INPUT-HISTORY  (PD-02-AF-015)
  # the boundary flash highlights the input border (chord owned by chat)
  Scenario: Hitting a history boundary flashes the input border
    Given a new chat surface sized 40 by 16
    When the user types "first" and submits
    And the "up" key is pressed
    And the "up" key is pressed
    Then the chat view shows the flash border color

  # use-case: UC-INPUT-CURSOR  (PD-02-AF-023)
  Scenario: Typing inserts at the cursor
    Given a focused input panel containing "ac" with the cursor at 1
    When the user types "b"
    Then the input value is "abc"
    And the cursor is at 2

  # use-case: UC-INPUT-CURSOR  (PD-02-AF-023)
  Scenario: Backspace deletes the rune before the cursor
    Given a focused input panel containing "abc" with the cursor at 2
    When the user presses backspace
    Then the input value is "ac"
    And the cursor is at 1

  # use-case: UC-INPUT-CURSOR  (PD-02-AF-017 / AF-018)
  Scenario: Left and Right move one rune and clamp at the edges
    Given a focused input panel containing "ab" with the cursor at 2
    When the user presses left
    Then the cursor is at 1
    When the user presses left
    Then the cursor is at 0
    When the user presses left
    Then the cursor is at 0
    When the user presses right
    Then the cursor is at 1

  # use-case: UC-INPUT-CURSOR  (PD-02-AF-019 / AF-020)
  Scenario: Ctrl-A and Ctrl-E jump to the buffer ends
    Given a focused input panel containing "hello world" with the cursor at 5
    When the user presses ctrl+a
    Then the cursor is at 0
    When the user presses ctrl+e
    Then the cursor is at 11

  # use-case: UC-INPUT-CURSOR  (PD-02-AF-021)
  Scenario: Alt-B jumps to the start of the prior word
    Given a focused input panel containing "foo bar baz" with the cursor at 11
    When the user presses alt+b
    Then the cursor is at 8
    When the user presses alt+b
    Then the cursor is at 4

  # use-case: UC-INPUT-CURSOR  (PD-02-AF-022)
  Scenario: Alt-F jumps to the start of the next word
    Given a focused input panel containing "foo bar baz" with the cursor at 0
    When the user presses alt+f
    Then the cursor is at 4
    When the user presses alt+f
    Then the cursor is at 8

  # use-case: UC-INPUT-CURSOR  (PD-02-AF-021 / AF-022)
  # Ctrl+arrow aliases survive zellij's Alt-f floating-pane binding
  Scenario: Ctrl-Right and Ctrl-Left move by word
    Given a focused input panel containing "foo bar" with the cursor at 0
    When the user presses ctrl+right
    Then the cursor is at 4
    When the user presses ctrl+left
    Then the cursor is at 0

  # use-case: UC-INPUT-CURSOR  (PD-02-AF-013)
  # seeding a prompt leaves the cursor at the end of the seeded text
  Scenario: Seeding a prompt leaves the cursor at the end
    Given a focused input panel
    And the input has submitted prompt "hello"
    When the user presses up
    Then the input value is "hello"
    And the cursor is at 5

  # use-case: UC-INPUT-CURSOR  (PD-02-AF-024)
  Scenario: The cursor is rendered only while focused
    Given a focused input panel containing "hi" with the cursor at 2
    Then the rendered input shows a cursor cell
    When the panel is blurred
    Then the rendered input shows no cursor cell

  # use-case: UC-INPUT-WRAP
  # long text word-wraps to the width and the panel grows vertically, no overflow
  Scenario: Long text word-wraps instead of running off the edge
    Given a focused input panel sized 20 wide with max 8 rows
    When the user types "alpha bravo charlie delta echo foxtrot"
    Then the input wants 3 rows
    And no input line is wider than 20

  # use-case: UC-INPUT-WRAP
  # variant: growth caps at the max and a scrollbar appears past it
  Scenario: The input caps its height and shows a scrollbar past the max
    Given a focused input panel sized 12 wide with max 3 rows
    When the user types "one two three four five six seven eight nine ten"
    Then the input wants 3 rows
    And no input line is wider than 12
    And the rendered input shows a scrollbar
