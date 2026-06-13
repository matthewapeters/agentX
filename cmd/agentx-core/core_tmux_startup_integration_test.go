package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
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
	if !strings.Contains(commands, "new-window -t "+core.tmuxSessionName+":1 -n "+tmuxLogsWindow) {
		t.Fatalf("expected startup to create logs window with deterministic name, commands:\n%s", commands)
	}
	for idx, tab := range []string{"files", "configuration", "context-history", "working-memory"} {
		expected := "new-window -t " + core.tmuxSessionName + ":" + strconv.Itoa(2+idx) + " -n " + tab
		if !strings.Contains(commands, expected) {
			t.Fatalf("expected dedicated applet window %q, commands:\n%s", expected, commands)
		}
	}
	if !strings.Contains(commands, "select-window -t "+core.tmuxSessionName+":0") {
		t.Fatalf("expected startup to re-select window 0 after logs creation, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "select-pane -t %3") {
		t.Fatalf("expected startup to focus input pane, commands:\n%s", commands)
	}
}

// GIVEN startup mode is visible-windows
// WHEN tmux initialization runs
// THEN core creates one visible window per applet and avoids split-pane topology.
func TestInitializeTmuxSession_VisibleWindowsStartupMode(t *testing.T) {
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

	cfg := &Config{ProjectDir: tmpDir, Username: "tester", SessionID: "sess-visible", StartupMode: visibleWindowsStartupMode}
	core := NewAgentXCore(cfg)
	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("failed to initialize tmux session: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read tmux command log: %v", err)
	}
	commands := string(data)

	if strings.Contains(commands, "split-window") {
		t.Fatalf("visible-windows mode must not use split-window layout, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "new-window -t "+core.tmuxSessionName+":1 -n "+tmuxLogsWindow) {
		t.Fatalf("expected logs window in visible-windows mode, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "new-window -t "+core.tmuxSessionName+":2 -n "+PaneTitleInput) {
		t.Fatalf("expected input window in visible-windows mode, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "new-window -t "+core.tmuxSessionName+":3 -n "+PaneTitleSystem) {
		t.Fatalf("expected system window in visible-windows mode, commands:\n%s", commands)
	}
	for idx, tab := range []string{"files", "configuration", "context-history", "working-memory"} {
		expected := "new-window -t " + core.tmuxSessionName + ":" + strconv.Itoa(4+idx) + " -n " + tab
		if !strings.Contains(commands, expected) {
			t.Fatalf("expected dedicated applet window %q in visible-windows mode, commands:\n%s", expected, commands)
		}
	}
	if !strings.Contains(commands, "select-window -t "+core.tmuxSessionName+":2") {
		t.Fatalf("expected input window selection in visible-windows mode, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "select-pane -t "+core.tmuxSessionName+":2.0") {
		t.Fatalf("expected input pane focus in visible-windows mode, commands:\n%s", commands)
	}

	if got := core.paneTargetForName(PaneTitleOutput); got != core.tmuxSessionName+":0.0" {
		t.Fatalf("unexpected output pane target: got %q", got)
	}
	if got := core.paneTargetForName(PaneTitleLogs); got != core.tmuxSessionName+":1.0" {
		t.Fatalf("unexpected logs pane target: got %q", got)
	}
	if got := core.paneTargetForName(PaneTitleInput); got != core.tmuxSessionName+":2.0" {
		t.Fatalf("unexpected input pane target: got %q", got)
	}
	if got := core.paneTargetForName(PaneTitleSystem); got != core.tmuxSessionName+":3.0" {
		t.Fatalf("unexpected system pane target: got %q", got)
	}
}

// GIVEN startup probes pane dimensions during default-layout bootstrap
// WHEN tmux reports cramped input/system panes
// THEN core enforces minimum input height and system width before applets launch.
func TestInitializeTmuxSession_DefaultLayoutEnforcesPaneMinimums(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux.log")
	fakeTmuxPath := filepath.Join(tmpDir, "tmux")
	fakeTmuxScript := "#!/usr/bin/env bash\n" +
		"set -euo pipefail\n" +
		"printf '%s\\n' \"$*\" >> \"${TMUX_LOG}\"\n" +
		"if [[ \"$1\" == \"split-window\" ]]; then\n" +
		"  if [[ \"$*\" == *\" -v \"* ]]; then echo \"%3\"; else echo \"%4\"; fi\n" +
		"  exit 0\n" +
		"fi\n" +
		"if [[ \"$1\" == \"display-message\" ]]; then\n" +
		"  if [[ \"$*\" == *\"-t %3\"* && \"$*\" == *\"#{pane_height}|#{pane_width}\"* ]]; then echo \"3|96\"; exit 0; fi\n" +
		"  if [[ \"$*\" == *\"-t %4\"* && \"$*\" == *\"#{pane_height}|#{pane_width}\"* ]]; then echo \"28|18\"; exit 0; fi\n" +
		"  exit 0\n" +
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

	cfg := &Config{ProjectDir: tmpDir, Username: "tester", SessionID: "sess-minimums"}
	core := NewAgentXCore(cfg)
	if err := core.InitializeTmuxSession(context.Background()); err != nil {
		t.Fatalf("failed to initialize tmux session: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read tmux command log: %v", err)
	}
	commands := string(data)

	if !strings.Contains(commands, "set-window-option -t "+core.tmuxSessionName+":0 window-size smallest") {
		t.Fatalf("expected startup window-size normalization command, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "display-message -p -t %3 #{pane_height}|#{pane_width}") {
		t.Fatalf("expected input pane dimension probe, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "resize-pane -t %3 -y 6") {
		t.Fatalf("expected input minimum height enforcement, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "display-message -p -t %4 #{pane_height}|#{pane_width}") {
		t.Fatalf("expected system pane dimension probe, commands:\n%s", commands)
	}
	if !strings.Contains(commands, "resize-pane -t %4 -x 24") {
		t.Fatalf("expected system minimum width enforcement, commands:\n%s", commands)
	}
}
