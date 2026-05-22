// Package main contains DemoMode scaffolding helpers.
package main

import (
	"bufio"
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

// DemoTestRunner executes one demo test case and returns a short result string.
type DemoTestRunner func(DemoTestCase) (string, error)

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

// runDemoMode executes the D2 interactive review loop for selected demo tests.
func runDemoMode(reader io.Reader, writer io.Writer, startSelector string, runner DemoTestRunner) error {
	if runner == nil {
		runner = defaultDemoTestRunner
	}

	sequence := defaultDemoSequence()
	startIndex, err := resolveDemoStartIndex(sequence, startSelector)
	if err != nil {
		return err
	}

	renderDemoSequence(writer, sequence, startIndex)

	selectedCount := len(sequence) - startIndex
	runCount := 0
	acceptedCount := 0
	failedTestID := ""

	inputReader := bufio.NewReader(reader)
	for idx := startIndex; idx < len(sequence); idx++ {
		testCase := sequence[idx]
		runCount++

		fmt.Fprintf(
			writer,
			"[AgentX Demo] Running test %d/%d: %s - %s\n",
			runCount,
			selectedCount,
			testCase.ID,
			testCase.Title,
		)

		resultText, runErr := runner(testCase)
		if runErr != nil {
			fmt.Fprintf(writer, "[AgentX Demo] Result: FAIL (%v)\n", runErr)
		} else {
			fmt.Fprintf(writer, "[AgentX Demo] Result: PASS (%s)\n", resultText)
		}

		decision, promptErr := readDemoDecision(inputReader, writer)
		if promptErr != nil {
			return promptErr
		}

		if decision == "X" {
			failedTestID = testCase.ID
			fmt.Fprintf(writer, "[AgentX Demo] Marked failed by user at %s\n", testCase.ID)
			break
		}

		acceptedCount++
		fmt.Fprintf(writer, "[AgentX Demo] Accepted %s, advancing\n", testCase.ID)
	}

	renderDemoSummary(writer, selectedCount, runCount, acceptedCount, failedTestID)
	return nil
}

func renderDemoSequence(writer io.Writer, sequence []DemoTestCase, startIndex int) {
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
}

func readDemoDecision(reader *bufio.Reader, writer io.Writer) (string, error) {
	for {
		fmt.Fprint(writer, "[AgentX Demo] Enter decision [N=next, X=fail]: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read demo decision: %w", err)
		}

		decision := strings.ToUpper(strings.TrimSpace(line))
		if decision == "N" || decision == "X" {
			return decision, nil
		}

		fmt.Fprintln(writer, "[AgentX Demo] Invalid decision; enter N or X")
	}
}

func renderDemoSummary(writer io.Writer, selectedCount, runCount, acceptedCount int, failedTestID string) {
	fmt.Fprintln(writer, "[AgentX Demo] Run summary:")
	fmt.Fprintf(writer, "[AgentX Demo] Selected tests: %d\n", selectedCount)
	fmt.Fprintf(writer, "[AgentX Demo] Tests run: %d\n", runCount)
	fmt.Fprintf(writer, "[AgentX Demo] Accepted tests: %d\n", acceptedCount)
	fmt.Fprintln(writer, "[AgentX Demo] Artifact paths: none (D3 diagnostics pending)")
	if failedTestID != "" {
		fmt.Fprintf(writer, "[AgentX Demo] Failed test: %s\n", failedTestID)
		fmt.Fprintln(writer, "[AgentX Demo] Readiness: Not ready for UAT")
		return
	}

	if acceptedCount == selectedCount {
		fmt.Fprintln(writer, "[AgentX Demo] Readiness: Ready for UAT")
		return
	}

	fmt.Fprintln(writer, "[AgentX Demo] Readiness: Not ready for UAT")
}

func defaultDemoTestRunner(testCase DemoTestCase) (string, error) {
	return "scaffold execution placeholder", nil
}
