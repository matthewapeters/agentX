@functional
Feature: Output applet interactive navigation
  As an AgentX user
  I want the output pane to show the latest turn expanded and let me navigate history
  So that I can review conversation turns without losing working space

  # -------------------------------------------------------------------------
  # OUT-AF-001 / OUT-AF-007: latest-first default with compact older turns
  # -------------------------------------------------------------------------

  Scenario: Fresh output starts with newest turn expanded and older turns compacted
    Given the output widget is initialized with no prior view state
    And core delivers turn 1 with role "assistant" and content "Hello, I am AgentX."
    And core delivers turn 2 with role "assistant" and content "Here is your answer."
    When the output widget renders the current view
    Then turn 2 should be expanded showing full content
    And turn 1 should be compacted to a single summary line
    And the focus index should be 2

  Scenario: New turn arrival auto-expands the newest turn and compacts the previously focused turn
    Given the output widget is initialized with no prior view state
    And core delivers turn 1 with role "assistant" and content "First response."
    And the output widget renders the current view
    When core delivers turn 2 with role "assistant" and content "Second response."
    And the output widget renders the current view
    Then turn 2 should be expanded showing full content
    And turn 1 should be compacted to a single summary line
    And the focus index should be 2

  # -------------------------------------------------------------------------
  # OUT-AF-002: turn navigation
  # -------------------------------------------------------------------------

  Scenario: Pressing j moves focus to the next newer turn
    Given the output widget is initialized with no prior view state
    And core delivers turn 1 with role "assistant" and content "First response."
    And core delivers turn 2 with role "assistant" and content "Second response."
    And the output widget renders the current view
    And the user navigates to turn 1 using the k key
    When the user presses the j key
    Then the focus index should be 2

  Scenario: Pressing k moves focus to the previous older turn
    Given the output widget is initialized with no prior view state
    And core delivers turn 1 with role "assistant" and content "First response."
    And core delivers turn 2 with role "assistant" and content "Second response."
    And the output widget renders the current view
    When the user presses the k key
    Then the focus index should be 1

  Scenario: Turn navigation does not move focus past the oldest turn
    Given the output widget is initialized with no prior view state
    And core delivers turn 1 with role "assistant" and content "Only turn."
    And the output widget renders the current view
    And the focus index should be 1
    When the user presses the k key
    Then the focus index should be 1

  Scenario: Turn navigation does not move focus past the newest turn
    Given the output widget is initialized with no prior view state
    And core delivers turn 1 with role "assistant" and content "Only turn."
    And the output widget renders the current view
    And the focus index should be 1
    When the user presses the j key
    Then the focus index should be 1

  Scenario: Home key jumps focus to the oldest turn
    Given the output widget is initialized with no prior view state
    And core delivers turn 1 with role "assistant" and content "First."
    And core delivers turn 2 with role "assistant" and content "Second."
    And core delivers turn 3 with role "assistant" and content "Third."
    And the output widget renders the current view
    When the user presses the Home key
    Then the focus index should be 1

  Scenario: End key jumps focus to the newest turn
    Given the output widget is initialized with no prior view state
    And core delivers turn 1 with role "assistant" and content "First."
    And core delivers turn 2 with role "assistant" and content "Second."
    And core delivers turn 3 with role "assistant" and content "Third."
    And the output widget renders the current view
    And the user navigates to turn 1 using the Home key
    When the user presses the End key
    Then the focus index should be 3

  # -------------------------------------------------------------------------
  # OUT-AF-003: manual collapse/expand restores content
  # -------------------------------------------------------------------------

  Scenario: Enter key expands a compacted turn and its full content is visible
    Given the output widget is initialized with no prior view state
    And core delivers turn 1 with role "assistant" and content "Detailed answer here."
    And core delivers turn 2 with role "assistant" and content "Latest turn."
    And the output widget renders the current view
    And the user navigates to turn 1 using the k key
    And turn 1 should be compacted to a single summary line
    When the user presses the Enter key
    Then turn 1 should be expanded showing full content "Detailed answer here."

  Scenario: Space key collapses an expanded turn to a single summary line
    Given the output widget is initialized with no prior view state
    And core delivers turn 1 with role "assistant" and content "Some content."
    And the output widget renders the current view
    And turn 1 should be expanded showing full content
    When the user presses the Space key
    Then turn 1 should be compacted to a single summary line

  Scenario: Collapsing and re-expanding a turn restores the original content
    Given the output widget is initialized with no prior view state
    And core delivers turn 1 with role "assistant" and content "Full content to restore."
    And the output widget renders the current view
    When the user presses the Space key
    Then turn 1 should be compacted to a single summary line
    When the user presses the Enter key
    Then turn 1 should be expanded showing full content "Full content to restore."

  # -------------------------------------------------------------------------
  # OUT-AF-007: compact mode preserves working space
  # -------------------------------------------------------------------------

  Scenario: Compacted turns render as exactly one line each
    Given the output widget is initialized with no prior view state
    And core delivers turn 1 with role "assistant" and content "First response with a lot of text that would normally span multiple lines."
    And core delivers turn 2 with role "assistant" and content "Second response."
    And the output widget renders the current view
    When I count the rendered lines for turn 1
    Then the rendered line count for turn 1 should be 1

  Scenario: Compact summary line includes role prefix and truncated content
    Given the output widget is initialized with no prior view state
    And core delivers turn 1 with role "assistant" and content "Detailed explanation spanning many lines."
    And core delivers turn 2 with role "assistant" and content "Latest."
    And the output widget renders the current view
    When I inspect the compact summary line for turn 1
    Then the compact summary line should start with "[assistant]"
    And the compact summary line should contain a truncated version of the content

  # -------------------------------------------------------------------------
  # OUT-AF-006: help overlay exposes the keymap
  # -------------------------------------------------------------------------

  Scenario: Pressing ? toggles the inline help overlay showing the keymap
    Given the output widget is initialized with no prior view state
    And core delivers turn 1 with role "assistant" and content "Some output."
    And the output widget renders the current view
    When the user presses the ? key
    Then the help overlay should be visible
    And the help overlay should contain key binding "j" with description for next turn
    And the help overlay should contain key binding "k" with description for previous turn
    And the help overlay should contain key binding "Enter" with description for expand
    And the help overlay should contain key binding "Space" with description for collapse
    And the help overlay should contain key binding "?" with description for help

  Scenario: Pressing any key while the help overlay is open dismisses it
    Given the output widget is initialized with no prior view state
    And core delivers turn 1 with role "assistant" and content "Some output."
    And the output widget renders the current view
    And the user presses the ? key
    And the help overlay should be visible
    When the user presses the ? key
    Then the help overlay should not be visible
