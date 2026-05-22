// Package main contains DemoMode scaffolding helpers.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	Given          string
	When           string
	Then           string
}

// DemoTestRunner executes one demo test case and returns a short result string.
type DemoTestRunner func(DemoTestCase) (string, error)

// DemoRuntimeConfig contains runtime values needed for diagnostics capture.
type DemoRuntimeConfig struct {
	ProjectDir      string
	Username        string
	SessionID       string
	TmuxSessionName string
	HealthAddr      string
}

func (cfg DemoRuntimeConfig) HealthURL() string {
	addr := strings.TrimSpace(cfg.HealthAddr)
	if addr == "" {
		addr = "127.0.0.1:9876"
	}
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/")
	}
	return "http://" + strings.TrimRight(addr, "/")
}

// DemoDiagnosticsCollector captures diagnostics artifacts when user marks a test failed.
type DemoDiagnosticsCollector func(DemoRuntimeConfig, DemoTestCase) ([]string, error)

type demoModeOptions struct {
	runtimeConfig DemoRuntimeConfig
	collector     DemoDiagnosticsCollector
	decisionCtx   context.Context
}

const (
	demoStatusPass = "PASS"
	demoStatusFail = "FAIL"
	demoStatusSkip = "SKIP"
)

// defaultDemoSequence returns the stable, ordered demo manifest for D1.
func defaultDemoSequence() []DemoTestCase {
	return []DemoTestCase{
		{
			ID:             "e2e-001",
			Title:          "Prompt route updates chat and context",
			Prompt:         "hello from input",
			ApproxDuration: "~15s",
			Tags:           []string{"e2e", "routing", "context"},
			Given:          "core session and applets are running",
			When:           "a normal input prompt is submitted",
			Then:           "live-core Chat pane and live-core Context pane both update with the same turn",
		},
		{
			ID:             "e2e-002",
			Title:          "Input command contract clear/quit",
			Prompt:         ":clear",
			ApproxDuration: "~10s",
			Tags:           []string{"e2e", "input"},
			Given:          "chat contains visible prior output",
			When:           "the :clear input command is submitted",
			Then:           "live-core Chat pane is cleared and live-core Input pane prompt remains active",
		},
		{
			ID:             "e2e-003",
			Title:          "Session shutdown path",
			Prompt:         ":q",
			ApproxDuration: "~10s",
			Tags:           []string{"e2e", "shutdown"},
			Given:          "controller has reached the final E2E test",
			When:           "the :q command is submitted",
			Then:           "live right pane exits and displays completion guidance",
		},
	}
}

func sanitizeDemoResultText(resultText string) string {
	trimmed := strings.TrimSpace(resultText)
	if trimmed == "" {
		return "ok"
	}
	oneLine := strings.Join(strings.Fields(trimmed), " ")
	if len(oneLine) > 120 {
		return oneLine[:120] + "..."
	}
	return oneLine
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
	return runDemoModeWithConfigAndContext(context.Background(), reader, writer, startSelector, runner, runtimeConfig)
}

func runDemoModeWithConfigAndContext(
	decisionCtx context.Context,
	reader io.Reader,
	writer io.Writer,
	startSelector string,
	runner DemoTestRunner,
	runtimeConfig DemoRuntimeConfig,
) error {
	if decisionCtx == nil {
		decisionCtx = context.Background()
	}

	return runDemoModeWithOptions(
		reader,
		writer,
		startSelector,
		runner,
		demoModeOptions{runtimeConfig: runtimeConfig, decisionCtx: decisionCtx},
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
	if options.decisionCtx == nil {
		options.decisionCtx = context.Background()
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
	statusByTestID := map[string]string{}
	for _, testCase := range sequence {
		statusByTestID[testCase.ID] = demoStatusSkip
	}

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
		runStatus := demoStatusPass
		if runErr != nil {
			fmt.Fprintf(writer, "\r[AgentX Demo] Result: FAIL (%v)\n", runErr)
			runStatus = demoStatusFail
		} else {
			fmt.Fprintf(writer, "\r[AgentX Demo] Result: PASS (%s)\n", sanitizeDemoResultText(resultText))
		}

		decision, promptErr := readDemoDecision(options.decisionCtx, inputReader, writer)
		if promptErr != nil {
			return promptErr
		}

		if decision == "X" {
			failedTestID = testCase.ID
			runStatus = demoStatusFail
			statusByTestID[testCase.ID] = runStatus
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

		statusByTestID[testCase.ID] = runStatus
		if runStatus == demoStatusPass {
			acceptedCount++
			fmt.Fprintf(writer, "[AgentX Demo] Accepted %s, advancing\n", testCase.ID)
		} else {
			fmt.Fprintf(writer, "[AgentX Demo] Recorded %s for %s, advancing\n", runStatus, testCase.ID)
		}
	}

	renderDemoSummary(writer, sequence, selectedCount, runCount, acceptedCount, failedTestID, artifactPaths, statusByTestID)
	return nil
}

func renderDemoSequence(writer io.Writer, sequence []DemoTestCase, startIndex int) {
	fmt.Fprintln(writer, "[AgentX Demo] Mode enabled")
	fmt.Fprintln(writer, "[AgentX Demo] Ordered test sequence (Gherkin):")
	for idx, testCase := range sequence {
		marker := " "
		if idx == startIndex {
			marker = "*"
		}
		fmt.Fprintf(writer, "  %s %d) %s (%s)\n", marker, idx+1, testCase.ID, testCase.ApproxDuration)
		fmt.Fprintf(writer, "      Name: %s\n", testCase.Title)
		fmt.Fprintf(writer, "      GIVEN %s\n", testCase.Given)
		fmt.Fprintf(writer, "      WHEN  %s\n", testCase.When)
		fmt.Fprintf(writer, "      THEN  %s\n", testCase.Then)
	}

	fmt.Fprintf(
		writer,
		"[AgentX Demo] Start selection: %d) %s\n",
		startIndex+1,
		sequence[startIndex].ID,
	)
}

func readDemoDecision(decisionCtx context.Context, reader *bufio.Reader, writer io.Writer) (string, error) {
	for {
		select {
		case <-decisionCtx.Done():
			return "", fmt.Errorf("demo decision cancelled: %w", decisionCtx.Err())
		default:
		}

		fmt.Fprint(writer, "\n[AgentX Demo] Enter decision [N=next, X=fail]: ")
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

func renderDemoSummary(
	writer io.Writer,
	sequence []DemoTestCase,
	selectedCount,
	runCount,
	acceptedCount int,
	failedTestID string,
	artifactPaths []string,
	statusByTestID map[string]string,
) {
	fmt.Fprintln(writer, "[AgentX Demo] Run summary:")
	fmt.Fprintf(writer, "[AgentX Demo] Selected tests: %d\n", selectedCount)
	fmt.Fprintf(writer, "[AgentX Demo] Tests run: %d\n", runCount)
	fmt.Fprintf(writer, "[AgentX Demo] Accepted tests: %d\n", acceptedCount)
	fmt.Fprintln(writer, "[AgentX Demo] Status ledger:")
	for _, testCase := range sequence {
		status := statusByTestID[testCase.ID]
		if strings.TrimSpace(status) == "" {
			status = demoStatusSkip
		}
		fmt.Fprintf(writer, "[AgentX Demo]   - %s: %s\n", testCase.ID, status)
	}
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

	coreSessionName := strings.TrimSpace(runtimeConfig.TmuxSessionName)
	if coreSessionName != "" {
		captureTmuxSessionDiagnostics(artifactDir, "core", coreSessionName)
		captureTmuxSessionDiagnostics(artifactDir, "split", coreSessionName+"_demo")
	}

	return []string{artifactDir}, nil
}

func captureTmuxSessionDiagnostics(artifactDir, prefix, sessionName string) {
	paneTargets := []string{}

	listWindowsOutput, listWindowsErr := runTmuxCommand("", "list-windows", "-t", sessionName, "-F", "#{window_index}|#{window_name}|#{window_active}")
	if listWindowsErr != nil {
		_ = os.WriteFile(filepath.Join(artifactDir, fmt.Sprintf("%s_tmux_list_windows.error.txt", prefix)), []byte(listWindowsErr.Error()+"\n"), 0o644)
	} else {
		_ = os.WriteFile(filepath.Join(artifactDir, fmt.Sprintf("%s_tmux_list_windows.txt", prefix)), []byte(listWindowsOutput), 0o644)
	}

	listPanesOutput, listPanesErr := runTmuxCommand(
		"",
		"list-panes",
		"-t",
		sessionName,
		"-F",
		"#{session_name}|#{window_index}|#{pane_index}|#{pane_id}|#{pane_title}|#{pane_active}",
	)
	if listPanesErr != nil {
		_ = os.WriteFile(filepath.Join(artifactDir, fmt.Sprintf("%s_tmux_list_panes.error.txt", prefix)), []byte(listPanesErr.Error()+"\n"), 0o644)
	} else {
		_ = os.WriteFile(filepath.Join(artifactDir, fmt.Sprintf("%s_tmux_list_panes.txt", prefix)), []byte(listPanesOutput), 0o644)
		for _, line := range strings.Split(strings.TrimSpace(listPanesOutput), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			parts := strings.Split(line, "|")
			if len(parts) < 4 {
				continue
			}
			paneTargets = append(paneTargets, strings.TrimSpace(parts[3]))
		}
	}

	displayOutput, displayErr := runTmuxCommand("", "display-message", "-p", "-t", sessionName, "#{session_name}:#{window_index}.#{pane_index}")
	if displayErr != nil {
		_ = os.WriteFile(filepath.Join(artifactDir, fmt.Sprintf("%s_tmux_display_message.error.txt", prefix)), []byte(displayErr.Error()+"\n"), 0o644)
	} else {
		_ = os.WriteFile(filepath.Join(artifactDir, fmt.Sprintf("%s_tmux_display_message.txt", prefix)), []byte(displayOutput), 0o644)
	}

	for _, paneTarget := range paneTargets {
		captureOutput, captureErr := runTmuxCommand("", "capture-pane", "-p", "-S", "-200", "-t", paneTarget)
		capturePath := filepath.Join(artifactDir, fmt.Sprintf("%s_pane_%s.txt", prefix, sanitizePathComponent(paneTarget)))
		if captureErr != nil {
			_ = os.WriteFile(capturePath, []byte("capture error: "+captureErr.Error()+"\n"), 0o644)
			continue
		}
		_ = os.WriteFile(capturePath, []byte(captureOutput), 0o644)
	}

	// Backward-compatible filenames for existing tooling/scripts that read legacy artifact names.
	if prefix == "core" {
		copyFileIfPresent(filepath.Join(artifactDir, "core_tmux_list_panes.txt"), filepath.Join(artifactDir, "tmux_list_panes.txt"))
		copyFileIfPresent(filepath.Join(artifactDir, "core_tmux_list_panes.error.txt"), filepath.Join(artifactDir, "tmux_list_panes.error.txt"))
		copyFileIfPresent(filepath.Join(artifactDir, "core_tmux_display_message.txt"), filepath.Join(artifactDir, "tmux_display_message.txt"))
		copyFileIfPresent(filepath.Join(artifactDir, "core_tmux_display_message.error.txt"), filepath.Join(artifactDir, "tmux_display_message.error.txt"))
		for _, paneTarget := range paneTargets {
			sourcePath := filepath.Join(artifactDir, fmt.Sprintf("core_pane_%s.txt", sanitizePathComponent(paneTarget)))
			targetPath := filepath.Join(artifactDir, fmt.Sprintf("pane_%s.txt", sanitizePathComponent(paneTarget)))
			copyFileIfPresent(sourcePath, targetPath)
		}
	}
}

func copyFileIfPresent(sourcePath, targetPath string) {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return
	}
	_ = os.WriteFile(targetPath, data, 0o644)
}

func submitDemoPrompt(ctx context.Context, healthURL, prompt string) (string, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(healthURL), "/") + "/submit"
	requestBody, err := json.Marshal(map[string]string{"prompt": prompt})
	if err != nil {
		return "", fmt.Errorf("failed to encode demo submit request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return "", fmt.Errorf("failed to create demo submit request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("failed to post demo submit request: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read demo submit response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("demo submit returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("failed to decode demo submit response: %w", err)
	}

	return payload.Response, nil
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
