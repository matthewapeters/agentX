# Source contracts:
#   - docs/architecture/adr/0013-llm-provider-abstraction.md
#   - internal/config (provider key in [agentx])
#
# Behavior: the runtime resolves a provider from config, defaults to "ollama",
# and accepts both "ollama" and "llamacpp" as values. The [agentx.ollama] and
# [agentx.llamacpp] sections share host/model keys. chat_backend is an alias
# for provider for backward compatibility.

@integration @arch:config-resolution
Feature: Provider configuration resolution
  As the agentX runtime
  I want a deterministic provider selection from config
  So that the orchestrator wires the correct model adapter

  # use-case: UC-CONFIG-PROVIDER-DEFAULT
  Scenario: Default provider is ollama when unset
    Given no deployment config exists
    When the runtime resolves configuration
    Then the effective provider is "ollama"

  # use-case: UC-CONFIG-PROVIDER-VALUE
  Scenario: An explicit provider value is honored
    Given a deployment config with provider "llamacpp"
    And a llamacpp host "localhost:8080" and model "ornith-1.0-35b-Q4_K_M"
    When the runtime resolves configuration
    Then the effective provider is "llamacpp"
    And the effective llamacpp host is "localhost:8080"
    And the effective llamacpp model is "ornith-1.0-35b-Q4_K_M"

  # use-case: UC-CONFIG-PROVIDER-OLLAMA
  Scenario: An ollama provider resolves the ollama host and model
    Given a deployment config with provider "ollama"
    And an ollama host "localhost:11434" and model "phi4-mini:3.8b"
    When the runtime resolves configuration
    Then the effective provider is "ollama"
    And the effective ollama host is "localhost:11434"
    And the effective ollama model is "phi4-mini:3.8b"

  # use-case: UC-CONFIG-PROVIDER-ALIASES
  # chat_backend is accepted as an alias for provider
  Scenario: chat_backend is accepted as a provider alias
    Given a deployment config with chat_backend "llamacpp"
    And a llamacpp host "peters01:8888" and model "ornith-1.0-35b-Q4_K_M"
    When the runtime resolves configuration
    Then the effective provider is "llamacpp"
    And the effective llamacpp host is "peters01:8888"

  # use-case: UC-CONFIG-PROVIDER-SEED
  Scenario: First run seeds provider into the deployment config
    Given no deployment config exists
    When the runtime resolves configuration with provider "llamacpp"
    Then a deployment config file is created
    And the seeded deployment config contains provider "llamacpp"
