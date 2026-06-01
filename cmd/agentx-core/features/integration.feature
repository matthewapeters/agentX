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
    And tmux commands should include "select-window -t"

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

  Scenario: Context pane summaries preserve turn order with bounded formatting
    Given a temporary project directory
    And a core config with username "dev" and session "sess-9"
    And a fake tmux executable that records commands
    When I construct the AgentX core
    And I initialize the tmux session
    And I start the applet supervisor
    And I route input prompt "first context turn"
    Then prompt routing should complete without error
    When I route input prompt "this second prompt is intentionally long for bounded context pane formatting validation"
    Then prompt routing should complete without error
    And tmux commands should include "[context] turn=1"
    And tmux commands should include "[context] turn=2"
    And tmux command snippet "[context] turn=1" should appear before "[context] turn=2"
    And tmux commands should include "..."

  Scenario: Bridge lifecycle events reflect timeout fallback and subsequent recovery
    Given a temporary project directory
    And the project contains flaky chat bridge applet
    And a core config with username "dev" and session "sess-10"
    And a fake tmux executable that records commands
    When I construct the AgentX core
    And I configure core chat bridge to use prepared applet script with timeout 150 ms
    And I initialize the tmux session
    And I start the applet supervisor
    And I route input prompt "first timeout"
    Then prompt routing should complete without error
    And routed response should equal "Echo: first timeout"
    And tmux commands should include "[bridge] event=bridge_timeout"
    And tmux commands should include "[bridge] event=bridge_fallback"
    When I route input prompt "second recovery"
    Then prompt routing should complete without error
    And routed response should equal "Flaky recovered: second recovery"
    And tmux commands should include "[bridge] event=bridge_response_ok"
    And tmux command snippet "[bridge] event=bridge_timeout" should appear before "[bridge] event=bridge_fallback"
    And tmux command snippet "[bridge] event=bridge_fallback" should appear before "[bridge] event=bridge_response_ok"
    And tmux commands should include "[bridge] event=bridge_start" at least 2 times

  Scenario: Malformed bridge frames are tolerated without fallback
    Given a temporary project directory
    And the project contains malformed chat bridge applet
    And a core config with username "dev" and session "sess-11"
    And a fake tmux executable that records commands
    When I construct the AgentX core
    And I configure core chat bridge to use prepared applet script with timeout 300 ms
    And I initialize the tmux session
    And I start the applet supervisor
    And I route input prompt "malformed case"
    Then prompt routing should complete without error
    And routed response should equal "Malformed recovered: malformed case"
    And tmux commands should include "[bridge] event=bridge_response_ok"
    And tmux commands should not include "[bridge] event=bridge_fallback"

  Scenario: Error frame triggers fallback then recovery on next prompt
    Given a temporary project directory
    And the project contains error-frame chat bridge applet
    And a core config with username "dev" and session "sess-12"
    And a fake tmux executable that records commands
    When I construct the AgentX core
    And I configure core chat bridge to use prepared applet script with timeout 300 ms
    And I initialize the tmux session
    And I start the applet supervisor
    And I route input prompt "error first"
    Then prompt routing should complete without error
    And routed response should equal "Echo: error first"
    And tmux commands should include "[bridge] event=bridge_response_error"
    And tmux commands should include "[bridge] event=bridge_fallback"
    When I route input prompt "recover second"
    Then prompt routing should complete without error
    And routed response should equal "Error recovered: recover second"
    And tmux commands should include "[bridge] event=bridge_response_ok"
    And tmux command snippet "[bridge] event=bridge_response_error" should appear before "[bridge] event=bridge_fallback"
    And tmux command snippet "[bridge] event=bridge_fallback" should appear before "[bridge] event=bridge_response_ok"
    And tmux commands should include "[bridge] event=bridge_start" at least 2 times

  Scenario: Empty chunk frames do not emit stream chunk events
    Given a temporary project directory
    And the project contains empty-chunk chat bridge applet
    And a core config with username "dev" and session "sess-13"
    And a fake tmux executable that records commands
    When I construct the AgentX core
    And I configure core chat bridge to use prepared applet script with timeout 300 ms
    And I initialize the tmux session
    And I start the applet supervisor
    And I route input prompt "empty chunk integration"
    Then prompt routing should complete without error
    And routed response should equal "Empty recovered: empty chunk integration"
    And tmux commands should include "[bridge] event=bridge_response_ok"
    And tmux commands should not include "[bridge] event=bridge_chunk"
    And tmux commands should not include "[assistant-stream]"

  Scenario: Go chat runtime falls back directly when backend is unavailable
    Given a temporary project directory
    And I set chat runtime override to "go"
    And I set chat backend override to "ollama"
    And I set ollama host override to "127.0.0.1:1"
    And a core config with username "dev" and session "sess-14"
    And a fake tmux executable that records commands
    When I construct the AgentX core
    And I initialize the tmux session
    And I start the applet supervisor
    And I route input prompt "go runtime primary fallback"
    Then prompt routing should complete without error
    And routed response should equal "Echo: go runtime primary fallback"
    And tmux commands should include "[bridge] event=go_chat_fallback"
    And tmux commands should not include "[bridge] event=bridge_route_start"
    And tmux commands should not include "[bridge] event=go_chat_bridge_fallback"

  Scenario: Go chat backend delayed recovery transitions from direct fallback to direct success
    Given a temporary project directory
    And I set chat runtime override to "go"
    And I set chat backend override to "ollama"
    And I start a sequenced delayed ollama backend with delay 120 ms and statuses 503 then 200
    And a core config with username "dev" and session "sess-19"
    And a fake tmux executable that records commands
    When I construct the AgentX core
    And I initialize the tmux session
    And I start the applet supervisor
    And I route input prompt "delayed recovery first"
    Then prompt routing should complete without error
    And routed response should equal "Echo: delayed recovery first"
    And tmux commands should include "[bridge] event=go_chat_fallback"
    And tmux commands should not include "[bridge] event=bridge_route_start"
    When I route input prompt "delayed recovery second"
    Then prompt routing should complete without error
    And routed response should equal "Delayed backend recovery reply"
    And tmux commands should include "[bridge] event=go_chat_response_ok"
    And tmux command snippet "[bridge] event=go_chat_fallback" should appear before "[bridge] event=go_chat_response_ok"
