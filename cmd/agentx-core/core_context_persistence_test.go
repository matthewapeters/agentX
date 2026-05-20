package main

import (
	"context"
	"strings"
	"testing"
)

// TestRouteInputPrompt_PersistsTurn validates that completed prompt/response exchanges are persisted.
//
// GIVEN an initialized core and applet supervisor
// WHEN a prompt is routed successfully
// THEN one chat turn is persisted and queryable via core state.
func TestRouteInputPrompt_PersistsTurn(t *testing.T) {
	_ = setupFakeTmux(t)
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
