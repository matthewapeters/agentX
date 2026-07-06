# Source contracts:
#   - docs/ux/03_PANEL_DETAILS.md PD-10 (ContextMeterWidget, re-authored for the TUI)
#   - docs/build-plan/06_system_surfaces_backlog.md (Future surfaces: context-visualizer)
#
# Behavior: the orchestrator reports the assembled context window's composition by
# content class (working memory, instructions, user, assistant) with the model's
# context window as the budget denominator, for the read-only visualizer surface.
# Sizes are the exact bytes withContext assembles; classes not yet fed into
# context (attachments, thinking, tools) are omitted.

@integration @arch:context-visualizer @ux:PD-10
Feature: Context window composition breakdown
  As the agentx runtime
  I want to report the context window's composition by content class
  So that the context-visualizer can show how the budget is spent

  # use-case: UC-CTXVIZ-BREAKDOWN
  Scenario: Breakdown sums each content class and reports the window
    Given an orchestrator for context breakdown with instructions "Be brief." answering "hello there" with context window 8192
    And the context-breakdown fact "os" is "linux"
    And the context-breakdown prompt "hi" is submitted
    When the context breakdown is requested
    Then the breakdown window is 8192
    And the breakdown class "instructions" has 9 chars
    And the breakdown class "user" has 2 chars
    And the breakdown class "assistant" has 11 chars
    And the breakdown includes class "working-memory"

  # use-case: UC-CTXVIZ-BREAKDOWN
  # Classes with no contribution to the assembled context are omitted (the surface
  # renders them as zero) rather than reported with a fabricated size.
  Scenario: Classes not fed into context are omitted
    Given an orchestrator for context breakdown with instructions "Be brief." answering "ok" with context window 4096
    And the context-breakdown prompt "hi" is submitted
    When the context breakdown is requested
    Then the breakdown omits class "thinking"
    And the breakdown omits class "tools"
    And the breakdown omits class "attachments"
