// Package main configuration structures.
package main

import (
	"os"
	"path/filepath"
	"time"
)

// Config holds runtime configuration for AgentX Core.
type Config struct {
	ProjectDir string // Root project directory
	Username   string // Username for session isolation
	SessionID  string // Session ID; auto-generated if empty
}

// PaneConfig defines a tmux pane in the layout.
type PaneConfig struct {
	Name       string        // Pane name (e.g., "chat", "logs", "input")
	Index      int           // Pane index in window
	Width      int           // Width percentage (if split horizontally; 0 = fill)
	Height     int           // Height percentage (if split vertically; 0 = fill)
	Command    string        // Command to run in pane (empty = placeholder)
	Env        map[string]string // Environment variables for applet
}

// SessionDir returns the session directory path.
func (c *Config) SessionDir() string {
	return filepath.Join(c.ProjectDir, "sessions", c.Username)
}

// SessionLogDir returns the session log directory.
func (c *Config) SessionLogDir(sessionID string) string {
	return filepath.Join(c.SessionDir(), sessionID, "logs")
}

// SessionContextDir returns the session context directory.
func (c *Config) SessionContextDir(sessionID string) string {
	return filepath.Join(c.SessionDir(), sessionID, "context")
}

// EnsureSessionDirs creates necessary session directories.
func (c *Config) EnsureSessionDirs(sessionID string) error {
	dirs := []string{
		c.SessionDir(),
		c.SessionLogDir(sessionID),
		c.SessionContextDir(sessionID),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// AppletConfig defines a Python applet to be launched.
type AppletConfig struct {
	Name      string        // Applet name (e.g., "chat", "logs")
	PaneName  string        // Pane to display in
	Script    string        // Path to Python script
	Args      []string      // Command-line arguments
	Env       map[string]string // Environment variables
	Timeout   time.Duration // Startup timeout
	RestartOn bool          // Auto-restart on crash (first iteration: false)
}

// DefaultPaneLayout returns the initial tmux layout (pane placeholders).
// Layout: Chat (80%x80% top-left) | Context (20%x80% top-right)
//         Input (100%x20% bottom)
//         Logs (hidden in separate window)
func DefaultPaneLayout() []PaneConfig {
	return []PaneConfig{
		{
			Name:   "chat",
			Index:  0,
			Width:  80, // 80% of window width (left, after vertical split)
			Height: 80, // 80% of window height (top)
		},
		{
			Name:   "context",
			Index:  1,
			Width:  20, // 20% of window width (right, after vertical split)
			Height: 80, // 80% of window height (top)
		},
		{
			Name:   "input",
			Index:  2,
			Height: 20, // 20% of window height (bottom)
		},
		{
			Name:   "logs",
			Index:  3,
			// Logs pane: created in separate hidden window
		},
	}
}
