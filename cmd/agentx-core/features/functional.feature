@functional
Feature: Core lifecycle behavior without external services
  As an AgentX core maintainer
  I want lifecycle methods to be safe in hermetic mode
  So that local development and CI can run without tmux dependencies

  Scenario: Core initialization sets session identity
    Given a temporary project directory
    And a core config with username "dev" and session ""
    When I construct the AgentX core
    Then the core session id should be non-empty
    And the core tmux session name should include username "dev"

  Scenario: Health endpoint routine exits on canceled context
    Given a temporary project directory
    Given a context manager with a temporary context directory
    And a canceled context
    When I run the health serve routine
    Then the routine should return context canceled
