package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// GIVEN startup bootstrap builds the first tmux session command
// WHEN command args are emitted through the abstraction seam
// THEN the command must preserve startup naming and deterministic geometry flags.
func TestMultiplexerContract_StartupCommandShape(t *testing.T) {
	sessionName := "agentx_contract"
	command := buildNewSessionCommand(defaultMultiplexerBackend, sessionName, 132, 44, "")

	want := []string{"new-session", "-d", "-s", sessionName, "-n", tmuxPrimaryWindow, "-x", "132", "-y", "44"}
	if len(command) != len(want) {
		t.Fatalf("expected startup command length %d, got %d (%v)", len(want), len(command), command)
	}
	for idx := range want {
		if command[idx] != want[idx] {
			t.Fatalf("startup command mismatch at index %d: got %q want %q", idx, command[idx], want[idx])
		}
	}
}

// GIVEN pane role routing is emitted through role-based mapping
// WHEN pane targets are constructed for chat/input/system
// THEN all required pane roles should map deterministically, including logs.
func TestMultiplexerContract_PaneRoutingByRole(t *testing.T) {
	targets := paneTargets("agentx_contract", "%1", "%3", "%4")
	if len(targets) != 4 {
		t.Fatalf("expected 4 pane targets, got %d", len(targets))
	}

	want := map[string]string{
		PaneTitleOutput: "%1",
		PaneTitleInput:  "%3",
		PaneTitleSystem: "%4",
		PaneTitleLogs:   "agentx_contract:1.0",
	}

	for _, target := range targets {
		wantTarget, ok := want[target.name]
		if !ok {
			t.Fatalf("unexpected pane role %q", target.name)
		}
		if target.target != wantTarget {
			t.Fatalf("pane role %q target mismatch: got %q want %q", target.name, target.target, wantTarget)
		}
	}
}

// GIVEN tmux cleanup can run against already-stopped sessions
// WHEN the command reports a missing-session error
// THEN cleanup classification should treat those messages as tolerated.
func TestMultiplexerContract_ShutdownMissingSessionClassification(t *testing.T) {
	if !isTmuxMissingSessionError(contractErr("can't find session: agentx_contract")) {
		t.Fatalf("expected can't find session errors to be classified as missing-session")
	}
	if !isTmuxMissingSessionError(contractErr("no server running on /tmp/tmux-1000/default")) {
		t.Fatalf("expected no server running errors to be classified as missing-session")
	}
	if isTmuxMissingSessionError(contractErr("permission denied")) {
		t.Fatalf("did not expect non-session errors to be classified as missing-session")
	}
}

// GIVEN runtime probe checks tmux/tmuxp before startup
// WHEN tmuxp probe fails
// THEN error propagation should include probe failure context for deterministic diagnostics.
func TestMultiplexerContract_RuntimeProbeErrorContext(t *testing.T) {
	tmpDir := t.TempDir()
	createExecutable(t, filepath.Join(tmpDir, "tmux"), "#!/usr/bin/env bash\nset -euo pipefail\nif [[ \"${1:-}\" == \"-V\" ]]; then\n  echo tmux 3.4\n  exit 0\nfi\nexit 1\n")
	createExecutable(t, filepath.Join(tmpDir, "tmuxp"), "#!/usr/bin/env bash\nset -euo pipefail\necho incompatible tmuxp >&2\nexit 2\n")

	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmpDir+":"+oldPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", oldPath)
	})

	err := validateRuntimePrerequisites(t.TempDir())
	if err == nil {
		t.Fatalf("expected tmuxp probe failure")
	}
	got := err.Error()
	if !strings.Contains(got, "tmuxp probe failed") || !strings.Contains(got, "incompatible tmuxp") {
		t.Fatalf("expected tmuxp probe context in error, got %q", got)
	}
}

type contractErr string

func (e contractErr) Error() string {
	return string(e)
}
