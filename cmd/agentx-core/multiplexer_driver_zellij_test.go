package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestZellijDriver_BackendName(t *testing.T) {
	driver := NewZellijMultiplexerDriver()
	if got := driver.BackendName(); got != "zellij" {
		t.Fatalf("BackendName() = %q, want %q", got, "zellij")
	}
}

func TestZellijDriver_Run_Success(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{})

	driver := NewZellijMultiplexerDriver()
	err := driver.Run(context.Background(), "action", "new-session", "--session-name", "test")
	if err != nil {
		t.Fatalf("expected Run success, got %v", err)
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{Args: []string{"action", "new-session", "--session-name", "test"}}})
}

func TestZellijDriver_Run_CommandFailure(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{ExitCode: 9, Stderr: "run failed\n"})

	driver := NewZellijMultiplexerDriver()
	err := driver.Run(context.Background(), "action", "new-session", "--session-name", "test")
	if err == nil {
		t.Fatal("expected Run failure")
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{Args: []string{"action", "new-session", "--session-name", "test"}}})
}

func TestZellijDriver_Run_ContextTimeout(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{SleepMS: 100})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	driver := NewZellijMultiplexerDriver()
	err := driver.Run(ctx, "action", "new-session", "--session-name", "test")
	if err == nil {
		t.Fatal("expected Run timeout")
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got ctx.Err()=%v err=%v", ctx.Err(), err)
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{Args: []string{"action", "new-session", "--session-name", "test"}}})
}

func TestZellijDriver_RunCombined_Success(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{Stdout: "session1\nsession2\n"})

	driver := NewZellijMultiplexerDriver()
	output, err := driver.RunCombined(context.Background(), "action", "list-sessions")
	if err != nil {
		t.Fatalf("expected RunCombined success, got %v", err)
	}
	if output != "session1\nsession2" {
		t.Fatalf("RunCombined output = %q, want %q", output, "session1\nsession2")
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{Args: []string{"action", "list-sessions"}}})
}

func TestZellijDriver_RunCombined_CommandFailure(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{ExitCode: 7, Stderr: "error: session not found\n"})

	driver := NewZellijMultiplexerDriver()
	output, err := driver.RunCombined(context.Background(), "action", "list-sessions")
	if err == nil {
		t.Fatal("expected RunCombined failure")
	}
	if output != "error: session not found" {
		t.Fatalf("RunCombined output = %q, want %q", output, "error: session not found")
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{Args: []string{"action", "list-sessions"}}})
}

func TestZellijDriver_RunCombined_EmptyOutput(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{})

	driver := NewZellijMultiplexerDriver()
	output, err := driver.RunCombined(context.Background(), "action", "list-sessions")
	if err != nil {
		t.Fatalf("expected RunCombined success, got %v", err)
	}
	if output != "" {
		t.Fatalf("RunCombined output = %q, want empty", output)
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{Args: []string{"action", "list-sessions"}}})
}

func TestZellijDriver_Capture_Success(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{Stdout: "pane_id_12345\n"})

	driver := NewZellijMultiplexerDriver()
	output, err := driver.Capture(context.Background(), "action", "list-sessions")
	if err != nil {
		t.Fatalf("expected Capture success, got %v", err)
	}
	if output != "pane_id_12345" {
		t.Fatalf("Capture output = %q, want %q", output, "pane_id_12345")
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{Args: []string{"action", "list-sessions"}}})
}

func TestZellijDriver_Capture_CommandFailure(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{ExitCode: 5, Stderr: "capture failed\n"})

	driver := NewZellijMultiplexerDriver()
	output, err := driver.Capture(context.Background(), "action", "list-sessions")
	if err == nil {
		t.Fatal("expected Capture failure")
	}
	if output != "" {
		t.Fatalf("Capture output = %q, want empty", output)
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{Args: []string{"action", "list-sessions"}}})
}

func TestZellijDriver_Capture_ContextTimeout(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{SleepMS: 100})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	driver := NewZellijMultiplexerDriver()
	output, err := driver.Capture(ctx, "action", "list-sessions")
	if err == nil {
		t.Fatal("expected Capture timeout")
	}
	if output != "" {
		t.Fatalf("Capture output = %q, want empty", output)
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got ctx.Err()=%v err=%v", ctx.Err(), err)
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{Args: []string{"action", "list-sessions"}}})
}

func TestZellijDriver_AttachSession_Success(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{})

	stdin := strings.NewReader("interactive-input")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	driver := NewZellijMultiplexerDriver()
	err := driver.AttachSession(context.Background(), "test_session", stdin, stdout, stderr)
	if err != nil {
		t.Fatalf("expected AttachSession success, got %v", err)
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{
		Args:         []string{"attach", "test_session"},
		CheckStdin:   true,
		StdinPresent: true,
		StdoutTTY:    false,
		StderrTTY:    false,
	}})
}

func TestZellijDriver_AttachSession_CommandFailure(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{ExitCode: 4, Stderr: "attach failed\n"})

	driver := NewZellijMultiplexerDriver()
	err := driver.AttachSession(context.Background(), "nonexistent", strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected AttachSession failure")
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{
		Args:         []string{"attach", "nonexistent"},
		CheckStdin:   true,
		StdinPresent: true,
		StdoutTTY:    false,
		StderrTTY:    false,
	}})
}

func TestZellijDriver_AttachSession_MissingSessionError(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{ExitCode: 17, Stderr: "error: session not found\n"})

	driver := NewZellijMultiplexerDriver()
	stderr := &bytes.Buffer{}
	err := driver.AttachSession(context.Background(), "missing", strings.NewReader(""), &bytes.Buffer{}, stderr)
	if err == nil {
		t.Fatal("expected AttachSession failure")
	}
	if !strings.Contains(strings.ToLower(stderr.String()), "session") {
		t.Fatalf("expected missing session diagnostics on stderr, got err=%v stderr=%q", err, stderr.String())
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{
		Args:         []string{"attach", "missing"},
		CheckStdin:   true,
		StdinPresent: true,
		StdoutTTY:    false,
		StderrTTY:    false,
	}})
}

func TestZellijDriver_UsesProjectAgentXConfigDir(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{})
	projectDir := t.TempDir()

	driver := NewZellijMultiplexerDriver(projectDir)
	err := driver.Run(context.Background(), "action", "list-sessions")
	if err != nil {
		t.Fatalf("expected Run success, got %v", err)
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{
		Args: []string{"--config-dir", filepath.Join(projectDir, ".agentx"), "action", "list-sessions"},
	}})

	if _, err := os.Stat(filepath.Join(projectDir, ".agentx")); err != nil {
		t.Fatalf("expected .agentx config directory to exist, got %v", err)
	}
	configBytes, err := os.ReadFile(filepath.Join(projectDir, ".agentx", "config.kdl"))
	if err != nil {
		t.Fatalf("expected .agentx config.kdl to exist, got %v", err)
	}
	config := string(configBytes)
	if !strings.Contains(config, "copy_on_select true") {
		t.Fatalf("expected auto-copy selection config, got %q", config)
	}
	if !strings.Contains(config, "copy_clipboard \"system\"") {
		t.Fatalf("expected system clipboard config, got %q", config)
	}
}

func TestZellijCopyCommand_PrefersWaylandClipboard(t *testing.T) {
	tmpDir := t.TempDir()
	createExecutable(t, filepath.Join(tmpDir, "wl-copy"), "#!/usr/bin/env bash\nexit 0\n")
	createExecutable(t, filepath.Join(tmpDir, "xclip"), "#!/usr/bin/env bash\nexit 0\n")

	oldPath := os.Getenv("PATH")
	oldWayland := os.Getenv("WAYLAND_DISPLAY")
	oldSessionType := os.Getenv("XDG_SESSION_TYPE")
	if err := os.Setenv("PATH", tmpDir+":"+oldPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	if err := os.Setenv("WAYLAND_DISPLAY", "wayland-0"); err != nil {
		t.Fatalf("failed to set WAYLAND_DISPLAY: %v", err)
	}
	if err := os.Setenv("XDG_SESSION_TYPE", "wayland"); err != nil {
		t.Fatalf("failed to set XDG_SESSION_TYPE: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
		_ = os.Setenv("WAYLAND_DISPLAY", oldWayland)
		_ = os.Setenv("XDG_SESSION_TYPE", oldSessionType)
	})

	if got := zellijCopyCommand(); got != "wl-copy" {
		t.Fatalf("expected wl-copy on Wayland, got %q", got)
	}
}

func TestZellijCopyCommand_FallsBackToXclip(t *testing.T) {
	tmpDir := t.TempDir()
	createExecutable(t, filepath.Join(tmpDir, "xclip"), "#!/usr/bin/env bash\nexit 0\n")

	oldPath := os.Getenv("PATH")
	oldWayland := os.Getenv("WAYLAND_DISPLAY")
	oldSessionType := os.Getenv("XDG_SESSION_TYPE")
	if err := os.Setenv("PATH", tmpDir+":"+oldPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	if err := os.Unsetenv("WAYLAND_DISPLAY"); err != nil {
		t.Fatalf("failed to unset WAYLAND_DISPLAY: %v", err)
	}
	if err := os.Setenv("XDG_SESSION_TYPE", "x11"); err != nil {
		t.Fatalf("failed to set XDG_SESSION_TYPE: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
		_ = os.Setenv("WAYLAND_DISPLAY", oldWayland)
		_ = os.Setenv("XDG_SESSION_TYPE", oldSessionType)
	})

	if got := zellijCopyCommand(); got != "xclip -selection clipboard" {
		t.Fatalf("expected xclip fallback, got %q", got)
	}
}

type fakeZellijBehavior struct {
	Stdout   string
	Stderr   string
	ExitCode int
	SleepMS  int
}

type fakeZellijInvocation struct {
	Args         []string
	CheckStdin   bool
	StdinPresent bool
	StdoutTTY    bool
	StderrTTY    bool
}

type fakeZellijFixture struct {
	logPath string
}

func newFakeZellijFixture(t *testing.T, behavior fakeZellijBehavior) *fakeZellijFixture {
	t.Helper()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "zellij-invocations.log")
	scriptPath := filepath.Join(tmpDir, "zellij")
	stdoutPath := filepath.Join(tmpDir, "stdout.txt")
	stderrPath := filepath.Join(tmpDir, "stderr.txt")
	if err := os.WriteFile(stdoutPath, []byte(behavior.Stdout), 0o644); err != nil {
		t.Fatalf("failed to write fake zellij stdout: %v", err)
	}
	if err := os.WriteFile(stderrPath, []byte(behavior.Stderr), 0o644); err != nil {
		t.Fatalf("failed to write fake zellij stderr: %v", err)
	}
	script := fmt.Sprintf("#!/usr/bin/env bash\nset -euo pipefail\nlog_file=\"${FAKE_ZELLIJ_LOG:?}\"\nprintf 'args=%%s\\n' \"$*\" >> \"$log_file\"\nif [[ -t 1 ]]; then stdout_tty=true; else stdout_tty=false; fi\nif [[ -t 2 ]]; then stderr_tty=true; else stderr_tty=false; fi\nif read -r -t 0 _; then stdin_present=true; else stdin_present=false; fi\nprintf 'stdin_present=%%s stdout_tty=%%s stderr_tty=%%s\\n' \"$stdin_present\" \"$stdout_tty\" \"$stderr_tty\" >> \"$log_file\"\nif [[ %d -gt 0 ]]; then sleep %0.3f; fi\ncat %q\ncat %q >&2\nexit %d\n", behavior.SleepMS, float64(behavior.SleepMS)/1000, stdoutPath, stderrPath, behavior.ExitCode)
	createExecutable(t, scriptPath, script)

	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmpDir+":"+oldPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
	})

	oldLog, hadLog := os.LookupEnv("FAKE_ZELLIJ_LOG")
	if err := os.Setenv("FAKE_ZELLIJ_LOG", logPath); err != nil {
		t.Fatalf("failed to set FAKE_ZELLIJ_LOG: %v", err)
	}
	t.Cleanup(func() {
		if hadLog {
			_ = os.Setenv("FAKE_ZELLIJ_LOG", oldLog)
			return
		}
		_ = os.Unsetenv("FAKE_ZELLIJ_LOG")
	})

	return &fakeZellijFixture{logPath: logPath}
}

func (f *fakeZellijFixture) assertInvocations(t *testing.T, want []fakeZellijInvocation) {
	t.Helper()

	data, err := os.ReadFile(f.logPath)
	if err != nil {
		t.Fatalf("failed to read zellij invocation log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != len(want)*2 {
		t.Fatalf("expected %d log lines, got %d: %q", len(want)*2, len(lines), string(data))
	}

	got := make([]fakeZellijInvocation, 0, len(want))
	for idx := 0; idx < len(lines); idx += 2 {
		argsLine := strings.TrimPrefix(lines[idx], "args=")
		flagsLine := lines[idx+1]
		invocation := fakeZellijInvocation{}
		if argsLine != "" {
			invocation.Args = strings.Fields(argsLine)
		}
		invocation.StdinPresent = strings.Contains(flagsLine, "stdin_present=true")
		invocation.StdoutTTY = strings.Contains(flagsLine, "stdout_tty=true")
		invocation.StderrTTY = strings.Contains(flagsLine, "stderr_tty=true")
		got = append(got, invocation)
	}
	for idx := range want {
		got[idx].CheckStdin = want[idx].CheckStdin
		if !want[idx].CheckStdin {
			got[idx].StdinPresent = want[idx].StdinPresent
		}
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("zellij invocations mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}