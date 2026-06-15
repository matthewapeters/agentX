// Package main configuration structures.
package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultChatBackend = "echo"
	defaultChatRuntime = "go"
	startupModeEnvKey = "AGENTX_STARTUP_MODE"
	defaultStartupMode = "default"
	visibleWindowsStartupMode = "visible-windows"
	defaultOllamaHost  = "localhost:11434"
	defaultOllamaModel = "llama3.2"
	defaultChatBridgeResponseTimeoutSeconds = 120
	defaultSubmitExecutionTimeoutSeconds    = 120
	defaultSubmitTimeoutSeconds             = 120
)

// CoreRuntimeConfig captures runtime chat backend settings used by the Go core.
type CoreRuntimeConfig struct {
	ChatBackend               string
	ChatRuntime               string
	OllamaHost                string
	OllamaModel               string
	ChatBridgeResponseTimeout time.Duration
	SubmitExecutionTimeout    time.Duration
	SubmitTimeout             time.Duration
}

// Config holds runtime configuration for AgentX Core.
type Config struct {
	ProjectDir string // Root project directory
	Username   string // Username for session isolation
	SessionID  string // Session ID; auto-generated if empty
	LayoutFile string // Optional tmuxp layout overlay file
	StartupMode string // Startup topology mode scaffold
}

func normalizeStartupMode(raw string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return defaultStartupMode, true
	}
	switch normalized {
	case defaultStartupMode, visibleWindowsStartupMode:
		return normalized, true
	default:
		return "", false
	}
}

func resolveStartupModeDefault() string {
	if mode, ok := normalizeStartupMode(os.Getenv(startupModeEnvKey)); ok {
		return mode
	}
	return defaultStartupMode
}

// PaneConfig defines a tmux pane in the layout.
type PaneConfig struct {
	Name    string            // Pane name (e.g., "chat", "logs", "input")
	Index   int               // Pane index in window
	Width   int               // Width percentage (if split horizontally; 0 = fill)
	Height  int               // Height percentage (if split vertically; 0 = fill)
	Command string            // Command to run in pane (empty = placeholder)
	Env     map[string]string // Environment variables for applet
}

// SessionDir returns the session directory path.
func (c *Config) SessionDir() string {
	return filepath.Join(c.ProjectDir, "sessions", c.Username)
}

// SessionDataDir returns the session root directory path.
func (c *Config) SessionDataDir(sessionID string) string {
	return filepath.Join(c.SessionDir(), sessionID)
}

// SessionLogDir returns the session log directory.
func (c *Config) SessionLogDir(sessionID string) string {
	return filepath.Join(c.SessionDataDir(sessionID), "logs")
}

// SessionContextDir returns the session context directory.
func (c *Config) SessionContextDir(sessionID string) string {
	return filepath.Join(c.SessionDataDir(sessionID), "context")
}

// AppletStateDir returns the session applet state directory.
// This directory stores versioned applet widget state snapshots.
func (c *Config) AppletStateDir(sessionID string) string {
	return filepath.Join(c.SessionDataDir(sessionID), "applet_state")
}

// EnsureSessionDirs creates necessary session directories.
func (c *Config) EnsureSessionDirs(sessionID string) error {
	dirs := []string{
		c.SessionDir(),
		c.SessionLogDir(sessionID),
		c.SessionContextDir(sessionID),
		c.AppletStateDir(sessionID),
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
	Name      string            // Applet name (e.g., "chat", "logs")
	PaneName  string            // Pane to display in
	Script    string            // Path to Python script
	Args      []string          // Command-line arguments
	Env       map[string]string // Environment variables
	Timeout   time.Duration     // Startup timeout
	RestartOn bool              // Auto-restart on crash (first iteration: false)
}

func defaultCoreRuntimeConfig() CoreRuntimeConfig {
	return CoreRuntimeConfig{
		ChatBackend:               defaultChatBackend,
		ChatRuntime:               defaultChatRuntime,
		OllamaHost:                defaultOllamaHost,
		OllamaModel:               defaultOllamaModel,
		ChatBridgeResponseTimeout: time.Duration(defaultChatBridgeResponseTimeoutSeconds) * time.Second,
		SubmitExecutionTimeout:    time.Duration(defaultSubmitExecutionTimeoutSeconds) * time.Second,
		SubmitTimeout:             time.Duration(defaultSubmitTimeoutSeconds) * time.Second,
	}
}

func resolveCoreRuntimeConfig(projectDir string) CoreRuntimeConfig {
	runtimeConfig := defaultCoreRuntimeConfig()
	applyAgentXTomlRuntimeConfig(projectDir, &runtimeConfig)
	applyRuntimeEnvOverrides(&runtimeConfig)
	runtimeConfig.ChatRuntime = normalizeChatRuntime(runtimeConfig.ChatRuntime)
	return runtimeConfig
}

func normalizeChatRuntime(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), appletRuntimeGo.String()) {
		return appletRuntimeGo.String()
	}
	return defaultChatRuntime
}

func applyRuntimeEnvOverrides(runtimeConfig *CoreRuntimeConfig) {
	if runtimeConfig == nil {
		return
	}

	if value := strings.TrimSpace(os.Getenv("AGENTX_CHAT_BACKEND")); value != "" {
		runtimeConfig.ChatBackend = value
	}
	if value := strings.TrimSpace(os.Getenv("AGENTX_CHAT_RUNTIME")); value != "" {
		runtimeConfig.ChatRuntime = value
	}
	if value := strings.TrimSpace(os.Getenv("AGENTX_OLLAMA_HOST")); value != "" {
		runtimeConfig.OllamaHost = value
	}
	if value := strings.TrimSpace(os.Getenv("AGENTX_OLLAMA_MODEL")); value != "" {
		runtimeConfig.OllamaModel = value
	}
	if seconds := parsePositiveSeconds(strings.TrimSpace(os.Getenv("AGENTX_CHAT_BRIDGE_RESPONSE_TIMEOUT_SEC"))); seconds > 0 {
		runtimeConfig.ChatBridgeResponseTimeout = time.Duration(seconds) * time.Second
	}
	if seconds := parsePositiveSeconds(strings.TrimSpace(os.Getenv("AGENTX_SUBMIT_EXEC_TIMEOUT_SEC"))); seconds > 0 {
		runtimeConfig.SubmitExecutionTimeout = time.Duration(seconds) * time.Second
	}
	if seconds := parsePositiveSeconds(strings.TrimSpace(os.Getenv("AGENTX_SUBMIT_TIMEOUT_SEC"))); seconds > 0 {
		runtimeConfig.SubmitTimeout = time.Duration(seconds) * time.Second
	}
}

func applyAgentXTomlRuntimeConfig(projectDir string, runtimeConfig *CoreRuntimeConfig) {
	if runtimeConfig == nil {
		return
	}

	configPath := filepath.Join(projectDir, "agentx.toml")
	file, err := os.Open(configPath)
	if err != nil {
		return
	}
	defer file.Close()

	currentSection := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			continue
		}

		key, value, ok := parseTomlKeyValue(line)
		if !ok {
			continue
		}

		switch currentSection {
		case "agentx":
			switch key {
			case "ollama_host":
				runtimeConfig.OllamaHost = value
			case "ollama_model":
				runtimeConfig.OllamaModel = value
			case "chat_backend":
				runtimeConfig.ChatBackend = value
			case "chat_runtime":
				runtimeConfig.ChatRuntime = value
			case "chat_bridge_response_timeout_seconds":
				if seconds := parsePositiveSeconds(value); seconds > 0 {
					runtimeConfig.ChatBridgeResponseTimeout = time.Duration(seconds) * time.Second
				}
			case "submit_execution_timeout_seconds":
				if seconds := parsePositiveSeconds(value); seconds > 0 {
					runtimeConfig.SubmitExecutionTimeout = time.Duration(seconds) * time.Second
				}
			case "submit_timeout_seconds":
				if seconds := parsePositiveSeconds(value); seconds > 0 {
					runtimeConfig.SubmitTimeout = time.Duration(seconds) * time.Second
				}
			}
		case "agentix":
			if key == "chat_backend" {
				runtimeConfig.ChatBackend = value
			}
			if key == "chat_runtime" {
				runtimeConfig.ChatRuntime = value
			}
		}
	}
}

func parsePositiveSeconds(raw string) int64 {
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func parseTomlKeyValue(line string) (string, string, bool) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}

	key := strings.TrimSpace(parts[0])
	rawValue := strings.TrimSpace(parts[1])
	if key == "" || rawValue == "" {
		return "", "", false
	}

	if strings.HasPrefix(rawValue, "[") || strings.HasPrefix(rawValue, "{") {
		return "", "", false
	}

	if hash := strings.Index(rawValue, "#"); hash >= 0 {
		rawValue = strings.TrimSpace(rawValue[:hash])
	}
	if rawValue == "" {
		return "", "", false
	}

	if strings.HasPrefix(rawValue, "\"") || strings.HasPrefix(rawValue, "'") {
		unquoted, err := strconv.Unquote(rawValue)
		if err == nil {
			return key, strings.TrimSpace(unquoted), true
		}
		rawValue = strings.Trim(rawValue, "\"'")
	}

	trimmed := strings.TrimSpace(rawValue)
	if trimmed == "" {
		return "", "", false
	}

	return key, trimmed, true
}

// DefaultPaneLayout returns the initial tmux layout (pane placeholders).
// Layout: Chat (80%x80% top-left) | Context (20%x80% top-right)
//
//	Input (100%x20% bottom)
//	Logs (hidden in separate window)
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
			Name:  "logs",
			Index: 3,
			// Logs pane: created in separate hidden window
		},
	}
}
