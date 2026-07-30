# Source contracts:
#   - docs/ux/PD-CONFIG_spec.md (Phase 3b: AF-008)
#
# Behavior: the config surface detects external edits to agentx.toml via the
# orchestrator's config_changed event, re-fetches the config, diffs against the
# current state, highlights changed keys with yellow background, and shows a
# reload prompt dialog.

@functional @arch:surface-client
Feature: Config surface — external file change detection (Phase 3b)
  As a user
  I want the config surface to detect and highlight external edits to agentx.toml
  So that I can see what changed and choose to reload

  # use-case: PD-CONFIG-AF-008 (detect external change and highlight)
  Scenario: External editor triggers change detection and highlighting
    Given a config surface loaded with initial config
    When an external editor modifies the config file "/tmp/agentx.toml"
    And the config surface detects an external change
    Then the config surface highlights changed keys
    And the config surface shows the reload prompt dialog
    And the config view contains the external change dialog

  # use-case: PD-CONFIG-AF-008 (reload via r key)
  Scenario: User reloads by pressing r
    Given a config surface loaded with initial config
    When an external editor modifies the config file "/tmp/agentx.toml"
    And the config surface detects an external change
    And the user presses "r" to reload from file

  # use-case: PD-CONFIG-AF-008 (keep changes option)
  Scenario: User keeps TUI changes and dismisses
    Given a config surface loaded with initial config
    When an external editor modifies the config file "/tmp/agentx.toml"
    And the config surface detects an external change
    And the user selects "Keep changes" and confirms
    Then the config view does not contain the external change dialog

  # use-case: PD-CONFIG-AF-008 (dismiss via escape)
  Scenario: User dismisses via escape
    Given a config surface loaded with initial config
    When an external editor modifies the config file "/tmp/agentx.toml"
    And the config surface detects an external change
    And the user presses escape to dismiss the external change dialog
    Then the config view does not contain the external change dialog

  # use-case: PD-CONFIG-AF-008 (debounce rapid changes)
  Scenario: Rapid successive external edits are debounced
    Given a config surface loaded with initial config
    When an external editor modifies the config file "/tmp/agentx.toml"
    And the config surface detects an external change
    And a second external change is detected while the first is pending
    Then no double external change dialog was created

  # use-case: PD-CONFIG-AF-008 (hint row shows r reload)
  Scenario: Hint row documents the reload keybinding
    Given a config surface loaded with initial config
    When an external editor modifies the config file "/tmp/agentx.toml"
    And the config surface detects an external change
    Then the hint row mentions "r reload"
