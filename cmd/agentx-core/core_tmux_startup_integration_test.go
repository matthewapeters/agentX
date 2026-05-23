package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// GIVEN core startup initializes tmux in a hermetic environment
// WHEN tmux commands are captured via a fake tmux executable
// THEN startup should name/select the primary chat window before logs.
func TestInitializeTmuxSession_PrimaryWindowSelectionRegression(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	fakeTmuxPath := filepath.Join(tmpDir, "tmux")
	fakeTmuxScript := "#!/usr/bin/env bash\n" +
		"set -euo pipefail\n" +
		"printf '%s\\n' \"$*\" >> \"${TMUX_LOG}\"\n" +
		"if [[ \"$1\" == \"split-window\" ]]; then\n" +
		"  if [[ \"$*\" == *\" -v \"* ]]; then echo \"%3\"; else echo \"%4\"; fi\n" +
		"fi\n"
	if err := os.WriteFile(fakeTmuxPath, []byte(fakeTmuxScript), 0o755); err != nil {
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

	cfg := &Config{ProjectDir: tmpDir, Username: "tester", SessionID: "sess1"}
	core := NewAgentXCore(cfg)
	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("failed to initialize tmux session: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read tmux command log: %v", err)
	}
	commands := string(data)

	if !strings.Contains(commands, "new-session -d -s "+core.tmuxSessionName+" -n tui-chat") {
		t.Fatalf("expected startup to name primary window as tui-chat, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "select-window -t "+core.tmuxSessionName+":0") {
		t.Fatalf("expected startup to re-select window 0 after logs creation, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "select-pane -t %3") {
		t.Fatalf("expected startup to focus input pane, commands:\n%s", commands)
	}
}
