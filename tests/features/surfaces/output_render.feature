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
    When an agent_delta event "Hel" is applied
    And an agent_delta event "lo" is applied
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

  # use-case: UC-OUTPUT-CLASSIFICATION  (classification is one line, no box)
  # Classification always holds a single line of metadata, so it renders flat —
  # emoji + title + text on one line — instead of a three-row box.
  Scenario: Classification renders as a single flat line
    Given an output panel sized 60 by 10
    When a classification event "greeting → respond_directly" is applied
    Then the output view contains "⚙ classification · greeting → respond_directly"
    And the output view does not contain "┌"

  # use-case: UC-OUTPUT-CHECKBOX  (context surface: enable/disable checkbox)
  # In toggle-state mode, toggleable elements carry an enabled checkbox left of the
  # emoji: [x] enabled (in context), [ ] disabled (withheld) — orthogonal to selection.
  Scenario: Enabled and disabled elements show a checkbox
    Given an output panel sized 40 by 10
    And the output panel shows enable/disable state
    When an agent_response event "kept" is applied
    Then the output view contains "[x]"
    When a disabled agent_response event "withheld" is applied
    Then the output view contains "[ ]"
    And the output view contains "withheld"

  # use-case: UC-OUTPUT-PIN-ELIGIBLE  (context surface: Pin-to-WM affordance, PD-CTX-AF-012)
  # Only a flat (untagged) tool_result is eligible for Pin — a tool_call proposal
  # (no useful WM content) and every other element kind are not.
  Scenario: Only a flat tool_result is eligible to pin
    Given an output panel sized 40 by 10
    When a tool_result event for "list_dir" is applied
    Then the selection is pin-eligible
    When a tool_call event for "list_dir" is applied
    Then the selection is not pin-eligible
    When an agent_response event "hi" is applied
    Then the selection is not pin-eligible

  # use-case: UC-OUTPUT-SUMMARY  (context surface: navigable summary)
  Scenario: Summary mode collapses elements by default
    Given an output panel sized 20 by 10
    And the output panel is a navigable summary
    When an agent_response event "the quick brown fox jumps over the lazy dog" is applied
    Then the output view contains "…"

  # Logo banner note: the banner used to render here, as the output panel's
  # first scrollable line (UC-OUTPUT-BANNER). It has moved to a
  # screen-pinned region owned by the chat surface — the output panel no
  # longer has a banner concept at all. See
  # tests/features/surfaces/chat_layout.feature and
  # docs/ux/06_OUTPUT_WIDGET.md ("Logo banner").

  # use-case: UC-OUTPUT-LAUNCH  (docs/ux/06_OUTPUT_WIDGET.md "Launch-info widget")
  Scenario: Launch info renders collapsed as the first widget
    Given an output panel sized 60 by 12
    And the launch info is set for 2 surface kinds
    Then the output view contains "Attach surfaces"
    And the output view does not contain "kind-1"

  # use-case: UC-OUTPUT-LAUNCH
  # variant: expand lists surfaces by name, never the command or token
  Scenario: Expanding launch info lists each surface by name
    Given an output panel sized 100 by 16
    And the launch info is set for 2 surface kinds
    When launch widget 0 is expanded
    Then the output view contains "kind-1"
    And the output view contains "kind-2"
    And the output view does not contain "token"
    And the output view contains "agentx surface launch <name>"
    And the output view contains "--session calm-otter"

  # use-case: UC-OUTPUT-LAUNCH
  # variant: every surface is disconnected (red) until one attaches
  Scenario: Disconnected surfaces show a red status by default
    Given an output panel sized 100 by 16
    And the launch info is set for 2 surface kinds
    When launch widget 0 is expanded
    Then the output view contains "🔴 kind-1"
    And the output view contains "🔴 kind-2"

  # use-case: UC-OUTPUT-LAUNCH
  # variant: an attached surface turns green; the others stay red
  Scenario: A connected surface shows a green status
    Given an output panel sized 100 by 16
    And the launch info is set for 2 surface kinds
    And launch widget 0 is expanded
    When the surface "kind-1" reports connected
    Then the output view contains "🟢 kind-1"
    And the output view contains "🔴 kind-2"

  # use-case: UC-OUTPUT-LAUNCH
  # variant: digit copies the command and confirms by name
  Scenario: Pressing a digit copies a surface's attach command
    Given an output panel sized 100 by 16
    And the launch info is set for 2 surface kinds
    And launch widget 0 is expanded
    And the launch info widget is selected
    When launch command 1 is copied
    Then the output view contains "copied kind-1"
    And the output view does not contain "token"

  # use-case: UC-OUTPUT-LAUNCH
  # variant: copy is rejected unless the launch widget is selected
  Scenario: Copy requires the launch widget to be selected
    Given an output panel sized 100 by 16
    And the launch info is set for 2 surface kinds
    Then copying launch command 1 is rejected

  # use-case: UC-OUTPUT-LAUNCH
  # variant: stays above applied widgets
  Scenario: Launch info precedes applied widgets
    Given an output panel sized 60 by 12
    And the launch info is set for 1 surface kinds
    When a user_prompt event "hello there" is applied
    Then the launch info precedes "hello there" in the output

  # use-case: UC-OUTPUT-SCROLLBAR
  # a transcript-level scrollbar in the right gutter shows position within the whole
  Scenario: The transcript shows a scrollbar when content overflows the viewport
    Given an output panel sized 20 by 3
    When 10 numbered user events are applied
    Then the output has a transcript scrollbar

  # use-case: UC-OUTPUT-SCROLLBAR
  # variant: no scrollbar when everything fits
  Scenario: No transcript scrollbar when content fits the viewport
    Given an output panel sized 40 by 40
    When a user_prompt event "hello there" is applied
    Then the output has no transcript scrollbar

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
