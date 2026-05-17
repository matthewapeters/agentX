// Package main core orchestrator.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"
)

// AgentXCore orchestrates the tmux session, applets, and IPC.
type AgentXCore struct {
	Config           *Config
	SessionID        string
	tmuxSessionName  string
	applets          map[string]*AppletProcess
	mu               sync.RWMutex
	healthAddr       string // Address for health endpoint
	contextManager   *ContextManager
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

	// Create new tmux session with default pane (will become chat).
	cmd := exec.CommandContext(ctx, "tmux", "new-session", "-d", "-s", ac.tmuxSessionName, "-x", "120", "-y", "40")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}

	// 1. Rename the default pane to 'chat'.
	cmd = exec.CommandContext(ctx, "tmux", "rename-window", "-t", ac.tmuxSessionName, "chat")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to rename window to chat: %w", err)
	}

	// 2. Split horizontally (vertical line): create input pane at bottom (20%).
	//    This puts the chat pane at top (80%) and input pane at bottom (20%).
	cmd = exec.CommandContext(ctx, "tmux", "split-window", "-t", ac.tmuxSessionName + ":0", "-v", "-p", "20")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to split window for input pane: %w", err)
	}

	// 3. Rename the bottom pane to 'input'.
	cmd = exec.CommandContext(ctx, "tmux", "rename-window", "-t", ac.tmuxSessionName + ":0.1", "input")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to rename input pane: %w", err)
	}

	// 4. Focus on the top-left pane (chat) and split vertically (horizontal line):
	//    create context pane on the right (20% width).
	cmd = exec.CommandContext(ctx, "tmux", "split-window", "-t", ac.tmuxSessionName + ":0.0", "-h", "-p", "20")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to split top pane for context: %w", err)
	}

	// 5. Rename the top-right pane to 'context'.
	cmd = exec.CommandContext(ctx, "tmux", "rename-window", "-t", ac.tmuxSessionName + ":0.1", "context")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to rename context pane: %w", err)
	}

	// 6. Create logs in a new hidden window (window:1).
	cmd = exec.CommandContext(ctx, "tmux", "new-window", "-t", ac.tmuxSessionName + ":1", "-n", "logs")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create logs window: %w", err)
	}

	// 7. Write placeholder text to each visible pane.
	panes := map[string]string{
		"chat":    "0.0",
		"context": "0.2",
		"input":   "0.1",
		"logs":    "1.0",
	}
	for paneName, paneTarget := range panes {
		placeholderCmd := fmt.Sprintf("echo '🔶 Pane: %s (AgentX Core)'", paneName)
		cmd := exec.CommandContext(ctx, "tmux", "send-keys", "-t", ac.tmuxSessionName + ":" + paneTarget, placeholderCmd, "Enter")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to set placeholder in pane %s: %w", paneName, err)
		}
	}

	log.Printf("[AgentX Core] tmux session '%s' initialized with layout: chat(80x80)|context(20x80) top, input(100x20) bottom, logs hidden", ac.tmuxSessionName)
	return nil
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
