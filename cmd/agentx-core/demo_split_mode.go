package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func runDemoSplitMode(ctx context.Context, cfg *Config, core *AgentXCore, startSelector string) error {
	if core == nil {
		return fmt.Errorf("demo split mode requires a live core")
	}

	healthAddr := strings.TrimSpace(core.healthAddr)
	if healthAddr == "" {
		healthAddr = "127.0.0.1:9876"
	}
	if err := waitForDemoHealthEndpoint(ctx, "http://"+healthAddr); err != nil {
		return err
	}

	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	demoSessionName := fmt.Sprintf("%s_demo", core.tmuxSessionName)
	storiesFilePath, err := prepareDemoStoriesBoardFile(cfg.ProjectDir, core.SessionID, startSelector)
	if err != nil {
		return err
	}
	controllerArgs := buildDemoControllerArgs(
		executablePath,
		cfg,
		core.SessionID,
		startSelector,
		core.tmuxSessionName,
		storiesFilePath,
	)
	storiesArgs := buildDemoStoriesPaneArgs(storiesFilePath)

	if err := prepareCoreSessionForSplitView(ctx, core.tmuxSessionName); err != nil {
		return fmt.Errorf("failed to prepare core session for split view: %w", err)
	}

	storiesPaneID, err := runTmuxCapture(ctx, append([]string{"new-session", "-d", "-P", "-F", "#{pane_id}", "-s", demoSessionName, "-n", "demo-control"}, storiesArgs...)...)
	if err != nil {
		return fmt.Errorf("failed to create demo controller session: %w", err)
	}
	storiesPaneID = strings.TrimSpace(storiesPaneID)
	if storiesPaneID == "" {
		return fmt.Errorf("failed to capture stories pane id")
	}

	liveCoreMirrorArgs := buildLiveCoreMirrorArgs(core.tmuxSessionName)
	liveCorePaneID, err := runTmuxCapture(ctx, append([]string{"split-window", "-P", "-F", "#{pane_id}", "-h", "-p", "45", "-t", storiesPaneID}, liveCoreMirrorArgs...)...)
	if err != nil {
		return fmt.Errorf("failed to create live core mirror pane: %w", err)
	}
	liveCorePaneID = strings.TrimSpace(liveCorePaneID)
	if liveCorePaneID == "" {
		return fmt.Errorf("failed to capture live core pane id")
	}

	controllerPaneID, err := runTmuxCapture(ctx, append([]string{"split-window", "-P", "-F", "#{pane_id}", "-v", "-p", "35", "-t", storiesPaneID}, controllerArgs...)...)
	if err != nil {
		return fmt.Errorf("failed to create demo prompt pane: %w", err)
	}
	controllerPaneID = strings.TrimSpace(controllerPaneID)
	if controllerPaneID == "" {
		return fmt.Errorf("failed to capture controller pane id")
	}

	if err := runTmuxInteractive(ctx, "select-pane", "-t", storiesPaneID, "-T", "stories"); err != nil {
		return fmt.Errorf("failed to label stories pane: %w", err)
	}
	if err := runTmuxInteractive(ctx, "select-pane", "-t", controllerPaneID, "-T", "controller"); err != nil {
		return fmt.Errorf("failed to label controller pane: %w", err)
	}
	if err := runTmuxInteractive(ctx, "select-pane", "-t", liveCorePaneID, "-T", "live-core"); err != nil {
		return fmt.Errorf("failed to label live core pane: %w", err)
	}
	if err := runTmuxInteractive(ctx, "select-pane", "-t", controllerPaneID); err != nil {
		return fmt.Errorf("failed to focus controller pane: %w", err)
	}

	fmt.Printf("[AgentX Demo] Split demo session initialized: %s\n", demoSessionName)
	fmt.Printf("[AgentX Demo] Left-top pane: story browser\n")
	fmt.Printf("[AgentX Demo] Left-bottom pane: controller prompt loop\n")
	fmt.Printf("[AgentX Demo] Right pane: live core session %s\n", core.tmuxSessionName)
	fmt.Printf("[AgentX Demo] Attach to the split demo session with: tmux attach -t %s\n", demoSessionName)

	attachErr := attachTmuxSession(ctx, demoSessionName)
	if killErr := runTmux(ctx, "kill-session", "-t", demoSessionName); killErr != nil && !isTmuxMissingSessionError(killErr) {
		fmt.Printf("[AgentX Demo] Warning: failed to clean up demo session %s: %v\n", demoSessionName, killErr)
	}
	if attachErr != nil {
		return attachErr
	}

	return nil
}

func buildDemoControllerArgs(executablePath string, cfg *Config, sessionID, startSelector, coreSessionName, storiesFilePath string) []string {
	args := []string{
		"--project-dir", cfg.ProjectDir,
		"--user", cfg.Username,
		"--session-id", sessionID,
		"--demo-controller",
		"--demo-split",
		"--demo-core-session", coreSessionName,
		"--demo-stories-file", storiesFilePath,
	}
	if trimmed := strings.TrimSpace(startSelector); trimmed != "" {
		args = append(args, "--demo-start", trimmed)
	}
	return append([]string{executablePath}, args...)
}

func buildLiveCoreMirrorArgs(coreSessionName string) []string {
	attachScript := fmt.Sprintf(
		`TMUX= tmux attach-session -r -t %s; printf '\n[AgentX Demo] demo complete. Press N or X to exit\n'; tail -f /dev/null`,
		shellQuote(coreSessionName),
	)
	return []string{"bash", "-lc", attachScript}
}

func buildDemoStoriesPaneArgs(storiesFilePath string) []string {
	storiesScript := fmt.Sprintf("if command -v less >/dev/null 2>&1; then exec less -R -c +g %s; else exec tail -n +1 -f %s; fi", shellQuote(storiesFilePath), shellQuote(storiesFilePath))
	return []string{"bash", "-lc", storiesScript}
}

func prepareDemoStoriesBoardFile(projectDir, sessionID, startSelector string) (string, error) {
	sequence := defaultDemoSequence()
	startIndex, err := resolveDemoStartIndex(sequence, startSelector)
	if err != nil {
		startIndex = 0
	}

	baseDir := filepath.Join(projectDir, "logs", "demo", sessionID)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create stories board directory: %w", err)
	}

	storiesFilePath := filepath.Join(baseDir, "stories_board.txt")
	statusByTestID := map[string]string{}
	for _, testCase := range sequence {
		statusByTestID[testCase.ID] = demoStatusSkip
	}
	if err := writeDemoStoriesBoard(storiesFilePath, sequence, startIndex, statusByTestID, sequence[startIndex].ID); err != nil {
		return "", err
	}

	return storiesFilePath, nil
}

func prepareCoreSessionForSplitView(ctx context.Context, coreSessionName string) error {
	const minInputPaneHeight = 3

	windowTarget := coreSessionName + ":0"

	if err := runTmux(ctx, "set-window-option", "-t", windowTarget, "window-size", "smallest"); err != nil {
		return err
	}

	zoomedFlag, err := runTmuxCapture(ctx, "display-message", "-p", "-t", windowTarget, "#{window_zoomed_flag}")
	if err != nil {
		return err
	}
	if strings.TrimSpace(zoomedFlag) == "1" {
		if err := runTmux(ctx, "resize-pane", "-t", windowTarget+".0", "-Z"); err != nil {
			return err
		}
	}

	inputTarget := windowTarget + ".2"
	inputHeightRaw, err := runTmuxCapture(ctx, "display-message", "-p", "-t", inputTarget, "#{pane_height}")
	if err != nil {
		return err
	}
	inputHeight, err := strconv.Atoi(strings.TrimSpace(inputHeightRaw))
	if err != nil {
		return fmt.Errorf("invalid input pane height %q: %w", inputHeightRaw, err)
	}
	if inputHeight < minInputPaneHeight {
		if err := runTmux(ctx, "resize-pane", "-t", inputTarget, "-y", strconv.Itoa(minInputPaneHeight)); err != nil {
			return err
		}
	}

	// In nested split-demo attach, tiled view keeps all panes represented in narrow right-pane clients.
	if err := runTmux(ctx, "select-layout", "-t", windowTarget, "tiled"); err != nil {
		return err
	}

	// Keep live-core input visible but avoid right-pane cursor dominance by focusing chat before nested attach.
	if err := runTmux(ctx, "select-pane", "-t", windowTarget+".0"); err != nil {
		return err
	}

	return nil
}

func closeCurrentTmuxSession(ctx context.Context) error {
	sessionName, err := runTmuxCapture(ctx, "display-message", "-p", "#{session_name}")
	if err != nil {
		return err
	}

	target := strings.TrimSpace(sessionName)
	if target == "" {
		return fmt.Errorf("unable to resolve current tmux session")
	}

	if err := runTmux(ctx, "kill-session", "-t", target); err != nil && !isTmuxMissingSessionError(err) {
		return err
	}
	return nil
}

func attachTmuxSession(ctx context.Context, sessionName string) error {
	return runTmuxInteractive(ctx, "attach-session", "-t", sessionName)
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func runTmuxInteractive(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux %s failed: %w", strings.Join(args, " "), err)
	}
	return nil
}

func runTmux(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func runTmuxCapture(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tmux %s failed: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func isTmuxMissingSessionError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "can't find session") || strings.Contains(lower, "no server running")
}

func waitForDemoHealthEndpoint(ctx context.Context, healthURL string) error {
	deadlineCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	for {
		if err := pingDemoHealthEndpoint(deadlineCtx, healthURL); err == nil {
			return nil
		}

		select {
		case <-deadlineCtx.Done():
			return fmt.Errorf("timed out waiting for demo health endpoint at %s", strings.TrimRight(strings.TrimSpace(healthURL), "/")+"/health")
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func pingDemoHealthEndpoint(ctx context.Context, healthURL string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(strings.TrimSpace(healthURL), "/")+"/health", nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned %s", response.Status)
	}
	return nil
}
