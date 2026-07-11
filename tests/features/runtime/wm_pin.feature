# Source contracts:
#   - docs/ux/03_PANEL_DETAILS.md PD-CTX-AF-012 / PD-WM (Pin static / live)
#   - docs/implementation/03_configuration_and_storage.md (Pinning to Working Memory)
#
# Behavior: pinning a tool_result conversation element copies it into working
# memory as a durable fact (Owner: pin) and disables the source event in context,
# so the content is represented exactly once. A static pin is a frozen snapshot;
# a live pin re-runs its source tool before every turn's context assembly
# (Orchestrator.refreshLiveFacts) and is refused unless the tool currently
# evaluates to policy Allow. Unpinning (deleting the fact) does not restore the
# source event's enabled state — that stays under the context surface's
# independent control (tool_context_enable.feature).

@integration @arch:context-surface @ux:PD-WM
Feature: Pin a tool result to working memory (static / live)
  As a user managing my agent's context
  I want to pin a tool result into working memory, frozen or self-refreshing
  So that durable, curated knowledge stays available without re-running by hand

  # use-case: UC-WM-PIN
  Scenario: Pinning a tool result creates a fact and disables it in context
    Given an orchestrator that runs the "list_dir" counting tool and answers "done"
    When the prompt "list the project" runs the wm-pin tool cycle
    And the last tool result is pinned static to working memory
    Then working memory has a pin-owned fact valued "run 1"
    And the source tool result is disabled in context

  # use-case: UC-WM-PIN
  Scenario: A static pin stays frozen across turns
    Given an orchestrator that runs the "list_dir" counting tool and answers "done"
    When the prompt "list the project" runs the wm-pin tool cycle
    And the last tool result is pinned static to working memory
    And the prompt "anything else" is submitted for wm-pin
    Then the pinned fact is still valued "run 1"

  # use-case: UC-WM-PIN-LIVE
  Scenario: A live pin refreshes before every subsequent turn
    Given an orchestrator that runs the "list_dir" counting tool and answers "done"
    When the prompt "list the project" runs the wm-pin tool cycle
    And the last tool result is pinned live to working memory
    And the prompt "anything else" is submitted for wm-pin
    Then the pinned fact is valued "run 2"

  # use-case: UC-WM-PIN-LIVE
  Scenario: Setting a fact live immediately refreshes it once
    Given an orchestrator that runs the "list_dir" counting tool and answers "done"
    When the prompt "list the project" runs the wm-pin tool cycle
    And the last tool result is pinned static to working memory
    And the pinned fact is set live
    Then the pinned fact is valued "run 2"

  # use-case: UC-WM-PIN-LIVE
  Scenario: Toggling a pin back to static stops refreshing it
    Given an orchestrator that runs the "list_dir" counting tool and answers "done"
    When the prompt "list the project" runs the wm-pin tool cycle
    And the last tool result is pinned live to working memory
    And the prompt "anything else" is submitted for wm-pin
    And the pinned fact is set static
    And the prompt "one more thing" is submitted for wm-pin
    Then the pinned fact is still valued "run 2"

  # use-case: UC-WM-PIN-POLICY
  # A blacklisted tool can still be pinned static (a frozen snapshot needs no
  # future permission), but going live must never silently re-run something that
  # would otherwise need approval.
  Scenario: Setting a fact live is refused when its source tool is blacklisted
    Given an orchestrator with the "list_dir" tool blacklisted that runs it and answers "done"
    When the prompt "list the project" runs the wm-pin tool cycle
    And the last tool result is pinned static to working memory
    Then setting the pinned fact live is refused

  # use-case: UC-WM-PIN-UNPIN
  Scenario: Unpinning does not restore the source event's enabled state
    Given an orchestrator that runs the "list_dir" counting tool and answers "done"
    When the prompt "list the project" runs the wm-pin tool cycle
    And the last tool result is pinned static to working memory
    And the pinned fact is deleted
    Then the source tool result is still disabled in context

  # use-case: UC-WM-PIN-BREAKDOWN
  # A pinned fact's bytes count as ordinary working memory, not the context
  # surface's "tools" band — Pin hands the content off to WM entirely.
  Scenario: A pinned fact counts under working-memory, not tools
    Given an orchestrator that runs the "list_dir" counting tool and answers "done"
    When the prompt "list the project" runs the wm-pin tool cycle
    And the last tool result is pinned static to working memory
    Then the wm-pin context breakdown includes class "working-memory"
    And the wm-pin context breakdown omits class "tools"
