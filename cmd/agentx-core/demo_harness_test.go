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

func TestRunDemoScaffolding_PrintsOrderedSequenceAndStartMarker(t *testing.T) {
	// GIVEN demo mode scaffolding with a selected start id
	// WHEN the scaffold output is rendered
	// THEN the ordered sequence and selected start marker should be displayed.
	var output bytes.Buffer

	err := runDemoScaffolding(&output, "e2e-002")
	if err != nil {
		t.Fatalf("expected demo scaffolding to succeed, got %v", err)
	}

	content := output.String()
	if !strings.Contains(content, "[AgentX Demo] Ordered test sequence:") {
		t.Fatalf("expected ordered sequence heading, got:\n%s", content)
	}
	if !strings.Contains(content, "* 2) e2e-002 - Input command contract clear/quit") {
		t.Fatalf("expected selected start marker for e2e-002, got:\n%s", content)
	}
	if !strings.Contains(content, "[AgentX Demo] Scaffolding only: demo execution loop is not implemented yet") {
		t.Fatalf("expected scaffolding notice, got:\n%s", content)
	}
}
