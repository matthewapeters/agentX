# Source contracts:
#   - docs/ux/03_PANEL_DETAILS.md PD-LOGS
#   - docs/build-plan/06_system_surfaces_backlog.md (Phase G, SS-9)
#
# Behavior: the logs surface renders every persisted session event as one
# wrapped, timestamped line, auto-following new events until the user
# scrolls, with vi-style /pattern and ?pattern search (highlighted matches,
# n/N cycling) and vim-style gg/G navigation. Strictly read-only — quit is
# owned by the shared surface host (SS-8), never this surface.

@functional @ux:PD-LOGS
Feature: Logs surface
  As a user reviewing backend/session activity
  I want a searchable, live-tailing view of the full session event log
  So that I can review what the backend actually did without reading raw JSON files

  # use-case: UC-LOGS-RENDER (PD-LOGS-AF-001 area — rendering, not tab placement itself)
  Scenario: Applied events render as timestamped lines
    Given a logs surface sized 80 by 10
    When a "tool_call" event with tool "read_file" and payload "reading config" is applied
    Then the logs view shows "tool_call"
    And the logs view shows "[read_file]"
    And the logs view shows "reading config"

  # use-case: UC-LOGS-FOLLOW  (PD-LOGS-AF-002)
  Scenario: New events auto-follow while the viewport sits at the bottom
    Given a logs surface sized 80 by 3
    And 5 "user_prompt" events are applied
    Then the logs view shows "prompt-5"
    When 1 more "user_prompt" events are applied
    Then the logs view shows "prompt-6"

  # use-case: UC-LOGS-FOLLOW  (PD-LOGS-AF-002)
  Scenario: Auto-follow pauses once the user scrolls up
    Given a logs surface sized 80 by 3
    And 5 "user_prompt" events are applied
    When the logs surface receives key "k"
    And 1 more "user_prompt" events are applied
    Then the logs view omits "prompt-6"

  # use-case: UC-LOGS-WRAP  (PD-LOGS-AF-006)
  Scenario: A long line wraps instead of running off the pane
    Given a logs surface sized 20 by 10
    When a "tool_result" event with tool "" and payload "a very long payload that must wrap across more than one line" is applied
    Then the logs view has more than 1 line

  # use-case: UC-LOGS-SEARCH  (PD-LOGS-AF-003 / PD-LOGS-AF-004 / PD-LOGS-AF-005)
  Scenario: Forward search highlights every match and reports a count
    Given a logs surface sized 80 by 10
    And 3 "tool_call" events are applied
    When the logs surface receives key "/"
    And the logs surface types "tool_call"
    And the logs surface receives key "enter"
    Then the logs view shows "1/3 matches"

  # use-case: UC-LOGS-SEARCH  (PD-LOGS-AF-004 — regex, not just literal substring)
  Scenario: Search patterns are regular expressions
    Given a logs surface sized 80 by 10
    And 1 "tool_call" events are applied
    And 1 "tool_result" events are applied
    When the logs surface receives key "/"
    And the logs surface types "tool_.*"
    And the logs surface receives key "enter"
    Then the logs view shows "1/2 matches"

  # use-case: UC-LOGS-SEARCH  (PD-LOGS-AF-008 / SS-8 integration)
  Scenario: A "q" typed mid-search is a literal character, not quit
    Given a logs surface sized 80 by 10
    When the logs surface receives key "/"
    And the logs surface types "q"
    Then the logs surface is capturing keys
    And the logs view shows "/q"

  # use-case: UC-LOGS-JUMP  (PD-LOGS-AF-007)
  Scenario: gg jumps to the top and G jumps back to the bottom
    Given a logs surface sized 80 by 3
    And 10 "user_prompt" events are applied
    When the logs surface receives key "g"
    And the logs surface receives key "g"
    Then the logs view shows "prompt-1"
    And the logs view omits "prompt-10"
    When the logs surface receives key "G"
    Then the logs view shows "prompt-10"
