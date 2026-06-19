package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

func TestZellijContract_SessionCreation_CommandShape(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{})

	// Session creation in zellij 0.40+: 'attach --create-background <name>' (positional, no flags)
	driver := NewZellijMultiplexerDriver()
	err := driver.Run(context.Background(), "attach", "--create-background", "contract")
	if err != nil {
		t.Fatalf("expected session creation success, got %v", err)
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{Args: []string{"attach", "--create-background", "contract"}}})
}

func TestZellijContract_SessionAttach_CommandShape(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{})

	driver := NewZellijMultiplexerDriver()
	err := driver.AttachSession(context.Background(), "contract", strings.NewReader("attach"), &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("expected attach success, got %v", err)
	}

	// zellij 0.40+: attach takes session name as positional arg, not --session-name flag
	fixture.assertInvocations(t, []fakeZellijInvocation{{
		Args:         []string{"attach", "contract"},
		CheckStdin:   true,
		StdinPresent: true,
		StdoutTTY:    false,
		StderrTTY:    false,
	}})
}

func TestZellijContract_SessionShutdown_CommandShape(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{ExitCode: 12, Stderr: "error: session not found\n"})

	// zellij 0.40+: kill-session is a top-level subcommand, takes session name as positional arg
	driver := NewZellijMultiplexerDriver()
	err := driver.Run(context.Background(), "kill-session", "contract")
	if err == nil {
		t.Fatal("expected shutdown failure")
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{Args: []string{"kill-session", "contract"}}})
}

func TestZellijContract_ErrorClassification_MissingSession(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{ExitCode: 10, Stderr: "error: session not found\n"})

	driver := NewZellijMultiplexerDriver()
	output, err := driver.RunCombined(context.Background(), "action", "list-sessions")
	if err == nil {
		t.Fatal("expected missing-session failure")
	}
	if output != "error: session not found" {
		t.Fatalf("expected preserved missing-session output, got %q", output)
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{Args: []string{"action", "list-sessions"}}})
}

func TestZellijContract_ErrorClassification_PermissionDenied(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{ExitCode: 13, Stderr: "permission denied\n"})

	driver := NewZellijMultiplexerDriver()
	output, err := driver.RunCombined(context.Background(), "kill-session", "contract")
	if err == nil {
		t.Fatal("expected permission failure")
	}
	if !strings.Contains(output, "permission denied") {
		t.Fatalf("expected permission diagnostics, got %q", output)
	}
	if strings.Contains(strings.ToLower(output), "session not found") {
		t.Fatalf("permission error unexpectedly classified as missing-session: %q", output)
	}

	// zellij 0.40+: kill-session is top-level subcommand with positional session name
	fixture.assertInvocations(t, []fakeZellijInvocation{{Args: []string{"kill-session", "contract"}}})
}

func TestZellijContract_PaneIDCapture(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{Stdout: "pane_1\n"})

	driver := NewZellijMultiplexerDriver()
	paneID, err := driver.Capture(context.Background(), "action", "list-panes")
	if err != nil {
		t.Fatalf("expected pane capture success, got %v", err)
	}
	if !isParseableZellijPaneID(paneID) {
		t.Fatalf("expected parseable zellij pane id, got %q", paneID)
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{Args: []string{"action", "list-panes"}}})
}

func TestZellijContract_SessionListFormat(t *testing.T) {
	fixture := newFakeZellijFixture(t, fakeZellijBehavior{Stdout: "alpha\nbeta\n"})

	driver := NewZellijMultiplexerDriver()
	output, err := driver.RunCombined(context.Background(), "action", "list-sessions")
	if err != nil {
		t.Fatalf("expected session list success, got %v", err)
	}
	sessions := strings.Split(output, "\n")
	if len(sessions) != 2 {
		t.Fatalf("expected two sessions, got %d from %q", len(sessions), output)
	}
	if sessions[0] != "alpha" || sessions[1] != "beta" {
		t.Fatalf("unexpected session list contents: %v", sessions)
	}

	fixture.assertInvocations(t, []fakeZellijInvocation{{Args: []string{"action", "list-sessions"}}})
}

func isParseableZellijPaneID(value string) bool {
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "pane_") && len(value) > len("pane_") {
		return true
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func mustSetPathPrefix(t *testing.T, prefix string) string {
	t.Helper()
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", prefix+":"+oldPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	return oldPath
}