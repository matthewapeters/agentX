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

  # use-case: UC-CTX-PIN-API  (Pin a tool_result into working memory, PD-CTX-AF-012 / PD-WM)
  Scenario: The pin endpoint copies a tool_result into working memory and disables it
    Given a running transport server
    And a recorded tool_result event for tool "list_dir" valued "project listing"
    When the client pins the last recorded element as static
    Then reading working memory includes a pin-owned fact valued "project listing"
    And the pin-source element is disabled in context

  # use-case: UC-WM-LIVE-API  (play/pause a pinned fact, PD-WM-AF-008)
  Scenario: The live endpoint toggles a pinned fact
    Given a running transport server
    And a recorded tool_result event for tool "list_dir" valued "project listing"
    When the client pins the last recorded element as static
    Then the client can set the pinned fact live over the transport

  # use-case: UC-CTX-PIN-NODE-API (Pin a plan node's own resolved value, ADR 0012
  # amendment — a Step/Know has no tool_result event to pin via /events/{ordinal}/pin)
  Scenario: The plan-node pin endpoint copies a resolved Step's value into working memory
    Given a running transport server
    And plan "w" node "w-1" resolved to "Go" for goal "the project's dominant language"
    When the client pins plan "w" node "w-1"
    Then reading working memory includes a pin-owned fact valued "Go"

  Scenario: Pinning an unresolved plan node is refused
    Given a running transport server
    When pinning plan "w" node "missing" fails

  # A plan-node pin has no tool Source to ever re-run (PinPlanNode's doc comment;
  # mirrors PD-WM-AF-009's existing policy-Allow-only-live refusal, applied here at
  # pin time instead of toggle time since there is no tool to gate on at all).
  Scenario: A plan-node pin can never be set live
    Given a running transport server
    And plan "w" node "w-1" resolved to "Go" for goal "the project's dominant language"
    And the client pins plan "w" node "w-1"
    When the client tries to set the pinned fact live over the transport
    Then the attempt is refused as not pinned to a tool source
