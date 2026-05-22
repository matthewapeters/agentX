package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDemoStartIndex_DefaultsToFirst(t *testing.T) {
	// GIVEN a demo sequence and no selector
	// WHEN resolving the start selector
	// THEN the first test should be selected.
	sequence := defaultDemoSequence()

	idx, err := resolveDemoStartIndex(sequence, "")
	if err != nil {
		t.Fatalf("expected no error for empty selector, got %v", err)
	}
	if idx != 0 {
		t.Fatalf("expected start index 0, got %d", idx)
	}
}

func TestResolveDemoStartIndex_AcceptsID(t *testing.T) {
	// GIVEN a demo sequence
	// WHEN resolving a selector by test id
	// THEN the matching test index should be returned.
	sequence := defaultDemoSequence()

	idx, err := resolveDemoStartIndex(sequence, "e2e-002")
	if err != nil {
		t.Fatalf("expected no error for id selector, got %v", err)
	}
	if idx != 1 {
		t.Fatalf("expected index 1 for e2e-002, got %d", idx)
	}
}

func TestResolveDemoStartIndex_AcceptsOneBasedIndex(t *testing.T) {
	// GIVEN a demo sequence
	// WHEN resolving a selector by 1-based index
	// THEN the corresponding zero-based index should be returned.
	sequence := defaultDemoSequence()

	idx, err := resolveDemoStartIndex(sequence, "3")
	if err != nil {
		t.Fatalf("expected no error for numeric selector, got %v", err)
	}
	if idx != 2 {
		t.Fatalf("expected index 2 for selector 3, got %d", idx)
	}
}

func TestResolveDemoStartIndex_RejectsInvalidSelector(t *testing.T) {
	// GIVEN a demo sequence
	// WHEN resolving an invalid selector
	// THEN an error should be returned.
	sequence := defaultDemoSequence()

	_, err := resolveDemoStartIndex(sequence, "does-not-exist")
	if err == nil {
		t.Fatal("expected error for invalid selector")
	}
	if !strings.Contains(err.Error(), "invalid --demo-start value") {
		t.Fatalf("expected invalid selector error message, got %v", err)
	}
}

func TestRunDemoMode_StartSelectionAndUserFailStopsSequence(t *testing.T) {
	// GIVEN DemoMode started from a selected test id
	// WHEN the user marks the first executed test as failed (X)
	// THEN execution stops and the summary reports Not ready for UAT.
	var output bytes.Buffer
	input := strings.NewReader("X\n")
	runCount := 0

	runner := func(testCase DemoTestCase) (string, error) {
		runCount++
		return "ok", nil
	}

	err := runDemoModeWithOptions(
		input,
		&output,
		"e2e-002",
		runner,
		demoModeOptions{
			collector: func(runtimeConfig DemoRuntimeConfig, testCase DemoTestCase) ([]string, error) {
				return nil, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("expected demo mode to succeed, got %v", err)
	}

	if runCount != 1 {
		t.Fatalf("expected one executed test before X stop, got %d", runCount)
	}

	content := output.String()
	if !strings.Contains(content, "[AgentX Demo] Start selection: 2) e2e-002") {
		t.Fatalf("expected selected start output, got:\n%s", content)
	}
	if !strings.Contains(content, "[AgentX Demo] Failed test: e2e-002") {
		t.Fatalf("expected failed test summary, got:\n%s", content)
	}
	if !strings.Contains(content, "[AgentX Demo] Artifact paths: none") {
		t.Fatalf("expected artifact-path summary line, got:\n%s", content)
	}
	if !strings.Contains(content, "[AgentX Demo] Readiness: Not ready for UAT") {
		t.Fatalf("expected not-ready summary, got:\n%s", content)
	}
}

func TestRunDemoMode_FailureDecisionReportsCollectedArtifactPath(t *testing.T) {
	// GIVEN DemoMode receives a custom diagnostics collector
	// WHEN user enters X at the first test
	// THEN collector artifact path is printed in the summary.
	var output bytes.Buffer
	input := strings.NewReader("X\n")

	collector := func(runtimeConfig DemoRuntimeConfig, testCase DemoTestCase) ([]string, error) {
		return []string{"logs/demo/test-session/e2e-001"}, nil
	}

	err := runDemoModeWithOptions(
		input,
		&output,
		"",
		nil,
		demoModeOptions{
			runtimeConfig: DemoRuntimeConfig{SessionID: "test-session"},
			collector:     collector,
		},
	)
	if err != nil {
		t.Fatalf("expected demo mode to succeed, got %v", err)
	}

	content := output.String()
	if !strings.Contains(content, "[AgentX Demo] Artifact paths: logs/demo/test-session/e2e-001") {
		t.Fatalf("expected collected artifact path in summary, got:\n%s", content)
	}
}

func TestRunDemoMode_InvalidDecisionReprompts(t *testing.T) {
	// GIVEN DemoMode awaiting per-test decision
	// WHEN user enters an invalid decision and then valid decisions
	// THEN DemoMode re-prompts and only accepts N or X.
	var output bytes.Buffer
	input := strings.NewReader("bad\nN\nX\n")
	runCount := 0

	runner := func(testCase DemoTestCase) (string, error) {
		runCount++
		return "ok", nil
	}

	err := runDemoMode(input, &output, "", runner)
	if err != nil {
		t.Fatalf("expected demo mode to succeed, got %v", err)
	}

	if runCount != 2 {
		t.Fatalf("expected two executed tests with N then X, got %d", runCount)
	}

	content := output.String()
	if !strings.Contains(content, "[AgentX Demo] Invalid decision; enter N or X") {
		t.Fatalf("expected invalid-input re-prompt message, got:\n%s", content)
	}
	if !strings.Contains(content, "[AgentX Demo] Enter decision [N=next, X=fail]:") {
		t.Fatalf("expected per-test prompt, got:\n%s", content)
	}
}

func TestRunDemoMode_AllAcceptedShowsReadyForUAT(t *testing.T) {
	// GIVEN DemoMode starts at the final test
	// WHEN user accepts the executed test with N
	// THEN readiness summary should report Ready for UAT.
	var output bytes.Buffer
	input := strings.NewReader("N\n")

	err := runDemoMode(input, &output, "e2e-003", nil)
	if err != nil {
		t.Fatalf("expected demo mode to succeed, got %v", err)
	}

	content := output.String()
	if !strings.Contains(content, "[AgentX Demo] Readiness: Ready for UAT") {
		t.Fatalf("expected ready summary, got:\n%s", content)
	}
	if !strings.Contains(content, "[AgentX Demo] Artifact paths: none") {
		t.Fatalf("expected no-artifact summary line, got:\n%s", content)
	}
}

func TestRunDemoMode_ContextCancelledExitsPromptLoop(t *testing.T) {
	// GIVEN DemoMode waiting for a per-test decision
	// WHEN controller context is cancelled (for example via Ctrl-C)
	// THEN the loop exits instead of leaving an orphaned pane prompt.
	var output bytes.Buffer
	input := strings.NewReader("invalid\n")
	runCount := 0

	runner := func(testCase DemoTestCase) (string, error) {
		runCount++
		return "ok", nil
	}

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runDemoModeWithOptions(
		input,
		&output,
		"",
		runner,
		demoModeOptions{decisionCtx: cancelledCtx},
	)
	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
	if !strings.Contains(err.Error(), "demo decision cancelled") {
		t.Fatalf("expected cancellation message, got %v", err)
	}
	if runCount != 1 {
		t.Fatalf("expected runner to execute once before cancellation, got %d", runCount)
	}
}

func TestDefaultDemoDiagnosticsCollector_WritesArtifacts(t *testing.T) {
	// GIVEN a fake tmux executable and a writable project directory
	// WHEN diagnostics are collected for a failed demo test
	// THEN deterministic artifact files are written under logs/demo/<session>/<test>.
	tmpDir := t.TempDir()
	tmuxScriptPath := filepath.Join(tmpDir, "tmux")
	tmuxScript := `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "list-panes" ]]; then
  echo '%1|chat'
  echo '%2|context'
  exit 0
fi
if [[ "$1" == "display-message" ]]; then
  echo 'demo-session:0.0'
  exit 0
fi
if [[ "$1" == "capture-pane" ]]; then
  if [[ "$4" == "%1" ]]; then
    echo 'chat pane content'
  else
    echo 'context pane content'
  fi
  exit 0
fi
echo "unexpected tmux args: $*" >&2
exit 1
`

	if err := os.WriteFile(tmuxScriptPath, []byte(tmuxScript), 0o755); err != nil {
		t.Fatalf("failed to write fake tmux script: %v", err)
	}

	originalPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmpDir+":"+originalPath); err != nil {
		t.Fatalf("failed to set PATH: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Setenv("PATH", originalPath)
	})

	artifacts, err := defaultDemoDiagnosticsCollector(
		DemoRuntimeConfig{
			ProjectDir:      tmpDir,
			SessionID:       "session-1",
			Username:        "tester",
			TmuxSessionName: "demo-session",
		},
		DemoTestCase{ID: "e2e-001", Title: "test"},
	)
	if err != nil {
		t.Fatalf("expected diagnostics collector to succeed, got %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected one artifact directory path, got %d", len(artifacts))
	}

	artifactDir := artifacts[0]
	expectedFiles := []string{
		"metadata.json",
		"tmux_list_panes.txt",
		"tmux_display_message.txt",
		"pane_%1.txt",
		"pane_%2.txt",
	}

	for _, name := range expectedFiles {
		if _, statErr := os.Stat(filepath.Join(artifactDir, name)); statErr != nil {
			t.Fatalf("expected artifact file %s to exist: %v", name, statErr)
		}
	}
}
