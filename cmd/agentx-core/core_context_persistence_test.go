package main

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestTrimForPaneSummary_BoundsLength validates deterministic bounded summary formatting.
//
// GIVEN summary input values of varying lengths
// WHEN trimForPaneSummary is applied
// THEN values are preserved when short and truncated with ellipsis when long.
func TestTrimForPaneSummary_BoundsLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    string
		maxLen   int
		expected string
	}{
		{name: "empty", value: "", maxLen: 10, expected: ""},
		{name: "short unchanged", value: "hello", maxLen: 10, expected: "hello"},
		{name: "trim whitespace", value: "  hello  ", maxLen: 10, expected: "hello"},
		{name: "truncate", value: "abcdefghijklmnopqrstuvwxyz", maxLen: 10, expected: "abcdefg..."},
		{name: "small max keeps raw", value: "abcdef", maxLen: 3, expected: "abcdef"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := trimForPaneSummary(tc.value, tc.maxLen)
			if got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

// TestRouteInputPrompt_PersistsTurn validates that completed prompt/response exchanges are persisted.
//
// GIVEN an initialized core and applet supervisor
// WHEN a prompt is routed successfully
// THEN one chat turn is persisted and queryable via core state.
func TestRouteInputPrompt_PersistsTurn(t *testing.T) {
	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-turn"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}
	if _, err := core.RouteInputPrompt(context.Background(), "persist this"); err != nil {
		t.Fatalf("RouteInputPrompt failed: %v", err)
	}

	turns := core.ContextTurnsSnapshot()
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if turns[0].Prompt != "persist this" {
		t.Fatalf("expected prompt persist this, got %q", turns[0].Prompt)
	}
	if turns[0].Response != "Echo: persist this" {
		t.Fatalf("expected response Echo: persist this, got %q", turns[0].Response)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if !strings.Contains(commands, "send-keys -t "+core.tmuxSessionName+":0.1") {
		t.Fatalf("expected context pane render command, got:\n%s", commands)
	}
	if !strings.Contains(commands, "echo '[context] turn=1 prompt=\"persist this\" response=\"Echo: persist this\"' Enter") {
		t.Fatalf("expected context summary render command in tmux log, got:\n%s", commands)
	}
}

// TestContextTurnsSnapshot_PersistsAcrossCoreRestart validates persisted turn reload behavior.
//
// GIVEN persisted turn data in a session context directory
// WHEN a new core is constructed with the same session id
// THEN turns are loaded and available from snapshot state.
func TestContextTurnsSnapshot_PersistsAcrossCoreRestart(t *testing.T) {
	_ = setupFakeTmux(t)
	projectDir := t.TempDir()
	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-restart"}

	coreA := NewAgentXCore(cfg)
	if err := coreA.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := coreA.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}
	if _, err := coreA.RouteInputPrompt(context.Background(), "first turn"); err != nil {
		t.Fatalf("RouteInputPrompt failed: %v", err)
	}

	coreB := NewAgentXCore(cfg)
	turns := coreB.ContextTurnsSnapshot()
	if len(turns) != 1 {
		t.Fatalf("expected 1 persisted turn after restart, got %d", len(turns))
	}
	if !strings.Contains(turns[0].Response, "Echo: first turn") {
		t.Fatalf("expected persisted response to contain Echo: first turn, got %q", turns[0].Response)
	}
}

// TestRouteInputPrompt_ContextSummaryOrderingAndTruncation validates context pane turn summary fidelity.
//
// GIVEN two routed prompts where the second is intentionally long
// WHEN both turns are persisted
// THEN context pane summary lines preserve turn ordering and include bounded truncation.
func TestRouteInputPrompt_ContextSummaryOrderingAndTruncation(t *testing.T) {
	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-context-order"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	if _, err := core.RouteInputPrompt(context.Background(), "short one"); err != nil {
		t.Fatalf("first RouteInputPrompt failed: %v", err)
	}

	longPrompt := "this prompt is intentionally very long so that context pane summary formatting must truncate both prompt and response representations"
	if _, err := core.RouteInputPrompt(context.Background(), longPrompt); err != nil {
		t.Fatalf("second RouteInputPrompt failed: %v", err)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)

	firstMarker := "[context] turn=1"
	secondMarker := "[context] turn=2"
	firstIdx := strings.Index(commands, firstMarker)
	secondIdx := strings.Index(commands, secondMarker)
	if firstIdx == -1 || secondIdx == -1 {
		t.Fatalf("expected both context turn markers in tmux log, got:\n%s", commands)
	}
	if firstIdx >= secondIdx {
		t.Fatalf("expected turn=1 summary before turn=2 summary, got:\n%s", commands)
	}

	if !strings.Contains(commands, "...") {
		t.Fatalf("expected bounded summary truncation indicator in tmux log, got:\n%s", commands)
	}
}
