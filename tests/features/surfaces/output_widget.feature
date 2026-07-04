# Source contracts:
#   - docs/ux/06_OUTPUT_WIDGET.md (collapsible output widget)
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-D1)
#
# Behavior: each output entry is a bordered, collapsible widget. The selection
# cursor highlights one widget; collapse/expand and inner scrolling target it.
# Bodies over the cap scroll in place with a proportional scrollbar.

@functional @arch:output-panel @ux:PD-01
Feature: Collapsible output widgets
  As a user of the chat surface
  I want bordered, collapsible, scrollable output widgets
  So that long content stays scannable and navigable

  # use-case: UC-WIDGET-BORDER
  Scenario: Entries render as bordered boxes with the selection highlighted
    Given an output panel sized 30 by 12
    When a user_prompt event "first" is applied
    And a user_prompt event "second" is applied
    Then the output view contains an unselected widget border
    And the output view contains a selected widget border

  # use-case: UC-WIDGET-COLLAPSE
  Scenario: The selected widget toggles collapse
    Given an output panel sized 30 by 12
    When a thinking event "secret reasoning" is applied
    Then the output view does not contain "secret reasoning"
    When the selected widget is toggled
    Then the output view contains "secret reasoning"

  # use-case: UC-WIDGET-COLLAPSE
  # variant: a narrative box (assistant) collapses to a first-line preview
  Scenario: The assistant widget collapses to a first-line preview
    Given an output panel sized 30 by 12
    When an agent_response event "alpha bravo charlie delta echo foxtrot golf hotel" is applied
    Then the output view contains "hotel"
    When the selected widget is toggled
    Then the output view contains "alpha"
    And the output view does not contain "hotel"

  # use-case: UC-WIDGET-COLLAPSE
  # variant: a narrative box (user) collapses to a first-line preview
  Scenario: The user widget collapses to a first-line preview
    Given an output panel sized 30 by 12
    When a user_prompt event "uno dos tres cuatro cinco seis siete ocho nueve diez" is applied
    Then the output view contains "diez"
    When the selected widget is toggled
    Then the output view contains "uno"
    And the output view does not contain "diez"

  # use-case: UC-WIDGET-SCROLL
  Scenario: A long body is capped, shows a scrollbar, and scrolls in place
    Given an output panel sized 30 by 40
    And the max widget lines is 5
    When a thinking event with 20 body lines is applied
    And the selected widget is toggled
    Then the output view contains "line-00"
    And the output view does not contain "line-19"
    And the output view contains a scrollbar
    When the selected widget scrolls down by 100
    Then the output view contains "line-19"
    And the output view does not contain "line-00"

  # use-case: UC-WIDGET-STREAM-FOLLOW (nits.md #2)
  # A streaming assistant body auto-follows the incoming tail so the user reads
  # the newest text without a manual scroll; the in-place scroll cap still holds.
  Scenario: A streaming assistant body follows the incoming tail
    Given an output panel sized 30 by 40
    And the max widget lines is 3
    When 20 agent_delta lines are streamed
    Then the output view contains "line-19"
    And the output view does not contain "line-00"
    And the output view contains a scrollbar

  # use-case: UC-WIDGET-STREAM-FOLLOW (nits.md #2)
  # variant: scrolling up while streaming detaches from the tail so the reader can
  # hold their place on earlier text.
  Scenario: Scrolling up detaches a streaming body from the tail
    Given an output panel sized 30 by 40
    And the max widget lines is 3
    When 20 agent_delta lines are streamed
    And the selected widget scrolls up by 100
    Then the output view contains "line-00"
    And the output view does not contain "line-19"

  # use-case: UC-WIDGET-MARKDOWN (nits.md #6, tier 1)
  # Assistant bodies get lightweight markdown emphasis rendered as ANSI styling so
  # LLM markdown reads richly in the terminal; the source markers are consumed.
  Scenario: An assistant response renders inline bold
    Given an output panel sized 40 by 12
    When an agent_response event "I am **AgentX**, your agent" is applied
    Then the output view shows bolded "AgentX"
    And the output view does not contain "**"

  # use-case: UC-WIDGET-MARKDOWN (nits.md #6, tier 1)
  Scenario: An assistant response renders inline code
    Given an output panel sized 40 by 12
    When an agent_response event "run `go build` now" is applied
    Then the output view shows inline code "go build"
    And the output view does not contain "`"

  # use-case: UC-WIDGET-MARKDOWN (nits.md #6, tier 1)
  Scenario: An assistant response renders ATX headers by level
    Given an output panel sized 40 by 12
    When an agent_response with body:
      """
      # One
      ## Two
      ### Three
      body
      """
    Then the output view shows an h1 header "One"
    And the output view shows an h2 header "Two"
    And the output view shows an h3 header "Three"
    And the output view does not contain "# One"

  # use-case: UC-WIDGET-MARKDOWN (nits.md #6, tier 1)
  # variant: only assistant bodies are styled; other kinds keep their text literal
  # so pasted prose / tool output is never mangled.
  Scenario: A user prompt is not markdown-styled
    Given an output panel sized 40 by 12
    When a user_prompt event "literal **stars** stay" is applied
    Then the output view contains "**stars**"

  # use-case: UC-WIDGET-MARKDOWN (nits.md #6, tier 2)
  # Unordered list markers (-, *, +) fold to one bold bullet glyph; the item text
  # still gets inline emphasis and the source marker is consumed.
  Scenario: An assistant response renders unordered lists
    Given an output panel sized 40 by 12
    When an agent_response with body:
      """
      - first item
      * second item
      + third item
      """
    Then the output view shows a bullet "first item"
    And the output view shows a bullet "second item"
    And the output view shows a bullet "third item"

  # use-case: UC-WIDGET-MARKDOWN (nits.md #6, tier 2)
  # Ordered list markers keep their number but the marker is emboldened.
  Scenario: An assistant response renders ordered lists
    Given an output panel sized 40 by 12
    When an agent_response with body:
      """
      1. alpha
      2. bravo
      """
    Then the output view shows an ordered marker "1."
    And the output view shows an ordered marker "2."
    And the output view contains "alpha"
    And the output view contains "bravo"

  # use-case: UC-WIDGET-MARKDOWN (nits.md #6, tier 2)
  # A blockquote line is rendered dim with a gutter marker; the "> " is consumed.
  Scenario: An assistant response renders blockquotes
    Given an output panel sized 40 by 12
    When an agent_response event "> quoted wisdom here" is applied
    Then the output view shows a blockquote "quoted wisdom here"

  # use-case: UC-WIDGET-MARKDOWN (nits.md #6, tier 3 — ADR 0007)
  # With the glamour renderer, a finalized answer is rendered as a full document:
  # GFM tables draw aligned box-drawing columns and the pipe markup is consumed.
  # Assertions are structural (not exact SGR) so they survive glamour theme/version.
  Scenario: The glamour renderer renders a GFM table
    Given an output panel sized 52 by 30
    And the markdown renderer is "glamour"
    When an agent_response with body:
      """
      | Name | Role |
      |------|------|
      | Ada | Engineer |
      """
    Then the output view contains "Name"
    And the output view contains "Ada"
    And the output view contains "│"
    And the output view does not contain "|------|"

  # use-case: UC-WIDGET-MARKDOWN (nits.md #6, tier 3 — ADR 0007)
  # A fenced code block is syntax-highlighted; the ``` fence markers are consumed.
  Scenario: The glamour renderer highlights a fenced code block
    Given an output panel sized 52 by 30
    And the markdown renderer is "glamour"
    When an agent_response with body:
      """
      ```go
      func main() {}
      ```
      """
    Then the output view contains "func"
    And the output view contains "main"
    And the output view does not contain "```"

  # use-case: UC-WIDGET-MARKDOWN (nits.md #6, tier 3 — ADR 0007)
  # The width contract: a table taller than the cap scrolls vertically, and NO line
  # ever exceeds the panel width — the output panel never forces a horizontal scroll.
  Scenario: A wide glamour table scrolls vertically without a horizontal scroll
    Given an output panel sized 40 by 12
    And the max widget lines is 4
    And the markdown renderer is "glamour"
    When an agent_response with body:
      """
      | Name | Role | Score |
      |------|------|-------|
      | Ada | Engineer | 99 |
      | Grace | Admiral | 100 |
      | Alan | Theorist | 88 |
      """
    Then the output view contains a scrollbar
    And no output line is wider than 40

  # use-case: UC-WIDGET-MARKDOWN (nits.md #6, tier 3 — ADR 0007)
  # variant: the scanner renderer does not render tables — the pipe markup stays
  # literal, so the glamour path is what unlocks tables. (Glamour is the product
  # default; this pins the opt-out behavior.)
  Scenario: The scanner renderer leaves table markup literal
    Given an output panel sized 52 by 12
    And the markdown renderer is "scanner"
    When an agent_response with body:
      """
      | A | B |
      | 1 | 2 |
      """
    Then the output view contains "| A | B |"
