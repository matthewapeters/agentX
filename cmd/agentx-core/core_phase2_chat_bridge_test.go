package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// GIVEN a project with Python template applet available
// WHEN a prompt is routed through the chat handler
// THEN the handler uses the Python bridge and returns a deterministic response.
func TestRouteInputPrompt_UsesPythonBridgeTemplate(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available in test environment")
	}

	templatePath := filepath.Join("..", "..", "applets", "template.py")
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("failed to read template applet: %v", err)
	}

	projectDir := t.TempDir()
	appletsDir := filepath.Join(projectDir, "applets")
	if err := os.MkdirAll(appletsDir, 0o755); err != nil {
		t.Fatalf("failed to create applets dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appletsDir, "template.py"), templateContent, 0o755); err != nil {
		t.Fatalf("failed to write template applet: %v", err)
	}

	logPath := setupFakeTmux(t)
	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-phase2"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}

	response, err := core.RouteInputPrompt(context.Background(), "phase2 bridge")
	if err != nil {
		t.Fatalf("RouteInputPrompt failed: %v", err)
	}
	if response != "Echo: phase2 bridge" {
		t.Fatalf("expected python bridge response, got %q", response)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	if len(commandsRaw) == 0 {
		t.Fatal("expected tmux commands to be recorded")
	}
}
