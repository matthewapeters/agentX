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

  # use-case: UC-CLI-SESSION
  # A --session name lets the user boot predictable session names for scripted
  # multiplexer layouts (zellij/tmux), instead of the generated adjective-noun.
  @unit
  Scenario: Session name flag is parsed
    When the arguments "--session my-layout" are parsed
    Then the parsed session name is "my-layout"

  # use-case: UC-CLI-SESSION-NEWNAME
  # `agentx session new-name` prints one session name in AgentX's own style and
  # exits, so a scripted launcher can pre-mint a name instead of rolling its own.
  @unit
  Scenario: Session new-name subcommand is parsed
    When the arguments "session new-name" are parsed
    Then the parsed command requests a generated session name

  # use-case: UC-APP-SESSION-NAME
  @integration
  Scenario: A provided session name names the session
    Given app options with a temp config selecting model "testmodel"
    And the session name option is "top-pane"
    When the app builds the runtime
    Then the active session name is "top-pane"

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
