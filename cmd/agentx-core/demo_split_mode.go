package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
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
	controllerArgs := buildDemoControllerArgs(executablePath, cfg, core.SessionID, startSelector, core.tmuxSessionName)

	if err := prepareCoreSessionForSplitView(ctx, core.tmuxSessionName); err != nil {
		return fmt.Errorf("failed to prepare core session for split view: %w", err)
	}

	if err := runTmuxInteractive(ctx, append([]string{"new-session", "-d", "-s", demoSessionName, "-n", "demo-control"}, controllerArgs...)...); err != nil {
		return fmt.Errorf("failed to create demo controller session: %w", err)
	}

	liveCoreMirrorArgs := buildLiveCoreMirrorArgs(core.tmuxSessionName)
	if err := runTmuxInteractive(ctx, append([]string{"split-window", "-h", "-p", "45", "-t", demoSessionName + ":0"}, liveCoreMirrorArgs...)...); err != nil {
		return fmt.Errorf("failed to create live core mirror pane: %w", err)
	}

	if err := runTmuxInteractive(ctx, "select-pane", "-t", demoSessionName+":0.0", "-T", "controller"); err != nil {
		return fmt.Errorf("failed to label controller pane: %w", err)
	}
	if err := runTmuxInteractive(ctx, "select-pane", "-t", demoSessionName+":0.1", "-T", "live-core"); err != nil {
		return fmt.Errorf("failed to label live core pane: %w", err)
	}
	if err := runTmuxInteractive(ctx, "select-pane", "-t", demoSessionName+":0.0"); err != nil {
		return fmt.Errorf("failed to focus controller pane: %w", err)
	}

	fmt.Printf("[AgentX Demo] Split demo session initialized: %s\n", demoSessionName)
	fmt.Printf("[AgentX Demo] Left pane: controller prompt loop\n")
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

func buildDemoControllerArgs(executablePath string, cfg *Config, sessionID, startSelector, coreSessionName string) []string {
	args := []string{
		"--project-dir", cfg.ProjectDir,
		"--user", cfg.Username,
		"--session-id", sessionID,
		"--demo-controller",
		"--demo-core-session", coreSessionName,
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

	// Make the input pane the active pane before nested attach so the prompt is visible on entry.
	if err := runTmux(ctx, "select-pane", "-t", inputTarget); err != nil {
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
