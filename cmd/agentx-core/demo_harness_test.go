package main

import (
	"bytes"
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

	err := runDemoMode(input, &output, "e2e-002", runner)
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
	if !strings.Contains(content, "[AgentX Demo] Artifact paths: none (D3 diagnostics pending)") {
		t.Fatalf("expected artifact-path summary line, got:\n%s", content)
	}
	if !strings.Contains(content, "[AgentX Demo] Readiness: Not ready for UAT") {
		t.Fatalf("expected not-ready summary, got:\n%s", content)
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
}
