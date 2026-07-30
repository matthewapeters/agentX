# Source contracts:
#   - internal/transport/http.Server.handlePostConfig (POST /config)
#   - internal/transport/http.Server.handleTestHost (POST /test/host)
#   - internal/runtime.Orchestrator.SetConfig (Phase 1d orchestrator-side impl)
#   - internal/config.WriteConfig (Phase 1b transactional write)
#   - internal/llm/provider/validation (Phase 1c type validation)
#
# Behavior: the transport serves write endpoints that validate, normalize, and
# atomically write config changes. The orchestrator validates types, classifies
# keys as live or restart-required, and writes atomically under the semaphore.

@integration @arch:config-write @arch:transport
Feature: Config write endpoints over the transport
  As the config surface
  I want POST /config and POST /test/host to validate and apply changes
  So that the TUI can save edits and test host endpoints

  # use-case: UC-CONFIG-WRITE-VALID
  Scenario: Valid config write returns applied
    Given a running transport server for config write tests
    When the client POSTs "/config" with payload
      | key | value |
      | ollama_model | "test-model:latest" |
    Then the config write response status is 200
    And the config write response field "status" is "applied"

  # use-case: UC-CONFIG-WRITE-INVALID-PROVIDER
  Scenario: Invalid provider is rejected
    Given a running transport server for config write tests
    When the client POSTs "/config" with payload
      | key | value |
      | provider | "invalid_provider" |
    Then the config write response status is 400
    And the config write response field "status" is "error"

  # use-case: UC-CONFIG-WRITE-NORMALIZE-CHATBACKEND
  Scenario: chat_backend is normalized to provider
    Given a running transport server for config write tests
    When the client POSTs "/config" with payload
      | key | value |
      | chat_backend | "ollama" |
    Then the config write response status is 200
    And the config write normalized key "chat_backend" → "provider"

  # use-case: UC-CONFIG-WRITE-KEYS-CLASSIFIED
  Scenario: Keys are classified as restart-required
    Given a running transport server for config write tests
    When the client POSTs "/config" with payload
      | key | value |
      | ollama_host | "localhost:11434" |
      | ollama_model | "phi4:latest" |
    Then the config write response status is 200
    And the config write response field "status" is "applied"

  # use-case: UC-TEST-HOST-VALID
  Scenario: Test a reachable host
    Given a running transport server for config write tests
    When the config write client tests host "localhost:11434" over the transport
    Then the config write test response shows "reachable" "true"

  # use-case: UC-TEST-HOST-UNREACHABLE
  Scenario: Test an unreachable host
    Given a running transport server for config write tests
    When the config write client tests host "localhost:99999" over the transport
    Then the config write test is rejected as "connection refused"

  # use-case: UC-TEST-HOST-MISSING-PROVIDER
  Scenario: Test host without provider returns bad request
    Given a running transport server for config write tests
    When the config write client tests host "localhost:11434" with empty provider
    Then the config write test returns status 400

  # use-case: UC-TEST-HOST-MISSING-HOST
  Scenario: Test host without host returns bad request
    Given a running transport server for config write tests
    When the config write client tests host "" with provider "ollama"
    Then the config write test returns status 400

  # use-case: UC-CONFIG-WRITE-INVALID-PAYLOAD
  Scenario: Empty config payload returns bad request
    Given a running transport server for config write tests
    When the client POSTs "/config" with empty payload
    Then the config write test returns status 400

  # use-case: UC-CONFIG-WRITE-INVALID-JSON
  Scenario: Malformed JSON returns bad request
    Given a running transport server for config write tests
    When the client POSTs "/config" with malformed JSON
    Then the config write test returns status 400
