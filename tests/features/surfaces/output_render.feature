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

  # use-case: UC-OUTPUT-SCROLL
  Scenario: Scrolling reveals earlier lines
    Given an output panel sized 20 by 3
    When 10 numbered user events are applied
    Then the output view does not contain "msg-01"
    When the panel scrolls up by 8
    Then the output view contains "msg-01"
