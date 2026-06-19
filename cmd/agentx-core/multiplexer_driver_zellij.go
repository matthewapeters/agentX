package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ZellijMultiplexerDriver is the Phase 1 zellij multiplexer backend implementation.
// Phase 1 provides structural scaffolding with placeholder implementations.
// Phase 2 will implement actual command execution logic.
type ZellijMultiplexerDriver struct {
	configDir string
}

func NewZellijMultiplexerDriver(projectDir ...string) *ZellijMultiplexerDriver {
	driver := &ZellijMultiplexerDriver{}
	if len(projectDir) > 0 {
		driver.configDir = zellijConfigDirectory(projectDir[0])
	}
	return driver
}

func (d *ZellijMultiplexerDriver) BackendName() string {
	return "zellij"
}

func (d *ZellijMultiplexerDriver) Run(ctx context.Context, args ...string) error {
	cmd, err := d.command(ctx, args...)
	if err != nil {
		return err
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func (d *ZellijMultiplexerDriver) RunCombined(ctx context.Context, args ...string) (string, error) {
	cmd, err := d.command(ctx, args...)
	if err != nil {
		return "", err
	}
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func (d *ZellijMultiplexerDriver) Capture(ctx context.Context, args ...string) (string, error) {
	cmd, err := d.command(ctx, args...)
	if err != nil {
		return "", err
	}
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (d *ZellijMultiplexerDriver) AttachSession(ctx context.Context, sessionName string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	// zellij 0.40+: 'attach' takes the session name as a positional argument, not a flag.
	cmd, err := d.command(ctx, "attach", sessionName)
	if err != nil {
		return err
	}
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func (d *ZellijMultiplexerDriver) command(ctx context.Context, args ...string) (*exec.Cmd, error) {
	commandArgs := args
	if strings.TrimSpace(d.configDir) != "" {
		if err := os.MkdirAll(d.configDir, 0o755); err != nil {
			return nil, err
		}
		if err := ensureZellijConfigFile(d.configDir); err != nil {
			return nil, err
		}
		commandArgs = append([]string{"--config-dir", d.configDir}, args...)
	}
	return exec.CommandContext(ctx, "zellij", commandArgs...), nil
}

func zellijConfigDirectory(projectDir string) string {
	trimmedProjectDir := strings.TrimSpace(projectDir)
	if trimmedProjectDir == "" {
		trimmedProjectDir = "."
	}
	return filepath.Join(trimmedProjectDir, ".agentx")
}

func ensureZellijConfigFile(configDir string) error {
	configPath := filepath.Join(configDir, "config.kdl")
	if _, err := os.Stat(configPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	copyCmd := zellijCopyCommand()
	if copyCmd == "" {
		log.Printf("[AgentX Core] ℹ Auto-generated .agentx/config.kdl with clipboard auto-copy (no clipboard helper found)")
	} else {
		log.Printf("[AgentX Core] ℹ Auto-generated .agentx/config.kdl with clipboard auto-copy (%s)", extractCopyToolName(copyCmd))
	}
	return os.WriteFile(configPath, []byte(defaultZellijConfigKDL()), 0o644)
}

func defaultZellijConfigKDL() string {
	lines := []string{
		"copy_on_select true",
		"copy_clipboard \"system\"",
	}
	if copyCommand := zellijCopyCommand(); copyCommand != "" {
		lines = append(lines, fmt.Sprintf("copy_command %q", copyCommand))
	}
	return strings.Join(lines, "\n") + "\n"
}

// zellijCopyCommand detects the system's clipboard helper.
// Prefers wl-copy on Wayland, falls back to xclip on X11.
// Clipboard detection occurs at instantiation time; mid-session X11↔Wayland switches
// are not supported because zellij requires static configuration files.
func zellijCopyCommand() string {
	if hasExecutable("wl-copy") && (strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) != "" || strings.EqualFold(strings.TrimSpace(os.Getenv("XDG_SESSION_TYPE")), "wayland")) {
		return "wl-copy"
	}
	if hasExecutable("xclip") {
		return "xclip -selection clipboard"
	}
	return ""
}

func extractCopyToolName(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) > 0 {
		return parts[0]
	}
	return cmd
}

func hasExecutable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
