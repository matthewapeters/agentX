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
