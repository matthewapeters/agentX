package main

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestHandleInputLine_ClearCommand validates the :clear command dispatch behavior.
//
// GIVEN an initialized core with fake tmux and applet supervisor
// WHEN the input line is :clear
// THEN live-core chat/input panes are cleared via tmux control sequences and no prompt routing occurs.
func TestHandleInputLine_ClearCommand(t *testing.T) {
	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-clear"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	resp, shouldExit, err := core.HandleInputLine(context.Background(), ":clear")
	if err != nil {
		t.Fatalf("HandleInputLine returned error: %v", err)
	}
	if shouldExit {
		t.Fatal("expected shouldExit false for :clear")
	}
	if resp != "cleared" {
		t.Fatalf("expected cleared response, got %q", resp)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if !strings.Contains(commands, "clear-history -t "+core.tmuxSessionName+":0.0") {
		t.Fatalf("expected chat pane history clear command in tmux log, got:\n%s", commands)
	}
	if !strings.Contains(commands, "send-keys -R -t "+core.tmuxSessionName+":0.0") {
		t.Fatalf("expected chat pane display reset command in tmux log, got:\n%s", commands)
	}
	if !strings.Contains(commands, "clear-history -t %3") && !strings.Contains(commands, "clear-history -t "+core.tmuxSessionName+":0.2") {
		t.Fatalf("expected input pane history clear command in tmux log, got:\n%s", commands)
	}
	if !strings.Contains(commands, "send-keys -t %3 C-u") && !strings.Contains(commands, "send-keys -t "+core.tmuxSessionName+":0.2 C-u") {
		t.Fatalf("expected input line reset command in tmux log, got:\n%s", commands)
	}
	if !strings.Contains(commands, "send-keys -R -t %3") && !strings.Contains(commands, "send-keys -R -t "+core.tmuxSessionName+":0.2") {
		t.Fatalf("expected input pane display reset command in tmux log, got:\n%s", commands)
	}
	if strings.Contains(commands, "send-keys -t "+core.tmuxSessionName+":0.0 clear Enter") {
		t.Fatalf("did not expect literal clear text to be sent to chat pane, got:\n%s", commands)
	}
	if strings.Contains(commands, "[assistant] Echo:") {
		t.Fatalf("did not expect chat routing echo for :clear, got:\n%s", commands)
	}
}

// TestHandleInputLine_QuitCommand validates the :q command contract.
//
// GIVEN an initialized core
// WHEN the input line is :q
// THEN shouldExit is true and no prompt routing occurs.
func TestHandleInputLine_QuitCommand(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-quit"}
	core := NewAgentXCore(cfg)

	resp, shouldExit, err := core.HandleInputLine(context.Background(), ":q")
	if err != nil {
		t.Fatalf("HandleInputLine returned error: %v", err)
	}
	if !shouldExit {
		t.Fatal("expected shouldExit true for :q")
	}
	if resp != "quit" {
		t.Fatalf("expected quit response, got %q", resp)
	}
}

// TestHandleInputLine_NormalPromptForwarding validates normal prompt forwarding remains intact.
//
// GIVEN initialized core with applet supervisor and fake tmux
// WHEN the input line is a normal prompt
// THEN prompt is forwarded through RouteInputPrompt and rendered in chat pane.
func TestHandleInputLine_NormalPromptForwarding(t *testing.T) {
	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-forward"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	resp, shouldExit, err := core.HandleInputLine(context.Background(), "hello b2")
	if err != nil {
		t.Fatalf("HandleInputLine returned error: %v", err)
	}
	if shouldExit {
		t.Fatal("expected shouldExit false for normal prompt")
	}
	if resp != "Echo: hello b2" {
		t.Fatalf("expected forwarded echo response, got %q", resp)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	if !strings.Contains(string(commandsRaw), "echo '[assistant] Echo: hello b2' Enter") {
		t.Fatalf("expected rendered forwarded response in tmux log, got:\n%s", string(commandsRaw))
	}
}

// TestHandleInputLine_HistoryDeterministic validates stable history ordering.
//
// GIVEN mixed commands and prompts
// WHEN lines are handled in sequence
// THEN history snapshot preserves deterministic order of non-empty inputs.
func TestHandleInputLine_HistoryDeterministic(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-hist"}
	core := NewAgentXCore(cfg)

	_, _, _ = core.HandleInputLine(context.Background(), "first")
	_, _, _ = core.HandleInputLine(context.Background(), ":unknown")
	_, _, _ = core.HandleInputLine(context.Background(), "  ")
	_, _, _ = core.HandleInputLine(context.Background(), ":q")

	history := core.InputHistorySnapshot()
	expected := []string{"first", ":unknown", ":q"}
	if len(history) != len(expected) {
		t.Fatalf("expected history size %d, got %d", len(expected), len(history))
	}
	for i := range expected {
		if history[i] != expected[i] {
			t.Fatalf("expected history[%d]=%q, got %q", i, expected[i], history[i])
		}
	}
}

// TestHandleInputLine_UnsupportedCommand validates unsupported command errors.
//
// GIVEN an unsupported command token
// WHEN the command is handled
// THEN an actionable unsupported command error is returned.
func TestHandleInputLine_UnsupportedCommand(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-unknown"}
	core := NewAgentXCore(cfg)

	_, _, err := core.HandleInputLine(context.Background(), ":unknown")
	if err == nil {
		t.Fatal("expected unsupported command error")
	}
	if !strings.Contains(err.Error(), "unsupported command") {
		t.Fatalf("expected unsupported command error, got %v", err)
	}
}

// TestHandleInputLine_ForwardingErrorPropagates validates forwarding failures bubble up.
//
// GIVEN a core with missing chat applet
// WHEN handling a normal prompt
// THEN the routing error is propagated unchanged.
func TestHandleInputLine_ForwardingErrorPropagates(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-prop"}
	core := NewAgentXCore(cfg)

	_, _, err := core.HandleInputLine(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected forwarding error")
	}
	if !strings.Contains(err.Error(), "chat applet is not registered") {
		t.Fatalf("expected chat applet registration error, got %v", err)
	}
}
