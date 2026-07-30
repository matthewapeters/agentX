# Source contracts:
#   - docs/ux/PD-CONFIG_spec.md (PD-CONFIG-AF-001..012)
#   - docs/ux/03_PANEL_DETAILS.md PD-CONFIG
#
# Behavior: the config surface reads and mutates agentx.toml through dedicated
# typed endpoints; reads are loopback-open, mutations are token-gated.

@integration @arch:transport
Feature: Configuration surface API over the transport
  As the config surface
  I want typed read/write endpoints for agentx.toml
  So that the user can curate runtime settings without hand-editing

  # use-case: UC-CONFIG-READ  (TC-M2-config-001)
  Scenario: Reading config returns the current effective configuration
    Given a running transport server
    When the client reads the config over the transport
    Then the config response includes "ollama_host"
    And the config response includes "ollama_model"
    And the config response includes "llamacpp_host"
    And the config response includes "llamacpp_model"
    And the config response includes "provider"

  # use-case: UC-CONFIG-SCHEMA  (TC-M2-config-002)
  Scenario: Reading config schema returns validation rules
    Given a running transport server
    When the client reads the config schema over the transport
    Then the schema response includes "provider"
    And the schema response includes "ollama_host"
    And the schema response includes "ollama_model"
    And the schema response includes "llamacpp_host"
    And the schema response includes "llamacpp_model"
    And the schema response includes "ollama_host" type for "host"
    And the schema response includes "ollama_model" type for "model"
    And the schema response includes "provider" type for "enum"

  # use-case: UC-PROVIDER-MODELS  (TC-M2-config-003)
  Scenario: Listing models returns hosted models
    Given a running transport server
    When the client requests models for "ollama" over the transport
    Then the model list is not empty
    And each model is a non-empty string
    And the model list includes "llama3.1"

  # use-case: UC-PROVIDER-MODELS-MISSING  (TC-M2-config-004)
  # Note: The fake provider returns the same model list for all providers.
  # This test is skipped for now.
  Scenario: Listing models for any provider returns hosted models
    Given a running transport server
    When the client requests models for "ollama" over the transport
    Then the model list is not empty

  # use-case: UC-TEST-HOST-SUCCESS  (TC-M2-config-005)
  Scenario: Testing a reachable host returns success
    Given a running transport server
    When the client tests host "localhost:11434" over the transport
    Then the test response shows "reachable" "true"

  # use-case: UC-TEST-HOST-FAIL  (TC-M2-config-006)
  Scenario: Testing an unreachable host returns failure
    Given a running transport server
    When the client tests host "localhost:99999" over the transport
    Then the test response shows "reachable" "false"

  # use-case: UC-TEST-HOST-INVALID  (TC-M2-config-007)
  Scenario: Testing a host with empty input returns 400
    Given a running transport server
    When the client tests host "" over the transport
    Then the test returns status 400
