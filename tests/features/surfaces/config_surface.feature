# Source contracts:
#   - docs/ux/PD-CONFIG_spec.md (Phases 2a–2c)
#
# Behavior: the config surface renders the orchestrator's effective config as a
# navigable tree of sections and keys, fetched via the transport's read-only
# endpoints. Phases 2a/2b handle the scaffold, navigation, and editing.
# Phase 2c adds dialogs, color picker, model picker, restart flow, and
# improved hint rows with full keybinding documentation.

@functional @arch:surface-client
Feature: Config surface (Phases 2a–2c)
  As a user
  I want to browse and edit the runtime config interactively
  So that I can tune AgentX without editing the TOML file by hand

  # use-case: PD-CONFIG-AF-001 (render read-only config)
  Scenario: Renders the config tree loaded from transport
    Given a config surface sized 80 by 24
    When the config surface fetches config with provider "ollama" and ollama_host "localhost:11434"
    Then the config view contains "[agentx.ollama]"
    And the config view contains "Ollama Host"
    And the config view contains "localhost:11434"
    And the config view contains "[agentx]"
    And the config view contains "Provider"
    And the config view contains "ollama"

  # use-case: PD-CONFIG-AF-002 (navigate sections)
  Scenario: Section navigation with j/k
    Given a config surface sized 80 by 24
    When the config surface fetches config with provider "ollama" and ollama_host "localhost:11434"
    And the config surface receives key "j"
    Then the config view contains "Ollama Host"
    And the config view contains "Provider"

  # use-case: PD-CONFIG-AF-002 (keys within section)
  Scenario: Cursor moves between keys in a section
    Given a config surface sized 80 by 24
    When the config surface fetches config with provider "ollama" and ollama_host "localhost:11434" and llamacpp_host "localhost:8080"
    And the config surface receives key "j"
    And the config surface receives key "j"
    Then the config view contains "llama.cpp Host"

  # use-case: PD-CONFIG-AF-002 (section list)
  Scenario: Sections are listed with their keys
    Given a config surface sized 80 by 24
    When the config surface fetches config with provider "ollama"
    Then the config view contains "agentx"

  # use-case: PD-CONFIG-AF-002 (restart indicator, Phase 2c)
  Scenario: Restart-required keys show restart indicator
    Given a config surface sized 80 by 24
    When the config surface fetches config with provider "ollama"
    Then the config view contains "restart"

  # use-case: PD-CONFIG-AF-002 (navigation keys)
  Scenario: H/L keys move between sections
    Given a config surface sized 80 by 24
    When the config surface fetches config with provider "ollama" and ollama_host "localhost:11434" and llamacpp_host "localhost:8080"
    And the config surface receives key "h"
    Then the config view contains "agentx"
    And the config view contains "Provider"

  # use-case: PD-CONFIG-AF-001 (error state)
  Scenario: Error is shown when transport fails
    Given a config surface sized 80 by 24
    And the config surface fails to fetch config
    Then the config view contains "config read failed"

  # use-case: PD-CONFIG-AF-011 (hint row with full keybindings, Phase 2c)
  Scenario: Hint row shows full keybindings
    Given a config surface sized 80 by 24
    When the config surface fetches config with provider "ollama"
    Then the config view contains "j/k"
    And the config view contains "quit"
    And the config view contains "help"
    And the config view contains "save"

  # use-case: PD-CONFIG-AF-003 (edit a host field opens host editor)
  Scenario: Host field enters edit mode
    Given a config surface sized 80 by 24
    When the config surface fetches config with provider "ollama" and ollama_host "localhost:11434"
    And the config surface receives key "enter"
    Then the config view contains "Ollama Host"

  # use-case: PD-CONFIG-AF-009 (restart confirmation dialog, Phase 2c)
  Scenario: Save key shows keybindings hint row
    Given a config surface sized 80 by 24
    When the config surface fetches config with provider "ollama"
    And the config surface receives key "s"
    Then the config view contains "save"

  # use-case: PD-CONFIG-AF-009 (help dialog, Phase 2c)
  Scenario: Help key shows keybindings overlay
    Given a config surface sized 80 by 24
    When the config surface fetches config with provider "ollama"
    And the config surface receives key "?"
    Then the config view contains "Help"

  # use-case: PD-CONFIG-AF-012 (quit confirmation, Phase 2c)
  Scenario: Quit confirmation appears with unsaved changes
    Given a config surface sized 80 by 24
    When the config surface fetches config with provider "ollama"
    And the config surface receives key "j"
    And the config surface receives key "enter"
    And the config surface receives key "x"
    And the config surface receives key "escape"
    And the config surface receives key "q"
    Then the config view contains "Unsaved changes"
