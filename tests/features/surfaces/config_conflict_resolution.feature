# Source contracts:
#   - docs/ux/PD-CONFIG_spec.md (Phase 3c: conflict resolution + edge cases)
#
# Behavior: the config surface handles two-way sync edge cases:
#   - TUI unsaved changes win over external file changes (conflict resolution)
#   - Surface-side debounce for rapid successive external change events
#   - External change events are queued during POST /config and processed after
#   the save completes
#   - When TUI wins, the status bar shows "External change discarded (TUI
#   changes take precedence)."

@functional @arch:surface-client
Feature: Config surface — conflict resolution and edge cases (Phase 3c)
  As a user
  I want the config surface to handle two-way sync edge cases correctly
  So that my edits are never lost and rapid external changes don't cause flicker

  # use-case: PD-CONFIG-AF-008 variant: TUI unsaved changes win over external change
  Scenario: TUI unsaved changes take precedence over external file change
    Given a config surface loaded with initial config
    And the config surface has unsaved TUI changes
    When an external editor modifies the config file "/tmp/agentx.toml"
    Then the config surface shows the conflict resolution dialog
    And the conflict resolution dialog has "Keep TUI changes" selected
    And the status bar shows "external change discarded"
    And the status bar shows "TUI changes take precedence"

  # use-case: PD-CONFIG-AF-008 variant: confirming "Keep TUI changes" dismisses
  Scenario: Confirming "Keep TUI changes" dismisses the dialog
    Given a config surface loaded with initial config
    And the config surface has unsaved TUI changes
    When an external editor modifies the config file "/tmp/agentx.toml"
    And the user selects "Keep TUI changes" and confirms
    Then the config view does not contain the conflict resolution dialog
    And the status bar shows "TUI changes take precedence"

  # use-case: PD-CONFIG-AF-008 variant: confirming "Discard" dismisses
  Scenario: Confirming "Discard" dismisses the conflict resolution dialog
    Given a config surface loaded with initial config
    And the config surface has unsaved TUI changes
    When an external editor modifies the config file "/tmp/agentx.toml"
    And the user selects "Discard" and confirms
    Then the config view does not contain the conflict resolution dialog

  # use-case: PD-CONFIG-AF-008 variant: without unsaved changes, normal dialog
  Scenario: Normal external change dialog when TUI has no unsaved changes
    Given a config surface loaded with initial config
    When an external editor modifies the config file "/tmp/agentx.toml"
    Then the config surface shows the external change dialog (not conflict resolution)
    And the external change dialog has "Reload" selected

  # use-case: PD-CONFIG-AF-008 variant: debounce rapid successive changes
  Scenario: Rapid successive external edits are debounced on the surface
    Given a config surface loaded with initial config
    And a previous external change was detected at epoch 1000
    When an external change is detected at epoch 3000
    And a second external change is detected at epoch 3500
    Then no double external change dialog was created
    And the last external event epoch is 3500

  # use-case: PD-CONFIG-AF-008 variant: event beyond debounce window
  Scenario: External change beyond debounce window is processed normally
    Given a config surface loaded with initial config
    And a previous external change was detected at epoch 1000
    When an external change is detected at epoch 5000
    Then the last external event epoch is 5000

  # use-case: PD-CONFIG-AF-008 variant: queue event during save
  Scenario: External change during POST /config is queued
    Given a config surface loaded with initial config
    When the config surface is saving
    And an external change is detected at epoch 5000 while saving
    Then the external change is queued
    And the status bar shows "external change queued"

  # use-case: PD-CONFIG-AF-008 variant: process queued event after save
  Scenario: Queued external change is processed after save completes
    Given a config surface loaded with initial config
    When the config surface is saving
    And an external change is detected at epoch 5000 while saving
    And the pending external event is processed
    Then the pending external event is cleared

  # use-case: PD-CONFIG-AF-008 variant: hint row mentions conflict resolution
  Scenario: Hint row shows conflict resolution dialog hint
    Given a config surface loaded with initial config
    And the config surface has unsaved TUI changes
    When an external editor modifies the config file "/tmp/agentx.toml"
    Then the hint row mentions "Keep TUI changes"
