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
			collector: func(runtimeConfig DemoRuntimeConfig, testCase DemoTestCase, feedback string) ([]string, error) {
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
	if !strings.Contains(content, "[AgentX Demo] Ordered test sequence (Gherkin):") {
		t.Fatalf("expected gherkin sequence heading, got:\n%s", content)
	}
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
	if !strings.Contains(content, "[AgentX Demo]   - e2e-001: SKIP") {
		t.Fatalf("expected skipped status for unselected earlier test, got:\n%s", content)
	}
	if !strings.Contains(content, "[AgentX Demo]   - e2e-002: FAIL") {
		t.Fatalf("expected failed status for X-marked test, got:\n%s", content)
	}
	if !strings.Contains(content, "[AgentX Demo]   - e2e-003: SKIP") {
		t.Fatalf("expected skipped status for non-executed trailing test, got:\n%s", content)
	}
}

func TestRunDemoMode_FailureDecisionReportsCollectedArtifactPath(t *testing.T) {
	// GIVEN DemoMode receives a custom diagnostics collector
	// WHEN user enters X at the first test
	// THEN collector artifact path is printed in the summary.
	var output bytes.Buffer
	input := strings.NewReader("X\n")

	collector := func(runtimeConfig DemoRuntimeConfig, testCase DemoTestCase, feedback string) ([]string, error) {
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

func TestRunDemoMode_FailureDecisionCapturesInlineFeedback(t *testing.T) {
	// GIVEN DemoMode with a collector that records feedback
	// WHEN user enters X with inline feedback text
	// THEN feedback is surfaced in summary and passed to diagnostics collector.
	var output bytes.Buffer
	input := strings.NewReader("X observed context truncation in live-core pane\n")
	collectorFeedback := ""

	err := runDemoModeWithOptions(
		input,
		&output,
		"",
		nil,
		demoModeOptions{
			runtimeConfig: DemoRuntimeConfig{SessionID: "test-session"},
			collector: func(runtimeConfig DemoRuntimeConfig, testCase DemoTestCase, feedback string) ([]string, error) {
				collectorFeedback = feedback
				return []string{"logs/demo/test-session/e2e-001"}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("expected demo mode to succeed, got %v", err)
	}

	if collectorFeedback != "observed context truncation in live-core pane" {
		t.Fatalf("expected feedback passed to collector, got %q", collectorFeedback)
	}

	content := output.String()
	if !strings.Contains(content, "[AgentX Demo] Feedback: observed context truncation in live-core pane") {
		t.Fatalf("expected summary to include feedback text, got:\n%s", content)
	}
}

func TestRunDemoMode_JumpDecisionSkipsInterveningTests(t *testing.T) {
	// GIVEN DemoMode started at first test
	// WHEN user accepts first test then jumps to test 3 and marks it failed
	// THEN test 2 remains SKIP and test 3 is executed.
	var output bytes.Buffer
	input := strings.NewReader("J 3\nX\n")
	runOrder := make([]string, 0)

	runner := func(testCase DemoTestCase) (string, error) {
		runOrder = append(runOrder, testCase.ID)
		return "ok", nil
	}

	err := runDemoMode(input, &output, "", runner)
	if err != nil {
		t.Fatalf("expected demo mode to succeed, got %v", err)
	}

	if len(runOrder) != 2 || runOrder[0] != "e2e-001" || runOrder[1] != "e2e-003" {
		t.Fatalf("expected run order [e2e-001 e2e-003], got %v", runOrder)
	}

	content := output.String()
	if !strings.Contains(content, "[AgentX Demo] Jumped to 3) e2e-003") {
		t.Fatalf("expected jump confirmation in output, got:\n%s", content)
	}
	if !strings.Contains(content, "[AgentX Demo]   - e2e-002: SKIP") {
		t.Fatalf("expected skipped status for jumped-over test, got:\n%s", content)
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
	if !strings.Contains(content, "[AgentX Demo] Invalid decision; use N, J <test number>, or X <feedback>.") {
		t.Fatalf("expected invalid-input re-prompt message, got:\n%s", content)
	}
	if !strings.Contains(content, "[AgentX Demo] Enter decision [N=next, J <num>=jump, X <feedback>=fail]:") {
		t.Fatalf("expected per-test prompt, got:\n%s", content)
	}
}

func TestRunDemoMode_AllAcceptedShowsReadyForUAT(t *testing.T) {
	// GIVEN DemoMode starts at the final test
	// WHEN user accepts the executed test with N
	// THEN readiness summary should report Ready for UAT.
	var output bytes.Buffer
	input := strings.NewReader("N\n")

	err := runDemoMode(input, &output, "e2e-006", nil)
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
	if !strings.Contains(content, "[AgentX Demo]   - e2e-001: SKIP") {
		t.Fatalf("expected skipped status for pre-start tests, got:\n%s", content)
	}
	if !strings.Contains(content, "[AgentX Demo]   - e2e-002: SKIP") {
		t.Fatalf("expected skipped status for pre-start tests, got:\n%s", content)
	}
	if !strings.Contains(content, "[AgentX Demo]   - e2e-006: PASS") {
		t.Fatalf("expected pass status for accepted final test, got:\n%s", content)
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
if [[ "$1" == "list-windows" ]]; then
	if [[ "$3" == "demo-session_demo" ]]; then
		echo '0|demo-control|1'
	else
		echo '0|tui-chat|1'
		echo '1|logs|0'
	fi
	exit 0
fi
if [[ "$1" == "list-panes" ]]; then
	if [[ "$3" == "demo-session_demo" ]]; then
		echo 'demo-session_demo|0|0|%3|controller|1'
		echo 'demo-session_demo|0|1|%4|live-core|0'
		exit 0
	fi
	echo 'demo-session|0|0|%1|chat|0'
	echo 'demo-session|0|1|%2|context|1'
  exit 0
fi
if [[ "$1" == "display-message" ]]; then
	if [[ "$4" == "demo-session_demo" ]]; then
		echo 'demo-session_demo:0.0'
	else
		echo 'demo-session:0.0'
	fi
  exit 0
fi
if [[ "$1" == "capture-pane" ]]; then
	if [[ "$6" == "%1" ]]; then
    echo 'chat pane content'
	elif [[ "$6" == "%2" ]]; then
		echo 'context pane content'
	elif [[ "$6" == "%3" ]]; then
		echo 'controller pane content'
  else
		echo 'live core pane content'
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
		"",
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
		"core_tmux_list_windows.txt",
		"core_tmux_list_panes.txt",
		"core_tmux_display_message.txt",
		"core_pane_%1.txt",
		"core_pane_%2.txt",
		"split_tmux_list_windows.txt",
		"split_tmux_list_panes.txt",
		"split_tmux_display_message.txt",
		"split_pane_%3.txt",
		"split_pane_%4.txt",
	}

	for _, name := range expectedFiles {
		if _, statErr := os.Stat(filepath.Join(artifactDir, name)); statErr != nil {
			t.Fatalf("expected artifact file %s to exist: %v", name, statErr)
		}
	}
}

func TestRunDemoModeWithConfigAndContext_SplitViewSuppressesSequenceAndClearsPane(t *testing.T) {
	// GIVEN split-view controller mode with a stories board file
	// WHEN the first test is marked failed using X
	// THEN controller output is compact (no duplicated sequence) and pane-clear ANSI is emitted.
	var output bytes.Buffer
	input := strings.NewReader("X split mode failure feedback\n")
	storiesFilePath := filepath.Join(t.TempDir(), "stories_board.txt")

	err := runDemoModeWithConfigAndContext(
		context.Background(),
		input,
		&output,
		"",
		nil,
		DemoRuntimeConfig{
			SplitView:       true,
			StoriesFilePath: storiesFilePath,
		},
	)
	if err != nil {
		t.Fatalf("expected split view demo mode to succeed, got %v", err)
	}

	content := output.String()
	if strings.Contains(content, "[AgentX Demo] Ordered test sequence (Gherkin):") {
		t.Fatalf("expected split view to suppress full sequence in controller pane, got:\n%s", content)
	}
	if !strings.Contains(content, "\033[H\033[2J") {
		t.Fatalf("expected split view controller pane clear ANSI sequence, got:\n%s", content)
	}
	if !strings.Contains(content, "[AgentX Demo] Controller Pane") {
		t.Fatalf("expected compact controller header in split view, got:\n%s", content)
	}
}

func TestRenderDemoStoriesBoard_ShowsStatusMarkers(t *testing.T) {
	// GIVEN a demo sequence with mixed statuses
	// WHEN rendering the stories board with an active test
	// THEN status markers are shown inline for pending, active, pass, and fail states.
	sequence := defaultDemoSequence()
	statusByTestID := map[string]string{}
	for _, testCase := range sequence {
		statusByTestID[testCase.ID] = demoStatusSkip
	}
	statusByTestID["e2e-001"] = demoStatusPass
	statusByTestID["e2e-003"] = demoStatusFail

	board := renderDemoStoriesBoard(sequence, 0, statusByTestID, "e2e-002")

	if !strings.Contains(board, "1 [P] e2e-001") {
		t.Fatalf("expected pass marker for first test, board:\n%s", board)
	}
	if !strings.Contains(board, "2 [/] e2e-002") {
		t.Fatalf("expected active marker for second test, board:\n%s", board)
	}
	if !strings.Contains(board, "3 [X] e2e-003") {
		t.Fatalf("expected fail marker for third test, board:\n%s", board)
	}
	if !strings.Contains(board, "4 [S] e2e-004") {
		t.Fatalf("expected pending marker for unrun test, board:\n%s", board)
	}
	if !strings.Contains(board, "Scroll affordance") {
		t.Fatalf("expected scroll affordance guidance in stories board, board:\n%s", board)
	}
}
