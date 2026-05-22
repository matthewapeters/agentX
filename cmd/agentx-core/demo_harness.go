// Package main contains DemoMode scaffolding helpers.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DemoTestCase defines one demo scenario entry shown to users.
type DemoTestCase struct {
	ID             string
	Title          string
	Prompt         string
	ApproxDuration string
	Tags           []string
}

// DemoTestRunner executes one demo test case and returns a short result string.
type DemoTestRunner func(DemoTestCase) (string, error)

// DemoRuntimeConfig contains runtime values needed for diagnostics capture.
type DemoRuntimeConfig struct {
	ProjectDir      string
	Username        string
	SessionID       string
	TmuxSessionName string
}

// DemoDiagnosticsCollector captures diagnostics artifacts when user marks a test failed.
type DemoDiagnosticsCollector func(DemoRuntimeConfig, DemoTestCase) ([]string, error)

type demoModeOptions struct {
	runtimeConfig DemoRuntimeConfig
	collector     DemoDiagnosticsCollector
}

// defaultDemoSequence returns the stable, ordered demo manifest for D1.
func defaultDemoSequence() []DemoTestCase {
	return []DemoTestCase{
		{
			ID:             "e2e-001",
			Title:          "Prompt route updates chat and context",
			Prompt:         "hello from input",
			ApproxDuration: "~15s",
			Tags:           []string{"e2e", "routing", "context"},
		},
		{
			ID:             "e2e-002",
			Title:          "Input command contract clear/quit",
			Prompt:         ":clear",
			ApproxDuration: "~10s",
			Tags:           []string{"e2e", "input"},
		},
		{
			ID:             "e2e-003",
			Title:          "Session shutdown path",
			Prompt:         ":q",
			ApproxDuration: "~10s",
			Tags:           []string{"e2e", "shutdown"},
		},
	}
}

func demoPromptForTestCase(testCase DemoTestCase) string {
	trimmed := strings.TrimSpace(testCase.Prompt)
	if trimmed != "" {
		return trimmed
	}

	switch testCase.ID {
	case "e2e-001":
		return "hello from input"
	case "e2e-002":
		return ":clear"
	case "e2e-003":
		return ":q"
	default:
		return strings.TrimSpace(testCase.Title)
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
	return runDemoModeWithOptions(reader, writer, startSelector, runner, demoModeOptions{})
}

func runDemoModeWithConfig(
	reader io.Reader,
	writer io.Writer,
	startSelector string,
	runner DemoTestRunner,
	runtimeConfig DemoRuntimeConfig,
) error {
	return runDemoModeWithOptions(
		reader,
		writer,
		startSelector,
		runner,
		demoModeOptions{runtimeConfig: runtimeConfig},
	)
}

func runDemoModeWithOptions(
	reader io.Reader,
	writer io.Writer,
	startSelector string,
	runner DemoTestRunner,
	options demoModeOptions,
) error {
	if runner == nil {
		runner = defaultDemoTestRunner
	}
	if options.collector == nil {
		options.collector = defaultDemoDiagnosticsCollector
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
	artifactPaths := []string{}

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
			capturedPaths, captureErr := options.collector(options.runtimeConfig, testCase)
			if len(capturedPaths) > 0 {
				artifactPaths = capturedPaths
			}
			if captureErr != nil {
				fmt.Fprintf(writer, "[AgentX Demo] Diagnostics capture failed: %v\n", captureErr)
			}
			fmt.Fprintf(writer, "[AgentX Demo] Marked failed by user at %s\n", testCase.ID)
			break
		}

		acceptedCount++
		fmt.Fprintf(writer, "[AgentX Demo] Accepted %s, advancing\n", testCase.ID)
	}

	renderDemoSummary(writer, selectedCount, runCount, acceptedCount, failedTestID, artifactPaths)
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

func renderDemoSummary(writer io.Writer, selectedCount, runCount, acceptedCount int, failedTestID string, artifactPaths []string) {
	fmt.Fprintln(writer, "[AgentX Demo] Run summary:")
	fmt.Fprintf(writer, "[AgentX Demo] Selected tests: %d\n", selectedCount)
	fmt.Fprintf(writer, "[AgentX Demo] Tests run: %d\n", runCount)
	fmt.Fprintf(writer, "[AgentX Demo] Accepted tests: %d\n", acceptedCount)
	if len(artifactPaths) == 0 {
		fmt.Fprintln(writer, "[AgentX Demo] Artifact paths: none")
	} else {
		fmt.Fprintf(writer, "[AgentX Demo] Artifact paths: %s\n", strings.Join(artifactPaths, ", "))
	}
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

func defaultDemoDiagnosticsCollector(runtimeConfig DemoRuntimeConfig, testCase DemoTestCase) ([]string, error) {
	projectDir := strings.TrimSpace(runtimeConfig.ProjectDir)
	if projectDir == "" {
		return nil, nil
	}

	sessionID := strings.TrimSpace(runtimeConfig.SessionID)
	if sessionID == "" {
		sessionID = fmt.Sprintf("demo_%d", time.Now().Unix())
	}

	artifactDir := filepath.Join(projectDir, "logs", "demo", sanitizePathComponent(sessionID), sanitizePathComponent(testCase.ID))
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create diagnostics directory: %w", err)
	}

	metadata := map[string]string{
		"session_id":        sessionID,
		"test_id":           testCase.ID,
		"test_title":        testCase.Title,
		"username":          runtimeConfig.Username,
		"tmux_session_name": runtimeConfig.TmuxSessionName,
		"timestamp_utc":     time.Now().UTC().Format(time.RFC3339),
	}

	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to encode diagnostics metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "metadata.json"), metadataJSON, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write diagnostics metadata: %w", err)
	}

	paneTargets := []string{}
	listOutput, listErr := runTmuxCommand(runtimeConfig.TmuxSessionName, "list-panes", "-a", "-F", "#{pane_id}|#{pane_title}")
	if listErr != nil {
		_ = os.WriteFile(filepath.Join(artifactDir, "tmux_list_panes.error.txt"), []byte(listErr.Error()+"\n"), 0o644)
	} else {
		_ = os.WriteFile(filepath.Join(artifactDir, "tmux_list_panes.txt"), []byte(listOutput), 0o644)
		for _, line := range strings.Split(strings.TrimSpace(listOutput), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			parts := strings.SplitN(line, "|", 2)
			paneTargets = append(paneTargets, strings.TrimSpace(parts[0]))
		}
	}

	displayOutput, displayErr := runTmuxCommand(runtimeConfig.TmuxSessionName, "display-message", "-p", "#{session_name}:#{window_index}.#{pane_index}")
	if displayErr != nil {
		_ = os.WriteFile(filepath.Join(artifactDir, "tmux_display_message.error.txt"), []byte(displayErr.Error()+"\n"), 0o644)
	} else {
		_ = os.WriteFile(filepath.Join(artifactDir, "tmux_display_message.txt"), []byte(displayOutput), 0o644)
	}

	for _, paneTarget := range paneTargets {
		captureOutput, captureErr := runTmuxCommand(runtimeConfig.TmuxSessionName, "capture-pane", "-p", "-t", paneTarget)
		capturePath := filepath.Join(artifactDir, fmt.Sprintf("pane_%s.txt", sanitizePathComponent(paneTarget)))
		if captureErr != nil {
			_ = os.WriteFile(capturePath, []byte("capture error: "+captureErr.Error()+"\n"), 0o644)
			continue
		}
		_ = os.WriteFile(capturePath, []byte(captureOutput), 0o644)
	}

	return []string{artifactDir}, nil
}

func runTmuxCommand(tmuxSessionName string, args ...string) (string, error) {
	commandArgs := make([]string, 0, len(args)+2)
	commandArgs = append(commandArgs, args...)
	hasExplicitTarget := false
	for idx, value := range commandArgs {
		if value == "-t" && idx+1 < len(commandArgs) {
			hasExplicitTarget = true
			break
		}
	}

	if strings.TrimSpace(tmuxSessionName) != "" && !hasExplicitTarget {
		commandArgs = append(commandArgs, "-t", tmuxSessionName)
	}

	cmd := exec.Command("tmux", commandArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s failed: %w (%s)", strings.Join(commandArgs, " "), err, strings.TrimSpace(string(output)))
	}

	return string(output), nil
}

func sanitizePathComponent(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unknown"
	}
	cleaned := strings.ReplaceAll(trimmed, "/", "_")
	cleaned = strings.ReplaceAll(cleaned, " ", "_")
	return cleaned
}
