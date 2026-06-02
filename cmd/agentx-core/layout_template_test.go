package main

import (
	"bytes"
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

func TestEnsureDefaultLayoutFile_MaterializesFile(t *testing.T) {
	projectDir := t.TempDir()
	path, err := ensureDefaultLayoutFile(projectDir)
	if err != nil {
		t.Fatalf("ensureDefaultLayoutFile failed: %v", err)
	}

	expectedPath := filepath.Join(projectDir, defaultLayoutRelativePath)
	if path != expectedPath {
		t.Fatalf("default layout path mismatch: got %q want %q", path, expectedPath)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read materialized default layout file: %v", err)
	}
	if !strings.Contains(string(data), "session_name: ${SESSION}") {
		t.Fatalf("expected default layout content to include session_name, got:\n%s", string(data))
	}
}

func TestDumpDefaultLayout_Stdout(t *testing.T) {
	var out bytes.Buffer
	if err := dumpDefaultLayout("-", &out); err != nil {
		t.Fatalf("dumpDefaultLayout failed: %v", err)
	}
	if !strings.Contains(out.String(), "window_name: tui-chat") {
		t.Fatalf("expected dumped layout content, got:\n%s", out.String())
	}
}

func TestDumpDefaultLayout_File(t *testing.T) {
	path := filepath.Join(t.TempDir(), "default-layout.yaml")
	if err := dumpDefaultLayout(path, nil); err != nil {
		t.Fatalf("dumpDefaultLayout file write failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read dumped default layout file: %v", err)
	}
	if !strings.Contains(string(data), "window_name: logs") {
		t.Fatalf("expected default layout logs window, got:\n%s", string(data))
	}
}
