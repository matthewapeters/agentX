package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type parityBackend struct {
	name   string
	driver MultiplexerDriver
	assert func(t *testing.T, want []fakeZellijInvocation)
}

func TestParity_BackendName(t *testing.T) {
	backends := newParityBackends(t, fakeZellijBehavior{})
	for _, backend := range backends {
		if got := backend.driver.BackendName(); got != backend.name {
			t.Fatalf("%s BackendName() = %q, want %q", backend.name, got, backend.name)
		}
	}
}

func TestParity_Run_ErrorPropagation(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		backends := newParityBackends(t, fakeZellijBehavior{})
		args := []string{"display-message", "-p", "ok"}
		for _, backend := range backends {
			if err := backend.driver.Run(context.Background(), args...); err != nil {
				t.Fatalf("%s Run() unexpected error: %v", backend.name, err)
			}
		}
	})

	t.Run("failure", func(t *testing.T) {
		backends := newParityBackends(t, fakeZellijBehavior{ExitCode: 7, Stderr: "run failed\n"})
		args := []string{"display-message", "-p", "fail"}
		for _, backend := range backends {
			if err := backend.driver.Run(context.Background(), args...); err == nil {
				t.Fatalf("%s Run() expected error", backend.name)
			}
		}
	})
}

func TestParity_Run_OutputDiscard(t *testing.T) {
	backends := newParityBackends(t, fakeZellijBehavior{Stdout: "output\n", Stderr: "error\n"})
	args := []string{"display-message", "-p", "discard"}
	for _, backend := range backends {
		if err := backend.driver.Run(context.Background(), args...); err != nil {
			t.Fatalf("%s Run() unexpected error: %v", backend.name, err)
		}
		backend.assert(t, []fakeZellijInvocation{{Args: args, CheckStdin: false, StdinPresent: false, StdoutTTY: false, StderrTTY: false}})
	}
}

func TestParity_RunCombined_OutputTrimming(t *testing.T) {
	backends := newParityBackends(t, fakeZellijBehavior{Stdout: "  output1\noutput2  \n"})
	args := []string{"display-message", "-p", "trim"}
	for _, backend := range backends {
		output, err := backend.driver.RunCombined(context.Background(), args...)
		if err != nil {
			t.Fatalf("%s RunCombined() unexpected error: %v", backend.name, err)
		}
		if output != "output1\noutput2" {
			t.Fatalf("%s RunCombined() = %q, want %q", backend.name, output, "output1\noutput2")
		}
	}
}

func TestParity_RunCombined_ErrorPreservesOutput(t *testing.T) {
	backends := newParityBackends(t, fakeZellijBehavior{ExitCode: 9, Stderr: "error: session not found\n"})
	args := []string{"display-message", "-p", "missing"}
	for _, backend := range backends {
		output, err := backend.driver.RunCombined(context.Background(), args...)
		if err == nil {
			t.Fatalf("%s RunCombined() expected error", backend.name)
		}
		if output != "error: session not found" {
			t.Fatalf("%s RunCombined() output = %q, want preserved error output", backend.name, output)
		}
	}
}

func TestParity_RunCombined_ContextTimeout(t *testing.T) {
	backends := newParityBackends(t, fakeZellijBehavior{Stdout: "partial\n", SleepMS: 100})
	args := []string{"display-message", "-p", "timeout"}
	for _, backend := range backends {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		output, err := backend.driver.RunCombined(ctx, args...)
		cancel()
		if err == nil {
			t.Fatalf("%s RunCombined() expected timeout", backend.name)
		}
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatalf("%s RunCombined() expected deadline exceeded, got err=%v ctx.Err()=%v", backend.name, err, ctx.Err())
		}
		if output != "" && output != "partial" {
			t.Fatalf("%s RunCombined() unexpected timeout output: %q", backend.name, output)
		}
	}
}

func TestParity_Capture_SuccessReturnsStdout(t *testing.T) {
	backends := newParityBackends(t, fakeZellijBehavior{Stdout: "pane_id_123\n"})
	args := []string{"display-message", "-p", "#{pane_id}"}
	for _, backend := range backends {
		output, err := backend.driver.Capture(context.Background(), args...)
		if err != nil {
			t.Fatalf("%s Capture() unexpected error: %v", backend.name, err)
		}
		if output != "pane_id_123" {
			t.Fatalf("%s Capture() = %q, want %q", backend.name, output, "pane_id_123")
		}
	}
}

func TestParity_Capture_ErrorReturnsEmptyString(t *testing.T) {
	backends := newParityBackends(t, fakeZellijBehavior{ExitCode: 5, Stderr: "capture failed\n"})
	args := []string{"display-message", "-p", "#{pane_id}"}
	for _, backend := range backends {
		output, err := backend.driver.Capture(context.Background(), args...)
		if err == nil {
			t.Fatalf("%s Capture() expected error", backend.name)
		}
		if output != "" {
			t.Fatalf("%s Capture() output = %q, want empty", backend.name, output)
		}
	}
}

func TestParity_AttachSession_StdioBound(t *testing.T) {
	backends := newParityBackends(t, fakeZellijBehavior{})
	for _, backend := range backends {
		stdin := strings.NewReader("interactive-input")
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		if err := backend.driver.AttachSession(context.Background(), "session", stdin, stdout, stderr); err != nil {
			t.Fatalf("%s AttachSession() unexpected error: %v", backend.name, err)
		}
		backend.assert(t, []fakeZellijInvocation{{Args: expectedAttachArgs(backend.name, "session"), CheckStdin: true, StdinPresent: true, StdoutTTY: false, StderrTTY: false}})
	}
}

func TestParity_AttachSession_ContextPropagation(t *testing.T) {
	backends := newParityBackends(t, fakeZellijBehavior{SleepMS: 100})
	for _, backend := range backends {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		err := backend.driver.AttachSession(ctx, "session", strings.NewReader("interactive-input"), &bytes.Buffer{}, &bytes.Buffer{})
		cancel()
		if err == nil {
			t.Fatalf("%s AttachSession() expected timeout", backend.name)
		}
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatalf("%s AttachSession() expected deadline exceeded, got err=%v ctx.Err()=%v", backend.name, err, ctx.Err())
		}
	}
}

func TestParity_ContextCancellation_AllMethods(t *testing.T) {
	backends := newParityBackends(t, fakeZellijBehavior{})
	methods := []struct {
		name string
		call func(driver MultiplexerDriver, ctx context.Context) error
	}{
		{name: "Run", call: func(driver MultiplexerDriver, ctx context.Context) error { return driver.Run(ctx, "display-message") }},
		{name: "RunVariant", call: func(driver MultiplexerDriver, ctx context.Context) error { return driver.Run(ctx, "display-message", "-p", "x") }},
		{name: "RunCombined", call: func(driver MultiplexerDriver, ctx context.Context) error { _, err := driver.RunCombined(ctx, "display-message", "-p", "x"); return err }},
		{name: "Capture", call: func(driver MultiplexerDriver, ctx context.Context) error { _, err := driver.Capture(ctx, "display-message", "-p", "x"); return err }},
		{name: "AttachSession", call: func(driver MultiplexerDriver, ctx context.Context) error { return driver.AttachSession(ctx, "session", strings.NewReader(""), io.Discard, io.Discard) }},
	}
	for _, backend := range backends {
		for _, method := range methods {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := method.call(backend.driver, ctx)
			if err == nil {
				t.Fatalf("%s %s expected canceled-context error", backend.name, method.name)
			}
			if !errors.Is(err, context.Canceled) && !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatalf("%s %s expected context canceled, got err=%v ctx.Err()=%v", backend.name, method.name, err, ctx.Err())
			}
		}
	}
}

func TestParity_CommandLineArgs_PassedCorrectly(t *testing.T) {
	tests := []struct {
		name string
		call func(driver MultiplexerDriver, args []string) error
		args []string
	}{
		{name: "Run", call: func(driver MultiplexerDriver, args []string) error { return driver.Run(context.Background(), args...) }, args: []string{"cmd", "--flag", "value"}},
		{name: "RunCombined", call: func(driver MultiplexerDriver, args []string) error { _, err := driver.RunCombined(context.Background(), args...); return err }, args: []string{"cmd", "sub", "--target", "pane-1"}},
		{name: "Capture", call: func(driver MultiplexerDriver, args []string) error { _, err := driver.Capture(context.Background(), args...); return err }, args: []string{"cmd", "list", "-F", "#{pane_id}"}},
	}
	for _, testCase := range tests {
		// Use isolated per-case fixtures to avoid contamination from tmux background calls
		// that the fake tmux binary may record from concurrent test state.
		zellijFixture := newFakeZellijFixture(t, fakeZellijBehavior{Stdout: "captured\n"})
		driver := NewZellijMultiplexerDriver()
		if err := testCase.call(driver, testCase.args); err != nil {
			t.Fatalf("zellij %s unexpected error: %v", testCase.name, err)
		}
		zellijFixture.assertInvocations(t, []fakeZellijInvocation{{Args: testCase.args, CheckStdin: false, StdinPresent: false, StdoutTTY: false, StderrTTY: false}})
	}
}

func newParityBackends(t *testing.T, behavior fakeZellijBehavior) []parityBackend {
	t.Helper()
	tmuxFixture := newFakeTmuxFixture(t, behavior)
	zellijFixture := newFakeZellijFixture(t, behavior)
	return []parityBackend{
		{name: "tmux", driver: NewTmuxMultiplexerDriver(), assert: tmuxFixture.assertInvocations},
		{name: "zellij", driver: NewZellijMultiplexerDriver(), assert: zellijFixture.assertInvocations},
	}
}

func expectedAttachArgs(backendName string, sessionName string) []string {
	// zellij 0.40+: positional session name, not --session-name flag
	if backendName == "zellij" {
		return []string{"attach", sessionName}
	}
	return []string{"attach-session", "-t", sessionName}
}

type fakeTmuxFixture struct {
	logPath string
}

func newFakeTmuxFixture(t *testing.T, behavior fakeZellijBehavior) *fakeTmuxFixture {
	t.Helper()
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "tmux-invocations.log")
	scriptPath := filepath.Join(tmpDir, "tmux")
	stdoutPath := filepath.Join(tmpDir, "stdout.txt")
	stderrPath := filepath.Join(tmpDir, "stderr.txt")
	if err := os.WriteFile(stdoutPath, []byte(behavior.Stdout), 0o644); err != nil {
		t.Fatalf("failed to write fake tmux stdout: %v", err)
	}
	if err := os.WriteFile(stderrPath, []byte(behavior.Stderr), 0o644); err != nil {
		t.Fatalf("failed to write fake tmux stderr: %v", err)
	}
	script := fmt.Sprintf("#!/usr/bin/env bash\nset -euo pipefail\nlog_file=\"${FAKE_TMUX_LOG:?}\"\nprintf 'args=%%s\\n' \"$*\" >> \"$log_file\"\nif read -r -t 0 _; then stdin_present=true; else stdin_present=false; fi\nif [[ -t 1 ]]; then stdout_tty=true; else stdout_tty=false; fi\nif [[ -t 2 ]]; then stderr_tty=true; else stderr_tty=false; fi\nprintf 'stdin_present=%%s stdout_tty=%%s stderr_tty=%%s\\n' \"$stdin_present\" \"$stdout_tty\" \"$stderr_tty\" >> \"$log_file\"\nif [[ %d -gt 0 ]]; then sleep %0.3f; fi\ncat %q\ncat %q >&2\nexit %d\n", behavior.SleepMS, float64(behavior.SleepMS)/1000, stdoutPath, stderrPath, behavior.ExitCode)
	createExecutable(t, scriptPath, script)

	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmpDir+":"+oldPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
	})

	oldLog, hadLog := os.LookupEnv("FAKE_TMUX_LOG")
	if err := os.Setenv("FAKE_TMUX_LOG", logPath); err != nil {
		t.Fatalf("failed to set FAKE_TMUX_LOG: %v", err)
	}
	t.Cleanup(func() {
		if hadLog {
			_ = os.Setenv("FAKE_TMUX_LOG", oldLog)
			return
		}
		_ = os.Unsetenv("FAKE_TMUX_LOG")
	})

	return &fakeTmuxFixture{logPath: logPath}
}

func (f *fakeTmuxFixture) assertInvocations(t *testing.T, want []fakeZellijInvocation) {
	t.Helper()
	data, err := os.ReadFile(f.logPath)
	if err != nil {
		t.Fatalf("failed to read tmux invocation log: %v", err)
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
		t.Fatalf("tmux invocations mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}