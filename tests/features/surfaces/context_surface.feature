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

  # use-case: PD-CTX-AF-001  (TC-M2-context-007)
  # the startup bootstrap exchange engages the session but is not user conversation
  Scenario: Ephemeral (bootstrap) events are omitted
    Given a context surface sized 40 by 12
    When the context surface applies an ephemeral agent_response "boot greeting"
    And the context surface applies an agent_response "real answer"
    Then the context view contains "real answer"
    And the context view does not contain "boot greeting"

  # use-case: PD-CTX-AF-004  (TC-M2-context-006)
  # the viewer is focused, so the selected object is highlighted for navigation
  Scenario: The selected object is highlighted
    Given a context surface sized 40 by 12
    When the context surface applies a user_prompt "hello there"
    Then the context view shows a selected object border

  # use-case: PD-CTX-AF-003  (TC-M2-context-004)
  Scenario: Thinking is collapsed by default
    Given a context surface sized 40 by 12
    When the context surface applies a thinking "secret reasoning"
    Then the context view does not contain "secret reasoning"

  # use-case: PD-CTX-AF-006  (enable/disable the selected element)
  # elements show an enabled checkbox; space unchecks the selected element.
  Scenario: Space unchecks the selected element
    Given a context surface sized 40 by 12
    When the context surface applies a user_prompt "keep this"
    And the context surface applies an agent_response "drop this"
    Then the context view contains "[x]"
    When the context surface receives key "space"
    Then the context view contains "[ ]"

  # use-case: PD-CTX-AF-008  (non-toggleable elements)
  # thinking never enters context, so it carries no checkbox and space is a no-op.
  Scenario: A thinking element has no checkbox
    Given a context surface sized 40 by 12
    When the context surface applies a thinking "reasoning"
    And the context surface receives key "space"
    Then the context view does not contain "[ ]"

  # use-case: PD-CTX-AF-011  (pin a tool_result)
  # a tool result starts unpinned (unchecked) — its text already reached the model
  # once, via the respond turn's own context block — but carries a checkbox because
  # the context surface can pin it into every subsequent turn too.
  Scenario: A tool result starts unchecked and can be pinned
    Given a context surface sized 40 by 12
    When the context surface applies a tool_result "project listing: a.go, b.go"
    Then the context view contains "[ ]"
    When the context surface receives key "space"
    Then the context view contains "[x]"

  # use-case: PD-CTX-AF-011  (pin a tool_call)
  Scenario: A tool call starts unchecked and can be pinned
    Given a context surface sized 40 by 12
    When the context surface applies a tool_call "tree ."
    Then the context view contains "[ ]"
    When the context surface receives key "space"
    Then the context view contains "[x]"
