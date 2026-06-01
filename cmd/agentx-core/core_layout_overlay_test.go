package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyOptionalLayoutOverlay_MissingLayoutFileIsNonFatal(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "sess-1", LayoutFile: filepath.Join(t.TempDir(), "missing.yaml")}
	core := NewAgentXCore(cfg)
	if err := core.applyOptionalLayoutOverlay(context.Background()); err != nil {
		t.Fatalf("expected missing layout file to be non-fatal, got %v", err)
	}
}

func TestApplyOptionalLayoutOverlay_UsesTmuxpAndReassertsOwnedWindows(t *testing.T) {
	tmpDir := t.TempDir()
	layoutFile := filepath.Join(tmpDir, "layout.yaml")
	if err := os.WriteFile(layoutFile, []byte("session_name: ${SESSION}\n"), 0o644); err != nil {
		t.Fatalf("failed to write layout file: %v", err)
	}

	tmuxLog := filepath.Join(tmpDir, "tmux.log")
	tmuxpLog := filepath.Join(tmpDir, "tmuxp.log")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	fakeTmuxp := filepath.Join(tmpDir, "tmuxp")

	fakeTmuxScript := "#!/usr/bin/env bash\n" +
		"set -euo pipefail\n" +
		"printf '%s\\n' \"$*\" >> \"${TMUX_LOG}\"\n"
	if err := os.WriteFile(fakeTmux, []byte(fakeTmuxScript), 0o755); err != nil {
		t.Fatalf("failed to write fake tmux script: %v", err)
	}

	fakeTmuxpScript := "#!/usr/bin/env bash\n" +
		"set -euo pipefail\n" +
		"printf 'ARGS:%s\\n' \"$*\" >> \"${TMUXP_LOG}\"\n" +
		"printf 'SESSION:%s\\n' \"${SESSION:-}\" >> \"${TMUXP_LOG}\"\n"
	if err := os.WriteFile(fakeTmuxp, []byte(fakeTmuxpScript), 0o755); err != nil {
		t.Fatalf("failed to write fake tmuxp script: %v", err)
	}

	oldPath := os.Getenv("PATH")
	oldTmuxLog := os.Getenv("TMUX_LOG")
	oldTmuxpLog := os.Getenv("TMUXP_LOG")
	if err := os.Setenv("PATH", tmpDir+":"+oldPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	if err := os.Setenv("TMUX_LOG", tmuxLog); err != nil {
		t.Fatalf("failed to set TMUX_LOG: %v", err)
	}
	if err := os.Setenv("TMUXP_LOG", tmuxpLog); err != nil {
		t.Fatalf("failed to set TMUXP_LOG: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
		_ = os.Setenv("TMUX_LOG", oldTmuxLog)
		_ = os.Setenv("TMUXP_LOG", oldTmuxpLog)
	})

	cfg := &Config{ProjectDir: tmpDir, Username: "tester", SessionID: "sess-1", LayoutFile: layoutFile}
	core := NewAgentXCore(cfg)

	if err := core.applyOptionalLayoutOverlay(context.Background()); err != nil {
		t.Fatalf("applyOptionalLayoutOverlay failed: %v", err)
	}

	tmuxpData, err := os.ReadFile(tmuxpLog)
	if err != nil {
		t.Fatalf("failed to read tmuxp log: %v", err)
	}
	tmuxpOutput := string(tmuxpData)
	if !strings.Contains(tmuxpOutput, "ARGS:load -y -d "+layoutFile) {
		t.Fatalf("expected tmuxp load invocation, got:\n%s", tmuxpOutput)
	}
	if !strings.Contains(tmuxpOutput, "SESSION:"+core.tmuxSessionName) {
		t.Fatalf("expected SESSION env to be passed to tmuxp, got:\n%s", tmuxpOutput)
	}

	tmuxData, err := os.ReadFile(tmuxLog)
	if err != nil {
		t.Fatalf("failed to read tmux log: %v", err)
	}
	commands := string(tmuxData)
	if !strings.Contains(commands, "rename-window -t "+core.tmuxSessionName+":0 "+tmuxPrimaryWindow) {
		t.Fatalf("expected primary window rename assertion, got:\n%s", commands)
	}
	if !strings.Contains(commands, "rename-window -t "+core.tmuxSessionName+":1 "+tmuxLogsWindow) {
		t.Fatalf("expected logs window rename assertion, got:\n%s", commands)
	}
}
