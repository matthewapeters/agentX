package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

func writeTmuxpLayoutTemplate(path string) error {
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

	if err := os.WriteFile(trimmedPath, []byte(tmuxpLayoutTemplate), 0o644); err != nil {
		return fmt.Errorf("failed to write tmuxp layout template: %w", err)
	}

	return nil
}
