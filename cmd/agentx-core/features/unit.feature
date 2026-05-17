@unit
Feature: Core configuration and layout primitives
  As an AgentX core maintainer
  I want deterministic config and pane layout behavior
  So that foundational units remain stable and hermetic

  Scenario: Session directories are created for a given user and session
    Given a temporary project directory
    And a config with username "tester"
    When I ensure session directories for session "s1"
    Then the session directory structure should exist

  Scenario: Default pane layout includes expected pane names
    Given the default pane layout
    Then pane names should include "chat"
    And pane names should include "logs"
    And pane names should include "input"
    And pane names should include "context"
