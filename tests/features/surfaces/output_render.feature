# Source contracts:
#   - docs/ux/03_PANEL_DETAILS.md PD-01 (message entry types, re-authored for TUI)
#   - docs/architecture/runtime_contracts/event-envelope.schema.json
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-B2)
#
# Behavior: the output panel renders conversation event types, streams assistant
# text into one entry, collapses thinking/tool bodies by default, and scrolls.

@functional @arch:output-panel @ux:PD-01
Feature: Output panel event rendering
  As a user of the chat surface
  I want streamed responses and tool activity rendered clearly
  So that I can follow the conversation

  # use-case: UC-OUTPUT-RENDER
  Scenario: Renders user and assistant entries
    Given an output panel sized 40 by 10
    When a user_prompt event "hello there" is applied
    And an agent_response event "hi back" is applied
    Then the output view contains "hello there"
    And the output view contains "hi back"

  # use-case: UC-OUTPUT-STREAM
  Scenario: Assistant response streams into a single entry
    Given an output panel sized 40 by 10
    When an agent_response event "Hel" is applied
    And an agent_response event "lo" is applied
    Then the output view contains "Hello"
    And the output has 1 assistant entry

  # use-case: UC-OUTPUT-COLLAPSE
  Scenario: Thinking block is collapsed by default and expands
    Given an output panel sized 40 by 10
    When a thinking event "secret reasoning" is applied
    Then the output view does not contain "secret reasoning"
    When entry 0 is toggled
    Then the output view contains "secret reasoning"

  # use-case: UC-OUTPUT-TOOL
  Scenario: Tool call renders with its name
    Given an output panel sized 40 by 10
    When a tool_call event for "read_file" is applied
    Then the output view contains "read_file"

  # use-case: UC-OUTPUT-WRAP
  Scenario: Long responses wrap to the panel width instead of truncating
    Given an output panel sized 20 by 10
    When an agent_response event "the quick brown fox jumps over the lazy dog" is applied
    Then no output line is wider than 20
    And the output view contains "lazy dog"

  # use-case: UC-OUTPUT-BANNER  (docs/ux/06_OUTPUT_WIDGET.md "Logo banner")
  Scenario: The logo banner renders as the first element before any event
    Given an output panel sized 40 by 10
    And the logo banner "AGENTX-LOGO" is set
    Then the output view contains "AGENTX-LOGO"

  # use-case: UC-OUTPUT-BANNER
  # the banner stays above the first widget
  Scenario: The logo banner precedes applied widgets
    Given an output panel sized 40 by 10
    And the logo banner "AGENTX-LOGO" is set
    When a user_prompt event "hello there" is applied
    Then the logo banner precedes "hello there" in the output

  # use-case: UC-OUTPUT-BANNER
  # variant: clipped to width, never wider than the panel
  Scenario: A wide banner line is clipped to the panel width
    Given an output panel sized 12 by 10
    And the logo banner "0123456789ABCDEFGHIJ" is set
    Then no output line is wider than 12

  # use-case: UC-OUTPUT-LAUNCH  (docs/ux/06_OUTPUT_WIDGET.md "Launch-info widget")
  Scenario: Launch info renders collapsed as the first widget
    Given an output panel sized 60 by 12
    And the launch info is set for 2 surface kinds
    Then the output view contains "Attach surfaces"
    And the output view does not contain "surface launch kind-1"

  # use-case: UC-OUTPUT-LAUNCH
  # variant: expand reveals a command per kind
  Scenario: Expanding launch info shows a command per kind
    Given an output panel sized 100 by 16
    And the launch info is set for 2 surface kinds
    When launch widget 0 is expanded
    Then the output view contains "surface launch kind-1"
    And the output view contains "surface launch kind-2"

  # use-case: UC-OUTPUT-LAUNCH
  # variant: stays above applied widgets
  Scenario: Launch info precedes applied widgets
    Given an output panel sized 60 by 12
    And the launch info is set for 1 surface kinds
    When a user_prompt event "hello there" is applied
    Then the launch info precedes "hello there" in the output

  # use-case: UC-OUTPUT-SCROLL
  Scenario: Scrolling to the top reveals the earliest widget
    Given an output panel sized 20 by 3
    When 10 numbered user events are applied
    Then the output view does not contain "msg-01"
    When the panel scrolls up by 100
    Then the output view contains "msg-01"

  # use-case: UC-OUTPUT-SCROLL
  # variant: page
  Scenario: Paging up reveals the previous page
    Given an output panel sized 20 by 9
    When 10 numbered user events are applied
    Then the output view does not contain "msg-07"
    When the panel pages up
    Then the output view contains "msg-07"
