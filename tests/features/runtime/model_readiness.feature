# Source contracts:
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-C4)
#   - docs/implementation/04_llm_prompt_tooling_runtime.md (Default Model Service)
#
# Behavior: the runtime reads the active model from config and verifies its
# readiness at startup, reporting a clear error when the model is unavailable so
# the surface is never launched against a dead model.

@integration @arch:runtime-bootstrap
Feature: Active-model readiness
  As the agentx runtime
  I want startup to verify the configured model is available
  So that an unavailable model is reported clearly instead of failing per prompt

  # use-case: UC-MODEL-READY
  Scenario: Configured model is available
    Given a started orchestrator with model "phi4-mini:3.8b" that is ready
    When the model is checked
    Then the model check passes

  # use-case: UC-MODEL-READY
  # variant: unavailable
  Scenario: Configured model is unavailable
    Given a started orchestrator with model "absent:7b" that is unavailable
    When the model is checked
    Then the model check fails clearly naming "absent:7b"
