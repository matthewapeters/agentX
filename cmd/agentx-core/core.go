// Package main core orchestrator.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type tmuxPaneTarget struct {
	name   string
	target string
}

// AgentXCore orchestrates the tmux session, applets, and IPC.
type AgentXCore struct {
	Config          *Config
	SessionID       string
	tmuxSessionName string
	applets         map[string]*AppletProcess
	mu              sync.RWMutex
	healthAddr      string // Address for health endpoint
	contextManager  *ContextManager
}

// AppletProcess tracks a running Python applet.
type AppletProcess struct {
	Name       string
	PaneName   string
	Cmd        *exec.Cmd
	Cancel     context.CancelFunc
	DoneChan   chan error
	StartedAt  time.Time
	CrashCount int
}

// NewAgentXCore creates a new orchestrator instance.
func NewAgentXCore(cfg *Config) *AgentXCore {
	sessionID := cfg.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("agentx_%d", time.Now().Unix())
	}

	return &AgentXCore{
		Config:          cfg,
		SessionID:       sessionID,
		tmuxSessionName: fmt.Sprintf("agentx_%s_%s", cfg.Username, sessionID),
		applets:         make(map[string]*AppletProcess),
		healthAddr:      "127.0.0.1:9876", // Default health endpoint
		contextManager:  NewContextManager(cfg.SessionContextDir(sessionID)),
	}
}

// InitializeTmuxSession creates the tmux session and panes with the designed layout:
// Top (80% height): Chat (80% width left) | Context (20% width right)
// Bottom (20% height): Input (full width)
// Separate window: Logs (hidden, navigable via ctrl-b)
func (ac *AgentXCore) InitializeTmuxSession(ctx context.Context) error {
	// Ensure session directories exist.
	if err := ac.Config.EnsureSessionDirs(ac.SessionID); err != nil {
		return err
	}

	if err := ac.runTmux(ctx, buildNewSessionCommand(ac.tmuxSessionName)...); err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}

	chatPaneTarget := ac.tmuxSessionName + ":0.0"
	if err := ac.runTmux(ctx, "select-pane", "-t", chatPaneTarget, "-T", "chat"); err != nil {
		return fmt.Errorf("failed to set chat pane title: %w", err)
	}

	inputPaneTarget, err := ac.runTmuxCapture(ctx, buildInputSplitCommand(chatPaneTarget)...)
	if err != nil {
		return fmt.Errorf("failed to split input pane: %w", err)
	}
	if err := ac.runTmux(ctx, "select-pane", "-t", inputPaneTarget, "-T", "input"); err != nil {
		return fmt.Errorf("failed to set input pane title: %w", err)
	}

	contextPaneTarget, err := ac.runTmuxCapture(ctx, buildContextSplitCommand(chatPaneTarget)...)
	if err != nil {
		return fmt.Errorf("failed to split context pane: %w", err)
	}
	if err := ac.runTmux(ctx, "select-pane", "-t", contextPaneTarget, "-T", "context"); err != nil {
		return fmt.Errorf("failed to set context pane title: %w", err)
	}

	if err := ac.runTmux(ctx, "new-window", "-t", ac.tmuxSessionName+":1", "-n", "logs"); err != nil {
		return fmt.Errorf("failed to create logs window: %w", err)
	}

	if err := ac.runTmux(ctx, "select-window", "-t", ac.tmuxSessionName+":0"); err != nil {
		return fmt.Errorf("failed to re-select primary window: %w", err)
	}

	for _, pane := range paneTargets(ac.tmuxSessionName, chatPaneTarget, inputPaneTarget, contextPaneTarget) {
		placeholderCmd := fmt.Sprintf("echo '🔶 Pane: %s (AgentX Core)'", pane.name)
		if err := ac.runTmux(ctx, "send-keys", "-t", pane.target, placeholderCmd, "Enter"); err != nil {
			return fmt.Errorf("failed to set placeholder in pane %s: %w", pane.name, err)
		}
	}

	log.Printf("[AgentX Core] tmux session '%s' initialized with layout: chat(80x80)|context(20x80) top, input(100x20) bottom, logs hidden", ac.tmuxSessionName)
	return nil
}

func (ac *AgentXCore) runTmux(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	return cmd.Run()
}

func (ac *AgentXCore) runTmuxCapture(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "tmux", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func paneTargets(sessionName, chatTarget, inputTarget, contextTarget string) []tmuxPaneTarget {
	return []tmuxPaneTarget{
		{name: "chat", target: chatTarget},
		{name: "input", target: inputTarget},
		{name: "context", target: contextTarget},
		{name: "logs", target: sessionName + ":1.0"},
	}
}

func buildNewSessionCommand(sessionName string) []string {
	return []string{"new-session", "-d", "-s", sessionName, "-n", "tui-chat", "-x", "120", "-y", "40"}
}

func buildInputSplitCommand(chatPaneTarget string) []string {
	return []string{"split-window", "-P", "-F", "#{pane_id}", "-t", chatPaneTarget, "-v", "-p", "20"}
}

func buildContextSplitCommand(chatPaneTarget string) []string {
	return []string{"split-window", "-P", "-F", "#{pane_id}", "-t", chatPaneTarget, "-h", "-p", "20"}
}

// StartAppletSupervisor launches Python applets in goroutines.
func (ac *AgentXCore) StartAppletSupervisor(ctx context.Context) error {
	// In first iteration, applets are placeholders.
	// They will be populated as functionality migrates from Python.
	log.Printf("[AgentX Core] Applet supervisor ready (0 applets in first iteration)")
	return nil
}

// StartHealthEndpoint starts the health/status HTTP endpoint.
func (ac *AgentXCore) StartHealthEndpoint(ctx context.Context) error {
	go func() {
		if err := ac.contextManager.ServeHealth(ctx, ac.healthAddr); err != nil {
			log.Printf("[AgentX Core] Health endpoint error: %v", err)
		}
	}()
	log.Printf("[AgentX Core] Health endpoint listening on %s", ac.healthAddr)
	return nil
}

// AttachTmuxSession attaches the current terminal to the managed tmux session.
func (ac *AgentXCore) AttachTmuxSession(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "tmux", "attach-session", "-t", ac.tmuxSessionName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to attach tmux session: %w", err)
	}
	return nil
}

// Shutdown gracefully stops all goroutines, applets, and tmux session.
func (ac *AgentXCore) Shutdown(ctx context.Context) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	// Stop all applets.
	for name, applet := range ac.applets {
		if applet.Cancel != nil {
			applet.Cancel()
		}
		if applet.Cmd != nil && applet.Cmd.Process != nil {
			applet.Cmd.Process.Kill()
		}
		log.Printf("[AgentX Core] Stopped applet: %s", name)
	}

	// Kill tmux session.
	cmd := exec.CommandContext(ctx, "tmux", "kill-session", "-t", ac.tmuxSessionName)
	if err := cmd.Run(); err != nil {
		log.Printf("[AgentX Core] Warning: failed to kill tmux session: %v", err)
	}

	log.Printf("[AgentX Core] Shutdown complete")
	return nil
}

// ContextManager handles session context (state, messages, tools).
type ContextManager struct {
	contextDir string
}

// NewContextManager creates a new context manager.
func NewContextManager(contextDir string) *ContextManager {
	return &ContextManager{contextDir: contextDir}
}

// ServeHealth starts an HTTP health endpoint.
func (cm *ContextManager) ServeHealth(ctx context.Context, addr string) error {
	// TODO: Implement HTTP health endpoint with chi router.
	// Endpoints:
	//   GET /health -> { "status": "ok", "session_id": "...", "uptime": "..." }
	//   GET /panes -> list of active panes
	//   GET /applets -> list of running applets
	//   POST /request-focus?pane=chat -> request focus change
	select {
	case <-ctx.Done():
		return ctx.Err()
	}
}
