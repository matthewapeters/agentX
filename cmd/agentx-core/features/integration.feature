@integration
Feature: IPC router integration
  As an AgentX core maintainer
  I want IPC FIFO paths to be provisioned hermetically
  So that Python applets can communicate with core reliably

  Scenario: FIFO pair is created for an applet
    Given a temporary project directory
    And an IPC router for session "sess-1"
    When I create an IPC FIFO pair for applet "chat"
    Then both FIFO files should exist
    And FIFO paths should include applet name "chat"

  Scenario: Core startup issues deterministic tmux bootstrap commands
    Given a temporary project directory
    And a core config with username "dev" and session "sess-1"
    And a fake tmux executable that records commands
    When I construct the AgentX core
    And I initialize the tmux session
    Then tmux initialization should complete without error
    And tmux commands should include "new-window -t"
    And tmux commands should include "send-keys -t"

  Scenario: Core startup names primary window as tui-chat
    Given a temporary project directory
    And a core config with username "dev" and session "sess-2"
    And a fake tmux executable that records commands
    When I construct the AgentX core
    And I initialize the tmux session
    Then startup should name window 0 as "tui-chat"

  Scenario: Core startup re-selects primary window after creating logs
    Given a temporary project directory
    And a core config with username "dev" and session "sess-3"
    And a fake tmux executable that records commands
    When I construct the AgentX core
    And I initialize the tmux session
    Then startup should select window 0
