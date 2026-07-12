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
  # The pinned logo banner (docs/ux/06_OUTPUT_WIDGET.md "Logo banner") is a fifth,
  # borderless fixed region above the output frame — 8 rows full-size (the art
  # rows only; blank padding is trimmed, see internal/surfaces/banner).
  Scenario: Surface fills the terminal with banner, output, status, hint, and input
    Given a new chat surface
    When the terminal size is 80 by 24
    Then the rendered view has 24 rows
    And the banner region is 8 rows
    And the output region is 9 rows
    And the input region is 1 rows

  # use-case: UC-CHAT-LAYOUT
  # the input grows with content (up to its max) and the output shrinks to match;
  # the banner's own region is unaffected by input growth
  Scenario: The input grows as the prompt spans more lines
    Given a new chat surface
    When the terminal size is 80 by 24
    And the user types the prompt "one"
    And the user inserts a prompt newline
    And the user types the prompt "two"
    Then the input region is 2 rows
    And the output region is 8 rows
    And the banner region is 8 rows

  # use-case: UC-CHAT-BANNER  (docs/ux/06_OUTPUT_WIDGET.md "Logo banner")
  # the banner is pinned outside the output viewport's scrollable content: it
  # never appears inside what Output().View() renders
  Scenario: The pinned banner is not part of the output viewport's content
    Given a new chat surface
    When the terminal size is 80 by 24
    And 3 numbered user events are applied to the chat surface
    Then the chat output does not contain "AgentX"

  # use-case: UC-CHAT-BANNER
  # content-based, sticky collapse: once applied content exceeds the budget
  # available under the full-size banner, it collapses to the one-row
  # "AgentX - <activity>" label and never re-expands, even once the terminal
  # grows much larger. Freshly collapsed with no work yet started, the label
  # is the idle default.
  Scenario: The banner collapses once content exceeds one screenful and stays collapsed
    Given a new chat surface
    When the terminal size is 80 by 15
    And 5 numbered user events are applied to the chat surface
    Then the banner region is 1 rows
    And the rendered view contains "AgentX - Your Local Agent"
    When the terminal size is 80 by 60
    Then the banner region is 1 rows

  # use-case: UC-CHAT-BANNER
  # rainbow-wave animation starts with the run and reverts once it ends
  Scenario: The banner animates while the run is working and reverts when idle
    Given a new chat surface
    When the terminal size is 80 by 24
    And the processing state becomes working in phase "respond"
    Then a banner tick changes the rendered banner
    When the processing state becomes idle
    Then a banner tick does not change the rendered banner

  # use-case: UC-CHAT-BANNER
  # the collapsed row's label tracks what the agent is currently doing
  # (chat.go's bannerLabel maps state.RunState/state.Phase to this text)
  Scenario Outline: The collapsed banner's label reflects the current activity
    Given a new chat surface
    When the terminal size is 80 by 15
    And 5 numbered user events are applied to the chat surface
    And the processing state becomes working in phase "<phase>"
    Then the rendered view contains "AgentX - <label>"

    Examples:
      | phase     | label      |
      | thinking  | Thinking   |
      | classify  | Thinking   |
      | tool      | Working    |
      | respond   | Responding |
      | planning  | Planning   |

  # use-case: UC-CHAT-BANNER
  # awaiting a decision (tool approval, verb continuation) takes priority
  # over phase — the user needs to know a decision is pending, not what phase
  # the run paused in
  Scenario: The collapsed banner shows "Needs Input" while awaiting a decision
    Given a new chat surface
    When the terminal size is 80 by 15
    And 5 numbered user events are applied to the chat surface
    And the processing state becomes awaiting input
    Then the rendered view contains "AgentX - Needs Input"

  # use-case: UC-CHAT-FOCUS
  Scenario: Input is focused on start
    Given a new chat surface
    Then the input panel is focused

  # use-case: UC-CHAT-QUIT
  Scenario: Ctrl+C requests quit
    Given a new chat surface
    When the user presses ctrl+c
    Then the surface requests quit
