# Source contracts:
#   - docs/implementation/03_configuration_and_storage.md (Configuration Source of Truth)
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-A1)
#
# Behavior: the runtime resolves effective configuration from the deployment config
# first, falls back to project-local defaults when the deployment config is missing,
# and seeds the deployment config with defaults on first launch.

@integration @arch:config-resolution
Feature: Configuration resolution and first-run seeding
  As the agentx runtime
  I want a deterministic effective configuration
  So that the orchestrator and surfaces share one source of truth

  # use-case: UC-CONFIG-RESOLVE
  Scenario: Deployment config takes precedence
    Given a deployment config with ollama_model "llama3"
    When the runtime resolves configuration
    Then the effective ollama_model is "llama3"
    And the config source is "deployment"

  # use-case: UC-CONFIG-RESOLVE
  # variant: first-run-seed
  Scenario: First run seeds the deployment config from built-in defaults
    Given no deployment config exists
    And no project config exists
    When the runtime resolves configuration
    Then a deployment config file is created
    And the effective ollama_model is the built-in default
    And the config source is "seeded"

  # use-case: UC-CONFIG-RESOLVE
  # variant: markdown renderer defaults to glamour (ADR 0007)
  Scenario: The markdown renderer defaults to glamour
    Given no deployment config exists
    And no project config exists
    When the runtime resolves configuration
    Then the effective markdown renderer is "glamour"

  # use-case: UC-CONFIG-RESOLVE
  # variant: an explicit "scanner" opts out of the glamour default
  Scenario: An explicit scanner setting is honored
    Given a deployment config with markdown_renderer "scanner"
    When the runtime resolves configuration
    Then the effective markdown renderer is "scanner"

  # use-case: UC-CONFIG-RESOLVE
  # variant: project-fallback
  Scenario: First run falls back to project-local defaults
    Given no deployment config exists
    And a project config with ollama_model "mistral"
    When the runtime resolves configuration
    Then the effective ollama_model is "mistral"
    And the seeded deployment config contains ollama_model "mistral"
    And the config source is "seeded"
