# Source contracts:
#   - docs/ux/03_PANEL_DETAILS.md PD-10 (ContextMeterWidget, re-authored for the TUI)
#   - docs/build-plan/06_system_surfaces_backlog.md (Future surfaces: context-visualizer)
#
# Behavior: the context-visualizer surface renders the assembled context window as
# a read-only budget meter — one bar per content class (with the app's content
# emoji) plus a remaining-capacity band — measured against the model's context
# window. It performs no writes; management lives on the context pane.

@functional @arch:context-visualizer @ux:PD-10
Feature: Context visualizer meter
  As a user arranging surfaces beside the chat
  I want a read-only breakdown of my context window by content class
  So that I can see how the budget is spent and know what to prune

  # use-case: UC-CTXVIZ-METER
  Scenario: The meter renders per-class bands against the model window
    Given a context visualizer for session "sunny-otter"
    And a visualizer window of 8192 tokens for model "qwen2.5"
    And the visualizer class "working-memory" contributes 400 chars
    And the visualizer class "user" contributes 800 chars
    And the visualizer breakdown is applied
    Then the visualizer view shows "context · sunny-otter"
    And the visualizer view shows "🧠 working memory"
    And the visualizer view shows "👤 user"
    And the visualizer view shows "remaining"
    And the visualizer view shows "/ 8192 est. tokens"
    And the visualizer view shows "qwen2.5"
    And the visualizer view shows "read-only"

  # use-case: UC-CTXVIZ-METER
  # When the model reports no context length the meter drops the budget percentage
  # and the ghost band rather than inventing a denominator.
  Scenario: The meter degrades gracefully when the window is unknown
    Given a context visualizer for session "mystery-fox"
    And a visualizer window of 0 tokens for model "mystery"
    And the visualizer class "user" contributes 40 chars
    And the visualizer breakdown is applied
    Then the visualizer view shows "window unknown"
    And the visualizer view omits "remaining"

  # use-case: UC-CTXVIZ-READONLY
  # A mutation key (add, in the working-memory surface) does nothing here — the
  # visualizer never opens an editor.
  Scenario: The visualizer is read-only
    Given a context visualizer for session "calm-otter"
    And a visualizer window of 4096 tokens for model "llama3"
    And the visualizer class "user" contributes 40 chars
    And the visualizer breakdown is applied
    And the visualizer receives key "a"
    Then the visualizer view shows "read-only"
    And the visualizer view omits "add"
