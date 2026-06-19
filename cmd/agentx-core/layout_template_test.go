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

func TestResolveImplicitLayoutFile_PrefersBackendSpecificTmuxLayout(t *testing.T) {
	projectDir := t.TempDir()
	path, ok := backendLayoutFilePath(projectDir, defaultMultiplexerBackend)
	if !ok {
		t.Fatalf("expected tmux backend layout path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create layout dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("session_name: ${SESSION}\n"), 0o644); err != nil {
		t.Fatalf("failed to write tmux layout file: %v", err)
	}

	resolved, err := resolveImplicitLayoutFile(projectDir, defaultMultiplexerBackend)
	if err != nil {
		t.Fatalf("resolveImplicitLayoutFile failed: %v", err)
	}
	if resolved != path {
		t.Fatalf("resolved tmux layout mismatch: got %q want %q", resolved, path)
	}
}

func TestResolveImplicitLayoutFile_FallsBackToLegacyTmuxLayout(t *testing.T) {
	projectDir := t.TempDir()
	legacyPath := defaultLayoutFilePath(projectDir)
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("failed to create legacy layout dir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("session_name: ${SESSION}\n"), 0o644); err != nil {
		t.Fatalf("failed to write legacy layout file: %v", err)
	}

	resolved, err := resolveImplicitLayoutFile(projectDir, defaultMultiplexerBackend)
	if err != nil {
		t.Fatalf("resolveImplicitLayoutFile failed: %v", err)
	}
	if resolved != legacyPath {
		t.Fatalf("resolved legacy tmux layout mismatch: got %q want %q", resolved, legacyPath)
	}
}

func TestResolveImplicitLayoutFile_MaterializesDefaultTmuxLayout(t *testing.T) {
	projectDir := t.TempDir()
	resolved, err := resolveImplicitLayoutFile(projectDir, defaultMultiplexerBackend)
	if err != nil {
		t.Fatalf("resolveImplicitLayoutFile failed: %v", err)
	}
	if resolved != defaultLayoutFilePath(projectDir) {
		t.Fatalf("resolved default tmux layout mismatch: got %q want %q", resolved, defaultLayoutFilePath(projectDir))
	}
	if _, err := os.Stat(resolved); err != nil {
		t.Fatalf("expected materialized tmux default layout file, got %v", err)
	}
}

func TestResolveImplicitLayoutFile_ZellijRequiresBackendSpecificLayout(t *testing.T) {
	projectDir := t.TempDir()
	_, err := resolveImplicitLayoutFile(projectDir, "zellij")
	if err == nil {
		t.Fatalf("expected missing zellij layout to fail")
	}
	if !strings.Contains(err.Error(), "zellij-layout.kdl") {
		t.Fatalf("expected missing zellij layout path in error, got %v", err)
	}
}

func TestResolveImplicitLayoutFile_ZellijUsesBackendSpecificLayout(t *testing.T) {
	projectDir := t.TempDir()
	path, ok := backendLayoutFilePath(projectDir, "zellij")
	if !ok {
		t.Fatalf("expected zellij backend layout path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create zellij layout dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("layout {}\n"), 0o644); err != nil {
		t.Fatalf("failed to write zellij layout file: %v", err)
	}

	resolved, err := resolveImplicitLayoutFile(projectDir, "zellij")
	if err != nil {
		t.Fatalf("resolveImplicitLayoutFile failed: %v", err)
	}
	if resolved != path {
		t.Fatalf("resolved zellij layout mismatch: got %q want %q", resolved, path)
	}
}
