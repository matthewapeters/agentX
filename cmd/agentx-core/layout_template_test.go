package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteTmuxpLayoutTemplate_WritesExpectedContent(t *testing.T) {
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "layouts", "agentx-layout.yaml")

	if err := writeTmuxpLayoutTemplate(templatePath); err != nil {
		t.Fatalf("writeTmuxpLayoutTemplate failed: %v", err)
	}

	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("failed to read template file: %v", err)
	}
	content := string(data)

	required := []string{
		"session_name: ${SESSION}",
		"window_name: tui-chat",
		"window_name: logs",
		"automatic-rename: \"off\"",
	}
	for _, fragment := range required {
		if !strings.Contains(content, fragment) {
			t.Fatalf("expected template to contain %q, got:\n%s", fragment, content)
		}
	}
}

func TestWriteTmuxpLayoutTemplate_RejectsEmptyPath(t *testing.T) {
	if err := writeTmuxpLayoutTemplate("   "); err == nil {
		t.Fatalf("expected empty template path to fail")
	}
}
