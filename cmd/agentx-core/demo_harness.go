// Package main contains DemoMode scaffolding helpers.
package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// DemoTestCase defines one demo scenario entry shown to users.
type DemoTestCase struct {
	ID             string
	Title          string
	ApproxDuration string
	Tags           []string
}

// defaultDemoSequence returns the stable, ordered demo manifest for D1.
func defaultDemoSequence() []DemoTestCase {
	return []DemoTestCase{
		{
			ID:             "e2e-001",
			Title:          "Prompt route updates chat and context",
			ApproxDuration: "~15s",
			Tags:           []string{"e2e", "routing", "context"},
		},
		{
			ID:             "e2e-002",
			Title:          "Input command contract clear/quit",
			ApproxDuration: "~10s",
			Tags:           []string{"e2e", "input"},
		},
		{
			ID:             "e2e-003",
			Title:          "Session shutdown path",
			ApproxDuration: "~10s",
			Tags:           []string{"e2e", "shutdown"},
		},
	}
}

// resolveDemoStartIndex resolves a start selector from ID or 1-based index.
func resolveDemoStartIndex(sequence []DemoTestCase, selector string) (int, error) {
	if len(sequence) == 0 {
		return 0, fmt.Errorf("demo sequence is empty")
	}

	trimmed := strings.TrimSpace(selector)
	if trimmed == "" {
		return 0, nil
	}

	for idx, testCase := range sequence {
		if strings.EqualFold(testCase.ID, trimmed) {
			return idx, nil
		}
	}

	oneBasedIndex, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("invalid --demo-start value %q (expected test id or 1-based index)", selector)
	}

	if oneBasedIndex < 1 || oneBasedIndex > len(sequence) {
		return 0, fmt.Errorf("invalid --demo-start index %d (valid range 1..%d)", oneBasedIndex, len(sequence))
	}

	return oneBasedIndex - 1, nil
}

// runDemoScaffolding renders the D1 sequence contract and exits without execution.
func runDemoScaffolding(writer io.Writer, startSelector string) error {
	sequence := defaultDemoSequence()
	startIndex, err := resolveDemoStartIndex(sequence, startSelector)
	if err != nil {
		return err
	}

	fmt.Fprintln(writer, "[AgentX Demo] Mode enabled")
	fmt.Fprintln(writer, "[AgentX Demo] Ordered test sequence:")
	for idx, testCase := range sequence {
		marker := " "
		if idx == startIndex {
			marker = "*"
		}
		fmt.Fprintf(
			writer,
			"  %s %d) %s - %s (%s) tags=%s\n",
			marker,
			idx+1,
			testCase.ID,
			testCase.Title,
			testCase.ApproxDuration,
			strings.Join(testCase.Tags, ","),
		)
	}

	fmt.Fprintf(
		writer,
		"[AgentX Demo] Start selection: %d) %s\n",
		startIndex+1,
		sequence[startIndex].ID,
	)
	fmt.Fprintln(writer, "[AgentX Demo] Scaffolding only: demo execution loop is not implemented yet")

	return nil
}
