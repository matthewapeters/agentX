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
