# Source contracts:
#   - internal/config.Config.Validate
#
# Behavior: Validate catches bad provider names, missing model/host for the
# active backend, and reports them with actionable error messages that name
# the offending key.

@unit @arch:config-validation
Feature: Configuration validation
  As the AgentX runtime
  I want invalid configuration caught early with helpful errors
  So that startup fails clearly instead of surfacing confusing model errors

  # use-case: UC-CONFIG-VALID
  Scenario: Valid ollama config passes validation
    Given a valid config with provider "ollama"
    When config validation passes

  # use-case: UC-CONFIG-VALID
  Scenario: Valid llamacpp config passes validation
    Given a valid config with provider "llamacpp"
    When config validation passes

  # use-case: UC-CONFIG-INVALID-PROVIDER
  Scenario: Invalid provider is rejected with valid choices listed
    Given a config with invalid provider "llamaccp"
    When config validation fails

  # use-case: UC-CONFIG-OLLAMA-MODEL
  Scenario: Ollama provider requires a model name
    Given a config with provider "ollama" and no model
    When config validation fails

  # use-case: UC-CONFIG-LLAMACPP-HOST
  Scenario: Llamacpp provider requires a host
    Given a config with provider "llamacpp" and no host
    When config validation fails

  # use-case: UC-CONFIG-LLAMACPP-MODEL
  Scenario: Llamacpp provider requires a model name
    Given a config with provider "llamacpp" and no model
    When config validation fails
