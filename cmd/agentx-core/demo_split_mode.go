package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
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

	if err := runTmuxInteractive(ctx, append([]string{"new-session", "-d", "-s", demoSessionName, "-n", "demo-control"}, controllerArgs...)...); err != nil {
		return fmt.Errorf("failed to create demo controller session: %w", err)
	}

	liveCoreMirrorArgs := buildLiveCoreMirrorArgs(core.tmuxSessionName)
	if err := runTmuxInteractive(ctx, append([]string{"split-window", "-h", "-t", demoSessionName + ":0"}, liveCoreMirrorArgs...)...); err != nil {
		return fmt.Errorf("failed to create live core mirror pane: %w", err)
	}

	if err := runTmuxInteractive(ctx, "select-pane", "-t", demoSessionName+":0.0", "-T", "controller"); err != nil {
		return fmt.Errorf("failed to label controller pane: %w", err)
	}
	if err := runTmuxInteractive(ctx, "select-pane", "-t", demoSessionName+":0.1", "-T", "live-core-mirror"); err != nil {
		return fmt.Errorf("failed to label live core pane: %w", err)
	}
	if err := runTmuxInteractive(ctx, "select-layout", "-t", demoSessionName+":0", "even-horizontal"); err != nil {
		return fmt.Errorf("failed to set demo layout: %w", err)
	}
	if err := runTmuxInteractive(ctx, "select-pane", "-t", demoSessionName+":0.0"); err != nil {
		return fmt.Errorf("failed to focus controller pane: %w", err)
	}

	fmt.Printf("[AgentX Demo] Split demo session initialized: %s\n", demoSessionName)
	fmt.Printf("[AgentX Demo] Left pane: controller prompt loop\n")
	fmt.Printf("[AgentX Demo] Right pane: live core mirror for session %s\n", core.tmuxSessionName)
	fmt.Printf("[AgentX Demo] Attach to the split demo session with: tmux attach -t %s\n", demoSessionName)

	attachErr := attachTmuxSession(ctx, demoSessionName)
	if killErr := runTmuxInteractive(ctx, "kill-session", "-t", demoSessionName); killErr != nil && !isTmuxMissingSessionError(killErr) {
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
	mirrorScript := fmt.Sprintf(
		`session=%s; last=''; while true; do panes="$(tmux list-panes -t "$session:0" -F '#{pane_index}|#{pane_title}|#{pane_id}' 2>/dev/null || true)"; frame="$(printf 'Live core mirror: %%s' "$session")"; while IFS='|' read -r pane_idx pane_title pane_id; do [[ -z "$pane_id" ]] && continue; snippet="$(tmux capture-pane -p -t "$pane_id" 2>/dev/null | awk 'NF{buf[++n]=$0} END{if(n==0){print "<no output>"; exit} start=n-10; if(start<1){start=1} for(i=start;i<=n;i++){print buf[i]}}')"; frame="$frame\n\n=== pane $pane_idx (${pane_title:-untitled}, $pane_id) ===\n$snippet"; done <<< "$panes"; if [[ "$frame" != "$last" ]]; then printf '\n[%%s]\n%%b\n' "$(date '+%%H:%%M:%%S')" "$frame"; last="$frame"; fi; sleep 0.8; done`,
		shellQuote(coreSessionName),
	)
	return []string{"bash", "-lc", mirrorScript}
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
