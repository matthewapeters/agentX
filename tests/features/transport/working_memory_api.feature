# Source contracts:
#   - docs/implementation/02_surface_orchestration_http.md (Working-Memory CRUD SS-6)
#   - docs/ux/03_PANEL_DETAILS.md PD-WM
#
# Behavior: the working-memory editor surface reads and mutates working_memory.json
# through dedicated typed endpoints; reads are loopback-open, mutations are token-gated.

@integration @arch:transport
Feature: Working-memory CRUD over the transport
  As the working-memory surface
  I want typed read/write endpoints for facts
  So that the user can curate what the agent sees

  # use-case: UC-WM-CRUD  (TC-M2-wm-001)
  Scenario: Add, edit, disable, and delete a fact
    Given a running transport server
    When the client adds a fact "color" valued "blue"
    Then reading working memory includes "color" valued "blue" enabled
    When the client edits fact "color" to "green"
    Then reading working memory includes "color" valued "green" enabled
    When the client disables fact "color"
    Then reading working memory shows "color" disabled
    When the client enables fact "color"
    Then reading working memory includes "color" valued "green" enabled
    When the client deletes fact "color"
    Then reading working memory has no "color" fact

  # use-case: UC-WM-AUTH  (TC-M2-wm-002)
  Scenario: Mutations require the attach token
    Given a running transport server
    When an unauthorized client adds a fact "x" valued "y"
    Then the working-memory mutation is rejected as "auth"
    And reading working memory has no "x" fact

  # use-case: UC-CTX-TOGGLE-API  (context surface → enable/disable endpoint)
  Scenario: The enable/disable endpoint reaches the orchestrator
    Given a running transport server
    When the client toggles element 7 to enabled false
    Then the provider recorded a toggle for ordinal 7 enabled false
