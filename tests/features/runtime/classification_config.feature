# Source contracts:
#   - docs/implementation/03_configuration_and_storage.md (Runtime tables)
#   - docs/implementation/04_llm_prompt_tooling_runtime.md (Classification Cycle)
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-D2)
#
# Behavior: the classify cycle and output widget read tunables from agentx.toml
# ([agentx.classification], [agentx.output]) with built-in defaults.

@integration @arch:config-resolution
Feature: Classification and output configuration
  As the agentx runtime
  I want classify/output tunables resolved from config with defaults
  So that retries, clarification options, and widget height are adjustable

  # use-case: UC-CLASSIFY-CONFIG
  Scenario: Defaults apply when unset
    Given a freshly resolved configuration
    Then the effective classification retries is 2
    And the effective clarification options is 3
    And the effective max widget lines is 20

  # use-case: UC-CLASSIFY-CONFIG
  # variant: override
  Scenario: Deployment config overrides the tunables
    Given a deployment config with classification retries 5 and max widget lines 30
    When the runtime resolves classification configuration
    Then the effective classification retries is 5
    And the effective max widget lines is 30
