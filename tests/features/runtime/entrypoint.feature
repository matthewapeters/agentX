# Source contracts:
#   - docs/implementation/08_go_module_layout.md (internal/app composition)
#   - docs/build-plan/03_chat_surface_backlog.md (CHT-A6)
#
# Behavior: the entrypoint parses arguments, and the composition root resolves
# configuration into a started orchestrator and serves until shutdown.

@arch:entrypoint
Feature: Application composition and entrypoint
  As the agentx entrypoint
  I want to assemble the runtime from resolved configuration
  So that one command boots a working session

  # use-case: UC-CLI-PARSE
  @unit
  Scenario: Version flag is parsed
    When the arguments "--version" are parsed
    Then the parsed command requests version output

  # use-case: UC-APP-BUILD
  @integration
  Scenario: Build starts the orchestrator from resolved config
    Given app options with a temp config selecting model "testmodel"
    When the app builds the runtime
    Then the built orchestrator has an active session
    And the orchestrator active model is "testmodel"

  # use-case: UC-APP-RUN
  @integration
  Scenario: Run serves then shuts down on cancel
    Given app options with a temp config selecting model "testmodel"
    When the app runs until the context is canceled
    Then the run returns without error
