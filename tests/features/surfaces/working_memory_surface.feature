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
  Scenario: Navigates the selection cursor with PgUp/PgDn
    Given a working memory surface
    And the surface loads:
      | key   | value | enabled |
      | color | blue  | true    |
      | size  | big   | false   |
    Then the working memory view highlights "color"
    When the surface receives key "pgdown"
    Then the working memory view highlights "size"
    When the surface receives key "pgup"
    Then the working memory view highlights "color"

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
    And the working memory view shows "pgup/pgdn move"

  # use-case: PD-WM-AF-011
  Scenario: A multi-line value collapses by default
    Given a working memory surface
    And the surface loads a fact "notes" with 5 lines of value
    Then the working memory view shows "notes = line 1 … (+4 lines)"
    And the working memory view does not show "line 5"

  # use-case: PD-WM-AF-011
  Scenario: Enter expands and re-collapses the selected multi-line fact
    Given a working memory surface
    And the surface loads a fact "notes" with 5 lines of value
    When the surface receives key "enter"
    Then the working memory view shows "line 5"
    And the working memory view does not show "… (+4 lines)"
    When the surface receives key "enter"
    Then the working memory view shows "notes = line 1 … (+4 lines)"
    And the working memory view does not show "line 5"

  # use-case: PD-WM-AF-011
  Scenario: Enter on a single-line fact is a no-op
    Given a working memory surface
    And the surface loads:
      | key   | value | enabled |
      | color | blue  | true    |
    When the surface receives key "enter"
    Then the working memory view shows "color = blue"

  # use-case: PD-WM-AF-010
  Scenario: An expanded fact over the per-fact cap shows a scrollbar and scrolls in place
    Given a working memory surface
    And the surface loads a fact "notes" with 20 lines of value
    When the surface receives key "enter"
    Then the working memory view shows "line 1"
    And the working memory view shows "line 12"
    And the working memory view does not show "line 13"
    And the working memory view has a scrollbar
    When the surface receives key "down"
    Then the working memory view shows "line 13"
    And the working memory view does not show "line 1 "

  # use-case: PD-WM-AF-013
  Scenario: The fact list shows a scrollbar when facts overflow the panel height
    Given a working memory surface sized 40 by 10
    And the surface loads 8 simple facts
    Then the working memory view has a scrollbar

  # use-case: PD-WM-AF-013
  Scenario: No scrollbar when all facts fit the panel height
    Given a working memory surface sized 40 by 10
    And the surface loads 2 simple facts
    Then the working memory view has no scrollbar

  # use-case: PD-WM-AF-002 / PD-WM-AF-012
  Scenario: PgDn keeps the newly selected fact visible past the fold
    Given a working memory surface sized 40 by 10
    And the surface loads 8 simple facts
    Then the working memory view highlights "k1"
    When the surface receives key "pgdown"
    And the surface receives key "pgdown"
    And the surface receives key "pgdown"
    And the surface receives key "pgdown"
    And the surface receives key "pgdown"
    Then the working memory view highlights "k6"
