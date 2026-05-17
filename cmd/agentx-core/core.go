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

// InitializeTmuxSession creates the tmux session and panes.
func (ac *AgentXCore) InitializeTmuxSession(ctx context.Context) error {
	// Ensure session directories exist.
	if err := ac.Config.EnsureSessionDirs(ac.SessionID); err != nil {
		return err
	}

	// Create new tmux session.
	cmd := exec.CommandContext(ctx, "tmux", "new-session", "-d", "-s", ac.tmuxSessionName, "-x", "120", "-y", "40")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}

	// Create panes based on layout.
	layout := DefaultPaneLayout()
	for i, pane := range layout {
		if i == 0 {
			// First pane already exists; rename it.
			cmd := exec.CommandContext(ctx, "tmux", "rename-window", "-t", ac.tmuxSessionName, pane.Name)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to rename pane: %w", err)
			}
		} else {
			// Create new panes.
			cmd := exec.CommandContext(ctx, "tmux", "split-window", "-t", ac.tmuxSessionName, "-v", "-p", fmt.Sprintf("%d", 100-pane.Height))
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to split pane %d: %w", i, err)
			}
		}

		// Write placeholder text to pane.
		placeholderCmd := fmt.Sprintf("echo '🔶 Pane: %s (AgentX Core)'", pane.Name)
		cmd := exec.CommandContext(ctx, "tmux", "send-keys", "-t", ac.tmuxSessionName, placeholderCmd, "Enter")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to set placeholder in pane %s: %w", pane.Name, err)
		}
	}

	log.Printf("[AgentX Core] tmux session '%s' initialized with %d panes", ac.tmuxSessionName, len(layout))
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
