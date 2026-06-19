package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type stubMultiplexerDriver struct {
	backendName string
	runErr      error
	runCombined string
	captureOut  string
	captureErr  error
	attachErr   error

	runCalls     [][]string
	captureCalls [][]string
	attachCalls  []attachCall
}

type attachCall struct {
	session string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
}

func (s *stubMultiplexerDriver) BackendName() string {
	if s.backendName == "" {
		return "stub"
	}
	return s.backendName
}

func (s *stubMultiplexerDriver) Run(_ context.Context, args ...string) error {
	s.runCalls = append(s.runCalls, append([]string(nil), args...))
	return s.runErr
}

func (s *stubMultiplexerDriver) RunCombined(_ context.Context, args ...string) (string, error) {
	s.runCalls = append(s.runCalls, append([]string(nil), args...))
	if s.runErr != nil {
		return "", s.runErr
	}
	return s.runCombined, nil
}

func (s *stubMultiplexerDriver) Capture(_ context.Context, args ...string) (string, error) {
	s.captureCalls = append(s.captureCalls, append([]string(nil), args...))
	if s.captureErr != nil {
		return "", s.captureErr
	}
	return s.captureOut, nil
}

func (s *stubMultiplexerDriver) AttachSession(_ context.Context, sessionName string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	s.attachCalls = append(s.attachCalls, attachCall{
		session: sessionName,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
	})
	return s.attachErr
}

func TestNewAgentXCoreWithDriver_UsesInjectedDriverAndDefaultsWhenNil(t *testing.T) {
	cfg := &Config{ProjectDir: t.TempDir(), Username: "tester", SessionID: "sess-1"}

	injected := &stubMultiplexerDriver{}
	coreWithInjected := NewAgentXCoreWithDriver(cfg, injected)
	if coreWithInjected.multiplexerDriver != injected {
		t.Fatalf("expected injected driver to be used")
	}

	coreWithFallback := NewAgentXCoreWithDriver(cfg, nil)
	if coreWithFallback.multiplexerDriver == nil {
		t.Fatalf("expected fallback driver when nil is injected")
	}
	if _, ok := coreWithFallback.multiplexerDriver.(*TmuxMultiplexerDriver); !ok {
		t.Fatalf("expected fallback driver type *TmuxMultiplexerDriver, got %T", coreWithFallback.multiplexerDriver)
	}
}

func TestNewAgentXCore_UsesConfiguredMultiplexerDriver(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	content := "[agentx]\nmultiplexer_backend = \"zellij\"\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	core := NewAgentXCore(&Config{ProjectDir: projectDir, Username: "tester", SessionID: "sess-zellij"})
	if _, ok := core.multiplexerDriver.(*ZellijMultiplexerDriver); !ok {
		t.Fatalf("expected configured driver type *ZellijMultiplexerDriver, got %T", core.multiplexerDriver)
	}
}

func TestAgentXCoreRunTmux_DelegatesToDriverOnce(t *testing.T) {
	driver := &stubMultiplexerDriver{}
	core := &AgentXCore{multiplexerDriver: driver}

	args := []string{"kill-session", "-t", "agentx_tester_sess"}
	if err := core.runTmux(context.Background(), args...); err != nil {
		t.Fatalf("expected runTmux to succeed, got %v", err)
	}

	if len(driver.runCalls) != 1 {
		t.Fatalf("expected one Run delegation, got %d", len(driver.runCalls))
	}
	if !reflect.DeepEqual(driver.runCalls[0], args) {
		t.Fatalf("delegated Run args mismatch: got %v want %v", driver.runCalls[0], args)
	}
}

func TestAgentXCoreRunTmuxCapture_DelegatesToDriverOnce(t *testing.T) {
	driver := &stubMultiplexerDriver{captureOut: "%3"}
	core := &AgentXCore{multiplexerDriver: driver}

	args := []string{"split-window", "-P", "-F", "#{pane_id}"}
	got, err := core.runTmuxCapture(context.Background(), args...)
	if err != nil {
		t.Fatalf("expected runTmuxCapture to succeed, got %v", err)
	}
	if got != "%3" {
		t.Fatalf("capture result mismatch: got %q want %q", got, "%3")
	}

	if len(driver.captureCalls) != 1 {
		t.Fatalf("expected one Capture delegation, got %d", len(driver.captureCalls))
	}
	if !reflect.DeepEqual(driver.captureCalls[0], args) {
		t.Fatalf("delegated Capture args mismatch: got %v want %v", driver.captureCalls[0], args)
	}
}

func TestAttachTmuxSession_DelegatesToDriverWithCoreSessionAndStdIO(t *testing.T) {
	driver := &stubMultiplexerDriver{}
	core := &AgentXCore{
		multiplexerDriver: driver,
		tmuxSessionName:   "agentx_tester_sess-attach",
	}

	if err := core.AttachTmuxSession(context.Background()); err != nil {
		t.Fatalf("expected attach to succeed, got %v", err)
	}
	if len(driver.attachCalls) != 1 {
		t.Fatalf("expected one AttachSession delegation, got %d", len(driver.attachCalls))
	}
	call := driver.attachCalls[0]
	if call.session != "agentx_tester_sess-attach" {
		t.Fatalf("attach session mismatch: got %q", call.session)
	}
	if call.stdin != os.Stdin {
		t.Fatalf("expected stdin delegation to os.Stdin")
	}
	if call.stdout != os.Stdout {
		t.Fatalf("expected stdout delegation to os.Stdout")
	}
	if call.stderr != os.Stderr {
		t.Fatalf("expected stderr delegation to os.Stderr")
	}
}

func TestAttachTmuxSession_PropagatesDriverError(t *testing.T) {
	wantErr := errors.New("attach failed")
	driver := &stubMultiplexerDriver{attachErr: wantErr}
	core := &AgentXCore{
		multiplexerDriver: driver,
		tmuxSessionName:   "agentx_tester_sess-attach",
	}

	err := core.AttachTmuxSession(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected attach error propagation, got %v", err)
	}
	if len(driver.attachCalls) != 1 {
		t.Fatalf("expected one AttachSession delegation, got %d", len(driver.attachCalls))
	}
}

func TestAgentXCoreRunTmux_PropagatesDriverError(t *testing.T) {
	wantErr := errors.New("driver run failed")
	driver := &stubMultiplexerDriver{runErr: wantErr}
	core := &AgentXCore{multiplexerDriver: driver}

	err := core.runTmux(context.Background(), "select-pane", "-t", "%1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected runTmux to propagate driver error, got %v", err)
	}
}

func TestSeam_NewMultiplexerDriverFromConfig_Zellij(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	content := "[agentx]\nmultiplexer_backend = \"zellij\"\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	driver, err := newMultiplexerDriverFromConfig(projectDir)
	if err != nil {
		t.Fatalf("expected zellij driver selection success, got %v", err)
	}
	if _, ok := driver.(*ZellijMultiplexerDriver); !ok {
		t.Fatalf("expected *ZellijMultiplexerDriver, got %T", driver)
	}
}

func TestSeam_RuntimeMultiplexerDriver_ReturnsZellijWhenConfigured(t *testing.T) {
	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, "agentx.toml")
	content := "[agentx]\nmultiplexer_backend = \"zellij\"\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	driver, err := runtimeMultiplexerDriver(projectDir)
	if err != nil {
		t.Fatalf("expected runtime driver selection success, got %v", err)
	}
	if got := driver.BackendName(); got != "zellij" {
		t.Fatalf("runtime driver backend = %q, want %q", got, "zellij")
	}
}

func TestSeam_ZellijDriver_MethodDelegation_Run(t *testing.T) {
	driver := &stubMultiplexerDriver{backendName: "zellij"}
	core := &AgentXCore{multiplexerDriver: driver}
	args := []string{"action", "new-session", "--session-name", "test"}

	if err := core.runTmux(context.Background(), args...); err != nil {
		t.Fatalf("expected zellij-backed run delegation success, got %v", err)
	}
	if len(driver.runCalls) != 1 || !reflect.DeepEqual(driver.runCalls[0], args) {
		t.Fatalf("delegated zellij Run args mismatch: got %v want %v", driver.runCalls, args)
	}
}

func TestSeam_ZellijDriver_MethodDelegation_RunCombined(t *testing.T) {
	driver := &stubMultiplexerDriver{backendName: "zellij", runCombined: "alpha\nbeta"}
	args := []string{"action", "list-sessions"}

	got, err := driver.RunCombined(context.Background(), args...)
	if err != nil {
		t.Fatalf("expected zellij RunCombined delegation success, got %v", err)
	}
	if got != "alpha\nbeta" {
		t.Fatalf("delegated RunCombined output = %q, want %q", got, "alpha\nbeta")
	}
	if len(driver.runCalls) != 1 || !reflect.DeepEqual(driver.runCalls[0], args) {
		t.Fatalf("delegated zellij RunCombined args mismatch: got %v want %v", driver.runCalls, args)
	}
}

func TestSeam_ZellijDriver_MethodDelegation_Capture(t *testing.T) {
	driver := &stubMultiplexerDriver{backendName: "zellij", captureOut: "pane_1"}
	core := &AgentXCore{multiplexerDriver: driver}
	args := []string{"action", "list-panes"}

	got, err := core.runTmuxCapture(context.Background(), args...)
	if err != nil {
		t.Fatalf("expected zellij Capture delegation success, got %v", err)
	}
	if got != "pane_1" {
		t.Fatalf("delegated Capture output = %q, want %q", got, "pane_1")
	}
	if len(driver.captureCalls) != 1 || !reflect.DeepEqual(driver.captureCalls[0], args) {
		t.Fatalf("delegated zellij Capture args mismatch: got %v want %v", driver.captureCalls, args)
	}
}

func TestSeam_ZellijDriver_MethodDelegation_AttachSession(t *testing.T) {
	driver := &stubMultiplexerDriver{backendName: "zellij"}
	core := &AgentXCore{multiplexerDriver: driver, tmuxSessionName: "zellij_session"}

	if err := core.AttachTmuxSession(context.Background()); err != nil {
		t.Fatalf("expected zellij attach delegation success, got %v", err)
	}
	if len(driver.attachCalls) != 1 {
		t.Fatalf("expected one zellij AttachSession delegation, got %d", len(driver.attachCalls))
	}
	call := driver.attachCalls[0]
	if call.session != "zellij_session" {
		t.Fatalf("attach session mismatch: got %q", call.session)
	}
	if call.stdin != os.Stdin || call.stdout != os.Stdout || call.stderr != os.Stderr {
		t.Fatalf("expected stdio preservation during zellij attach delegation")
	}
}
