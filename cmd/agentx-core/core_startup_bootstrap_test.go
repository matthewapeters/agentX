package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeBootstrapPromptFile(t *testing.T, projectDir string, prompt string) {
	t.Helper()
	bootstrapDir := filepath.Join(projectDir, ".agentx")
	if err := os.MkdirAll(bootstrapDir, 0o755); err != nil {
		t.Fatalf("failed to create bootstrap directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bootstrapDir, "bootstrap-prompt.md"), []byte(prompt), 0o644); err != nil {
		t.Fatalf("failed to write bootstrap prompt: %v", err)
	}
}

func setupFakeTmuxLogsHeadless(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	scriptPath := filepath.Join(tmpDir, "tmux")
	script := "#!/usr/bin/env bash\n" +
		"set -euo pipefail\n" +
		"printf '%s\\n' \"$*\" >> \"${TMUX_LOG}\"\n" +
		"if [[ \"$1\" == \"send-keys\" && \"$*\" == *\":1.0\"* ]]; then\n" +
		"  exit 1\n" +
		"fi\n" +
		"if [[ \"$1\" == \"split-window\" ]]; then\n" +
		"  if [[ \"$*\" == *\" -v \"* ]]; then echo \"%3\"; else echo \"%4\"; fi\n" +
		"fi\n"

	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write fake tmux script: %v", err)
	}

	oldPath := os.Getenv("PATH")
	oldTmuxLog := os.Getenv("TMUX_LOG")
	oldPaneRenderMode := os.Getenv("AGENTX_PANE_RENDER_MODE")
	if err := os.Setenv("PATH", tmpDir+":"+oldPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	if err := os.Setenv("TMUX_LOG", logPath); err != nil {
		t.Fatalf("failed to set TMUX_LOG: %v", err)
	}
	if err := os.Setenv("AGENTX_PANE_RENDER_MODE", "core"); err != nil {
		t.Fatalf("failed to set AGENTX_PANE_RENDER_MODE: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
		_ = os.Setenv("TMUX_LOG", oldTmuxLog)
		_ = os.Setenv("AGENTX_PANE_RENDER_MODE", oldPaneRenderMode)
	})

	return logPath
}

func TestRunStartupBootstrap_OneShotPerSession(t *testing.T) {
	logPath := setupFakeTmux(t)
	projectDir := t.TempDir()
	writeBootstrapPromptFile(t, projectDir, "Introduce yourself in one sentence.")

	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-bootstrap-once"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}
	if err := core.RunStartupBootstrap(context.Background()); err != nil {
		t.Fatalf("first RunStartupBootstrap failed: %v", err)
	}
	if err := core.RunStartupBootstrap(context.Background()); err != nil {
		t.Fatalf("second RunStartupBootstrap failed: %v", err)
	}

	turns := core.ContextTurnsSnapshot()
	if len(turns) != 1 {
		t.Fatalf("expected one persisted bootstrap turn, got %d", len(turns))
	}
	if strings.TrimSpace(turns[0].Prompt) != "Introduce yourself in one sentence." {
		t.Fatalf("unexpected bootstrap prompt persisted: %q", turns[0].Prompt)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if count := strings.Count(commands, "[startup] bootstrap complete response_chars="); count != 1 {
		t.Fatalf("expected one bootstrap completion status log, got %d\ncommands:\n%s", count, commands)
	}
	if !strings.Contains(commands, "[startup] bootstrap skipped: existing session turns") {
		t.Fatalf("expected bootstrap skip status after second run, commands:\n%s", commands)
	}
}

func TestRunStartupBootstrap_LogsPaneUnavailableStillPersistsTurn(t *testing.T) {
	setupFakeTmuxLogsHeadless(t)
	projectDir := t.TempDir()
	writeBootstrapPromptFile(t, projectDir, "Introduce yourself for logs fallback test.")

	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-bootstrap-headless-logs"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}
	if err := core.RunStartupBootstrap(context.Background()); err != nil {
		t.Fatalf("RunStartupBootstrap failed when logs pane was unavailable: %v", err)
	}

	turns := core.ContextTurnsSnapshot()
	if len(turns) != 1 {
		t.Fatalf("expected one persisted bootstrap turn even when logs pane unavailable, got %d", len(turns))
	}
}

func TestRunStartupBootstrap_UsesGoChatPathWithMockedBackend(t *testing.T) {
	t.Setenv("AGENTX_CHAT_RUNTIME", "go")
	t.Setenv("AGENTX_CHAT_BACKEND", "mock")

	logPath := setupFakeTmux(t)
	projectDir := t.TempDir()
	stageTemplateApplet(t, projectDir)
	writeBootstrapPromptFile(t, projectDir, "startup integration prompt")

	cfg := &Config{ProjectDir: projectDir, Username: "tester", SessionID: "s-bootstrap-bridge"}
	core := NewAgentXCore(cfg)

	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("InitializeTmuxSession failed: %v", err)
	}
	if err := core.StartAppletSupervisor(context.Background()); err != nil {
		t.Fatalf("StartAppletSupervisor failed: %v", err)
	}
	if err := core.RunStartupBootstrap(context.Background()); err != nil {
		t.Fatalf("RunStartupBootstrap failed: %v", err)
	}

	turns := core.ContextTurnsSnapshot()
	if len(turns) != 1 {
		t.Fatalf("expected one persisted bootstrap turn, got %d", len(turns))
	}
	if turns[0].Prompt != "startup integration prompt" {
		t.Fatalf("expected persisted bootstrap prompt, got %q", turns[0].Prompt)
	}
	if turns[0].Response != "Echo: startup integration prompt" {
		t.Fatalf("expected mocked go-chat echo response, got %q", turns[0].Response)
	}

	commandsRaw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed reading tmux command log: %v", err)
	}
	commands := string(commandsRaw)
	if !strings.Contains(commands, "event=go_chat_response_ok") {
		t.Fatalf("expected go chat response ok lifecycle in tmux logs, got:\n%s", commands)
	}
}
