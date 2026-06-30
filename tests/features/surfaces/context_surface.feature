# Source contracts:
#   - docs/ux/03_PANEL_DETAILS.md PD-CTX (TUI Context Surface)
#   - docs/build-plan/06_system_surfaces_backlog.md (SS-3)
#   - docs/ux/06_OUTPUT_WIDGET.md (reused renderer)
#
# Behavior: the context viewer projects the session event stream into the shared
# collapsible output renderer (read-only) and shows a processing-state line;
# processing-state events update that line rather than rendering as widgets.

@functional @ux:PD-CTX @arch:surface-client
Feature: Context viewer surface
  As a user
  I want a read-only surface mirroring the conversation
  So that I can watch the session beside the chat surface

  # use-case: PD-CTX-AF-001  (TC-M2-context-001)
  Scenario: Renders applied conversation events
    Given a context surface sized 40 by 12
    When the context surface applies a user_prompt "hello there"
    And the context surface applies an agent_response "hi back"
    Then the context view contains "hello there"
    And the context view contains "hi back"

  # use-case: PD-CTX-AF-005  (TC-M2-context-002)
  Scenario: A processing-state event updates the status line, not a widget
    Given a context surface sized 40 by 12
    When the context surface applies processing-state "working" "respond"
    Then the context status line shows "working"
    And the context status line shows "respond"

  # use-case: PD-CTX-AF-005  (TC-M2-context-003)
  Scenario: Idle status by default
    Given a context surface sized 40 by 12
    Then the context status line shows "idle"

  # use-case: PD-CTX-AF-003  (TC-M2-context-004)
  Scenario: Thinking is collapsed by default
    Given a context surface sized 40 by 12
    When the context surface applies a thinking "secret reasoning"
    Then the context view does not contain "secret reasoning"
