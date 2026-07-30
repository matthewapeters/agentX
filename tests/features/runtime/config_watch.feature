# Source contracts:
#   - docs/ux/PD-CONFIG_spec.md (PD-CONFIG-AF-008)
#
# Behavior: external edits to agentx.toml are detected by the orchestrator and
# fanned out as config_changed events to attached surfaces.

@integration @arch:runtime
Feature: Orchestrator detects external edits to agentx.toml and publishes config_changed events

  # use-case: UC-CONFIG-WATCH-001
  Scenario: External edit publishes a config_changed event
    Given an orchestrator with config watcher enabled
    And the config file exists at "/tmp/agentx-watch-test/agentx.toml"
    When I edit the config file (simulate external editor)
    And I wait for debounce window
    Then a "config_changed" event is published on the bus
    And the config_changed event has session_id set

  # use-case: UC-CONFIG-WATCH-002
  Scenario: Rapid successive edits are debounced to a single event
    Given an orchestrator with config watcher enabled
    And the config file exists at "/tmp/agentx-watch-test/agentx.toml"
    When I rapidly edit the config file 5 times (within debounce window)
    And I wait for debounce window
    Then at most 2 "config_changed" events are published on the bus

  # use-case: UC-CONFIG-WATCH-003
  Scenario: Event carries the config path in payload
    Given an orchestrator with config watcher enabled
    And the config file exists at "/tmp/agentx-watch-test/agentx.toml"
    When I edit the config file (simulate external editor)
    And I wait for debounce window
    Then the config_changed event payload contains "path"
    And the config_changed event payload path ends with "agentx.toml"

  # use-case: UC-CONFIG-WATCH-004
  Scenario: Events are delivered to bus subscribers
    Given an orchestrator with config watcher enabled
    And the config file exists at "/tmp/agentx-watch-test/agentx.toml"
    And a subscriber is attached to the bus
    When I edit the config file (simulate external editor)
    And I wait for debounce window
    Then the subscriber receives a "config_changed" event
