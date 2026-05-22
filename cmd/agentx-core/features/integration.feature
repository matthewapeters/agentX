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

  Scenario: Input prompt routes through chat applet and renders response
    Given a temporary project directory
    And a core config with username "dev" and session "sess-4"
    And a fake tmux executable that records commands
    When I construct the AgentX core
    And I initialize the tmux session
    And I start the applet supervisor
    And I route input prompt "hello from input"
    Then prompt routing should complete without error
    And routed response should equal "Echo: hello from input"
    And tmux should include rendered chat response "Echo: hello from input"

  Scenario: Input command contract handles clear, quit, and prompt forwarding
    Given a temporary project directory
    And a core config with username "dev" and session "sess-5"
    And a fake tmux executable that records commands
    When I construct the AgentX core
    And I initialize the tmux session
    And I start the applet supervisor
    And I handle input line ":clear"
    Then input handling should complete without error
    And input response should equal "cleared"
    And input exit flag should be false
    And tmux commands should include "send-keys -t"
    When I handle input line ":q"
    Then input handling should complete without error
    And input response should equal "quit"
    And input exit flag should be true
    When I handle input line "hello command contract"
    Then input handling should complete without error
    And input response should equal "Echo: hello command contract"
    And tmux should include rendered chat response "Echo: hello command contract"

  Scenario: Persisted turns survive core reconstruction
    Given a temporary project directory
    And a core config with username "dev" and session "sess-6"
    And a fake tmux executable that records commands
    When I construct the AgentX core
    And I initialize the tmux session
    And I start the applet supervisor
    And I route input prompt "persist across restart"
    Then prompt routing should complete without error
    When I reconstruct the AgentX core with the same config
    And I capture the context turns snapshot
    Then context turns should have length 1
    And context turns should include prompt "persist across restart"

  Scenario: Python chat bridge process is reused across routed prompts
    Given a temporary project directory
    And the project contains template chat applet
    And a core config with username "dev" and session "sess-7"
    And a fake tmux executable that records commands
    When I construct the AgentX core
    And I initialize the tmux session
    And I start the applet supervisor
    And I route input prompt "bridge prompt one"
    Then prompt routing should complete without error
    And routed response should equal "Echo: bridge prompt one"
    And I capture the tracked chat applet process pid
    When I route input prompt "bridge prompt two"
    Then prompt routing should complete without error
    And routed response should equal "Echo: bridge prompt two"
    And the tracked chat applet process pid should remain the same

  Scenario: Python bridge streaming renders chunks and persists final turn
    Given a temporary project directory
    And the project contains template chat applet
    And a core config with username "dev" and session "sess-8"
    And a fake tmux executable that records commands
    When I construct the AgentX core
    And I initialize the tmux session
    And I start the applet supervisor
    And I route input prompt "streaming godog prompt"
    Then prompt routing should complete without error
    And routed response should equal "Echo: streaming godog prompt"
    And tmux commands should include "[assistant-stream]"
    And tmux should include rendered chat response "Echo: streaming godog prompt"
    When I capture the context turns snapshot
    Then context turns should have length 1
    And context turns should include prompt "streaming godog prompt"
    And context turns should include response "Echo: streaming godog prompt"
