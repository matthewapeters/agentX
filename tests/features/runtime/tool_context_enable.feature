# Source contracts:
#   - docs/ux/03_PANEL_DETAILS.md PD-CTX-AF-011 (enable a tool-call/tool-result)
#   - docs/implementation/03_configuration_and_storage.md (Enabled Semantics)
#
# Behavior: a tool_call/tool_result element is retained but disabled by default —
# its text already reached the model once, folded into the turn that produced it.
# Enabling it from the context surface (the same toggle as PD-CTX-AF-006, applied
# to a content class that starts off) is session-scoped: the orchestrator
# registers it as a context-history entry, so its text folds into every
# subsequent turn's assembled context too, until it is disabled again. This is
# the session-scoped mechanism for carrying a tool's output (e.g. a `tree .`
# project listing) forward across turns without re-running the tool — distinct
# from Pin (docs/ux/03_PANEL_DETAILS.md PD-CTX-AF-012 / PD-WM), the durable,
# working-memory-backed mechanism covered by wm_pin.feature. An enabled tool
# element's bytes are also counted under the context-visualizer's "tools" band.

@integration @arch:context-surface @ux:PD-CTX
Feature: Enable a tool call/result for subsequent turns' context
  As a user managing my agent's context
  I want to enable a prior tool call's output
  So that it stays available across several turns without re-running the tool

  # use-case: UC-CTX-ENABLE
  Scenario: A disabled tool result is scoped to the turn that produced it
    Given an orchestrator that runs the "list_dir" tool, captures context, and answers "done"
    When the prompt "list the project" runs the enabling tool cycle
    And the prompt "what did you just say" is submitted for enabling
    Then the enabling-turn context omits "project listing: a.go, b.go"

  # use-case: UC-CTX-ENABLE
  Scenario: Enabling a tool result folds it into a later turn's context
    Given an orchestrator that runs the "list_dir" tool, captures context, and answers "done"
    When the prompt "list the project" runs the enabling tool cycle
    And the last tool result is enabled
    And the prompt "what files are there" is submitted for enabling
    Then the enabling-turn context includes "project listing: a.go, b.go"

  # use-case: UC-CTX-ENABLE
  Scenario: Disabling a tool result again withholds it
    Given an orchestrator that runs the "list_dir" tool, captures context, and answers "done"
    When the prompt "list the project" runs the enabling tool cycle
    And the last tool result is enabled
    And the prompt "first follow-up" is submitted for enabling
    And the last tool result is disabled
    And the prompt "second follow-up" is submitted for enabling
    Then the enabling-turn context omits "project listing: a.go, b.go"

  # use-case: UC-CTX-ENABLE
  # Enabling is content-agnostic: a tool_call element (the proposed command) is
  # just as toggleable as a tool_result element (PD-CTX-AF-011 covers both). The
  # enabled text is the rendered argv ("ls -la -- .") — the command actually run,
  # not the tool id.
  Scenario: Enabling a tool call folds its proposal text into a later turn's context
    Given an orchestrator that runs the "list_dir" tool, captures context, and answers "done"
    When the prompt "list the project" runs the enabling tool cycle
    And the last tool call is enabled
    And the prompt "what did you run" is submitted for enabling
    Then the enabling-turn context includes "ls -la -- ."

  # use-case: UC-CTX-ENABLE-BREAKDOWN
  # An enabled tool element's bytes land in the visualizer's otherwise-always-zero
  # "tools" band (PD-CTXVIZ), not misattributed to "user".
  Scenario: An enabled tool result feeds the context-breakdown "tools" class
    Given an orchestrator that runs the "list_dir" tool, captures context, and answers "done"
    When the prompt "list the project" runs the enabling tool cycle
    And the last tool result is enabled
    Then the enabling context breakdown includes class "tools"
