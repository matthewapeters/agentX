# Source contracts:
#   - docs/ux/03_PANEL_DETAILS.md PD-WM (working-memory editor)
#   - docs/implementation/02_surface_orchestration_http.md (Working-Memory CRUD SS-6)
#
# Behavior: the working-memory editor lists facts (enabled ● / disabled ○), navigates
# with the selection cursor, and drives add/edit/delete/toggle. Mutations issue a
# transport command; the inline editor captures key=value for add and a value for edit.

@functional @ux:PD-WM @arch:surface-client
Feature: Working-memory editor surface
  As a user
  I want to list and curate working-memory facts
  So that I control what the agent sees

  # use-case: PD-WM-AF-001  (TC-M2-wm-005)
  Scenario: Lists facts with enabled and disabled markers
    Given a working memory surface
    And the surface loads:
      | key   | value | enabled |
      | color | blue  | true    |
      | size  | big   | false   |
    Then the working memory view shows "color = blue"
    And the working memory view shows "size = big"
    And the working memory view shows "● color = blue"
    And the working memory view shows "○ size = big"

  # use-case: PD-WM-AF-002  (TC-M2-wm-006)
  Scenario: Navigates the selection cursor
    Given a working memory surface
    And the surface loads:
      | key   | value | enabled |
      | color | blue  | true    |
      | size  | big   | false   |
    Then the working memory view highlights "color"
    When the surface receives key "down"
    Then the working memory view highlights "size"

  # use-case: PD-WM-AF-003  (TC-M2-wm-007)
  Scenario: Space toggles the selected fact (issues a command)
    Given a working memory surface
    And the surface loads:
      | key   | value | enabled |
      | color | blue  | true    |
    When the surface receives key "space"
    Then the surface issued a command

  # use-case: PD-WM-AF-004  (TC-M2-wm-008)
  Scenario: Delete issues a command for the selected fact
    Given a working memory surface
    And the surface loads:
      | key   | value | enabled |
      | color | blue  | true    |
    When the surface receives key "d"
    Then the surface issued a command

  # use-case: PD-WM-AF-005  (TC-M2-wm-009)
  Scenario: Add captures key=value and saves on enter
    Given a working memory surface
    When the surface receives key "a"
    And the surface types "fruit=apple"
    Then the working memory view shows "fruit=apple"
    When the surface receives key "enter"
    Then the surface issued a command

  # use-case: PD-WM-AF-006  (TC-M2-wm-010)
  Scenario: Escape cancels the inline editor without saving
    Given a working memory surface
    And the surface loads:
      | key   | value | enabled |
      | color | blue  | true    |
    When the surface receives key "a"
    And the surface types "x=y"
    And the surface receives key "esc"
    Then the working memory view does not show "x=y"
    And the working memory view shows "↑/↓ move"
