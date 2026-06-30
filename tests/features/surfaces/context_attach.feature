# Source contracts:
#   - docs/implementation/02_surface_orchestration_http.md (Seed + Resume SS-1, Surface Host SS-2)
#   - docs/ux/03_PANEL_DETAILS.md PD-CTX
#   - docs/build-plan/06_system_surfaces_backlog.md (SS-3)
#
# Behavior: a context surface attaching to a live orchestrator seeds the prior
# exchange from the durable log and renders live events streamed thereafter — the
# full data path (HTTP seed + SSE resume + context projection) end to end.

@e2e @arch:surface-client @ux:PD-CTX
Feature: Context surface attaches and mirrors the session
  As a user
  I want to launch a context surface beside the chat
  So that it shows the whole session, seeded and live

  # use-case: PD-CTX-AF-001 / PD-CTX-AF-002  (TC-M2-context-005)
  Scenario: Seed renders the prior exchange and live events stream in
    Given a running orchestrator serving the transport
    And a recorded exchange prompt "design the API" response "here is the design"
    When a context surface attaches over the transport
    Then the attached context view contains "design the API"
    And the attached context view contains "here is the design"
    When a live response "and the tests" is recorded
    Then the attached context view renders "and the tests"
