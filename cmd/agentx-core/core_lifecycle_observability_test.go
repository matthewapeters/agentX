package main

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestStartAppletSupervisor_EmitsStartupLifecycleOnce validates startup lifecycle hook semantics.
//
// GIVEN an initialized core with fake tmux logging
// WHEN StartAppletSupervisor is called more than once
// THEN startup greeting lifecycle event is emitted exactly once.
func TestStartAppletSupervisor_EmitsStartupLifecycleOnce(t *testing.T) {
	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-lifecycle-startup"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("first StartAppletSupervisor failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("second StartAppletSupervisor failed: %v", err)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if count := strings.Count(commands, "stage=startup_greeting"); count != 1 {
		t.Fatalf("expected exactly one startup lifecycle event, got %d\ncommands:\n%s", count, commands)
	}
}

func TestStartAppletSupervisor_DoesNotEmitRuntimeContractSignal(t *testing.T) {
	logPath := setupFakeTmux(t)
	t.Setenv("AGENTX_CHAT_RUNTIME", "python")

	cfg := &Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-lifecycle-runtime-contract"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)

	if strings.Contains(commands, "stage=runtime_contract") {
		t.Fatalf("expected no runtime contract lifecycle signal after chat runtime normalization, commands:\n%s", commands)
	}
}

// TestRouteInputPrompt_EmitsLifecycleStagesInOrder validates deterministic lifecycle stage ordering.
//
// GIVEN an initialized core and applet supervisor
// WHEN a prompt is routed through RouteInputPrompt
// THEN lifecycle events are emitted in canonical deterministic order.
func TestRouteInputPrompt_EmitsLifecycleStagesInOrder(t *testing.T) {
	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-lifecycle-order"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}
	if _, err := core.RouteInputPrompt(context.Background(), "list the files here"); err != nil {
		t.Fatalf("RouteInputPrompt failed: %v", err)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)

	orderedStages := []string{
		"stage=submitted",
		"stage=classified",
		"stage=thinking",
		"stage=tool",
		"stage=final_response",
	}

	lastIndex := -1
	for _, marker := range orderedStages {
		idx := strings.Index(commands, marker)
		if idx == -1 {
			t.Fatalf("expected lifecycle marker %q in commands:\n%s", marker, commands)
		}
		if idx <= lastIndex {
			t.Fatalf("expected marker %q after previous lifecycle marker; commands:\n%s", marker, commands)
		}
		lastIndex = idx
	}
}
