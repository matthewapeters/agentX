package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupFakeTmux(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	scriptPath := filepath.Join(tmpDir, "tmux")
	script := "#!/usr/bin/env bash\n" +
		"set -euo pipefail\n" +
		"printf '%s\\n' \"$*\" >> \"${TMUX_LOG}\"\n" +
		"if [[ \"$1\" == \"split-window\" ]]; then\n" +
		"  if [[ \"$*\" == *\" -v \"* ]]; then echo \"%3\"; else echo \"%4\"; fi\n" +
		"fi\n"

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write fake tmux script: %v", err)
	}

	oldPath := os.Getenv("PATH")
	oldTmuxLog := os.Getenv("TMUX_LOG")
	if err := os.Setenv("PATH", tmpDir+":"+oldPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	if err := os.Setenv("TMUX_LOG", logPath); err != nil {
		t.Fatalf("failed to set TMUX_LOG: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
		_ = os.Setenv("TMUX_LOG", oldTmuxLog)
	})

	return logPath
}

// TestRouteInputPrompt_RendersChatResponse validates Sprint B1 deterministic prompt routing.
//
// GIVEN initialized tmux core and applet supervisor
// WHEN an input prompt is routed through chat applet
// THEN a deterministic response is rendered to the chat pane.
func TestRouteInputPrompt_RendersChatResponse(t *testing.T) {
	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-chat"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	response, err := core.RouteInputPrompt(context.Background(), "hello from input")
	if err != nil {
		t.Fatalf("RouteInputPrompt failed: %v", err)
	}
	if response != "Echo: hello from input" {
		t.Fatalf("expected deterministic response, got %q", response)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if !strings.Contains(commands, "send-keys -t "+core.tmuxSessionName+":0.0") {
		t.Fatalf("expected chat pane render command, got:\n%s", commands)
	}
	if !strings.Contains(commands, "echo '[assistant] Echo: hello from input' Enter") {
		t.Fatalf("expected rendered response command in tmux log, got:\n%s", commands)
	}
}

// TestRouteInputPrompt_WithoutChatAppletFails validates actionable failure path logging and error return.
//
// GIVEN a core without registered chat applet
// WHEN an input prompt is routed
// THEN routing fails with a deterministic error.
func TestRouteInputPrompt_WithoutChatAppletFails(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "s-no-chat"}
	core := NewAgentXCore(cfg)

	_, err := core.RouteInputPrompt(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error when chat applet is missing")
	}
	if !strings.Contains(err.Error(), "chat applet is not registered") {
		t.Fatalf("expected chat applet registration error, got %v", err)
	}
}
