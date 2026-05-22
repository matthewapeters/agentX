@e2e
Feature: End-to-end hermetic shutdown path
  As an AgentX core maintainer
  I want graceful shutdown to complete even when no applets are running
  So that process termination remains robust during migration

  Scenario: Shutdown succeeds with empty applet registry
    Given a temporary project directory
    And a core config with username "e2e" and session "sess-e2e"
    And a constructed AgentX core
    When I invoke core shutdown
    Then shutdown should complete without error

  Scenario: Prompt routing updates response and persisted context in one flow
    Given a temporary project directory
    And a core config with username "e2e" and session "sess-e2e-flow"
    And a fake tmux executable that records commands
    When I construct the AgentX core
    And I initialize the tmux session
    And I start the applet supervisor
    And I route input prompt "what is 2+2?"
    Then prompt routing should complete without error
    And routed response should equal "Echo: what is 2+2?"
    And tmux should include rendered chat response "Echo: what is 2+2?"
    And tmux commands should include "[context] turn=1"
    When I capture the context turns snapshot
    And context turns should have length 1
    And context turns should include prompt "what is 2+2?"
    And context turns should include response "Echo: what is 2+2?"
