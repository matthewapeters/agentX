package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const defaultLayoutRelativePath = ".agentx/layouts/default-layout.yaml"

const backendLayoutDirRelativePath = ".agentx/layouts"

const tmuxpLayoutTemplate = `# AgentX tmuxp layout template
#
# Usage:
#   export SESSION="<running-agentx-session>"
#   tmuxp load -y this-file.yaml
#
# AgentX owns session + required windows. This file is for pane topology overlays.

session_name: ${SESSION}
windows:
  - window_name: tui-chat
    options:
      automatic-rename: "off"
    panes:
      - shell_command: ""
      - shell_command: ""
      - shell_command: ""
  - window_name: logs
    options:
      automatic-rename: "off"
    panes:
      - shell_command: ""
`

const tmuxpDefaultLayout = `# AgentX default tmuxp composition
#
# This file is used as the implicit --layout when no custom layout is provided.
# It is designed to reproduce the current default runtime windows/panes.

session_name: ${SESSION}
windows:
  - window_name: tui-chat
    options:
      automatic-rename: "off"
    panes:
      - shell_command: ""
      - shell_command: ""
      - shell_command: ""
  - window_name: logs
    options:
      automatic-rename: "off"
    panes:
      - shell_command: ""
`

var backendLayoutExtensions = map[string]string{
	defaultMultiplexerBackend: ".yaml",
	"zellij":                ".kdl",
}

func backendLayoutDirectory(projectDir string) string {
	trimmedProjectDir := strings.TrimSpace(projectDir)
	if trimmedProjectDir == "" {
		trimmedProjectDir = "."
	}
	return filepath.Join(trimmedProjectDir, backendLayoutDirRelativePath)
}

func backendLayoutExtension(backendName string) (string, bool) {
	ext, ok := backendLayoutExtensions[strings.ToLower(strings.TrimSpace(backendName))]
	return ext, ok
}

func backendLayoutFilePath(projectDir string, backendName string) (string, bool) {
	ext, ok := backendLayoutExtension(backendName)
	if !ok {
		return "", false
	}
	return filepath.Join(backendLayoutDirectory(projectDir), strings.ToLower(strings.TrimSpace(backendName))+"-layout"+ext), true
}

// resolveImplicitLayoutFile picks the backend-native layout file when the user did not
// explicitly pass --layout/--layout-file. The deterministic pattern is:
// .agentx/layouts/{backend}-layout.{ext}. Tmux preserves backward compatibility by
// falling back to default-layout.yaml and materializing it for legacy installs.
// Zellij requires zellij-layout.kdl so missing files fail deterministically.
func resolveImplicitLayoutFile(projectDir string, backendName string) (string, error) {
	normalizedBackend := strings.ToLower(strings.TrimSpace(backendName))
	specificPath, ok := backendLayoutFilePath(projectDir, normalizedBackend)
	if !ok {
		return "", nil
	}
	if _, err := os.Stat(specificPath); err == nil {
		return specificPath, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to stat %s layout file: %w", normalizedBackend, err)
	}

	if normalizedBackend == defaultMultiplexerBackend {
		legacyPath := defaultLayoutFilePath(projectDir)
		if _, err := os.Stat(legacyPath); err == nil {
			return legacyPath, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("failed to stat legacy tmux layout file: %w", err)
		}
		return ensureDefaultLayoutFile(projectDir)
	}

	return "", fmt.Errorf("layout file not found for backend %q: expected %s", normalizedBackend, specificPath)
}

func defaultLayoutFilePath(projectDir string) string {
	trimmedProjectDir := strings.TrimSpace(projectDir)
	if trimmedProjectDir == "" {
		trimmedProjectDir = "."
	}
	return filepath.Join(trimmedProjectDir, defaultLayoutRelativePath)
}

func ensureDefaultLayoutFile(projectDir string) (string, error) {
	path := defaultLayoutFilePath(projectDir)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to stat default layout file: %w", err)
	}

	if err := writeLayoutFile(path, tmuxpDefaultLayout); err != nil {
		return "", err
	}
	return path, nil
}

func dumpDefaultLayout(path string, out io.Writer) error {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return fmt.Errorf("dump path cannot be empty")
	}
	if trimmedPath == "-" {
		if out == nil {
			return fmt.Errorf("output writer cannot be nil when dump path is '-'" )
		}
		_, err := io.WriteString(out, tmuxpDefaultLayout)
		return err
	}

	return writeLayoutFile(trimmedPath, tmuxpDefaultLayout)
}

func writeTmuxpLayoutTemplate(path string) error {
	return writeLayoutFile(path, tmuxpLayoutTemplate)
}

func writeLayoutFile(path string, content string) error {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return fmt.Errorf("layout template path cannot be empty")
	}

	dir := filepath.Dir(trimmedPath)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create template directory: %w", err)
		}
	}

	if err := os.WriteFile(trimmedPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write tmuxp layout template: %w", err)
	}

	return nil
}
